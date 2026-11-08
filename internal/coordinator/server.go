package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vasishtk1/Linkscope/internal/api"
	"github.com/vasishtk1/Linkscope/internal/jobs"
)

type Server struct {
	store *jobs.Store
	lease time.Duration
	now   func() time.Time
	logf  func(string, ...any)

	mu         sync.Mutex
	started    time.Time
	firstDone  int
	firstPages int64
	seen       map[string]*seenWorker
}

// seenWorker is what the coordinator remembers about a worker from its requests; it's lost on restart.
type seenWorker struct {
	last     time.Time
	jobID    int64
	path     string
	since    time.Time
	finished bool
}

func New(store *jobs.Store, lease time.Duration) *Server {
	return &Server{store: store, lease: lease, now: time.Now, logf: log.Printf, seen: map[string]*seenWorker{}}
}

// saw records a request from a worker; update changes what it's doing.
func (s *Server) saw(name string, update func(*seenWorker)) {
	if name == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w := s.seen[name]
	if w == nil {
		w = &seenWorker{}
		s.seen[name] = w
	}
	w.last = s.now()
	if update != nil {
		update(w)
	}
}

func idle(w *seenWorker) { w.jobID, w.path = 0, "" }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /jobs/claim", s.claim)
	mux.HandleFunc("POST /jobs/{id}/heartbeat", s.heartbeat)
	mux.HandleFunc("POST /jobs/{id}/complete", s.complete)
	mux.HandleFunc("POST /jobs/{id}/fail", s.fail)
	mux.HandleFunc("POST /jobs/{id}/release", s.release)
	mux.HandleFunc("GET /status", s.status)
	mux.HandleFunc("GET /timeline", s.timeline)
	return mux
}

// RunReaper expires dead leases on a timer so the status page stays accurate between claims.
func (s *Server) RunReaper(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n, err := s.store.Expire(s.now()); err != nil {
				s.logf("reaper: %v", err)
			} else if n > 0 {
				s.logf("reaper: %d lease(s) expired, jobs returned to the queue", n)
			}
		}
	}
}

func (s *Server) claim(w http.ResponseWriter, r *http.Request) {
	var req api.ClaimRequest
	if !decode(w, r, &req) || !requireWorker(w, req.Worker) {
		return
	}
	j, ok, err := s.store.Claim(req.Worker, s.lease, s.now())
	if err != nil {
		s.internal(w, err)
		return
	}
	if !ok {
		sum, err := s.store.Summary(0)
		if err != nil {
			s.internal(w, err)
			return
		}
		s.saw(req.Worker, func(sw *seenWorker) { idle(sw); sw.finished = sum.Finished() })
		writeJSON(w, http.StatusOK, api.ClaimResponse{Finished: sum.Finished()})
		return
	}
	s.saw(req.Worker, func(sw *seenWorker) { sw.jobID, sw.path, sw.since, sw.finished = j.ID, j.Path, s.now(), false })
	s.logf("job %d (attempt %d) -> %s", j.ID, j.Attempt, req.Worker)
	writeJSON(w, http.StatusOK, api.ClaimResponse{Job: &api.Job{
		ID: j.ID, Path: j.Path, Attempt: j.Attempt, LeaseSeconds: s.lease.Seconds(),
	}})
}

func (s *Server) heartbeat(w http.ResponseWriter, r *http.Request) {
	var req api.LeaseRequest
	id, ok := jobID(w, r)
	if !ok || !decode(w, r, &req) {
		return
	}
	s.saw(req.Worker, nil)
	s.leaseResult(w, s.store.Heartbeat(id, req.Attempt, s.lease, s.now()))
}

func (s *Server) complete(w http.ResponseWriter, r *http.Request) {
	var req api.CompleteRequest
	id, ok := jobID(w, r)
	if !ok || !decode(w, r, &req) {
		return
	}
	if err := s.store.Complete(id, req.Result, s.now()); err != nil {
		s.internal(w, err)
		return
	}
	s.saw(req.Worker, idle)
	s.logf("job %d done by %s: %d pages, %d edges in %dms", id, req.Worker, req.Result.Pages, req.Result.Edges, req.Result.DurationMs)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request) {
	var req api.FailRequest
	id, ok := jobID(w, r)
	if !ok || !decode(w, r, &req) {
		return
	}
	err := s.store.Fail(id, req.Attempt, req.Error)
	s.saw(req.Worker, idle)
	if err == nil {
		s.logf("job %d attempt %d failed on %s: %s", id, req.Attempt, req.Worker, req.Error)
	}
	s.leaseResult(w, err)
}

func (s *Server) release(w http.ResponseWriter, r *http.Request) {
	var req api.LeaseRequest
	id, ok := jobID(w, r)
	if !ok || !decode(w, r, &req) {
		return
	}
	err := s.store.Release(id, req.Attempt)
	s.saw(req.Worker, idle)
	if err == nil {
		s.logf("job %d released by %s", id, req.Worker)
	}
	s.leaseResult(w, err)
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	sum, err := s.store.Summary(20)
	if err != nil {
		s.internal(w, err)
		return
	}
	now := s.now()

	// Rates count only work finished since this coordinator started, not jobs from earlier runs.
	s.mu.Lock()
	started, firstDone, firstPages := s.started, s.firstDone, s.firstPages
	s.mu.Unlock()

	st := api.Status{
		Summary: sum,
		Total:   sum.Pending + sum.Running + sum.Done + sum.Failed,
		Now:     now,
	}
	if !started.IsZero() {
		uptime := now.Sub(started)
		st.UptimeSeconds = uptime.Seconds()
		doneSince, pagesSince := sum.Done-firstDone, sum.Pages-firstPages
		if uptime > 0 {
			st.JobsPerMinute = float64(doneSince) / uptime.Minutes()
			st.PagesPerSecond = float64(pagesSince) / uptime.Seconds()
		}
	}
	if st.Workers, err = s.workers(now); err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// workers merges the workers seen since startup with every worker that finished a job in the database.
func (s *Server) workers(now time.Time) ([]api.Worker, error) {
	totals, err := s.store.ByWorker()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []api.Worker{}
	for name, sw := range s.seen {
		w := api.Worker{Name: name, LastSeenSeconds: now.Sub(sw.last).Seconds(), Done: totals[name].Done, Pages: totals[name].Pages}
		switch {
		case sw.finished:
			w.State = "finished"
		case now.Sub(sw.last) > s.lease:
			w.State = "gone"
		case sw.jobID != 0:
			w.State, w.JobID, w.Path, w.JobSeconds = "working", sw.jobID, sw.path, now.Sub(sw.since).Seconds()
		default:
			w.State = "idle"
		}
		out = append(out, w)
	}
	for name, t := range totals {
		if _, ok := s.seen[name]; !ok && name != "" {
			out = append(out, api.Worker{Name: name, State: "offline", LastSeenSeconds: -1, Done: t.Done, Pages: t.Pages})
		}
	}
	slices.SortFunc(out, func(a, b api.Worker) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

func (s *Server) timeline(w http.ResponseWriter, r *http.Request) {
	fin, err := s.store.Finished()
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, api.Timeline{Jobs: fin})
}

// MarkStarted sets the starting point for throughput; status shows no rates until it's called.
func (s *Server) MarkStarted() error {
	sum, err := s.store.Summary(0)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.started, s.firstDone, s.firstPages = s.now(), sum.Done, sum.Pages
	s.mu.Unlock()
	return nil
}

func (s *Server) leaseResult(w http.ResponseWriter, err error) {
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, jobs.ErrLeaseLost):
		http.Error(w, "lease lost", http.StatusConflict)
	default:
		s.internal(w, err)
	}
}

func (s *Server) internal(w http.ResponseWriter, err error) {
	s.logf("error: %v", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}

func jobID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "bad job id", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return false
	}
	return true
}

func requireWorker(w http.ResponseWriter, worker string) bool {
	if worker == "" {
		http.Error(w, "worker is required", http.StatusBadRequest)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
