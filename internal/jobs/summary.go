package jobs

import "time"

type Summary struct {
	Pending    int           `json:"pending"`
	Running    int           `json:"running"`
	Done       int           `json:"done"`
	Failed     int           `json:"failed"`
	Pages      int64         `json:"pages"`
	CrossLinks int64         `json:"crossLinks"`
	Edges      int64         `json:"edges"`
	Active     []ActiveJob   `json:"active"`
	Failures   []FailedJob   `json:"failures"`
	Recent     []FinishedJob `json:"recent"`
}

type ActiveJob struct {
	ID           int64     `json:"id"`
	Path         string    `json:"path"`
	Worker       string    `json:"worker"`
	Attempt      int       `json:"attempt"`
	LeaseExpires time.Time `json:"leaseExpires"`
}

type FailedJob struct {
	ID       int64  `json:"id"`
	Path     string `json:"path"`
	Attempts int    `json:"attempts"`
	Error    string `json:"error"`
}

type FinishedJob struct {
	ID         int64     `json:"id"`
	Path       string    `json:"path"`
	Pages      int       `json:"pages"`
	Edges      int       `json:"edges"`
	DurationMs int64     `json:"durationMs"`
	FinishedAt time.Time `json:"finishedAt"`
}

// Finished is true when no job is pending or running.
func (s Summary) Finished() bool { return s.Pending == 0 && s.Running == 0 }

func (s *Store) Summary(recent int) (Summary, error) {
	var sum Summary
	rows, err := s.db.Query(`SELECT state, COUNT(*) FROM jobs GROUP BY state`)
	if err != nil {
		return sum, err
	}
	for rows.Next() {
		var st State
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			rows.Close()
			return sum, err
		}
		switch st {
		case Pending:
			sum.Pending = n
		case Running:
			sum.Running = n
		case Done:
			sum.Done = n
		case Failed:
			sum.Failed = n
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return sum, err
	}

	err = s.db.QueryRow(`SELECT COALESCE(SUM(pages),0), COALESCE(SUM(cross_links),0), COALESCE(SUM(edges),0) FROM jobs WHERE state = 'done'`).
		Scan(&sum.Pages, &sum.CrossLinks, &sum.Edges)
	if err != nil {
		return sum, err
	}

	if sum.Active, err = s.active(); err != nil {
		return sum, err
	}
	if sum.Failures, err = s.failures(); err != nil {
		return sum, err
	}
	sum.Recent, err = s.recent(recent)
	return sum, err
}

func (s *Store) active() ([]ActiveJob, error) {
	rows, err := s.db.Query(`SELECT id, path, worker, attempt, lease_expires FROM jobs WHERE state = 'running' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ActiveJob{}
	for rows.Next() {
		var a ActiveJob
		var lease int64
		if err := rows.Scan(&a.ID, &a.Path, &a.Worker, &a.Attempt, &lease); err != nil {
			return nil, err
		}
		a.LeaseExpires = time.UnixMilli(lease)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) failures() ([]FailedJob, error) {
	rows, err := s.db.Query(`SELECT id, path, attempt, last_error FROM jobs WHERE state = 'failed' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FailedJob{}
	for rows.Next() {
		var f FailedJob
		if err := rows.Scan(&f.ID, &f.Path, &f.Attempts, &f.Error); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) recent(limit int) ([]FinishedJob, error) {
	rows, err := s.db.Query(`
		SELECT id, path, pages, edges, duration_ms, finished_at FROM jobs
		WHERE state = 'done' ORDER BY finished_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FinishedJob{}
	for rows.Next() {
		var f FinishedJob
		var finished int64
		if err := rows.Scan(&f.ID, &f.Path, &f.Pages, &f.Edges, &f.DurationMs, &finished); err != nil {
			return nil, err
		}
		f.FinishedAt = time.UnixMilli(finished)
		out = append(out, f)
	}
	return out, rows.Err()
}
