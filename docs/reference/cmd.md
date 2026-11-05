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
| `-fetch-retries` | `3` | Download retries before failing a job |
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
