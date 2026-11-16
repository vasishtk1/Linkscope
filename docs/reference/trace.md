# internal/trace

Records each step the agent takes, so you can watch it work instead of only seeing the final table. `cmd/linkcheck -trace` prints the steps live:

```
    8.8s  check     fansided.com/fansided-openings: redirects to the homepage
   18.1s  sitemap   reading the sitemap of fansided.com to see which pages are live
   34.1s  sitemap   14 files read, 50000 live pages
   34.2s  match     fansided.com/fansided-openings: comparing addresses: no clear match (closest page is too different), closest fansided.com/posts/ranking-4-current-wnba-head-coach-openings (score 0.12)
   40.5s  archive   fansided.com/fansided-openings: the Internet Archive's copy from 20230819 was titled "FanSided Openings - Become a FanSided Contributor, Expert, or Podcaster today."
   40.5s  read      fansided.com/fansided-openings: reading the titles of the 10 closest live pages
   49.4s  ask       fansided.com/fansided-openings: asking Gemini which page replaces it, if any
```

A step from Gemini also carries its thinking, printed as an indented block under the step.

The same events go into the `-json` output as `steps`, with `at` (seconds since the start), `step`, `page`, `text` and `thinking`. The website's agent page will play these back as a timeline.

## trace.go

### `Event`

One step: when (`At`, in seconds), what kind (`Step`: `check`, `sitemap`, `match`, `archive`, `read`, `ask`, `decide`, `verify`, `done`), which page, the text, and optional thinking.

### `New(out io.Writer) *Log`, `(l *Log) Say(step, page, format, args...)`, `(l *Log) Add(e Event)`

`Say` is the short form, like `fmt.Printf`. `Add` takes a full event, which is how thinking gets attached. Both stamp the time, keep the event and, if `out` isn't nil, print it.

Page checks run in two goroutines at once, so a mutex guards the list. Calling a method on a nil `*Log` does nothing, so code that doesn't want a trace can pass `nil` instead of checking everywhere.

### `(l *Log) Events() []Event`

A copy of the events, so the caller can't change the log's own list while another goroutine adds to it.

## Tests

`trace_test.go`: printed format including the thinking block, a nil log, and 20 goroutines logging at once (run with `-race`).
