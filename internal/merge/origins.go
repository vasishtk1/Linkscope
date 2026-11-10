package merge

import (
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const originsHeader = "dst\tfile\tpages"

// Origin is one result file that had links to a site, and how many linking pages it had.
type Origin struct {
	File  string `json:"file"`
	Pages int    `json:"pages"`
}

// Origins maps each linked-to site to the pages linking to it, per result file.
type Origins map[string]map[string]int

func (o Origins) add(dst, file string, pages int) {
	m := o[dst]
	if m == nil {
		m = map[string]int{}
		o[dst] = m
	}
	m[file] += pages
}

// FileName is a result file's name without folder or extension, e.g. data/results/X-00042.edges.tsv.gz -> X-00042.
func FileName(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".edges.tsv.gz")
}

// List returns one site's origins, most linking pages first, then by file name.
func (o Origins) List(dst string) []Origin {
	out := make([]Origin, 0, len(o[dst]))
	for f, n := range o[dst] {
		out = append(out, Origin{File: f, Pages: n})
	}
	sortOrigins(out)
	return out
}

func sortOrigins(l []Origin) {
	sort.Slice(l, func(i, j int) bool {
		if l[i].Pages != l[j].Pages {
			return l[i].Pages > l[j].Pages
		}
		return l[i].File < l[j].File
	})
}

// WriteOrigins saves origins as dst, file, pages rows sorted by site, gzipped if path ends in .gz.
func WriteOrigins(path string, o Origins) error {
	dsts := make([]string, 0, len(o))
	for d := range o {
		dsts = append(dsts, d)
	}
	sort.Strings(dsts)
	return writeAtomic(path, func(w io.Writer) error {
		bw := bufio.NewWriter(w)
		fmt.Fprintln(bw, originsHeader)
		for _, d := range dsts {
			for _, x := range o.List(d) {
				fmt.Fprintf(bw, "%s\t%s\t%d\n", d, x.File, x.Pages)
			}
		}
		return bw.Flush()
	})
}

// ReadOrigins loads a file written by WriteOrigins into per-site lists, most linking pages first.
func ReadOrigins(path string) (map[string][]Origin, error) {
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
	if !sc.Scan() || sc.Text() != originsHeader {
		return nil, errors.New("missing origins header")
	}
	out := map[string][]Origin{}
	for line := 2; sc.Scan(); line++ {
		f := strings.Split(sc.Text(), "\t")
		if len(f) != 3 {
			return nil, fmt.Errorf("line %d: want 3 fields, got %d", line, len(f))
		}
		n, err := strconv.Atoi(f[2])
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		out[f[0]] = append(out[f[0]], Origin{File: f[1], Pages: n})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	for _, l := range out {
		sortOrigins(l)
	}
	return out, nil
}
