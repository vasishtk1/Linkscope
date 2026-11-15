package redirect

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/vasishtk1/Linkscope/internal/linkcheck"
)

const (
	MaxSitemapFiles = 20
	MaxPages        = 50_000
	maxSitemapBytes = 50 << 20
)

// Sitemap is what a site's sitemaps list: its live pages, and how many sitemap files were read.
type Sitemap struct {
	Pages  []string `json:"-"`
	Files  int      `json:"files"`
	Failed int      `json:"failed"`
	Capped bool     `json:"capped"`
}

type sitemapDoc struct {
	URLs     []string `xml:"url>loc"`
	Sitemaps []string `xml:"sitemap>loc"`
}

// Fetcher is the part of linkcheck.Checker this package needs, so tests can use a fake.
type Fetcher interface {
	Get(ctx context.Context, rawURL string, limit int64) (int, []byte, error)
}

// ReadSitemaps follows sitemap index files from the start addresses, stopping at MaxSitemapFiles or MaxPages.
func ReadSitemaps(ctx context.Context, f Fetcher, start []string) Sitemap {
	var sm Sitemap
	queue := append([]string(nil), start...)
	seenFile := map[string]bool{}
	seenPage := map[string]bool{}
	for len(queue) > 0 && ctx.Err() == nil {
		u := strings.TrimSpace(queue[0])
		queue = queue[1:]
		if u == "" || seenFile[u] {
			continue
		}
		if sm.Files+sm.Failed >= MaxSitemapFiles || len(sm.Pages) >= MaxPages {
			sm.Capped = true
			break
		}
		seenFile[u] = true
		doc, err := readSitemap(ctx, f, u)
		if err != nil {
			sm.Failed++
			continue
		}
		sm.Files++
		queue = append(queue, doc.Sitemaps...)
		for _, p := range doc.URLs {
			p = strings.TrimSpace(p)
			if p != "" && !seenPage[p] && len(sm.Pages) < MaxPages {
				seenPage[p] = true
				sm.Pages = append(sm.Pages, p)
			}
		}
	}
	return sm
}

func readSitemap(ctx context.Context, f Fetcher, u string) (sitemapDoc, error) {
	status, body, err := f.Get(ctx, u, maxSitemapBytes)
	if err != nil {
		return sitemapDoc{}, err
	}
	if status < 200 || status >= 300 {
		return sitemapDoc{}, fmt.Errorf("%s: status %d", u, status)
	}
	return parseSitemap(body)
}

// parseSitemap reads an XML sitemap or sitemap index, gzipped or not, or a plain text list of URLs.
func parseSitemap(body []byte) (sitemapDoc, error) {
	if len(body) > 2 && body[0] == 0x1f && body[1] == 0x8b {
		zr, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return sitemapDoc{}, err
		}
		body, err = io.ReadAll(io.LimitReader(zr, maxSitemapBytes))
		if err != nil {
			return sitemapDoc{}, err
		}
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] != '<' {
		var doc sitemapDoc
		for _, line := range strings.Split(string(trimmed), "\n") {
			if line = strings.TrimSpace(line); strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") {
				doc.URLs = append(doc.URLs, line)
			}
		}
		return doc, nil
	}
	var doc sitemapDoc
	if err := xml.Unmarshal(trimmed, &doc); err != nil {
		return sitemapDoc{}, err
	}
	return doc, nil
}

var _ Fetcher = (*linkcheck.Checker)(nil)
