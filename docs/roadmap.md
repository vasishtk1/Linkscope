# Roadmap

| Stage | Goal | Status |
|---|---|---|
| 0 | Setup: repo structure, docs, Docker, CI | ✅ Done |
| 1 | Single-file extractor: parse one WAT file into domain-to-domain links | ✅ Done |
| 2 | Distributed workers: coordinator, job leases, retries, per-job output | ✅ Done |
| 3 | Chaos test: kill workers mid-run; output must match a clean run | Skipped for now |
| 4 | Merge + PageRank; validate against Common Crawl's published host ranks | ✅ Done |
| 5 | Sharded index + API; load test | ✅ Done |
| 6 | Web app: search, score, link graph, pipeline dashboard | ✅ Done |
| 7 | Scale run + deploy | |

## Future

- GitHub App that opens pull requests to fix SEO problems found with the backlink index (redirects for dead pages that still receive backlinks, broken outbound links)
  - Phase 1 done: `cmd/linkcheck` finds a site's most linked pages that are broken (404/410) or redirect to the homepage, respecting robots.txt ([reference](reference/linkcheck.md))
  - Next: pick a redirect target for each broken page, edit the site's redirect file, open the pull request
