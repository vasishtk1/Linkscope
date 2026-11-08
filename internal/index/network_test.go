package index

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/vasishtk1/Linkscope/internal/links"
)

func TestNetworkPicksTopSitesAndLinksAmongThem(t *testing.T) {
	edges := []links.Edge{
		{Src: "b.com", Dst: "a.com", Pages: 5},
		{Src: "c.com", Dst: "a.com", Pages: 2},
		{Src: "a.com", Dst: "b.com", Pages: 3},
		{Src: "d.com", Dst: "b.com", Pages: 9},
		{Src: "a.com", Dst: "c.com", Pages: 1},
		{Src: "a.com", Dst: "e.com", Pages: 4},
	}
	scores := map[string]int{"a.com": 90, "b.com": 70, "c.com": 70, "d.com": 99, "e.com": 10}
	dir := filepath.Join(t.TempDir(), "index")
	if _, err := Build(dir, edges, scores, Options{Shards: 4}); err != nil {
		t.Fatal(err)
	}
	ix, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()

	nw, err := ix.Network(3)
	if err != nil {
		t.Fatal(err)
	}
	// d.com has the best score but nothing links to it; b.com beats c.com on linking domains.
	wantNodes := []NetworkNode{{"a.com", 90}, {"b.com", 70}, {"c.com", 70}}
	if !reflect.DeepEqual(nw.Nodes, wantNodes) {
		t.Fatalf("nodes = %v, want %v", nw.Nodes, wantNodes)
	}
	wantLinks := map[NetworkLink]bool{
		{"b.com", "a.com", 5}: true, {"c.com", "a.com", 2}: true,
		{"a.com", "b.com", 3}: true, {"a.com", "c.com", 1}: true,
	}
	if len(nw.Links) != len(wantLinks) {
		t.Fatalf("links = %v", nw.Links)
	}
	for _, l := range nw.Links {
		if !wantLinks[l] {
			t.Errorf("unexpected link %v", l)
		}
	}

	top := nw.Top(2)
	if len(top.Nodes) != 2 || len(top.Links) != 2 {
		t.Fatalf("Top(2) = %+v", top)
	}
	for _, l := range top.Links {
		if l.Source == "c.com" || l.Target == "c.com" {
			t.Errorf("Top(2) kept a link to a dropped site: %v", l)
		}
	}
	if !reflect.DeepEqual(nw.Top(10), nw) {
		t.Error("Top beyond the size should return the whole network")
	}
}
