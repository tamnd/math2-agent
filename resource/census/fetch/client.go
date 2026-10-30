package fetch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// UserAgent names math2 to each API.
const UserAgent = "math2-agent census (https://github.com/tamnd/math2-agent)"

// ErrNotFound is the error of a request that gives the status 404.
var ErrNotFound = errors.New("fetch: not found")

// Client gets JSON from the APIs with a gap between two requests and retries on the statuses 429 and 5xx.
type Client struct {
	HTTP    *http.Client
	Gap     time.Duration // the shortest time between two requests
	Retries int           // the retries of one request
	mu      sync.Mutex
	last    time.Time
}

// NewClient makes a client with the gap.
func NewClient(gap time.Duration) *Client {
	return &Client{HTTP: &http.Client{Timeout: 90 * time.Second}, Gap: gap, Retries: 5}
}

// JSON gets the URL and decodes the body into v.
func (c *Client) JSON(ctx context.Context, url string, v any) error {
	body, err := c.Get(ctx, url)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("%s: %w", url, err)
	}
	return nil
}

// Get gets the URL and gives the body.
func (c *Client) Get(ctx context.Context, url string) ([]byte, error) {
	var last error
	for try := 0; try <= c.Retries; try++ {
		if try > 0 {
			wait := time.Duration(1<<min(try, 6)) * time.Second
			if err := sleep(ctx, wait); err != nil {
				return nil, err
			}
		}
		c.pace(ctx)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", UserAgent)
		req.Header.Set("Accept", "application/json")
		resp, err := c.HTTP.Do(req) // #nosec G107 G704 -- the census builds each URL from a fixed API base.
		if err != nil {
			last = err
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 256<<20))
		_ = resp.Body.Close()
		switch {
		case err != nil:
			last = err
		case resp.StatusCode == http.StatusOK:
			return body, nil
		case resp.StatusCode == http.StatusNotFound:
			return nil, ErrNotFound
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			last = fmt.Errorf("%s: status %d", url, resp.StatusCode)
			if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 && s < 600 {
				if err := sleep(ctx, time.Duration(s)*time.Second); err != nil {
					return nil, err
				}
			}
		default:
			return nil, fmt.Errorf("%s: status %d: %.200s", url, resp.StatusCode, body)
		}
	}
	return nil, last
}

func (c *Client) pace(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if d := c.Gap - time.Since(c.last); d > 0 {
		_ = sleep(ctx, d)
	}
	c.last = time.Now()
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
