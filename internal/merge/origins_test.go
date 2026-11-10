package merge

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestOriginsAddUpToMergedPages(t *testing.T) {
	dir := t.TempDir()
	writeResult(t, filepath.Join(dir, "x-00000.edges.tsv.gz"), pages[:2])
	writeResult(t, filepath.Join(dir, "x-00001.edges.tsv.gz"), pages[2:4])
	writeResult(t, filepath.Join(dir, "x-00002.edges.tsv.gz"), pages[4:])
	paths, err := ResultFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	g, o, _, err := FilesWithOrigins(paths)
	if err != nil {
		t.Fatal(err)
	}
	want := []Origin{{"x-00000", 2}, {"x-00001", 1}, {"x-00002", 1}}
	if got := o.List("b.com"); !reflect.DeepEqual(got, want) {
		t.Fatalf("b.com origins = %v, want %v", got, want)
	}
	total := map[string]int{}
	for _, e := range g.Edges() {
		total[e.Dst] += e.Pages
	}
	for dst, n := range total {
		sum := 0
		for _, x := range o.List(dst) {
			sum += x.Pages
		}
		if sum != n {
			t.Errorf("%s: origins add to %d pages, merged graph has %d", dst, sum, n)
		}
	}

	path := filepath.Join(t.TempDir(), "origins.tsv.gz")
	if err := WriteOrigins(path, o); err != nil {
		t.Fatal(err)
	}
	back, err := ReadOrigins(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(back) != len(o) || !reflect.DeepEqual(back["b.com"], want) {
		t.Fatalf("round trip: %v", back)
	}
}

func TestFileName(t *testing.T) {
	if got := FileName("data/results/CC-MAIN-x-00042.edges.tsv.gz"); got != "CC-MAIN-x-00042" {
		t.Fatalf("got %q", got)
	}
}
