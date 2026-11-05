package merge

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/vasishtk1/Linkscope/internal/links"
)

func page(url, domain string, targets ...links.Target) links.Outlinks {
	return links.Outlinks{SourceURL: url, SourceDomain: domain, Targets: targets}
}

func to(domain, url string) links.Target { return links.Target{Domain: domain, URL: url} }

var pages = []links.Outlinks{
	page("https://a.com/1", "a.com", to("b.com", "https://b.com/"), to("c.com", "https://c.com/")),
	page("https://a.com/2", "a.com", to("b.com", "https://b.com/x")),
	page("https://c.com/", "c.com", to("a.com", "https://a.com/")),
	page("https://a.com/0", "a.com", links.Target{Domain: "b.com", URL: "https://b.com/n", Nofollow: true}),
	page("https://d.com/", "d.com", to("b.com", "https://b.com/")),
}

// writeResult saves one worker's share of the pages the same way a worker does.
func writeResult(t *testing.T, path string, share []links.Outlinks) {
	t.Helper()
	g := links.NewGraph()
	for _, p := range share {
		g.Add(p)
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if err := links.WriteTSV(gz, g.Edges()); err != nil {
		t.Fatal(err)
	}
	gz.Close()
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMergeMatchesSingleGraph(t *testing.T) {
	whole := links.NewGraph()
	for _, p := range pages {
		whole.Add(p)
	}

	dir := t.TempDir()
	writeResult(t, filepath.Join(dir, "x-00000.edges.tsv.gz"), pages[:2])
	writeResult(t, filepath.Join(dir, "x-00001.edges.tsv.gz"), pages[2:4])
	writeResult(t, filepath.Join(dir, "x-00002.edges.tsv.gz"), pages[4:])

	paths, err := ResultFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	g, s, err := Files(paths)
	if err != nil {
		t.Fatal(err)
	}
	if s.Files != 3 {
		t.Fatalf("files = %d", s.Files)
	}
	if !reflect.DeepEqual(g.Edges(), whole.Edges()) {
		t.Fatalf("merged result differs from one big graph:\n%+v\n%+v", g.Edges(), whole.Edges())
	}

	reversed := []string{paths[2], paths[1], paths[0]}
	g2, _, err := Files(reversed)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(g2.Edges(), g.Edges()) {
		t.Fatal("file order changed the result")
	}
}

func TestResultFilesIgnoresTempAndOtherFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"b.edges.tsv.gz", "a.edges.tsv.gz", "c.edges.tsv.gz.tmp-123", "notes.txt"} {
		os.WriteFile(filepath.Join(dir, name), nil, 0o644)
	}
	paths, err := ResultFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(dir, "a.edges.tsv.gz"), filepath.Join(dir, "b.edges.tsv.gz")}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("got %v", paths)
	}
}

func TestBadFileNamesTheFile(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.edges.tsv.gz")
	os.WriteFile(bad, []byte("not gzip"), 0o644)
	_, _, err := Files([]string{bad})
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("bad.edges.tsv.gz")) {
		t.Fatalf("err = %v", err)
	}
}

func TestWriteFileRoundTrip(t *testing.T) {
	g := links.NewGraph()
	for _, p := range pages {
		g.Add(p)
	}
	for _, name := range []string{"out.edges.tsv.gz", "out.edges.tsv"} {
		path := filepath.Join(t.TempDir(), name)
		if err := WriteFile(path, g.Edges()); err != nil {
			t.Fatal(err)
		}
		back, _, err := Files([]string{path})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(back.Edges(), g.Edges()) {
			t.Fatalf("%s: round trip changed the edges", name)
		}
	}
}
