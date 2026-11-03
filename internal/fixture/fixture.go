package fixture

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"strings"
)

func Page(uri, status string, links ...string) string {
	parts := make([]string, len(links))
	for i, l := range links {
		parts[i] = fmt.Sprintf(`{"path":"A@/href","url":%q}`, l)
	}
	body := fmt.Sprintf(`{"Envelope":{"Payload-Metadata":{"HTTP-Response-Metadata":{"Response-Message":{"Status":%q},"HTML-Metadata":{"Links":[%s]}}}}}`,
		status, strings.Join(parts, ","))
	return Record("metadata", uri, body)
}

func Record(typ, uri, body string) string {
	return fmt.Sprintf("WARC/1.0\r\nWARC-Type: %s\r\nWARC-Target-URI: %s\r\nContent-Length: %d\r\n\r\n%s\r\n\r\n", typ, uri, len(body), body)
}

// Gzip compresses each record as its own gzip member, like Common Crawl does.
func Gzip(records ...string) []byte {
	var buf bytes.Buffer
	for _, r := range records {
		zw := gzip.NewWriter(&buf)
		zw.Write([]byte(r))
		zw.Close()
	}
	return buf.Bytes()
}
