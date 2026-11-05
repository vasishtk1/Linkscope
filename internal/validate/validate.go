package validate

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/vasishtk1/Linkscope/internal/pagerank"
)

type Reference struct {
	PRPos int
	PR    float64
}

type Level struct {
	MinLinks int
	Domains  int
	Matched  int
	Spearman float64
}

type Row struct {
	OurPos         int
	Domain         string
	Score          int
	LinkingDomains int
	CCPos          int
}

type Report struct {
	ReferenceSize int
	Levels        []Level
	Top           []Row
}

// ReadRanks reads Common Crawl's domain ranks file, turning "com.facebook" back into "facebook.com".
func ReadRanks(r io.Reader) (map[string]Reference, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	out := map[string]Reference{}
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 5 {
			return nil, fmt.Errorf("line %d: want at least 5 fields, got %d", n, len(f))
		}
		pos, err := strconv.Atoi(f[2])
		if err != nil {
			return nil, fmt.Errorf("line %d: bad pr_pos %q", n, f[2])
		}
		pr, err := strconv.ParseFloat(f[3], 64)
		if err != nil {
			return nil, fmt.Errorf("line %d: bad pr_val %q", n, f[3])
		}
		out[reverseDomain(f[4])] = Reference{PRPos: pos, PR: pr}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no ranks found")
	}
	return out, nil
}

func reverseDomain(s string) string {
	parts := strings.Split(s, ".")
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, ".")
}

// Compare measures how closely our ranking agrees with Common Crawl's, at several minimum backlink counts.
func Compare(ours []pagerank.Score, ref map[string]Reference, minLinks []int, top int) Report {
	rep := Report{ReferenceSize: len(ref)}
	for _, m := range minLinks {
		lv := Level{MinLinks: m}
		var x, y []float64
		for _, s := range ours {
			if s.LinkingDomains < m {
				continue
			}
			lv.Domains++
			if r, ok := ref[s.Domain]; ok {
				x = append(x, s.Rank)
				y = append(y, r.PR)
			}
		}
		lv.Matched = len(x)
		lv.Spearman = Spearman(x, y)
		rep.Levels = append(rep.Levels, lv)
	}
	for i, s := range ours[:min(top, len(ours))] {
		rep.Top = append(rep.Top, Row{OurPos: i + 1, Domain: s.Domain, Score: s.Score, LinkingDomains: s.LinkingDomains, CCPos: ref[s.Domain].PRPos})
	}
	return rep
}

// Spearman is the rank correlation of x and y: 1 means the same order, 0 no relation, -1 reversed.
func Spearman(x, y []float64) float64 {
	if len(x) != len(y) || len(x) < 2 {
		return math.NaN()
	}
	return pearson(ranks(x), ranks(y))
}

// ranks gives each value its position (1 = smallest), averaging positions for ties.
func ranks(x []float64) []float64 {
	idx := make([]int, len(x))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return x[idx[a]] < x[idx[b]] })
	out := make([]float64, len(x))
	for i := 0; i < len(idx); {
		j := i
		for j+1 < len(idx) && x[idx[j+1]] == x[idx[i]] {
			j++
		}
		avg := float64(i+j)/2 + 1
		for k := i; k <= j; k++ {
			out[idx[k]] = avg
		}
		i = j + 1
	}
	return out
}

func pearson(x, y []float64) float64 {
	n := float64(len(x))
	var mx, my float64
	for i := range x {
		mx += x[i]
		my += y[i]
	}
	mx, my = mx/n, my/n
	var sxy, sxx, syy float64
	for i := range x {
		dx, dy := x[i]-mx, y[i]-my
		sxy += dx * dy
		sxx += dx * dx
		syy += dy * dy
	}
	if sxx == 0 || syy == 0 {
		return math.NaN()
	}
	return sxy / math.Sqrt(sxx*syy)
}
