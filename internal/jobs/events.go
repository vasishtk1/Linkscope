package jobs

import (
	"database/sql"
	"time"
)

type EventKind string

const (
	EventClaimed   EventKind = "claimed"
	EventCompleted EventKind = "completed"
	EventFailed    EventKind = "failed"
	EventExpired   EventKind = "expired"
	EventReleased  EventKind = "released"
)

// Event is one thing that happened to a job; Final means the job won't be tried again.
type Event struct {
	ID         int64     `json:"id"`
	At         time.Time `json:"at"`
	Job        int64     `json:"job"`
	Path       string    `json:"path"`
	Worker     string    `json:"worker"`
	Kind       EventKind `json:"kind"`
	Attempt    int       `json:"attempt"`
	Final      bool      `json:"final,omitempty"`
	Pages      int       `json:"pages,omitempty"`
	CrossLinks int       `json:"crossLinks,omitempty"`
	Edges      int       `json:"edges,omitempty"`
	DurationMs int64     `json:"durationMs,omitempty"`
	Error      string    `json:"error,omitempty"`
}

func logEvent(tx *sql.Tx, e Event) error {
	_, err := tx.Exec(`
		INSERT INTO events (at, job, worker, kind, attempt, final, pages, cross_links, edges, duration_ms, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.At.UnixMilli(), e.Job, e.Worker, e.Kind, e.Attempt, e.Final, e.Pages, e.CrossLinks, e.Edges, e.DurationMs, e.Error)
	return err
}

// Events returns every event with an id above after, oldest first, with each job's file path.
func (s *Store) Events(after int64) ([]Event, error) {
	rows, err := s.db.Query(`
		SELECT e.id, e.at, e.job, j.path, e.worker, e.kind, e.attempt, e.final, e.pages, e.cross_links, e.edges, e.duration_ms, e.error
		FROM events e JOIN jobs j ON j.id = e.job
		WHERE e.id > ? ORDER BY e.id`, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		var at int64
		if err := rows.Scan(&e.ID, &at, &e.Job, &e.Path, &e.Worker, &e.Kind, &e.Attempt, &e.Final,
			&e.Pages, &e.CrossLinks, &e.Edges, &e.DurationMs, &e.Error); err != nil {
			return nil, err
		}
		e.At = time.UnixMilli(at)
		out = append(out, e)
	}
	return out, rows.Err()
}
