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

## SQLite for the job list

- **Decision:** the coordinator stores jobs in a single SQLite file, through the pure-Go driver `modernc.org/sqlite`.
- **Alternatives:** PostgreSQL; keeping jobs in memory; a queue like Redis or SQS.
- **Why:** the job list must survive a coordinator crash, so it has to be on disk. SQLite gives real transactions with no server to run. One coordinator writing a few rows per minute is nowhere near its limits. The pure-Go driver needs no C compiler, so the binary stays static and fits the distroless image. Postgres would be the next step if several coordinators had to share the list.

## Leases instead of "assigned" flags

- **Decision:** a job is handed out with a 60-second lease that the worker renews every 20 seconds. If renewals stop, the job goes back in the queue.
- **Alternatives:** mark a job as assigned forever; have the coordinator ping workers.
- **Why:** the coordinator can't tell a crashed worker from a slow one, and a crashed worker can't announce that it crashed. A lease turns "has this worker died?" into "has it checked in recently?", which the coordinator can answer alone. Renewing at a third of the lease means two heartbeats in a row can be lost before the job is taken away.

## The attempt number is the lease's ID

- **Decision:** every heartbeat, failure, and release must include the attempt number the worker was given. Calls with an old attempt number are rejected with `409 Conflict`.
- **Alternatives:** identify leases by worker name.
- **Why:** a worker that freezes (a long pause or a network blip) can come back after its job was given to someone else. Its old attempt number no longer matches, so it can't extend, fail, or release the new owner's lease. When the worker sees the 409, it cancels its own download and moves on.

## Make re-running a job harmless (idempotent results)

- **Decision:** each job always writes to the same file (`<wat name>.edges.tsv.gz`), and that file is written to a temp file, synced, and atomically renamed into place. Completing a job that's already done changes nothing.
- **Alternatives:** prevent duplicate runs entirely (exactly-once delivery).
- **Why:** with leases, the same job will sometimes run twice: a slow worker gets replaced but finishes anyway. Preventing that completely isn't possible when machines and networks can fail, because the coordinator can never be sure whether a silent worker is dead or just slow. Instead, a duplicate run produces the same bytes at the same path, so it doesn't matter. Extraction is deterministic (see "Order-independent aggregation"), and gzip output carries no timestamp. The atomic rename means anything reading the results sees either no file or a complete one.

## Release on shutdown doesn't count as an attempt

- **Decision:** a worker stopped with Ctrl+C or `docker compose down` hands its job back with `Release`, which undoes the attempt count.
- **Alternatives:** let the lease expire; report it as a failure.
- **Why:** waiting for the lease wastes up to a minute. Counting it as a failure means stopping the cluster three times could mark a perfectly good file as `failed`.

## Retry policy

- **Decision:** downloads retry in the worker with exponential backoff (1s, 2s, 4s) on network errors, `429`, and `5xx`, but not on other `4xx`. Whole jobs get 3 attempts through the coordinator. Workers retry the coordinator itself indefinitely, with backoff capped at 30s.
- **Why:** these are different kinds of failure. A `503` from Common Crawl is usually gone in seconds, so a quick local retry is cheapest. A `404` won't fix itself, so retrying wastes time. A file that's corrupt fails the same way every time, so after 3 tries it's marked failed instead of looping forever. The coordinator being down is temporary by definition (it's restarting), so workers wait for it.

## Time limits on every network call

- **Decision:** every request to the coordinator has a 20s limit. A download must start within 30s, and is cancelled if no data arrives for 20s. The coordinator's HTTP server also drops requests that take more than 20s to read or answer.
- **Alternatives:** no limits (Go's default); one total limit per download.
- **Why:** a crash is easy to handle because the connection fails right away. A freeze is worse: the connection stays open and, without a limit, the other side waits forever. A time limit turns a freeze into an ordinary error that the existing retry logic already handles. Downloads use a "no data for 20s" limit instead of a total one, because healthy files take anywhere from ~8s to over 30s depending on the connection, so a total limit would either kill normal downloads or be too long to catch a freeze. The stall timer only runs while the worker is waiting on the network, so a busy extractor never counts as a stall. A stalled download is retried inside the job, since it's a network problem, not a problem with the file.

## Stream downloads straight into the extractor

- **Decision:** a worker reads the HTTP response directly into `extract.Run`, without saving the file first.
- **Alternatives:** download to disk, then process.
- **Why:** no 150 MB temp file per job, and parsing starts as soon as the first bytes arrive, overlapping with the download. The trade-off: a dropped or stalled connection mid-file means re-downloading from the start. At ~150 MB per file that's a few seconds, so resuming partial downloads isn't worth the complexity.

## Gzip the result files

- **Decision:** results are written as `.edges.tsv.gz`.
- **Why:** each file's edges are ~21 MB as plain text and about 3.5 MB gzipped, so 100,000 files would be ~2 TB plain versus ~350 GB compressed. The merge step in stage 4 reads them as a stream, so compression costs very little.
