# internal/coordinator and internal/api

The coordinator is an HTTP server in front of the job list (`internal/jobs`). Workers talk to it with small JSON requests. `internal/api` holds the request and response types both sides share.

## HTTP API

| Method & path | Body | Response |
|---|---|---|
| `POST /jobs/claim` | `{"worker"}` | `200 {"job": {...}}` with a job, `200 {"finished": false}` if nothing is free right now, `200 {"finished": true}` if all work is done |
| `POST /jobs/{id}/heartbeat` | `{"worker", "attempt"}` | `204`, or `409` if the lease was lost |
| `POST /jobs/{id}/complete` | `{"worker", "attempt", "result"}` | `204` |
| `POST /jobs/{id}/fail` | `{"worker", "attempt", "error"}` | `204`, or `409` if the lease was lost |
| `POST /jobs/{id}/release` | `{"worker", "attempt"}` | `204`, or `409` if the lease was lost |
| `GET /status` | | `200` with counts, totals, rates, every worker's state, and the active, failed, and recent job lists |
| `GET /timeline` | | `200 {"jobs": [...]}`: every finished job in finish order, with its worker, attempt, pages, edges, duration and finish time |

Bad JSON, a missing worker name, or a bad job id get `400`. Database errors get `500`, and the details are logged on the coordinator only.

**Why "nothing free right now" and "all done" are separate:** when every remaining job is running on someone else, an idle worker must not exit. One of those jobs might be on a worker that crashes, and then it needs someone to pick it up. So workers keep polling until the coordinator says `finished: true`.

## internal/api types

| Type | Used for |
|---|---|
| `ClaimRequest`, `ClaimResponse` | Claim |
| `Job` | A handed-out job: `ID`, `Path`, `Attempt`, `LeaseSeconds` (so workers know how often to heartbeat) |
| `LeaseRequest` | Heartbeat and release |
| `CompleteRequest` | Complete, carrying a `jobs.Result` |
| `FailRequest` | Fail, carrying the error message |
| `Status` | `jobs.Summary` plus `Total`, `UptimeSeconds`, `JobsPerMinute`, `PagesPerSecond`, `Workers`, and `Now`, the coordinator's clock. The dashboard measures job times against `Now` instead of the browser's clock, which could be set differently on another machine |
| `Worker` | One worker: `Name`, `State`, its current `JobID`, `Path` and `JobSeconds` while working, `LastSeenSeconds`, and `Done` / `Pages` totals |
| `Timeline` | `Jobs`, the finished jobs for `/timeline` |

## server.go

### `Server`

Holds the store, the lease length, a clock (`now`), a logger, and the throughput baseline (`started`, `firstDone`, `firstPages`), which is protected by a mutex because HTTP handlers run on many goroutines at once.

Also holds `seen`, what it remembers about each worker (below), under the same mutex.

### `seenWorker`

The last time a worker sent any request, the job it's on (id, path, and when it got it), and whether the coordinator has told it all work is finished. This lives in memory only. Workers already talk to the coordinator all the time (claims, heartbeats, completions), so tracking them costs no extra requests and needs no database writes.

### `New(store *jobs.Store, lease time.Duration) *Server`

Creates a server using the real clock and the standard logger. Tests replace `now` and `logf`.

### `(*Server) Handler() http.Handler`

Builds the routes with Go's standard router. Patterns like `POST /jobs/{id}/heartbeat` match the method and capture `{id}`, so wrong methods automatically get `405`.

### `(*Server) saw(name string, update func(*seenWorker))`

Called by every worker request: records the time, then lets the handler say what changed. `claim` sets the new job (or marks the worker idle, and finished if all work is done), and `complete`, `fail` and `release` mark it idle. `heartbeat` only updates the time.

### `idle(w *seenWorker)`

Clears the worker's current job.

### `(*Server) workers(now) ([]api.Worker, error)`

Builds the `Workers` list for `/status`. A worker's state is decided in this order:

| State | When |
|---|---|
| `finished` | It was told all work is done, so it has exited |
| `gone` | No request for longer than a lease (60s by default). Its job will be expired and handed to someone else, so it has probably crashed |
| `working` | It holds a job; `JobSeconds` is how long it's had it |
| `idle` | Alive, but there's nothing free to hand it right now |
| `offline` | It finished jobs in the database but hasn't talked to this coordinator since it started (for example after a coordinator restart). `LastSeenSeconds` is `-1` |

`Done` and `Pages` come from `store.ByWorker`, so they're correct even for workers this coordinator never saw. The list is sorted by name so the dashboard's cards don't jump around.

### `timeline` handler

Returns `store.Finished()`. It's a separate route from `/status` because the list grows with the run, while `/status` stays small.

### `(*Server) RunReaper(ctx, every)`

Calls `store.Expire` on a timer until `ctx` is cancelled, and logs whenever leases expire. `Claim` already expires leases, but without this the status page would show dead workers as "running" until the next claim.

### `(*Server) MarkStarted() error`

Records the current time and current done and page counts as the baseline for throughput. Rates only count work finished since this coordinator process started. Otherwise, restarting the coordinator on a database with 1,000 finished jobs would report an impossible jobs-per-minute.

### Handlers: `claim`, `heartbeat`, `complete`, `fail`, `release`, `status`, `timeline`

Each one decodes the body, calls the matching `Store` method, and writes the response from the table above. `claim`, `complete`, `fail`, and `release` log one line each, which gives a readable timeline of the run in the coordinator's output.

### `leaseResult(w, err)`

Maps `nil` to `204`, `ErrLeaseLost` to `409`, and anything else to `500`.

### `internal(w, err)`

Logs the real error and sends a generic `500`, so database details aren't exposed to clients.

### `jobID(w, r)`

Parses `{id}` from the path and rejects non-numbers and ids ≤ 0 with `400`.

### `decode(w, r, v)`

Decodes a JSON body, capped at 1 MB so a bad client can't make the coordinator read unlimited data.

### `requireWorker(w, worker)`

Rejects claims without a worker name, since every lease needs an owner to show on the status page.

### `writeJSON(w, code, v)`

Sets the content type and writes `v` as JSON.

## paths.go

### `CrawlPathsURL(source, crawl string) string`

Builds the URL of a crawl's file list, e.g. `https://data.commoncrawl.org/crawl-data/CC-MAIN-2026-43/wat.paths.gz`.

### `LoadPaths(ctx, hc, location string) ([]string, error)`

Reads a list of WAT paths from a URL or a local file, decompressing it if the name ends in `.gz`. A local file lets you run the coordinator offline or hand-pick files.

### `readPaths(r io.Reader, gzipped bool) ([]string, error)`

Reads one path per line and skips blank lines.

### `Window(paths []string, offset, limit int) []string`

Picks which slice of the list to queue: `-offset 100 -limit 20` queues files 100–119. `limit <= 0` means everything after the offset. Out-of-range values are clamped instead of crashing.
