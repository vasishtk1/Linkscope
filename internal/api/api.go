package api

import (
	"time"

	"github.com/vasishtk1/Linkscope/internal/jobs"
)

type ClaimRequest struct {
	Worker string `json:"worker"`
}

type ClaimResponse struct {
	Job *Job `json:"job,omitempty"`
	// Finished is true once nothing is pending or running, so workers can exit.
	Finished bool `json:"finished"`
}

type Job struct {
	ID           int64   `json:"id"`
	Path         string  `json:"path"`
	Attempt      int     `json:"attempt"`
	LeaseSeconds float64 `json:"leaseSeconds"`
}

type LeaseRequest struct {
	Worker  string `json:"worker"`
	Attempt int    `json:"attempt"`
}

type CompleteRequest struct {
	Worker  string      `json:"worker"`
	Attempt int         `json:"attempt"`
	Result  jobs.Result `json:"result"`
}

type FailRequest struct {
	Worker  string `json:"worker"`
	Attempt int    `json:"attempt"`
	Error   string `json:"error"`
}

type Status struct {
	jobs.Summary
	Total          int      `json:"total"`
	UptimeSeconds  float64  `json:"uptimeSeconds"`
	JobsPerMinute  float64  `json:"jobsPerMinute"`
	PagesPerSecond float64  `json:"pagesPerSecond"`
	Workers        []Worker `json:"workers"`
	// Now is the coordinator's clock, so the dashboard can place jobs in time without trusting the browser's.
	Now time.Time `json:"now"`
}

// Worker is one worker as the coordinator last saw it.
type Worker struct {
	Name            string  `json:"name"`
	State           string  `json:"state"`
	JobID           int64   `json:"jobId,omitempty"`
	Path            string  `json:"path,omitempty"`
	JobSeconds      float64 `json:"jobSeconds,omitempty"`
	LastSeenSeconds float64 `json:"lastSeenSeconds"`
	Done            int     `json:"done"`
	Pages           int64   `json:"pages"`
}

type Timeline struct {
	Jobs []jobs.FinishedJob `json:"jobs"`
}
