package links

import (
	"net/url"
	"testing"
)

func TestDomain(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"https://www.nytimes.com/2026/article", "nytimes.com", true},
		{"https://blog.nytimes.com/", "nytimes.com", true},
		{"https://www.bbc.co.uk/news", "bbc.co.uk", true},
		{"http://EXAMPLE.COM:8080/x", "example.com", true},
		{"https://example.com./", "example.com", true},
		{"https://alice.github.io/", "alice.github.io", true},
		{"https://münchen.de/", "xn--mnchen-3ya.de", true},
		{"https://co.uk/", "", false},
		{"http://192.168.1.1/", "", false},
		{"http://[::1]/", "", false},
		{"http://printer.local/", "", false},
		{"http://localhost/", "", false},
		{"mailto:someone@example.com", "", false},
		{"javascript:void(0)", "", false},
		{"ftp://example.com/file", "", false},
	}
	for _, c := range cases {
		u, err := url.Parse(c.in)
		if err != nil {
			t.Fatalf("parse %q: %v", c.in, err)
		}
		got, ok := Domain(u)
		if got != c.want || ok != c.ok {
			t.Errorf("Domain(%q) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}
