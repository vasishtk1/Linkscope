package loadtest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestPercentile(t *testing.T) {
	r := Result{}
	for i := 1; i <= 100; i++ {
		r.Latencies = append(r.Latencies, time.Duration(i)*time.Millisecond)
	}
	for p, want := range map[float64]time.Duration{50: 50 * time.Millisecond, 99: 99 * time.Millisecond, 100: 100 * time.Millisecond, 0: time.Millisecond} {
		if got := r.Percentile(p); got != want {
			t.Errorf("p%v = %v, want %v", p, got, want)
		}
	}
	if (Result{}).Percentile(50) != 0 {
		t.Error("empty result should give 0")
	}
}

func TestRunCountsRequestsAndErrors(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d := r.URL.Query().Get("domain")
		if r.URL.Path != "/lookup" || r.URL.Query().Get("limit") != "7" {
			t.Errorf("unexpected request %s", r.URL)
		}
		mu.Lock()
		seen[d]++
		mu.Unlock()
		if d == "missing.com" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	res, err := Run(context.Background(), srv.Client(), Options{
		URL: srv.URL, Domains: []string{"a.com", "b.com", "missing.com"},
		Concurrency: 4, Duration: 300 * time.Millisecond, Limit: 7, Seed: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv.Close()
	total := 0
	for _, n := range seen {
		total += n
	}
	if res.Requests == 0 || res.Requests > total {
		t.Fatalf("%d requests recorded, server saw %d", res.Requests, total)
	}
	if res.Errors == 0 || res.Errors >= res.Requests {
		t.Fatalf("errors = %d of %d, want some but not all", res.Errors, res.Requests)
	}
	if len(seen) != 3 {
		t.Fatalf("domains requested: %v", seen)
	}
	for i := 1; i < len(res.Latencies); i++ {
		if res.Latencies[i] < res.Latencies[i-1] {
			t.Fatal("latencies not sorted")
		}
	}
	if res.PerSecond() <= 0 || res.Bytes == 0 {
		t.Fatalf("got %+v", res)
	}
}

func TestRunRejectsBadOptions(t *testing.T) {
	if _, err := Run(context.Background(), http.DefaultClient, Options{Concurrency: 1, Duration: time.Second}); err == nil {
		t.Fatal("expected an error without domains")
	}
}
