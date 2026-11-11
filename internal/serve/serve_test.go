package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/vasishtk1/Linkscope/internal/index"
	"github.com/vasishtk1/Linkscope/internal/links"
	"github.com/vasishtk1/Linkscope/internal/merge"
	"github.com/vasishtk1/Linkscope/internal/record"
)

func newServer(t *testing.T) *httptest.Server { return newServerWithRun(t, nil) }

func newServerWithRun(t *testing.T, run *record.Recording) *httptest.Server {
	t.Helper()
	edges := []links.Edge{
		{Src: "reddit.com", Dst: "github.com", Pages: 12, ExampleSrc: "https://reddit.com/r/go", ExampleDst: "https://github.com/golang/go"},
		{Src: "go.dev", Dst: "github.com", Pages: 30, NofollowPages: 1, ExampleSrc: "https://go.dev/", ExampleDst: "https://github.com/golang"},
		{Src: "github.com", Dst: "go.dev", Pages: 2, ExampleSrc: "https://github.com/", ExampleDst: "https://go.dev/dl"},
	}
	dir := filepath.Join(t.TempDir(), "index")
	if _, err := index.Build(dir, edges, map[string]int{"github.com": 100}, index.Options{Shards: 4, Source: "test", Origins: map[string][]merge.Origin{
		"github.com": {{File: "x-00001", Pages: 30}, {File: "x-00000", Pages: 12}},
	}, Pages: map[string]merge.SitePages{
		"github.com": {Total: 2, Top: []merge.TopPage{
			{Page: "github.com/golang", Sites: 1, Pages: 30, NofollowPages: 1, Sources: []merge.PageSource{{Domain: "go.dev", Pages: 30}}},
			{Page: "github.com/golang/go", Sites: 1, Pages: 12, Sources: []merge.PageSource{{Domain: "reddit.com", Pages: 12}}},
		}},
	}}); err != nil {
		t.Fatal(err)
	}
	ix, err := index.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(Handler(ix, run))
	t.Cleanup(func() { srv.Close(); ix.Close() })
	return srv
}

func get(t *testing.T, url string, into any) int {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if into != nil {
		if err := json.NewDecoder(res.Body).Decode(into); err != nil {
			t.Fatal(err)
		}
	}
	return res.StatusCode
}

func TestLookup(t *testing.T) {
	srv := newServer(t)
	var got index.Lookup
	if code := get(t, srv.URL+"/lookup?domain=https://www.GitHub.com/golang", &got); code != 200 {
		t.Fatalf("status %d", code)
	}
	if got.Domain != "github.com" || *got.Score != 100 || got.LinkingDomains != 2 || got.LinkingPages != 42 ||
		len(got.Backlinks) != 2 || got.Backlinks[0].Domain != "go.dev" || got.Source != "test" {
		t.Fatalf("got %+v", got)
	}
	if got.OriginFiles != 2 || len(got.Origins) != 2 || got.Origins[0] != (merge.Origin{File: "x-00001", Pages: 30}) || got.TookMs < 0 {
		t.Fatalf("origins %d %+v, took %v", got.OriginFiles, got.Origins, got.TookMs)
	}
	if got.LinkedPages != 2 || len(got.TopPages) != 2 || got.TopPages[0].Page != "github.com/golang" || got.TopPages[0].Sources[0].Domain != "go.dev" {
		t.Fatalf("top pages %d %+v", got.LinkedPages, got.TopPages)
	}
}

func TestLookupLimit(t *testing.T) {
	srv := newServer(t)
	var got index.Lookup
	get(t, srv.URL+"/lookup?domain=github.com&limit=1", &got)
	if len(got.Backlinks) != 1 || got.LinkingDomains != 2 {
		t.Fatalf("got %+v", got)
	}
}

func TestLookupErrors(t *testing.T) {
	srv := newServer(t)
	for path, want := range map[string]int{
		"/lookup":                            400,
		"/lookup?domain=%20":                 400,
		"/lookup?domain=github.com&limit=x":  400,
		"/lookup?domain=github.com&limit=-1": 400,
		"/lookup?domain=reddit.com":          404,
		"/lookup?domain=unknown.example":     404,
	} {
		var body map[string]string
		if code := get(t, srv.URL+path, &body); code != want || body["error"] == "" {
			t.Errorf("%s: status %d body %v, want %d with an error message", path, code, body, want)
		}
	}
	res, _ := http.Post(srv.URL+"/lookup?domain=github.com", "text/plain", nil)
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST: status %d", res.StatusCode)
	}
}

func TestKnown(t *testing.T) {
	srv := newServer(t)
	for domain, want := range map[string]int{"reddit.com": 200, "github.com": 200, "docs.github.com": 404, "evil.example": 404} {
		if code := get(t, srv.URL+"/known?domain="+domain, nil); code != want {
			t.Errorf("%s: status %d, want %d", domain, code, want)
		}
	}
}

func TestNetwork(t *testing.T) {
	srv := newServer(t)
	var nw index.Network
	if code := get(t, srv.URL+"/network", &nw); code != 200 {
		t.Fatalf("status %d", code)
	}
	// Only github.com has a score, so it's the whole network.
	if len(nw.Nodes) != 1 || nw.Nodes[0].Domain != "github.com" || nw.Links == nil || len(nw.Links) != 0 {
		t.Fatalf("got %+v", nw)
	}
	for _, q := range []string{"0", "201", "x"} {
		var body map[string]string
		if code := get(t, srv.URL+"/network?n="+q, &body); code != 400 || body["error"] == "" {
			t.Errorf("n=%s: status %d body %v", q, code, body)
		}
	}
}

func TestStatsAndHealth(t *testing.T) {
	srv := newServer(t)
	var m index.Manifest
	if code := get(t, srv.URL+"/stats", &m); code != 200 || m.Shards != 4 || m.Domains != 3 || m.Edges != 3 {
		t.Fatalf("status %d, manifest %+v", code, m)
	}
	if code := get(t, srv.URL+"/healthz", nil); code != 200 {
		t.Fatalf("healthz status %d", code)
	}
}

func TestParallelLookups(t *testing.T) {
	srv := newServer(t)
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			var got index.Lookup
			if code := get(t, srv.URL+"/lookup?domain=github.com", &got); code != 200 || got.LinkingDomains != 2 {
				t.Errorf("status %d, %+v", code, got)
			}
		})
	}
	wg.Wait()
}

func TestRun(t *testing.T) {
	var body map[string]string
	if code := get(t, newServer(t).URL+"/run", &body); code != 404 || body["error"] == "" {
		t.Fatalf("without a run: status %d body %v", code, body)
	}
	srv := newServerWithRun(t, &record.Recording{Name: "demo", Totals: record.Totals{Files: 6, Done: 6}})
	var got record.Recording
	if code := get(t, srv.URL+"/run", &got); code != 200 || got.Name != "demo" || got.Totals.Done != 6 {
		t.Fatalf("status %d, run %+v", code, got)
	}
}
