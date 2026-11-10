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
| `make sample-index` | Builds the sample index into `data/sample-index/` from the sample edges and scores |
| `make web` | Starts the Go API on :8081 and runs the website on localhost:3000 (Ctrl+C stops both). It serves the demo's index (`data/demo/index`) if `make demo-save` has built one, so searches and the pipeline replay come from the same run; otherwise it builds and serves the sample index. `WEB_INDEX=...` picks one by hand |
| `make merge` | Merges every result file in `RESULTS` (default `data/results`) into `data/merged.edges.tsv.gz` (`RESULTS=data/cluster/results make merge` for the Docker cluster) |
| `make pagerank` | Scores every domain in `data/merged.edges.tsv.gz` and writes `data/scores.tsv` |
| `make index` | Builds the sharded backlink index in `data/index/` from `data/merged.edges.tsv.gz` and `data/scores.tsv` |
| `make api` | Serves lookups from `INDEX` (default `data/index`) on localhost:8081 |
| `make loadtest` | Measures lookup speed against a running API (8 at once for 10s; uses most of the CPU while it runs) |
| `make cc-ranks` | Saves Common Crawl's top 1,000,000 domains (`CC_TOP`) from their web graph (`CC_GRAPH`, default `cc-main-2026-aug-sep-oct`) to `data/cc-domain-ranks.tsv` |
| `make validate` | Compares `data/scores.tsv` with `data/cc-domain-ranks.tsv` and prints how closely they agree |
| `make demo` | A small, light local run to watch on the dashboard: cuts the sample into `DEMO_FILES` (default 10) files of `DEMO_PAGES` (default 2,000) real pages, serves them from your own machine, and starts a coordinator on :8080 with `DEMO_WORKERS` (default 3) workers using one parser each. Nothing is downloaded. Ctrl+C stops everything |
| `make record` | Saves the finished run in `RUN_DB` (default `data/demo/jobs.db`) to `data/run.json` for the website's replay. `NAME="..."` sets its label. Restart the API (`make web`) to load it |
| `make demo-index` | Merges the demo's results (noting which files each site's links came from), runs PageRank, and builds `data/demo/index` |
| `make demo-save` | `make demo-index` then `make record`: after a demo, this is the one command that saves it for the website |
| `make sample` | Downloads the first WAT file of the crawl in `CRAWL` (default `CC-MAIN-2026-43`) to `data/sample.warc.wat.gz` |

## Running a cluster without Docker

```sh
make build
./bin/coordinator -limit 20 &                       # queue 20 files
for i in 1 2 3 4; do ./bin/worker -id w$i -parsers 2 & done
curl -s localhost:8080/status                       # watch progress
```

Workers exit on their own once every job is done or failed. Stop the coordinator with Ctrl+C (or `kill`).

## Watching a run on the dashboard

Start the website in one terminal and a run in another, then open http://localhost:3000/pipeline:

```sh
make web     # terminal 1: website + lookup API
make demo    # terminal 2: coordinator + 3 workers on local copies of the sample file
```

The dashboard reads any coordinator at `LINKSCOPE_COORDINATOR` (default `http://localhost:8080`), so it also works with `make up` or a cluster started by hand.

**About `make demo`:** `cmd/slicewat` cuts the start of the sample into small files of consecutive records, so every file has different real pages and links (10 × 2,000 = 20,000 of the sample's 22,020 pages). They're served by `python3 -m http.server` on 127.0.0.1:8090, and the workers download from there instead of Common Crawl. The files are small on purpose, so a demo is easy on a laptop: a 100-page slice took 0.03s to extract on one core, and a 2,000-page file is about a tenth of the sample. An earlier version gave each worker a full copy of the sample, which kept a core busy for about 9 seconds per file.

The coordinator keeps running after the work is done so the finished run stays on the dashboard. Ctrl+C stops it, then `make demo-save` builds the demo's own index and saves the run to `data/run.json`. Restart `make web` and the search box and the pipeline replay both use that run.

**Why the demo gets its own index:** the website shows a replay of each search, highlighting the files the site's links came from. That's only true if the search answers come from the same run as the replay. Before this, searches came from the sample index and the replay came from the demo run.

`make demo` deletes `data/demo/` (and its index) first, so save a run before starting another. Until the next `make demo-save`, `make web` falls back to the sample index.

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
