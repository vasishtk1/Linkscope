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

## pages.go

Builds each site's **most linked pages** from the page-level files the workers write next to their results (`X.pages.tsv.gz`, see `links/pages.go`).

```
x-00000.pages.tsv.gz   nytimes.com/games/wordle  ← reddit.com   3 pages
x-00001.pages.tsv.gz   nytimes.com/games/wordle  ← reddit.com   2 pages
x-00001.pages.tsv.gz   nytimes.com/games/wordle  ← bbc.co.uk    1 page     ──▶   nytimes.com/games/wordle   2 sites, 6 pages
```

### `MaxPages`, `MaxPageSources`

200 pages kept per site, and the top 10 linking sites kept per page. **Why cap:** a big site like facebook.com has millions of linked pages. The website shows a top-10 list with "show all", and the planned agent cares about pages with many links, so the long tail isn't worth the index space. `SitePages.Total` still counts every linked page.

### `PageSource`, `TopPage`, `SitePages`

| Type | Fields |
|---|---|
| `PageSource` | `Domain` (a linking site), `Pages` (how many of its pages link) |
| `TopPage` | `Page`, `Sites` (different linking sites, exact), `Pages`, `NofollowPages`, `Sources` (biggest linking sites, at most 10) |
| `SitePages` | `Total` (how many of the site's pages get links), `Top` (at most 200 `TopPage`s) |

### `PageStats`

`Files` (pages files read), `Missing` (results without a pages file, e.g. from before page-level links existed), `Rows` (rows read), `Pairs` (unique page and linking-site pairs).

### `PagesPath(edgesPath string) string`

`x-00042.edges.tsv.gz` → `x-00042.pages.tsv.gz`.

### `Pages(paths []string) (map[string]SitePages, PageStats, error)`

Reads the pages file next to each result and builds every site's top pages.

1. Adds every row into one map keyed by `(page, linking site)`, summing pages across files.
2. Groups those by page: the number of entries for a page is its exact count of **different** linking sites.
3. Groups pages by site, sorts them (most linking sites first, then most pages, then by name) and keeps the top 200.

**Why count sites exactly instead of adding up per-file counts:** the same site often links to the same page from several files. Adding per-file site counts would count it again in each file. Keeping `(page, site)` pairs until the end avoids that, at the cost of memory: the pairs are the biggest thing merge holds. `cmd/merge` builds them only after the edge graph has been written and freed.

**Why sort by sites, not pages:** one site linking from 5,000 pages (a sidebar link, say) is weaker evidence than 50 different sites linking once each.

A result with no pages file is skipped and counted in `Missing`, so old results still merge.

### `readPagesFile(path string, add func(links.PageLink)) error`

Opens one gzipped pages file and passes each row to `add`.

### `sortPages`, `sortSources`

Fixed orders so the output is the same every run. Sources: most pages first, then by domain.

### `SourcesField(l []PageSource) string`, `ParseSourcesField(f string) ([]PageSource, error)`

Packs a page's linking sites into one field, `reddit.com:3,bbc.co.uk:1`, and reads it back. Domain names can't contain `:` or `,`, so splitting is safe. The index uses the same format.

### `WritePages(path string, sp map[string]SitePages) error`

One row per kept page, sorted by site then rank: `dst	total_pages	page	sites	pages	nofollow_pages	sources`. Written atomically, gzipped if the name ends in `.gz`. `total_pages` repeats on each of a site's rows; gzip makes that nearly free.

### `ReadPages(path string) (map[string]SitePages, error)`

Loads that file back. `cmd/index` uses it.

### Tests (`pages_test.go`)

| Test | What it proves |
|---|---|
| `TestPagesCombineAcrossFiles` | Three result files combine into exact site counts, page counts, nofollow counts and sources, in the right order |
| `TestPagesCountsResultsWithoutPagesFile` | A result without a pages file is counted as missing, not an error |
| `TestPagesAreCapped` | 205 pages keep 200 and report a total of 205; 13 linking sites keep 10 sources and report 13 sites |
| `TestWritePagesRoundTrip` | Writing then reading gives back the same data |
