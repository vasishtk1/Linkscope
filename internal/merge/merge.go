package merge

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vasishtk1/Linkscope/internal/links"
)

type Stats struct {
	Files     int
	EdgesRead int
}

// ResultFiles lists the finished worker results in dir, sorted by name.
func ResultFiles(dir string) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.edges.tsv.gz"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

// Files reads every file and combines repeated src -> dst pairs into one edge.
func Files(paths []string) (*links.Graph, Stats, error) {
	return read(paths, nil)
}

// FilesWithOrigins is Files that also notes which result files each site's links came from.
func FilesWithOrigins(paths []string) (*links.Graph, Origins, Stats, error) {
	o := Origins{}
	g, s, err := read(paths, o)
	return g, o, s, err
}

func read(paths []string, o Origins) (*links.Graph, Stats, error) {
	g := links.NewGraph()
	var s Stats
	for _, p := range paths {
		add := g.AddEdge
		if o != nil {
			name := FileName(p)
			add = func(e links.Edge) {
				g.AddEdge(e)
				o.add(e.Dst, name, e.Pages)
			}
		}
		n, err := readFile(p, add)
		if err != nil {
			return nil, s, fmt.Errorf("%s: %w", p, err)
		}
		s.Files++
		s.EdgesRead += n
	}
	return g, s, nil
}

func readFile(path string, add func(links.Edge)) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var r io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return 0, err
		}
		defer gz.Close()
		r = gz
	}

	n := 0
	err = links.ReadTSV(r, func(e links.Edge) error {
		add(e)
		n++
		return nil
	})
	return n, err
}

// WriteFile writes edges as TSV (gzipped if path ends in .gz) via a temp file and rename.
func WriteFile(path string, edges []links.Edge) error {
	return writeAtomic(path, func(w io.Writer) error { return links.WriteTSV(w, edges) })
}

// writeAtomic writes through a temp file and renames it into place, so a crash never leaves half a file.
func writeAtomic(path string, write func(io.Writer) error) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	var w io.Writer = tmp
	var gz *gzip.Writer
	if strings.HasSuffix(path, ".gz") {
		gz = gzip.NewWriter(tmp)
		w = gz
	}
	if err := write(w); err != nil {
		tmp.Close()
		return err
	}
	if gz != nil {
		if err := gz.Close(); err != nil {
			tmp.Close()
			return err
		}
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
