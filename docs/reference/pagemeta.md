# internal/pagemeta

Reads what a page says it's about: its title and description. Gemini needs this to choose a redirect target (see [gemini.md](gemini.md)). An address like `/fansided-openings` says little, but the title "FanSided Openings - Become a FanSided Contributor" makes it clear it was a jobs page.

All downloads go through a `Fetcher`, which `linkcheck.Checker` provides. So these requests follow the same rules as page checks: one request per second, our User-Agent, and robots.txt.

## What the dead page used to be

The dead page can't be read anymore, so `Archived` asks the Internet Archive (the Wayback Machine):

1. `https://archive.org/wayback/available?url=fansided.com/fansided-openings&timestamp=20261001` returns the saved copy closest to the crawl month (October 2026).
2. That copy is downloaded in its `id_` form (`/web/20230819142241id_/https://...`), which is the page exactly as it was, without the archive's toolbar around it.
3. Its title and description are read.

Two requests per page. If there's no copy, or it can't be read, Gemini decides from the address alone and is told so.

## pagemeta.go

### `Info`

`URL`, `Title`, `Description`, and `Archived`: the date of the archive copy (`20230819`), empty for live pages.

### `(r Reader) Page(ctx, rawURL) Info`

Downloads a live page (at most 256 KB) and reads its title. A page that fails comes back with just its URL, so Gemini still sees the address.

### `(r Reader) Archived(ctx, page) Info`

The steps above. It only sets `Archived` if it actually found a title or description.

### `rawSnapshot(snapshot, timestamp string) (string, error)`

Turns the archive's link into the `id_` form, over https.

### `Parse(body []byte) (title, description string)`

Reads `<title>` and `<meta name="description">` with `golang.org/x/net/html`'s tokenizer, which already handles messy HTML and turns `&amp;` back into `&`. `og:title` and `og:description` (the tags used for link previews) are used only when the plain ones are missing. It stops at `<body>` once it has a title, since these tags live in `<head>`.

### `clean(s string) string`

Collapses whitespace and cuts text to 300 characters, so one page with a huge description can't fill up the prompt.

## Tests

`pagemeta_test.go` uses a fake fetcher: plain tags, `og:` fallbacks, a page without a head, stopping at `<body>`, long text, the archive lookup with and without a copy, and live pages that fail.
