package record

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/vasishtk1/Linkscope/internal/jobs"
)

var t0 = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

func TestFromStore(t *testing.T) {
	s, err := jobs.Open(filepath.Join(t.TempDir(), "jobs.db"), 2)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Add([]string{"a", "b", "c"})
	claim := func(w string, at time.Duration) jobs.Job {
		j, ok, err := s.Claim(w, time.Minute, t0.Add(at))
		if !ok || err != nil {
			t.Fatalf("claim: %v %v", ok, err)
		}
		return j
	}
	a := claim("w2", 0)
	b := claim("w1", time.Second)
	s.Complete(a.ID, jobs.Result{Pages: 10, CrossLinks: 40, Edges: 20, DurationMs: 3000}, t0.Add(3*time.Second))
	s.Fail(b.ID, b.Attempt, "download error", t0.Add(4*time.Second))
	b2 := claim("w1", 5*time.Second)
	c := claim("w2", 5*time.Second)
	s.Complete(b2.ID, jobs.Result{Pages: 7, CrossLinks: 9, Edges: 5}, t0.Add(8*time.Second))
	s.Fail(c.ID, c.Attempt, "corrupt", t0.Add(9*time.Second))
	c2 := claim("w2", 10*time.Second)
	s.Fail(c2.ID, c2.Attempt, "corrupt again", t0.Add(12*time.Second))

	r, err := FromStore(s, "test run", t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	want := Totals{Files: 3, Done: 2, Failed: 1, Pages: 17, CrossLinks: 49, Edges: 25, FailedAttempts: 3, Seconds: 12}
	if r.Totals != want {
		t.Fatalf("totals = %+v, want %+v", r.Totals, want)
	}
	if r.Derived || r.Name != "test run" || len(r.Events) != 10 || len(r.Files) != 3 {
		t.Fatalf("recording = %+v", r)
	}
	if len(r.Workers) != 2 || r.Workers[0] != "w1" || r.Workers[1] != "w2" {
		t.Fatalf("workers = %v", r.Workers)
	}
	if f := r.Files[2]; f.Path != "c" || f.State != jobs.Failed || f.Attempts != 2 {
		t.Fatalf("file c = %+v", f)
	}
}

func TestDeriveFromFinishedJobs(t *testing.T) {
	rows := []jobs.JobRow{
		{ID: 1, Path: "a", State: jobs.Done, Attempt: 1, Worker: "w1", Pages: 5, DurationMs: 4000, FinishedAt: t0.Add(5 * time.Second)},
		{ID: 2, Path: "b", State: jobs.Done, Attempt: 2, Worker: "w2", Pages: 3, DurationMs: 1000, FinishedAt: t0.Add(2 * time.Second)},
		{ID: 3, Path: "c", State: jobs.Pending},
	}
	ev := derive(rows)
	type row struct {
		job  int64
		kind jobs.EventKind
		at   time.Duration
	}
	want := []row{{1, jobs.EventClaimed, time.Second}, {2, jobs.EventClaimed, time.Second}, {2, jobs.EventCompleted, 2 * time.Second}, {1, jobs.EventCompleted, 5 * time.Second}}
	if len(ev) != len(want) {
		t.Fatalf("events = %+v", ev)
	}
	for i, w := range want {
		e := ev[i]
		if e.Job != w.job || e.Kind != w.kind || e.At.Sub(t0) != w.at || e.ID != int64(i+1) {
			t.Errorf("event %d = %+v, want %+v", i, e, w)
		}
	}
	if ev[3].Pages != 5 || ev[1].Worker != "w2" || ev[1].Attempt != 2 {
		t.Fatalf("details lost: %+v", ev)
	}
}
