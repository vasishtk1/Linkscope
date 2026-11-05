package pagerank

import (
	"bytes"
	"math"
	"reflect"
	"testing"

	"github.com/vasishtk1/Linkscope/internal/links"
)

func edge(src, dst string) links.Edge {
	return links.Edge{Src: src, Dst: dst, Pages: 1}
}

func rankOf(t *testing.T, r Result, domain string) float64 {
	t.Helper()
	for i, d := range r.Domains {
		if d == domain {
			return r.Rank[i]
		}
	}
	t.Fatalf("%s not in result", domain)
	return 0
}

func sum(xs []float64) float64 {
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s
}

// A links to B and C, B links to C, C links to A: the worked example in docs/reference/pagerank.md.
func TestThreeSiteExample(t *testing.T) {
	edges := []links.Edge{edge("a.com", "b.com"), edge("a.com", "c.com"), edge("b.com", "c.com"), edge("c.com", "a.com")}

	noDamping := Compute(edges, Options{Damping: 1, MaxIter: 1000, Tolerance: 1e-12})
	for domain, want := range map[string]float64{"a.com": 0.4, "b.com": 0.2, "c.com": 0.4} {
		if got := rankOf(t, noDamping, domain); math.Abs(got-want) > 1e-6 {
			t.Errorf("no damping: %s = %.6f, want %.1f", domain, got, want)
		}
	}

	r := Compute(edges, DefaultOptions())
	a, b, c := rankOf(t, r, "a.com"), rankOf(t, r, "b.com"), rankOf(t, r, "c.com")
	base := 0.15 / 3
	for name, diff := range map[string]float64{
		"a": a - (base + 0.85*c),
		"b": b - (base + 0.85*a/2),
		"c": c - (base + 0.85*(a/2+b)),
	} {
		if math.Abs(diff) > 1e-8 {
			t.Errorf("%s does not satisfy the PageRank equation (off by %g)", name, diff)
		}
	}
}

func TestRanksAddUpToOneWithDeadEnds(t *testing.T) {
	edges := []links.Edge{edge("a.com", "b.com"), edge("a.com", "c.com"), edge("d.com", "c.com")}
	r := Compute(edges, DefaultOptions())
	if s := sum(r.Rank); math.Abs(s-1) > 1e-9 {
		t.Fatalf("ranks sum to %f, want 1", s)
	}
	if r.Iterations >= DefaultOptions().MaxIter {
		t.Fatalf("did not converge (delta %g)", r.Delta)
	}
}

func TestLinkFromStrongSiteBeatsLinkFromWeakSite(t *testing.T) {
	edges := []links.Edge{
		edge("x1.com", "big.com"), edge("x2.com", "big.com"), edge("x3.com", "big.com"), edge("x4.com", "big.com"),
		edge("big.com", "lucky.com"),
		edge("nobody.com", "unlucky.com"),
	}
	r := Compute(edges, DefaultOptions())
	if rankOf(t, r, "lucky.com") <= rankOf(t, r, "unlucky.com") {
		t.Fatal("a link from a well-linked site should be worth more than a link from an unlinked one")
	}
}

func TestOneVotePerSiteNotPerPage(t *testing.T) {
	few := []links.Edge{edge("a.com", "t.com"), edge("b.com", "t.com")}
	many := []links.Edge{{Src: "a.com", Dst: "t.com", Pages: 5000}, {Src: "b.com", Dst: "t.com", Pages: 3}}
	if !reflect.DeepEqual(Compute(few, DefaultOptions()), Compute(many, DefaultOptions())) {
		t.Fatal("page counts should not change the result")
	}
}

func TestNofollowOnlyEdgesPassNoScore(t *testing.T) {
	edges := []links.Edge{
		{Src: "a.com", Dst: "followed.com", Pages: 2, NofollowPages: 1},
		{Src: "a.com", Dst: "nofollow.com", Pages: 2, NofollowPages: 2},
	}
	r := Compute(edges, DefaultOptions())
	if rankOf(t, r, "followed.com") <= rankOf(t, r, "nofollow.com") {
		t.Fatal("nofollow-only links should not pass score")
	}
	for i, d := range r.Domains {
		if d == "nofollow.com" && r.LinkingDomains[i] != 0 {
			t.Fatalf("nofollow.com linking domains = %d, want 0", r.LinkingDomains[i])
		}
	}
}

func TestInputOrderDoesNotChangeResult(t *testing.T) {
	edges := []links.Edge{edge("a.com", "b.com"), edge("c.com", "b.com"), edge("b.com", "a.com"), edge("d.com", "c.com")}
	reversed := make([]links.Edge, len(edges))
	for i := range edges {
		reversed[i] = edges[len(edges)-1-i]
	}
	if !reflect.DeepEqual(Compute(edges, DefaultOptions()), Compute(reversed, DefaultOptions())) {
		t.Fatal("edge order changed the result")
	}
}

func TestScoresRunFromZeroToHundred(t *testing.T) {
	edges := []links.Edge{
		edge("x1.com", "top.com"), edge("x2.com", "top.com"), edge("x3.com", "top.com"),
		edge("x1.com", "mid.com"),
	}
	s := Scores(Compute(edges, DefaultOptions()))
	if s[0].Domain != "top.com" || s[0].Score != 100 {
		t.Fatalf("first = %+v, want top.com at 100", s[0])
	}
	last := s[len(s)-1]
	if last.Score != 0 || last.LinkingDomains != 0 {
		t.Fatalf("last = %+v, want a site with no backlinks at 0", last)
	}
	for i := 1; i < len(s); i++ {
		if s[i].Score > s[i-1].Score {
			t.Fatalf("scores not sorted: %+v", s)
		}
	}
}

func TestEmptyGraph(t *testing.T) {
	if s := Scores(Compute(nil, DefaultOptions())); len(s) != 0 {
		t.Fatalf("got %v", s)
	}
}

func TestWriteTSV(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteTSV(&buf, []Score{{Domain: "a.com", Score: 87, Rank: 0.0123, LinkingDomains: 42}}); err != nil {
		t.Fatal(err)
	}
	want := "domain\tscore\tpagerank\tlinking_domains\na.com\t87\t1.230000e-02\t42\n"
	if buf.String() != want {
		t.Fatalf("got %q", buf.String())
	}
}
