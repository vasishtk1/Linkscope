package pagemeta

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct{ name, html, title, desc string }{
		{"plain", `<html><head><title> Jobs &amp; Careers </title><meta name="description" content="Work with us."></head><body>x</body></html>`, "Jobs & Careers", "Work with us."},
		{"og fallback", `<head><meta property="og:title" content="OG Title"><meta property="og:description" content="OG desc"></head><body>`, "OG Title", "OG desc"},
		{"plain wins over og", `<head><meta property="og:title" content="OG"><title>Real</title></head>`, "Real", ""},
		{"no head", `just text`, "", ""},
		{"stops at body", `<head><title>T</title></head><body><meta name="description" content="late"></body>`, "T", ""},
		{"long text cut", `<title>` + strings.Repeat("a ", 400) + `</title>`, strings.TrimSpace(strings.Repeat("a ", 150)) + "…", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			title, desc := Parse([]byte(c.html))
			if title != c.title || desc != c.desc {
				t.Errorf("Parse = %q, %q; want %q, %q", title, desc, c.title, c.desc)
			}
		})
	}
}

type fakeFetcher map[string]string

func (f fakeFetcher) Get(_ context.Context, u string, _ int64) (int, []byte, error) {
	body, ok := f[u]
	if !ok {
		return 404, nil, nil
	}
	if body == "ERR" {
		return 0, nil, errors.New("connection failed")
	}
	return 200, []byte(body), nil
}

func TestArchived(t *testing.T) {
	f := fakeFetcher{
		archiveAPI + "example.com%2Fjobs&timestamp=" + crawlMonth:                 `{"archived_snapshots":{"closest":{"available":true,"status":"200","url":"http://web.archive.org/web/20230819142241/https://example.com/jobs/","timestamp":"20230819142241"}}}`,
		"https://web.archive.org/web/20230819142241id_/https://example.com/jobs/": `<title>Jobs at Example</title>`,
		archiveAPI + "example.com%2Fnone&timestamp=" + crawlMonth:                 `{"archived_snapshots":{}}`,
	}
	r := Reader{F: f}
	got := r.Archived(context.Background(), "example.com/jobs")
	if got.Title != "Jobs at Example" || got.Archived != "20230819" || got.URL != "https://example.com/jobs" {
		t.Errorf("Archived = %+v", got)
	}
	if got := r.Archived(context.Background(), "example.com/none"); got.Title != "" || got.Archived != "" {
		t.Errorf("no snapshot: %+v", got)
	}
}

func TestPage(t *testing.T) {
	r := Reader{F: fakeFetcher{"https://example.com/a": `<title>A</title>`, "https://example.com/b": "ERR"}}
	if got := r.Page(context.Background(), "https://example.com/a"); got.Title != "A" {
		t.Errorf("Page(a) = %+v", got)
	}
	for _, u := range []string{"https://example.com/b", "https://example.com/missing"} {
		if got := r.Page(context.Background(), u); got.URL != u || got.Title != "" {
			t.Errorf("Page(%s) = %+v", u, got)
		}
	}
}
