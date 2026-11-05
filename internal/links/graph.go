package links

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
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

// AddEdge merges an already-counted edge, e.g. one read back from a worker's result file.
func (g *Graph) AddEdge(e Edge) {
	k := edgeKey{e.Src, e.Dst}
	cur, ok := g.edges[k]
	if !ok {
		g.edges[k] = &e
		return
	}
	cur.Pages += e.Pages
	cur.NofollowPages += e.NofollowPages
	if e.ExampleSrc < cur.ExampleSrc || (e.ExampleSrc == cur.ExampleSrc && e.ExampleDst < cur.ExampleDst) {
		cur.ExampleSrc, cur.ExampleDst = e.ExampleSrc, e.ExampleDst
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

const tsvHeader = "src\tdst\tpages\tnofollow_pages\texample_src\texample_dst"

func WriteTSV(w io.Writer, edges []Edge) error {
	bw := bufio.NewWriter(w)
	fmt.Fprintln(bw, tsvHeader)
	for _, e := range edges {
		fmt.Fprintf(bw, "%s\t%s\t%d\t%d\t%s\t%s\n", e.Src, e.Dst, e.Pages, e.NofollowPages, e.ExampleSrc, e.ExampleDst)
	}
	return bw.Flush()
}

// ReadTSV reads a file written by WriteTSV and calls fn for each edge.
func ReadTSV(r io.Reader, fn func(Edge) error) error {
	br := bufio.NewReader(r)
	for n := 1; ; n++ {
		line, err := br.ReadString('\n')
		if errors.Is(err, io.EOF) && line == "" {
			if n == 1 {
				return errors.New("empty file: missing header")
			}
			return nil
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		line = strings.TrimSuffix(line, "\n")
		if n == 1 {
			if line != tsvHeader {
				return fmt.Errorf("line 1: unexpected header %q", line)
			}
			continue
		}
		e, perr := parseEdge(line)
		if perr != nil {
			return fmt.Errorf("line %d: %w", n, perr)
		}
		if ferr := fn(e); ferr != nil {
			return ferr
		}
	}
}

func parseEdge(line string) (Edge, error) {
	f := strings.Split(line, "\t")
	if len(f) != 6 {
		return Edge{}, fmt.Errorf("want 6 fields, got %d", len(f))
	}
	pages, err := strconv.Atoi(f[2])
	if err != nil || pages < 1 {
		return Edge{}, fmt.Errorf("bad pages %q", f[2])
	}
	nofollow, err := strconv.Atoi(f[3])
	if err != nil || nofollow < 0 || nofollow > pages {
		return Edge{}, fmt.Errorf("bad nofollow_pages %q", f[3])
	}
	if f[0] == "" || f[1] == "" {
		return Edge{}, errors.New("empty domain")
	}
	return Edge{Src: f[0], Dst: f[1], Pages: pages, NofollowPages: nofollow, ExampleSrc: f[4], ExampleDst: f[5]}, nil
}
