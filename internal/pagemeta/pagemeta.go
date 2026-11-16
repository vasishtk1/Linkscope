package pagemeta

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

const (
	maxPage     = 256 << 10
	maxText     = 300
	archiveAPI  = "https://archive.org/wayback/available?url="
	crawlMonth  = "20261001"
	archiveHost = "web.archive.org"
)

// Info is what a page says it's about.
type Info struct {
	URL         string `json:"url"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Archived    string `json:"archived,omitempty"`
}

// Fetcher downloads a URL politely; linkcheck.Checker does this.
type Fetcher interface {
	Get(ctx context.Context, rawURL string, limit int64) (int, []byte, error)
}

// Reader reads page titles from live pages and from the Internet Archive.
type Reader struct{ F Fetcher }

// Page reads a live page's title and description; an unreadable page comes back with only its URL.
func (r Reader) Page(ctx context.Context, rawURL string) Info {
	info := Info{URL: rawURL}
	status, body, err := r.F.Get(ctx, rawURL, maxPage)
	if err != nil || status/100 != 2 {
		return info
	}
	info.Title, info.Description = Parse(body)
	return info
}

// Archived finds the Internet Archive's copy of a page closest to the crawl date and reads its title.
func (r Reader) Archived(ctx context.Context, page string) Info {
	info := Info{URL: "https://" + page}
	status, body, err := r.F.Get(ctx, archiveAPI+url.QueryEscape(page)+"&timestamp="+crawlMonth, 64<<10)
	if err != nil || status != 200 {
		return info
	}
	var res struct {
		Snapshots struct {
			Closest struct {
				Available bool   `json:"available"`
				Status    string `json:"status"`
				URL       string `json:"url"`
				Timestamp string `json:"timestamp"`
			} `json:"closest"`
		} `json:"archived_snapshots"`
	}
	if json.Unmarshal(body, &res) != nil {
		return info
	}
	c := res.Snapshots.Closest
	if !c.Available || c.Status != "200" || c.Timestamp == "" {
		return info
	}
	raw, err := rawSnapshot(c.URL, c.Timestamp)
	if err != nil {
		return info
	}
	status, body, err = r.F.Get(ctx, raw, maxPage)
	if err != nil || status/100 != 2 {
		return info
	}
	info.Title, info.Description = Parse(body)
	if info.Title != "" || info.Description != "" {
		info.Archived = c.Timestamp[:min(8, len(c.Timestamp))]
	}
	return info
}

// rawSnapshot turns a snapshot link into its "id_" form, which serves the page as it was, without the archive's toolbar.
func rawSnapshot(snapshot, timestamp string) (string, error) {
	u, err := url.Parse(snapshot)
	if err != nil {
		return "", err
	}
	u.Scheme, u.Host = "https", archiveHost
	u.Path = strings.Replace(u.Path, "/web/"+timestamp+"/", "/web/"+timestamp+"id_/", 1)
	return u.String(), nil
}

// Parse reads the title and description from a page's <head>, preferring og: tags only when the plain ones are missing.
func Parse(body []byte) (title, description string) {
	var ogTitle, ogDesc string
	z := html.NewTokenizer(bytes.NewReader(body))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return clean(first(title, ogTitle)), clean(first(description, ogDesc))
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			switch string(name) {
			case "title":
				if title == "" && z.Next() == html.TextToken {
					title = string(z.Text())
				}
			case "meta":
				if !hasAttr {
					continue
				}
				key, content := metaAttrs(z)
				switch key {
				case "description":
					description = first(description, content)
				case "og:title":
					ogTitle = first(ogTitle, content)
				case "og:description":
					ogDesc = first(ogDesc, content)
				}
			case "body":
				if title != "" || ogTitle != "" {
					return clean(first(title, ogTitle)), clean(first(description, ogDesc))
				}
			}
		}
	}
}

func metaAttrs(z *html.Tokenizer) (key, content string) {
	for {
		k, v, more := z.TagAttr()
		switch strings.ToLower(string(k)) {
		case "name", "property":
			key = strings.ToLower(string(v))
		case "content":
			content = string(v)
		}
		if !more {
			return key, content
		}
	}
}

func first(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// clean collapses whitespace and cuts long text, so a page can't fill the prompt.
func clean(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > maxText {
		s = strings.TrimSpace(string(r[:maxText])) + "…"
	}
	return s
}
