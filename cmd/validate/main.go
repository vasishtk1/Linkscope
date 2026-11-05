package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/vasishtk1/Linkscope/internal/pagerank"
	"github.com/vasishtk1/Linkscope/internal/validate"
)

func main() {
	scores := flag.String("scores", "data/scores.tsv", "our scores from cmd/pagerank")
	ranks := flag.String("ranks", "data/cc-domain-ranks.tsv", "Common Crawl's domain ranks (make cc-ranks)")
	levels := flag.String("min-links", "1,5,20,100", "compare only domains with at least this many linking sites (comma-separated)")
	top := flag.Int("top", 20, "show this many of our top domains next to Common Crawl's positions")
	flag.Parse()
	minLinks, err := parseLevels(*levels)
	if err != nil || flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(*scores, *ranks, minLinks, *top); err != nil {
		fmt.Fprintln(os.Stderr, "validate:", err)
		os.Exit(1)
	}
}

func parseLevels(s string) ([]int, error) {
	var out []int
	for _, p := range strings.Split(s, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 0 {
			return nil, fmt.Errorf("bad level %q", p)
		}
		out = append(out, n)
	}
	return out, nil
}

func run(scoresPath, ranksPath string, minLinks []int, top int) error {
	sf, err := os.Open(scoresPath)
	if err != nil {
		return err
	}
	defer sf.Close()
	ours, err := pagerank.ReadTSV(sf)
	if err != nil {
		return fmt.Errorf("%s: %w", scoresPath, err)
	}

	rf, err := os.Open(ranksPath)
	if err != nil {
		return fmt.Errorf("%w (run make cc-ranks first)", err)
	}
	defer rf.Close()
	ref, err := validate.ReadRanks(rf)
	if err != nil {
		return fmt.Errorf("%s: %w", ranksPath, err)
	}

	rep := validate.Compare(ours, ref, minLinks, top)
	fmt.Printf("our domains: %d   Common Crawl reference: top %d domains\n\n", len(ours), rep.ReferenceSize)
	fmt.Println("min linking sites   our domains   also in CC list   rank agreement (Spearman)")
	for _, lv := range rep.Levels {
		fmt.Printf("%17d   %11d   %15d   %25.3f\n", lv.MinLinks, lv.Domains, lv.Matched, lv.Spearman)
	}
	fmt.Printf("\nour top %d next to Common Crawl's PageRank position:\n", len(rep.Top))
	for _, r := range rep.Top {
		cc := "not in list"
		if r.CCPos > 0 {
			cc = fmt.Sprintf("#%d", r.CCPos)
		}
		fmt.Printf("  %3d. %-32s score %3d   %5d linking sites   CC %s\n", r.OurPos, r.Domain, r.Score, r.LinkingDomains, cc)
	}
	return nil
}
