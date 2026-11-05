.PHONY: build test lint fmt up down clean sample sample-edges sample-scores web merge pagerank cc-ranks validate

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

sample-scores: sample-edges
	./bin/pagerank -in data/sample.edges.tsv -o data/sample.scores.tsv -top 0

web: sample-scores
	cd web && npm install && npm run dev

RESULTS ?= data/results

merge: build
	./bin/merge -in $(RESULTS) -o data/merged.edges.tsv.gz

pagerank: build
	./bin/pagerank -in data/merged.edges.tsv.gz -o data/scores.tsv

CC_GRAPH ?= cc-main-2026-aug-sep-oct
CC_TOP ?= 1000000

cc-ranks:
	mkdir -p data
	curl -fsS --retry 8 --retry-delay 15 --retry-all-errors \
		https://data.commoncrawl.org/projects/hyperlinkgraph/$(CC_GRAPH)/domain/$(CC_GRAPH)-domain-ranks.txt.gz \
		| gunzip | head -n $$(( $(CC_TOP) + 1 )) > data/cc-domain-ranks.tsv

validate: build
	./bin/validate -scores data/scores.tsv -ranks data/cc-domain-ranks.tsv
