# internal/linkcheck

Checks whether a site's most linked pages still work. It's phase 1 of the pull request agent: before the agent can add a redirect for a broken page, it has to find pages that are broken but still have other sites linking to them. The list of pages comes from the index (each site's top 200 most linked pages, see [index.md](index.md)). This package only checks them live. No AI, no GitHub.

## What counts as broken

| Result | When | Shown as |
|---|---|---|
| `broken` | the page answers **404 Not Found** or **410 Gone**, including after following redirects | `broken (404)` |
| `homepage` | a deep page (not `/`) redirects to the site's homepage. Sites often do this instead of showing a 404, so the old page is probably gone (a "soft 404") | `redirects to the homepage` |
| `ok` | any 2xx answer, after redirects | `ok`, or `ok, redirects to /new-page` |
| `unknown` | server errors (5xx), blocked (401, 403, 429, 999), timeouts, too many redirects, domain not found, connection failed | `couldn't check (timeout)` |
| `skipped` | the site's robots.txt asks bots not to visit the page, or robots.txt itself is unavailable | `skipped (robots.txt)` |

**Only 404 and 410 count as broken.** A page that's down for a moment (503), blocked for bots (403) or slow (timeout) is reported as `unknown`, never as broken. Otherwise the agent would open pull requests for pages that work fine. Being wrong in that direction is much worse than missing a broken page.

## How a page is checked

```
page from the index: "example.com/old-pricing"
   │
   ├─ first page on this host? read robots.txt once (https, else http). That also decides the scheme
   ├─ robots.txt blocks the path? → skipped
   │
   ├─ HEAD https://example.com/old-pricing     (status only, no page body)
   │     2xx → trust it
   │     4xx/5xx → ask again with GET           (some servers answer HEAD wrongly)
   │
   └─ classify the final status after up to 5 redirects
```

**Why HEAD first, then GET:** HEAD asks for just the status, so it's cheap for both sides. But some servers don't support HEAD (405) or answer it wrongly. Some return 404 for HEAD while the page loads fine with GET. So a page is only called broken after a GET confirms it. The GET reads at most 1 KB of the body and then hangs up.

## Being polite

- **One request per `Interval`** (1 second by default) across everything the checker does, robots.txt included. A shared `time.Ticker` hands out turns, and every request waits for one.
- **At most 2 requests in flight** (`-workers`), so a slow page doesn't hold up the rest, but also never more than 1 new request per second.
- **10-second time limit** per request, redirects included.
- **An honest name:** every request sends `User-Agent: LinkscopeBot/0.1 (+https://github.com/vasishtk1/Linkscope)`, so a site owner can see who's visiting and why.
- **robots.txt is respected**, following the standard ([RFC 9309](https://www.rfc-editor.org/rfc/rfc9309)):
  - A 4xx answer (no robots.txt) means everything is allowed.
  - A 5xx or broken answer means "assume nothing is allowed", so the host's pages are skipped.

## robots.go

### `Robots`

The rules from one robots.txt that apply to us. A zero `Robots` (`allowAll`) allows everything.

### `ParseRobots(text, token string) *Robots`

A robots.txt is made of groups: one or more `User-agent:` lines, then `Allow:` and `Disallow:` rules for those bots. This keeps the rules from groups naming our bot (`linkscopebot`, matched without caring about upper or lower case). If there are none, it keeps the `*` group, meaning every bot.

- A group for us replaces the `*` group completely, even an empty one. `User-agent: linkscopebot` followed by an empty `Disallow:` lets us in when `*` is blocked.
- `#` starts a comment. Unknown lines (`Sitemap:`, `Crawl-delay:`) are ignored. Rules before the first `User-agent:` belong to no group.
- An empty `Disallow:` means "nothing is disallowed", so it adds no rule.
- An HTML page served as robots.txt (some sites do this) has no rule lines, so it allows everything.

### `(r *Robots) Allowed(path string) bool`

The **longest** matching rule wins. If an `Allow` and a `Disallow` are the same length, `Allow` wins. No match means allowed. So `Disallow: /a` with `Allow: /a/b` blocks `/a/c` but allows `/a/b/c`.

### `match(pattern, path string) bool`

Prefix match with robots.txt's two special characters: `*` means any text, and `$` at the end means the path must end there. `/*.pdf$` matches `/docs/file.pdf` but not `/docs/file.pdfx`. The pattern is split on `*`: the first piece must start the path, each later piece is found in order, and with `$` the last piece must be at the very end.

## check.go

### `Result`, `Check`

`Result` is one of `ok`, `broken`, `homepage`, `unknown`, `skipped`. `Check` is the outcome for one page:

- `Page`: the index key, host + path with no scheme.
- `URL`: what was requested.
- `Status`: the final HTTP status, 0 if there was no answer.
- `FinalURL`: where redirects ended, only if different.
- `Reason`: why it's `unknown` or `skipped`.

It turns into JSON for the next phase (picking redirect targets).

### `(c *Checker) Check(ctx, page string) Check`

Checks one page, following the steps in the diagram above. It never returns an error. Every problem becomes a result with a reason, so one bad page can't stop a whole run.

### `(c *Checker) fetch(ctx, method, rawURL string, limit int64) (response, error)`

Waits for its turn, sends one request with our User-Agent, follows redirects (at most 5, set in `New`), and reads at most `limit` bytes of the body. A body read error only matters for robots.txt. For pages, the status is all we need.

### `classify(status int, path string, landedOnHomepage bool) (Result, string)`

Turns the final status into a result, using the table at the top of this page. `landedOnHomepage` is true when a redirect ended on `/`. It only counts for deep pages, since `/` redirecting to `/` is just the homepage.

### `describe(err error) string`

Turns a request error into a short reason: `too many redirects`, `domain not found` (DNS failed), `timeout`, `stopped` (Ctrl+C) or `connection failed`.

### `isHomepage(rawURL string) bool`, `splitPage(page string) (host, path string)`

Small helpers. `isHomepage` checks for an empty or `/` path (a query string doesn't matter). `splitPage` turns `example.com/a/b` into `example.com` and `/a/b`.

## site.go

### `Options`, `New(o Options) *Checker`, `(c *Checker) Close()`

`New` builds the HTTP client:

- the per-request time limit;
- a `CheckRedirect` function that stops after 5 redirects (Go's default is 10);
- the shared ticker that paces requests.

`Close` stops the ticker.

### `(c *Checker) CheckAll(ctx, pages []string, workers int) []Check`

Checks many pages with a small pool of goroutines, the same pattern as the extractor's parser pool:

- `workers` goroutines read page numbers from an unbuffered channel, and the main goroutine sends `0, 1, 2…`, then closes the channel.
- Each goroutine writes its result into its own slot of the output slice. Two goroutines never write the same slot, so no lock is needed, and the results come back in the input order (most linked pages first).
- `sync.WaitGroup.Go` starts each goroutine and `Wait` blocks until all of them finish.

### `(c *Checker) wait(ctx) error`

Blocks until the ticker gives the next turn, or until the context is cancelled. A `time.Ticker` drops ticks nobody is waiting for, so pauses never build up into a burst of requests afterwards.

### `(c *Checker) host(ctx, host string) *hostInfo`, `loadHost(...)`

`host` keeps one `hostInfo` per host, guarded by a mutex. Its `sync.Once` makes sure robots.txt is fetched exactly once, even when two goroutines ask for the same new host at the same moment. The second one waits for the first to finish.

`loadHost` tries `https://host/robots.txt`, then `http://`. The first scheme that answers at all is used for every page on that host. If neither answers, every page on the host is `unknown` with the connection error, without trying each page separately.

## Tests

`check_test.go` runs a fake site on a local test server (`httptest`). Nothing touches the internet. It covers:
- ok, 404, 410;
- a redirect to a working page, a redirect to a 404, and a redirect to the homepage;
- no HEAD support, and HEAD claiming 404 while GET works;
- 503, 403, a slow page (timeout) and a redirect loop;
- a page robots.txt blocks, which the test fails if it's ever requested;
- an unreachable host;
- that every request sends our User-Agent;
- that `CheckAll` keeps the input order and never goes faster than one request per interval.

The fake server only speaks http, so every test also exercises the https → http fallback.

`robots_test.go` covers groups, our group replacing `*`, longest match, ties, `*` and `$`, comments, upper and lower case, groups that name several bots, and rules outside any group.

## Measured (2026-11-15, demo index, 1 request per second, 2 in flight)

| Site | Pages | Time | ok | broken | homepage | couldn't check |
|---|---|---|---|---|---|---|
| wordpress.org | 30 | 48.5s | 30 | 0 | 0 | 0 |
| creativecommons.org | 30 | 31.0s | 30 | 0 | 0 | 0 |
| cardplayer.com | 15 | 30.8s | 1 | 4 | 0 | 10 |
| fansided.com | 15 | 18.1s | 13 | 1 | 1 | 0 |
| escapistmagazine.com | 15 | 29.0s | 1 | 5 | 0 | 9 |

- The broken pages were real: fansided.com/mobile and several old casino-guide pages answer 410 Gone, and fansided.com/fansided-openings redirects to the homepage.
- The "couldn't check" pages answered 403, which means the site blocks bots or blocks by country. That's the reason they count as unknown rather than broken.
- The crawl is from October 2026, only weeks old, so most linked pages still work. Large, well-kept sites had nothing broken.
