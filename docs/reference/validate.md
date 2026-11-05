# internal/validate

Checks our PageRank against Common Crawl's own published domain ranking. This is the last part of stage 4, and it's how we know the scores are right rather than just plausible.

## Where the reference comes from

Common Crawl publishes a [web graph](https://commoncrawl.org/web-graphs) every few months, with a PageRank for every domain. We use `cc-main-2026-aug-sep-oct`, built from three crawls including `CC-MAIN-2026-43`, the one our files come from. Its domain ranks file is 2.4 GB, sorted from strongest to weakest, so `make cc-ranks` streams only the first million lines (65 MB as plain text) into `data/cc-domain-ranks.tsv`.

**Why the two rankings can't match exactly:**
- Their graph counts every link, including scripts, fonts and images. That's why `googleapis.com` and `googletagmanager.com` are near the top of theirs. We count only clickable links (see "Only clickable links count" in [design-decisions.md](../design-decisions.md)).
- Theirs is built from three full crawls. Ours is built from however many files we've processed.

So the goal is strong agreement, not an identical list.

## How agreement is measured

**Spearman rank correlation.** Line up both rankings for the domains that appear in both, and measure how similar the *order* is: 1 means the same order, 0 means no relation, −1 means reversed. Only order matters, not the raw values, which is what we want, since our numbers and theirs are on different scales.

It's reported at several **minimum linking-site counts**. A domain with one backlink in our data is mostly noise: we've seen only a tiny slice of the web that links to it. Agreement should rise as we require more evidence per domain. If it doesn't, something is wrong with the scoring.

## validate.go

### `Reference`

One domain from Common Crawl's list: `PRPos` (its PageRank position, 1 = top) and `PR` (the raw value).

### `ReadRanks(r io.Reader) (map[string]Reference, error)`

Reads Common Crawl's ranks file. Each line has `harmonicc_pos`, `harmonicc_val`, `pr_pos`, `pr_val`, `host_rev`, `n_hosts`. Lines starting with `#` are headers. The domain is stored backwards (`com.facebook`), so it's turned back with `reverseDomain`. Fails on an empty file or a bad number.

### `reverseDomain(s string) string`

`com.facebook` → `facebook.com`, `uk.co.bbc` → `bbc.co.uk`. Common Crawl stores names backwards so that related domains sort next to each other.

### `Level`, `Row`, `Report`

- `Level`: one line of the agreement table: the minimum linking sites, how many of our domains qualify, how many of those are in Common Crawl's list, and the Spearman value.
- `Row`: one of our top domains with its position in Common Crawl's PageRank (`CCPos`, 0 if not in their list).
- `Report`: the reference size, the levels, and the top rows.

### `Compare(ours []pagerank.Score, ref map[string]Reference, minLinks []int, top int) Report`

For each minimum, keeps our domains with at least that many linking sites, finds the ones in Common Crawl's list, and computes Spearman between our rank and theirs. Also builds the side-by-side table for our top domains.

### `Spearman(x, y []float64) float64`

Converts both lists to positions with `ranks`, then measures how well the positions line up (Pearson correlation of the positions). Returns `NaN` for fewer than 2 points or when every value is the same.

### `ranks(x []float64) []float64`

Gives each value its position, with 1 for the smallest. **Ties share the average position:** `[5, 1, 5, 3]` → `[3.5, 1, 3.5, 2]`. This matters because many small domains have exactly the same PageRank, and giving them made-up different positions would distort the result.

### `pearson(x, y []float64) float64`

Standard correlation: how well the two lists rise and fall together, from −1 to 1.

## Results

One WAT file (`CC-MAIN-2026-43`, 77,793 domains) against Common Crawl's top 1,000,000:

| Min linking sites | Our domains | In CC list | Agreement (Spearman) |
|---|---|---|---|
| 1 | 57,671 | 13,744 | 0.466 |
| 5 | 704 | 616 | 0.516 |
| 20 | 142 | 137 | 0.643 |
| 100 | 35 | 35 | 0.759 |

All of our top 20 are in Common Crawl's top 179, and 18 are in their top 100. Agreement rises with the evidence per domain, as expected. One file is a tiny sample of the web, so these numbers should go up as more files are processed.
