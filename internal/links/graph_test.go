package links

import (
	"bytes"
	"reflect"
	"testing"
)

func page(src string, targets ...Target) Outlinks {
	u := "https://" + src + "/"
	return Outlinks{SourceURL: u + "p", SourceDomain: src, Targets: targets}
}

func TestPageCountedOncePerTargetDomain(t *testing.T) {
	g := NewGraph()
	g.Add(page("a.com",
		Target{Domain: "b.com", URL: "https://b.com/2"},
		Target{Domain: "b.com", URL: "https://b.com/1"},
		Target{Domain: "b.com", URL: "https://b.com/3"},
	))
	edges := g.Edges()
	if len(edges) != 1 || edges[0].Pages != 1 {
		t.Fatalf("edges = %+v", edges)
	}
	if edges[0].ExampleDst != "https://b.com/1" {
		t.Fatalf("example should be the smallest URL, got %q", edges[0].ExampleDst)
	}
}

func TestNofollowOnlyWhenEveryLinkIsNofollow(t *testing.T) {
	g := NewGraph()
	g.Add(page("a.com",
		Target{Domain: "b.com", URL: "https://b.com/", Nofollow: true},
		Target{Domain: "b.com", URL: "https://b.com/x"},
	))
	g.Add(Outlinks{SourceURL: "https://a.com/q", SourceDomain: "a.com", Targets: []Target{
		{Domain: "b.com", URL: "https://b.com/", Nofollow: true},
	}})
	e := g.Edges()[0]
	if e.Pages != 2 || e.NofollowPages != 1 {
		t.Fatalf("pages=%d nofollow=%d, want 2 and 1", e.Pages, e.NofollowPages)
	}
}

func TestEdgesIndependentOfInsertionOrder(t *testing.T) {
	pages := []Outlinks{
		{SourceURL: "https://a.com/z", SourceDomain: "a.com", Targets: []Target{{Domain: "b.com", URL: "https://b.com/z"}}},
		{SourceURL: "https://a.com/a", SourceDomain: "a.com", Targets: []Target{{Domain: "b.com", URL: "https://b.com/a"}, {Domain: "c.com", URL: "https://c.com/"}}},
		{SourceURL: "https://c.com/", SourceDomain: "c.com", Targets: []Target{{Domain: "a.com", URL: "https://a.com/"}}},
	}
	forward, backward := NewGraph(), NewGraph()
	for i := range pages {
		forward.Add(pages[i])
		backward.Add(pages[len(pages)-1-i])
	}
	if !reflect.DeepEqual(forward.Edges(), backward.Edges()) {
		t.Fatalf("order changed the result:\n%+v\n%+v", forward.Edges(), backward.Edges())
	}
	e := forward.Edges()[0]
	if e.Src != "a.com" || e.Dst != "b.com" || e.Pages != 2 || e.ExampleSrc != "https://a.com/a" {
		t.Fatalf("first edge = %+v", e)
	}
}

func TestWriteTSV(t *testing.T) {
	var buf bytes.Buffer
	err := WriteTSV(&buf, []Edge{{Src: "a.com", Dst: "b.com", Pages: 3, NofollowPages: 1, ExampleSrc: "https://a.com/", ExampleDst: "https://b.com/"}})
	if err != nil {
		t.Fatal(err)
	}
	want := "src\tdst\tpages\tnofollow_pages\texample_src\texample_dst\n" +
		"a.com\tb.com\t3\t1\thttps://a.com/\thttps://b.com/\n"
	if buf.String() != want {
		t.Fatalf("got %q", buf.String())
	}
}
