package merge

import (
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/vasishtk1/Linkscope/internal/links"
)

const (
	MaxPages       = 200
	MaxPageSources = 10
	pagesHeader    = "dst\ttotal_pages\tpage\tsites\tpages\tnofollow_pages\tsources"
)

// PageSource is one site linking to a page, and how many of its pages do.
type PageSource struct {
	Domain string `json:"domain"`
	Pages  int    `json:"pages"`
}

// TopPage is one linked-to page: how many different sites and pages link to it, and the biggest of those sites.
type TopPage struct {
	Page          string       `json:"page"`
	Sites         int          `json:"sites"`
	Pages         int          `json:"pages"`
	NofollowPages int          `json:"nofollowPages"`
	Sources       []PageSource `json:"sources"`
}

// SitePages is a site's most linked pages (at most MaxPages), and how many of its pages get links in total.
type SitePages struct {
	Total int
	Top   []TopPage
}

type PageStats struct {
	Files   int
	Missing int
	Rows    int
	Pairs   int
}

// PagesPath is the page-level file a worker writes next to a result, e.g. X.edges.tsv.gz -> X.pages.tsv.gz.
func PagesPath(edgesPath string) string {
	return strings.TrimSuffix(edgesPath, ".edges.tsv.gz") + ".pages.tsv.gz"
}

// Pages reads the pages file next to each result and builds every site's top pages; results without one are counted as missing.
func Pages(paths []string) (map[string]SitePages, PageStats, error) {
	type pair struct{ page, src string }
	type counts struct{ pages, nofollow int }
	pairs := map[pair]*counts{}
	dstOf := map[string]string{}
	var s PageStats
	for _, p := range paths {
		pp := PagesPath(p)
		if _, err := os.Stat(pp); errors.Is(err, os.ErrNotExist) {
			s.Missing++
			continue
		}
		err := readPagesFile(pp, func(pl links.PageLink) {
			k := pair{pl.Page, pl.Src}
			c := pairs[k]
			if c == nil {
				c = &counts{}
				pairs[k] = c
				dstOf[pl.Page] = pl.Dst
			}
			c.pages += pl.Pages
			c.nofollow += pl.NofollowPages
			s.Rows++
		})
		if err != nil {
			return nil, s, fmt.Errorf("%s: %w", pp, err)
		}
		s.Files++
	}
	s.Pairs = len(pairs)

	byPage := map[string]*TopPage{}
	for k, c := range pairs {
		tp := byPage[k.page]
		if tp == nil {
			tp = &TopPage{Page: k.page}
			byPage[k.page] = tp
		}
		tp.Sites++
		tp.Pages += c.pages
		tp.NofollowPages += c.nofollow
		tp.Sources = append(tp.Sources, PageSource{Domain: k.src, Pages: c.pages})
	}
	clear(pairs)

	bySite := map[string][]TopPage{}
	for page, tp := range byPage {
		sortSources(tp.Sources)
		tp.Sources = tp.Sources[:min(len(tp.Sources), MaxPageSources)]
		bySite[dstOf[page]] = append(bySite[dstOf[page]], *tp)
	}
	out := make(map[string]SitePages, len(bySite))
	for dst, l := range bySite {
		sortPages(l)
		out[dst] = SitePages{Total: len(l), Top: l[:min(len(l), MaxPages)]}
	}
	return out, s, nil
}

func readPagesFile(path string, add func(links.PageLink)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	return links.ReadPagesTSV(gz, func(p links.PageLink) error { add(p); return nil })
}

// sortPages puts the pages linked from the most different sites first, then most linking pages, then by name.
func sortPages(l []TopPage) {
	sort.Slice(l, func(i, j int) bool {
		if l[i].Sites != l[j].Sites {
			return l[i].Sites > l[j].Sites
		}
		if l[i].Pages != l[j].Pages {
			return l[i].Pages > l[j].Pages
		}
		return l[i].Page < l[j].Page
	})
}

func sortSources(l []PageSource) {
	sort.Slice(l, func(i, j int) bool {
		if l[i].Pages != l[j].Pages {
			return l[i].Pages > l[j].Pages
		}
		return l[i].Domain < l[j].Domain
	})
}

// SourcesField packs a page's linking sites into one field: domain:pages pairs separated by commas.
func SourcesField(l []PageSource) string {
	var b strings.Builder
	for i, s := range l {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%s:%d", s.Domain, s.Pages)
	}
	return b.String()
}

// ParseSourcesField reads a field written by SourcesField.
func ParseSourcesField(f string) ([]PageSource, error) {
	if f == "" {
		return []PageSource{}, nil
	}
	parts := strings.Split(f, ",")
	out := make([]PageSource, 0, len(parts))
	for _, p := range parts {
		i := strings.LastIndexByte(p, ':')
		n, err := strconv.Atoi(p[i+1:])
		if i <= 0 || err != nil {
			return nil, fmt.Errorf("bad source %q", p)
		}
		out = append(out, PageSource{Domain: p[:i], Pages: n})
	}
	return out, nil
}

// WritePages saves every site's top pages, one row per page, sorted by site then rank.
func WritePages(path string, sp map[string]SitePages) error {
	dsts := make([]string, 0, len(sp))
	for d := range sp {
		dsts = append(dsts, d)
	}
	sort.Strings(dsts)
	return writeAtomic(path, func(w io.Writer) error {
		bw := bufio.NewWriter(w)
		fmt.Fprintln(bw, pagesHeader)
		for _, d := range dsts {
			s := sp[d]
			for _, p := range s.Top {
				fmt.Fprintf(bw, "%s\t%d\t%s\t%d\t%d\t%d\t%s\n", d, s.Total, p.Page, p.Sites, p.Pages, p.NofollowPages, SourcesField(p.Sources))
			}
		}
		return bw.Flush()
	})
}

// ReadPages loads a file written by WritePages.
func ReadPages(path string) (map[string]SitePages, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var r io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		r = gz
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	if !sc.Scan() || sc.Text() != pagesHeader {
		return nil, errors.New("missing pages header")
	}
	out := map[string]SitePages{}
	for line := 2; sc.Scan(); line++ {
		f := strings.Split(sc.Text(), "\t")
		if len(f) != 7 {
			return nil, fmt.Errorf("line %d: want 7 fields, got %d", line, len(f))
		}
		var nums [4]int
		for i, v := range []string{f[1], f[3], f[4], f[5]} {
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", line, err)
			}
			nums[i] = n
		}
		src, err := ParseSourcesField(f[6])
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		s := out[f[0]]
		s.Total = nums[0]
		s.Top = append(s.Top, TopPage{Page: f[2], Sites: nums[1], Pages: nums[2], NofollowPages: nums[3], Sources: src})
		out[f[0]] = s
	}
	return out, sc.Err()
}
