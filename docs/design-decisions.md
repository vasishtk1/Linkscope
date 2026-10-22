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
