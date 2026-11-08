# internal/index

The sharded backlink index. It's built once from the merged link graph and the PageRank scores. After that, any site's backlinks can be looked up without reading the whole graph. This is stage 5.

**Why it's needed:** the website used to load the entire edge file into memory and group it on every start. That works for one Common Crawl file, but not for the stage 7 run. The index does that grouping once, ahead of time, and saves it in a form where one lookup reads only the bytes it needs.

## How a lookup works

```
"github.com"
   │  ShardOf: FNV-1a hash of the name, % 8  →  5
   ▼
shard-005.idx
   │  directory (in memory, sorted by domain): binary search  →  offset 1,204,331, length 9,882
   ▼
one ReadAt of 9,882 bytes from disk  →  github.com's backlinks, most linking pages first
```

Two separate ideas are involved:

- **Sharding** splits the index into N files (8 by default). The hash of a domain decides which file it goes in. The same name always gives the same number, so building and looking up always agree, and a lookup opens only one shard. Hash values look random, so domains spread evenly: on the 20-file data, every shard holds between 73,127 and 73,763 domains.
- **Indexing** is what makes a lookup fast inside a shard. Each shard ends with a directory that is sorted by domain and says where each domain's backlinks are stored. Finding a domain is a binary search, about 17 comparisons for 73,000 domains, followed by one disk read.

## File layout

```
data/index/
├── manifest.json     # shard count, hash name, totals, data description
├── shard-000.idx
├── …
└── shard-007.idx
```

Each shard file has three parts:

| Part | Contents |
|---|---|
| Backlink lists | For each domain, in name order, its backlinks as TSV lines: `src  pages  nofollow_pages  example_src  example_dst`, most linking pages first (ties by name) |
| Directory | One TSV line per domain in this shard: `domain  score  linking_domains  linking_pages  nofollow_pages  offset  length`. `score` is `-1` when there's no score |
| Footer | 16 bytes: where the directory starts (8-byte big-endian number), then the marker `LSIDX001` |

The footer goes last so the file can be written in one pass: the directory's position is only known after every backlink list has been written. Opening a shard means reading the footer, jumping to the directory, and loading it.

**Every domain is in the directory**, including domains with no backlinks (with offset and length 0). A domain that only links out still has a score and is still a "known" site.

## build.go

### `Manifest`

| Field | Meaning |
|---|---|
| `Shards` | Number of shard files |
| `Hash` | `"fnv1a-64"`; `Open` refuses anything else, so a future change of hash can't silently send lookups to the wrong shard |
| `Domains` | Domains in the index |
| `Edges` | `src → dst` edges in the index |
| `Source` | Description of the data, shown on the website, e.g. "Sample: 1 Common Crawl file from CC-MAIN-2026-43" |

### `Options`

`Shards` (at least 1) and `Source`.

### `ShardOf(domain string, shards int) int`

FNV-1a 64-bit hash of the name, modulo the shard count. FNV is in Go's standard library, fast on short strings like domain names, and spreads them evenly. It isn't a secure hash, but nothing here needs one.

### `Build(dir string, edges []links.Edge, scores map[string]int, o Options) (Manifest, error)`

1. Groups edges by target domain, and collects every domain from the edges and the scores.
2. Assigns each domain to a shard with `ShardOf` and sorts each shard's domains.
3. Writes every shard and the manifest into a temp folder next to `dir`, then swaps it into place.

Writing to a temp folder means a failed build never leaves a half-written index. The old index is deleted just before the rename. A server that already has the old shards open keeps reading them, because on Linux and macOS an open file stays readable after it's deleted. It sees the new index after a restart.

**The output is byte-identical for the same input, in any order:** domains are sorted inside each shard and backlinks are sorted by pages, then name. A test builds from reversed input and compares the files.

### `writeShard(path string, domains []string, byDst map[string][]Backlink, scores map[string]int) error`

Writes one shard: the backlink lists, keeping track of each one's start and length, then the directory, then the footer.

### `sortBacklinks(bl []Backlink)`

Most linking pages first, ties broken by domain name. The website shows backlinks in this order, so the API can return the top N with one read.

## index.go

### `Backlink`

One site linking to the looked-up domain: `Domain`, `Pages`, `NofollowPages`, `ExampleSource`, `ExampleTarget`. The JSON field names match what the website already used.

### `Entry`

One directory line: `Domain`, `Score` (`-1` if none), `LinkingDomains`, `LinkingPages`, `NofollowPages`, plus unexported shard number, offset and length.

### `Open(dir string) (*Index, error)`

Reads the manifest and every shard's directory into memory. The backlink lists stay on disk. It checks the footer marker, that the directory is sorted (binary search depends on it), and that every offset and length falls inside the file. A damaged or cut-off shard is refused at startup instead of returning wrong answers later.

**Memory:** only the directories are loaded. On the 20-file data that's 588,294 entries, small compared to the 200.7 MB of shard files.

### `(*Index) Find(domain string) (Entry, bool)`

Exact-name lookup: hash to the shard, then `sort.Search` (binary search) over its directory.

### `(*Index) Backlinks(e Entry, limit int) ([]Backlink, error)`

One `ReadAt` of the domain's whole list, then parses up to `limit` lines (0 = all). `ReadAt` is safe to call from many goroutines at once on the same file, so the API server can answer many lookups in parallel without locks.

### `(*Index) Close() error`

Closes every shard file.

## lookup.go

### `Lookup`

The answer to one search: `Domain`, `Score` (`null` in JSON when unknown), `LinkingDomains`, `LinkingPages`, `NofollowPages`, `Backlinks`, `Source`. This is the same shape the website's `Lookup` type already had.

### `Normalize(input string) string`

Turns what a user types into a bare host: lowercase, trim, drop the scheme, path, query, `user@`, port and trailing dot. `" HTTPS://www.GitHub.com/a?b "` becomes `www.github.com`.

**It keeps `www.`** and leaves it to `Resolve`. Some registered domains really start with `www.` because the rest is a public suffix: `www.gov.uk`, `www.nhs.uk`, `www.gov.br`. Stripping `www.` turned those into `gov.uk`, which isn't a site. The load test found this: 95 of the 412,419 domains with backlinks on the 20-file run couldn't be looked up. The old website had the same bug.

### `(*Index) Resolve(host string) (Entry, bool)`

Tries the host, then drops subdomains one at a time until it finds a domain that has backlinks. `docs.github.com` and `www.github.com` fall back to `github.com`, while `www.gov.uk` matches as it is. It stops before the last label, so it never tries a bare `com`.

### `(*Index) Lookup(input string, limit int) (*Lookup, error)`

`Normalize`, then `Resolve`, then `Backlinks`. Returns `nil` (not an error) when nothing links to the site.

## network.go

The data behind the animated background on the website's landing page: the most important sites and the links among them. It's the network we measured, not a made-up picture.

### `NetworkNode`, `NetworkLink`, `Network`

`Network` is `Nodes` (`domain`, `score`) plus `Links` (`source` links to `target` on `pages` pages). `Links` is always a list, never `null`, so the website doesn't have to check.

### `(*Index) Network(n int) (Network, error)`

1. Goes through every directory entry and keeps sites that have a score and at least one backlink. A site nothing links to would have no lines in the picture.
2. Sorts by score, highest first. Scores are whole numbers, so many sites tie; ties go to the site with more linking domains, then by name, so the answer is always the same.
3. Keeps the first `n`, reads each one's full backlink list, and keeps only the backlinks that come from another site in the top `n`.

It scans all directories and reads whole backlink lists, so it's slower than a lookup. The API runs it once and caches the answer (see `serve.md`). On the 1-file sample, the top 60 sites have 77 links among them, and the first call took 0.11s.

### `(Network) Top(n int) Network`

The first `n` nodes and only the links whose two ends are both kept. Because `Network` is sorted, the top 60 of the top 200 is the same as asking for the top 60 directly. This lets the API compute once for the largest size and answer any smaller one.

## Tests

| Test | Checks |
|---|---|
| `TestLookupMatchesInput` | Full lookup result with 1, 4 and 16 shards, from a messy URL |
| `TestLimitReturnsTopBacklinks` | `limit` returns the top N; totals still count all |
| `TestSubdomainFallsBackToItsSite` | `docs.github.com` → `github.com` |
| `TestDomainThatStartsWithWWW` | `www.gov.uk` is found as itself; `www.github.com` still finds `github.com` |
| `TestDomainsWithoutBacklinks` | Link-only domains aren't search results but are still in the directory with their score |
| `TestMissingScoreIsNull` | No score → `null` |
| `TestEveryDomainIsInTheShardItsHashPicks` | Every domain is in the shard `ShardOf` names, and none are lost |
| `TestBuildIsByteIdenticalWhateverTheInputOrder` | Same files from reversed input |
| `TestRebuildReplacesOldIndex` | Rebuilding with fewer shards leaves no old shards or temp folders |
| `TestOpenRejectsDamagedShard` | A cut-off shard fails to open |
| `TestBuildRejectsZeroShards` | Bad option is an error |
| `TestNetworkPicksTopSitesAndLinksAmongThem` | Highest scores first, sites without backlinks skipped, ties go to more linking domains, only links inside the top N, `Top` drops links to removed sites |
| `TestNormalize` | Scheme, case, path, port, `user@`, trailing dot; `www.` kept |

The index was also checked against the raw edges on real data. Every domain with backlinks was looked up and compared row by row with the edge file: all 62,052 on the 1-file sample and all 462,254 on the 20-file run matched.
