package api

import "github.com/vasishtk1/Linkscope/internal/jobs"

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
	Total          int     `json:"total"`
	UptimeSeconds  float64 `json:"uptimeSeconds"`
	JobsPerMinute  float64 `json:"jobsPerMinute"`
	PagesPerSecond float64 `json:"pagesPerSecond"`
}
