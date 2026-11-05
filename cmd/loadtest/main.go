package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/vasishtk1/Linkscope/internal/loadtest"
	"github.com/vasishtk1/Linkscope/internal/pagerank"
)

func main() {
	o := loadtest.Options{Seed: 1}
	flag.StringVar(&o.URL, "url", "http://localhost:8081", "lookup API address")
	scores := flag.String("scores", "data/scores.tsv", "scores file to pick domains from (must match the API's index)")
	top := flag.Int("top", 0, "only look up the N highest-ranked domains (0 = every domain with backlinks)")
	flag.IntVar(&o.Concurrency, "c", 8, "lookups in flight at once")
	flag.DurationVar(&o.Duration, "d", 10*time.Second, "how long to measure")
	warmup := flag.Duration("warmup", 2*time.Second, "unmeasured run first, so files are in the OS cache")
	flag.IntVar(&o.Limit, "limit", 1000, "backlinks per answer (the website asks for 1000; 0 = all)")
	flag.Parse()
	if flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(o, *scores, *top, *warmup); err != nil {
		fmt.Fprintln(os.Stderr, "loadtest:", err)
		os.Exit(1)
	}
}

func run(o loadtest.Options, scoresPath string, top int, warmup time.Duration) error {
	domains, err := pickDomains(scoresPath, top)
	if err != nil {
		return err
	}
	o.Domains = domains
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{MaxIdleConnsPerHost: o.Concurrency, MaxConnsPerHost: o.Concurrency},
	}
	ctx := context.Background()
	if warmup > 0 {
		w := o
		w.Duration = warmup
		if _, err := loadtest.Run(ctx, client, w); err != nil {
			return err
		}
	}
	res, err := loadtest.Run(ctx, client, o)
	if err != nil {
		return err
	}

	ms := func(d time.Duration) string { return fmt.Sprintf("%.2f ms", float64(d.Microseconds())/1000) }
	fmt.Printf("domains:     %d to pick from, limit %d backlinks per answer\n", len(domains), o.Limit)
	fmt.Printf("concurrency: %d for %s\n", o.Concurrency, o.Duration)
	fmt.Printf("requests:    %d (%d errors)\n", res.Requests, res.Errors)
	fmt.Printf("throughput:  %.0f lookups/s\n", res.PerSecond())
	fmt.Printf("latency:     p50 %s, p90 %s, p99 %s, max %s\n",
		ms(res.Percentile(50)), ms(res.Percentile(90)), ms(res.Percentile(99)), ms(res.Percentile(100)))
	if res.Requests > 0 {
		fmt.Printf("answer size: %.1f KB average\n", float64(res.Bytes)/float64(res.Requests)/1000)
	}
	if res.Errors > 0 {
		return fmt.Errorf("%d lookups failed", res.Errors)
	}
	return nil
}

// pickDomains returns domains that have backlinks, highest-ranked first, so every lookup should succeed.
func pickDomains(path string, top int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rows, err := pagerank.ReadTSV(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var out []string
	for _, r := range rows {
		if r.LinkingDomains > 0 {
			out = append(out, r.Domain)
		}
		if top > 0 && len(out) == top {
			break
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: no domains with backlinks", path)
	}
	return out, nil
}
