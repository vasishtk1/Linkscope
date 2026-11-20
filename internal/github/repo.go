package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

var (
	ErrNotFound = errors.New("not found")
	ErrExists   = errors.New("already exists")
)

// APIError is an error answer from GitHub; it never includes the token.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("GitHub error %d", e.Status)
	}
	return fmt.Sprintf("GitHub error %d: %s", e.Status, e.Message)
}

// Repo is one repository, reached with an installation token.
type Repo struct {
	Owner, Name string
	c           client
}

// PR is the part of a pull request we use.
type PR struct {
	Number int    `json:"number"`
	URL    string `json:"html_url"`
	Title  string `json:"title"`
	Head   struct {
		Ref string `json:"ref"`
	} `json:"head"`
}

func (r *Repo) path(p string) string { return "/repos/" + r.Owner + "/" + r.Name + p }

// DefaultBranch is the branch pull requests go into, usually main.
func (r *Repo) DefaultBranch(ctx context.Context) (string, error) {
	var info struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := r.c.do(ctx, "GET", r.path(""), nil, &info); err != nil {
		return "", err
	}
	return info.DefaultBranch, nil
}

// File reads a file on a branch and returns its content and blob SHA (needed to update it); ErrNotFound if it isn't there.
func (r *Repo) File(ctx context.Context, path, ref string) ([]byte, string, error) {
	var f struct {
		Type     string `json:"type"`
		Encoding string `json:"encoding"`
		Content  string `json:"content"`
		SHA      string `json:"sha"`
	}
	err := r.c.do(ctx, "GET", r.path("/contents/"+escapePath(path)+"?ref="+url.QueryEscape(ref)), nil, &f)
	if err != nil {
		var ae *APIError
		if errors.As(err, &ae) && ae.Status == http.StatusNotFound {
			return nil, "", ErrNotFound
		}
		return nil, "", err
	}
	if f.Type != "file" {
		return nil, "", ErrNotFound
	}
	if f.Encoding != "base64" {
		return nil, "", fmt.Errorf("%s: unexpected encoding %q (file too large?)", path, f.Encoding)
	}
	data, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(f.Content, "\n", ""))
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", path, err)
	}
	return data, f.SHA, nil
}

// BranchSHA is the commit a branch points to.
func (r *Repo) BranchSHA(ctx context.Context, branch string) (string, error) {
	var ref struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := r.c.do(ctx, "GET", r.path("/git/ref/heads/"+escapePath(branch)), nil, &ref); err != nil {
		return "", err
	}
	return ref.Object.SHA, nil
}

// CreateBranch makes a branch at a commit; ErrExists if the name is taken.
func (r *Repo) CreateBranch(ctx context.Context, name, sha string) error {
	err := r.c.do(ctx, "POST", r.path("/git/refs"), map[string]string{"ref": "refs/heads/" + name, "sha": sha}, nil)
	var ae *APIError
	if errors.As(err, &ae) && ae.Status == http.StatusUnprocessableEntity && strings.Contains(ae.Message, "already exists") {
		return ErrExists
	}
	return err
}

// PutFile saves a file on a branch as one commit; sha is the old blob's SHA, or "" for a new file.
func (r *Repo) PutFile(ctx context.Context, path, branch, message string, content []byte, sha string) error {
	body := map[string]string{
		"message": message,
		"content": base64.StdEncoding.EncodeToString(content),
		"branch":  branch,
	}
	if sha != "" {
		body["sha"] = sha
	}
	return r.c.do(ctx, "PUT", r.path("/contents/"+escapePath(path)), body, nil)
}

// OpenPRs lists open pull requests (the first 100).
func (r *Repo) OpenPRs(ctx context.Context) ([]PR, error) {
	var prs []PR
	err := r.c.do(ctx, "GET", r.path("/pulls?state=open&per_page=100"), nil, &prs)
	return prs, err
}

// CreatePR opens a pull request from head into base.
func (r *Repo) CreatePR(ctx context.Context, title, body, head, base string) (PR, error) {
	var pr PR
	err := r.c.do(ctx, "POST", r.path("/pulls"), map[string]string{"title": title, "body": body, "head": head, "base": base}, &pr)
	return pr, err
}

func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}

type client struct {
	base  string
	http  *http.Client
	token string
}

// do sends one API request with the token and decodes the JSON answer into out, if given.
func (c client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Authorization", "Bearer "+c.token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, strings.SplitN(path, "?", 2)[0], err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		var e struct {
			Message string `json:"message"`
			Errors  []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		json.Unmarshal(data, &e)
		msg := e.Message
		for _, x := range e.Errors {
			if x.Message != "" {
				msg += ": " + x.Message
			}
		}
		return &APIError{Status: resp.StatusCode, Message: msg}
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}
