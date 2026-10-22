# internal/extract

Runs one whole WAT file through the pipeline and returns its link graph. The Stage 2 workers will call this for every file they're assigned.

## Types

### `Stats`

Counters from one run.

| Field | Meaning |
|---|---|
| `Records` | All WARC records read |
| `Responses` | Records that were HTTP responses (crawled pages) |
| `PagesUsed` | Responses that passed `links.Extract` (2xx with a valid domain) |
| `CrossLinks` | Links to other domains, before per-page de-duplication |
| `ParseErrors` | Records whose JSON couldn't be parsed |

### `outcome`, `result` (unexported)

What happened to one record (`parseError`, `notResponse`, `skipped`, `used`), plus its `Outlinks` if used. Parser goroutines send these back to the aggregator.

## Functions

### `Run(r io.Reader, workers int) (*links.Graph, Stats, error)`

Processes a gzipped WAT stream using `workers` parser goroutines.

```
               records channel            results channel
 reader  ──▶  [ buffered, 256 ]  ──▶  N parsers  ──▶  [ buffered, 256 ]  ──▶  aggregator
 (1 goroutine:                         (JSON parse +                          (this goroutine:
  gunzip + split records)               link extraction)                       Graph.Add + stats)
```

1. **Reader goroutine:** decompresses and splits the stream into records, and sends `metadata` records into the `records` channel. It closes the channel when done.
2. **Parser goroutines (N):** each takes records from the channel, runs `wat.ParsePage` and `links.Extract`, and sends a `result`.
3. When every parser has finished, a small goroutine closes `results`.
4. **Aggregator (the calling goroutine):** adds each result to the graph and stats.

**Why this shape:**
- JSON parsing took about 90% of the time on one core (gunzip alone is 0.75s of 7.6s), and it's independent per record, so it's the part worth parallelizing.
- Only the aggregator touches the `Graph`, so the graph needs no locks. Go's guideline here: "share memory by communicating."
- The channels are buffered, so a brief slowdown in one stage doesn't immediately stall the others.
- The result doesn't depend on the worker count or timing, because `Graph.Add` is order-independent. A test checks this with 1, 2, 4, and 16 workers.

**Errors:** an invalid gzip header or a corrupt or truncated WARC record stops the run and returns an error. A record with invalid JSON is only counted in `ParseErrors`, since one bad record shouldn't throw away a whole file.

**Measured on one real file** (Oct 2026 crawl, 149 MB compressed, 22,022 pages, Apple Silicon, 8 cores):

| Workers | Time | Pages/s |
|---|---|---|
| 1 | 7.60s | 2,897 |
| 4 | 2.99s | 7,353 |
| 8 | 3.06s | 7,208 |

Speed stops improving at about 4 workers because the single reader goroutine (decompression and record splitting) becomes the bottleneck. More throughput comes from processing many files at once (Stage 2), not more parsers per file.

### `parse(rec *warc.Record) result`

Runs one record through `wat.ParsePage` and `links.Extract` and labels what happened. It's kept separate from `Run` so the per-record logic is easy to read and test.
