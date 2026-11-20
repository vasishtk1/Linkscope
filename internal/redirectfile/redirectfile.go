package redirectfile

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Kind is which redirect file format a repo uses.
type Kind string

const (
	Vercel      Kind = "vercel.json"
	Netlify     Kind = "_redirects"
	NetlifyTOML Kind = "netlify.toml"
)

// places is where each file is looked for, in order; the first one found is used.
var places = []struct {
	path string
	kind Kind
}{
	{"vercel.json", Vercel},
	{"public/_redirects", Netlify},
	{"static/_redirects", Netlify},
	{"_redirects", Netlify},
	{"netlify.toml", NetlifyTOML},
}

var ErrNoFile = errors.New("no vercel.json, _redirects or netlify.toml found")

// File is a redirect file in a repo; Path is relative to the repo folder.
type File struct {
	Path string `json:"path"`
	Kind Kind   `json:"kind"`
}

// Redirect sends visitors from a path on the site to a path on the same site or a full URL elsewhere.
type Redirect struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Skip is a redirect that wasn't added, and why.
type Skip struct {
	Redirect
	Reason string `json:"reason"`
}

// Result is the new file content plus what was added and skipped.
type Result struct {
	Content []byte     `json:"-"`
	Added   []Redirect `json:"added"`
	Skipped []Skip     `json:"skipped"`
}

// Places lists where redirect files are looked for, in order, for repos that aren't on disk.
func Places() []File {
	fs := make([]File, len(places))
	for i, p := range places {
		fs[i] = File{Path: p.path, Kind: p.kind}
	}
	return fs
}

// Find returns the first redirect file in the repo folder, or ErrNoFile.
func Find(repo string) (File, error) {
	for _, p := range places {
		if st, err := os.Stat(filepath.Join(repo, filepath.FromSlash(p.path))); err == nil && st.Mode().IsRegular() {
			return File{Path: p.path, Kind: p.kind}, nil
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return File{}, err
		}
	}
	return File{}, ErrNoFile
}

// New turns a broken page ("example.com/old") and its target URL into a redirect, using just the path when the target is on the same site.
func New(page, target string) Redirect {
	host, path, ok := strings.Cut(page, "/")
	path = "/" + path
	if !ok {
		path = "/"
	}
	r := Redirect{From: path, To: target}
	u, err := url.Parse(target)
	if err == nil && u.Host != "" && bareHost(u.Hostname()) == bareHost(host) {
		r.To = u.EscapedPath()
		if r.To == "" {
			r.To = "/"
		}
		if u.RawQuery != "" {
			r.To += "?" + u.RawQuery
		}
	}
	return r
}

// Apply adds the redirects to a file's content, skipping ones that are unsafe or already there.
func Apply(kind Kind, content []byte, rs []Redirect) (Result, error) {
	var existing []string
	var err error
	switch kind {
	case Vercel:
		existing, err = vercelSources(content)
	case Netlify:
		existing = netlifySources(content)
	case NetlifyTOML:
		existing = tomlSources(content)
	default:
		return Result{}, errors.New("unknown redirect file kind " + string(kind))
	}
	if err != nil {
		return Result{}, err
	}
	var res Result
	for _, r := range rs {
		if reason := problem(r); reason != "" {
			res.Skipped = append(res.Skipped, Skip{r, reason})
			continue
		}
		key := samePath(r.From)
		if containsPath(existing, key) {
			res.Skipped = append(res.Skipped, Skip{r, "already redirected in " + string(kind)})
			continue
		}
		existing = append(existing, key)
		res.Added = append(res.Added, r)
	}
	if len(res.Added) == 0 {
		res.Content = content
		return res, nil
	}
	switch kind {
	case Vercel:
		res.Content, err = addVercel(content, res.Added)
	case Netlify:
		res.Content = addNetlify(content, res.Added)
	case NetlifyTOML:
		res.Content = addTOML(content, res.Added)
	}
	return res, err
}

// problem says why a redirect can't be written safely, or "" if it can.
func problem(r Redirect) string {
	switch {
	case !strings.HasPrefix(r.From, "/") || r.From == "/":
		return "not a page path"
	case strings.ContainsAny(r.From, ":*()+?{}\\#!"):
		return "path has characters that mean patterns in redirect files"
	case strings.ContainsAny(r.From+r.To, " \t\r\n\"'"):
		return "has spaces or quotes"
	case r.To == "":
		return "no target"
	case samePath(r.From) == samePath(r.To):
		return "redirects to itself"
	}
	return ""
}

func samePath(p string) string {
	if len(p) > 1 {
		p = strings.TrimRight(p, "/")
	}
	return p
}

func containsPath(list []string, p string) bool {
	for _, x := range list {
		if samePath(x) == p {
			return true
		}
	}
	return false
}

func bareHost(h string) string { return strings.TrimPrefix(strings.ToLower(h), "www.") }
