package linkcheck

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

type Result string

const (
	OK       Result = "ok"
	Broken   Result = "broken"
	Homepage Result = "homepage"
	Unknown  Result = "unknown"
	Skipped  Result = "skipped"
)

const (
	UserAgent    = "LinkscopeBot/0.1 (+https://github.com/vasishtk1/Linkscope)"
	robotsToken  = "linkscopebot"
	maxRedirects = 5
	bodyPeek     = 1 << 10
	maxRobots    = 500 << 10
)

var errTooManyRedirects = errors.New("too many redirects")

// Check is the outcome for one page.
type Check struct {
	Page     string `json:"page"`
	URL      string `json:"url,omitempty"`
	Result   Result `json:"result"`
	Status   int    `json:"status,omitempty"`
	FinalURL string `json:"finalUrl,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type response struct {
	status int
	final  string
	body   []byte
}

// Check tests one page from the index (host + path, no scheme); it never returns an error, only a result.
func (c *Checker) Check(ctx context.Context, page string) Check {
	host, path := splitPage(page)
	out := Check{Page: page}
	h := c.host(ctx, host)
	if h.reason != "" {
		out.Result, out.Reason = h.result, h.reason
		return out
	}
	if !h.robots.Allowed(path) {
		out.Result, out.Reason = Skipped, "robots.txt"
		return out
	}
	out.URL = h.scheme + "://" + page
	r, err := c.fetch(ctx, http.MethodHead, out.URL, 0)
	if err == nil && r.status >= 400 {
		r, err = c.fetch(ctx, http.MethodGet, out.URL, bodyPeek)
	}
	if err != nil {
		out.Result, out.Reason = Unknown, describe(err)
		return out
	}
	out.Status = r.status
	redirected := r.final != out.URL
	if redirected {
		out.FinalURL = r.final
	}
	out.Result, out.Reason = classify(r.status, path, redirected && isHomepage(r.final))
	return out
}

// fetch sends one request after waiting its turn, and reads at most limit bytes of the body.
func (c *Checker) fetch(ctx context.Context, method, rawURL string, limit int64) (response, error) {
	if err := c.wait(ctx); err != nil {
		return response{}, err
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return response{}, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := c.client.Do(req)
	if err != nil {
		return response{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil && limit == maxRobots {
		return response{}, err
	}
	return response{status: resp.StatusCode, final: resp.Request.URL.String(), body: body}, nil
}

func classify(status int, path string, landedOnHomepage bool) (Result, string) {
	switch {
	case status == http.StatusNotFound || status == http.StatusGone:
		return Broken, ""
	case status >= 200 && status < 300:
		if landedOnHomepage && path != "/" {
			return Homepage, ""
		}
		return OK, ""
	case status >= 500:
		return Unknown, fmt.Sprintf("server error %d", status)
	case status == 401 || status == 403 || status == 429 || status == 999:
		return Unknown, fmt.Sprintf("blocked %d", status)
	default:
		return Unknown, fmt.Sprintf("status %d", status)
	}
}

func describe(err error) string {
	var dns *net.DNSError
	var ne net.Error
	switch {
	case errors.Is(err, errTooManyRedirects):
		return "too many redirects"
	case errors.Is(err, context.Canceled):
		return "stopped"
	case errors.As(err, &dns):
		return "domain not found"
	case errors.As(err, &ne) && ne.Timeout():
		return "timeout"
	default:
		return "connection failed"
	}
}

func isHomepage(rawURL string) bool {
	u, err := url.Parse(rawURL)
	return err == nil && (u.Path == "" || u.Path == "/")
}

func splitPage(page string) (host, path string) {
	if i := strings.IndexByte(page, '/'); i >= 0 {
		return page[:i], page[i:]
	}
	return page, "/"
}
