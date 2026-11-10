package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/vasishtk1/Linkscope/internal/links"
	"github.com/vasishtk1/Linkscope/internal/merge"
)

func main() {
	in := flag.String("in", "data/results", "folder of worker result files")
	out := flag.String("o", "data/merged.edges.tsv.gz", "merged output file")
	origins := flag.String("origins", "", "also write which result files each site's links came from (optional)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: merge [-in data/results] [-o data/merged.edges.tsv.gz] [-origins data/merged.origins.tsv.gz]")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(*in, *out, *origins); err != nil {
		fmt.Fprintln(os.Stderr, "merge:", err)
		os.Exit(1)
	}
}

func run(in, out, originsPath string) error {
	paths, err := merge.ResultFiles(in)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return errors.New("no *.edges.tsv.gz files in " + in)
	}

	start := time.Now()
	var (
		g       *links.Graph
		origins merge.Origins
		s       merge.Stats
	)
	if originsPath != "" {
		g, origins, s, err = merge.FilesWithOrigins(paths)
	} else {
		g, s, err = merge.Files(paths)
	}
	if err != nil {
		return err
	}
	edges := g.Edges()
	if err := merge.WriteFile(out, edges); err != nil {
		return err
	}
	if originsPath != "" {
		if err := merge.WriteOrigins(originsPath, origins); err != nil {
			return err
		}
	}

	domains := map[string]bool{}
	pages := 0
	for _, e := range edges {
		domains[e.Src] = true
		domains[e.Dst] = true
		pages += e.Pages
	}
	fmt.Printf("files merged:  %d\n", s.Files)
	fmt.Printf("edges read:    %d\n", s.EdgesRead)
	fmt.Printf("merged edges:  %d\n", len(edges))
	fmt.Printf("domains:       %d\n", len(domains))
	fmt.Printf("linking pages: %d\n", pages)
	fmt.Printf("time:          %.2fs\n", time.Since(start).Seconds())
	fmt.Printf("wrote %s\n", out)
	if originsPath != "" {
		fmt.Printf("wrote %s (%d sites)\n", originsPath, len(origins))
	}
	return nil
}
