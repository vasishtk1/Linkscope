package links

import (
	"net"
	"net/url"
	"strings"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
)

// Domain reduces a URL to its registered domain, e.g. blog.bbc.co.uk -> bbc.co.uk.
func Domain(u *url.URL) (string, bool) {
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", false
	}
	host := strings.TrimSuffix(u.Hostname(), ".")
	if host == "" || net.ParseIP(host) != nil {
		return "", false
	}
	host, err := idna.Lookup.ToASCII(host)
	if err != nil {
		return "", false
	}
	// Reject made-up TLDs like .local or .internal.
	if suffix, icann := publicsuffix.PublicSuffix(host); !icann && !strings.Contains(suffix, ".") {
		return "", false
	}
	d, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		return "", false
	}
	return d, true
}
