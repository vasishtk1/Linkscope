package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/vasishtk1/Linkscope/internal/api"
	"github.com/vasishtk1/Linkscope/internal/jobs"
)

type Client struct {
	base    string
	http    *http.Client
	timeout time.Duration
}

// NewClient gives every request a time limit, so a frozen coordinator looks the same as a crashed one.
func NewClient(base string, hc *http.Client, timeout time.Duration) *Client {
	return &Client{base: strings.TrimRight(base, "/"), http: hc, timeout: timeout}
}

func (c *Client) Claim(ctx context.Context, worker string) (api.ClaimResponse, error) {
	var resp api.ClaimResponse
	err := c.post(ctx, "/jobs/claim", api.ClaimRequest{Worker: worker}, &resp)
	return resp, err
}

func (c *Client) Heartbeat(ctx context.Context, j api.Job, worker string) error {
	return c.post(ctx, jobPath(j, "heartbeat"), api.LeaseRequest{Worker: worker, Attempt: j.Attempt}, nil)
}

func (c *Client) Complete(ctx context.Context, j api.Job, worker string, r jobs.Result) error {
	return c.post(ctx, jobPath(j, "complete"), api.CompleteRequest{Worker: worker, Attempt: j.Attempt, Result: r}, nil)
}

func (c *Client) Fail(ctx context.Context, j api.Job, worker, msg string) error {
	return c.post(ctx, jobPath(j, "fail"), api.FailRequest{Worker: worker, Attempt: j.Attempt, Error: msg}, nil)
}

func (c *Client) Release(ctx context.Context, j api.Job, worker string) error {
	return c.post(ctx, jobPath(j, "release"), api.LeaseRequest{Worker: worker, Attempt: j.Attempt}, nil)
}

func jobPath(j api.Job, action string) string {
	return fmt.Sprintf("/jobs/%d/%s", j.ID, action)
}

// post returns jobs.ErrLeaseLost when the coordinator answers 409 Conflict.
func (c *Client) post(ctx context.Context, path string, body, out any) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("coordinator %s: no answer within %s", path, c.timeout)
		}
		return err
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusConflict:
		return jobs.ErrLeaseLost
	case resp.StatusCode >= 300:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("coordinator %s: %s: %s", path, resp.Status, strings.TrimSpace(string(msg)))
	case out != nil:
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
