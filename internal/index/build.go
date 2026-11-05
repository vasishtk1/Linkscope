package index

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"

	"github.com/vasishtk1/Linkscope/internal/links"
)

const (
	manifestName = "manifest.json"
	magic        = "LSIDX001"
	footerSize   = 16
	noScore      = -1
)

type Manifest struct {
	Shards  int    `json:"shards"`
	Hash    string `json:"hash"`
	Domains int    `json:"domains"`
	Edges   int    `json:"edges"`
	Source  string `json:"source"`
}

type Options struct {
	Shards int
	Source string
}

// ShardOf picks which shard a domain lives in: FNV-1a hash of the name, modulo the shard count.
func ShardOf(domain string, shards int) int {
	h := fnv.New64a()
	h.Write([]byte(domain))
	return int(h.Sum64() % uint64(shards))
}

func shardName(i int) string { return fmt.Sprintf("shard-%03d.idx", i) }

// Build writes a fresh index into dir, replacing any index already there.
func Build(dir string, edges []links.Edge, scores map[string]int, o Options) (Manifest, error) {
	if o.Shards < 1 {
		return Manifest{}, errors.New("shards must be at least 1")
	}
	byDst := map[string][]Backlink{}
	all := map[string]bool{}
	for _, e := range edges {
		byDst[e.Dst] = append(byDst[e.Dst], Backlink{
			Domain: e.Src, Pages: e.Pages, NofollowPages: e.NofollowPages,
			ExampleSource: e.ExampleSrc, ExampleTarget: e.ExampleDst,
		})
		all[e.Src] = true
		all[e.Dst] = true
	}
	for d := range scores {
		all[d] = true
	}

	perShard := make([][]string, o.Shards)
	for d := range all {
		s := ShardOf(d, o.Shards)
		perShard[s] = append(perShard[s], d)
	}

	parent := filepath.Dir(filepath.Clean(dir))
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return Manifest{}, err
	}
	tmp, err := os.MkdirTemp(parent, filepath.Base(dir)+".tmp-*")
	if err != nil {
		return Manifest{}, err
	}
	defer os.RemoveAll(tmp)

	for i, domains := range perShard {
		sort.Strings(domains)
		if err := writeShard(filepath.Join(tmp, shardName(i)), domains, byDst, scores); err != nil {
			return Manifest{}, err
		}
	}
	m := Manifest{Shards: o.Shards, Hash: "fnv1a-64", Domains: len(all), Edges: len(edges), Source: o.Source}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return Manifest{}, err
	}
	if err := os.WriteFile(filepath.Join(tmp, manifestName), append(b, '\n'), 0o644); err != nil {
		return Manifest{}, err
	}
	if err := os.RemoveAll(dir); err != nil {
		return Manifest{}, err
	}
	return m, os.Rename(tmp, dir)
}

// writeShard writes each domain's backlinks, then the directory that says where each list starts, then the footer.
func writeShard(path string, domains []string, byDst map[string][]Backlink, scores map[string]int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	var pos int64
	dirRows := make([]Entry, 0, len(domains))
	for _, d := range domains {
		bl := byDst[d]
		sortBacklinks(bl)
		e := Entry{Domain: d, Score: noScore, LinkingDomains: len(bl), offset: pos}
		if s, ok := scores[d]; ok {
			e.Score = s
		}
		for _, b := range bl {
			e.LinkingPages += b.Pages
			e.NofollowPages += b.NofollowPages
			n, _ := fmt.Fprintf(w, "%s\t%d\t%d\t%s\t%s\n", b.Domain, b.Pages, b.NofollowPages, b.ExampleSource, b.ExampleTarget)
			pos += int64(n)
		}
		e.length = pos - e.offset
		dirRows = append(dirRows, e)
	}
	dirOffset := pos
	for _, e := range dirRows {
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\t%d\t%d\n", e.Domain, e.Score, e.LinkingDomains, e.LinkingPages, e.NofollowPages, e.offset, e.length)
	}
	var footer [footerSize]byte
	binary.BigEndian.PutUint64(footer[:8], uint64(dirOffset))
	copy(footer[8:], magic)
	w.Write(footer[:])
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// sortBacklinks orders by linking pages, most first, then by domain name so the order is always the same.
func sortBacklinks(bl []Backlink) {
	sort.Slice(bl, func(i, j int) bool {
		if bl[i].Pages != bl[j].Pages {
			return bl[i].Pages > bl[j].Pages
		}
		return bl[i].Domain < bl[j].Domain
	})
}
