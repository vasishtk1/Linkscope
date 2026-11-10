# internal/merge

Combines the workers' result files (one per Common Crawl file) into a single domain link graph. This is the first half of stage 4. PageRank runs on its output.

**Why it's needed:** each worker saves the links it found in its own file. The same pair, like `reddit.com → github.com`, shows up in thousands of those files. Merging turns all of them into one line with the page counts added up.

```
x-00000.edges.tsv.gz   reddit.com → github.com   12 pages
x-00001.edges.tsv.gz   reddit.com → github.com    7 pages      ──merge──▶   reddit.com → github.com   19 pages
```

## merge.go

### `Stats`

| Field | Meaning |
|---|---|
| `Files` | Result files read |
| `EdgesRead` | Edges read across all files, before combining |

### `ResultFiles(dir string) ([]string, error)`

Lists every `*.edges.tsv.gz` file in `dir`, sorted by name.

Workers write to a temporary file (`….edges.tsv.gz.tmp-123`) and rename it when done, so a half-written file never matches this pattern. Running the merge while workers are still busy just skips their unfinished jobs.

### `Files(paths []string) (*links.Graph, Stats, error)`

Reads each file in turn and adds every edge to one graph with `links.Graph.AddEdge`. If any file is bad, it stops and returns an error that names the file.

**Why the order doesn't matter:** `AddEdge` adds counts together and keeps the smallest example link, and neither depends on order. So merging files 1, 2, 3 gives exactly the same result as 3, 2, 1. A test checks this, and also checks that splitting pages across files and merging gives exactly the same graph as processing all the pages together.

**Memory:** the whole merged graph is held in memory. This is fine for laptop-sized runs: merging one real file (130,138 edges) peaked at about 86 MB of memory, roughly 650 bytes per edge. A full crawl would not fit; see "Merge in memory for now" in [design-decisions.md](../design-decisions.md).

### `FilesWithOrigins(paths []string) (*links.Graph, Origins, Stats, error)`

Same as `Files`, but it also notes, for every linked-to site, which result files had links to it and how many linking pages each one had. The website uses this to show which files a searched site's links came from (see `origins.go` below).

It's a separate function because the extra map costs memory, so a normal merge doesn't pay for it.

### `read(paths []string, o Origins) (*links.Graph, Stats, error)`

Does the work for both functions above. When `o` isn't nil, each edge is also added to it under the file's name.

### `readFile(path string, add func(links.Edge)) (int, error)`

Opens one file, unzips it if the name ends in `.gz`, and calls `add` for each edge. Returns how many edges it read.

### `WriteFile(path string, edges []links.Edge) error`

Writes edges as TSV, gzipped if `path` ends in `.gz`, using `writeAtomic`.

### `writeAtomic(path string, write func(io.Writer) error) error`

Creates the folder if needed, writes to a temp file, then renames it into place, so nothing reading the output ever sees a half-written file. Gzips when `path` ends in `.gz`.

## origins.go

Keeps track of where each site's links were found. After a merge, `reddit.com → github.com, 19 pages` no longer says which files those pages were in. Origins keeps that:

```
github.com   x-00000   12 pages
github.com   x-00001    7 pages
```

The file names match the job files in the pipeline replay (`x-00000.edges.tsv.gz` came from job `x-00000.warc.wat.gz`). That's how the website highlights the right files when it replays a search.

### `Origin`

| Field | Meaning |
|---|---|
| `File` | Result file name without folder or extension, e.g. `x-00000` |
| `Pages` | Linking pages to the site found in that file |

### `Origins`

`map[site]map[file]pages`. Built while merging; `add(dst, file, pages)` adds one edge's pages.

### `FileName(path string) string`

`data/results/x-00042.edges.tsv.gz` → `x-00042`.

### `(o Origins) List(dst string) []Origin`

One site's origins, most linking pages first, then by file name, so the order is always the same.

### `WriteOrigins(path string, o Origins) error`

Writes `dst, file, pages` rows with a header, sorted by site, then by `List` order. Gzipped if the name ends in `.gz`, written atomically.

### `ReadOrigins(path string) (map[string][]Origin, error)`

Loads that file back into per-site lists in `List` order. `cmd/index` uses it to store origins in the index.

**Test:** for every site, the pages in its origins add up to the pages linking to it in the merged graph, and the file survives a write and read.
