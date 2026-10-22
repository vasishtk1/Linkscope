# Reference

Every package and function, documented here instead of in code comments. Each entry covers what it does, its inputs and outputs, and why it's built that way.

| Package | What it does |
|---|---|
| [internal/warc](warc.md) | Reads raw records out of a WARC/WAT file |
| [internal/wat](wat.md) | Parses a WAT record's JSON into a page with its links |
| [internal/links](links.md) | Turns URLs into domains, picks which links count, and builds the link graph |
| [internal/extract](extract.md) | Runs a whole WAT file through a pool of parser goroutines |
| [cmd/](cmd.md) | The runnable programs: `extract`, `coordinator`, `worker` |

## How the packages connect

```
.wat.gz file → gzip → warc.Reader.Next() → wat.ParsePage() → links.Extract() → links.Graph.Add() → Graph.Edges()
                      └──────────────────── all orchestrated by extract.Run() ───────────────────┘
```
