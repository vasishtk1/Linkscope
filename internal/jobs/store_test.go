package jobs

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

const lease = time.Minute

func open(t *testing.T, maxAttempts int) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "jobs.db"), maxAttempts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func mustClaim(t *testing.T, s *Store, worker string, now time.Time) Job {
	t.Helper()
	j, ok, err := s.Claim(worker, lease, now)
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	return j
}

func TestAddSkipsDuplicates(t *testing.T) {
	s := open(t, 3)
	n, err := s.Add([]string{"a", "b"})
	if err != nil || n != 2 {
		t.Fatalf("added %d, err %v", n, err)
	}
	n, err = s.Add([]string{"b", "c"})
	if err != nil || n != 1 {
		t.Fatalf("second add: added %d, err %v", n, err)
	}
}

func TestClaimHandsOutJobsInOrderOnce(t *testing.T) {
	s := open(t, 3)
	s.Add([]string{"a", "b"})
	j1 := mustClaim(t, s, "w1", t0)
	j2 := mustClaim(t, s, "w2", t0)
	if j1.Path != "a" || j2.Path != "b" || j1.Attempt != 1 {
		t.Fatalf("got %+v and %+v", j1, j2)
	}
	if _, ok, _ := s.Claim("w3", lease, t0); ok {
		t.Fatal("no jobs should be left")
	}
}

func TestConcurrentClaimsNeverShareAJob(t *testing.T) {
	s := open(t, 3)
	paths := make([]string, 50)
	for i := range paths {
		paths[i] = string(rune('A' + i))
	}
	s.Add(paths)

	var mu sync.Mutex
	seen := map[int64]string{}
	var wg sync.WaitGroup
	for w := range 8 {
		wg.Go(func() {
			worker := string(rune('a' + w))
			for {
				j, ok, err := s.Claim(worker, lease, t0)
				if err != nil {
					t.Error(err)
					return
				}
				if !ok {
					return
				}
				mu.Lock()
				if other, dup := seen[j.ID]; dup {
					t.Errorf("job %d given to %s and %s", j.ID, other, worker)
				}
				seen[j.ID] = worker
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if len(seen) != len(paths) {
		t.Fatalf("claimed %d of %d jobs", len(seen), len(paths))
	}
}

func TestExpiredLeaseGoesToAnotherWorker(t *testing.T) {
	s := open(t, 3)
	s.Add([]string{"a"})
	first := mustClaim(t, s, "w1", t0)

	if _, ok, _ := s.Claim("w2", lease, t0.Add(30*time.Second)); ok {
		t.Fatal("lease still valid, job should not be reassigned")
	}
	second := mustClaim(t, s, "w2", t0.Add(lease))
	if second.ID != first.ID || second.Attempt != 2 || second.Worker != "w2" {
		t.Fatalf("second claim = %+v", second)
	}
}

func TestHeartbeatKeepsLeaseAlive(t *testing.T) {
	s := open(t, 3)
	s.Add([]string{"a"})
	j := mustClaim(t, s, "w1", t0)
	if err := s.Heartbeat(j.ID, j.Attempt, lease, t0.Add(50*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Claim("w2", lease, t0.Add(90*time.Second)); ok {
		t.Fatal("heartbeat should have extended the lease past 90s")
	}
}

func TestStaleWorkerLosesLease(t *testing.T) {
	s := open(t, 3)
	s.Add([]string{"a"})
	old := mustClaim(t, s, "w1", t0)
	mustClaim(t, s, "w2", t0.Add(2*lease))

	if err := s.Heartbeat(old.ID, old.Attempt, lease, t0.Add(2*lease)); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale heartbeat: %v", err)
	}
	if err := s.Fail(old.ID, old.Attempt, "boom"); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale fail: %v", err)
	}
	if j, _ := s.Get(old.ID); j.State != Running || j.Worker != "w2" {
		t.Fatalf("stale worker changed the job: %+v", j)
	}
}

func TestHeartbeatAfterExpiryFails(t *testing.T) {
	s := open(t, 3)
	s.Add([]string{"a"})
	j := mustClaim(t, s, "w1", t0)
	if err := s.Heartbeat(j.ID, j.Attempt, lease, t0.Add(lease)); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("expected ErrLeaseLost, got %v", err)
	}
}

func TestFailRetriesThenGivesUp(t *testing.T) {
	s := open(t, 2)
	s.Add([]string{"a"})
	j := mustClaim(t, s, "w1", t0)
	if err := s.Fail(j.ID, j.Attempt, "download error"); err != nil {
		t.Fatal(err)
	}
	j = mustClaim(t, s, "w1", t0)
	if j.Attempt != 2 {
		t.Fatalf("attempt = %d", j.Attempt)
	}
	s.Fail(j.ID, j.Attempt, "download error again")
	got, _ := s.Get(j.ID)
	if got.State != Failed || got.LastError != "download error again" {
		t.Fatalf("job = %+v", got)
	}
	if _, ok, _ := s.Claim("w1", lease, t0); ok {
		t.Fatal("failed job should not be handed out")
	}
}

func TestExpiryCountsAsAnAttempt(t *testing.T) {
	s := open(t, 2)
	s.Add([]string{"a"})
	mustClaim(t, s, "w1", t0)
	mustClaim(t, s, "w2", t0.Add(lease))
	if n, _ := s.Expire(t0.Add(3 * lease)); n != 1 {
		t.Fatalf("expired %d", n)
	}
	j, _ := s.Get(1)
	if j.State != Failed || j.LastError != "lease expired on worker w2" {
		t.Fatalf("job = %+v", j)
	}
}

func TestReleaseDoesNotUseAnAttempt(t *testing.T) {
	s := open(t, 1)
	s.Add([]string{"a"})
	j := mustClaim(t, s, "w1", t0)
	if err := s.Release(j.ID, j.Attempt); err != nil {
		t.Fatal(err)
	}
	again := mustClaim(t, s, "w2", t0)
	if again.Attempt != 1 {
		t.Fatalf("attempt = %d, want 1", again.Attempt)
	}
}

func TestCompleteIsIdempotent(t *testing.T) {
	s := open(t, 3)
	s.Add([]string{"a"})
	old := mustClaim(t, s, "w1", t0)
	mustClaim(t, s, "w2", t0.Add(lease))

	if err := s.Complete(old.ID, Result{Pages: 10, Edges: 5}, t0.Add(lease)); err != nil {
		t.Fatal(err)
	}
	if err := s.Complete(old.ID, Result{Pages: 999, Edges: 999}, t0.Add(lease)); err != nil {
		t.Fatal(err)
	}
	sum, err := s.Summary(10)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Done != 1 || sum.Running != 0 || sum.Pages != 10 || sum.Edges != 5 {
		t.Fatalf("summary = %+v", sum)
	}
	if !sum.Finished() {
		t.Fatal("expected finished")
	}
}

func TestStateSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.db")
	s, err := Open(path, 3)
	if err != nil {
		t.Fatal(err)
	}
	s.Add([]string{"a", "b"})
	j, _, _ := s.Claim("w1", lease, t0)
	s.Complete(j.ID, Result{Pages: 1}, t0)
	s.Close()

	s, err = Open(path, 3)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	sum, _ := s.Summary(10)
	if sum.Done != 1 || sum.Pending != 1 {
		t.Fatalf("after restart: %+v", sum)
	}
}

func TestSummaryLists(t *testing.T) {
	s := open(t, 1)
	s.Add([]string{"a", "b", "c"})
	a := mustClaim(t, s, "w1", t0)
	b := mustClaim(t, s, "w2", t0)
	mustClaim(t, s, "w3", t0)
	s.Complete(a.ID, Result{Pages: 3, Edges: 2, DurationMs: 100}, t0)
	s.Fail(b.ID, b.Attempt, "corrupt file")

	sum, err := s.Summary(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sum.Active) != 1 || sum.Active[0].Worker != "w3" {
		t.Fatalf("active = %+v", sum.Active)
	}
	if len(sum.Failures) != 1 || sum.Failures[0].Error != "corrupt file" {
		t.Fatalf("failures = %+v", sum.Failures)
	}
	if len(sum.Recent) != 1 || sum.Recent[0].Path != "a" {
		t.Fatalf("recent = %+v", sum.Recent)
	}
}
