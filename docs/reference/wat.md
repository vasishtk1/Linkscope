# internal/wat

Parses the JSON body of a WAT metadata record into a `Page`.

Each crawled page produces three metadata records: the HTTP request, the HTTP response, and some crawl metadata. Only **response** records contain the page's links. In a real file, about one record in three is a response (22,022 of 66,068 in our sample).

The relevant part of the JSON:

```json
{"Envelope": {"Payload-Metadata": {"HTTP-Response-Metadata": {
  "Response-Message": {"Status": "200"},
  "HTML-Metadata": {
    "Head": {"Base": "https://example.com/"},
    "Links": [{"path": "A@/href", "url": "/about", "rel": "nofollow", "text": "About"}]
  }
}}}}
```

## Types

### `Link`

One link found on the page.

| Field | Meaning |
|---|---|
| `Path` | Which HTML tag and attribute it came from: `A@/href` is a normal link, `IMG@/src` is an image, `SCRIPT@/src` a script, etc. |
| `URL` | The link target exactly as written in the HTML, often relative (`/about`, `page.html`) |
| `Rel` | The `rel` attribute, e.g. `nofollow` |

### `Page`

A crawled page: its `URL`, HTTP `Status`, the `<base>` tag's `Base` URL (if any), and its `Links`.

### `metadata` (unexported)

Mirrors just the parts of the WAT JSON we need. Go's JSON decoder skips any field not in the struct, so the rest of the (large) JSON is ignored without extra code. `HTTPResponseMetadata` is a pointer so a missing section (request records) shows up as `nil` instead of an empty struct.

## Functions

### `ParsePage(targetURI string, body []byte) (Page, bool, error)`

Parses one record's JSON.

| Returns | When |
|---|---|
| `page, true, nil` | It's an HTTP response record |
| `_, false, nil` | Valid JSON, but not a response (request or crawl-info record) |
| `_, false, err` | Invalid JSON |

A non-numeric status becomes `0`, which `links.Extract` then skips as non-2xx.

**Why three return values:** callers need to tell "not a page" (normal, skip silently) apart from "broken data" (count it as an error).
