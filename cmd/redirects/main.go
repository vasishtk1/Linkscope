package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/vasishtk1/Linkscope/internal/redirect"
	"github.com/vasishtk1/Linkscope/internal/redirectfile"
)

type checks struct {
	Site  string `json:"site"`
	Pages []struct {
		Page       string               `json:"page"`
		Sites      int                  `json:"sites"`
		Suggestion *redirect.Suggestion `json:"suggestion"`
	} `json:"pages"`
}

var newFiles = map[string]redirectfile.File{
	"vercel":  {Path: "vercel.json", Kind: redirectfile.Vercel},
	"netlify": {Path: "public/_redirects", Kind: redirectfile.Netlify},
}

func main() {
	in := flag.String("in", "-", "JSON from linkcheck -suggest -json (- for stdin)")
	repo := flag.String("repo", "", "folder of the site's repo")
	create := flag.String("create", "", "if the repo has no redirect file, make one: vercel or netlify")
	write := flag.Bool("write", false, "save the file (otherwise just show what would change)")
	flag.Parse()
	if *repo == "" || flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}
	if _, ok := newFiles[*create]; *create != "" && !ok {
		fmt.Fprintln(os.Stderr, "redirects: -create must be vercel or netlify")
		os.Exit(2)
	}
	if err := run(os.Stdout, *in, *repo, *create, *write); err != nil {
		fmt.Fprintln(os.Stderr, "redirects:", err)
		os.Exit(1)
	}
}

func run(w io.Writer, in, repo, create string, write bool) error {
	c, err := readChecks(in)
	if err != nil {
		return err
	}
	var rs []redirectfile.Redirect
	sites := map[string]int{}
	for _, p := range c.Pages {
		if p.Suggestion == nil || p.Suggestion.Target == "" {
			continue
		}
		r := redirectfile.New(p.Page, p.Suggestion.Target)
		rs = append(rs, r)
		sites[r.From] = p.Sites
	}
	if len(rs) == 0 {
		fmt.Fprintf(w, "%s: no broken pages with a clear redirect target, nothing to do\n", c.Site)
		return nil
	}

	f, err := redirectfile.Find(repo)
	var content []byte
	switch {
	case errors.Is(err, redirectfile.ErrNoFile) && create != "":
		f = newFiles[create]
	case err != nil:
		return fmt.Errorf("%s: %w (use -create vercel or -create netlify to make one)", repo, err)
	}
	path := filepath.Join(repo, filepath.FromSlash(f.Path))
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
		if content, err = os.ReadFile(path); err != nil {
			return err
		}
	}
	res, err := redirectfile.Apply(f.Kind, content, rs)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "%s: %s to add\n", f.Path, plural(len(res.Added), "redirect"))
	for _, r := range res.Added {
		fmt.Fprintf(w, "  %s → %s  (%s)\n", r.From, r.To, plural(sites[r.From], "linking site"))
	}
	if len(res.Skipped) > 0 {
		fmt.Fprintln(w, "Skipped:")
		for _, s := range res.Skipped {
			fmt.Fprintf(w, "  %s → %s  (%s)\n", s.From, s.To, s.Reason)
		}
	}
	if len(res.Added) == 0 {
		return nil
	}
	if !write {
		fmt.Fprintln(w, "Nothing saved. Run again with -write to save the file.")
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, res.Content, mode); err != nil {
		return err
	}
	fmt.Fprintf(w, "Saved %s\n", path)
	return nil
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

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
