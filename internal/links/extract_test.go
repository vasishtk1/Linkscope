package links

import (
	"reflect"
	"testing"

	"github.com/vasishtk1/Linkscope/internal/wat"
)

func a(href string) wat.Link { return wat.Link{Path: "A@/href", URL: href} }

func TestExtract(t *testing.T) {
	page := wat.Page{
		URL:    "https://pinchofyum.com/best-pans",
		Status: 200,
		Links: []wat.Link{
			a("https://ourplace.com/always-pan#reviews"),
			a("/about"),
			a("https://www.pinchofyum.com/contact"),
			a("//shop.amazon.com/pan"),
			{Path: "A@/href", URL: "https://ads.example.com/", Rel: "Sponsored noopener"},
			{Path: "IMG@/src", URL: "https://cdn.images.com/pan.jpg"},
			{Path: "LINK@/href", URL: "https://fonts.googleapis.com/css"},
			a("mailto:hi@pinchofyum.com"),
			a("  https://padded.com/  "),
		},
	}
	got, ok := Extract(page)
	if !ok {
		t.Fatal("expected page to be used")
	}
	if got.SourceDomain != "pinchofyum.com" {
		t.Fatalf("source domain = %q", got.SourceDomain)
	}
	want := []Target{
		{Domain: "ourplace.com", URL: "https://ourplace.com/always-pan"},
		{Domain: "amazon.com", URL: "https://shop.amazon.com/pan"},
		{Domain: "example.com", URL: "https://ads.example.com/", Nofollow: true},
		{Domain: "padded.com", URL: "https://padded.com/"},
	}
	if !reflect.DeepEqual(got.Targets, want) {
		t.Fatalf("targets =\n%+v\nwant\n%+v", got.Targets, want)
	}
}

func TestExtractResolvesAgainstBaseTag(t *testing.T) {
	page := wat.Page{
		URL:    "https://site.com/blog/post",
		Status: 200,
		Base:   "https://cdn-mirror.net/",
		Links:  []wat.Link{a("page.html")},
	}
	got, _ := Extract(page)
	if len(got.Targets) != 1 || got.Targets[0].URL != "https://cdn-mirror.net/page.html" {
		t.Fatalf("targets = %+v", got.Targets)
	}
}

func TestExtractSkipsNon2xx(t *testing.T) {
	for _, status := range []int{0, 301, 404, 500} {
		if _, ok := Extract(wat.Page{URL: "https://a.com/", Status: status}); ok {
			t.Errorf("status %d should be skipped", status)
		}
	}
}

func TestExtractSkipsPagesWithoutValidDomain(t *testing.T) {
	if _, ok := Extract(wat.Page{URL: "http://10.0.0.1/", Status: 200}); ok {
		t.Error("IP-address page should be skipped")
	}
}
