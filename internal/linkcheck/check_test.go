package linkcheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func fakeSite(t *testing.T) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("User-agent: *\nDisallow: /private\n"))
	})
	mux.HandleFunc("/{$}", func(w http.ResponseWriter, _ *http.Request) {})
	mux.HandleFunc("/ok", func(w http.ResponseWriter, _ *http.Request) {})
	mux.HandleFunc("/gone", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusGone) })
	mux.HandleFunc("/moved", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/ok", http.StatusMovedPermanently) })
	mux.HandleFunc("/moved-dead", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/nothing-here", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/old-post", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/", http.StatusFound) })
	mux.HandleFunc("/no-head", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/head-lies", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusNotFound)
		}
	})
	mux.HandleFunc("/error", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) })
	mux.HandleFunc("/forbidden", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) })
	mux.HandleFunc("/slow", func(w http.ResponseWriter, _ *http.Request) { time.Sleep(300 * time.Millisecond) })
	mux.HandleFunc("/loop", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/loop", http.StatusFound) })
	mux.HandleFunc("/private/page", func(w http.ResponseWriter, _ *http.Request) { t.Error("visited a page robots.txt blocks") })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.UserAgent() != UserAgent {
			t.Errorf("User-Agent = %q", r.UserAgent())
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

func TestCheck(t *testing.T) {
	host := fakeSite(t)
	c := New(Options{Timeout: 100 * time.Millisecond, Interval: time.Millisecond})
	defer c.Close()

	cases := []struct {
		path   string
		want   Result
		status int
		reason string
	}{
		{"/ok", OK, 200, ""},
		{"/missing", Broken, 404, ""},
		{"/gone", Broken, 410, ""},
		{"/moved", OK, 200, ""},
		{"/moved-dead", Broken, 404, ""},
		{"/old-post", Homepage, 200, ""},
		{"/", OK, 200, ""},
		{"/no-head", OK, 200, ""},
		{"/head-lies", OK, 200, ""},
		{"/error", Unknown, 503, "server error 503"},
		{"/forbidden", Unknown, 403, "blocked 403"},
		{"/slow", Unknown, 0, "timeout"},
		{"/loop", Unknown, 0, "too many redirects"},
		{"/private/page", Skipped, 0, "robots.txt"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got := c.Check(context.Background(), host+tc.path)
			if got.Result != tc.want || got.Status != tc.status || got.Reason != tc.reason {
				t.Errorf("got %s %d %q, want %s %d %q", got.Result, got.Status, got.Reason, tc.want, tc.status, tc.reason)
			}
		})
	}

	moved := c.Check(context.Background(), host+"/moved")
	if !strings.HasSuffix(moved.FinalURL, "/ok") || moved.URL != "http://"+host+"/moved" {
		t.Errorf("redirect not recorded: %+v", moved)
	}
}

func TestUnreachableHost(t *testing.T) {
	c := New(Options{Timeout: time.Second, Interval: time.Millisecond})
	defer c.Close()
	got := c.Check(context.Background(), "127.0.0.1:1/page")
	if got.Result != Unknown || got.Reason != "connection failed" {
		t.Errorf("got %+v", got)
	}
}

func TestCheckAllKeepsOrderAndPace(t *testing.T) {
	host := fakeSite(t)
	interval := 20 * time.Millisecond
	c := New(Options{Timeout: time.Second, Interval: interval})
	defer c.Close()

	pages := []string{host + "/ok", host + "/missing", host + "/gone", host + "/moved", host + "/no-head"}
	start := time.Now()
	got := c.CheckAll(context.Background(), pages, 2)
	if took := time.Since(start); took < time.Duration(len(pages))*interval {
		t.Errorf("finished in %v, faster than one request per %v", took, interval)
	}
	want := []Result{OK, Broken, Broken, OK, OK}
	for i := range pages {
		if got[i].Page != pages[i] || got[i].Result != want[i] {
			t.Errorf("result %d = %s %s, want %s %s", i, got[i].Page, got[i].Result, pages[i], want[i])
		}
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		status   int
		path     string
		homepage bool
		want     Result
	}{
		{200, "/a", false, OK},
		{200, "/a", true, Homepage},
		{200, "/", true, OK},
		{404, "/a", false, Broken},
		{410, "/a", false, Broken},
		{429, "/a", false, Unknown},
		{500, "/a", false, Unknown},
		{418, "/a", false, Unknown},
	}
	for _, c := range cases {
		if got, _ := classify(c.status, c.path, c.homepage); got != c.want {
			t.Errorf("classify(%d, %q, %v) = %s, want %s", c.status, c.path, c.homepage, got, c.want)
		}
	}
}
