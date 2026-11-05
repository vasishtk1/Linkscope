.PHONY: build test lint fmt up down clean sample sample-edges web merge pagerank

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

sample-edges: build
	./bin/extract -o data/sample.edges.tsv -top 0 data/sample.warc.wat.gz

web: sample-edges
	cd web && npm install && npm run dev

RESULTS ?= data/results

merge: build
	./bin/merge -in $(RESULTS) -o data/merged.edges.tsv.gz

pagerank: build
	./bin/pagerank -in data/merged.edges.tsv.gz -o data/scores.tsv
