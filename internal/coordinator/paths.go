package coordinator

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// CrawlPathsURL is where Common Crawl lists every WAT file of a crawl.
func CrawlPathsURL(source, crawl string) string {
	return strings.TrimRight(source, "/") + "/crawl-data/" + crawl + "/wat.paths.gz"
}

// LoadPaths reads a WAT path list from a URL or a local file; .gz lists are decompressed.
func LoadPaths(ctx context.Context, hc *http.Client, location string) ([]string, error) {
	var r io.ReadCloser
	if strings.HasPrefix(location, "http://") || strings.HasPrefix(location, "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
		if err != nil {
			return nil, err
		}
		resp, err := hc.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("fetch %s: %s", location, resp.Status)
		}
		r = resp.Body
	} else {
		f, err := os.Open(location)
		if err != nil {
			return nil, err
		}
		r = f
	}
	defer r.Close()
	return readPaths(r, strings.HasSuffix(location, ".gz"))
}

func readPaths(r io.Reader, gzipped bool) ([]string, error) {
	if gzipped {
		gz, err := gzip.NewReader(r)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		r = gz
	}
	var paths []string
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		if p := strings.TrimSpace(sc.Text()); p != "" {
			paths = append(paths, p)
		}
	}
	return paths, sc.Err()
}

// Window returns paths[offset : offset+limit], clamped to the list; limit <= 0 means all.
func Window(paths []string, offset, limit int) []string {
	if offset >= len(paths) {
		return nil
	}
	paths = paths[max(offset, 0):]
	if limit > 0 && limit < len(paths) {
		paths = paths[:limit]
	}
	return paths
}
