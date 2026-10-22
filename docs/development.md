# Development

## Requirements

- Go 1.26+
- Docker (with Docker Compose)

## Commands

| Command | What it does |
|---|---|
| `make build` | Builds every binary in `cmd/` into `bin/` |
| `make test` | Runs all tests with the race detector on |
| `make lint` | Runs `go vet` and fails if any file isn't `gofmt`-formatted |
| `make fmt` | Formats all Go files |
| `make up` | Builds the Docker image and starts 1 coordinator + 3 workers |
| `make down` | Stops the Docker cluster |
| `make clean` | Deletes `bin/` |
| `make sample` | Downloads the first WAT file of the crawl in `CRAWL` (default `CC-MAIN-2026-43`) to `data/sample.warc.wat.gz` |

## CI

GitHub Actions runs lint, test, and build on every push to `main` and every pull request (`.github/workflows/ci.yml`).

## Local data

Downloaded Common Crawl files and pipeline output go in `data/`, which is ignored by git.
