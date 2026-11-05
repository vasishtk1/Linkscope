package index

import (
	"regexp"
	"strings"
)

type Lookup struct {
	Domain         string     `json:"domain"`
	Score          *int       `json:"score"`
	LinkingDomains int        `json:"linkingDomains"`
	LinkingPages   int        `json:"linkingPages"`
	NofollowPages  int        `json:"nofollowPages"`
	Backlinks      []Backlink `json:"backlinks"`
	Source         string     `json:"source"`
}

var scheme = regexp.MustCompile(`^[a-z][a-z0-9+.-]*://`)

// Normalize turns whatever the user typed ("https://www.Example.com/page") into a bare host.
func Normalize(input string) string {
	s := strings.ToLower(strings.TrimSpace(input))
	s = scheme.ReplaceAllString(s, "")
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, "@"); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.Index(s, ":"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSuffix(s, ".")
	return strings.TrimPrefix(s, "www.")
}

// Resolve finds the indexed domain with backlinks for a host, dropping subdomains until one matches.
func (ix *Index) Resolve(host string) (Entry, bool) {
	labels := strings.Split(host, ".")
	for i := 0; i < len(labels)-1; i++ {
		if e, ok := ix.Find(strings.Join(labels[i:], ".")); ok && e.LinkingDomains > 0 {
			return e, true
		}
	}
	return Entry{}, false
}

// Lookup answers a search box query; it returns nil when nothing links to the site.
func (ix *Index) Lookup(input string, limit int) (*Lookup, error) {
	e, ok := ix.Resolve(Normalize(input))
	if !ok {
		return nil, nil
	}
	bl, err := ix.Backlinks(e, limit)
	if err != nil {
		return nil, err
	}
	l := &Lookup{
		Domain: e.Domain, LinkingDomains: e.LinkingDomains, LinkingPages: e.LinkingPages,
		NofollowPages: e.NofollowPages, Backlinks: bl, Source: ix.m.Source,
	}
	if e.Score != noScore {
		l.Score = &e.Score
	}
	return l, nil
}
