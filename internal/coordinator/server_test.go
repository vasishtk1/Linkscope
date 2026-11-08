package coordinator

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/vasishtk1/Linkscope/internal/api"
	"github.com/vasishtk1/Linkscope/internal/jobs"
)

func newServer(t *testing.T, paths ...string) (*Server, *httptest.Server, *time.Time) {
	t.Helper()
	store, err := jobs.Open(filepath.Join(t.TempDir(), "jobs.db"), 3)
	if err != nil {
		t.Fatal(err)
	}
	store.Add(paths)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	s := New(store, time.Minute)
	s.now = func() time.Time { return now }
	s.logf = t.Logf
	hs := httptest.NewServer(s.Handler())
	t.Cleanup(func() { hs.Close(); store.Close() })
	return s, hs, &now
}

func post(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func claim(t *testing.T, base, worker string) api.ClaimResponse {
	t.Helper()
	resp := post(t, base+"/jobs/claim", api.ClaimRequest{Worker: worker})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("claim status %d", resp.StatusCode)
	}
	var cr api.ClaimResponse
	json.NewDecoder(resp.Body).Decode(&cr)
	return cr
}

func TestClaimHeartbeatComplete(t *testing.T) {
	_, hs, _ := newServer(t, "a")
	cr := claim(t, hs.URL, "w1")
	if cr.Job == nil || cr.Job.Path != "a" || cr.Job.Attempt != 1 || cr.Job.LeaseSeconds != 60 {
		t.Fatalf("claim = %+v", cr)
	}
	if r := post(t, hs.URL+"/jobs/1/heartbeat", api.LeaseRequest{Worker: "w1", Attempt: 1}); r.StatusCode != http.StatusNoContent {
		t.Fatalf("heartbeat status %d", r.StatusCode)
	}
	if cr := claim(t, hs.URL, "w2"); cr.Job != nil || cr.Finished {
		t.Fatalf("job is running: want no job and not finished, got %+v", cr)
	}
	if r := post(t, hs.URL+"/jobs/1/complete", api.CompleteRequest{Worker: "w1", Attempt: 1, Result: jobs.Result{Pages: 5}}); r.StatusCode != http.StatusNoContent {
		t.Fatalf("complete status %d", r.StatusCode)
	}
	if cr := claim(t, hs.URL, "w2"); !cr.Finished {
		t.Fatalf("expected finished, got %+v", cr)
	}
}

func TestStaleAttemptGetsConflict(t *testing.T) {
	_, hs, now := newServer(t, "a")
	claim(t, hs.URL, "w1")
	*now = now.Add(2 * time.Minute)
	claim(t, hs.URL, "w2")
	for _, action := range []string{"heartbeat", "fail", "release"} {
		r := post(t, hs.URL+"/jobs/1/"+action, api.FailRequest{Worker: "w1", Attempt: 1, Error: "x"})
		if r.StatusCode != http.StatusConflict {
			t.Errorf("%s from stale attempt: status %d, want 409", action, r.StatusCode)
		}
	}
}

func TestBadRequests(t *testing.T) {
	_, hs, _ := newServer(t, "a")
	cases := []struct {
		path string
		body string
	}{
		{"/jobs/claim", `{"worker":""}`},
		{"/jobs/claim", `not json`},
		{"/jobs/abc/heartbeat", `{}`},
		{"/jobs/0/complete", `{}`},
	}
	for _, c := range cases {
		resp, err := http.Post(hs.URL+c.path, "application/json", strings.NewReader(c.body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s %s: status %d, want 400", c.path, c.body, resp.StatusCode)
		}
	}
	resp, _ := http.Get(hs.URL + "/jobs/claim")
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET claim: status %d, want 405", resp.StatusCode)
	}
}

func TestStatusRates(t *testing.T) {
	s, hs, now := newServer(t, "a", "b")
	s.MarkStarted()
	claim(t, hs.URL, "w1")
	post(t, hs.URL+"/jobs/1/complete", api.CompleteRequest{Worker: "w1", Attempt: 1, Result: jobs.Result{Pages: 600}})
	*now = now.Add(time.Minute)

	resp, err := http.Get(hs.URL + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var st api.Status
	json.NewDecoder(resp.Body).Decode(&st)
	if st.Total != 2 || st.Done != 1 || st.Pending != 1 || st.JobsPerMinute != 1 || st.PagesPerSecond != 10 || st.UptimeSeconds != 60 {
		t.Fatalf("status = %+v", st)
	}
}

func TestReadPathsAndWindow(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	gz.Write([]byte("a\nb\n\nc\nd\n"))
	gz.Close()
	paths, err := readPaths(&buf, true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(paths, []string{"a", "b", "c", "d"}) {
		t.Fatalf("paths = %v", paths)
	}
	cases := []struct {
		offset, limit int
		want          []string
	}{
		{0, 2, []string{"a", "b"}},
		{1, 2, []string{"b", "c"}},
		{3, 10, []string{"d"}},
		{0, 0, []string{"a", "b", "c", "d"}},
		{9, 1, nil},
	}
	for _, c := range cases {
		if got := Window(paths, c.offset, c.limit); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Window(%d, %d) = %v, want %v", c.offset, c.limit, got, c.want)
		}
	}
	if u := CrawlPathsURL("https://data.commoncrawl.org/", "CC-MAIN-2026-43"); u != "https://data.commoncrawl.org/crawl-data/CC-MAIN-2026-43/wat.paths.gz" {
		t.Fatalf("url = %s", u)
	}
}

func getJSON(t *testing.T, url string, into any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s: status %d", url, resp.StatusCode)
	}
	json.NewDecoder(resp.Body).Decode(into)
}

func TestStatusShowsWorkers(t *testing.T) {
	_, hs, now := newServer(t, "a", "b")
	claim(t, hs.URL, "w1")
	claim(t, hs.URL, "w2")
	*now = now.Add(10 * time.Second)
	post(t, hs.URL+"/jobs/1/complete", api.CompleteRequest{Worker: "w1", Attempt: 1, Result: jobs.Result{Pages: 7}})
	claim(t, hs.URL, "w3")
	*now = now.Add(5 * time.Second)

	var st api.Status
	getJSON(t, hs.URL+"/status", &st)
	want := []api.Worker{
		{Name: "w1", State: "idle", LastSeenSeconds: 5, Done: 1, Pages: 7},
		{Name: "w2", State: "working", JobID: 2, Path: "b", JobSeconds: 15, LastSeenSeconds: 15},
		{Name: "w3", State: "idle", LastSeenSeconds: 5},
	}
	if !st.Now.Equal(*now) {
		t.Fatalf("now = %v, want %v", st.Now, *now)
	}
	if !reflect.DeepEqual(st.Workers, want) {
		t.Fatalf("workers = %+v\nwant      %+v", st.Workers, want)
	}

	// w2 stops talking: after a lease with no word it's shown as gone.
	*now = now.Add(2 * time.Minute)
	getJSON(t, hs.URL+"/status", &st)
	if st.Workers[1].State != "gone" {
		t.Fatalf("w2 = %+v, want gone", st.Workers[1])
	}
}

func TestFinishedWorkerAndRestart(t *testing.T) {
	s, hs, _ := newServer(t, "a")
	claim(t, hs.URL, "w1")
	post(t, hs.URL+"/jobs/1/complete", api.CompleteRequest{Worker: "w1", Attempt: 1, Result: jobs.Result{Pages: 2}})
	claim(t, hs.URL, "w1")

	var st api.Status
	getJSON(t, hs.URL+"/status", &st)
	if len(st.Workers) != 1 || st.Workers[0].State != "finished" {
		t.Fatalf("workers = %+v", st.Workers)
	}

	// A restarted coordinator hasn't seen anyone yet, but still lists who did the work.
	s.seen = map[string]*seenWorker{}
	getJSON(t, hs.URL+"/status", &st)
	if len(st.Workers) != 1 || st.Workers[0] != (api.Worker{Name: "w1", State: "offline", LastSeenSeconds: -1, Done: 1, Pages: 2}) {
		t.Fatalf("workers = %+v", st.Workers)
	}
}

func TestTimeline(t *testing.T) {
	_, hs, now := newServer(t, "a", "b")
	claim(t, hs.URL, "w1")
	claim(t, hs.URL, "w2")
	post(t, hs.URL+"/jobs/2/complete", api.CompleteRequest{Worker: "w2", Attempt: 1, Result: jobs.Result{Pages: 3, DurationMs: 400}})
	*now = now.Add(time.Second)
	post(t, hs.URL+"/jobs/1/complete", api.CompleteRequest{Worker: "w1", Attempt: 1, Result: jobs.Result{Pages: 5, DurationMs: 900}})

	var tl api.Timeline
	getJSON(t, hs.URL+"/timeline", &tl)
	if len(tl.Jobs) != 2 || tl.Jobs[0].Path != "b" || tl.Jobs[0].Worker != "w2" || tl.Jobs[1].Path != "a" || tl.Jobs[1].DurationMs != 900 {
		t.Fatalf("timeline = %+v", tl.Jobs)
	}
}
