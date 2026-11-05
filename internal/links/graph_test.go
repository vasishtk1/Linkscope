package links

import (
	"bytes"
	"reflect"
	"strings"
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

func TestReadTSVRoundTrip(t *testing.T) {
	want := []Edge{
		{Src: "a.com", Dst: "b.com", Pages: 3, NofollowPages: 1, ExampleSrc: "https://a.com/", ExampleDst: "https://b.com/"},
		{Src: "a.com", Dst: "c.com", Pages: 1, NofollowPages: 0, ExampleSrc: "https://a.com/x", ExampleDst: "https://c.com/y"},
	}
	var buf bytes.Buffer
	if err := WriteTSV(&buf, want); err != nil {
		t.Fatal(err)
	}
	var got []Edge
	if err := ReadTSV(&buf, func(e Edge) error { got = append(got, e); return nil }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestReadTSVRejectsBadInput(t *testing.T) {
	header := "src\tdst\tpages\tnofollow_pages\texample_src\texample_dst\n"
	for name, in := range map[string]string{
		"empty":           "",
		"wrong header":    "a\tb\n",
		"too few fields":  header + "a.com\tb.com\t1\n",
		"bad pages":       header + "a.com\tb.com\tx\t0\tu\tv\n",
		"nofollow > page": header + "a.com\tb.com\t1\t2\tu\tv\n",
		"empty domain":    header + "\tb.com\t1\t0\tu\tv\n",
	} {
		if err := ReadTSV(strings.NewReader(in), func(Edge) error { return nil }); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestAddEdgeSumsCountsAndKeepsSmallestExample(t *testing.T) {
	a := Edge{Src: "a.com", Dst: "b.com", Pages: 2, NofollowPages: 1, ExampleSrc: "https://a.com/z", ExampleDst: "https://b.com/"}
	b := Edge{Src: "a.com", Dst: "b.com", Pages: 5, NofollowPages: 0, ExampleSrc: "https://a.com/a", ExampleDst: "https://b.com/q"}
	ab, ba := NewGraph(), NewGraph()
	ab.AddEdge(a)
	ab.AddEdge(b)
	ba.AddEdge(b)
	ba.AddEdge(a)
	if !reflect.DeepEqual(ab.Edges(), ba.Edges()) {
		t.Fatalf("order changed the result:\n%+v\n%+v", ab.Edges(), ba.Edges())
	}
	e := ab.Edges()[0]
	if e.Pages != 7 || e.NofollowPages != 1 || e.ExampleSrc != "https://a.com/a" || e.ExampleDst != "https://b.com/q" {
		t.Fatalf("merged edge = %+v", e)
	}
}
