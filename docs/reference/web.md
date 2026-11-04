# web/

The Linkscope website: Next.js (React, TypeScript), Tailwind CSS, Motion for animations, D3 for the graph physics, lucide-react icons.

Until the real index exists (stages 4–5), the API routes read `data/sample.edges.tsv`, the edges from one Common Crawl file, made with `make sample-edges`. The authority score is shown as "—". Swapping in the real API later doesn't change the page.

```sh
make web    # extracts the sample edges, installs packages, runs localhost:3000
```

## Page flow

1. **Landing:** name, description, and the search box in the middle of the screen, plus example sites to try.
2. **Search:** the URL becomes `/?d=<site>`, so results can be shared and the back button works. The header moves up and the graph builds.
3. **Scroll down:** stats cards and the backlinks table.
4. **Bottom:** "Download backlinks CSV".

## app/

### `page.tsx`

Renders `Linkscope` inside `Suspense`, which Next.js requires for components that read the URL's query string.

### `layout.tsx`, `globals.css`

Fonts (Geist, Geist Mono), page title and description, and the dark theme colors.

### `api/lookup/route.ts` — `GET /api/lookup?domain=`

Returns a `Lookup` as JSON: the domain, score (`null` for now), counts, and every backlink sorted by linking pages. `400` without a domain, `404` when nothing links to it.

### `api/backlinks.csv/route.ts` — `GET /api/backlinks.csv?domain=`

The same backlinks as a CSV download (`linking_domain, pages, nofollow_pages, example_source, example_target`). Built on the server so large sites don't make the browser assemble thousands of rows. `csvField` quotes values containing commas, quotes or newlines.

### `api/favicon/route.ts` — `GET /api/favicon?domain=`

Fetches a site's `/favicon.ico` (then `www.` host), 3s limit each, and caches the result in memory, including misses. Returns `404` when there's no icon so the page shows a letter badge.

- **Only domains from our own data** are fetched. Otherwise anyone could use the server to make requests to arbitrary hosts.
- **Why not Google's favicon service:** it would tell Google every site users look up.

## lib/

### `edges.ts`

- **`loadIndex()`** reads the edge file once per server process and builds two things: backlinks grouped by target domain (sorted by pages), and a set of every known domain.
- **`normalizeInput(input)`** turns what the user typed into a bare host: drops the scheme, path, port, and `www.`.
- **`resolveDomain(host, known)`** drops subdomains until a known domain matches, so `blog.github.com` finds `github.com`. The real API will use the Public Suffix List like the Go code does.
- **`lookup(input)`** resolves the domain and adds up the counts.
- **`isKnownDomain(domain)`** is used by the favicon route.

The path comes from `LINKSCOPE_EDGES`, defaulting to `../data/sample.edges.tsv`.

### `types.ts`

`Backlink` and `Lookup`, the shapes the API returns.

### `badge.ts`

- **`badgeColor(domain)`** hashes the name to a hue, so a site without a favicon always gets the same color.
- **`faviconUrl(domain)`** builds the favicon route URL.

## components/

### `linkscope.tsx` — `Linkscope`

The page. Reads `?d=` from the URL and fetches `/api/lookup`. The page state (idle, loading, found, missing, error) is derived from the URL plus the last answer, rather than stored separately, so it can never fall out of sync with the address bar. Old requests are cancelled with an `AbortController` when the search changes.

### `search-bar.tsx` — `SearchBar`

The search box. It's given `key={query}`, so React resets it when the search in the URL changes, for example with the back button.

### `link-graph.tsx` — `LinkGraph`

The animated graph, drawn on a `<canvas>`.

- **Building live:** backlinks are added strongest first, every 60ms, so the graph grows in front of the user. Each node pops in (`easeOutBack`) and its line grows out from the center (`easeOut`). The center pulses until the graph is complete.
- **"Sites shown" slider:** 10 up to 200 (or however many backlinks exist), default 60. A timer moves the graph toward the slider's value: raising it adds the next-strongest sites, in small batches so big jumps don't take long, and lowering it removes the weakest. The slider writes to a ref that the timer reads, so moving it doesn't restart the graph. The cap is 200 because past that the graph is too cluttered to read; the full list is in the table and CSV.
- **`NodeCount`:** the number next to the slider is also a text box. It shows a border on hover. Click it, type a number, then press Enter or click away to apply it; Escape cancels. Only digits can be typed, and the value is clamped to the slider's range, so typing 999 gives the maximum. While you type, the text is kept in a separate draft, so the graph only changes once you confirm.
- **Layout:** a D3 force simulation. Nodes push each other apart (`forceManyBody`), lines pull them toward the center (`forceLink`, shorter for sites with more pages), and `forceCollide` stops overlaps. The searched site is pinned at the center.
- **Size:** `10 + 4·log2(pages + 1)`, so bigger means more linking pages without huge sites swallowing the screen.
- **Colors:** followed links are solid blue; nofollow-only links are dashed orange.
- **Icons:** each site's favicon is drawn inside its circle, or a colored letter badge if there isn't one.
- **Interaction:** drag a site to move it (`d3-drag`), scroll to zoom, drag the background to pan (`d3-zoom`), hover for a tooltip.
- **Pinning:** a dragged site stays where it's dropped, by keeping its fixed position (`fx`, `fy`) after the drag ends. It gets a small white dot to show it's pinned. Only an actual drag pins a site, not a click. **Reset layout** unpins everything and lets the physics rearrange the graph.
- **Why canvas instead of SVG:** hundreds of moving elements redrawn every frame stay smooth on a canvas; with SVG each one is a DOM element the browser has to update.

### `stat-cards.tsx` — `StatCards`, `CountUp`

Score, linking sites, linking pages, nofollow-only pages. `CountUp` animates each number from 0 the first time it scrolls into view.

### `backlinks-table.tsx` — `BacklinksTable`

First 15 rows with a "Show all" toggle. Each row: favicon (or letter badge), site, pages, link type (Followed, Nofollow, or Mixed), and an example linking page. Example links open with `rel="nofollow"`.

### `download-csv.tsx` — `DownloadCsv`

The download button at the bottom of the page.
