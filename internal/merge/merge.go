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
	g := links.NewGraph()
	var s Stats
	for _, p := range paths {
		n, err := readFile(p, g)
		if err != nil {
			return nil, s, fmt.Errorf("%s: %w", p, err)
		}
		s.Files++
		s.EdgesRead += n
	}
	return g, s, nil
}

func readFile(path string, g *links.Graph) (int, error) {
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
		g.AddEdge(e)
		n++
		return nil
	})
	return n, err
}

// WriteFile writes edges as TSV (gzipped if path ends in .gz) via a temp file and rename.
func WriteFile(path string, edges []links.Edge) error {
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
	if err := links.WriteTSV(w, edges); err != nil {
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
