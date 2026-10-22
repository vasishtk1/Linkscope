package extract

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"reflect"
	"testing"
)

func metadataRecord(uri, status string, links ...string) string {
	linkJSON := ""
	for i, l := range links {
		if i > 0 {
			linkJSON += ","
		}
		linkJSON += fmt.Sprintf(`{"path":"A@/href","url":%q}`, l)
	}
	body := fmt.Sprintf(`{"Envelope":{"Payload-Metadata":{"HTTP-Response-Metadata":{"Response-Message":{"Status":%q},"HTML-Metadata":{"Links":[%s]}}}}}`, status, linkJSON)
	return fmt.Sprintf("WARC/1.0\r\nWARC-Type: metadata\r\nWARC-Target-URI: %s\r\nContent-Length: %d\r\n\r\n%s\r\n\r\n", uri, len(body), body)
}

func gzipped(t *testing.T, parts ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	for _, p := range parts {
		// Common Crawl files are many gzip members back to back, one per record.
		zw := gzip.NewWriter(&buf)
		if _, err := zw.Write([]byte(p)); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return buf.Bytes()
}

func sampleFile(t *testing.T) []byte {
	return gzipped(t,
		"WARC/1.0\r\nWARC-Type: warcinfo\r\nContent-Length: 4\r\n\r\ninfo\r\n\r\n",
		metadataRecord("https://a.com/1", "200", "https://b.com/", "https://c.com/", "/internal"),
		metadataRecord("https://a.com/2", "200", "https://b.com/x"),
		metadataRecord("https://c.com/", "200", "https://a.com/"),
		metadataRecord("https://d.com/", "404", "https://b.com/"),
		"WARC/1.0\r\nWARC-Type: metadata\r\nContent-Length: 5\r\n\r\n{bad}\r\n\r\n",
	)
}

func TestRun(t *testing.T) {
	g, s, err := Run(bytes.NewReader(sampleFile(t)), 2)
	if err != nil {
		t.Fatal(err)
	}
	wantStats := Stats{Records: 6, Responses: 4, PagesUsed: 3, CrossLinks: 4, ParseErrors: 1}
	if s != wantStats {
		t.Fatalf("stats = %+v, want %+v", s, wantStats)
	}
	got := map[string]int{}
	for _, e := range g.Edges() {
		got[e.Src+"->"+e.Dst] = e.Pages
	}
	want := map[string]int{"a.com->b.com": 2, "a.com->c.com": 1, "c.com->a.com": 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("edges = %v, want %v", got, want)
	}
}

func TestRunSameResultForAnyWorkerCount(t *testing.T) {
	file := sampleFile(t)
	g1, _, err := Run(bytes.NewReader(file), 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []int{2, 4, 16} {
		gw, _, err := Run(bytes.NewReader(file), w)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(g1.Edges(), gw.Edges()) {
			t.Fatalf("%d workers gave a different graph", w)
		}
	}
}

func TestRunReportsTruncatedFile(t *testing.T) {
	file := gzipped(t, "WARC/1.0\r\nWARC-Type: metadata\r\nContent-Length: 999\r\n\r\nshort")
	if _, _, err := Run(bytes.NewReader(file), 2); err == nil {
		t.Fatal("expected error for truncated record")
	}
}

func TestRunRejectsNonGzip(t *testing.T) {
	if _, _, err := Run(bytes.NewReader([]byte("not gzip")), 1); err == nil {
		t.Fatal("expected error")
	}
}
