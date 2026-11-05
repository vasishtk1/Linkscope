# Roadmap

| Stage | Goal | Status |
|---|---|---|
| 0 | Setup: repo structure, docs, Docker, CI | ✅ Done |
| 1 | Single-file extractor: parse one WAT file into domain-to-domain links | ✅ Done |
| 2 | Distributed workers: coordinator, job leases, retries, per-job output | ✅ Done |
| 3 | Chaos test: kill workers mid-run; output must match a clean run | Skipped for now |
| 4 | Merge + PageRank; validate against Common Crawl's published host ranks | ✅ Done |
| 5 | Sharded index + API; load test | In progress (index done) |
| 6 | Web app: search, score, link graph, pipeline dashboard | In progress (search, score, graph done; dashboard to do) |
| 7 | Scale run + deploy | |

## Future

- GitHub App that opens pull requests to fix SEO problems found with the backlink index (redirects for dead pages that still receive backlinks, broken outbound links)
