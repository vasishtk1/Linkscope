package links

import (
	"net/url"
	"strings"

	"github.com/vasishtk1/Linkscope/internal/wat"
)

type Target struct {
	Domain   string
	URL      string
	Nofollow bool
}

type Outlinks struct {
	SourceURL    string
	SourceDomain string
	Targets      []Target
}

// Extract returns the page's links to other domains; ok=false if the page should be skipped.
func Extract(p wat.Page) (Outlinks, bool) {
	if p.Status < 200 || p.Status > 299 {
		return Outlinks{}, false
	}
	pageURL, err := url.Parse(p.URL)
	if err != nil {
		return Outlinks{}, false
	}
	src, ok := Domain(pageURL)
	if !ok {
		return Outlinks{}, false
	}

	base := pageURL
	if p.Base != "" {
		if b, err := pageURL.Parse(strings.TrimSpace(p.Base)); err == nil {
			base = b
		}
	}

	out := Outlinks{SourceURL: pageURL.String(), SourceDomain: src}
	for _, l := range p.Links {
		if l.Path != "A@/href" && l.Path != "AREA@/href" {
			continue
		}
		u, err := base.Parse(strings.TrimSpace(l.URL))
		if err != nil {
			continue
		}
		dst, ok := Domain(u)
		if !ok || dst == src {
			continue
		}
		u.Fragment = ""
		out.Targets = append(out.Targets, Target{Domain: dst, URL: u.String(), Nofollow: isNofollow(l.Rel)})
	}
	return out, true
}

func isNofollow(rel string) bool {
	for _, token := range strings.Fields(strings.ToLower(rel)) {
		if token == "nofollow" || token == "sponsored" || token == "ugc" {
			return true
		}
	}
	return false
}
