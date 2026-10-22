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

A map from `(src, dst)` to `Edge`.

### `NewGraph() *Graph`

Creates an empty graph.

### `(*Graph) Add(o Outlinks)`

Adds one page's links.

- **Counts each page once per target domain.** A page with 50 links to `facebook.com` (a footer repeated on every page, say) counts as 1. Counting raw links would let navigation menus and footers drown out real editorial links.
- A page counts as nofollow for a domain only if **every** link from it to that domain is nofollow.
- **Examples are picked as the smallest URL**, alphabetically, not the first one seen.

**Why "smallest" instead of "first":** the result must be identical no matter what order pages arrive in. Pages arrive in a different order every run when several goroutines parse in parallel, and in a different order again when the distributed workers' outputs are merged in Stage 2. Choosing the minimum makes `Add` order-independent, the same way `+` is. Stage 3's crash test relies on this: a run with crashes must produce byte-identical output to a clean run.

### `(*Graph) Len() int`

Number of edges.

### `(*Graph) Edges() []Edge`

All edges, sorted by `Src` and then `Dst`. Sorting gives a stable, comparable output, since Go deliberately randomizes map iteration order.

### `WriteTSV(w io.Writer, edges []Edge) error`

Writes edges as tab-separated values with a header row. TSV is safe here because tabs can't appear in parsed URLs or domains (Go's URL parser rejects control characters).
