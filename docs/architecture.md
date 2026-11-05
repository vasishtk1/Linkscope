# Architecture

> Planned design. Sections are filled in with real details as each stage is built.

## Data source

[Common Crawl](https://commoncrawl.org) publishes a free archive of billions of web pages, refreshed roughly monthly. Each crawl is split into tens of thousands of files. Linkscope uses the **WAT** files, which contain each page's metadata and outgoing links without the full HTML, so they're much smaller to download and parse.

Measured on the October 2026 crawl (`CC-MAIN-2026-43`):

| | |
|---|---|
| WAT files in the crawl | 100,000 |
| One WAT file | ~150 MB compressed, ~650 MB uncompressed |
| Records per file | ~66,000 (3 per crawled page: request, response, metadata) |
| Pages per file | ~22,000 |
| Clickable links per file | ~2.9 million, of which ~257,000 point to other domains |
| Unique domain → domain edges per file | ~130,000 |

## Stage 1: single-file pipeline

```
.wat.gz → gunzip → split records → parse JSON → keep <a> links to other domains → count per (src, dst) → sorted edges
```

Details in [reference/extract.md](reference/extract.md).

## Components

```
 Common Crawl          Coordinator            Workers (N)               Merge          PageRank        Index + API        Web app
 (WAT files)  ──────▶  job queue   ──jobs──▶  download, extract  ──▶   combine  ──▶   score    ──▶   sharded     ──▶   search, graph,
                       + leases    ◀─status─  domain links             output         domains         lookups           dashboard
```

| Component | Responsibility | Stage |
|---|---|---|
| Coordinator | Tracks which files are pending, in progress, or done; hands jobs to workers; reassigns jobs from workers that stop responding | 2 |
| Worker | Downloads one WAT file, extracts links between different domains, writes the results for that job (extraction built in stage 1) | 1–2 |
| Merge | Combines all job outputs into a single domain link graph | 4 |
| PageRank | Computes an authority score for every domain from the graph | 4 |
| Index + API | Stores backlinks split across shards; answers "who links to this domain?" quickly | 5 |
| Web app | URL search, authority score, interactive link graph, live pipeline dashboard | 6 |

## Stage 2: coordinator and workers

```
                         ┌────────────────────────────┐
                         │        Coordinator         │
                         │  HTTP API  +  SQLite jobs  │
                         │  lease reaper (every 1s)   │
                         └──────▲──────────────▲──────┘
             claim / heartbeat /│              │ complete / fail / release
                                │              │
          ┌─────────────────────┴───┐      ┌───┴─────────────────────┐
          │ Worker 1                │ ...  │ Worker N                │
          │  stream WAT from CC  ──▶│      │  stream WAT from CC  ──▶│
          │  extract (stage 1)   ──▶│      │  extract (stage 1)   ──▶│
          │  write result atomically│      │  write result atomically│
          └───────────┬─────────────┘      └────────────┬────────────┘
                      ▼                                 ▼
               results/<file>.edges.tsv.gz  (one per job, shared folder)
```

Each Common Crawl file is one job. A worker:

1. **Claims** a job and gets a 60s lease
2. **Streams** the file from Common Crawl straight into the extractor, without saving the download
3. **Heartbeats** every 20s to keep the lease. If the coordinator answers "lease lost," the worker cancels the download and moves on
4. **Writes** `<file>.edges.tsv.gz` via temp file, fsync, and atomic rename
5. **Reports** complete, fail, or (on shutdown) release, then claims the next job
6. **Exits** once the coordinator says nothing is left to do

Details: [reference/jobs.md](reference/jobs.md), [reference/coordinator.md](reference/coordinator.md), [reference/worker.md](reference/worker.md).

### Failure handling

| What goes wrong | What happens |
|---|---|
| Worker crashes or freezes | Its heartbeats stop, the lease expires, and the job returns to the queue for another worker |
| Frozen worker wakes up later | Its attempt number is out of date, so its heartbeats get `409`; it abandons the job |
| Two workers both finish the same job | Same output file, same bytes; the second completion is ignored |
| Crash while writing a result | Only a temp file is affected; the real file is never half-written |
| Common Crawl returns 503 / network blip | The download retries with backoff (1s, 2s, 4s) |
| Common Crawl never starts sending | Cancelled after 30s and retried |
| Download goes silent partway through | Cancelled after 20s with no data and retried from the start, inside the same job attempt |
| Coordinator freezes (connection open, no answer) | Every worker request has a 20s limit; a timeout is treated like the coordinator being down, and the worker retries |
| A worker freezes mid-request to the coordinator | The coordinator's HTTP server drops requests that take more than 20s to read or answer |
| File is missing (404) | Fails immediately, no pointless retries |
| File is corrupt | Fails each attempt; marked `failed` after 3 and reported on `/status` |
| Coordinator restarts | Jobs are in SQLite, so nothing is lost; workers keep retrying until it's back |
| Worker stopped with Ctrl+C | Hands its job back immediately without using up an attempt |

### Measured (20 files, Oct 2026 crawl, MacBook on home internet)

| | 1 worker | 4 workers |
|---|---|---|
| Wall time | 206s | 103s |
| Pages processed | 441,193 | 441,193 |
| Cross-domain links | 5,269,606 | 5,269,606 |
| Per-file edges (summed across files) | 2,528,210 | 2,528,210 |
| Result files | 20, 70 MB gzipped | 20, byte-identical to the 1-worker run |

With 4 workers, each job took 13–33s (avg 20s), versus ~8s when running alone. Four downloads at once reached ~29 MB/s total, about the same as a single download, so the home connection is the limit at this point. On cloud machines in the same region as Common Crawl's storage (AWS us-east-1), bandwidth per machine is much higher, so adding workers should keep helping. Stage 7 will measure this.

The edge totals above are summed per file. The same `a.com → b.com` pair can appear in several files; combining them into unique edges is the merge step in stage 4.

## Stage 4: merge

`cmd/merge` reads every result file and combines repeated `src → dst` pairs into one edge, adding up their page counts. The output, `merged.edges.tsv.gz`, is the full domain link graph that PageRank scores.

The merge follows the same order-independent rules as extraction, so the merged graph is the same as if every page had been processed in a single program, in any order. Details in [reference/merge.md](reference/merge.md).

`cmd/pagerank` then scores every domain. Each site splits its score among the sites it links to, in rounds, until the numbers settle. Each linking site counts as one vote, and nofollow-only links don't count. The raw ranks become a 0–100 score on a log scale, written to `scores.tsv`. Details in [reference/pagerank.md](reference/pagerank.md).

### Measured (one WAT file, Oct 2026 crawl)

| | |
|---|---|
| Domains | 77,793 |
| Edges | 130,138 |
| Merge time | 0.50s, ~86 MB peak memory |
| PageRank | 21 rounds, 0.06s |
| Top 5 | facebook.com, instagram.com, google.com, twitter.com, youtube.com |

`cmd/validate` then checks the ranking against Common Crawl's own published PageRank. On one file, rank agreement (Spearman) is 0.47 across all domains with a backlink and 0.76 for domains with 100+ linking sites, and our top 20 are all in Common Crawl's top 179. Details in [reference/validate.md](reference/validate.md).

## Stage 5: index and API

`cmd/index` turns the merged graph and the scores into a sharded index: 8 files, with each domain in the file its name hashes to. Inside each file, a sorted directory says where every domain's backlinks are stored. A lookup hashes the name to pick the file, binary-searches the directory, and reads that one list from disk. Details in [reference/index.md](reference/index.md).

`cmd/api` opens the index and answers `GET /lookup?domain=…` over HTTP. Only the directories are loaded into memory; backlink lists are read from disk per lookup. Routes in [reference/serve.md](reference/serve.md).

The website's routes now call this API instead of reading data files. Pages get the top 1,000 backlinks; the CSV download gets all of them.

### Measured

| | 1 file (sample) | 20 files |
|---|---|---|
| Edges | 130,138 | 1,199,002 |
| Domains | 77,793 | 588,294 |
| Domains with backlinks | 62,052 | 462,254 |
| Index size (8 shards) | 22.8 MB | 200.7 MB |
| Build time | 0.27s | 3.59s |
| Build peak memory | ~128 MB | ~904 MB |
| Domains per shard | 9,627 – 9,952 | 73,127 – 73,763 |
| API startup (open index) | 31 ms | 148 ms |
| API memory after startup | ~25 MB | ~192 MB |

### Load test (20 files, Apple M1 laptop, middle of 3 runs)

`cmd/loadtest` looked up random domains from the 412,419 with backlinks, with `limit=1000` like the website. Details in [reference/loadtest.md](reference/loadtest.md).

| Lookups at once | Lookups/s | p50 | p99 | Errors |
|---|---|---|---|---|
| 1 | 11,025 | 0.08 ms | 0.24 ms | 0 |
| 8 | 28,829 | 0.21 ms | 1.13 ms | 0 |
| 32 | 38,511 | 0.78 ms | 2.26 ms | 0 |

Every domain with backlinks was looked up in the index and compared row by row with the edge file, and all of them matched.

## Fault tolerance tests (stage 3)

Stage 2 covers each failure above with an automated test (see the table in [reference/worker.md](reference/worker.md)). Stage 3 goes further: randomly killing workers during real runs and proving the final output matches a clean run byte for byte.
