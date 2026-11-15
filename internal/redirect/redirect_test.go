package redirect

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/vasishtk1/Linkscope/internal/linkcheck"
)

func TestWords(t *testing.T) {
	cases := map[string][]string{
		"/blog/Go-Tips_2023.html":  {"blog", "go", "tips", "2023"},
		"/index.php?x":             {"x"},
		"/p/1234567/story-of-cats": {"p", "story", "of", "cats"},
		"/caf%C3%A9/menu":          {"café", "menu"},
		"/a/a/b":                   {"a", "b"},
		"/":                        nil,
	}
	for in, want := range cases {
		if got := words(in); !slices.Equal(got, want) {
			t.Errorf("words(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestRankAndClear(t *testing.T) {
	live := []string{
		"https://www.example.com/",
		"https://www.example.com/blog/go-tips/",
		"https://www.example.com/blog/go-faq",
		"https://www.example.com/blog/2023-in-review",
		"https://www.example.com/blog/go-tips-2023",
		"https://other.com/blog/go-tips",
	}
	got := Rank("example.com/blog/go-tips-2023", live)
	if len(got) == 0 || got[0].Path != "/blog/go-tips/" {
		t.Fatalf("best = %+v, want /blog/go-tips/", got)
	}
	if !slices.Equal(got[0].Shared, []string{"blog", "go", "tips"}) || !got[0].SameSection {
		t.Errorf("why = %v section %v", got[0].Shared, got[0].SameSection)
	}
	for _, c := range got {
		if c.Path == "/" || strings.Contains(c.URL, "other.com") || c.Path == "/blog/go-tips-2023" {
			t.Errorf("should not be a candidate: %s", c.URL)
		}
	}
	if !Clear(got) {
		t.Errorf("expected a clear winner: %+v", got[:2])
	}

	brand := Rank("fansided.com/mobile", []string{"https://fansided.com/fansided-mobile-app", "https://fansided.com/author/fansided"})
	if len(brand) != 1 || brand[0].Path != "/fansided-mobile-app" || !Clear(brand) {
		t.Errorf("the site's own name should not count as a word: %+v", brand)
	}

	if c := Rank("example.com/mobile", live); len(c) != 0 {
		t.Errorf("mobile should match nothing, got %+v", c)
	}

	close := Rank("example.com/guides/cats", []string{"https://example.com/guides/dogs", "https://example.com/guides/birds"})
	if Clear(close) {
		t.Errorf("two equally weak pages should not be clear: %+v", close)
	}
}

type fakeVerifier map[string]linkcheck.Result

func (f fakeVerifier) Check(_ context.Context, page string) linkcheck.Check {
	r, ok := f[page]
	if !ok {
		r = linkcheck.OK
	}
	return linkcheck.Check{Page: page, Result: r}
}

func TestSuggest(t *testing.T) {
	live := []string{
		"https://example.com/blog/go-tips",
		"https://example.com/blog/go-tips-guide-book",
		"https://example.com/shop/red-shoes",
		"https://example.com/shop/blue-shoes",
	}
	s := Suggest(context.Background(), fakeVerifier{}, "example.com/blog/go-tips-2023", live)
	if s.Target != "https://example.com/blog/go-tips" || s.Pick == nil || s.RunnerUp == nil {
		t.Errorf("got %+v", s)
	}

	dead := fakeVerifier{"example.com/blog/go-tips": linkcheck.Broken}
	s = Suggest(context.Background(), dead, "example.com/blog/go-tips-2023", live)
	if s.Target != "https://example.com/blog/go-tips-guide-book" {
		t.Errorf("should fall back to the next page when the first is broken, got %+v", s)
	}

	s = Suggest(context.Background(), fakeVerifier{}, "example.com/shop/green-shoes", live)
	if s.Target != "" || s.NoMatch != "top two are too close to call" {
		t.Errorf("got %+v", s)
	}

	s = Suggest(context.Background(), fakeVerifier{}, "example.com/careers", live)
	if s.Target != "" || s.NoMatch != "no live page shares a word with it" {
		t.Errorf("got %+v", s)
	}
}

type fakeFetcher map[string]string

func (f fakeFetcher) Get(_ context.Context, u string, _ int64) (int, []byte, error) {
	body, ok := f[u]
	if !ok {
		return 404, nil, nil
	}
	if body == "ERR" {
		return 0, nil, errors.New("boom")
	}
	return 200, []byte(body), nil
}

func gz(s string) string {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Write([]byte(s))
	w.Close()
	return b.String()
}

func TestReadSitemaps(t *testing.T) {
	f := fakeFetcher{
		"https://example.com/sitemap.xml": `<?xml version="1.0"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap><loc>https://example.com/posts.xml.gz</loc></sitemap>
  <sitemap><loc> https://example.com/pages.txt </loc></sitemap>
  <sitemap><loc>https://example.com/missing.xml</loc></sitemap>
  <sitemap><loc>https://example.com/broken.xml</loc></sitemap>
  <sitemap><loc>https://example.com/sitemap.xml</loc></sitemap>
</sitemapindex>`,
		"https://example.com/posts.xml.gz": gz(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://example.com/blog/a</loc></url>
  <url><loc>https://example.com/blog/b</loc></url>
</urlset>`),
		"https://example.com/pages.txt":  "https://example.com/about\nnot a url\nhttps://example.com/blog/a\n",
		"https://example.com/broken.xml": "ERR",
	}
	sm := ReadSitemaps(context.Background(), f, []string{"https://example.com/sitemap.xml"})
	want := []string{"https://example.com/blog/a", "https://example.com/blog/b", "https://example.com/about"}
	if !slices.Equal(sm.Pages, want) || sm.Files != 3 || sm.Failed != 2 || sm.Capped {
		t.Errorf("got pages %v files %d failed %d capped %v", sm.Pages, sm.Files, sm.Failed, sm.Capped)
	}
}

func TestReadSitemapsStopsAtFileLimit(t *testing.T) {
	f := fakeFetcher{}
	var idx strings.Builder
	idx.WriteString("<sitemapindex>")
	for i := range MaxSitemapFiles + 5 {
		u := "https://example.com/s" + string(rune('a'+i)) + ".xml"
		idx.WriteString("<sitemap><loc>" + u + "</loc></sitemap>")
		f[u] = "<urlset><url><loc>https://example.com/p" + string(rune('a'+i)) + "</loc></url></urlset>"
	}
	idx.WriteString("</sitemapindex>")
	f["https://example.com/sitemap.xml"] = idx.String()
	sm := ReadSitemaps(context.Background(), f, []string{"https://example.com/sitemap.xml"})
	if sm.Files != MaxSitemapFiles || !sm.Capped {
		t.Errorf("files %d capped %v", sm.Files, sm.Capped)
	}
}
