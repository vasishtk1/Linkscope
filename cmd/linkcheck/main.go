package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/vasishtk1/Linkscope/internal/index"
	"github.com/vasishtk1/Linkscope/internal/linkcheck"
	"github.com/vasishtk1/Linkscope/internal/merge"
)

type row struct {
	linkcheck.Check
	Sites int `json:"sites"`
}

func main() {
	dir := flag.String("index", "data/demo/index", "index folder from cmd/index")
	site := flag.String("site", "", "site to check, like example.com")
	limit := flag.Int("limit", 30, "check this many of the site's most linked pages")
	workers := flag.Int("workers", 2, "requests in flight at once")
	interval := flag.Duration("interval", time.Second, "time between requests")
	timeout := flag.Duration("timeout", 10*time.Second, "time limit per request")
	asJSON := flag.Bool("json", false, "print JSON instead of a table")
	flag.Parse()
	if *site == "" || flag.NArg() != 0 || *limit < 1 || *workers < 1 || *interval <= 0 {
		flag.Usage()
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	o := linkcheck.Options{Timeout: *timeout, Interval: *interval}
	if err := run(ctx, *dir, *site, *limit, *workers, o, *asJSON); err != nil {
		fmt.Fprintln(os.Stderr, "linkcheck:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, dir, site string, limit, workers int, o linkcheck.Options, asJSON bool) error {
	ix, err := index.Open(dir)
	if err != nil {
		return err
	}
	defer ix.Close()
	e, ok := ix.Resolve(index.Normalize(site))
	if !ok {
		return fmt.Errorf("nothing links to %s in %s", site, dir)
	}
	top, err := ix.TopPages(e)
	if err != nil {
		return err
	}
	if len(top) == 0 {
		return fmt.Errorf("%s has no page-level data for %s (built before page links were added?)", dir, e.Domain)
	}
	top = top[:min(limit, len(top))]
	pages := make([]string, len(top))
	for i, p := range top {
		pages[i] = p.Page
	}

	c := linkcheck.New(o)
	defer c.Close()
	start := time.Now()
	checks := c.CheckAll(ctx, pages, workers)
	took := time.Since(start)

	rows := make([]row, len(checks))
	for i := range checks {
		rows[i] = row{Check: checks[i], Sites: top[i].Sites}
	}
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{"site": e.Domain, "tookMs": took.Milliseconds(), "pages": rows})
	}
	printTable(os.Stdout, e.Domain, top, rows, took)
	return ctx.Err()
}

func printTable(w io.Writer, domain string, top []merge.TopPage, rows []row, took time.Duration) {
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "PAGE\tLINKING SITES\tRESULT")
	counts := map[linkcheck.Result]int{}
	for _, r := range rows {
		counts[r.Result]++
		fmt.Fprintf(tw, "%s\t%d\t%s\n", label(r.Page, domain), r.Sites, describe(r.Check, domain))
	}
	tw.Flush()
	fmt.Fprintf(w, "\n%s: checked %d pages in %.1fs. %d ok, %d broken, %d redirect to the homepage, %d couldn't check, %d skipped by robots.txt\n",
		domain, len(rows), took.Seconds(), counts[linkcheck.OK], counts[linkcheck.Broken], counts[linkcheck.Homepage],
		counts[linkcheck.Unknown], counts[linkcheck.Skipped])
}

func describe(c linkcheck.Check, domain string) string {
	switch c.Result {
	case linkcheck.OK:
		if moved(c) {
			return "ok, redirects to " + label(bare(c.FinalURL), domain)
		}
		return "ok"
	case linkcheck.Broken:
		if moved(c) {
			return fmt.Sprintf("broken (%d after redirect to %s)", c.Status, label(bare(c.FinalURL), domain))
		}
		return fmt.Sprintf("broken (%d)", c.Status)
	case linkcheck.Homepage:
		return "redirects to the homepage"
	case linkcheck.Skipped:
		return "skipped (" + c.Reason + ")"
	default:
		return "couldn't check (" + c.Reason + ")"
	}
}

// moved is true when a redirect goes to a different page, not just https, www or a trailing slash.
func moved(c linkcheck.Check) bool {
	return c.FinalURL != "" && same(bare(c.FinalURL)) != same(c.Page)
}

func bare(rawURL string) string {
	if _, rest, ok := strings.Cut(rawURL, "://"); ok {
		return rest
	}
	return rawURL
}

func same(page string) string {
	page, _, _ = strings.Cut(page, "?")
	return strings.TrimSuffix(strings.TrimPrefix(page, "www."), "/")
}

// label shows just the path for pages on the site itself, and host + path for subdomains.
func label(page, domain string) string {
	host, path, _ := strings.Cut(page, "/")
	host = strings.TrimPrefix(host, "www.")
	if host == domain {
		return "/" + path
	}
	return strings.TrimSuffix(host+"/"+path, "/")
}
