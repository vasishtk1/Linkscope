package extract

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/vasishtk1/Linkscope/internal/fixture"
)

func sampleFile() []byte {
	return fixture.Gzip(
		fixture.Record("warcinfo", "", "info"),
		fixture.Page("https://a.com/1", "200", "https://b.com/", "https://c.com/", "/internal"),
		fixture.Page("https://a.com/2", "200", "https://b.com/x"),
		fixture.Page("https://c.com/", "200", "https://a.com/"),
		fixture.Page("https://d.com/", "404", "https://b.com/"),
		fixture.Record("metadata", "https://e.com/", "{bad}"),
	)
}

func TestRun(t *testing.T) {
	g, s, err := Run(bytes.NewReader(sampleFile()), 2)
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
	file := sampleFile()
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
	file := fixture.Gzip("WARC/1.0\r\nWARC-Type: metadata\r\nContent-Length: 999\r\n\r\nshort")
	if _, _, err := Run(bytes.NewReader(file), 2); err == nil {
		t.Fatal("expected error for truncated record")
	}
}

func TestRunRejectsNonGzip(t *testing.T) {
	if _, _, err := Run(bytes.NewReader([]byte("not gzip")), 1); err == nil {
		t.Fatal("expected error")
	}
}
