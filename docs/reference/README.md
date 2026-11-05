# Reference

Every package and function, documented here instead of in code comments. Each entry covers what it does, its inputs and outputs, and why it's built that way.

| Package | What it does |
|---|---|
| [internal/warc](warc.md) | Reads raw records out of a WARC/WAT file |
| [internal/wat](wat.md) | Parses a WAT record's JSON into a page with its links |
| [internal/links](links.md) | Turns URLs into domains, picks which links count, and builds the link graph |
| [internal/extract](extract.md) | Runs a whole WAT file through a pool of parser goroutines |
| [internal/jobs](jobs.md) | The coordinator's job list in SQLite: claims, leases, retries |
| [internal/coordinator, internal/api](coordinator.md) | The coordinator's HTTP API and the request/response types |
| [internal/worker](worker.md) | The worker loop: claim, download, extract, save, report, heartbeat |
| [internal/merge](merge.md) | Combines every worker's result file into one link graph |
| [internal/pagerank](pagerank.md) | Scores every domain from the merged graph with PageRank |
| [internal/validate](validate.md) | Compares our ranking with Common Crawl's published one |
| [internal/index](index.md) | Sharded backlink index: build once, look up any site with one disk read |
| [internal/serve](serve.md) | The lookup API's HTTP routes |
| [internal/fixture](#internalfixture) | Builds fake Common Crawl files for tests |
| [web/](web.md) | The website: pages, API routes, animated graph |
| [cmd/](cmd.md) | The runnable programs: `extract`, `coordinator`, `worker`, `merge`, `pagerank`, `validate`, `index`, `api` |

## How the packages connect

**Inside one file (stage 1):**

```
.wat.gz file → gzip → warc.Reader.Next() → wat.ParsePage() → links.Extract() → links.Graph.Add() → Graph.Edges()
                      └──────────────────── all orchestrated by extract.Run() ───────────────────┘
```

**Across many files (stage 2):**

```
cmd/coordinator ── coordinator.Server ── jobs.Store (SQLite)
       ▲
       │  HTTP: claim / heartbeat / complete / fail / release   (types in internal/api)
       ▼
cmd/worker ── worker.Worker ── fetch from Common Crawl ── extract.Run() ── result .edges.tsv.gz
```

**Combining the results (stage 4):**

```
results/*.edges.tsv.gz → merge.ResultFiles() → merge.Files() → links.ReadTSV() → Graph.AddEdge() → merge.WriteFile()
                                                                                                    merged.edges.tsv.gz
merged.edges.tsv.gz → pagerank.Compute() → pagerank.Scores() → pagerank.WriteTSV() → scores.tsv
scores.tsv + cc-domain-ranks.tsv → pagerank.ReadTSV() + validate.ReadRanks() → validate.Compare() → agreement report
```

**Serving lookups (stage 5):**

```
merged.edges.tsv.gz + scores.tsv → index.Build() → data/index/ (manifest.json + shard-000.idx … shard-007.idx)
website → GET /lookup (cmd/api, serve.Handler) → index.Lookup():
"github.com" → index.Normalize() → Resolve() → Find(): ShardOf() picks the shard, binary search its directory → Backlinks(): one ReadAt
```

## internal/fixture

Test-only helpers for building WAT files in memory, so tests never need network access or large downloads.

### `Page(uri, status string, links ...string) string`

One WAT metadata record for an HTTP response at `uri` with the given status and `<a href>` links.

### `Record(typ, uri, body string) string`

One raw WARC record with the correct `Content-Length`.

### `Gzip(records ...string) []byte`

Compresses each record as its own gzip member, back to back, the way Common Crawl files are built. Testing with that layout ensures the reader handles multi-member gzip streams.
