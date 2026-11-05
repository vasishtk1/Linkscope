# internal/pagerank

Scores every domain from the merged link graph. This is the second half of stage 4.

## The idea

A link is a vote, and votes from important sites count more. Each site **splits its score evenly among the sites it links to**, so a link from a site that links to 1,000 places is worth less than a link from a site that links to 2.

Your score is the sum, over every site linking to you, of (their score ÷ how many sites they link to). That's circular, since you need everyone's score to work out anyone's, so it's computed in rounds: start everyone equal, hand scores along the links, and repeat until the numbers stop changing.

**Worked example** (no damping): A links to B and C, B links to C, C links to A.

| Round | A | B | C |
|---|---|---|---|
| Start | 0.33 | 0.33 | 0.33 |
| 1 | 0.33 | 0.17 | 0.50 |
| 2 | 0.50 | 0.17 | 0.33 |
| 3 | 0.33 | 0.25 | 0.42 |
| Settled | 0.40 | 0.20 | 0.40 |

A has one backlink but ties C, which has two, because A's one link comes from C, the strongest site, and C links nowhere else. `TestThreeSiteExample` checks these exact numbers.

**The full formula** for a site `x`, with `N` sites and damping `d = 0.85`:

```
rank(x) = (1 − d) / N  +  d × dead_end_total / N  +  d × Σ rank(s) / out_links(s)    for every s that links to x
```

- **`(1 − d) / N`:** 15% of all score is shared equally every round. Without it, two sites that only link to each other would soak up score forever.
- **`dead_end_total / N`:** sites that link nowhere (most domains we only see as link targets) would otherwise leak their score out of the system. Their total is spread evenly, so the ranks always add up to 1.

## pagerank.go

### `Options` and `DefaultOptions()`

| Field | Default | Meaning |
|---|---|---|
| `Damping` | 0.85 | Share of score that flows through links (the value from the original PageRank paper) |
| `MaxIter` | 100 | Stop after this many rounds even if not settled |
| `Tolerance` | 1e-9 | Stop once the total change across all sites in a round is below this |

### `Result`

| Field | Meaning |
|---|---|
| `Domains` | Every domain in the graph, sorted alphabetically |
| `Rank` | Raw PageRank for each domain (same order); adds up to 1 |
| `LinkingDomains` | How many sites pass score to each domain |
| `Iterations`, `Delta` | Rounds run, and the total change in the last round |

### `Compute(edges []links.Edge, o Options) Result`

1. Collects every domain from the edges, sorts them, and gives each a number. Working with numbers in plain arrays is much faster than looking up strings in a map every round.
2. Turns each edge into a vote, **skipping nofollow-only edges**. Each `src → dst` pair is **one vote no matter how many pages it covers**.
3. Starts every site at `1/N` and runs rounds of the formula above until the change drops below `Tolerance` or `MaxIter` is hit. Two arrays are swapped each round instead of allocating new ones.

**Same output every run:** domains and votes are sorted before the math, so floating-point additions always happen in the same order. Adding the same numbers in a different order can give slightly different results, so without the sort the output could change between runs.

On the sample file (77,793 domains, 130,138 edges) it settles in 21 rounds and takes 0.06s.

### `Score`

One row of output: `Domain`, the 0–100 `Score`, the raw `Rank`, and `LinkingDomains`.

### `Scores(r Result) []Score`

Turns raw ranks into a 0–100 score and sorts from highest to lowest (ties broken alphabetically).

```
score = round(100 × (log(rank) − log(lowest)) / (log(highest) − log(lowest)))
```

**Why a log scale:** raw ranks are extremely lopsided. On the sample, facebook.com's rank is about 1,500 times the lowest. On a straight scale almost every site would get 0. A log scale spreads them out, so going from 10 to 20 means about the same jump in strength as going from 80 to 90.

The highest-ranked site gets 100. Sites nothing votes for share the lowest rank and get 0. Scores are **relative to the data processed**: the same site can score differently after more files are added, as with other authority scores.

### `WriteTSV(w io.Writer, scores []Score) error`

Writes `domain`, `score`, `pagerank` (scientific notation), and `linking_domains` as TSV with a header row.

### `ReadTSV(r io.Reader) ([]Score, error)`

Reads a file written by `WriteTSV`, in file order (highest first). Checks the header and fails with the line number on a bad row. Used by `cmd/validate`, and the same format is read by the website.
