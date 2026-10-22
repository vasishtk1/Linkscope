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

## Fault tolerance (stage 2–3)

Workers can crash at any point. The design goals:

- A job held by a crashed worker is eventually given to another worker
- A job that runs twice produces the same output, written once, so nothing is double-counted
- A run with random worker crashes produces exactly the same final output as a clean run
