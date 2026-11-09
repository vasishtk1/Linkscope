# internal/jobs

The coordinator's job list, stored in SQLite. One row per Common Crawl file. This package is where "no file gets lost" and "no file is given to two workers at once" are enforced.

## Job lifecycle

```
            Claim                        Complete
 pending ─────────▶ running ─────────────────────────▶ done
    ▲                  │
    │  Fail / lease    │  Fail / lease expires
    │  expires         │  on the last attempt
    │  (attempts left) ▼
    └──────────────  (back to pending)        failed
                       │
                       └── Release (clean shutdown): back to pending, attempt not counted
```

## The table

| Column | Meaning |
|---|---|
| `id` | Job number, assigned in insert order |
| `path` | Common Crawl path of the WAT file (unique) |
| `state` | `pending`, `running`, `done`, or `failed` |
| `attempt` | How many times the job has been handed out |
| `worker` | Who holds it right now. Kept after the job is done, so the dashboard can show who did which file. Cleared when a job goes back in line |
| `lease_expires` | When the current lease runs out (Unix ms) |
| `last_error` | Why the last attempt failed |
| `pages`, `cross_links`, `edges`, `duration_ms`, `finished_at` | Results reported on completion |

An index on `(state, id)` makes "oldest pending job" a quick lookup instead of a full table scan.

## The event log

The `jobs` table only holds each file's **current** state. A file that failed twice and then worked just says `done`. To replay a run on the website, the store also keeps an **`events`** table: one row every time something happens to a job.

| Column | Meaning |
|---|---|
| `id` | Event number, in the order they happened |
| `at` | When (Unix ms, from the coordinator's clock) |
| `job`, `worker`, `attempt` | Which job, which worker, which attempt |
| `kind` | `claimed`, `completed`, `failed`, `expired` (the worker went silent), or `released` (the worker shut down cleanly) |
| `final` | For `failed` and `expired`: the job has used every attempt and won't be tried again |
| `pages`, `cross_links`, `edges`, `duration_ms` | For `completed`: the results |
| `error` | For `failed` and `expired`: why |

**Every event is written in the same transaction as the job change it describes.** Either both are saved or neither is, so the history can never disagree with the jobs table, even if the coordinator crashes halfway. Actions that are refused (a stale worker's `Fail`, a second `Complete`) change nothing and log nothing.

A 20-file run produces about 40–60 events, a 1,000-file run a few thousand, so the table stays small.

Older databases get the `events` table automatically on `Open` (`CREATE TABLE IF NOT EXISTS`), but they have no history from before it existed.

## Types

### `State`

`Pending`, `Running`, `Done`, `Failed`.

### `Job`

A row as Go values: `ID`, `Path`, `State`, `Attempt`, `Worker`, `LeaseExpires`, `LastError`.

### `Result`

What a worker reports when it finishes: `Pages`, `CrossLinks`, `Edges`, `DurationMs`.

### `Store`

Wraps the database connection and the `maxAttempts` setting.

### `ErrLeaseLost`

Returned when a worker acts on a job it no longer holds, because its lease expired and the job moved on. The coordinator turns this into HTTP `409 Conflict`.

## Functions

### `Open(path string, maxAttempts int) (*Store, error)`

Opens (or creates) the database file and its table.

Settings:
- **WAL journal mode:** reads don't block writes, so the status page doesn't slow down claims.
- **`synchronous(FULL)`:** every commit is flushed to disk, so a coordinator crash never loses a job's state change.
- **`busy_timeout(5000)`:** waits up to 5s for a lock instead of failing immediately.
- **One open connection (`SetMaxOpenConns(1)`):** every transaction runs one at a time. That's what makes it impossible for two simultaneous claims to grab the same job. A test checks this with 8 goroutines claiming 50 jobs at once.

### `(*Store) Close() error`

Closes the database.

### `(*Store) Add(paths []string) (int, error)`

Adds paths as pending jobs in one transaction and returns how many were new. Uses `INSERT OR IGNORE`, so re-running the coordinator with the same or a bigger list never duplicates jobs. Running with `-limit 20` then `-limit 50` queues just the 30 new files.

### `(*Store) Claim(worker string, lease time.Duration, now time.Time) (Job, bool, error)`

Gives the oldest pending job to `worker`. In one transaction it:
1. Expires any dead leases first (see `expire`), so their jobs can be handed out right away
2. Picks the lowest-id pending job
3. Marks it running, increments `attempt`, records the worker, and sets `lease_expires = now + lease`

Returns `ok=false` if nothing is pending.

**Why `now` is a parameter:** tests pass in a fake clock, so "the lease ran out 60 seconds later" takes no real time to test.

**The attempt number is the lease's ID.** Every later call (heartbeat, fail, release) must include it. A worker whose lease expired still has the old attempt number, so its calls no longer match and are rejected. This is how a "zombie" worker that comes back from a long freeze is kept from interfering with the worker that took over.

### `(*Store) Expire(now time.Time) (int, error)`

Moves every running job whose lease has passed back to `pending`, or to `failed` if it has used all its attempts. Returns how many it moved. The coordinator calls this every second, and `Claim` calls it too.

### `expire(tx, now)` (unexported)

The shared SQL behind `Expire` and `Claim`, run inside the caller's transaction. Records `lease expired on worker <name>` as the error. First it reads which jobs are about to expire, and logs an `expired` event for each with the worker that lost it, because the update clears the worker's name.

### `(*Store) Heartbeat(id int64, attempt int, lease time.Duration, now time.Time) error`

Extends the lease to `now + lease`. Only succeeds if the job is still running, still on this attempt, **and the lease hasn't already run out**. Without that last check, a worker that froze past its deadline could revive a lease the coordinator was about to hand to someone else. Returns `ErrLeaseLost` otherwise.

### `(*Store) Complete(id int64, r Result, now time.Time) error`

Marks the job done and saves the results. **Accepted from any attempt**, even a stale one, as long as the job isn't already done. Completing twice changes nothing.

The `worker` column is left as it is, so a finished job remembers which worker held it last. That's almost always the worker that finished it. The rare exception is a stale worker finishing after its lease moved to someone else; the job is then credited to the new holder.

It runs in a transaction: read the job, skip it if it's already done, update it, and log a `completed` event with the results.

**Why stale completions are accepted:** the result file is the same no matter which worker produced it, since extraction is deterministic and the file is replaced atomically. So if an old worker finishes first, its result is just as good, and the job is done sooner.

### `(*Store) Fail(id int64, attempt int, msg string, now time.Time) error`

Records the error and logs a `failed` event (with `final` set on the last attempt). The job goes back to pending, or to `failed` once `attempt >= maxAttempts`. Requires the current attempt; otherwise `ErrLeaseLost`.

### `(*Store) Release(id int64, attempt int, now time.Time) error`

Logs a `released` event and puts a running job back to pending **and undoes the attempt count**. Workers call this when shutting down cleanly (Ctrl+C, `docker compose down`), so stopping the cluster doesn't push jobs toward `failed`. Requires the current attempt.

`Fail` and `Release` take `now` so their events have a time. The coordinator passes its clock, and tests pass a fixed time.

### `leaseChange(id, attempt, e, query, args...)` (unexported)

The shared body of `Fail` and `Release`: in one transaction, read the job's current worker (the update is about to clear it), run the update, check the caller still held the lease, and log the event.

### `leaseUpdate(res, err)` (unexported)

Turns "the UPDATE matched 0 rows" into `ErrLeaseLost`. Shared by `Heartbeat`, `Fail`, and `Release`.

### `(*Store) Get(id int64) (Job, error)`

Reads one job. Used by tests and for debugging.

## events.go

### `EventKind`, `Event`

The kinds and one row of the log, as above. `Event` also carries the job's file `Path` (joined from `jobs`), so a replay can label files without a second lookup.

### `logEvent(tx, e)` (unexported)

Inserts one event inside the caller's transaction.

### `(*Store) Events(after int64) ([]Event, error)`

Every event with an id above `after`, oldest first. `Events(0)` is the whole history. A live page passes the last id it has, so each update only carries what's new.

## summary.go

### `Summary`

Snapshot for the status page: counts per state, totals of pages/links/edges from finished jobs, and three lists: `Active` (running jobs, their worker, attempt, and lease deadline), `Failures` (failed jobs and why), and `Recent` (the latest finished jobs).

### `ActiveJob`, `FailedJob`, `FinishedJob`

The rows in those lists. `FinishedJob` also has `Worker` and `Attempt`, so the dashboard can draw each file on its worker's row of the timeline and spot retries. Its start time is `FinishedAt − DurationMs`, so no separate start column is needed.

### `(Summary) Finished() bool`

True when nothing is pending or running. Workers exit when the coordinator reports this.

### `(*Store) Summary(recent int) (Summary, error)`

Builds the snapshot: one `GROUP BY state` count, one `SUM` over finished jobs, then the three lists (`recent` limits the finished list). Lists are never `nil`, so they encode as `[]` in JSON instead of `null`, which keeps the future dashboard code simpler.

### `active()`, `failures()`, `recent(limit)` (unexported)

The queries behind the three lists. `recent` is `finished("DESC", limit)`, newest first.

### `(*Store) Finished() ([]FinishedJob, error)`

Every done job, in the order they finished. This feeds the coordinator's `/timeline` route, which the dashboard uses for its timeline and its pages-over-time chart.

### `finished(order string, limit int)` (unexported)

The shared query for `recent` and `Finished`. `order` is only ever the constant `"ASC"` or `"DESC"` from this file, never user input, so building it into the SQL is safe. A `limit` of `-1` means no limit in SQLite.

### `WorkerTotals`, `(*Store) ByWorker() (map[string]WorkerTotals, error)`

Finished jobs and pages per worker, from one `GROUP BY worker` over done jobs. It reads from the database, so the counts survive a coordinator restart.
