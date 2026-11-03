package worker_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vasishtk1/Linkscope/internal/api"
	"github.com/vasishtk1/Linkscope/internal/coordinator"
	"github.com/vasishtk1/Linkscope/internal/extract"
	"github.com/vasishtk1/Linkscope/internal/fixture"
	"github.com/vasishtk1/Linkscope/internal/jobs"
	"github.com/vasishtk1/Linkscope/internal/links"
	"github.com/vasishtk1/Linkscope/internal/worker"
)

// source is a fake Common Crawl server. Hooks let a test make a file fail or hang.
type source struct {
	files map[string][]byte
	mu    sync.Mutex
	hook  func(w http.ResponseWriter, r *http.Request) bool
	hits  map[string]int
}

func (s *source) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/")
	s.mu.Lock()
	s.hits[p]++
	hook := s.hook
	s.mu.Unlock()
	if hook != nil && hook(w, r) {
		return
	}
	b, ok := s.files[p]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Write(b)
}

func (s *source) count(p string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits[p]
}

type cluster struct {
	t      *testing.T
	store  *jobs.Store
	coord  *httptest.Server
	src    *source
	srcSrv *httptest.Server
	out    string
	paths  []string
	// tune changes worker settings for one test.
	tune func(*worker.Config)
}

func newCluster(t *testing.T, nFiles int, lease time.Duration, maxAttempts int) *cluster {
	t.Helper()
	store, err := jobs.Open(filepath.Join(t.TempDir(), "jobs.db"), maxAttempts)
	if err != nil {
		t.Fatal(err)
	}
	src := &source{files: map[string][]byte{}, hits: map[string]int{}}
	var paths []string
	for i := range nFiles {
		p := fmt.Sprintf("crawl-data/TEST/segments/1/wat/TEST-%05d.warc.wat.gz", i)
		src.files[p] = watFile(i)
		paths = append(paths, p)
	}
	if _, err := store.Add(paths); err != nil {
		t.Fatal(err)
	}
	srv := coordinator.New(store, lease)
	ctx, cancel := context.WithCancel(context.Background())
	go srv.RunReaper(ctx, 20*time.Millisecond)

	c := &cluster{
		t: t, store: store, src: src, paths: paths,
		coord:  httptest.NewServer(srv.Handler()),
		srcSrv: httptest.NewServer(src),
		out:    t.TempDir(),
	}
	t.Cleanup(func() {
		cancel()
		c.coord.Close()
		c.srcSrv.Close()
		store.Close()
	})
	return c
}

func watFile(i int) []byte {
	site := fmt.Sprintf("site%d.com", i)
	return fixture.Gzip(
		fixture.Page("https://"+site+"/a", "200", "https://shared.com/", fmt.Sprintf("https://friend%d.org/", i), "/internal"),
		fixture.Page("https://"+site+"/b", "200", "https://shared.com/x"),
		fixture.Page("https://other.net/", "200", "https://"+site+"/"),
	)
}

func (c *cluster) worker(id string) *worker.Worker {
	cfg := worker.Config{
		ID: id, Coordinator: c.coord.URL, Source: c.srcSrv.URL, OutDir: c.out,
		Parsers: 2, PollInterval: 10 * time.Millisecond,
		FetchRetries: 3, RetryBackoff: 5 * time.Millisecond,
		Logf: func(f string, a ...any) { c.t.Logf(id+" "+f, a...) },
	}
	if c.tune != nil {
		c.tune(&cfg)
	}
	return worker.New(cfg)
}

// runWorkers runs n workers until they all exit, failing the test if that takes too long.
func (c *cluster) runWorkers(n int) {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			if err := c.worker(fmt.Sprintf("w%d", i)).Run(ctx); err != nil {
				c.t.Error(err)
			}
		})
	}
	wg.Wait()
	if ctx.Err() != nil {
		c.t.Fatal("workers did not finish in time")
	}
}

func (c *cluster) summary() jobs.Summary {
	c.t.Helper()
	s, err := c.store.Summary(100)
	if err != nil {
		c.t.Fatal(err)
	}
	return s
}

// expected is what extracting the file directly, with no cluster involved, produces.
func expected(t *testing.T, file []byte) []byte {
	t.Helper()
	g, _, err := extract.Run(bytes.NewReader(file), 1)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := links.WriteTSV(&buf, g.Edges()); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func readResult(t *testing.T, path string) []byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(gz)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestClusterProcessesEveryFileExactlyOnce(t *testing.T) {
	c := newCluster(t, 12, time.Second, 3)
	c.runWorkers(4)

	sum := c.summary()
	if sum.Done != 12 || sum.Failed != 0 || sum.Pending != 0 || sum.Running != 0 {
		t.Fatalf("summary = %+v", sum)
	}
	for _, p := range c.paths {
		got := readResult(t, filepath.Join(c.out, worker.ResultName(p)))
		if !bytes.Equal(got, expected(t, c.src.files[p])) {
			t.Errorf("%s: result differs from direct extraction", p)
		}
		if n := c.src.count(p); n != 1 {
			t.Errorf("%s downloaded %d times, want 1", p, n)
		}
	}
	entries, _ := os.ReadDir(c.out)
	if len(entries) != 12 {
		t.Fatalf("%d files in output dir, want 12 (no leftover temp files)", len(entries))
	}
	if sum.Pages != 12*3 {
		t.Fatalf("pages = %d, want %d", sum.Pages, 12*3)
	}
}

func TestFlakyDownloadIsRetried(t *testing.T) {
	c := newCluster(t, 1, time.Second, 3)
	var calls atomic.Int32
	c.src.hook = func(w http.ResponseWriter, r *http.Request) bool {
		if calls.Add(1) <= 2 {
			http.Error(w, "slow down", http.StatusServiceUnavailable)
			return true
		}
		return false
	}
	c.runWorkers(1)
	if sum := c.summary(); sum.Done != 1 {
		t.Fatalf("summary = %+v", sum)
	}
	if calls.Load() != 3 {
		t.Fatalf("downloads = %d, want 3", calls.Load())
	}
}

func TestCorruptFileIsMarkedFailedAndOthersFinish(t *testing.T) {
	c := newCluster(t, 3, time.Second, 2)
	c.src.files[c.paths[1]] = []byte("not a gzip file")
	c.runWorkers(2)

	sum := c.summary()
	if sum.Done != 2 || sum.Failed != 1 {
		t.Fatalf("summary = %+v", sum)
	}
	if f := sum.Failures[0]; f.Path != c.paths[1] || f.Attempts != 2 || !strings.Contains(f.Error, "extract") {
		t.Fatalf("failure = %+v", f)
	}
}

func TestMissingFileFailsWithoutRetryingDownload(t *testing.T) {
	c := newCluster(t, 1, time.Second, 1)
	delete(c.src.files, c.paths[0])
	c.runWorkers(1)
	if sum := c.summary(); sum.Failed != 1 {
		t.Fatalf("summary = %+v", sum)
	}
	if n := c.src.count(c.paths[0]); n != 1 {
		t.Fatalf("404 was retried: %d requests", n)
	}
}

func TestJobFromCrashedWorkerIsRedone(t *testing.T) {
	c := newCluster(t, 2, 200*time.Millisecond, 3)
	client := worker.NewClient(c.coord.URL, http.DefaultClient, time.Second)
	// A worker that claims a job and then dies without a word.
	resp, err := client.Claim(context.Background(), "ghost")
	if err != nil || resp.Job == nil {
		t.Fatalf("claim: %+v %v", resp, err)
	}
	c.runWorkers(1)

	sum := c.summary()
	if sum.Done != 2 {
		t.Fatalf("summary = %+v", sum)
	}
	j, _ := c.store.Get(resp.Job.ID)
	if j.Attempt != 2 {
		t.Fatalf("ghost's job attempt = %d, want 2", j.Attempt)
	}
}

// hangUntilCancelled makes downloads of path stall until the client goes away.
func hangUntilCancelled(c *cluster, path string, started chan<- struct{}) {
	var once sync.Once
	c.src.hook = func(w http.ResponseWriter, r *http.Request) bool {
		if strings.TrimPrefix(r.URL.Path, "/") != path {
			return false
		}
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		once.Do(func() { close(started) })
		<-r.Context().Done()
		return true
	}
}

func TestWorkerAbandonsJobWhenLeaseIsLost(t *testing.T) {
	c := newCluster(t, 1, 150*time.Millisecond, 3)
	started := make(chan struct{})
	hangUntilCancelled(c, c.paths[0], started)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		c.worker("slow").Run(ctx)
		close(done)
	}()
	<-started

	// The coordinator gives the job away behind the worker's back.
	if err := c.store.Release(1, 1); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
	for {
		j, _ := c.store.Get(1)
		if j.State == jobs.Running && j.Attempt == 1 && j.Worker == "slow" {
			break // the same worker re-claimed it after abandoning the old attempt
		}
		select {
		case <-deadline:
			t.Fatalf("worker never abandoned its lost lease; job = %+v", j)
		case <-time.After(10 * time.Millisecond):
		}
	}
	if _, err := os.Stat(filepath.Join(c.out, worker.ResultName(c.paths[0]))); err == nil {
		t.Fatal("abandoned job should not have written a result")
	}
	cancel()
	<-done
}

func TestShutdownReleasesJobWithoutUsingAnAttempt(t *testing.T) {
	c := newCluster(t, 1, 5*time.Second, 1)
	started := make(chan struct{})
	hangUntilCancelled(c, c.paths[0], started)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		c.worker("w").Run(ctx)
		close(done)
	}()
	<-started
	cancel()
	<-done

	j, _ := c.store.Get(1)
	if j.State != jobs.Pending || j.Attempt != 0 {
		t.Fatalf("job = %+v, want pending with 0 attempts", j)
	}
}

func TestWorkerRetriesWhileCoordinatorIsDown(t *testing.T) {
	c := newCluster(t, 1, time.Second, 3)
	var refused atomic.Int32
	real := c.coord.Config.Handler
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if refused.Add(1) <= 3 {
			http.Error(w, "starting up", http.StatusServiceUnavailable)
			return
		}
		real.ServeHTTP(w, r)
	}))
	defer down.Close()

	w := worker.New(worker.Config{
		ID: "early", Coordinator: down.URL, Source: c.srcSrv.URL, OutDir: c.out,
		PollInterval: 10 * time.Millisecond, RetryBackoff: 5 * time.Millisecond,
		Logf: t.Logf,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := w.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != nil {
		t.Fatal("worker did not finish")
	}
	if sum := c.summary(); sum.Done != 1 {
		t.Fatalf("summary = %+v", sum)
	}
}

func TestStatusEndpoint(t *testing.T) {
	c := newCluster(t, 3, time.Second, 3)
	c.runWorkers(2)
	resp, err := http.Get(c.coord.URL + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var st api.Status
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	if st.Total != 3 || st.Done != 3 || len(st.Recent) != 3 {
		t.Fatalf("status = %+v", st)
	}
}

func TestWorkerRecoversFromFrozenCoordinator(t *testing.T) {
	c := newCluster(t, 1, time.Second, 3)
	var frozen atomic.Int32
	real := c.coord.Config.Handler
	// Accepts connections but never answers the first 3 requests.
	stuck := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if frozen.Add(1) <= 3 {
			// Reading the body lets the server notice when the worker hangs up.
			io.Copy(io.Discard, r.Body)
			<-r.Context().Done()
			return
		}
		real.ServeHTTP(w, r)
	}))
	defer stuck.Close()
	c.tune = func(cfg *worker.Config) {
		cfg.Coordinator = stuck.URL
		cfg.RequestTimeout = 100 * time.Millisecond
	}

	c.runWorkers(1)
	if sum := c.summary(); sum.Done != 1 {
		t.Fatalf("summary = %+v", sum)
	}
	if n := frozen.Load(); n < 4 {
		t.Fatalf("only %d requests reached the coordinator", n)
	}
}

func TestStalledDownloadIsRetried(t *testing.T) {
	c := newCluster(t, 1, time.Second, 3)
	file := c.src.files[c.paths[0]]
	var calls atomic.Int32
	// The first download sends half the file, then goes silent.
	c.src.hook = func(w http.ResponseWriter, r *http.Request) bool {
		if calls.Add(1) > 1 {
			return false
		}
		w.Write(file[:len(file)/2])
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		return true
	}
	c.tune = func(cfg *worker.Config) { cfg.StallTimeout = 100 * time.Millisecond }

	c.runWorkers(1)
	if sum := c.summary(); sum.Done != 1 {
		t.Fatalf("summary = %+v", sum)
	}
	if calls.Load() != 2 {
		t.Fatalf("downloads = %d, want 2", calls.Load())
	}
	got := readResult(t, filepath.Join(c.out, worker.ResultName(c.paths[0])))
	if !bytes.Equal(got, expected(t, file)) {
		t.Fatal("result after a stalled download differs from direct extraction")
	}
	if j, _ := c.store.Get(1); j.Attempt != 1 {
		t.Fatalf("attempt = %d, want 1 (the retry should happen inside the job)", j.Attempt)
	}
}

func TestDownloadThatNeverStartsIsRetried(t *testing.T) {
	c := newCluster(t, 1, time.Second, 3)
	var calls atomic.Int32
	// The first request gets no response at all, not even headers.
	c.src.hook = func(w http.ResponseWriter, r *http.Request) bool {
		if calls.Add(1) > 1 {
			return false
		}
		<-r.Context().Done()
		return true
	}
	c.tune = func(cfg *worker.Config) { cfg.FirstByteTimeout = 100 * time.Millisecond }

	c.runWorkers(1)
	if sum := c.summary(); sum.Done != 1 {
		t.Fatalf("summary = %+v", sum)
	}
	if calls.Load() != 2 {
		t.Fatalf("downloads = %d, want 2", calls.Load())
	}
}

func TestSlowButSteadyDownloadIsNotCancelled(t *testing.T) {
	c := newCluster(t, 1, time.Second, 3)
	file := c.src.files[c.paths[0]]
	// Sends the file in small pieces; the whole download takes far longer than the stall limit.
	c.src.hook = func(w http.ResponseWriter, r *http.Request) bool {
		for i := 0; i < len(file); i += 64 {
			w.Write(file[i:min(i+64, len(file))])
			w.(http.Flusher).Flush()
			time.Sleep(30 * time.Millisecond)
		}
		return true
	}
	c.tune = func(cfg *worker.Config) { cfg.StallTimeout = 100 * time.Millisecond }

	start := time.Now()
	c.runWorkers(1)
	if sum := c.summary(); sum.Done != 1 {
		t.Fatalf("summary = %+v", sum)
	}
	if n := c.src.count(c.paths[0]); n != 1 {
		t.Fatalf("downloads = %d, want 1", n)
	}
	if took := time.Since(start); took < 200*time.Millisecond {
		t.Fatalf("download took %s; test needs it to run longer than the stall limit", took)
	}
}
