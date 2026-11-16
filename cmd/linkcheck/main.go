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

	"github.com/vasishtk1/Linkscope/internal/gemini"
	"github.com/vasishtk1/Linkscope/internal/index"
	"github.com/vasishtk1/Linkscope/internal/linkcheck"
	"github.com/vasishtk1/Linkscope/internal/pagemeta"
	"github.com/vasishtk1/Linkscope/internal/redirect"
	"github.com/vasishtk1/Linkscope/internal/trace"
)

type row struct {
	linkcheck.Check
	Sites      int                  `json:"sites"`
	Suggestion *redirect.Suggestion `json:"suggestion,omitempty"`
}

type config struct {
	dir, site      string
	limit, workers int
	opts           linkcheck.Options
	asJSON         bool
	suggest        bool
	trace          bool
	gemini         bool
	geminiMax      int
	model          string
}

func main() {
	dir := flag.String("index", "data/demo/index", "index folder from cmd/index")
	site := flag.String("site", "", "site to check, like example.com")
	limit := flag.Int("limit", 30, "check this many of the site's most linked pages")
	workers := flag.Int("workers", 2, "requests in flight at once")
	interval := flag.Duration("interval", time.Second, "time between requests")
	timeout := flag.Duration("timeout", 10*time.Second, "time limit per request")
	asJSON := flag.Bool("json", false, "print JSON instead of a table")
	suggest := flag.Bool("suggest", false, "pick a live page to redirect each broken page to, using the site's sitemap")
	showTrace := flag.Bool("trace", false, "print each step as it happens (to stderr)")
	useGemini := flag.Bool("gemini", false, "ask Gemini to decide pages the address matching leaves unclear (implies -suggest)")
	geminiMax := flag.Int("gemini-max", 20, "most Gemini requests in one run")
	model := flag.String("gemini-model", gemini.DefaultModels, "Gemini models, tried in order when one is busy")
	flag.Parse()
	if *site == "" || flag.NArg() != 0 || *limit < 1 || *workers < 1 || *interval <= 0 || *geminiMax < 1 {
		flag.Usage()
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	cfg := config{
		dir: *dir, site: *site, limit: *limit, workers: *workers,
		opts:   linkcheck.Options{Timeout: *timeout, Interval: *interval},
		asJSON: *asJSON, suggest: *suggest || *useGemini,
		trace: *showTrace, gemini: *useGemini, geminiMax: *geminiMax, model: *model,
	}
	if err := run(ctx, cfg); err != nil {
		fmt.Fprintln(os.Stderr, "linkcheck:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config) error {
	dir, site := cfg.dir, cfg.site
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
	top = top[:min(cfg.limit, len(top))]
	pages := make([]string, len(top))
	for i, p := range top {
		pages[i] = p.Page
	}

	var out io.Writer
	if cfg.trace {
		out = os.Stderr
	}
	log := trace.New(out)
	var ai redirect.Picker
	if cfg.gemini {
		g, err := gemini.New(ctx, gemini.Key(".env"), cfg.model)
		if err != nil {
			return err
		}
		ai = g
	}

	c := linkcheck.New(cfg.opts)
	defer c.Close()
	start := time.Now()
	log.Say("start", "", "checking the %d most linked pages of %s", len(pages), e.Domain)
	checks := c.CheckEach(ctx, pages, cfg.workers, func(chk linkcheck.Check) {
		log.Say("check", chk.Page, "%s", describe(chk, e.Domain))
	})

	rows := make([]row, len(checks))
	for i := range checks {
		rows[i] = row{Check: checks[i], Sites: top[i].Sites}
	}
	var sitemaps map[string]redirect.Sitemap
	if cfg.suggest {
		sitemaps = suggest(ctx, c, rows, ai, cfg.geminiMax, log)
	}
	log.Say("done", "", "finished in %.1fs", time.Since(start).Seconds())
	took := time.Since(start)

	if cfg.asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		res := map[string]any{"site": e.Domain, "tookMs": took.Milliseconds(), "pages": rows, "steps": log.Events()}
		if sitemaps != nil {
			res["sitemaps"] = sitemaps
		}
		return enc.Encode(res)
	}
	printTable(os.Stdout, e.Domain, rows, cfg.suggest, took)
	printSitemaps(os.Stdout, sitemaps)
	return ctx.Err()
}

// suggest reads the sitemap of each host with a lost page once, then picks a redirect target for every lost page, asking ai (if set) about unclear ones.
func suggest(ctx context.Context, c *linkcheck.Checker, rows []row, ai redirect.Picker, aiMax int, log *trace.Log) map[string]redirect.Sitemap {
	sitemaps := map[string]redirect.Sitemap{}
	asked := 0
	for i := range rows {
		if !lost(rows[i].Check) || ctx.Err() != nil {
			continue
		}
		host, _, _ := strings.Cut(rows[i].Page, "/")
		sm, ok := sitemaps[host]
		if !ok {
			log.Say("sitemap", "", "reading the sitemap of %s to see which pages are live", host)
			sm = redirect.ReadSitemaps(ctx, c, c.Sitemaps(ctx, host))
			sitemaps[host] = sm
			log.Say("sitemap", "", "%d files read, %d live pages", sm.Files, len(sm.Pages))
		}
		s := redirect.Suggest(ctx, c, rows[i].Page, sm.Pages)
		log.Say("match", s.Page, "%s", matchStep(s))
		if s.Target == "" && ai != nil && len(sm.Pages) > 0 {
			if asked < aiMax {
				asked++
				s = redirect.Ask(ctx, ai, pagemeta.Reader{F: c}, c, s, sm.Pages, log)
			} else if asked == aiMax {
				asked++
				log.Say("ask", "", "reached the limit of %d Gemini requests, leaving the rest unclear", aiMax)
			}
		}
		rows[i].Suggestion = &s
	}
	return sitemaps
}

// matchStep describes the address matching's result in one line for the trace.
func matchStep(s redirect.Suggestion) string {
	if s.Target != "" {
		return fmt.Sprintf("comparing addresses: clear winner %s (score %.2f, shares %q), and it loads", bare(s.Target), s.Pick.Score, strings.Join(s.Pick.Shared, ", "))
	}
	if s.Pick != nil {
		return fmt.Sprintf("comparing addresses: no clear match (%s), closest %s (score %.2f)", s.NoMatch, bare(s.Pick.URL), s.Pick.Score)
	}
	return "comparing addresses: no clear match (" + s.NoMatch + ")"
}

func lost(c linkcheck.Check) bool {
	return c.Result == linkcheck.Broken || c.Result == linkcheck.Homepage
}

func printTable(w io.Writer, domain string, rows []row, withSuggestions bool, took time.Duration) {
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	if withSuggestions {
		fmt.Fprintln(tw, "PAGE\tLINKING SITES\tRESULT\tSEND VISITORS TO")
	} else {
		fmt.Fprintln(tw, "PAGE\tLINKING SITES\tRESULT")
	}
	counts := map[linkcheck.Result]int{}
	matched := 0
	for _, r := range rows {
		counts[r.Result]++
		line := fmt.Sprintf("%s\t%d\t%s", label(r.Page, domain), r.Sites, describe(r.Check, domain))
		if r.Suggestion == nil {
			if withSuggestions {
				line += "\t"
			}
			fmt.Fprintln(tw, line)
			continue
		}
		if r.Suggestion.Target != "" {
			matched++
		}
		target, why := explain(*r.Suggestion, domain)
		fmt.Fprintf(tw, "%s\t%s\n", line, target)
		for _, l := range why {
			fmt.Fprintf(tw, "\t\t\t  %s\n", l)
		}
	}
	tw.Flush()
	fmt.Fprintf(w, "\n%s: checked %d pages in %.1fs. %d ok, %d broken, %d redirect to the homepage, %d couldn't check, %d skipped by robots.txt\n",
		domain, len(rows), took.Seconds(), counts[linkcheck.OK], counts[linkcheck.Broken], counts[linkcheck.Homepage],
		counts[linkcheck.Unknown], counts[linkcheck.Skipped])
	if withSuggestions {
		fmt.Fprintf(w, "Redirect targets: %d of %d broken or homepage-redirected pages have a clear match\n",
			matched, counts[linkcheck.Broken]+counts[linkcheck.Homepage])
	}
}

// explain is the suggestion column plus the "why" lines printed under it.
func explain(s redirect.Suggestion, domain string) (string, []string) {
	var why []string
	if s.AI != nil {
		if s.Target != "" {
			return label(bare(s.Target), domain), []string{"picked by Gemini: " + s.AI.Reason}
		}
		why = append(why, "Gemini: "+s.AI.Reason)
		if s.AI.Choice != "" {
			why[0] = fmt.Sprintf("Gemini leaned to %s (%s confidence): %s", label(bare(s.AI.Choice), domain), s.AI.Confidence, s.AI.Reason)
		}
		return "no clear match (" + s.NoMatch + ")", why
	}
	if s.Pick != nil {
		w := fmt.Sprintf("why: shares %q", strings.Join(s.Pick.Shared, ", "))
		if s.Pick.SameSection {
			w += ", same section"
		}
		why = append(why, fmt.Sprintf("%s (score %.2f)", w, s.Pick.Score))
	}
	if s.RunnerUp != nil {
		why = append(why, fmt.Sprintf("runner-up: %s (score %.2f)", label(bare(s.RunnerUp.URL), domain), s.RunnerUp.Score))
	}
	if s.Target != "" {
		return label(bare(s.Target), domain), why
	}
	if s.Pick != nil {
		why[0] = strings.Replace(why[0], "why:", "closest: "+label(bare(s.Pick.URL), domain)+",", 1)
	}
	return "no clear match (" + s.NoMatch + ")", why
}

func printSitemaps(w io.Writer, sitemaps map[string]redirect.Sitemap) {
	for host, sm := range sitemaps {
		note := ""
		switch {
		case sm.Capped && len(sm.Pages) >= redirect.MaxPages:
			note = fmt.Sprintf(", stopped at the %d-page limit", redirect.MaxPages)
		case sm.Capped:
			note = fmt.Sprintf(", stopped at the %d-file limit", redirect.MaxSitemapFiles)
		}
		fmt.Fprintf(w, "Sitemap for %s: %d files read, %d failed, %d pages%s\n", host, sm.Files, sm.Failed, len(sm.Pages), note)
	}
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
