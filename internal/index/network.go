package index

import "sort"

type NetworkNode struct {
	Domain string `json:"domain"`
	Score  int    `json:"score"`
}

type NetworkLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Pages  int    `json:"pages"`
}

type Network struct {
	Nodes []NetworkNode `json:"nodes"`
	Links []NetworkLink `json:"links"`
}

// Network returns the n highest-scored sites that have backlinks, and the links among them.
func (ix *Index) Network(n int) (Network, error) {
	var top []Entry
	for _, s := range ix.shards {
		for _, e := range s.entries {
			if e.Score != noScore && e.LinkingDomains > 0 {
				top = append(top, e)
			}
		}
	}
	sort.Slice(top, func(i, j int) bool {
		a, b := top[i], top[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.LinkingDomains != b.LinkingDomains {
			return a.LinkingDomains > b.LinkingDomains
		}
		return a.Domain < b.Domain
	})
	if len(top) > n {
		top = top[:n]
	}

	nw := Network{Nodes: make([]NetworkNode, len(top)), Links: []NetworkLink{}}
	in := make(map[string]bool, len(top))
	for i, e := range top {
		nw.Nodes[i] = NetworkNode{Domain: e.Domain, Score: e.Score}
		in[e.Domain] = true
	}
	for _, e := range top {
		bl, err := ix.Backlinks(e, 0)
		if err != nil {
			return Network{}, err
		}
		for _, b := range bl {
			if in[b.Domain] {
				nw.Links = append(nw.Links, NetworkLink{Source: b.Domain, Target: e.Domain, Pages: b.Pages})
			}
		}
	}
	return nw, nil
}

// Top cuts a network down to its first n nodes and the links among them.
func (nw Network) Top(n int) Network {
	if n >= len(nw.Nodes) {
		return nw
	}
	out := Network{Nodes: nw.Nodes[:n], Links: []NetworkLink{}}
	in := make(map[string]bool, n)
	for _, nd := range out.Nodes {
		in[nd.Domain] = true
	}
	for _, l := range nw.Links {
		if in[l.Source] && in[l.Target] {
			out.Links = append(out.Links, l)
		}
	}
	return out
}
