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

### `main()`

Placeholder until Stage 2. Prints that it isn't implemented yet.

## cmd/worker

### `main()`

Placeholder until Stage 2. Prints that it isn't implemented yet.
