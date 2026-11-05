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

### `readFile(path string, g *links.Graph) (int, error)`

Opens one file, unzips it if the name ends in `.gz`, and feeds its edges into `g`. Returns how many edges it read.

### `WriteFile(path string, edges []links.Edge) error`

Writes edges as TSV, gzipped if `path` ends in `.gz`. It creates the folder if needed, writes to a temp file, then renames it into place, so nothing reading the output ever sees a half-written file.
