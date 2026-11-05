# internal/worker

A worker asks the coordinator for a job, downloads and processes that file with the Stage 1 extractor, saves the result, reports back, and repeats until all work is done.

## The loop

```
 ┌──────────────────────────────────────────────────────────────────────┐
 │ claim ──▶ got a job? ──yes──▶ process ──────────────────────────────┐ │
 │   ▲          │                  │                                   │ │
 │   │          no                 ├── download (retries, time limits) │ │
 │   │          │                  ├── extract (Stage 1 code)          │ │
 │   │          ├── all finished? ──▶ exit                             │ │
 │   │          └── wait, poll again                                   │ │
 │   │                             ├── write result (temp + rename)    │ │
 │   │                             └── report complete / fail          │ │
 │   │                                                                 │ │
 │   │         meanwhile: heartbeat every lease/3 ─▶ lost? cancel job  │ │
 │   └─────────────────────────────────────────────────────────────────┘ │
 └──────────────────────────────────────────────────────────────────────┘
```

## worker.go

### `Config`

| Field | Meaning | Default |
|---|---|---|
| `ID` | Name shown on the coordinator | (set by `cmd/worker`: hostname-pid) |
| `Coordinator` | Coordinator URL | |
| `Source` | Common Crawl base URL | |
| `OutDir` | Where result files go | |
| `Parsers` | Parser goroutines per file (see `extract.Run`) | 1 |
| `PollInterval` | Wait between claims when no job is free | 2s |
| `FetchRetries` | Download retries before failing the job | 0 |
| `RetryBackoff` | First retry delay for network errors and other `5xx`; doubles each time | 1s |
| `BusyBackoff` | First wait after Common Crawl says it's busy (`429` or `503`); doubles up to 2 minutes, with jitter | 15s |
| `RequestTimeout` | Time limit for each call to the coordinator | 20s |
| `FirstByteTimeout` | How long Common Crawl can take to start sending a file | 30s |
| `StallTimeout` | Cancel a download when no data arrives for this long | 20s |
| `HTTP` | HTTP client | default client |
| `Logf` | Logger | `log.Printf` |

### `New(cfg Config) *Worker`

Fills in defaults and creates the coordinator client.

### `(*Worker) Run(ctx) error`

The main loop. Creates the output folder, then repeatedly claims:
- **Got a job:** process it.
- **Nothing free, not finished:** wait `PollInterval` and ask again (another worker's job might come back if that worker dies).
- **Finished:** return.
- **Coordinator unreachable or erroring:** wait and retry with exponential backoff, capped at 30s. Workers can start before the coordinator, or keep going through a coordinator restart.

Returns `nil` when `ctx` is cancelled (Ctrl+C), since a requested shutdown isn't an error.

### `(*Worker) process(ctx, j api.Job)`

Runs one job and reports the outcome:
1. Creates a job context and starts the heartbeat goroutine
2. Runs the job and times it
3. Stops the heartbeat and waits for it to exit
4. Reports, choosing by what happened:

| Situation | Action |
|---|---|
| Success | `Complete`, even if the lease was lost meanwhile, since the result is valid either way |
| Lease lost mid-job | Do nothing. The job already belongs to someone else |
| Shutting down | `Release`, so the job goes straight back to the queue without counting an attempt |
| Error | `Fail` with the message |

Reports use a context that ignores cancellation (`context.WithoutCancel`). Otherwise, the Ctrl+C that triggered a shutdown would also cancel the release message telling the coordinator about it. The client's 20s limit still applies, so a frozen coordinator can't hold up shutdown forever.

If `Complete` can't reach the coordinator, the worker just logs it. The lease expires and another worker redoes the job, which is safe because results are idempotent.

### `(*Worker) heartbeat(ctx, j, lost, cancel)`

Renews the lease every third of its length (every 20s for a 60s lease), so two heartbeats can be missed before the lease runs out. On `ErrLeaseLost` it sets `lost` and **cancels the job**, which aborts the download mid-stream. A worker that's lost its job stops spending bandwidth and CPU on it. Other errors, such as the coordinator briefly being down or not answering within the request time limit, are logged and retried on the next tick.

### `(*Worker) runJob(ctx, j) (jobs.Result, error)`

Runs `attempt` and starts over, with exponential backoff, if the download broke. Up to `FetchRetries` retries happen inside the job, without using up one of the coordinator's attempts. Only errors marked as `downloadError` are retried. A corrupt file or a `404` fails right away, since downloading it again gives the same result.

There are two kinds of wait:

| Failure | Waits | Why |
|---|---|---|
| Network error, timeout, stall, `500`, `502`, `504` | 1s, 2s, 4s, 8s… | Usually a blip that's gone in seconds |
| Common Crawl says it's busy (`429`, `503`) | ~15s, ~30s, ~60s, then ~2 min each | The server is overloaded, so retrying quickly just adds to the pile |

The busy waits are **jittered**: each one is a random amount between half and all of the planned wait. Without that, several workers turned away at the same moment would all come back at the same moment and get turned away again.

**Why this was needed:** in a 20-file run with 4 workers, Common Crawl answered `503 Slow Down` so often that 11 of the 20 files failed. The old waits (1s, 2s, 4s) gave up after about 7 seconds in total. With the cluster's default of 6 retries, a busy file now keeps trying for roughly 4–8 minutes. Heartbeats keep running during the waits, so the job's lease doesn't expire.

Starting over means downloading the whole file again. Resuming halfway isn't possible, because the extractor has already consumed the first half. At ~150 MB per file that costs a few seconds.

### `(*Worker) attempt(ctx, url, watPath) (jobs.Result, error)`

One try: download, then extract, then write the result, returning the counts. The download is **streamed straight into the extractor**, so nothing is saved to disk except the final result.

### `downloadError`

Wraps an error to mark it worth retrying: network errors, timeouts, stalls, `429 Too Many Requests` and `5xx`. Its `busy` field is true for `429` and `503`, the codes a server uses to say "slow down", so `runJob` knows to use the longer waits. It has `Unwrap`, so `errors.As` can find it even after the extractor has wrapped it in its own message.

### `jitter(d time.Duration) time.Duration`

Returns a random wait between `d/2` and `d`. Used for busy waits so workers spread out their retries.

### `(*Worker) open(ctx, url) (io.ReadCloser, error)`

Starts the download and returns a `watchdog` wrapped around the response body.

1. Creates a context just for this download and starts a timer for `FirstByteTimeout` (30s). If Common Crawl hasn't sent a response by then, the timer cancels the request.
2. On a response, it stops the timer. `200` returns the body. `429` and `5xx` return a `downloadError`. Other `4xx` codes, like `404`, return a plain error, because retrying won't fix a missing file.

### `watchdog`

Wraps the download body and cancels it if it goes silent.

- **`Read`** starts a `StallTimeout` (20s) timer, reads, then stops the timer. If the timer fires first, the download is cancelled, and `Read` returns `download stalled: no data for 20s` as a `downloadError`.
- **Why only during `Read`:** the timer runs only while the worker is waiting on the network. If the extractor is busy and not asking for more data, that's not the network's fault and doesn't count as a stall.
- **Why "no data for 20s" instead of a total limit:** a healthy file takes anywhere from ~8s to over 30s depending on the connection, so any total limit would either kill normal downloads or be too long to catch freezes. A download that's slow but still moving is fine; only one that's gone silent is cancelled.
- **`expire`** marks the timer as fired and cancels the download. **`stop`** stops the timer and releases the download's context. **`Close`** does both and closes the body.

### `ResultName(watPath string) string`

Maps a WAT path to its result file name: `.../CC-MAIN-...-00042.warc.wat.gz` becomes `CC-MAIN-...-00042.edges.tsv.gz`. Each job always writes to the same file, which is what makes re-running a job harmless.

### `writeResult(dst string, edges []links.Edge) error`

Writes the edges as gzipped TSV, safely:
1. Write to a temp file in the same folder, with a unique random suffix so two attempts of the same job never write into the same temp file
2. `Sync`: force the bytes onto the disk
3. `Rename` the temp file to the final name. On the same disk this is atomic: other programs see either the old file or the complete new one, never half a file
4. Sync the folder, so the rename itself survives a power cut

If anything fails, the deferred `os.Remove` deletes the temp file, so crashes don't leave junk behind. A test checks that the output folder holds exactly one file per job.

Gzip cuts each result from ~21 MB to a few MB. Go's gzip writer puts no timestamp in the header, so the same edges always produce byte-identical files, which Stage 3 relies on.

### `syncDir(dir string) error`

Opens a folder and syncs it, for step 4 above.

### `sleep(ctx, d) bool`

Waits `d`, or returns `false` early if `ctx` is cancelled. Used everywhere the worker waits, so Ctrl+C never has to wait out a backoff.

## client.go

### `Client`, `NewClient(base, hc, timeout)`

A small typed client for the coordinator API. Every request gets `timeout` (20s from `Config.RequestTimeout`).

### `Claim`, `Heartbeat`, `Complete`, `Fail`, `Release`

One method per endpoint, each sending the worker name and the job's attempt number.

### `jobPath(j, action)`

Builds `/jobs/{id}/{action}`.

### `(*Client) post(ctx, path, body, out) error`

Sends JSON and decodes the reply into `out` if given. The whole request, including reading the reply, must finish within the client's time limit. A coordinator that accepts the connection but never answers is the dangerous case: without a limit the worker would wait forever. With it, the worker gets `no answer within 20s` and handles it like any other coordinator error, by retrying with backoff. `409` becomes `jobs.ErrLeaseLost`, and any other non-2xx becomes an error that includes the status and the start of the response body.

## Tests (`worker_test.go`)

End-to-end tests with a real coordinator and job database, real workers, and a fake Common Crawl server that can be scripted to fail or hang:

| Test | What it proves |
|---|---|
| `TestClusterProcessesEveryFileExactlyOnce` | 4 workers, 12 files: every job done, every file downloaded once, every result byte-identical to running the extractor directly, no leftover temp files |
| `TestFlakyDownloadIsRetried` | Two `503`s, then success, and the job still completes |
| `TestBusyServerGetsLongerWaits` | Two `503`s (and separately two `429`s) use the long, jittered busy waits, and the job still completes |
| `TestOtherServerErrorsKeepShortWaits` | Two `500`s retry quickly, without the long busy wait |
| `TestCorruptFileIsMarkedFailedAndOthersFinish` | A broken file fails after its attempts run out, and the rest finish |
| `TestMissingFileFailsWithoutRetryingDownload` | `404` isn't retried |
| `TestJobFromCrashedWorkerIsRedone` | A worker claims a job and disappears; after the lease expires another worker does it |
| `TestWorkerAbandonsJobWhenLeaseIsLost` | Mid-download the job is taken away; the worker stops and writes nothing |
| `TestShutdownReleasesJobWithoutUsingAnAttempt` | Cancelling a worker mid-job puts the job back with its attempt undone |
| `TestWorkerRetriesWhileCoordinatorIsDown` | The coordinator refuses the first requests; the worker keeps trying and finishes |
| `TestWorkerRecoversFromFrozenCoordinator` | The coordinator accepts the first requests and never answers; the worker times out, retries, and finishes |
| `TestStalledDownloadIsRetried` | A download sends half the file and goes silent; it's cancelled and retried inside the same job attempt, and the result is still byte-identical |
| `TestDownloadThatNeverStartsIsRetried` | Common Crawl sends nothing, not even headers; the first-byte limit cancels it and the retry succeeds |
| `TestSlowButSteadyDownloadIsNotCancelled` | A download that trickles in small pieces, taking longer than the stall limit overall, still completes on the first try |
| `TestStatusEndpoint` | `/status` reports totals and recent jobs |
