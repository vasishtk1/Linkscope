package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vasishtk1/Linkscope/internal/merge"
	"github.com/vasishtk1/Linkscope/internal/pagerank"
)

func main() {
	def := pagerank.DefaultOptions()
	in := flag.String("in", "data/merged.edges.tsv.gz", "merged link graph from cmd/merge")
	out := flag.String("o", "data/scores.tsv", "where to write the scores")
	damping := flag.Float64("damping", def.Damping, "share of score that flows through links")
	maxIter := flag.Int("max-iter", def.MaxIter, "stop after this many rounds")
	tol := flag.Float64("tol", def.Tolerance, "stop once the total change in a round is below this")
	top := flag.Int("top", 20, "print this many top domains")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: pagerank [-in merged.edges.tsv.gz] [-o scores.tsv] [flags]")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 0 || *damping <= 0 || *damping > 1 || *maxIter < 1 {
		flag.Usage()
		os.Exit(2)
	}
	opts := pagerank.Options{Damping: *damping, MaxIter: *maxIter, Tolerance: *tol}
	if err := run(*in, *out, opts, *top); err != nil {
		fmt.Fprintln(os.Stderr, "pagerank:", err)
		os.Exit(1)
	}
}

func run(in, out string, opts pagerank.Options, top int) error {
	start := time.Now()
	g, _, err := merge.Files([]string{in})
	if err != nil {
		return err
	}
	edges := g.Edges()
	loaded := time.Since(start)

	start = time.Now()
	r := pagerank.Compute(edges, opts)
	scores := pagerank.Scores(r)
	computed := time.Since(start)

	if err := writeScores(out, scores); err != nil {
		return err
	}

	fmt.Printf("domains:     %d\n", len(r.Domains))
	fmt.Printf("edges:       %d\n", len(edges))
	fmt.Printf("rounds:      %d (last change %.2e)\n", r.Iterations, r.Delta)
	fmt.Printf("time:        %.2fs to load, %.2fs to score\n", loaded.Seconds(), computed.Seconds())
	fmt.Printf("wrote %s\n", out)
	top = min(top, len(scores))
	fmt.Printf("\ntop %d domains:\n", top)
	for i, s := range scores[:top] {
		fmt.Printf("  %3d. %-34s score %3d   pagerank %.2e   %d linking sites\n", i+1, s.Domain, s.Score, s.Rank, s.LinkingDomains)
	}
	return nil
}

func writeScores(path string, scores []pagerank.Score) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := pagerank.WriteTSV(tmp, scores); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
