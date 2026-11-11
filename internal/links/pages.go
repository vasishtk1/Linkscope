package links

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// PageLink is how many pages on one site link to one exact page on another site.
type PageLink struct {
	Dst           string
	Page          string
	Src           string
	Pages         int
	NofollowPages int
}

type pageKey struct{ page, src string }

// PageKey turns a link target into the page it points at: host and path, without scheme, www., query or fragment.
func PageKey(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", false
	}
	host := strings.TrimPrefix(strings.ToLower(strings.TrimSuffix(u.Hostname(), ".")), "www.")
	if host == "" {
		return "", false
	}
	p := u.EscapedPath()
	if len(p) > 1 {
		p = strings.TrimRight(p, "/")
	}
	if p == "" {
		p = "/"
	}
	return host + p, true
}

func (g *Graph) addPages(o Outlinks) {
	type seen struct {
		dst      string
		followed bool
	}
	perPage := map[string]*seen{}
	for _, t := range o.Targets {
		page, ok := PageKey(t.URL)
		if !ok {
			continue
		}
		s, ok := perPage[page]
		if !ok {
			s = &seen{dst: t.Domain}
			perPage[page] = s
		}
		s.followed = s.followed || !t.Nofollow
	}
	for page, s := range perPage {
		k := pageKey{page, o.SourceDomain}
		p, ok := g.pages[k]
		if !ok {
			p = &PageLink{Dst: s.dst, Page: page, Src: o.SourceDomain}
			g.pages[k] = p
		}
		p.Pages++
		if !s.followed {
			p.NofollowPages++
		}
	}
}

// PageLinks returns every page link sorted by linked-to site, then page, then linking site.
func (g *Graph) PageLinks() []PageLink {
	out := make([]PageLink, 0, len(g.pages))
	for _, p := range g.pages {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Dst != b.Dst {
			return a.Dst < b.Dst
		}
		if a.Page != b.Page {
			return a.Page < b.Page
		}
		return a.Src < b.Src
	})
	return out
}

const pagesHeader = "dst\tpage\tsrc\tpages\tnofollow_pages"

func WritePagesTSV(w io.Writer, pl []PageLink) error {
	bw := bufio.NewWriter(w)
	fmt.Fprintln(bw, pagesHeader)
	for _, p := range pl {
		fmt.Fprintf(bw, "%s\t%s\t%s\t%d\t%d\n", p.Dst, p.Page, p.Src, p.Pages, p.NofollowPages)
	}
	return bw.Flush()
}

// ReadPagesTSV reads a file written by WritePagesTSV and calls fn for each row.
func ReadPagesTSV(r io.Reader, fn func(PageLink) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return err
		}
		return errors.New("empty file: missing header")
	}
	if sc.Text() != pagesHeader {
		return fmt.Errorf("line 1: unexpected header %q", sc.Text())
	}
	for n := 2; sc.Scan(); n++ {
		p, err := parsePageLink(sc.Text())
		if err != nil {
			return fmt.Errorf("line %d: %w", n, err)
		}
		if err := fn(p); err != nil {
			return err
		}
	}
	return sc.Err()
}

func parsePageLink(line string) (PageLink, error) {
	f := strings.Split(line, "\t")
	if len(f) != 5 {
		return PageLink{}, fmt.Errorf("want 5 fields, got %d", len(f))
	}
	pages, err := strconv.Atoi(f[3])
	if err != nil || pages < 1 {
		return PageLink{}, fmt.Errorf("bad pages %q", f[3])
	}
	nofollow, err := strconv.Atoi(f[4])
	if err != nil || nofollow < 0 || nofollow > pages {
		return PageLink{}, fmt.Errorf("bad nofollow_pages %q", f[4])
	}
	if f[0] == "" || f[1] == "" || f[2] == "" {
		return PageLink{}, errors.New("empty field")
	}
	return PageLink{Dst: f[0], Page: f[1], Src: f[2], Pages: pages, NofollowPages: nofollow}, nil
}
