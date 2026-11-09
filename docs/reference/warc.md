# internal/warc

Reads records from a WARC file. WARC is the archive format Common Crawl uses; WAT files are WARC files whose records hold JSON metadata.

A record looks like this: a version line, header lines, a blank line, then exactly `Content-Length` bytes of body.

```
WARC/1.0
WARC-Type: metadata
WARC-Target-URI: https://pinchofyum.com/best-pans
Content-Length: 10402

{"Envelope": ...10402 bytes of JSON...}
```

The reader works on an already-decompressed stream. Decompression happens in `extract.Run`.

## Types

### `Record`

One WARC record.

| Field | Meaning |
|---|---|
| `Type` | `WARC-Type` header: `warcinfo`, `metadata`, etc. |
| `TargetURI` | `WARC-Target-URI` header: the page this record describes |
| `Body` | The record's content (JSON for WAT metadata records) |

Only the headers Linkscope uses are kept; the rest are skipped.

### `Reader`

Reads records one at a time from a stream. It wraps the input in a 1 MB buffered reader, so reading line by line doesn't hit the underlying stream for every byte.

## Functions

### `NewReader(r io.Reader) *Reader`

Creates a reader over any byte stream: a file, a gzip stream, or an in-memory string in tests. It takes an `io.Reader` instead of a filename so the same code works on all of them.

### `(*Reader) Next() (*Record, error)`

Returns the next record, or `io.EOF` when the stream ends cleanly.

1. Skips blank lines, then expects a line starting with `WARC/`
2. Reads header lines until a blank line, keeping `WARC-Type`, `WARC-Target-URI`, and `Content-Length` (header names compared case-insensitively)
3. Reads exactly `Content-Length` bytes as the body

**Why read by length instead of scanning for a separator:** the body can contain blank lines or even text like `WARC/1.0`. The length is the only reliable boundary.

**Errors:**
- Not starting with `WARC/`: the stream is corrupt or not a WARC file
- Missing or negative `Content-Length`: the body boundary can't be known
- Stream ends mid-record: `io.ErrUnexpectedEOF`, which is distinct from a clean `io.EOF`, so a truncated download isn't mistaken for a finished file

### `WriteRecord(w io.Writer, rec *Record) error`

The opposite of `Next`: writes a record with `WARC-Type`, `WARC-Target-URI` and `Content-Length` headers, the body, and the closing blank lines, compressed as its own gzip member like Common Crawl does. `cmd/slicewat` uses it to cut small demo files out of a real WAT file. Only the three headers this package reads are written; the others in real files (dates, record ids) aren't needed by the extractor.

A round-trip test writes two records and reads them back with `Next`.

### `readLine() (string, error)`

Reads one line and strips the trailing `\r\n` or `\n`. WARC uses `\r\n`, but stripping both is more forgiving.

### `readNonEmptyLine() (string, error)`

Like `readLine`, but skips blank lines. Records are separated by two blank lines, so this moves past them to the next record.

### `unexpected(err error) error`

Turns `io.EOF` into `io.ErrUnexpectedEOF`. It's used inside a record, where hitting the end of the stream means the file was cut off.
