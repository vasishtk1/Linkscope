package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"sort"
	"time"

	"github.com/vasishtk1/Linkscope/internal/extract"
	"github.com/vasishtk1/Linkscope/internal/links"
)

func main() {
	out := flag.String("o", "", "write all edges as TSV to this file")
	top := flag.Int("top", 15, "print this many top edges")
	workers := flag.Int("workers", runtime.NumCPU(), "number of parser goroutines")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: extract [-o edges.tsv] [-top N] [-workers N] file.warc.wat.gz")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(flag.Arg(0), *out, *top, *workers); err != nil {
		fmt.Fprintln(os.Stderr, "extract:", err)
		os.Exit(1)
	}
}

func run(path, outPath string, top, workers int) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}

	start := time.Now()
	g, s, err := extract.Run(f, workers)
	if err != nil {
		return err
	}
	elapsed := time.Since(start)
	edges := g.Edges()

	if outPath != "" {
		if err := writeEdges(outPath, edges); err != nil {
			return err
		}
	}
	printReport(s, edges, top, workers, elapsed, float64(info.Size())/1e6)
	return nil
}

func writeEdges(path string, edges []links.Edge) error {
	o, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := links.WriteTSV(o, edges); err != nil {
		o.Close()
		return err
	}
	return o.Close()
}

func printReport(s extract.Stats, edges []links.Edge, top, workers int, elapsed time.Duration, compressedMB float64) {
	srcs, dsts := map[string]bool{}, map[string]bool{}
	for _, e := range edges {
		srcs[e.Src] = true
		dsts[e.Dst] = true
	}
	secs := elapsed.Seconds()
	fmt.Printf("records:            %d\n", s.Records)
	fmt.Printf("pages (responses):  %d\n", s.Responses)
	fmt.Printf("pages used (2xx):   %d\n", s.PagesUsed)
	fmt.Printf("cross-domain links: %d\n", s.CrossLinks)
	fmt.Printf("domain edges:       %d\n", len(edges))
	fmt.Printf("source domains:     %d\n", len(srcs))
	fmt.Printf("target domains:     %d\n", len(dsts))
	fmt.Printf("parse errors:       %d\n", s.ParseErrors)
	fmt.Printf("time:               %.2fs with %d workers (%.0f pages/s, %.1f MB/s compressed)\n",
		secs, workers, float64(s.Responses)/secs, compressedMB/secs)

	sort.SliceStable(edges, func(i, j int) bool { return edges[i].Pages > edges[j].Pages })
	top = min(top, len(edges))
	fmt.Printf("\ntop %d edges by linking pages:\n", top)
	for _, e := range edges[:top] {
		fmt.Printf("  %-32s -> %-28s %6d pages (%d nofollow)\n", e.Src, e.Dst, e.Pages, e.NofollowPages)
	}
}
