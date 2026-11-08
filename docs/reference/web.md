# web/

The Linkscope website: Next.js (React, TypeScript), Tailwind CSS, Motion for animations, D3 for the graph physics, lucide-react icons.

The website doesn't read data files. Its API routes ask the Go lookup API (`cmd/api`, see [serve.md](serve.md)), which serves the sharded index.

```sh
make web    # builds the sample index, starts the Go API on :8081, runs the website on localhost:3000
```

`make web` uses the sample index (one Common Crawl file). To browse a bigger run, start the API on that index and point the website at it:

```sh
./bin/api -index data/index -addr :8082
cd web && LINKSCOPE_API=http://localhost:8082 npm run dev
```

## Page flow

1. **Landing:** name, description, and the search box in the middle of the screen, plus example sites to try. Behind them, the top sites in the index drift toward you with their real icons and the links between them (`NetworkBackground`).
2. **Search:** the URL becomes `/?d=<site>`, so results can be shared and the back button works. The header moves up and the graph builds.
3. **Scroll down:** stats cards and the backlinks table.
4. **Bottom:** "Download backlinks CSV".

## app/

### `page.tsx`

Renders `Linkscope` inside `Suspense`, which Next.js requires for components that read the URL's query string.

### `layout.tsx`, `globals.css`

Fonts (Geist, Geist Mono), page title and description, and the dark theme colors.

### `api/lookup/route.ts` — `GET /api/lookup?domain=`

Returns a `Lookup` as JSON: the domain, its 0–100 authority score (`null` if the index has no scores), counts, and the top 1,000 backlinks (`PAGE_LIMIT`) sorted by linking pages. The counts always cover every backlink. `400` without a domain, `404` when nothing links to it, `503` if the Go API can't be reached.

### `api/backlinks.csv/route.ts` — `GET /api/backlinks.csv?domain=`

Every backlink as a CSV download (`linking_domain, pages, nofollow_pages, example_source, example_target`), with no 1,000 limit. Built on the server so large sites don't make the browser assemble thousands of rows. `csvField` quotes values containing commas, quotes or newlines.

### `api/favicon/route.ts` — `GET /api/favicon?domain=`

Fetches a site's `/favicon.ico` (then `www.` host), 3s limit each, and caches the result in memory, including misses. Returns `404` when there's no icon so the page shows a letter badge.

- **Only domains from our own data** are fetched. Otherwise anyone could use the server to make requests to arbitrary hosts.
- **Why not Google's favicon service:** it would tell Google every site users look up.

### `api/network/route.ts` — `GET /api/network`

The top 60 sites and the links among them, from the Go API's `/network?n=60`, for the landing page background. Browsers may cache it for an hour (`Cache-Control: public, max-age=3600`), since it only changes when the index is rebuilt. `503` if the Go API can't be reached; the background then shows only its drifting dots.

## lib/

### `api.ts`

Talks to the Go lookup API at `LINKSCOPE_API` (default `http://localhost:8081`). It's marked `server-only`, so it can't end up in the browser bundle.

- **`call(path)`** fetches from the API with no caching and a 10s limit. If the API can't be reached it throws `ApiUnavailable`, which the lookup route turns into a `503`.
- **`lookup(input, limit = PAGE_LIMIT)`** passes the user's text straight to `GET /lookup`. The Go API does the cleanup (scheme, `www.`, subdomains), so the rules live in one place. Returns `undefined` on `404`.
- **`isKnownDomain(domain)`** asks `GET /known`, for the favicon route.
- **`network(n)`** asks `GET /network?n=`, for the background.
- **`PAGE_LIMIT` (1,000):** on the 20-file data, facebook.com's full backlink list is 17.7 MB of JSON, too much to send to a browser. The top 1,000 is more than the graph can show legibly. The CSV route passes `0` to get all of them.

### `types.ts`

`Backlink`, `Lookup` and `Network`, the shapes the API returns.

### `badge.ts`

- **`badgeColor(domain)`** hashes the name to a hue, so a site without a favicon always gets the same color.
- **`faviconUrl(domain)`** builds the favicon route URL.

## components/

### `linkscope.tsx` — `Linkscope`

The page. Reads `?d=` from the URL and fetches `/api/lookup`. The page state (idle, loading, found, missing, error) is derived from the URL plus the last answer, rather than stored separately, so it can never fall out of sync with the address bar. Old requests are cancelled with an `AbortController` when the search changes.

It shows `NetworkBackground` only while there's no search, inside `AnimatePresence`: it fades in over 1.2s and fades out in 0.4s when a search starts. Unmounting it stops its animation loop completely, so it costs nothing while you look at results.

### `network-background.tsx` — `NetworkBackground`

A full-screen canvas behind the landing page that slowly flies forward through the network we measured: the 60 top-scored sites with their favicons (or letter badges) and the links between them, plus 140 small dots for a sense of depth.

**How the 3D works, without a 3D library:** every site has a position `x, y` and a depth `z` (0.35 to 5). Each frame `z` shrinks a little, which is the camera moving forward. A point is drawn at `center + (x / z) × focal`, with a radius of `size × focal × 0.045 / z`. Dividing by `z` is the whole perspective trick: far things are small and close to the middle, and as `z` gets smaller they grow and slide outward. A site that passes the camera or leaves the screen moves back to the far end at a new random spot, so the flight never ends.

- **`place`** puts points on a ring around the middle (never right in front of the camera), so sites fly past the search box instead of through it. The ring is stretched by the screen's width-to-height ratio to fill wide screens.
- **Fading:** `depthAlpha` fades points in from the distance and out just before the camera. `centerAlpha` dims everything near the middle of the screen, so the title and search box stay readable.
- **Size:** bigger scores get slightly bigger icons (0.7× to 1.3×).
- **Lines:** a link is drawn between two sites when either one is on screen, at most 35% as bright as the fainter end.
- **Drawing order:** far sites first, so close ones cover them. Tiny far sites (under 6px) are plain dots, which skips the costly clipped image.
- **Mouse:** moving the pointer shifts the camera a little, and it eases toward the new spot. Close sites move more than far ones (parallax), which makes the depth easier to see.

**Keeping it light on the laptop:**

| Guard | What it does |
|---|---|
| 30 frames per second | Frames that come sooner are skipped; that's smooth for slow movement and about half the work of 60 |
| Pixel ratio capped at 1.5 | Retina screens would otherwise draw 4× the pixels |
| Hidden tab | The loop stops on `visibilitychange` and restarts when you come back |
| Reduced motion | If the system asks for less motion, one still frame is drawn (and redrawn as icons arrive), with no loop at all |
| After a search | The component is unmounted, so nothing runs |

Icons come through `/api/favicon`, the same cached route the graph uses, so only sites from our own data are fetched.

### `search-bar.tsx` — `SearchBar`

The search box. It's given `key={query}`, so React resets it when the search in the URL changes, for example with the back button.

### `link-graph.tsx` — `LinkGraph`

The animated graph, drawn on a `<canvas>`.

- **Building live:** backlinks are added strongest first, every 60ms, so the graph grows in front of the user. Each node pops in (`easeOutBack`) and its line grows out from the center (`easeOut`). The center pulses until the graph is complete.
- **"Sites shown" slider:** 0 up to every loaded linking site (at most 1,000), default 60 (or all of them if there are fewer). At 0 only the searched site is shown. A timer moves the graph toward the slider's value: raising it adds the next-strongest sites, in batches of about 1/20th of the gap so big jumps don't take long, and lowering it removes the weakest. The slider writes to a ref that the timer reads, so moving it doesn't restart the graph. There's no cap, so very large numbers (thousands of sites) get crowded and can slow the animation down.
- **`NodeCount`:** the number next to the slider is also a text box. It shows a border on hover. Click it, type a number, then press Enter or click away to apply it; Escape cancels. Only digits can be typed, and the value is clamped to the slider's range, so typing more than the number of linking sites gives all of them, and 0 shows only the searched site. While you type, the text is kept in a separate draft, so the graph only changes once you confirm.
- **Layout:** a D3 force simulation. Nodes push each other apart (`forceManyBody`), lines pull them toward the center (`forceLink`, shorter for sites with more pages), and `forceCollide` stops overlaps. The searched site is pinned at the center.
- **Size:** `10 + 4·log2(pages + 1)`, so bigger means more linking pages without huge sites swallowing the screen.
- **Colors:** followed links are solid blue; nofollow-only links are dashed orange.
- **Icons:** each site's favicon is drawn inside its circle, or a colored letter badge if there isn't one.
- **Interaction:** drag a site to move it (`d3-drag`), scroll to zoom, drag the background to pan (`d3-zoom`), hover for a tooltip.
- **Pinning:** a dragged site stays where it's dropped, by keeping its fixed position (`fx`, `fy`) after the drag ends. It gets a small white dot to show it's pinned. Only an actual drag pins a site, not a click. **Reset layout** unpins everything and lets the physics rearrange the graph.
- **Why canvas instead of SVG:** hundreds of moving elements redrawn every frame stay smooth on a canvas; with SVG each one is a DOM element the browser has to update.

### `stat-cards.tsx` — `StatCards`, `CountUp`

Score, linking sites, linking pages, nofollow-only pages. `CountUp` animates each number from 0 the first time it scrolls into view. The score shows as "58 / 100", or "—" when there's no score.

**Why "linking sites" can be higher than the PageRank count:** the card counts every site that links, including nofollow-only ones. PageRank's `linking_domains` leaves those out, since they pass no score.

### `backlinks-table.tsx` — `BacklinksTable`

First 15 rows with a toggle to show every loaded row ("Show all", or "Show top 1,000" when the site has more, with a note that the CSV has the rest). Each row: favicon (or letter badge), site, pages, link type (Followed, Nofollow, or Mixed), and an example linking page. Example links open with `rel="nofollow"`.

### `download-csv.tsx` — `DownloadCsv`

The download button at the bottom of the page.
