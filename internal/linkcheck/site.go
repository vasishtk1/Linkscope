package linkcheck

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type Options struct {
	Timeout  time.Duration
	Interval time.Duration
}

// Checker checks pages politely: one request per Interval across all goroutines, robots.txt read once per host.
type Checker struct {
	client *http.Client
	tick   *time.Ticker
	mu     sync.Mutex
	hosts  map[string]*hostInfo
}

type hostInfo struct {
	once   sync.Once
	scheme string
	robots *Robots
	result Result
	reason string
}

func New(o Options) *Checker {
	return &Checker{
		client: &http.Client{
			Timeout: o.Timeout,
			CheckRedirect: func(_ *http.Request, via []*http.Request) error {
				if len(via) >= maxRedirects {
					return errTooManyRedirects
				}
				return nil
			},
		},
		tick:  time.NewTicker(o.Interval),
		hosts: map[string]*hostInfo{},
	}
}

func (c *Checker) Close() { c.tick.Stop() }

// CheckAll checks pages with at most workers requests in flight, returning results in the same order.
func (c *Checker) CheckAll(ctx context.Context, pages []string, workers int) []Check {
	out := make([]Check, len(pages))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for i := range jobs {
				out[i] = c.Check(ctx, pages[i])
			}
		})
	}
	for i := range pages {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return out
}

func (c *Checker) wait(ctx context.Context) error {
	select {
	case <-c.tick.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// host returns what we know about a host, reading its robots.txt the first time any goroutine asks.
func (c *Checker) host(ctx context.Context, host string) *hostInfo {
	c.mu.Lock()
	h, ok := c.hosts[host]
	if !ok {
		h = &hostInfo{}
		c.hosts[host] = h
	}
	c.mu.Unlock()
	h.once.Do(func() { c.loadHost(ctx, host, h) })
	return h
}

// loadHost finds which scheme the host answers on by fetching robots.txt over https, then http.
func (c *Checker) loadHost(ctx context.Context, host string, h *hostInfo) {
	var lastErr error
	for _, scheme := range []string{"https", "http"} {
		r, err := c.fetch(ctx, http.MethodGet, scheme+"://"+host+"/robots.txt", maxRobots)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				break
			}
			continue
		}
		h.scheme = scheme
		switch {
		case r.status >= 200 && r.status < 300:
			h.robots = ParseRobots(string(r.body), robotsToken)
		case r.status >= 400 && r.status < 500:
			h.robots = allowAll
		default:
			h.result, h.reason = Skipped, fmt.Sprintf("robots.txt unavailable (%d)", r.status)
		}
		return
	}
	h.result, h.reason = Unknown, describe(lastErr)
}
