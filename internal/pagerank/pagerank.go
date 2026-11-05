package pagerank

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"sort"

	"github.com/vasishtk1/Linkscope/internal/links"
)

type Options struct {
	Damping   float64
	MaxIter   int
	Tolerance float64
}

func DefaultOptions() Options {
	return Options{Damping: 0.85, MaxIter: 100, Tolerance: 1e-9}
}

type Result struct {
	Domains        []string
	Rank           []float64
	LinkingDomains []int
	Iterations     int
	Delta          float64
}

type Score struct {
	Domain         string
	Score          int
	Rank           float64
	LinkingDomains int
}

// Compute runs PageRank over the domain graph, counting each followed src -> dst pair as one vote.
func Compute(edges []links.Edge, o Options) Result {
	idx := map[string]int{}
	for _, e := range edges {
		idx[e.Src] = 0
		idx[e.Dst] = 0
	}
	domains := make([]string, 0, len(idx))
	for d := range idx {
		domains = append(domains, d)
	}
	sort.Strings(domains)
	for i, d := range domains {
		idx[d] = i
	}

	n := len(domains)
	res := Result{Domains: domains, Rank: make([]float64, n), LinkingDomains: make([]int, n)}
	if n == 0 {
		return res
	}

	type pair struct{ src, dst int }
	votes := make([]pair, 0, len(edges))
	for _, e := range edges {
		if e.Pages > e.NofollowPages {
			votes = append(votes, pair{idx[e.Src], idx[e.Dst]})
		}
	}
	sort.Slice(votes, func(i, j int) bool {
		if votes[i].src != votes[j].src {
			return votes[i].src < votes[j].src
		}
		return votes[i].dst < votes[j].dst
	})
	outDegree := make([]int, n)
	for _, v := range votes {
		outDegree[v.src]++
		res.LinkingDomains[v.dst]++
	}

	rank, next := res.Rank, make([]float64, n)
	for i := range rank {
		rank[i] = 1 / float64(n)
	}
	d := o.Damping
	for res.Iterations < o.MaxIter {
		dangling := 0.0
		for i, r := range rank {
			if outDegree[i] == 0 {
				dangling += r
			}
		}
		base := (1-d)/float64(n) + d*dangling/float64(n)
		for i := range next {
			next[i] = base
		}
		for _, v := range votes {
			next[v.dst] += d * rank[v.src] / float64(outDegree[v.src])
		}
		delta := 0.0
		for i := range rank {
			delta += math.Abs(next[i] - rank[i])
		}
		rank, next = next, rank
		res.Iterations++
		res.Delta = delta
		if delta < o.Tolerance {
			break
		}
	}
	res.Rank = rank
	return res
}

// Scores turns raw ranks into 0–100 on a log scale, sorted from highest to lowest.
func Scores(r Result) []Score {
	out := make([]Score, len(r.Domains))
	if len(out) == 0 {
		return out
	}
	lo, hi := r.Rank[0], r.Rank[0]
	for _, x := range r.Rank {
		lo, hi = min(lo, x), max(hi, x)
	}
	span := math.Log(hi) - math.Log(lo)
	for i, d := range r.Domains {
		s := 0
		if span > 0 {
			s = int(math.Round(100 * (math.Log(r.Rank[i]) - math.Log(lo)) / span))
		}
		out[i] = Score{Domain: d, Score: s, Rank: r.Rank[i], LinkingDomains: r.LinkingDomains[i]}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Rank != out[j].Rank {
			return out[i].Rank > out[j].Rank
		}
		return out[i].Domain < out[j].Domain
	})
	return out
}

func WriteTSV(w io.Writer, scores []Score) error {
	bw := bufio.NewWriter(w)
	fmt.Fprintln(bw, "domain\tscore\tpagerank\tlinking_domains")
	for _, s := range scores {
		fmt.Fprintf(bw, "%s\t%d\t%.6e\t%d\n", s.Domain, s.Score, s.Rank, s.LinkingDomains)
	}
	return bw.Flush()
}
