package warc

import (
	"compress/gzip"
	"fmt"
	"io"
)

// WriteRecord writes one record as its own gzip member, the way Common Crawl stores them.
func WriteRecord(w io.Writer, rec *Record) error {
	zw := gzip.NewWriter(w)
	if _, err := fmt.Fprintf(zw, "WARC/1.0\r\nWARC-Type: %s\r\nWARC-Target-URI: %s\r\nContent-Length: %d\r\n\r\n", rec.Type, rec.TargetURI, len(rec.Body)); err != nil {
		return err
	}
	if _, err := zw.Write(rec.Body); err != nil {
		return err
	}
	if _, err := io.WriteString(zw, "\r\n\r\n"); err != nil {
		return err
	}
	return zw.Close()
}
