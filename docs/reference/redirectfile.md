# internal/redirectfile

Writes the redirects into the site's own redirect file. It's phase 3 of the pull request agent:

- Phase 1 ([linkcheck.md](linkcheck.md)) finds broken pages that other sites still link to.
- Phase 2 ([redirect.md](redirect.md)) picks a live page for each one.
- This package edits the file in the site's repo that tells the host where to send visitors.

It only changes text in a local folder. No network, no AI, no GitHub. Opening the pull request is the next phase.

```
linkcheck -suggest -json        →  /mobile should go to /fansided-mobile-app
redirects -repo ./site          →  finds vercel.json, adds the redirect, keeps everything else
                                   {"source": "/mobile", "destination": "/fansided-mobile-app", "permanent": true}
```

## Which file

`Find` looks in this order and uses the first file it finds:

| File | Host | What a redirect looks like |
|---|---|---|
| `vercel.json` | Vercel | `{"source": "/old", "destination": "/new", "permanent": true}` in `"redirects"` |
| `public/_redirects`, `static/_redirects`, `_redirects` | Netlify (and Cloudflare Pages) | a line `/old /new 301` |
| `netlify.toml` | Netlify | a `[[redirects]]` block with `from`, `to`, `status = 301` |

`public/` and `static/` come first for `_redirects` because that's where frameworks like Next.js, Vite and Hugo keep files that get copied into the built site unchanged.

**Why permanent redirects (301, or 308 on Vercel):** a permanent redirect tells search engines the page moved for good, so the links pointing at the old address count for the new one. That's the reason for the whole agent: links to a dead page are wasted until it redirects somewhere.

## Rules for editing someone else's file

The file belongs to the site owner, and the change will show up in a pull request, so it must be small and safe:

- **Nothing else changes.** In `vercel.json`, every other setting stays, in the same order, with the file's own indent. `_redirects` and `netlify.toml` are edited as text, so comments and spacing stay exactly as they were.
- **New redirects go first.** Hosts use the first rule that matches. Many sites end with a catch-all like `/* /index.html 200`, which would hide anything added after it. Rules for exact paths are safe at the top, since they only match that one page.
- **Already redirected pages are skipped.** If the file already has a rule for the path (a trailing `/` doesn't matter), the owner already decided where it goes.
- **Paths with pattern characters are skipped.** In these files, `:name`, `*`, `(…)` and similar mean "match many pages". A real path that happens to contain them would turn into a pattern, so it's left out instead of escaped. Paths with spaces or quotes are skipped too.
- **Files the tool can't edit safely are refused**, not guessed at: `vercel.json` that isn't one JSON object, or that uses the old `"routes"` setting (Vercel doesn't allow `"routes"` and `"redirects"` together).

## redirectfile.go

### `Kind`, `File`, `Find(repo string) (File, error)`

`Kind` is one of `vercel.json`, `_redirects`, `netlify.toml`. `Find` returns the first file from the table above as a `File` (its path inside the repo plus its kind), or `ErrNoFile`.

### `Places() []File`

The same table as a list, in order, for repos that aren't on disk. `cmd/pr` asks GitHub for each path in turn and uses the first one that exists.

### `Redirect`, `New(page, target string) Redirect`

A redirect from a path to a target. `New` turns phase 2's output into one: the page `fansided.com/mobile` becomes `/mobile`. The target becomes just its path (and query) when it's on the same site, `www.` or not, and stays a full URL when it's on another host, such as a subdomain. Paths are kept exactly as they appear in addresses, so `%C3%A9` stays encoded, which is what the hosts expect.

### `Apply(kind Kind, content []byte, rs []Redirect) (Result, error)`

The whole edit:
1. Reads the paths the file already redirects.
2. Checks each new redirect, sorting it into `Added` or `Skipped` with a reason.
3. Writes the added ones in the file's format.

It takes and returns the file's bytes instead of reading and writing disk, so tests don't need files, and the next phase can send the new content straight to GitHub. If nothing was added, `Content` is the input unchanged. A duplicate in the input list is skipped as already redirected.

### `problem(r Redirect) string`

Why a redirect can't be written, or `""`: not a page path (`/` itself), pattern characters, spaces or quotes, no target, or a redirect to itself.

### `samePath`, `containsPath`, `bareHost`

Small helpers. `samePath` drops a trailing `/` so `/kept` and `/kept/` count as the same path. Upper and lower case are kept apart, since paths are case-sensitive.

## formats.go

### `parseObject(content []byte) ([]member, error)`

Reads `vercel.json`'s top-level keys **in file order**. A Go `map` doesn't keep order, and `json.Marshal` sorts map keys, so decoding into a map and writing it back would shuffle the owner's file and make the pull request look like it rewrote everything. Instead `json.Decoder.Token` walks the object key by key, and each value is kept as `json.RawMessage` (the raw bytes, not decoded). An empty file counts as `{}`. Extra text after the object is an error.

### `vercelSources`, `addVercel`

`vercelSources` lists the existing `source` paths and refuses `"routes"`. `addVercel` puts the new redirects in front of the existing ones, or adds a `"redirects"` key at the end, then joins the members back together and formats with `json.Indent`, using the indent from `indentOf`.

### `marshal(v any) ([]byte, error)`

`json.Marshal` turns `&`, `<` and `>` into `&`-style escapes (meant for JSON placed inside HTML). In a redirect file that would turn `/page?a=1&b=2` into something unreadable, so this uses a `json.Encoder` with `SetEscapeHTML(false)`.

### `indentOf(content []byte) string`

The leading whitespace of the first indented line, so a file indented with 4 spaces or tabs stays that way. 2 spaces for a new or one-line file.

### `netlifySources`, `addNetlify`

`_redirects` is one rule per line: from, to, status. Lines starting with `#` are comments. The new rules go at the top under one comment line, then a blank line, then the original file.

### `tomlSources`, `addTOML`

Finds existing `from = "…"` lines, and puts the new `[[redirects]]` blocks just before the first existing one (same first-match reason), or at the end if there are none. It's edited as text rather than parsed with a TOML library, so nothing else in the file is rewritten and the module needs no new dependency.

## Tests

`redirectfile_test.go` uses no network:
- **`New`:** same site, `www.`, a query string, the homepage, and another host;
- **`Find`:** an empty repo, the search order;
- **`vercel.json`:** other keys and their order kept, 4-space indent kept, `&` not escaped, an existing rule with a trailing `/`, a pattern path, a duplicate in the input, an empty file, a file without `"redirects"`;
- **refused files:** `"routes"`, a JSON array, extra text, cut-off JSON;
- **`_redirects`:** new rules go above a catch-all, comments kept;
- **`netlify.toml`:** blocks go before the first `[[redirects]]`, or at the end;
- **nothing to add:** the file comes back byte for byte.

## Tried on real data (2026-11-15)

`linkcheck -site fansided.com -limit 15 -suggest -json`, then `redirects` on a test folder:
- With a `vercel.json` holding only `"cleanUrls": true`, it added `/mobile` → `/fansided-mobile-app` and kept `cleanUrls`.
- Run again, it skipped the page as already redirected.
- With a `public/_redirects` holding a `/* /index.html 200` catch-all, the new rule went above it.
