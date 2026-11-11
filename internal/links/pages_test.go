package links

import (
	"bytes"
	"reflect"
	"testing"
)

func TestPageKey(t *testing.T) {
	cases := map[string]string{
		"https://www.Example.com/About/?ref=x#top": "example.com/About",
		"http://example.com":                       "example.com/",
		"https://example.com/":                     "example.com/",
		"https://blog.example.com/post/":           "blog.example.com/post",
		"https://example.com/a%20b":                "example.com/a%20b",
		"https://example.com./x":                   "example.com/x",
	}
	for in, want := range cases {
		got, ok := PageKey(in)
		if !ok || got != want {
			t.Errorf("PageKey(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	if _, ok := PageKey("mailto:a@b.com"); ok {
		t.Error("a URL without a host should be rejected")
	}
}

func TestPageLinksCountEachSourcePageOncePerTargetPage(t *testing.T) {
	g := NewGraph()
	g.Add(page("a.com",
		Target{Domain: "b.com", URL: "https://b.com/x?utm=1"},
		Target{Domain: "b.com", URL: "https://www.b.com/x/"},
		Target{Domain: "b.com", URL: "https://b.com/y", Nofollow: true},
	))
	g.Add(Outlinks{SourceURL: "https://a.com/q", SourceDomain: "a.com", Targets: []Target{
		{Domain: "b.com", URL: "https://b.com/x", Nofollow: true},
	}})
	g.Add(page("c.com", Target{Domain: "b.com", URL: "https://b.com/x"}))

	want := []PageLink{
		{Dst: "b.com", Page: "b.com/x", Src: "a.com", Pages: 2, NofollowPages: 1},
		{Dst: "b.com", Page: "b.com/x", Src: "c.com", Pages: 1},
		{Dst: "b.com", Page: "b.com/y", Src: "a.com", Pages: 1, NofollowPages: 1},
	}
	if got := g.PageLinks(); !reflect.DeepEqual(got, want) {
		t.Fatalf("page links:\n got %+v\nwant %+v", got, want)
	}
}

func TestPagesTSVRoundTrip(t *testing.T) {
	in := []PageLink{
		{Dst: "b.com", Page: "b.com/x", Src: "a.com", Pages: 3, NofollowPages: 1},
		{Dst: "b.com", Page: "b.com/", Src: "c.com", Pages: 1},
	}
	var buf bytes.Buffer
	if err := WritePagesTSV(&buf, in); err != nil {
		t.Fatal(err)
	}
	var out []PageLink
	if err := ReadPagesTSV(&buf, func(p PageLink) error { out = append(out, p); return nil }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("got %+v, want %+v", out, in)
	}
}

func TestReadPagesTSVRejectsBadRows(t *testing.T) {
	for _, body := range []string{
		"",
		"wrong header\n",
		pagesHeader + "\nb.com\tb.com/x\ta.com\t0\t0\n",
		pagesHeader + "\nb.com\tb.com/x\ta.com\t1\t2\n",
		pagesHeader + "\nb.com\tb.com/x\ta.com\t1\n",
	} {
		err := ReadPagesTSV(bytes.NewBufferString(body), func(PageLink) error { return nil })
		if err == nil {
			t.Errorf("expected an error for %q", body)
		}
	}
}
