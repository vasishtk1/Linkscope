package links

import (
	"bufio"
	"fmt"
	"io"
	"sort"
)

type Edge struct {
	Src           string
	Dst           string
	Pages         int
	NofollowPages int
	ExampleSrc    string
	ExampleDst    string
}

type edgeKey struct{ src, dst string }

type Graph struct {
	edges map[edgeKey]*Edge
}

func NewGraph() *Graph {
	return &Graph{edges: map[edgeKey]*Edge{}}
}

// Add counts each page at most once per target domain, no matter how many links it has to it.
func (g *Graph) Add(o Outlinks) {
	type pageLink struct {
		followed bool
		example  string
	}
	perDomain := map[string]*pageLink{}
	for _, t := range o.Targets {
		pl, ok := perDomain[t.Domain]
		if !ok {
			pl = &pageLink{example: t.URL}
			perDomain[t.Domain] = pl
		}
		pl.followed = pl.followed || !t.Nofollow
		if t.URL < pl.example {
			pl.example = t.URL
		}
	}

	for dst, pl := range perDomain {
		k := edgeKey{o.SourceDomain, dst}
		e, ok := g.edges[k]
		if !ok {
			e = &Edge{Src: o.SourceDomain, Dst: dst, ExampleSrc: o.SourceURL, ExampleDst: pl.example}
			g.edges[k] = e
		}
		e.Pages++
		if !pl.followed {
			e.NofollowPages++
		}
		if o.SourceURL < e.ExampleSrc || (o.SourceURL == e.ExampleSrc && pl.example < e.ExampleDst) {
			e.ExampleSrc, e.ExampleDst = o.SourceURL, pl.example
		}
	}
}

func (g *Graph) Len() int { return len(g.edges) }

// Edges returns all edges sorted by source then destination.
func (g *Graph) Edges() []Edge {
	out := make([]Edge, 0, len(g.edges))
	for _, e := range g.edges {
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Src != out[j].Src {
			return out[i].Src < out[j].Src
		}
		return out[i].Dst < out[j].Dst
	})
	return out
}

func WriteTSV(w io.Writer, edges []Edge) error {
	bw := bufio.NewWriter(w)
	fmt.Fprintln(bw, "src\tdst\tpages\tnofollow_pages\texample_src\texample_dst")
	for _, e := range edges {
		fmt.Fprintf(bw, "%s\t%s\t%d\t%d\t%s\t%s\n", e.Src, e.Dst, e.Pages, e.NofollowPages, e.ExampleSrc, e.ExampleDst)
	}
	return bw.Flush()
}
