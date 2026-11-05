package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/vasishtk1/Linkscope/internal/index"
	"github.com/vasishtk1/Linkscope/internal/merge"
	"github.com/vasishtk1/Linkscope/internal/pagerank"
)

func main() {
	edgesPath := flag.String("edges", "data/merged.edges.tsv.gz", "merged link graph")
	scoresPath := flag.String("scores", "data/scores.tsv", "PageRank scores (optional; missing file means no scores)")
	out := flag.String("o", "data/index", "index folder to write")
	shards := flag.Int("shards", 8, "number of shard files")
	source := flag.String("source", "", "description of the data, shown on the website")
	flag.Parse()
	if flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(*edgesPath, *scoresPath, *out, *shards, *source); err != nil {
		fmt.Fprintln(os.Stderr, "index:", err)
		os.Exit(1)
	}
}

func run(edgesPath, scoresPath, out string, shards int, source string) error {
	start := time.Now()
	g, _, err := merge.Files([]string{edgesPath})
	if err != nil {
		return err
	}
	scores, err := readScores(scoresPath)
	if err != nil {
		return err
	}
	m, err := index.Build(out, g.Edges(), scores, index.Options{Shards: shards, Source: source})
	if err != nil {
		return err
	}
	var size int64
	for i := range shards {
		if st, err := os.Stat(fmt.Sprintf("%s/shard-%03d.idx", out, i)); err == nil {
			size += st.Size()
		}
	}
	fmt.Printf("edges:   %d\n", m.Edges)
	fmt.Printf("domains: %d (%d with scores)\n", m.Domains, len(scores))
	fmt.Printf("shards:  %d, %.1f MB total\n", m.Shards, float64(size)/1e6)
	fmt.Printf("time:    %.2fs\n", time.Since(start).Seconds())
	fmt.Printf("wrote %s\n", out)
	return nil
}

func readScores(path string) (map[string]int, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "index: %s not found, building without scores\n", path)
		return map[string]int{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rows, err := pagerank.ReadTSV(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	out := make(map[string]int, len(rows))
	for _, r := range rows {
		out[r.Domain] = r.Score
	}
	return out, nil
}
