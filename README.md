# Linkscope

Distributed backlink index and domain authority engine built on Common Crawl data.

Type in a domain and see who links to it, an authority score, and an interactive graph of its connections. Linkscope processes Common Crawl's public web archive across a pool of workers, extracts every link between websites, and scores domains with PageRank.

> **Status:** stage 1 of 7 done (single-file link extraction). See [docs/roadmap.md](docs/roadmap.md).

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

## Repo structure

```
Linkscope/
├── .github/workflows/ci.yml   # lint, test, build on every push and PR
├── cmd/
│   ├── coordinator/main.go    # coordinator binary (assigns jobs to workers; stage 2)
│   ├── extract/main.go        # runs the extractor on one WAT file and prints a report
│   └── worker/main.go         # worker binary (processes Common Crawl files; stage 2)
├── internal/
│   ├── extract/               # runs a WAT file through parallel parser goroutines
│   ├── links/                 # URL → domain, which links count, domain link graph
│   ├── warc/                  # reads records out of WARC/WAT files
│   └── wat/                   # parses WAT JSON into pages and links
├── docs/
│   ├── README.md              # docs index
│   ├── architecture.md        # how the system fits together
│   ├── design-decisions.md    # why things are built the way they are
│   ├── development.md         # local setup and workflow
│   ├── roadmap.md             # build stages and progress
│   └── reference/             # function-by-function reference, one file per package
├── Dockerfile                 # builds all binaries into one image
├── docker-compose.yml         # local cluster: 1 coordinator + 3 workers
├── Makefile                   # build, test, lint, sample download, run shortcuts
├── go.mod
└── go.sum
```
