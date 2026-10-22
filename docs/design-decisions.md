# Design decisions

Each entry: what was decided, the alternatives, and why.

## Go for the backend

- **Decision:** the pipeline, coordinator, workers, and API are written in Go.
- **Alternatives:** Python.
- **Why:** goroutines and channels make concurrent download and parse code straightforward, Go parses large JSON files much faster than Python, and it compiles to a single static binary that's easy to ship in Docker. Most modern infrastructure tools (Docker, Kubernetes, etcd) are written in Go.

## WAT files instead of full WARC files

- **Decision:** process Common Crawl WAT files.
- **Alternatives:** WARC (full raw HTML) or WET (plain text only).
- **Why:** WAT files already contain the extracted links for each page. WARC files are several times larger and would require HTML parsing; WET files strip the links entirely.

## Local cluster first

- **Decision:** develop and test with a coordinator and several workers running in Docker on one machine.
- **Alternatives:** start on cloud machines.
- **Why:** free, fast to iterate, and the same code runs unchanged on real machines later. Crashes and network problems can be simulated locally for testing.

## One Docker image for all binaries

- **Decision:** a single image contains the coordinator and worker; `docker-compose.yml` picks which one each container runs.
- **Alternatives:** a separate Dockerfile per binary.
- **Why:** one build, shared code is compiled once, and adding a new binary needs no new Dockerfile. The final image is distroless (no shell or package manager), so it's small and has little attack surface.

## Only clickable links count

- **Decision:** count only `<a href>` and `<area href>` links. Ignore images, scripts, stylesheets, iframes, and forms.
- **Alternatives:** count every URL the page references.
- **Why:** a backlink is one site pointing readers to another. A page loading a font from Google or an image from a CDN isn't recommending those sites. In a real file, clickable links are about 2.9M of the ~4.2M URLs, so counting everything would heavily inflate CDNs and analytics services.

## Count pages, not links

- **Decision:** an edge's weight is the number of **pages** on the source domain linking to the target, with each page counted once per target domain.
- **Alternatives:** count raw links.
- **Why:** site-wide footers and menus repeat the same links on every page. A page with 5 footer links to Facebook would count 5 times under raw counting. Counting pages limits each page to one vote per target site.

## Track nofollow instead of dropping it

- **Decision:** keep nofollow links (`rel="nofollow"`, `sponsored`, `ugc`) and record how many pages link only with nofollow.
- **Alternatives:** drop them, or ignore the attribute.
- **Why:** they're still real backlinks a site owner wants to see, but search engines treat them as weak signals. Keeping the count separate lets PageRank (stage 4) decide how much weight to give them, without re-processing the data.

## Domain = registered domain via the Public Suffix List

- **Decision:** group URLs by registered domain (`blog.nytimes.com` → `nytimes.com`, `bbc.co.uk` stays `bbc.co.uk`) using `golang.org/x/net/publicsuffix`, after IDNA normalization.
- **Alternatives:** group by full hostname, or take the last two labels.
- **Why:** hostnames split one site into many (`www.`, `blog.`, `shop.`). "Last two labels" breaks on `co.uk`-style endings and on platforms like `github.io`, where each subdomain is a different owner. The Public Suffix List handles both. IDNA normalization keeps Unicode and lookalike spellings of the same name from becoming separate domains.

## Order-independent aggregation

- **Decision:** `Graph.Add` gives the same result no matter what order pages arrive in. Counts are sums, and example links are chosen as the alphabetically smallest URL, not the first seen.
- **Alternatives:** keep the first example seen.
- **Why:** parallel parsers deliver pages in a different order every run, and distributed workers will too. Order independence makes output reproducible byte for byte, which the stage 3 crash test needs: a run with crashed workers must produce exactly the same output as a clean run.

## Parallelize parsing within a file

- **Decision:** one reader goroutine feeds a pool of parser goroutines through channels; only the main goroutine updates the graph.
- **Alternatives:** single-threaded; or a shared graph protected by a mutex.
- **Why:** profiling showed decompression is ~0.75s of 7.6s, so JSON parsing dominates and parallelizes cleanly. Keeping the graph in one goroutine avoids locks entirely. Measured speedup: 2.5x at 4 workers. Beyond that, the single reader is the bottleneck, which is why scaling further means processing many files at once (stage 2).
