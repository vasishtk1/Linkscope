package loadtest

import (
	"context"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"time"
)

type Options struct {
	URL         string
	Domains     []string
	Concurrency int
	Duration    time.Duration
	Limit       int
	Seed        uint64
}

type Result struct {
	Requests  int
	Errors    int
	Bytes     int64
	Elapsed   time.Duration
	Latencies []time.Duration
}

type workerResult struct {
	latencies []time.Duration
	errors    int
	bytes     int64
}

// Run keeps Concurrency lookups in flight for Duration, each for a random domain from the list.
func Run(ctx context.Context, client *http.Client, o Options) (Result, error) {
	if len(o.Domains) == 0 || o.Concurrency < 1 || o.Duration <= 0 {
		return Result{}, fmt.Errorf("need domains, concurrency of at least 1 and a positive duration")
	}
	ctx, cancel := context.WithTimeout(ctx, o.Duration)
	defer cancel()

	results := make([]workerResult, o.Concurrency)
	start := time.Now()
	var wg sync.WaitGroup
	for i := range o.Concurrency {
		wg.Go(func() {
			rng := rand.New(rand.NewPCG(o.Seed, uint64(i)))
			r := &results[i]
			for ctx.Err() == nil {
				d := o.Domains[rng.IntN(len(o.Domains))]
				began := time.Now()
				n, err := lookup(ctx, client, o.URL, d, o.Limit)
				if ctx.Err() != nil {
					return
				}
				r.latencies = append(r.latencies, time.Since(began))
				r.bytes += n
				if err != nil {
					r.errors++
				}
			}
		})
	}
	wg.Wait()

	res := Result{Elapsed: time.Since(start)}
	for _, r := range results {
		res.Latencies = append(res.Latencies, r.latencies...)
		res.Errors += r.errors
		res.Bytes += r.bytes
	}
	res.Requests = len(res.Latencies)
	slices.Sort(res.Latencies)
	return res, nil
}

// lookup reads the whole answer, since a real client has to receive all of it.
func lookup(ctx context.Context, client *http.Client, base, domain string, limit int) (int64, error) {
	u := fmt.Sprintf("%s/lookup?domain=%s&limit=%d", base, url.QueryEscape(domain), limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	res, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	n, err := io.Copy(io.Discard, res.Body)
	if err != nil {
		return n, err
	}
	if res.StatusCode != http.StatusOK {
		return n, fmt.Errorf("%s: status %d", domain, res.StatusCode)
	}
	return n, nil
}

// Percentile uses the nearest-rank method: p99 is the latency that 99% of requests were at or under.
func (r Result) Percentile(p float64) time.Duration {
	if len(r.Latencies) == 0 {
		return 0
	}
	i := int(math.Ceil(p/100*float64(len(r.Latencies)))) - 1
	return r.Latencies[max(0, min(i, len(r.Latencies)-1))]
}

func (r Result) PerSecond() float64 {
	if r.Elapsed <= 0 {
		return 0
	}
	return float64(r.Requests) / r.Elapsed.Seconds()
}
