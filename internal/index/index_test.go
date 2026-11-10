package index

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/vasishtk1/Linkscope/internal/links"
	"github.com/vasishtk1/Linkscope/internal/merge"
)

func edge(src, dst string, pages, nofollow int) links.Edge {
	return links.Edge{Src: src, Dst: dst, Pages: pages, NofollowPages: nofollow,
		ExampleSrc: "https://" + src + "/a", ExampleDst: "https://" + dst + "/b"}
}

var testEdges = []links.Edge{
	edge("reddit.com", "github.com", 12, 0),
	edge("news.ycombinator.com", "github.com", 40, 2),
	edge("blog.example.org", "github.com", 12, 12),
	edge("github.com", "go.dev", 3, 0),
	edge("reddit.com", "go.dev", 1, 0),
	edge("go.dev", "github.com", 5, 0),
}

var testScores = map[string]int{"github.com": 100, "go.dev": 61, "reddit.com": 0}

func build(t *testing.T, edges []links.Edge, shards int) (string, *Index) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "index")
	if _, err := Build(dir, edges, testScores, Options{Shards: shards, Source: "test data"}); err != nil {
		t.Fatal(err)
	}
	ix, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ix.Close() })
	return dir, ix
}

func TestLookupMatchesInput(t *testing.T) {
	for _, shards := range []int{1, 4, 16} {
		_, ix := build(t, testEdges, shards)
		got, err := ix.Lookup("https://www.GitHub.com/vasishtk1/Linkscope", 0)
		if err != nil {
			t.Fatal(err)
		}
		score := 100
		want := &Lookup{
			Domain: "github.com", Score: &score, LinkingDomains: 4, LinkingPages: 69, NofollowPages: 14,
			Backlinks: []Backlink{
				{"news.ycombinator.com", 40, 2, "https://news.ycombinator.com/a", "https://github.com/b"},
				{"blog.example.org", 12, 12, "https://blog.example.org/a", "https://github.com/b"},
				{"reddit.com", 12, 0, "https://reddit.com/a", "https://github.com/b"},
				{"go.dev", 5, 0, "https://go.dev/a", "https://github.com/b"},
			},
			Source: "test data",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%d shards:\ngot  %+v\nwant %+v", shards, got, want)
		}
	}
}

func TestLimitReturnsTopBacklinks(t *testing.T) {
	_, ix := build(t, testEdges, 4)
	got, err := ix.Lookup("github.com", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Backlinks) != 2 || got.Backlinks[0].Domain != "news.ycombinator.com" || got.LinkingDomains != 4 {
		t.Fatalf("got %+v", got)
	}
}

func TestSubdomainFallsBackToItsSite(t *testing.T) {
	_, ix := build(t, testEdges, 4)
	got, err := ix.Lookup("docs.github.com", 0)
	if err != nil || got == nil || got.Domain != "github.com" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

// Some registered domains really start with www., because the part after it is a public suffix.
func TestDomainThatStartsWithWWW(t *testing.T) {
	_, ix := build(t, append([]links.Edge{edge("github.com", "www.gov.uk", 2, 0)}, testEdges...), 4)
	for in, want := range map[string]string{"www.gov.uk": "www.gov.uk", "https://www.gov.uk/x": "www.gov.uk", "www.github.com": "github.com"} {
		got, err := ix.Lookup(in, 0)
		if err != nil || got == nil || got.Domain != want {
			t.Errorf("%s: got %+v, %v, want %s", in, got, err, want)
		}
	}
}

func TestDomainsWithoutBacklinks(t *testing.T) {
	_, ix := build(t, testEdges, 4)
	if got, err := ix.Lookup("reddit.com", 0); got != nil || err != nil {
		t.Fatalf("reddit.com has no backlinks, got %+v, %v", got, err)
	}
	if got, _ := ix.Lookup("nothing-here.com", 0); got != nil {
		t.Fatalf("unknown site, got %+v", got)
	}
	e, ok := ix.Find("reddit.com")
	if !ok || e.Score != 0 || e.LinkingDomains != 0 {
		t.Fatalf("reddit.com should still be known with its score: %+v %v", e, ok)
	}
}

func TestMissingScoreIsNull(t *testing.T) {
	_, ix := build(t, []links.Edge{edge("a.com", "b.com", 1, 0)}, 2)
	got, _ := ix.Lookup("b.com", 0)
	if got == nil || got.Score != nil {
		t.Fatalf("got %+v", got)
	}
}

func TestEveryDomainIsInTheShardItsHashPicks(t *testing.T) {
	_, ix := build(t, testEdges, 4)
	total := 0
	for i, s := range ix.shards {
		for _, e := range s.entries {
			if ShardOf(e.Domain, 4) != i {
				t.Errorf("%s is in shard %d, hash says %d", e.Domain, i, ShardOf(e.Domain, 4))
			}
			total++
		}
	}
	if total != ix.Manifest().Domains || total != 5 {
		t.Fatalf("%d domains across shards, manifest says %d", total, ix.Manifest().Domains)
	}
}

func TestBuildIsByteIdenticalWhateverTheInputOrder(t *testing.T) {
	a, _ := build(t, testEdges, 4)
	reversed := slices.Clone(testEdges)
	slices.Reverse(reversed)
	b, _ := build(t, reversed, 4)
	for _, name := range []string{manifestName, shardName(0), shardName(1), shardName(2), shardName(3)} {
		x, _ := os.ReadFile(filepath.Join(a, name))
		y, _ := os.ReadFile(filepath.Join(b, name))
		if !bytes.Equal(x, y) {
			t.Errorf("%s differs between builds", name)
		}
	}
}

func TestRebuildReplacesOldIndex(t *testing.T) {
	dir, _ := build(t, testEdges, 8)
	if _, err := Build(dir, testEdges, testScores, Options{Shards: 2}); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*"))
	if len(files) != 3 {
		t.Fatalf("want manifest + 2 shards, got %v", files)
	}
	leftovers, _ := filepath.Glob(dir + ".tmp-*")
	if len(leftovers) != 0 {
		t.Fatalf("temp folders left behind: %v", leftovers)
	}
}

func TestOpenRejectsDamagedShard(t *testing.T) {
	dir, _ := build(t, testEdges, 2)
	path := filepath.Join(dir, shardName(1))
	b, _ := os.ReadFile(path)
	os.WriteFile(path, b[:len(b)-3], 0o644)
	if _, err := Open(dir); err == nil {
		t.Fatal("expected an error for a cut-off shard")
	}
}

func TestBuildRejectsZeroShards(t *testing.T) {
	if _, err := Build(t.TempDir(), testEdges, nil, Options{}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"github.com":                        "github.com",
		"  HTTPS://www.GitHub.com/a?b#c ":   "www.github.com",
		"user@example.com:8080/path":        "example.com",
		"example.com.":                      "example.com",
		"http://docs.python.org/3/library/": "docs.python.org",
	} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOriginsAreStoredAndCapped(t *testing.T) {
	many := make([]merge.Origin, maxOrigins+5)
	for i := range many {
		many[i] = merge.Origin{File: fmt.Sprintf("x-%05d", i), Pages: len(many) - i}
	}
	dir := filepath.Join(t.TempDir(), "index")
	og := map[string][]merge.Origin{"github.com": many, "go.dev": {{File: "a:b", Pages: 4}}}
	if _, err := Build(dir, testEdges, testScores, Options{Shards: 3, Origins: og}); err != nil {
		t.Fatal(err)
	}
	ix, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()

	l, err := ix.Lookup("github.com", 1)
	if err != nil {
		t.Fatal(err)
	}
	if l.OriginFiles != maxOrigins+5 || !reflect.DeepEqual(l.Origins, many[:maxOrigins]) {
		t.Fatalf("got %d files, %d stored", l.OriginFiles, len(l.Origins))
	}
	if bl, _ := ix.Backlinks(mustFind(t, ix, "github.com"), 0); len(bl) != 4 {
		t.Fatalf("origins broke the backlink list: %d", len(bl))
	}
	l, _ = ix.Lookup("go.dev", 0)
	if !reflect.DeepEqual(l.Origins, og["go.dev"]) {
		t.Fatalf("file name with a colon: %+v", l.Origins)
	}
}

func TestDirectoryRowsWithoutOrigins(t *testing.T) {
	e, err := parseEntry("github.com\t100\t3\t64\t14\t0\t90", 2, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if e.LinkingPages != 64 || e.length != 90 || e.OriginFiles != 0 || e.olength != 0 {
		t.Fatalf("got %+v", e)
	}
}

func mustFind(t *testing.T, ix *Index, d string) Entry {
	t.Helper()
	e, ok := ix.Find(d)
	if !ok {
		t.Fatalf("%s not found", d)
	}
	return e
}
