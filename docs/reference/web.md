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
4. **How we found this:** how many Common Crawl files the site's links came from, with a **Watch how** button that opens that search's replay on the Pipeline page. Only shown when the index has origins (see [index.md](index.md)).
5. **Bottom:** "Download backlinks CSV".

Every site found is saved in this browser (`searches.ts`), so the Pipeline page can list it under "Your searches".

The **Pipeline** link in the header opens `/pipeline`: press Play to watch a real pipeline run replayed, or watch one live while it's happening (see `pipeline.tsx` below).

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

### `api/pipeline/route.ts` — `GET /api/pipeline`

The live coordinator: its `/status` and its whole `/events` log, combined into `{status, events}` so the page makes one request per update. `503` when no coordinator answers.

### `api/run/route.ts` — `GET /api/run`

The saved run the Go API was started with (`cmd/api -run`, see [record.md](record.md)). `404` if there isn't one, `503` if the API is down.

### `pipeline/page.tsx`

The `/pipeline` page: the shared header (with "Pipeline" marked as the current page), the same blue glow as the home page, and the `Pipeline` component inside `<Suspense>`, because it reads `?search=` from the URL. It doesn't use the animated network background, which would compete with the pipeline animation.

## lib/

### `api.ts`

Talks to the Go lookup API at `LINKSCOPE_API` (default `http://localhost:8081`). It's marked `server-only`, so it can't end up in the browser bundle.

- **`call(path)`** fetches from the API with no caching and a 10s limit. If the API can't be reached it throws `ApiUnavailable`, which the lookup route turns into a `503`.
- **`lookup(input, limit = PAGE_LIMIT)`** passes the user's text straight to `GET /lookup`. The Go API does the cleanup (scheme, `www.`, subdomains), so the rules live in one place. Returns `undefined` on `404`.
- **`isKnownDomain(domain)`** asks `GET /known`, for the favicon route.
- **`network(n)`** asks `GET /network?n=`, for the background.
- **`savedRun()`** asks `GET /run`; `undefined` when there's no saved run.
- **`PAGE_LIMIT` (1,000):** on the 20-file data, facebook.com's full backlink list is 17.7 MB of JSON, too much to send to a browser. The top 1,000 is more than the graph can show legibly. The CSV route passes `0` to get all of them.

### `pipeline.ts`

Talks to the coordinator at `LINKSCOPE_COORDINATOR` (default `http://localhost:8080`). Also `server-only`.

- **`pipeline()`** fetches `/status` and `/events` at the same time (`Promise.all`) and returns both.
- **3s limit, not 10s:** the dashboard asks every second, so a coordinator that isn't there should fail fast and show "No pipeline is running" instead of hanging.
- Throws `CoordinatorUnavailable` when the coordinator can't be reached or answers with an error.

### `types.ts`

`Backlink`, `Origin`, `Lookup` and `Network`, the shapes the lookup API returns (`originFiles`, `origins` and `tookMs` are optional because older indexes have no origins). `PipelineStatus`, `PipelineWorker`, `RunEvent` and `Pipeline` match the coordinator's JSON, and `Recording` matches `cmd/record`'s file.

### `replay.ts`

The replay engine. Plain functions with no React, so the same code drives both the replay and the live view.

- **`makeRun(opts)`** prepares a run: puts every event on one clock (milliseconds), sorts them, and works out, for every claim, when that attempt ended (`closeAt`) and how (`segments`: done, failed, or still open). It also takes the median time a file took (`typicalMs`), used to estimate progress in live mode. `total` is the number of files, `start` and `end` the first and last event (or the coordinator's `now` for a live run).
- **`frameAt(run, t, flightMs)`** answers "what did the pipeline look like at time `t`?" by replaying every event up to `t`: which file each worker holds and how far along it is, which workers went silent ("no contact"), how many files are done, failed or still waiting, and the running totals of pages, links and failed attempts. Events in the last `flightMs` also become **flights**, file tiles moving coordinator → worker (claimed), worker → merge (completed) or worker → queue (failed, expired, released).
- **Why compute each frame from scratch:** the state at any time depends only on the events before it, so the slider can jump anywhere, backwards included, and always shows exactly what happened. Replaying a few hundred events 30 times a second is cheap.
- **Progress bars:** in a replay the end of each attempt is known, so a worker's bar is exact. While live, the end isn't known yet, so the bar estimates from the median file time and stops at 95% until the file actually finishes.
- **`completed` and flight paths:** each frame also lists the files finished so far, and each flight and scrubber segment knows its file, so a search's files can be picked out.
- **`makeFocus(run, search)`** lays a saved search over a run. It matches the search's origins to the run's files by name (`demo-1` from the index is the job `demo-1.warc.wat.gz`). `matched` is how many of the run's files had links to the site. It's 0 when the search came from different data than the saved run, and the page says so instead of highlighting nothing.
- **`focusProgress(focus, frame)`** counts the focused files finished by this frame and the linking pages they've added. At the end of a replay it equals the site's linking pages: on the saved demo run, github.com's two files add up to its 484 pages.
- **`fileLabel`, `fmt`, `clock`:** short file names, numbers with commas, and `m:ss` times.

### `searches.ts`

Remembers the sites you've searched, on this device only, so the Pipeline page can replay each one.

- **Where it's kept:** `localStorage` under `linkscope.searches.v1`. It survives reloads and closing the browser, stays on this device and browser, and is never sent anywhere. Each person's list is their own.
- **`SavedSearch`:** site, score, linking sites and pages, origins (which files the links came from), lookup time, data source, and when it was searched. That's everything the replay needs, so the Pipeline page doesn't have to look the site up again.
- **`saveSearch(lookup)`** adds a search to the top of the list. **`addSearch(list, s)`** does the list part: a repeat search moves to the top instead of showing twice, and the list keeps the newest 20.
- **`parse(raw)`** reads the stored text. Anything broken (not JSON, not a list, rows missing fields) becomes an empty list or is dropped, so a bad value can't break the page.
- **`useSearches()`** gives a component the current list and re-renders it when the list changes, in this tab or another one. It uses React's `useSyncExternalStore`, which is made for reading outside state like storage. The parsed list is cached by its raw text, so React gets the same array until something actually changes.
- **`clearSearches()`** empties the list.
- If storage is blocked (some private windows), saving quietly does nothing and the list stays empty.

Checked with a small Node script: empty, broken and wrong-shape values give an empty list, bad rows are dropped, 25 searches keep the newest 20, and a repeat moves to the top without a duplicate.

### `badge.ts`

- **`badgeColor(domain)`** hashes the name to a hue, so a site without a favicon always gets the same color.
- **`faviconUrl(domain)`** builds the favicon route URL.

## components/

### `site-header.tsx` — `SiteHeader`

The header on both pages: the logo (a link home, which also clears a search) plus "Pipeline" and "GitHub" links. `active="pipeline"` highlights the Pipeline link and sets `aria-current="page"` for screen readers.

### `linkscope.tsx` — `Linkscope`

The page. Reads `?d=` from the URL and fetches `/api/lookup`. The page state (idle, loading, found, missing, error) is derived from the URL plus the last answer, rather than stored separately, so it can never fall out of sync with the address bar. Old requests are cancelled with an `AbortController` when the search changes.

When a site is found, it's saved with `saveSearch`, and `HowWeFound` shows below the table if the answer has origins.

It shows `NetworkBackground` only while there's no search, inside `AnimatePresence`: it fades in over 1.2s and fades out in 0.4s when a search starts. Unmounting it stops its animation loop completely, so it costs nothing while you look at results.

### `network-background.tsx` — `NetworkBackground`

A full-screen canvas behind the landing page that slowly flies forward through the network we measured: the 60 top-scored sites with their favicons (or letter badges) and the links between them, plus 140 small dots for a sense of depth.

**How the 3D works, without a 3D library:** every site has a position `x, y` and a depth `z` (0.35 to 5). Each frame `z` shrinks a little, which is the camera moving forward. A point is drawn at `center + (x / z) × focal`, with a radius of `size × focal × 0.045 / z`. Dividing by `z` is the whole perspective trick: far things are small and close to the middle, and as `z` gets smaller they grow and slide outward. A site that passes the camera or leaves the screen moves back to the far end at a new random spot, so the flight never ends.

- **`place`** puts points on a ring around the middle (never right in front of the camera), so sites fly past the search box instead of through it. The ring is stretched by the screen's width-to-height ratio to fill wide screens.
- **Fading:** `depthAlpha` fades points in from the distance and out just before the camera. `centerAlpha` dims everything near the middle of the screen, so the title and search box stay readable.
- **Size:** bigger scores get slightly bigger icons (0.7× to 1.3×).
- **Lines:** a link is drawn between two sites when either one is on screen, at most 35% as bright as the fainter end.
- **Drawing order:** far sites first, so close ones cover them. Tiny far sites (under 6px) are plain dots, which skips the costly clipped image.
- **No mouse effect:** the flight ignores the pointer and just keeps moving at the same pace.

**Keeping it light on the laptop:**

| Guard | What it does |
|---|---|
| 30 frames per second | Frames that come sooner are skipped; that's smooth for slow movement and about half the work of 60 |
| Pixel ratio capped at 1.5 | Retina screens would otherwise draw 4× the pixels |
| Hidden tab | The loop stops on `visibilitychange` and restarts when you come back |
| Reduced motion | If the system asks for less motion, one still frame is drawn (and redrawn as icons arrive), with no loop at all |
| After a search | The component is unmounted, so nothing runs |

Icons come through `/api/favicon`, the same cached route the graph uses, so only sites from our own data are fetched.

### `pipeline.tsx` — `Pipeline`

The pipeline page. It picks what to show, in this order:

1. **A live run**, if the coordinator says files are still waiting or being processed. The scene follows it in real time.
2. **The run on the coordinator**, once a live run you were watching finishes, so it doesn't jump away. It can be replayed like a saved one.
3. **The saved run** from `/api/run`. This is what visitors to a hosted site see.
4. Otherwise, how to make one: `make demo`, `make demo-save`, `make web`.

When both a saved run and a finished coordinator run exist, a switch above the scene picks between them.

**Your searches.** On the left (a row of buttons on phones) are "Full run" and every site searched on this device (`useSearches`), newest first, with its file count and score. "Clear" empties the list. Picking one puts `?search=<site>` in the URL and replays the saved run focused on that site:

- **`FocusBar`** replaces the source line: "Following github.com · its links were found in 2 of 2 files", a note if the index capped the list (it keeps the 200 files with the most links), or a warning if the search came from different data. "Show full run" goes back.
- **In the scene** the site's files stay blue and the rest fade (see `pipeline-scene.tsx`). The scrubber greys out bars for other files.
- **`Counters`** gets an extra first tile: links to the site so far out of its total, and how many of its files are in.
- A picked search always replays the saved run, even while a live run is going, because the search's answer came from the saved run's index.

The page reads `?search=` with `useSearchParams`, so `pipeline/page.tsx` wraps it in `<Suspense>`, which Next.js requires for that.

- **`useLive`** polls `/api/pipeline` every second, or every 4 seconds while no coordinator answers, so a new run appears on its own. It stops while the tab is hidden, never lets two requests overlap, and remembers if it ever saw a run in progress.
- **`useRecording`** fetches the saved run once.
- **`Replay`** holds the replay clock `t`. **Play** moves it forward 30 times a second; **Pause** stops the animation loop completely, so a paused page does no work. The page opens paused at the start with a big Play button over the scene. Speeds: *Fit to 30s* (the default, so a 3-second demo and a 2-minute run both take 30s), 1×, 10× and 60×. File tiles take 0.7s of screen time to move, whatever the speed. Hiding the tab pauses it.
- **`LiveView`** shows a run as it happens. The coordinator's clock comes with each update; between updates the page moves it forward with the browser's timer, so tiles glide instead of jumping once a second.
- **`Scrubber`** is the slider: one thin row per worker and one bar per file attempt (blue processed, red failed), with the playhead on top. Click or drag to jump; arrow keys step by 2%.
- **`Counters`** below the scene: files done, pages read, links found, site pairs (per file, before merging), and failed attempts (each one retried, or how many gave up).
- **Derived runs:** a recording made from a database older than the event log says so, because its retries weren't recorded.

### `pipeline-scene.tsx` — `PipelineScene`

Draws one frame as SVG, left to right:

| Part | Shows |
|---|---|
| Common Crawl files | One small tile per waiting file (up to 72, then "+N more") |
| Coordinator | The node that hands files out and retries failures |
| Workers | One box per worker: its name, the file it's on, a progress bar, and its state (working, idle, retry with the attempt number, or "no contact" with a red border) |
| Merge, PageRank, Index | Merge counts files as they arrive. PageRank and Index light up once every file is in, labeled as the steps that run next; their timings aren't in the recording, so no numbers are shown for them |
| Flying tiles | Files moving between boxes along straight lines with ease-in-out; red when a failed file goes back to the queue |

**With a search picked** (`focus`):

| Part | Shows |
|---|---|
| Waiting files | The site's files still waiting are blue at the front of the queue, the rest dark. Tiles stand for counts, not exact queue positions |
| Workers | A worker holding one of the site's files gets a bright border and "+N links" instead of its state; the others fade to 40% |
| Flying tiles | The site's files blue, the rest grey. When one reaches Merge, a "+N" floats up and fades |
| Merge | Links to the site merged so far |
| PageRank | The site's score once every file is in |
| Index | How long your search's lookup took inside the API (`tookMs`, measured when you searched) |

The SVG has a fixed 1000-wide coordinate system and scales to the page width. Its height grows with the number of workers.

### `search-bar.tsx` — `SearchBar`

The search box. It's given `key={query}`, so React resets it when the search in the URL changes, for example with the back button.

### `link-graph.tsx` — `LinkGraph`

The animated graph, drawn on a `<canvas>`.

- **Building live, without bumping:** new sites are laid out first, and drawn only once the layout is done. Then each one glides out from the center to its spot (`easeOut`) and pops to full size (`easeOutBack`), strongest first, a few dozen ms apart (the whole build takes at most about 1.5s). The center pulses until every site is showing.
- **Why lay out first:** the old version added sites in about 20 batches, dropped each batch next to the center and restarted the physics every time, so every dot got shoved around 20 times before it settled. Now the physics runs once per change, out of sight, and nothing moves after a dot lands.
- **Never freezing the page:** the layout runs a few steps per animation frame (`LAYOUT_BUDGET_MS`, 10ms), not all at once. Measured with the same settings outside the browser: 60 sites need about 60ms of layout work in total, 1,000 sites about 1.6s, so doing it in one go would freeze the page at large sizes. New sites start at their golden-angle spot (`GOLDEN_ANGLE`) roughly the right distance from the center, so the layout has less to fix.
- **"Sites shown" slider:** 0 up to every loaded linking site (at most 1,000), default 60 (or all of them if there are fewer). At 0 only the searched site is shown. The graph updates 150ms after the slider stops moving (`SLIDER_WAIT_MS`), so dragging it doesn't redo the layout on every step. Raising it lays out and adds the next-strongest sites while the ones already showing stay put; lowering it removes the weakest. There's no cap, so very large numbers (thousands of sites) get crowded.
- **`NodeCount`:** the number next to the slider is also a text box. It shows a border on hover. Click it, type a number, then press Enter or click away to apply it; Escape cancels. Only digits can be typed, and the value is clamped to the slider's range, so typing more than the number of linking sites gives all of them, and 0 shows only the searched site. While you type, the text is kept in a separate draft, so the graph only changes once you confirm.
- **Layout:** a D3 force simulation. Nodes push each other apart (`forceManyBody`), lines pull them toward the center (`forceLink`, shorter for sites with more pages), and `forceCollide` stops overlaps. The searched site is pinned at the center. `velocityDecay(0.55)` (D3's default is 0.4) adds friction so movement dies down quickly.
- **Size:** `10 + 4·log2(pages + 1)`, so bigger means more linking pages without huge sites swallowing the screen.
- **Colors:** followed links are solid blue; nofollow-only links are dashed orange.
- **Icons:** each site's favicon is drawn inside its circle, or a colored letter badge if there isn't one.
- **Interaction:** drag a site to move it (`d3-drag`), scroll to zoom, drag the background to pan (`d3-zoom`), hover for a tooltip.
- **Pinning:** a dragged site stays where it's dropped, by keeping its fixed position (`fx`, `fy`) after the drag ends. It gets a small white dot to show it's pinned. Only an actual drag pins a site, not a click. While dragging, the physics runs gently (`alphaTarget(0.1)`), so only nearby sites shift. **Reset layout** unpins everything and lets the physics rearrange the graph gently.
- **Why canvas instead of SVG:** hundreds of moving elements redrawn every frame stay smooth on a canvas; with SVG each one is a DOM element the browser has to update.

### `stat-cards.tsx` — `StatCards`, `CountUp`

Score, linking sites, linking pages, nofollow-only pages. `CountUp` animates each number from 0 the first time it scrolls into view. The score shows as "58 / 100", or "—" when there's no score.

**Why "linking sites" can be higher than the PageRank count:** the card counts every site that links, including nofollow-only ones. PageRank's `linking_domains` leaves those out, since they pass no score.

### `backlinks-table.tsx` — `BacklinksTable`

First 15 rows with a toggle to show every loaded row ("Show all", or "Show top 1,000" when the site has more, with a note that the CSV has the rest). Each row: favicon (or letter badge), site, pages, link type (Followed, Nofollow, or Mixed), and an example linking page. Example links open with `rel="nofollow"`.

### `download-csv.tsx` — `DownloadCsv`

The download button at the bottom of the page.
