package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/vasishtk1/Linkscope/internal/envfile"
	"github.com/vasishtk1/Linkscope/internal/github"
	"github.com/vasishtk1/Linkscope/internal/redirect"
	"github.com/vasishtk1/Linkscope/internal/redirectfile"
	"github.com/vasishtk1/Linkscope/internal/trace"
)

const branchPrefix = "linkscope/redirects-"

type checks struct {
	Site  string `json:"site"`
	Pages []page `json:"pages"`
}

type page struct {
	Page       string               `json:"page"`
	Sites      int                  `json:"sites"`
	Suggestion *redirect.Suggestion `json:"suggestion"`
}

type config struct {
	in, repo, create, env string
	open, trace           bool
	baseURL               string
	now                   func() time.Time
}

var newFiles = map[string]redirectfile.File{
	"vercel":  {Path: "vercel.json", Kind: redirectfile.Vercel},
	"netlify": {Path: "public/_redirects", Kind: redirectfile.Netlify},
}

func main() {
	var cfg config
	flag.StringVar(&cfg.in, "in", "-", "JSON from linkcheck -suggest -json (- for stdin)")
	flag.StringVar(&cfg.repo, "repo", "", "GitHub repo to open the pull request on, as owner/name")
	flag.StringVar(&cfg.create, "create", "", "if the repo has no redirect file, make one: vercel or netlify")
	flag.BoolVar(&cfg.open, "open", false, "open the pull request (otherwise just show what it would contain)")
	flag.BoolVar(&cfg.trace, "trace", false, "print each step to stderr")
	flag.StringVar(&cfg.env, "env", ".env", "file with GITHUB_APP_ID and GITHUB_APP_KEY")
	flag.Parse()
	if cfg.repo == "" || flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}
	if _, ok := newFiles[cfg.create]; cfg.create != "" && !ok {
		fmt.Fprintln(os.Stderr, "pr: -create must be vercel or netlify")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	var log *trace.Log
	if cfg.trace {
		log = trace.New(os.Stderr)
	}
	if err := run(ctx, os.Stdout, cfg, log); err != nil {
		fmt.Fprintln(os.Stderr, "pr:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, w io.Writer, cfg config, log *trace.Log) error {
	owner, name, ok := strings.Cut(cfg.repo, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return fmt.Errorf("-repo must look like owner/name, got %q", cfg.repo)
	}
	c, err := readChecks(cfg.in)
	if err != nil {
		return err
	}
	rs, fixed := redirects(c)
	if len(rs) == 0 {
		fmt.Fprintf(w, "%s: no broken pages with a redirect target, nothing to do\n", c.Site)
		return nil
	}

	app, err := loadApp(cfg)
	if err != nil {
		return err
	}
	log.Say("github", "", "signing in as the GitHub App on %s", cfg.repo)
	repo, err := app.Repo(ctx, owner, name)
	if err != nil {
		return err
	}
	base, err := repo.DefaultBranch(ctx)
	if err != nil {
		return err
	}
	open, err := repo.OpenPRs(ctx)
	if err != nil {
		return err
	}
	for _, pr := range open {
		if strings.HasPrefix(pr.Head.Ref, branchPrefix) {
			log.Say("github", "", "pull request #%d is already open, stopping", pr.Number)
			fmt.Fprintf(w, "A Linkscope pull request is already open: #%d %s\nMerge or close it first, so the same redirects aren't proposed twice.\n", pr.Number, pr.URL)
			return nil
		}
	}

	f, content, sha, err := findFile(ctx, repo, base, cfg.create)
	if err != nil {
		return err
	}
	log.Say("github", "", "read %s from %s", f.Path, base)
	res, err := redirectfile.Apply(f.Kind, content, rs)
	if err != nil {
		return fmt.Errorf("%s: %w", f.Path, err)
	}
	if len(res.Added) == 0 {
		fmt.Fprintf(w, "%s already has every redirect, nothing to do\n", f.Path)
		printSkipped(w, res.Skipped)
		return nil
	}

	title := prTitle(len(res.Added))
	body := prBody(c, res, fixed, f.Path)
	fmt.Fprintf(w, "Repo: %s (into %s)\nFile: %s\nTitle: %s\n\n%s\n\n--- %s after the change ---\n%s", cfg.repo, base, f.Path, title, body, f.Path, res.Content)
	if !cfg.open {
		fmt.Fprintln(w, "\nNothing was changed on GitHub. Run again with -open to open this pull request.")
		return nil
	}

	now := time.Now()
	if cfg.now != nil {
		now = cfg.now()
	}
	head, err := repo.BranchSHA(ctx, base)
	if err != nil {
		return err
	}
	branch, err := newBranch(ctx, repo, branchPrefix+now.Format("2006-01-02"), head)
	if err != nil {
		return err
	}
	log.Say("github", "", "created branch %s", branch)
	if err := repo.PutFile(ctx, f.Path, branch, title, res.Content, sha); err != nil {
		return err
	}
	log.Say("github", "", "saved %s on %s", f.Path, branch)
	pr, err := repo.CreatePR(ctx, title, body, branch, base)
	if err != nil {
		return err
	}
	log.Say("github", "", "opened pull request #%d", pr.Number)
	fmt.Fprintf(w, "\nOpened pull request #%d: %s\n", pr.Number, pr.URL)
	return nil
}

// redirects turns pages with a target into redirects, keyed by path so the description can find each page again.
func redirects(c checks) ([]redirectfile.Redirect, map[string]page) {
	var rs []redirectfile.Redirect
	fixed := map[string]page{}
	for _, p := range c.Pages {
		if p.Suggestion == nil || p.Suggestion.Target == "" {
			continue
		}
		r := redirectfile.New(p.Page, p.Suggestion.Target)
		rs = append(rs, r)
		fixed[r.From] = p
	}
	return rs, fixed
}

func loadApp(cfg config) (*github.App, error) {
	id := envfile.Get(cfg.env, "GITHUB_APP_ID")
	keyPath := envfile.Get(cfg.env, "GITHUB_APP_KEY")
	if id == "" || keyPath == "" {
		return nil, fmt.Errorf("set GITHUB_APP_ID and GITHUB_APP_KEY (path to the app's .pem file) in %s or the environment", cfg.env)
	}
	if strings.HasPrefix(keyPath, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			keyPath = home + keyPath[1:]
		}
	}
	key, err := github.LoadKey(keyPath)
	if err != nil {
		return nil, fmt.Errorf("reading the GitHub App key: %w", err)
	}
	return &github.App{ID: id, Key: key, BaseURL: cfg.baseURL}, nil
}

// findFile reads the first redirect file on the branch, or starts an empty one if create is set.
func findFile(ctx context.Context, repo *github.Repo, ref, create string) (redirectfile.File, []byte, string, error) {
	for _, f := range redirectfile.Places() {
		content, sha, err := repo.File(ctx, f.Path, ref)
		if err == nil {
			return f, content, sha, nil
		}
		if !errors.Is(err, github.ErrNotFound) {
			return f, nil, "", err
		}
	}
	if create != "" {
		return newFiles[create], nil, "", nil
	}
	return redirectfile.File{}, nil, "", fmt.Errorf("%w on %s (use -create vercel or -create netlify to make one)", redirectfile.ErrNoFile, ref)
}

// newBranch creates the branch, adding -2, -3... if today's name is taken by an earlier run.
func newBranch(ctx context.Context, repo *github.Repo, name, sha string) (string, error) {
	for i := 1; i <= 9; i++ {
		b := name
		if i > 1 {
			b = fmt.Sprintf("%s-%d", name, i)
		}
		err := repo.CreateBranch(ctx, b, sha)
		if err == nil {
			return b, nil
		}
		if !errors.Is(err, github.ErrExists) {
			return "", err
		}
	}
	return "", fmt.Errorf("branches %s to %s-9 all exist; delete old ones first", name, name)
}

func readChecks(in string) (checks, error) {
	var r io.Reader = os.Stdin
	if in != "-" {
		f, err := os.Open(in)
		if err != nil {
			return checks{}, err
		}
		defer f.Close()
		r = f
	}
	var c checks
	if err := json.NewDecoder(r).Decode(&c); err != nil {
		return checks{}, fmt.Errorf("reading linkcheck JSON: %w", err)
	}
	return c, nil
}

func printSkipped(w io.Writer, skipped []redirectfile.Skip) {
	for _, s := range skipped {
		fmt.Fprintf(w, "  skipped %s → %s (%s)\n", s.From, s.To, s.Reason)
	}
}
