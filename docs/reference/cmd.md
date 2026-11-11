# cmd/

Each folder is one runnable program.

## cmd/extract

Runs the extractor on a single WAT file and prints a report.

```sh
make build
./bin/extract data/sample.warc.wat.gz                 # report + top 15 edges
./bin/extract -o data/edges.tsv -top 30 data/sample.warc.wat.gz
./bin/extract -workers 1 data/sample.warc.wat.gz      # single-threaded, for comparison
./bin/extract -o edges.tsv -pages pages.tsv.gz -top 0 data/sample.warc.wat.gz   # also page-level links
```

| Flag | Default | Meaning |
|---|---|---|
| `-o` | none | Write every edge as TSV to this file |
| `-pages` | none | Write page-level links (which exact pages each site links to, see [links.md](links.md#pagesgo)) to this file, gzipped if it ends in `.gz`. Same format as a worker's `X.pages.tsv.gz` |
| `-top` | 15 | How many top edges to print |
| `-workers` | number of CPU cores | Parser goroutines |

### `main()`

Parses flags, checks there's exactly one file argument, and calls `run`. On error it prints the error and exits with status 1 (status 2 for wrong usage).

### `run(path, outPath, pagesPath string, top, workers int) error`

Opens the file, times `extract.Run`, optionally writes the edges and page links, and prints the report. It returns errors instead of exiting, so `main` is the only place that decides how the program ends.

### `writeFile(path string, write func(io.Writer) error) error`

Creates the output file and fills it with `write`, gzipping when the name ends in `.gz`. It checks the errors from closing the gzip writer and the file, since that's where a failed final write to disk shows up.

On the 1-file sample with 4 parsers, writing both files took 3.9s with a 208 MB memory peak. It finds 177,399 site → page links, against 130,138 site → site edges.

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
| `-origins` | none | Also write which result files each site's links came from (see [merge.md](merge.md#originsgo)). `cmd/index -origins` stores them in the index |
| `-pages` | none | Also write each site's most linked pages, from the `*.pages.tsv.gz` files next to the results (see [merge.md](merge.md#pagesgo)). `cmd/index -pages` stores them in the index |

### `main()`

Parses flags, rejects extra arguments, and calls `run`.

### `run(in, out, originsPath, pagesPath string) error`

Lists the result files, then runs `mergeEdges` and, when `pagesPath` is set, `mergePages`. It fails if the folder has no result files, so a typo in `-in` doesn't quietly produce an empty graph.

### `mergeEdges(paths []string, out, originsPath string) error`

Merges the edges (with origins when `originsPath` is set), writes the output, and prints a report: files merged, edges read, merged edges, unique domains, total linking pages, and time.

**Why it's a separate function:** once it returns, nothing points at the edge graph any more, so Go's garbage collector can free it before `mergePages` builds its own big table. The two never have to fit in memory at the same time.

### `mergePages(paths []string, out string) error`

Runs `merge.Pages`, writes the result, and prints the pages files read, how many results had none, rows read, unique site → page pairs, and time.

On the demo results (2 copies of the 1-file sample), the whole merge with origins and pages took 1.53s with a 140 MB memory peak: 0.75s for edges, 0.57s for pages (354,798 rows read, 177,399 unique pairs, 62,052 sites).

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
| `-origins` | none | Origins file from `cmd/merge -origins`; stored next to each site's backlinks |
| `-pages` | none | Top pages file from `cmd/merge -pages`; stored after each site's origins |

### `main()`

Parses flags, rejects extra arguments, and calls `run`.

### `run(edgesPath, scoresPath, out string, shards int, source, originsPath, pagesPath string) error`

Loads the edges (reusing `merge.Files`, which handles gzip), reads the scores, origins and top pages (if given), calls `index.Build`, and prints the edge and domain counts, total shard size, and time. On the demo data with origins and pages: 0.81s, 197 MB memory peak, 31.2 MB of shards.

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
| `-run` | `data/run.json` | Saved run from `cmd/record`, served at `/run` for the pipeline replay. Optional: if the file doesn't exist, `/run` answers `404` |

### `main()`

Parses flags and calls `run`.

### `run(addr, dir, runPath string) error`

Opens the index, logs how many domains, edges and shards it loaded and how long that took, loads the saved run, and serves `serve.Handler`. Like the coordinator, it has time limits on reading requests and writing responses, and shuts down cleanly on Ctrl+C or `SIGTERM`.

### `loadRun(path string) (*record.Recording, error)`

Reads and parses the saved run. A missing file returns `nil` with a log line, so the API works without one. A file that exists but can't be parsed is an error, so a broken recording is noticed at startup.

## cmd/record

Saves a finished run from a coordinator's job database to a JSON file for the website's replay. It only reads the database, so it's instant and safe to run while the coordinator is still up.

```sh
make record                                          # data/demo/jobs.db → data/run.json
make record RUN_DB=data/cluster/jobs.db NAME="20 Common Crawl files, 4 workers"
```

| Flag | Default | Meaning |
|---|---|---|
| `-db` | `data/demo/jobs.db` | Job database of the run |
| `-o` | `data/run.json` | Where to write the recording |
| `-name` | `"<files> files, <workers> workers"` | Label shown on the website |

Prints a one-line summary. If the database is older than the event log, it also says the history was rebuilt and retries aren't shown.

### `run(dbPath, out, name string) error`

Checks the database exists (so a typo doesn't create an empty one), builds the recording with `record.FromStore`, and writes it to a temp file that's then renamed into place, so the API never reads a half-written file. Restart the API to pick up a new recording.

## cmd/slicewat

Cuts the start of a WAT file into smaller files of real pages, for `make demo`.

```sh
./bin/slicewat -in data/sample.warc.wat.gz -out data/demo/files -files 10 -pages 2000 > data/demo/paths.txt
```

| Flag | Default | Meaning |
|---|---|---|
| `-in` | `data/sample.warc.wat.gz` | WAT file to cut up |
| `-out` | `data/demo/files` | Folder for the new files (`demo-001.warc.wat.gz`, …) |
| `-files` | `10` | How many files to make |
| `-pages` | `2000` | Pages (HTTP responses) per file |

Prints each new file's name, one per line, which is exactly the path list the coordinator's `-paths` flag reads. If the input runs out early, it stops and says so on stderr.

### `run(in, out string, files, pages int) error`

Opens the input once and calls `writeFile` for each output, so file 2 continues where file 1 stopped. It only reads as much of the input as it needs.

### `writeFile(rd *warc.Reader, path string, pages int) (int, error)`

Copies `metadata` records with `warc.WriteRecord` until it has written `pages` HTTP responses. A record counts as a response if its JSON contains `"HTTP-Response-Metadata"`, which is a byte search instead of a full JSON parse, so cutting is quick. On a 100-page test file this count matched the extractor's exactly (100 pages, 0 parse errors).

## cmd/loadtest

Measures lookup speed against a running `cmd/api`. **It uses most of the CPU while it runs**, so keep runs short on a laptop.

```sh
make api &                                          # serve data/index on :8081
make loadtest                                       # 8 at once for 10s, domains from data/scores.tsv
./bin/loadtest -c 32 -d 10s                         # more at once
./bin/loadtest -top 100                             # only the 100 most-linked sites (big answers)
```

| Flag | Default | Meaning |
|---|---|---|
| `-url` | `http://localhost:8081` | API address |
| `-scores` | `data/scores.tsv` | Where to get domain names; must come from the same run as the API's index |
| `-top` | `0` | Only the N highest-ranked domains (0 = every domain with backlinks) |
| `-c` | `8` | Lookups in flight at once |
| `-d` | `10s` | How long to measure |
| `-warmup` | `2s` | Unmeasured run first, so the shard files are in the OS cache |
| `-limit` | `1000` | Backlinks per answer, like the website (0 = all) |

### `main()`

Parses flags and calls `run`.

### `run(o loadtest.Options, scoresPath string, top int, warmup time.Duration) error`

Picks domains, sets up an HTTP client that keeps one connection per goroutine open, runs the warm-up, then the measured run, and prints requests, errors, lookups per second, p50/p90/p99/max, and average answer size. It exits with an error if any lookup failed.

### `pickDomains(path string, top int) ([]string, error)`

Reads the scores file and keeps domains with at least one linking site, so every lookup should return `200`. Any error then points at a real problem.
