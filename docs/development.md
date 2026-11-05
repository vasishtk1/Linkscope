# Development

## Requirements

- Go 1.26+
- Docker (with Docker Compose)

## Commands

| Command | What it does |
|---|---|
| `make build` | Builds every binary in `cmd/` into `bin/` |
| `make test` | Runs all tests with the race detector on |
| `make lint` | Runs `go vet` and fails if any file isn't `gofmt`-formatted |
| `make fmt` | Formats all Go files |
| `make up` | Builds the Docker image and starts 1 coordinator + 4 workers (`FILES=50 WORKERS=6 make up` to change) |
| `make down` | Stops the Docker cluster |
| `make clean` | Deletes `bin/` |
| `make sample-edges` | Extracts `data/sample.warc.wat.gz` into `data/sample.edges.tsv` for the website |
| `make sample-scores` | Runs PageRank on the sample edges into `data/sample.scores.tsv` for the website |
| `make web` | Builds both sample files, installs the website's packages, and runs it on localhost:3000 |
| `make merge` | Merges every result file in `RESULTS` (default `data/results`) into `data/merged.edges.tsv.gz` (`RESULTS=data/cluster/results make merge` for the Docker cluster) |
| `make pagerank` | Scores every domain in `data/merged.edges.tsv.gz` and writes `data/scores.tsv` |
| `make cc-ranks` | Saves Common Crawl's top 1,000,000 domains (`CC_TOP`) from their web graph (`CC_GRAPH`, default `cc-main-2026-aug-sep-oct`) to `data/cc-domain-ranks.tsv` |
| `make validate` | Compares `data/scores.tsv` with `data/cc-domain-ranks.tsv` and prints how closely they agree |
| `make sample` | Downloads the first WAT file of the crawl in `CRAWL` (default `CC-MAIN-2026-43`) to `data/sample.warc.wat.gz` |

## Running a cluster without Docker

```sh
make build
./bin/coordinator -limit 20 &                       # queue 20 files
for i in 1 2 3 4; do ./bin/worker -id w$i -parsers 2 & done
curl -s localhost:8080/status                       # watch progress
```

Workers exit on their own once every job is done or failed. Stop the coordinator with Ctrl+C (or `kill`).

## Running a cluster in Docker

```sh
make up                       # 1 coordinator + 4 workers, 20 files
FILES=50 WORKERS=6 make up    # more files, more workers
make down
```

Everything is stored under `data/cluster/`: the job database (`jobs.db`) and the results (`results/*.edges.tsv.gz`). The job database persists between runs, so running `make up` again only processes jobs that aren't done yet. Delete `data/cluster/` to start over.

The build context excludes `data/`, `bin/`, and docs (`.dockerignore`), so building the image doesn't upload downloaded crawl files to Docker.

## CI

GitHub Actions runs lint, test, and build on every push to `main` and every pull request (`.github/workflows/ci.yml`).

## Local data

Downloaded Common Crawl files and pipeline output go in `data/`, which is ignored by git.
