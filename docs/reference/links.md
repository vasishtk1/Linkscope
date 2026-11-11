# internal/links

Decides which links count as backlinks, reduces URLs to domains, and aggregates them into a domain-to-domain graph.

## domain.go

### `Domain(u *url.URL) (string, bool)`

Reduces a URL to its **registered domain**, the part someone actually buys and owns.

| Input | Output |
|---|---|
| `https://www.nytimes.com/article` | `nytimes.com` |
| `https://blog.bbc.co.uk/` | `bbc.co.uk` |
| `https://alice.github.io/` | `alice.github.io` |
| `https://münchen.de/` | `xn--mnchen-3ya.de` |
| `http://192.168.1.1/`, `mailto:…`, `http://printer.local/` | rejected |

Steps:
1. Only `http`/`https`
2. Strip port and trailing dot; reject IP addresses
3. Convert international names to their ASCII form (punycode) with IDNA lookup rules, which also lowercases and normalizes lookalike Unicode characters. Without this, the same site could appear as several different strings
4. Reject hosts whose ending isn't a real TLD (`.local`, `.internal`, `localhost`)
5. Use the [Public Suffix List](https://publicsuffix.org) to find the registered domain

**Why a list is needed:** you can't just take the last two labels. `bbc.co.uk` is one site, but `co.uk` is not. The Public Suffix List records every ending under which people register names, including private ones like `github.io`, where each user's subdomain is a separate site.

## extract.go

### `Target`

One outgoing link that counts: the target `Domain`, the full resolved `URL` (fragment removed), and whether it's `Nofollow`.

### `Outlinks`

A page's counted links: `SourceURL`, `SourceDomain`, and its `Targets`.

### `Extract(p wat.Page) (Outlinks, bool)`

Picks out the links on a page that count as backlinks to other sites.

1. Skip the page unless the status is 2xx (error pages and redirects aren't real content)
2. Find the page's own domain; skip the page if it has none
3. Resolve relative links against the `<base>` tag if present, otherwise against the page URL. This matches browser behavior: `page.html` on `https://site.com/blog/post` means `https://site.com/blog/page.html`
4. Keep only `A@/href` and `AREA@/href`, which are clickable links. Images, scripts, and stylesheets are resources the page loads, not recommendations of another site
5. Drop links to the page's own domain, since internal links aren't backlinks
6. Remove `#fragment`, since `page#a` and `page#b` are the same page

### `isNofollow(rel string) bool`

True if `rel` contains `nofollow`, `sponsored`, or `ugc` (case-insensitive). All three tell search engines not to treat the link as an endorsement: `sponsored` marks paid links and `ugc` marks user-generated content such as comments.

## graph.go

### `Edge`

A link relationship from one domain to another.

| Field | Meaning |
|---|---|
| `Src`, `Dst` | Source and destination domains |
| `Pages` | Number of pages on `Src` that link to `Dst` |
| `NofollowPages` | How many of those pages link to `Dst` **only** with nofollow links |
| `ExampleSrc`, `ExampleDst` | One example link, for display ("pinchofyum.com/best-pans → ourplace.com/always-pan") |

### `Graph`

A map from `(src, dst)` to `Edge`, plus a second map from `(exact page, src)` to `PageLink` (see `pages.go`).

### `NewGraph() *Graph`

Creates an empty graph.

### `(*Graph) Add(o Outlinks)`

Adds one page's links.

- **Counts each page once per target domain.** A page with 50 links to `facebook.com` (a footer repeated on every page, say) counts as 1. Counting raw links would let navigation menus and footers drown out real editorial links.
- A page counts as nofollow for a domain only if **every** link from it to that domain is nofollow.
- **Examples are picked as the smallest URL**, alphabetically, not the first one seen.
- Also records page-level links with `addPages` (see `pages.go`).

**Why "smallest" instead of "first":** the result must be identical no matter what order pages arrive in. Pages arrive in a different order every run when several goroutines parse in parallel, and in a different order again when the distributed workers' outputs are merged in Stage 2. Choosing the minimum makes `Add` order-independent, the same way `+` is. Stage 3's crash test relies on this: a run with crashes must produce byte-identical output to a clean run.

### `(*Graph) AddEdge(e Edge)`

Adds an edge that was already counted, such as one read back from a worker's result file. Used by the merge in stage 4.

- If the `Src → Dst` pair is new, the edge is stored as-is.
- If it already exists, `Pages` and `NofollowPages` are added together, and the example link becomes whichever is alphabetically smaller.

This follows the same rules as `Add`, so the merged result is the same no matter what order the files are read in. Adding up per-file counts is correct because each page lives in exactly one Common Crawl file, so no page is counted twice.

### `(*Graph) Len() int`

Number of edges.

### `(*Graph) Edges() []Edge`

All edges, sorted by `Src` and then `Dst`. Sorting gives a stable, comparable output, since Go deliberately randomizes map iteration order.

### `WriteTSV(w io.Writer, edges []Edge) error`

Writes edges as tab-separated values with a header row. TSV is safe here because tabs can't appear in parsed URLs or domains (Go's URL parser rejects control characters).

### `ReadTSV(r io.Reader, fn func(Edge) error) error`

Reads a file written by `WriteTSV` and calls `fn` once per edge, in file order.

- Checks the header row, so a file in the wrong format fails right away instead of producing garbage.
- Errors include the line number (`line 4012: bad pages "x"`).
- Rejects lines without exactly 6 fields, page counts below 1, a nofollow count that's negative or larger than the page count, and empty domains.
- If `fn` returns an error, reading stops and that error is returned.

**Why a callback instead of returning a slice:** the caller decides what to do with each edge as it's read. The merge adds each edge straight into one shared graph, so a file's edges never need to be held in memory as a separate list first.

### `parseEdge(line string) (Edge, error)`

Splits one TSV line into an `Edge` and runs the checks above.

## pages.go

Page-level links: not just "site A links to site B", but "site A links to **this exact page** on site B". The domain graph keeps only one example URL per pair, which isn't enough to say which of a site's pages get linked. Lost backlink recovery (the planned PR agent) needs exactly that: a page that's now broken but still gets links.

### `PageLink`

| Field | Meaning |
|---|---|
| `Dst` | The linked-to site (registered domain), e.g. `nytimes.com` |
| `Page` | The exact page, from `PageKey`, e.g. `nytimes.com/games/wordle` |
| `Src` | The linking site |
| `Pages` | How many pages on `Src` link to `Page` |
| `NofollowPages` | How many of those link to it only with nofollow links |

### `PageKey(raw string) (string, bool)`

Turns a link target into the page it points at: host and path, without the scheme, a leading `www.`, the `?query` or the `#fragment`. Examples: `https://www.Example.com/About/?ref=x#top` becomes `example.com/About`; `http://example.com` becomes `example.com/`.

- **Why drop the query:** most query strings on links are tracking tags (`?utm_source=...`, `?ref=...`), so keeping them would split one page into many. The cost is that the rare site that uses queries for real pages (`/item?id=5`) gets those merged.
- **Why drop `http`/`https` and `www.`:** they almost always serve the same page. The host is lowercased (hosts aren't case-sensitive) but the path isn't (paths can be).
- **Why keep subdomains:** `blog.example.com/post` and `example.com/post` are different pages.
- A trailing `/` is removed except on the home page, so `/post/` and `/post` count as one.
- The path stays percent-escaped (`EscapedPath`), so a page key can never contain a tab or newline and break the TSV.

### `(*Graph) addPages(o Outlinks)`

Called by `Add`. Counts each source page **once per exact target page**, the same rule `Add` uses per domain: a page that links to `b.com/x` three times (with different tracking tags, say) counts as 1. A page counts as nofollow for a target page only if every link from it to that page is nofollow.

### `(*Graph) PageLinks() []PageLink`

All page links, sorted by `Dst`, then `Page`, then `Src`, so the output is identical every run.

### `WritePagesTSV(w io.Writer, pl []PageLink) error`

Writes page links as TSV with the header `dst	page	src	pages	nofollow_pages`.

### `ReadPagesTSV(r io.Reader, fn func(PageLink) error) error`

Reads a file written by `WritePagesTSV`, calling `fn` per row. Like `ReadTSV`, it checks the header and gives line numbers, and rejects rows without 5 fields, empty fields, page counts below 1, and nofollow counts out of range.

### `parsePageLink(line string) (PageLink, error)`

Splits one row and runs those checks.

### Tests (`pages_test.go`)

| Test | What it proves |
|---|---|
| `TestPageKey` | Scheme, `www.`, query, fragment, trailing slash and a trailing dot on the host are dropped; subdomains and escaping are kept; links without a host are rejected |
| `TestPageLinksCountEachSourcePageOncePerTargetPage` | Two links to the same page with different tracking tags count once; nofollow is per page; output is sorted |
| `TestPagesTSVRoundTrip` | Writing then reading gives back the same rows |
| `TestReadPagesTSVRejectsBadRows` | Empty file, wrong header, bad counts and missing fields all fail |
