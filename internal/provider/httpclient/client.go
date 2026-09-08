package httpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

type Error struct {
	Status    int
	Operation string
}

func (e *Error) Error() string {
	return fmt.Sprintf("provider %s failed (HTTP %d)", e.Operation, e.Status)
}

type Client struct{ HTTP *http.Client }

func New() *Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = 16
	t.MaxIdleConnsPerHost = 4
	t.ResponseHeaderTimeout = 5 * time.Second
	return &Client{HTTP: &http.Client{Transport: t, Timeout: 8 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (c *Client) Get(ctx context.Context, url string, out any) error {
	return c.Read(ctx, func() (*http.Request, error) { return http.NewRequestWithContext(ctx, http.MethodGet, url, nil) }, out)
}

// Read retries a side-effect-free operation only. The factory regenerates
// authentication and body for each attempt; never pass a trading operation.
func (c *Client) Read(ctx context.Context, factory func() (*http.Request, error), out any) error {
	for attempt := 0; attempt < 3; attempt++ {
		req, err := factory()
		if err != nil {
			return fmt.Errorf("create request: invalid URL")
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("provider transport unavailable")
		}
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			if attempt == 2 {
				return &Error{Status: resp.StatusCode, Operation: req.Method}
			}
			delay := time.Duration(1<<attempt) * 200 * time.Millisecond
			if v := resp.Header.Get("Retry-After"); v != "" {
				if n, e := strconv.Atoi(v); e == nil && n > 0 {
					delay = time.Duration(n) * time.Second
				} else if when, e := http.ParseTime(v); e == nil && time.Until(when) > 0 {
					delay = time.Until(when)
				}
			}
			if delay > 10*time.Second {
				return &Error{Status: resp.StatusCode, Operation: "rate limited"}
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			continue
		}
		if resp.StatusCode != 200 {
			_ = resp.Body.Close()
			return &Error{Status: resp.StatusCode, Operation: req.Method}
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
		_ = resp.Body.Close()
		if err != nil {
			return fmt.Errorf("read provider response: %w", err)
		}
		if len(body) > 2*1024*1024 {
			return fmt.Errorf("provider response too large")
		}
		if err = json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("invalid provider JSON")
		}
		return nil
	}
	return fmt.Errorf("retry exhausted")
}
