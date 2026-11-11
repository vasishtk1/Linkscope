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
	pagesOut := flag.String("pages", "", "also write each site's most linked pages, from the workers' pages files (optional)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: merge [-in data/results] [-o data/merged.edges.tsv.gz] [-origins data/merged.origins.tsv.gz] [-pages data/merged.pages.tsv.gz]")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(*in, *out, *origins, *pagesOut); err != nil {
		fmt.Fprintln(os.Stderr, "merge:", err)
		os.Exit(1)
	}
}

func run(in, out, originsPath, pagesPath string) error {
	paths, err := merge.ResultFiles(in)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return errors.New("no *.edges.tsv.gz files in " + in)
	}
	if err := mergeEdges(paths, out, originsPath); err != nil {
		return err
	}
	if pagesPath == "" {
		return nil
	}
	return mergePages(paths, pagesPath)
}

// mergeEdges is its own function so the edge graph is freed before the page tables are built.
func mergeEdges(paths []string, out, originsPath string) error {
	start := time.Now()
	var (
		g       *links.Graph
		origins merge.Origins
		s       merge.Stats
		err     error
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

func mergePages(paths []string, out string) error {
	start := time.Now()
	sp, s, err := merge.Pages(paths)
	if err != nil {
		return err
	}
	if err := merge.WritePages(out, sp); err != nil {
		return err
	}
	fmt.Printf("pages files:   %d (%d results had none)\n", s.Files, s.Missing)
	fmt.Printf("page links:    %d rows read, %d unique site -> page pairs\n", s.Rows, s.Pairs)
	fmt.Printf("pages time:    %.2fs\n", time.Since(start).Seconds())
	fmt.Printf("wrote %s (%d sites)\n", out, len(sp))
	return nil
}
