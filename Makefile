.PHONY: build test lint fmt up down clean sample

build:
	go build -o bin/ ./cmd/...

test:
	go test -race ./...

lint:
	go vet ./...
	@test -z "$$(gofmt -l .)" || (echo "Run 'make fmt' on:"; gofmt -l .; exit 1)

fmt:
	gofmt -w .

up:
	docker compose up --build

down:
	docker compose down

clean:
	rm -rf bin/

CRAWL ?= CC-MAIN-2026-43

sample:
	mkdir -p data
	curl -fsSL https://data.commoncrawl.org/crawl-data/$(CRAWL)/wat.paths.gz | gunzip | head -1 \
		| xargs -I{} curl -fSL -o data/sample.warc.wat.gz https://data.commoncrawl.org/{}
