package jobs

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type State string

const (
	Pending State = "pending"
	Running State = "running"
	Done    State = "done"
	Failed  State = "failed"
)

var ErrLeaseLost = errors.New("jobs: lease lost")

type Job struct {
	ID           int64
	Path         string
	State        State
	Attempt      int
	Worker       string
	LeaseExpires time.Time
	LastError    string
}

type Result struct {
	Pages      int   `json:"pages"`
	CrossLinks int   `json:"crossLinks"`
	Edges      int   `json:"edges"`
	DurationMs int64 `json:"durationMs"`
}

type Store struct {
	db          *sql.DB
	maxAttempts int
}

const schema = `
CREATE TABLE IF NOT EXISTS jobs (
	id            INTEGER PRIMARY KEY,
	path          TEXT    NOT NULL UNIQUE,
	state         TEXT    NOT NULL DEFAULT 'pending',
	attempt       INTEGER NOT NULL DEFAULT 0,
	worker        TEXT    NOT NULL DEFAULT '',
	lease_expires INTEGER NOT NULL DEFAULT 0,
	last_error    TEXT    NOT NULL DEFAULT '',
	pages         INTEGER NOT NULL DEFAULT 0,
	cross_links   INTEGER NOT NULL DEFAULT 0,
	edges         INTEGER NOT NULL DEFAULT 0,
	duration_ms   INTEGER NOT NULL DEFAULT 0,
	finished_at   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS jobs_by_state ON jobs (state, id);
`

func Open(path string, maxAttempts int) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(FULL)")
	if err != nil {
		return nil, err
	}
	// One connection serializes every transaction, so two claims can never grab the same job.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("jobs: create schema: %w", err)
	}
	return &Store{db: db, maxAttempts: maxAttempts}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Add inserts new paths as pending jobs and skips ones already present.
func (s *Store) Add(paths []string) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	added := 0
	for _, p := range paths {
		res, err := tx.Exec(`INSERT OR IGNORE INTO jobs (path) VALUES (?)`, p)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		added += int(n)
	}
	return added, tx.Commit()
}

// Claim leases the oldest pending job to a worker; ok=false if none is available.
func (s *Store) Claim(worker string, lease time.Duration, now time.Time) (Job, bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return Job{}, false, err
	}
	defer tx.Rollback()

	if _, err := s.expire(tx, now); err != nil {
		return Job{}, false, err
	}
	var j Job
	err = tx.QueryRow(`SELECT id, path, attempt FROM jobs WHERE state = 'pending' ORDER BY id LIMIT 1`).
		Scan(&j.ID, &j.Path, &j.Attempt)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, false, tx.Commit()
	}
	if err != nil {
		return Job{}, false, err
	}
	j.Attempt++
	j.State = Running
	j.Worker = worker
	j.LeaseExpires = now.Add(lease)
	_, err = tx.Exec(`UPDATE jobs SET state = 'running', attempt = ?, worker = ?, lease_expires = ? WHERE id = ?`,
		j.Attempt, worker, j.LeaseExpires.UnixMilli(), j.ID)
	if err != nil {
		return Job{}, false, err
	}
	return j, true, tx.Commit()
}

// Expire returns jobs whose lease ran out to pending, or to failed once they've used every attempt.
func (s *Store) Expire(now time.Time) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	n, err := s.expire(tx, now)
	if err != nil {
		return 0, err
	}
	return n, tx.Commit()
}

func (s *Store) expire(tx *sql.Tx, now time.Time) (int, error) {
	res, err := tx.Exec(`
		UPDATE jobs
		SET state = CASE WHEN attempt >= ? THEN 'failed' ELSE 'pending' END,
		    last_error = 'lease expired on worker ' || worker,
		    worker = '', lease_expires = 0
		WHERE state = 'running' AND lease_expires <= ?`,
		s.maxAttempts, now.UnixMilli())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// Heartbeat extends a lease; ErrLeaseLost means the job was given to someone else.
func (s *Store) Heartbeat(id int64, attempt int, lease time.Duration, now time.Time) error {
	res, err := s.db.Exec(`
		UPDATE jobs SET lease_expires = ?
		WHERE id = ? AND attempt = ? AND state = 'running' AND lease_expires > ?`,
		now.Add(lease).UnixMilli(), id, attempt, now.UnixMilli())
	return leaseUpdate(res, err)
}

// Complete marks a job done; finishing an already-done job is a no-op.
func (s *Store) Complete(id int64, r Result, now time.Time) error {
	_, err := s.db.Exec(`
		UPDATE jobs
		SET state = 'done', worker = '', lease_expires = 0, last_error = '',
		    pages = ?, cross_links = ?, edges = ?, duration_ms = ?, finished_at = ?
		WHERE id = ? AND state != 'done'`,
		r.Pages, r.CrossLinks, r.Edges, r.DurationMs, now.UnixMilli(), id)
	return err
}

// Fail records an error and puts the job back in line, or marks it failed after the last attempt.
func (s *Store) Fail(id int64, attempt int, msg string) error {
	res, err := s.db.Exec(`
		UPDATE jobs
		SET state = CASE WHEN attempt >= ? THEN 'failed' ELSE 'pending' END,
		    last_error = ?, worker = '', lease_expires = 0
		WHERE id = ? AND attempt = ? AND state = 'running'`,
		s.maxAttempts, msg, id, attempt)
	return leaseUpdate(res, err)
}

// Release hands a job back without using up an attempt, for workers shutting down cleanly.
func (s *Store) Release(id int64, attempt int) error {
	res, err := s.db.Exec(`
		UPDATE jobs SET state = 'pending', attempt = attempt - 1, worker = '', lease_expires = 0
		WHERE id = ? AND attempt = ? AND state = 'running'`,
		id, attempt)
	return leaseUpdate(res, err)
}

func leaseUpdate(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrLeaseLost
	}
	return nil
}

func (s *Store) Get(id int64) (Job, error) {
	var j Job
	var lease int64
	err := s.db.QueryRow(`SELECT id, path, state, attempt, worker, lease_expires, last_error FROM jobs WHERE id = ?`, id).
		Scan(&j.ID, &j.Path, &j.State, &j.Attempt, &j.Worker, &lease, &j.LastError)
	if lease > 0 {
		j.LeaseExpires = time.UnixMilli(lease)
	}
	return j, err
}
