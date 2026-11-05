# cmd/

Each folder is one runnable program.

## cmd/extract

Runs the extractor on a single WAT file and prints a report.

```sh
make build
./bin/extract data/sample.warc.wat.gz                 # report + top 15 edges
./bin/extract -o data/edges.tsv -top 30 data/sample.warc.wat.gz
./bin/extract -workers 1 data/sample.warc.wat.gz      # single-threaded, for comparison
```

| Flag | Default | Meaning |
|---|---|---|
| `-o` | none | Write every edge as TSV to this file |
| `-top` | 15 | How many top edges to print |
| `-workers` | number of CPU cores | Parser goroutines |

### `main()`

Parses flags, checks there's exactly one file argument, and calls `run`. On error it prints the error and exits with status 1 (status 2 for wrong usage).

### `run(path, outPath string, top, workers int) error`

Opens the file, times `extract.Run`, optionally writes the TSV, and prints the report. It returns errors instead of exiting, so `main` is the only place that decides how the program ends.

### `writeEdges(path string, edges []links.Edge) error`

Creates the output file and writes the TSV. It checks the error from `Close`, since that's where a failed final write to disk shows up.

### `printReport(...)`

Prints the counters, unique source and target domains, timing, throughput, and the top edges ranked by linking pages.

## cmd/coordinator

Loads the list of files to process, queues them as jobs, and serves the coordinator API.

```sh
./bin/coordinator                         # queue the first 20 files of CC-MAIN-2026-43, listen on :8080
./bin/coordinator -offset 20 -limit 100   # queue files 20–119 as well (existing jobs are kept)
curl localhost:8080/status                # progress as JSON
```

| Flag | Default | Meaning |
|---|---|---|
| `-addr` | `:8080` | Listen address |
| `-db` | `data/jobs.db` | SQLite job database (created if missing) |
| `-source` | `https://data.commoncrawl.org` | Common Crawl base URL |
| `-crawl` | `CC-MAIN-2026-43` | Which crawl |
| `-paths` | the crawl's `wat.paths.gz` | Path list to load instead (URL or local file) |
| `-offset`, `-limit` | `0`, `20` | Which slice of the list to queue (`-limit 0` = all) |
| `-lease` | `60s` | How long a job is held without a heartbeat |
| `-max-attempts` | `3` | Attempts before a job is marked failed |

### `main()`

Parses flags, sets millisecond log timestamps, and calls `run`.

### `run(...) error`

1. Sets up Ctrl+C / `SIGTERM` handling through a context
2. Opens the job database and creates its folder if needed
3. Loads the path list and queues the chosen slice (already-queued files are skipped)
4. Records the throughput baseline and starts the lease reaper (every second)
5. Serves HTTP until shutdown, then gives in-flight requests up to 5s to finish

The server has time limits so a frozen worker can't tie it up: 5s to send request headers (`ReadHeaderTimeout`), 20s to send the whole request (`ReadTimeout`), 20s to receive the answer (`WriteTimeout`), and idle connections are closed after 60s (`IdleTimeout`).

## cmd/worker

Runs one worker until the coordinator says all work is done.

```sh
./bin/worker                                    # connect to localhost:8080
./bin/worker -id w2 -parsers 2 -out data/results
```

| Flag | Default | Meaning |
|---|---|---|
| `-id` | `hostname-pid` | Name shown on the coordinator |
| `-coordinator` | `http://localhost:8080` | Coordinator URL |
| `-source` | `https://data.commoncrawl.org` | Common Crawl base URL |
| `-out` | `data/results` | Where result files are written |
| `-parsers` | number of CPU cores | Parser goroutines per file |
| `-fetch-retries` | `6` | Download retries before failing a job |
| `-busy-backoff` | `15s` | First wait when Common Crawl says it's busy (`429`/`503`); doubles up to 2 minutes |
| `-request-timeout` | `20s` | Time limit for each call to the coordinator |
| `-first-byte-timeout` | `30s` | How long Common Crawl can take to start sending a file |
| `-stall-timeout` | `20s` | Cancel a download when no data arrives for this long |

### `main()`

Parses flags, prefixes every log line with the worker's id (so logs from many workers can be told apart), sets up Ctrl+C / `SIGTERM` handling, and runs the worker. On Ctrl+C the worker hands its current job back with `Release` before exiting.

## cmd/merge

Combines every worker result file in a folder into one merged link graph.

```sh
make merge                                              # data/results → data/merged.edges.tsv.gz
./bin/merge -in data/cluster/results                    # results from the Docker cluster
./bin/merge -in data/results -o data/merged.edges.tsv   # plain text instead of gzip
```

| Flag | Default | Meaning |
|---|---|---|
| `-in` | `data/results` | Folder of `*.edges.tsv.gz` result files |
| `-o` | `data/merged.edges.tsv.gz` | Output file (gzipped if it ends in `.gz`) |

### `main()`

Parses flags, rejects extra arguments, and calls `run`.

### `run(in, out string) error`

Lists the result files, merges them, writes the output, and prints a report: files merged, edges read, merged edges, unique domains, total linking pages, and time. It fails if the folder has no result files, so a typo in `-in` doesn't quietly produce an empty graph.

## cmd/pagerank

Scores every domain in the merged link graph and prints the top ones.

```sh
make pagerank                                    # data/merged.edges.tsv.gz → data/scores.tsv
./bin/pagerank -top 50                           # print more of the ranking
./bin/pagerank -damping 1 -max-iter 1000         # no damping, for experiments
```

| Flag | Default | Meaning |
|---|---|---|
| `-in` | `data/merged.edges.tsv.gz` | Merged link graph from `cmd/merge` |
| `-o` | `data/scores.tsv` | Output scores |
| `-damping` | `0.85` | Share of score that flows through links (must be above 0 and at most 1) |
| `-max-iter` | `100` | Maximum rounds |
| `-tol` | `1e-9` | Stop once a round changes the total by less than this |
| `-top` | `20` | How many top domains to print |

### `main()`

Parses and checks flags, then calls `run`.

### `run(in, out string, opts pagerank.Options, top int) error`

Loads the merged graph (reusing `merge.Files` on the single input file), runs `pagerank.Compute` and `pagerank.Scores`, writes the output, and prints the domain count, rounds, timings, and top domains.

### `writeScores(path string, scores []pagerank.Score) error`

Writes the scores to a temp file and renames it into place, so the website never reads a half-written file.

## cmd/validate

Compares our scores with Common Crawl's published domain ranking and prints how closely they agree.

```sh
make cc-ranks       # once: save Common Crawl's top 1,000,000 domains to data/cc-domain-ranks.tsv
make validate       # compare data/scores.tsv against it
./bin/validate -min-links 1,10,50 -top 50
```

| Flag | Default | Meaning |
|---|---|---|
| `-scores` | `data/scores.tsv` | Our scores from `cmd/pagerank` |
| `-ranks` | `data/cc-domain-ranks.tsv` | Common Crawl's ranks (`make cc-ranks`) |
| `-min-links` | `1,5,20,100` | Report agreement for domains with at least this many linking sites |
| `-top` | `20` | Show this many of our top domains next to Common Crawl's positions |

### `main()`

Parses flags, including the comma-separated levels, and calls `run`.

### `parseLevels(s string) ([]int, error)`

Turns `"1,5,20"` into `[1 5 20]`, rejecting anything that isn't a whole number of 0 or more.

### `run(scoresPath, ranksPath string, minLinks []int, top int) error`

Reads both files, runs `validate.Compare`, and prints the agreement table and the side-by-side top list. If the ranks file is missing, the error says to run `make cc-ranks`.

## cmd/index

Builds the sharded backlink index from the merged link graph and the scores.

```sh
make index                                         # data/merged.edges.tsv.gz + data/scores.tsv → data/index/
make sample-index                                  # the sample file → data/sample-index/, for the website
./bin/index -shards 16 -source "20 files from CC-MAIN-2026-43"
```

| Flag | Default | Meaning |
|---|---|---|
| `-edges` | `data/merged.edges.tsv.gz` | Merged link graph from `cmd/merge` (plain `.tsv` works too) |
| `-scores` | `data/scores.tsv` | Scores from `cmd/pagerank`; if the file is missing, the index is built without scores |
| `-o` | `data/index` | Index folder (replaced if it exists) |
| `-shards` | `8` | Number of shard files |
| `-source` | none | Description of the data, shown on the website |

### `main()`

Parses flags, rejects extra arguments, and calls `run`.

### `run(edgesPath, scoresPath, out string, shards int, source string) error`

Loads the edges (reusing `merge.Files`, which handles gzip), reads the scores, calls `index.Build`, and prints the edge and domain counts, total shard size, and time.

### `readScores(path string) (map[string]int, error)`

Reads `scores.tsv` into a domain → score map. A missing file isn't an error, so an index can be built before PageRank has run; every score then shows as "—".

## cmd/api

Opens the index and serves lookups over HTTP. Routes are in [serve.md](serve.md).

```sh
make api                                    # serve data/index on :8081
INDEX=data/sample-index make api            # serve the sample index
curl -s "localhost:8081/lookup?domain=github.com&limit=5"
```

| Flag | Default | Meaning |
|---|---|---|
| `-addr` | `:8081` | Listen address |
| `-index` | `data/index` | Index folder from `cmd/index` |

### `main()`

Parses flags and calls `run`.

### `run(addr, dir string) error`

Opens the index, logs how many domains, edges and shards it loaded and how long that took, and serves `serve.Handler`. Like the coordinator, it has time limits on reading requests and writing responses, and shuts down cleanly on Ctrl+C or `SIGTERM`.
