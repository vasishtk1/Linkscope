# internal/serve

The HTTP API that answers backlink lookups from the index (stage 5). `cmd/api` opens the index and serves this handler. The website calls it instead of reading data files itself.

```
browser → website (Next.js, :3000) → Go API (:8081) → index shards on disk
```

**Why a separate Go server:** the index format, hashing, and lookup logic already live in Go. Keeping lookups in one Go program means there's one implementation to test and load-test. It also means the API can later run on its own machine, or one per shard, without touching the website.

## Routes

| Route | Answer |
|---|---|
| `GET /lookup?domain=github.com&limit=500` | `200` with the lookup JSON below. `limit` is optional (0 or missing = every backlink) |
| `GET /known?domain=github.com` | `200 {"known":true}` if the exact domain is in the index (with or without backlinks), else `404` |
| `GET /network?n=60` | `200` with the `n` highest-scored sites and the links among them, for the landing page background. `n` is 1 to 200 (default 60) |
| `GET /stats` | `200` with the index manifest: shards, hash, domain and edge counts, data description |
| `GET /healthz` | `200 ok` |

Lookup JSON:

```json
{
  "domain": "github.com",
  "score": 58,
  "linkingDomains": 221,
  "linkingPages": 242,
  "nofollowPages": 18,
  "backlinks": [
    {"domain": "mozilla.org", "pages": 4, "nofollowPages": 0,
     "exampleSource": "https://support.mozilla.org/de/questions/1578885",
     "exampleTarget": "https://github.com/mozilla/kitsune/"}
  ],
  "source": "Sample: 1 Common Crawl file from CC-MAIN-2026-43 (22,020 pages)"
}
```

`score` is `null` when the index was built without scores. The totals (`linkingDomains`, `linkingPages`, `nofollowPages`) always count every backlink, even when `limit` cuts the list short.

Network JSON (`/network?n=3`):

```json
{
  "nodes": [{"domain": "facebook.com", "score": 100}, {"domain": "instagram.com", "score": 95}, {"domain": "google.com", "score": 93}],
  "links": [{"source": "google.com", "target": "facebook.com", "pages": 1},
            {"source": "google.com", "target": "instagram.com", "pages": 2}]
}
```

Errors are JSON too, `{"error": "…"}`:

| Status | When |
|---|---|
| `400` | `domain` missing or blank, `limit` isn't a whole number of 0 or more, or `n` isn't 1 to 200 |
| `404` | Nothing links to the site (also for unknown sites) |
| `405` | Wrong method, e.g. `POST` (Go's router handles this) |
| `500` | Reading a shard failed; the details are logged, not sent |

## serve.go

### `Handler(ix *index.Index) http.Handler`

Builds the router with Go's method-and-path patterns (`"GET /lookup"`). Every route only reads from the index. Directories are never changed after `Open`, and `ReadAt` is safe to call from many goroutines at once, so Go's HTTP server runs every request on its own goroutine with no locks.

**Why `limit`:** a big site's full backlink list is large. On the 20-file data, facebook.com's complete answer is 17.7 MB of JSON. The backlinks are stored sorted by linking pages, so the top N are just the first N lines of the list.

**Why `/network` is cached:** building it scans every directory and reads whole backlink lists (see `index.md`). `sync.OnceValues` runs `ix.Network(200)` the first time anyone asks, and every later request reuses that answer and cuts it down with `Top(n)`. `sync.OnceValues` also makes sure that if many requests arrive at once before it's ready, only one does the work and the others wait for it. The answer never changes because the index never changes while the server runs.

### `parseLimit(s string) (int, error)`

Empty means 0 (all). Anything that isn't a whole number of 0 or more is an error.

### `errorBody(msg string) map[string]string`

`{"error": msg}`.

### `writeJSON(w http.ResponseWriter, status int, v any)`

Sets the JSON content type, writes the status, and encodes `v`.

## Tests

| Test | Checks |
|---|---|
| `TestLookup` | Full result from a messy URL: domain, score, totals, order, data description |
| `TestLookupLimit` | `limit=1` returns one backlink; totals still count all |
| `TestLookupErrors` | Missing or blank domain, bad or negative limit → 400; no backlinks or unknown → 404; `POST` → 405 |
| `TestKnown` | Exact domains only: a link-only domain is known, a subdomain or a made-up site isn't |
| `TestNetwork` | Default size, an empty link list is `[]` not `null`, bad `n` → 400 |
| `TestStatsAndHealth` | Manifest and health check |
| `TestParallelLookups` | 50 lookups at once, all correct (run with the race detector) |
