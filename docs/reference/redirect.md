# internal/redirect

Picks a live page to send visitors to for each broken page. It's phase 2 of the pull request agent. Phase 1 ([linkcheck.md](linkcheck.md)) finds pages that are broken but still have sites linking to them. This package reads the site's sitemap and picks the live page whose address matches best, only if it's a clear winner and actually loads. It uses no AI. Unclear cases are left as "no clear match" for a later phase (Gemini).

```
broken page: fansided.com/mobile
   │
   ├─ sitemap: robots.txt "Sitemap:" lines, else /sitemap.xml → 50,000 live pages
   ├─ Rank: score each live page by shared address words     → /fansided-mobile-app 0.50, /mobile-app-account-deletion 0.25
   ├─ Clear: score ≥ 0.5 and ≥ 0.15 ahead of second place?   → yes
   └─ verify: does /fansided-mobile-app load (2xx, not sent to the homepage)? → yes → suggestion
```

## How pages are scored

1. **Split each address into words.** `/blog/Go-Tips_2023.html` becomes `blog, go, tips, 2023`.
   - Splitting happens on anything that isn't a letter or a digit. Letters are lowercased and `%C3%A9` is decoded to `é`, so non-English addresses work.
   - Filler is dropped: `html`, `php`, `index`, `amp` and similar.
   - Long numbers (5+ digits) are dropped. These are usually database IDs, like `/p/1234567/story`, that say nothing about the topic. Years like `2023` stay.
   - **The site's own name is dropped.** On fansided.com, the word `fansided` appears in many addresses, so it says nothing about what a page is about. Before this rule, `/mobile` → `/fansided-mobile-app` scored only 0.33 and was rejected. With it, the score is 0.50 and the page is picked.
2. **Score = shared words ÷ all distinct words in both addresses** (Jaccard similarity). It's 1.0 for the same words and 0 for none in common. Extra words on either side lower the score, so a long, unrelated article that happens to share one word scores low.
3. **+0.1 for the same section,** when both pages sit in the same first folder (`/blog/…`) and the broken page is inside a folder.
4. **Only pages on the same site count** (`www.` ignored). The broken page itself is never a candidate, even if the sitemap still lists it. The homepage never is either, since it has no words.

## When a pick counts as clear

- **Good enough:** score at least `MinScore` (0.5), so at least half of all the words involved are shared.
- **Clearly ahead:** at least `MinLead` (0.15) above second place. If two pages are about equally good (`/shop/red-shoes` and `/shop/blue-shoes` for `/shop/green-shoes`), picking one would be a guess.
- **It loads:** the pick is checked with the same checker as phase 1 and must return `ok`. A pick that's broken, blocked or sent to the homepage is skipped and the next one is tried, at most 3 tries. If the pick redirects somewhere, the final address is used, so the site never gets a redirect that leads to another redirect.

Anything else is **no clear match**, with the reason and the closest page, so a person (or Gemini later) can decide. Being cautious matters more than matching every page: a wrong redirect sends visitors to an unrelated page, which is worse than the 404.

## sitemap.go

### `Sitemap`

What was read: `Pages` (live page addresses, de-duplicated, in sitemap order), `Files` read, `Failed` files, and `Capped` (true if a limit stopped it early).

### `Fetcher`

An interface with one method, `Get`, which `linkcheck.Checker` already has. The package depends on this small interface instead of the concrete checker, so tests can pass a fake that serves sitemaps from a map without a server. `var _ Fetcher = (*linkcheck.Checker)(nil)` makes the compiler confirm the real checker fits.

### `ReadSitemaps(ctx, f Fetcher, start []string) Sitemap`

Starts from the addresses `linkcheck.Checker.Sitemaps` gives: the robots.txt `Sitemap:` lines, or `/sitemap.xml`. It works through a queue:

- A **sitemap index** (a list of other sitemaps, common on big sites) adds its files to the end of the queue.
- A normal sitemap adds its pages.
- It stops after `MaxSitemapFiles` (20) files or `MaxPages` (50,000) pages. On a news site, 20 files can be years of articles, and every file is one more request at one per second.
- A file that fails (404, bad XML, timeout) is counted and skipped. It never stops the run.
- Each file is read once, even if the index lists itself.

### `parseSitemap(body []byte) (sitemapDoc, error)`

Handles the three formats sites use:
- **XML**: `<urlset><url><loc>` for pages and `<sitemapindex><sitemap><loc>` for other sitemaps. `encoding/xml` fills both lists from one struct (`xml:"url>loc"` means "the `loc` inside each `url`").
- **Gzipped XML** (`.xml.gz`): detected by the first two bytes (`1f 8b`), whatever the file is called.
- **Plain text**: one URL per line. Lines that aren't URLs are skipped.

## match.go

### `Candidate`

One live page with its `Score`, the `Shared` words and `SameSection`. These fields are the "why" shown next to each suggestion.

### `Rank(page string, live []string) []Candidate`

Scores every live page against the broken page, using the rules above, best first. Ties go to the shorter address, which is usually the more general page. Pages sharing no words are left out.

### `Clear(c []Candidate) bool`

True if the first candidate is good enough and far enough ahead of the second (see above).

### `words(path string) []string`

Splits an address into words, using the rules above. Each word appears once.

### `firstSegment`, `splitKey`, `bareHost`, `without`, `isDigits`, `intersect`, `union`

Small helpers:
- `firstSegment` gives the section of a deep page (`/blog/x` → `blog`, and nothing for `/about`).
- `splitKey` splits an index key into host and path.
- `without` removes the site-name words.
- `intersect` and `union` work on word lists. These lists are short, a handful of words, so plain slices are simpler than maps and just as fast.

## suggest.go

### `Suggestion`

For one broken page:
- `Target`: the address to redirect to, empty if none.
- `Pick` and `RunnerUp`: the top two candidates, kept even with no match so the closest page can be shown.
- `NoMatch`: the reason there's no target.

### `Verifier`

An interface with `Check`, which `linkcheck.Checker` has. As with `Fetcher`, tests pass a fake that says which pages are broken.

### `Suggest(ctx, v Verifier, page string, live []string) Suggestion`

Ranks the live pages and checks the clear winner. If the winner doesn't load, it drops it and repeats, up to 3 times. Without a target, `NoMatch` gives one of these reasons:
- `no live page shares a word with it`
- `closest page is too different`: below 0.5.
- `top two are too close to call`: less than 0.15 apart.
- `the best matches didn't load`

## Tests

`redirect_test.go` uses no network:
- **word splitting:** filler, IDs, accents, repeats;
- **ranking:** a clear winner with the right shared words, no other sites, no homepage, no matching the page to itself, and the site-name rule;
- **unclear cases:** a page with no words in common, and two equally weak pages;
- **`Suggest`:** with a fake verifier, falling back when the first pick is broken, and each no-match reason;
- **`ReadSitemaps`:** with a fake fetcher, an index pointing to a gzipped sitemap, a text sitemap, a 404, a failing file and itself, plus the 20-file limit.

## Measured (2026-11-15, top 15 pages per site, demo index)

| Site | Broken or sent to homepage | Clear match | Sitemap read |
|---|---|---|---|
| fansided.com | 2 | 1: `/mobile` → `/fansided-mobile-app` | 14 files, 50,000 pages (page limit) |
| cardplayer.com | 4 | 0 | 20 files (file limit), 14,005 pages |
| escapistmagazine.com | 5 | 0 | 20 files (file limit), 13,818 pages |

- The 9 with no match are casino guides the sites deleted on purpose (410 Gone). Nothing replaced them, and the closest pages were unrelated news stories, so no match is the right answer.
- `/fansided-openings` (a jobs page, sent to the homepage) was correctly not matched to an article about WNBA coaching openings that shares only the word "openings" (score 0.12).
- The fansided.com run took 35.3s in total: 15 page checks, 14 sitemap files, and 1 check of the picked page.

**Caveat:** when there are many candidate pages and words in common are rare, address matching alone can't tell what pages are *about*. That's where Gemini comes in, reading page titles and picking from the top few candidates.
