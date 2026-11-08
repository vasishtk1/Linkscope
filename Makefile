.PHONY: build test lint fmt up down clean sample sample-edges sample-scores sample-index web merge pagerank index api loadtest cc-ranks validate demo

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

sample-index: sample-scores
	./bin/index -edges data/sample.edges.tsv -scores data/sample.scores.tsv -o data/sample-index \
		-source "Sample: 1 Common Crawl file from $(CRAWL) (22,020 pages)"

web: sample-index
	cd web && npm install
	./bin/api -index data/sample-index & API=$$!; trap "kill $$API" EXIT; cd web && npm run dev

RESULTS ?= data/results

merge: build
	./bin/merge -in $(RESULTS) -o data/merged.edges.tsv.gz

pagerank: build
	./bin/pagerank -in data/merged.edges.tsv.gz -o data/scores.tsv

index: build
	./bin/index -edges data/merged.edges.tsv.gz -scores data/scores.tsv -o data/index

INDEX ?= data/index

api: build
	./bin/api -index $(INDEX)

loadtest: build
	./bin/loadtest -scores data/scores.tsv

CC_GRAPH ?= cc-main-2026-aug-sep-oct
CC_TOP ?= 1000000

cc-ranks:
	mkdir -p data
	curl -fsS --retry 8 --retry-delay 15 --retry-all-errors \
		https://data.commoncrawl.org/projects/hyperlinkgraph/$(CC_GRAPH)/domain/$(CC_GRAPH)-domain-ranks.txt.gz \
		| gunzip | head -n $$(( $(CC_TOP) + 1 )) > data/cc-domain-ranks.tsv

validate: build
	./bin/validate -scores data/scores.tsv -ranks data/cc-domain-ranks.tsv

DEMO_FILES ?= 6
DEMO_WORKERS ?= 3

demo: build
	@test -f data/sample.warc.wat.gz || $(MAKE) sample
	rm -rf data/demo && mkdir -p data/demo/files
	for i in $$(seq 1 $(DEMO_FILES)); do \
		ln -s ../../sample.warc.wat.gz data/demo/files/demo-$$i.warc.wat.gz; echo demo-$$i.warc.wat.gz; \
	done > data/demo/paths.txt
	@pids=""; trap 'kill $$pids 2>/dev/null' EXIT; \
	python3 -m http.server 8090 -b 127.0.0.1 -d data/demo/files >/dev/null 2>&1 & pids="$$pids $$!"; \
	./bin/coordinator -db data/demo/jobs.db -paths data/demo/paths.txt -limit 0 & pids="$$pids $$!"; \
	sleep 1; \
	for i in $$(seq 1 $(DEMO_WORKERS)); do \
		./bin/worker -id worker-$$i -source http://127.0.0.1:8090 -out data/demo/results -parsers 1 & pids="$$pids $$!"; \
	done; \
	echo "Watch it at http://localhost:3000/pipeline (start the site with 'make web'). Ctrl+C stops the demo."; \
	wait
