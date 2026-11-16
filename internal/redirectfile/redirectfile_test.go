package redirectfile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	cases := []struct{ page, target, from, to string }{
		{"example.com/old", "https://www.example.com/new", "/old", "/new"},
		{"example.com/old", "https://example.com/new?x=1&y=2", "/old", "/new?x=1&y=2"},
		{"example.com/old", "https://example.com", "/old", "/"},
		{"blog.example.com/old", "https://example.com/new", "/old", "https://example.com/new"},
	}
	for _, c := range cases {
		if r := New(c.page, c.target); r.From != c.from || r.To != c.to {
			t.Errorf("New(%q, %q) = %+v, want %s → %s", c.page, c.target, r, c.from, c.to)
		}
	}
}

func TestFind(t *testing.T) {
	dir := t.TempDir()
	if _, err := Find(dir); !errors.Is(err, ErrNoFile) {
		t.Fatalf("empty repo: err = %v, want ErrNoFile", err)
	}
	write(t, dir, "netlify.toml", "")
	write(t, dir, "public/_redirects", "")
	if f, _ := Find(dir); f.Path != "public/_redirects" || f.Kind != Netlify {
		t.Errorf("Find = %+v, want public/_redirects", f)
	}
	write(t, dir, "vercel.json", "{}")
	if f, _ := Find(dir); f.Kind != Vercel {
		t.Errorf("Find = %+v, want vercel.json first", f)
	}
}

func TestVercel(t *testing.T) {
	in := `{
    "cleanUrls": true,
    "redirects": [
        { "source": "/kept", "destination": "/other?a=1&b=2", "permanent": false }
    ],
    "headers": []
}
`
	res, err := Apply(Vercel, []byte(in), []Redirect{
		{"/mobile", "/fansided-mobile-app"},
		{"/kept/", "/x"},
		{"/blog/:slug", "/x"},
		{"/mobile", "/again"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{
    "cleanUrls": true,
    "redirects": [
        {
            "source": "/mobile",
            "destination": "/fansided-mobile-app",
            "permanent": true
        },
        {
            "source": "/kept",
            "destination": "/other?a=1&b=2",
            "permanent": false
        }
    ],
    "headers": []
}
`
	if string(res.Content) != want {
		t.Errorf("got\n%s\nwant\n%s", res.Content, want)
	}
	if len(res.Added) != 1 || len(res.Skipped) != 3 {
		t.Fatalf("added %v, skipped %v", res.Added, res.Skipped)
	}
	for i, reason := range []string{"already redirected", "patterns", "already redirected"} {
		if !strings.Contains(res.Skipped[i].Reason, reason) {
			t.Errorf("skip %d reason %q, want %q", i, res.Skipped[i].Reason, reason)
		}
	}
}

func TestVercelNewFile(t *testing.T) {
	for _, in := range []string{"", `{"cleanUrls":true}`} {
		res, err := Apply(Vercel, []byte(in), []Redirect{{"/a", "https://other.com/b"}})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(res.Content), `"redirects": [`) || !strings.Contains(string(res.Content), `"destination": "https://other.com/b"`) {
			t.Errorf("from %q got\n%s", in, res.Content)
		}
	}
}

func TestVercelRefuses(t *testing.T) {
	for _, in := range []string{`{"routes":[]}`, `[1,2]`, `{"a":1} {}`, `{"a":`} {
		if _, err := Apply(Vercel, []byte(in), []Redirect{{"/a", "/b"}}); err == nil {
			t.Errorf("Apply(%q) had no error", in)
		}
	}
}

func TestNetlify(t *testing.T) {
	in := "# my rules\n/kept /new 301\n/*  /index.html  200"
	res, err := Apply(Netlify, []byte(in), []Redirect{{"/mobile", "/app"}, {"/kept", "/x"}})
	if err != nil {
		t.Fatal(err)
	}
	want := header + "\n/mobile /app 301\n\n# my rules\n/kept /new 301\n/*  /index.html  200\n"
	if string(res.Content) != want {
		t.Errorf("got\n%q\nwant\n%q", res.Content, want)
	}
	if len(res.Skipped) != 1 {
		t.Errorf("skipped %v", res.Skipped)
	}
}

func TestTOML(t *testing.T) {
	in := "[build]\n  publish = \"dist\"\n\n[[redirects]]\n  from = \"/kept\"\n  to = \"/new\"\n"
	res, err := Apply(NetlifyTOML, []byte(in), []Redirect{{"/mobile", "/app"}, {"/kept", "/x"}})
	if err != nil {
		t.Fatal(err)
	}
	want := "[build]\n  publish = \"dist\"\n\n" + header + "\n[[redirects]]\n  from = \"/mobile\"\n  to = \"/app\"\n  status = 301\n\n[[redirects]]\n  from = \"/kept\"\n  to = \"/new\"\n"
	if string(res.Content) != want {
		t.Errorf("got\n%s\nwant\n%s", res.Content, want)
	}

	res, _ = Apply(NetlifyTOML, []byte("[build]\n  publish = \"dist\""), []Redirect{{"/a", "/b"}})
	want = "[build]\n  publish = \"dist\"\n\n" + header + "\n[[redirects]]\n  from = \"/a\"\n  to = \"/b\"\n  status = 301\n"
	if string(res.Content) != want {
		t.Errorf("no existing blocks: got\n%s\nwant\n%s", res.Content, want)
	}
}

func TestNothingToAdd(t *testing.T) {
	in := []byte(`{"redirects":[{"source":"/a","destination":"/b","permanent":true}]}`)
	res, err := Apply(Vercel, in, []Redirect{{"/a", "/c"}, {"/", "/x"}, {"/d", "/d/"}, {"/e f", "/g"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(res.Content) != string(in) || len(res.Added) != 0 || len(res.Skipped) != 4 {
		t.Errorf("content changed or wrong counts: %s %v %v", res.Content, res.Added, res.Skipped)
	}
}

func write(t *testing.T, dir, name, text string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
