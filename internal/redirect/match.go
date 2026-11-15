package redirect

import (
	"net/url"
	"slices"
	"sort"
	"strings"
	"unicode"
)

const (
	MinScore     = 0.5
	MinLead      = 0.15
	sectionBonus = 0.1
)

var filler = map[string]bool{
	"html": true, "htm": true, "php": true, "asp": true, "aspx": true, "jsp": true,
	"index": true, "default": true, "amp": true, "www": true,
}

// Candidate is one live page scored against a broken one.
type Candidate struct {
	URL         string   `json:"url"`
	Path        string   `json:"path"`
	Score       float64  `json:"score"`
	Shared      []string `json:"shared"`
	SameSection bool     `json:"sameSection"`
}

// Rank scores every live page on the broken page's host, best first, dropping pages that share no words.
func Rank(page string, live []string) []Candidate {
	host, path := splitKey(page)
	site := strings.Split(host, ".")
	want := without(words(path), site)
	if len(want) == 0 {
		return nil
	}
	section := firstSegment(path)
	var out []Candidate
	for _, raw := range live {
		u, err := url.Parse(raw)
		if err != nil || bareHost(u.Hostname()) != host {
			continue
		}
		p := u.EscapedPath()
		if p == "" {
			p = "/"
		}
		if strings.TrimSuffix(p, "/") == strings.TrimSuffix(path, "/") {
			continue
		}
		have := without(words(p), site)
		shared := intersect(want, have)
		if len(shared) == 0 {
			continue
		}
		c := Candidate{URL: raw, Path: p, Shared: shared}
		c.Score = float64(len(shared)) / float64(len(union(want, have)))
		if section != "" && firstSegment(p) == section {
			c.SameSection = true
			c.Score += sectionBonus
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return len(out[i].Path) < len(out[j].Path)
	})
	return out
}

// Clear says whether the best candidate is good enough and far enough ahead of the next one.
func Clear(c []Candidate) bool {
	if len(c) == 0 || c[0].Score < MinScore {
		return false
	}
	return len(c) == 1 || c[0].Score-c[1].Score >= MinLead
}

// words splits a path into lowercase words: "/blog/Go-Tips_2023.html" → blog, go, tips, 2023.
func words(path string) []string {
	if p, err := url.PathUnescape(path); err == nil {
		path = p
	}
	parts := strings.FieldsFunc(strings.ToLower(path), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	var out []string
	for _, w := range parts {
		if filler[w] || (len(w) >= 5 && isDigits(w)) || slices.Contains(out, w) {
			continue
		}
		out = append(out, w)
	}
	return out
}

// firstSegment is the section a deep page sits in ("/blog/x" → "blog"); a top-level page has none.
func firstSegment(path string) string {
	seg, rest, ok := strings.Cut(strings.Trim(path, "/"), "/")
	if !ok || rest == "" {
		return ""
	}
	return strings.ToLower(seg)
}

func splitKey(page string) (host, path string) {
	host, path, ok := strings.Cut(page, "/")
	if !ok {
		return bareHost(host), "/"
	}
	return bareHost(host), "/" + path
}

func bareHost(h string) string { return strings.TrimPrefix(strings.ToLower(h), "www.") }

// without drops words that are just the site's own name ("fansided" on fansided.com), since they say nothing about the topic.
func without(ws, drop []string) []string {
	out := ws[:0:0]
	for _, w := range ws {
		if !slices.Contains(drop, w) {
			out = append(out, w)
		}
	}
	return out
}

func isDigits(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func intersect(a, b []string) []string {
	var out []string
	for _, w := range a {
		if slices.Contains(b, w) {
			out = append(out, w)
		}
	}
	return out
}

func union(a, b []string) []string {
	out := append([]string(nil), a...)
	for _, w := range b {
		if !slices.Contains(out, w) {
			out = append(out, w)
		}
	}
	return out
}
