# internal/loadtest

Measures how fast the lookup API answers when many lookups arrive at once (stage 5). `cmd/loadtest` runs it against a running `cmd/api` and prints the results.

**Why:** "fast" means nothing without a number. A load test keeps a fixed number of lookups in flight for a set time and records how long each one took. Three numbers come out of that:

- **Throughput:** lookups answered per second.
- **p50 (median):** half of the lookups were faster than this. The typical experience.
- **p99:** 99% were faster than this. The slowest 1% of users feel this one, so it's the number that matters most for "is it fast for everyone."

## loadtest.go

### `Options`

| Field | Meaning |
|---|---|
| `URL` | API base address, e.g. `http://localhost:8081` |
| `Domains` | Domains to pick from, at random |
| `Concurrency` | Lookups in flight at once (one goroutine each) |
| `Duration` | How long to run |
| `Limit` | `limit` sent with each lookup (the website uses 1,000) |
| `Seed` | Random seed, so the same options pick the same domains in the same order |

### `Result`

`Requests`, `Errors`, `Bytes` received, `Elapsed`, and every request's latency, sorted.

### `Run(ctx context.Context, client *http.Client, o Options) (Result, error)`

Starts `Concurrency` goroutines. Each one loops until time is up: pick a random domain, look it up, read the whole answer, record how long it took. Each goroutine has its own random generator and its own results slice, so they never share memory and need no locks. The slices are combined and sorted at the end.

A request that's still running when time runs out is dropped instead of being recorded as an error, since it was cut off by the test, not by the server.

### `lookup(ctx, client, base, domain string, limit int) (int64, error)`

One `GET /lookup`. It reads the whole body, because a real client has to receive all of it, so big answers count fully. Anything other than `200` is an error.

### `(Result) Percentile(p float64) time.Duration`

Nearest-rank percentile: sort all latencies, and p99 is the one 99% of the way through the list. `Percentile(100)` is the slowest request.

### `(Result) PerSecond() float64`

Requests divided by elapsed seconds.

## Tests

| Test | Checks |
|---|---|
| `TestPercentile` | p0, p50, p99, p100 on 1–100 ms; empty result gives 0 |
| `TestRunCountsRequestsAndErrors` | Against a fake server: every domain gets picked, `404`s count as errors, `limit` is sent, latencies are sorted |
| `TestRunRejectsBadOptions` | No domains is an error |

## Results

20-file index (588,294 domains), random lookups over the 412,419 domains with backlinks, `limit=1000` like the website. Measured 2026-11-05 on an Apple M1 laptop (8 cores, 16 GB), with the load tester and the API on the same machine and other apps open. Each row is the middle of 3 runs of 10 seconds.

| Lookups at once | Lookups/s | p50 | p99 | Errors |
|---|---|---|---|---|
| 1 | 11,025 | 0.08 ms | 0.24 ms | 0 |
| 8 | 28,829 | 0.21 ms | 1.13 ms | 0 |
| 32 | 38,511 | 0.78 ms | 2.26 ms | 0 |

Average answer size was 0.6 KB, because most sites have only a few backlinks.

**What this shows:** a single lookup takes well under a millisecond. Going from 8 to 32 at once adds about a third more throughput, while each lookup waits longer, because all 8 cores are busy and requests start to queue. The load tester itself uses a large share of the CPU, so on a separate machine the API would do better than this.

**Big sites are slower:** with only the 100 most-linked sites (average answer 147 KB), one run at 32 at once gave 3,140 lookups/s with p50 5.06 ms and p99 70.59 ms. That's a single run, so it's a rough number. Those answers are mostly JSON to build and send; a cache for popular sites would be the fix if it matters.

**It found a bug:** the first run had 0.02% errors. They were 95 sites whose registered domain starts with `www.` (`www.gov.uk`, `www.nhs.uk`), which the input cleanup had turned into `gov.uk`. See `Normalize` in [index.md](index.md).
