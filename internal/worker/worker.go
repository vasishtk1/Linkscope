package worker

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vasishtk1/Linkscope/internal/api"
	"github.com/vasishtk1/Linkscope/internal/extract"
	"github.com/vasishtk1/Linkscope/internal/jobs"
	"github.com/vasishtk1/Linkscope/internal/links"
)

type Config struct {
	ID          string
	Coordinator string
	Source      string
	OutDir      string
	Parsers     int
	// PollInterval is how long to wait when no job is available yet.
	PollInterval time.Duration
	// FetchRetries is how many times a download is retried before the job is failed.
	FetchRetries int
	RetryBackoff time.Duration
	// RequestTimeout limits each call to the coordinator.
	RequestTimeout time.Duration
	// FirstByteTimeout limits how long Common Crawl can take to start sending a file.
	FirstByteTimeout time.Duration
	// StallTimeout cancels a download when no data arrives for this long.
	StallTimeout time.Duration
	HTTP         *http.Client
	Logf         func(string, ...any)
}

type Worker struct {
	cfg    Config
	client *Client
}

func New(cfg Config) *Worker {
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{}
	}
	if cfg.Logf == nil {
		cfg.Logf = log.Printf
	}
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 2 * time.Second
	}
	if cfg.RetryBackoff == 0 {
		cfg.RetryBackoff = time.Second
	}
	if cfg.Parsers < 1 {
		cfg.Parsers = 1
	}
	if cfg.RequestTimeout == 0 {
		cfg.RequestTimeout = 20 * time.Second
	}
	if cfg.FirstByteTimeout == 0 {
		cfg.FirstByteTimeout = 30 * time.Second
	}
	if cfg.StallTimeout == 0 {
		cfg.StallTimeout = 20 * time.Second
	}
	return &Worker{cfg: cfg, client: NewClient(cfg.Coordinator, cfg.HTTP, cfg.RequestTimeout)}
}

// Run processes jobs until the coordinator reports that all work is finished or ctx is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	if err := os.MkdirAll(w.cfg.OutDir, 0o755); err != nil {
		return err
	}
	backoff := w.cfg.RetryBackoff
	for {
		resp, err := w.client.Claim(ctx, w.cfg.ID)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			w.cfg.Logf("claim failed, retrying in %s: %v", backoff, err)
			if !sleep(ctx, backoff) {
				return nil
			}
			backoff = min(backoff*2, 30*time.Second)
			continue
		}
		backoff = w.cfg.RetryBackoff

		switch {
		case resp.Job != nil:
			w.process(ctx, *resp.Job)
		case resp.Finished:
			w.cfg.Logf("all jobs finished, exiting")
			return nil
		default:
			if !sleep(ctx, w.cfg.PollInterval) {
				return nil
			}
		}
	}
}

func (w *Worker) process(ctx context.Context, j api.Job) {
	w.cfg.Logf("job %d attempt %d: %s", j.ID, j.Attempt, path.Base(j.Path))
	jobCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var lost atomic.Bool
	var wg sync.WaitGroup
	wg.Go(func() { w.heartbeat(jobCtx, j, &lost, cancel) })

	start := time.Now()
	result, err := w.runJob(jobCtx, j)
	result.DurationMs = time.Since(start).Milliseconds()
	cancel()
	wg.Wait()

	// Reporting ignores shutdown so the final message still goes out; the client's time limit still applies.
	report := context.WithoutCancel(ctx)

	switch {
	case err == nil:
		if err := w.client.Complete(report, j, w.cfg.ID, result); err != nil {
			w.cfg.Logf("job %d: report complete: %v (lease will expire and the job will be redone)", j.ID, err)
		}
	case lost.Load():
		w.cfg.Logf("job %d: lease lost, abandoning", j.ID)
	case ctx.Err() != nil:
		w.cfg.Logf("job %d: shutting down, releasing job", j.ID)
		w.client.Release(report, j, w.cfg.ID)
	default:
		w.cfg.Logf("job %d: failed: %v", j.ID, err)
		if err := w.client.Fail(report, j, w.cfg.ID, err.Error()); err != nil && !errors.Is(err, jobs.ErrLeaseLost) {
			w.cfg.Logf("job %d: report failure: %v", j.ID, err)
		}
	}
}

// heartbeat renews the lease at a third of its length and cancels the job if the lease is lost.
func (w *Worker) heartbeat(ctx context.Context, j api.Job, lost *atomic.Bool, cancel context.CancelFunc) {
	every := max(time.Duration(j.LeaseSeconds*float64(time.Second)/3), 10*time.Millisecond)
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			err := w.client.Heartbeat(ctx, j, w.cfg.ID)
			if errors.Is(err, jobs.ErrLeaseLost) {
				lost.Store(true)
				cancel()
				return
			}
			if err != nil && ctx.Err() == nil {
				w.cfg.Logf("job %d: heartbeat: %v", j.ID, err)
			}
		}
	}
}

// runJob downloads and extracts a file, starting over if the download breaks partway through.
func (w *Worker) runJob(ctx context.Context, j api.Job) (jobs.Result, error) {
	url := strings.TrimRight(w.cfg.Source, "/") + "/" + strings.TrimLeft(j.Path, "/")
	backoff := w.cfg.RetryBackoff
	var lastErr error
	for try := 0; try <= w.cfg.FetchRetries; try++ {
		if try > 0 {
			w.cfg.Logf("download retry %d/%d in %s: %v", try, w.cfg.FetchRetries, backoff, lastErr)
			if !sleep(ctx, backoff) {
				return jobs.Result{}, ctx.Err()
			}
			backoff *= 2
		}
		res, err := w.attempt(ctx, url, j.Path)
		if err == nil {
			return res, nil
		}
		if ctx.Err() != nil {
			return jobs.Result{}, ctx.Err()
		}
		var de *downloadError
		if !errors.As(err, &de) {
			return jobs.Result{}, err
		}
		lastErr = err
	}
	return jobs.Result{}, lastErr
}

func (w *Worker) attempt(ctx context.Context, url, watPath string) (jobs.Result, error) {
	body, err := w.open(ctx, url)
	if err != nil {
		return jobs.Result{}, err
	}
	defer body.Close()

	g, stats, err := extract.Run(body, w.cfg.Parsers)
	if err != nil {
		return jobs.Result{}, fmt.Errorf("extract: %w", err)
	}
	edges := g.Edges()
	if err := writeResult(filepath.Join(w.cfg.OutDir, ResultName(watPath)), edges); err != nil {
		return jobs.Result{}, fmt.Errorf("write result: %w", err)
	}
	return jobs.Result{Pages: stats.PagesUsed, CrossLinks: stats.CrossLinks, Edges: len(edges)}, nil
}

// downloadError marks failures worth retrying: network errors, timeouts, 429 and 5xx responses.
type downloadError struct{ err error }

func (e *downloadError) Error() string { return e.err.Error() }
func (e *downloadError) Unwrap() error { return e.err }

// open starts a download and returns a body that fails if data stops arriving.
func (w *Worker) open(ctx context.Context, url string) (io.ReadCloser, error) {
	dlCtx, cancel := context.WithCancel(ctx)
	wd := &watchdog{cancel: cancel, stall: w.cfg.StallTimeout}
	wd.timer = time.AfterFunc(w.cfg.FirstByteTimeout, wd.expire)

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, url, nil)
	if err != nil {
		wd.stop()
		return nil, err
	}
	resp, err := w.cfg.HTTP.Do(req)
	if err != nil {
		wd.stop()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if wd.fired.Load() {
			err = fmt.Errorf("download %s: no response within %s", url, w.cfg.FirstByteTimeout)
		}
		return nil, &downloadError{err}
	}
	wd.timer.Stop()
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		wd.stop()
		err := fmt.Errorf("download %s: %s", url, resp.Status)
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			return nil, &downloadError{err}
		}
		return nil, err
	}
	wd.body = resp.Body
	return wd, nil
}

// watchdog cancels a download when a single read waits longer than the stall timeout.
type watchdog struct {
	body   io.ReadCloser
	timer  *time.Timer
	cancel context.CancelFunc
	stall  time.Duration
	fired  atomic.Bool
}

func (d *watchdog) expire() {
	d.fired.Store(true)
	d.cancel()
}

func (d *watchdog) stop() {
	d.timer.Stop()
	d.cancel()
}

// Read only runs the timer while waiting on the network, so slow parsing never counts as a stall.
func (d *watchdog) Read(p []byte) (int, error) {
	d.timer.Reset(d.stall)
	n, err := d.body.Read(p)
	d.timer.Stop()
	if err != nil && err != io.EOF {
		if d.fired.Load() {
			err = fmt.Errorf("download stalled: no data for %s", d.stall)
		}
		return n, &downloadError{err}
	}
	return n, err
}

func (d *watchdog) Close() error {
	d.stop()
	return d.body.Close()
}

// ResultName maps a WAT path to its output file, e.g. .../X-00042.warc.wat.gz -> X-00042.edges.tsv.gz.
func ResultName(watPath string) string {
	return strings.TrimSuffix(path.Base(watPath), ".warc.wat.gz") + ".edges.tsv.gz"
}

// writeResult writes to a temp file and renames it into place, so readers never see a partial file.
func writeResult(dst string, edges []links.Edge) error {
	dir := filepath.Dir(dst)
	tmp, err := os.CreateTemp(dir, filepath.Base(dst)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	gz := gzip.NewWriter(tmp)
	if err := links.WriteTSV(gz, edges); err != nil {
		tmp.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		return err
	}
	return syncDir(dir)
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
