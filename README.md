# Linkscope

Distributed backlink index and domain authority engine built on Common Crawl data.

Type in a domain and see who links to it, an authority score, and an interactive graph of its connections. Linkscope processes Common Crawl's public web archive across a pool of workers, extracts every link between websites, and scores domains with PageRank.

> **Status:** stage 2 of 7 done (distributed workers with crash and freeze recovery). See [docs/roadmap.md](docs/roadmap.md).

## How it works

1. **Coordinator** hands out Common Crawl files to workers as jobs
2. **Workers** download a file, extract domain-to-domain links, and save results per job
3. **Merge** combines worker output into one link graph
4. **PageRank** scores every domain
5. **Index + API** serves fast backlink lookups from sharded storage
6. **Web app** shows scores, link graphs, and a live pipeline dashboard

Details in [docs/architecture.md](docs/architecture.md).

## Getting started

Requires Go 1.26+ and Docker.

```sh
make build   # build binaries into bin/
make test    # run tests
make lint    # go vet + gofmt check
make up      # run coordinator + 3 workers in Docker
```

Try the extractor on a real Common Crawl file (~150 MB download):

```sh
make sample build
./bin/extract data/sample.warc.wat.gz
```

On one file from the October 2026 crawl: 22,020 pages → 257,059 links to other sites → 130,138 unique domain-to-domain edges, in about 3 seconds on 8 cores.

Run the website on localhost:3000 (needs Node 20+ and the sample file above). Until scores exist it shows real backlinks from the sample file:

```sh
make web
```

Run a local cluster (1 coordinator + 4 workers) on 20 files:

```sh
make up                       # or: FILES=50 WORKERS=6 make up
curl -s localhost:8080/status # progress
make down
```

| Run (20 files, Oct 2026 crawl, home internet) | Time | Pages |
|---|---|---|
| 1 worker | 206s | 441,193 |
| 4 workers | 103s | 441,193 |

Both runs produce byte-identical results. At 4 workers the download (~29 MB/s) is the bottleneck, not the code; see [docs/architecture.md](docs/architecture.md).

## Repo structure

```
Linkscope/
├── .github/workflows/ci.yml   # lint, test, build on every push and PR
├── cmd/
│   ├── coordinator/main.go    # queues files as jobs and serves the coordinator API
│   ├── extract/main.go        # runs the extractor on one WAT file and prints a report
│   ├── merge/main.go          # combines all worker results into one link graph
│   ├── pagerank/main.go       # scores every domain from the merged graph
│   └── worker/main.go         # claims jobs, processes files, reports back
├── internal/
│   ├── api/                   # request/response types shared by coordinator and worker
│   ├── coordinator/           # coordinator HTTP server, lease reaper, file list loading
│   ├── extract/               # runs a WAT file through parallel parser goroutines
│   ├── fixture/               # builds fake Common Crawl files for tests
│   ├── jobs/                  # SQLite job list: claims, leases, retries, status
│   ├── links/                 # URL → domain, which links count, domain link graph
│   ├── merge/                 # combines worker result files into one link graph
│   ├── pagerank/              # PageRank over the domain graph, 0–100 scores
│   ├── warc/                  # reads records out of WARC/WAT files
│   ├── wat/                   # parses WAT JSON into pages and links
│   └── worker/                # worker loop: claim, download, extract, save, heartbeat
├── docs/
│   ├── README.md              # docs index
│   ├── architecture.md        # how the system fits together
│   ├── design-decisions.md    # why things are built the way they are
│   ├── development.md         # local setup and workflow
│   ├── roadmap.md             # build stages and progress
│   └── reference/             # function-by-function reference, one file per package
├── web/                       # Next.js website
│   ├── app/                   # pages and API routes (lookup, CSV download, favicons)
│   ├── components/            # search bar, animated link graph, stats, backlinks table
│   └── lib/                   # loads backlink data, shared types, badge colors
├── .dockerignore              # keeps data/ and docs out of the Docker build
├── Dockerfile                 # builds all binaries into one small image
├── docker-compose.yml         # local cluster: 1 coordinator + N workers
├── Makefile                   # build, test, lint, sample download, run shortcuts
├── go.mod
└── go.sum
```
