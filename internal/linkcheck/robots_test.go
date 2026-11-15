package linkcheck

import "testing"

func TestRobots(t *testing.T) {
	cases := []struct {
		name, text, path string
		want             bool
	}{
		{"empty file", "", "/anything", true},
		{"star group blocks", "User-agent: *\nDisallow: /private\n", "/private/a", false},
		{"star group allows the rest", "User-agent: *\nDisallow: /private\n", "/public", true},
		{"our group overrides star", "User-agent: *\nDisallow: /\n\nUser-agent: LinkscopeBot\nDisallow:\n", "/x", true},
		{"other bot's group ignored", "User-agent: googlebot\nDisallow: /\n", "/x", true},
		{"longest rule wins", "User-agent: *\nDisallow: /a\nAllow: /a/b\n", "/a/b/c", true},
		{"shorter rule still applies elsewhere", "User-agent: *\nDisallow: /a\nAllow: /a/b\n", "/a/c", false},
		{"allow wins a tie", "User-agent: *\nDisallow: /a\nAllow: /a\n", "/a", true},
		{"wildcard and end", "User-agent: *\nDisallow: /*.pdf$\n", "/docs/file.pdf", false},
		{"end anchor needs the end", "User-agent: *\nDisallow: /*.pdf$\n", "/docs/file.pdfx", true},
		{"comments and case", "user-AGENT: * # everyone\nDISALLOW: /tmp # scratch\n", "/tmp/x", false},
		{"group with several agents", "User-agent: foo\nUser-agent: linkscopebot\nDisallow: /f\n", "/f", false},
		{"rules before any agent ignored", "Disallow: /\nUser-agent: *\nDisallow: /p\n", "/x", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ParseRobots(c.text, robotsToken).Allowed(c.path); got != c.want {
				t.Errorf("Allowed(%q) = %v, want %v", c.path, got, c.want)
			}
		})
	}
}

func TestRobotsSitemaps(t *testing.T) {
	r := ParseRobots("Sitemap: https://example.com/a.xml\nUser-agent: *\nDisallow: /x\nsitemap: https://example.com/b.xml # news\n", robotsToken)
	if len(r.Sitemaps) != 2 || r.Sitemaps[0] != "https://example.com/a.xml" || r.Sitemaps[1] != "https://example.com/b.xml" {
		t.Errorf("Sitemaps = %v", r.Sitemaps)
	}
}
