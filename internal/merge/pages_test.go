package merge

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/vasishtk1/Linkscope/internal/links"
)

// writeWithPages saves one worker's share as both files a worker writes: edges and pages.
func writeWithPages(t *testing.T, path string, share []links.Outlinks) {
	t.Helper()
	writeResult(t, path, share)
	g := links.NewGraph()
	for _, p := range share {
		g.Add(p)
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if err := links.WritePagesTSV(gz, g.PageLinks()); err != nil {
		t.Fatal(err)
	}
	gz.Close()
	if err := os.WriteFile(PagesPath(path), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPagesCombineAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	writeWithPages(t, filepath.Join(dir, "x-00000.edges.tsv.gz"), pages[:2])
	writeWithPages(t, filepath.Join(dir, "x-00001.edges.tsv.gz"), pages[2:4])
	writeWithPages(t, filepath.Join(dir, "x-00002.edges.tsv.gz"), pages[4:])
	paths, err := ResultFiles(dir)
	if err != nil {
		t.Fatal(err)
	}

	sp, s, err := Pages(paths)
	if err != nil {
		t.Fatal(err)
	}
	if s.Files != 3 || s.Missing != 0 {
		t.Fatalf("stats = %+v", s)
	}
	want := SitePages{Total: 3, Top: []TopPage{
		{Page: "b.com/", Sites: 2, Pages: 2, Sources: []PageSource{{"a.com", 1}, {"d.com", 1}}},
		{Page: "b.com/n", Sites: 1, Pages: 1, NofollowPages: 1, Sources: []PageSource{{"a.com", 1}}},
		{Page: "b.com/x", Sites: 1, Pages: 1, Sources: []PageSource{{"a.com", 1}}},
	}}
	if got := sp["b.com"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("b.com pages:\n got %+v\nwant %+v", got, want)
	}
}

func TestPagesCountsResultsWithoutPagesFile(t *testing.T) {
	dir := t.TempDir()
	writeWithPages(t, filepath.Join(dir, "x-00000.edges.tsv.gz"), pages[:2])
	writeResult(t, filepath.Join(dir, "x-00001.edges.tsv.gz"), pages[2:])
	paths, _ := ResultFiles(dir)
	_, s, err := Pages(paths)
	if err != nil {
		t.Fatal(err)
	}
	if s.Files != 1 || s.Missing != 1 {
		t.Fatalf("stats = %+v, want 1 file and 1 missing", s)
	}
}

func TestPagesAreCapped(t *testing.T) {
	var share []links.Outlinks
	for i := range MaxPages + 5 {
		target := to("big.com", fmt.Sprintf("https://big.com/p%d", i))
		for j := range MaxPageSources + 3 {
			src := fmt.Sprintf("s%d.com", j)
			share = append(share, page(fmt.Sprintf("https://%s/%d", src, i), src, target))
		}
	}
	dir := t.TempDir()
	writeWithPages(t, filepath.Join(dir, "x-00000.edges.tsv.gz"), share)
	paths, _ := ResultFiles(dir)
	sp, _, err := Pages(paths)
	if err != nil {
		t.Fatal(err)
	}
	b := sp["big.com"]
	if b.Total != MaxPages+5 || len(b.Top) != MaxPages {
		t.Fatalf("total %d, kept %d; want %d and %d", b.Total, len(b.Top), MaxPages+5, MaxPages)
	}
	if n := len(b.Top[0].Sources); n != MaxPageSources || b.Top[0].Sites != MaxPageSources+3 {
		t.Fatalf("kept %d sources of %d sites; want %d of %d", n, b.Top[0].Sites, MaxPageSources, MaxPageSources+3)
	}
}

func TestWritePagesRoundTrip(t *testing.T) {
	in := map[string]SitePages{
		"b.com": {Total: 7, Top: []TopPage{
			{Page: "b.com/", Sites: 2, Pages: 5, NofollowPages: 1, Sources: []PageSource{{"a.com", 4}, {"d.com", 1}}},
			{Page: "b.com/x", Sites: 1, Pages: 1, Sources: []PageSource{{"a.com", 1}}},
		}},
		"c.com": {Total: 1, Top: []TopPage{{Page: "c.com/", Sites: 1, Pages: 1, Sources: []PageSource{{"a.com", 1}}}}},
	}
	path := filepath.Join(t.TempDir(), "merged.pages.tsv.gz")
	if err := WritePages(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadPages(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("got %+v, want %+v", out, in)
	}
}
