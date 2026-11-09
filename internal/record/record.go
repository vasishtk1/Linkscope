package record

import (
	"cmp"
	"slices"
	"time"

	"github.com/vasishtk1/Linkscope/internal/jobs"
)

// Recording is everything needed to replay a finished run: its files, its workers and every event in order.
type Recording struct {
	Name       string       `json:"name"`
	RecordedAt time.Time    `json:"recordedAt"`
	Derived    bool         `json:"derived"`
	Files      []File       `json:"files"`
	Workers    []string     `json:"workers"`
	Events     []jobs.Event `json:"events"`
	Totals     Totals       `json:"totals"`
}

type File struct {
	ID         int64      `json:"id"`
	Path       string     `json:"path"`
	State      jobs.State `json:"state"`
	Attempts   int        `json:"attempts"`
	Pages      int        `json:"pages"`
	CrossLinks int        `json:"crossLinks"`
	Edges      int        `json:"edges"`
}

type Totals struct {
	Files          int     `json:"files"`
	Done           int     `json:"done"`
	Failed         int     `json:"failed"`
	Pages          int64   `json:"pages"`
	CrossLinks     int64   `json:"crossLinks"`
	Edges          int64   `json:"edges"`
	FailedAttempts int     `json:"failedAttempts"`
	Seconds        float64 `json:"seconds"`
}

// FromStore saves the run in a job database; without an event log it rebuilds one from each finished job's timing.
func FromStore(s *jobs.Store, name string, now time.Time) (Recording, error) {
	rows, err := s.List()
	if err != nil {
		return Recording{}, err
	}
	events, err := s.Events(0)
	if err != nil {
		return Recording{}, err
	}
	r := Recording{Name: name, RecordedAt: now, Files: []File{}, Workers: []string{}, Events: events}
	if len(events) == 0 {
		r.Derived = true
		r.Events = derive(rows)
	}

	for _, j := range rows {
		r.Files = append(r.Files, File{ID: j.ID, Path: j.Path, State: j.State, Attempts: j.Attempt, Pages: j.Pages, CrossLinks: j.CrossLinks, Edges: j.Edges})
		r.Totals.Files++
		switch j.State {
		case jobs.Done:
			r.Totals.Done++
			r.Totals.Pages += int64(j.Pages)
			r.Totals.CrossLinks += int64(j.CrossLinks)
			r.Totals.Edges += int64(j.Edges)
		case jobs.Failed:
			r.Totals.Failed++
		}
	}
	seen := map[string]bool{}
	for _, e := range r.Events {
		if e.Kind == jobs.EventFailed || e.Kind == jobs.EventExpired {
			r.Totals.FailedAttempts++
		}
		if e.Worker != "" && !seen[e.Worker] {
			seen[e.Worker] = true
			r.Workers = append(r.Workers, e.Worker)
		}
	}
	slices.Sort(r.Workers)
	if n := len(r.Events); n > 0 {
		r.Totals.Seconds = r.Events[n-1].At.Sub(r.Events[0].At).Seconds()
	}
	return r, nil
}

// derive turns each finished job into a claim and a completion; retries and failures before the event log existed are lost.
func derive(rows []jobs.JobRow) []jobs.Event {
	out := []jobs.Event{}
	for _, j := range rows {
		if j.State != jobs.Done {
			continue
		}
		start := j.FinishedAt.Add(-time.Duration(j.DurationMs) * time.Millisecond)
		out = append(out,
			jobs.Event{At: start, Job: j.ID, Path: j.Path, Worker: j.Worker, Kind: jobs.EventClaimed, Attempt: j.Attempt},
			jobs.Event{At: j.FinishedAt, Job: j.ID, Path: j.Path, Worker: j.Worker, Kind: jobs.EventCompleted, Attempt: j.Attempt,
				Pages: j.Pages, CrossLinks: j.CrossLinks, Edges: j.Edges, DurationMs: j.DurationMs},
		)
	}
	slices.SortStableFunc(out, func(a, b jobs.Event) int { return cmp.Compare(a.At.UnixMilli(), b.At.UnixMilli()) })
	for i := range out {
		out[i].ID = int64(i + 1)
	}
	return out
}
