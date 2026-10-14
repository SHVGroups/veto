package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is TypeSafe's API host.
const DefaultBaseURL = "https://api.typesafe.ai"

// Endpoint is one provider that implements the System One API.
type Endpoint struct {
	// Name labels the endpoint in logs and evaluation records.
	Name    string
	BaseURL string
	APIKey  string
	// Model is the model id sent in requests. Pin a version such as
	// "jev-1.13.0" rather than an alias so tuned thresholds do not drift.
	Model string
}

// Result is a validated response plus what it took to get it.
type Result struct {
	Response    *Response
	Endpoint    string
	Model       string
	Attempts    int
	Latency     time.Duration
	RequestBody json.RawMessage
	RawResponse json.RawMessage
}

// APIError is a non-success HTTP response.
type APIError struct {
	Endpoint   string
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("jev: %s returned %d: %s", e.Endpoint, e.StatusCode, e.Body)
}

// Retryable reports whether the same request may succeed if sent again.
func (e *APIError) Retryable() bool {
	return e.StatusCode == http.StatusTooManyRequests || e.StatusCode == 529 || e.StatusCode >= 500
}

// ErrNoEndpoints is returned by a client with nothing to call.
var ErrNoEndpoints = errors.New("jev: no endpoints configured")

// Client sends requests to a primary endpoint and falls back to the others in
// order when the primary is unavailable.
type Client struct {
	endpoints   []Endpoint
	http        *http.Client
	maxAttempts int
	baseBackoff time.Duration
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient sets the HTTP client used for requests.
func WithHTTPClient(c *http.Client) Option { return func(cl *Client) { cl.http = c } }

// WithMaxAttempts sets how many times each endpoint is tried before moving on.
func WithMaxAttempts(n int) Option {
	return func(cl *Client) {
		if n > 0 {
			cl.maxAttempts = n
		}
	}
}

// WithBackoff sets the delay before the first retry; later retries double it.
func WithBackoff(d time.Duration) Option { return func(cl *Client) { cl.baseBackoff = d } }

// NewClient returns a client for the given endpoints, the first being primary.
func NewClient(endpoints []Endpoint, opts ...Option) *Client {
	c := &Client{
		endpoints:   endpoints,
		http:        &http.Client{Timeout: 10 * time.Second},
		maxAttempts: 2,
		baseBackoff: 100 * time.Millisecond,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Configured reports whether the client has at least one endpoint.
func (c *Client) Configured() bool { return c != nil && len(c.endpoints) > 0 }

// PrimaryModel returns the model id of the primary endpoint.
func (c *Client) PrimaryModel() string {
	if !c.Configured() {
		return ""
	}
	return c.endpoints[0].Model
}

// Evaluate sends the request, retrying and falling back as needed, and
// returns the first response that passes validation. The context deadline
// bounds the whole operation including retries.
func (c *Client) Evaluate(ctx context.Context, req Request) (*Result, error) {
	if !c.Configured() {
		return nil, ErrNoEndpoints
	}
	start := time.Now()
	attempts := 0
	var errs []error
	for _, ep := range c.endpoints {
		r := req
		if r.Model == "" {
			r.Model = ep.Model
		}
		if err := r.Validate(); err != nil {
			return nil, err
		}
		body, err := json.Marshal(r)
		if err != nil {
			return nil, fmt.Errorf("jev: encode request: %w", err)
		}
		for try := 0; try < c.maxAttempts; try++ {
			attempts++
			resp, raw, wait, err := c.send(ctx, ep, body, r)
			if err == nil {
				return &Result{
					Response:    resp,
					Endpoint:    ep.Name,
					Model:       resp.Model,
					Attempts:    attempts,
					Latency:     time.Since(start),
					RequestBody: body,
					RawResponse: raw,
				}, nil
			}
			errs = append(errs, err)
			if ctx.Err() != nil {
				return nil, errors.Join(errs...)
			}
			var apiErr *APIError
			if errors.As(err, &apiErr) {
				if apiErr.StatusCode == http.StatusUnprocessableEntity || apiErr.StatusCode == http.StatusBadRequest {
					// The request itself is malformed; no endpoint will accept it.
					return nil, errors.Join(errs...)
				}
				if !apiErr.Retryable() {
					break
				}
			}
			if try+1 < c.maxAttempts {
				if wait == 0 {
					wait = c.backoff(try)
				}
				if !sleep(ctx, wait) {
					return nil, errors.Join(errs...)
				}
			}
		}
	}
	return nil, errors.Join(errs...)
}

func (c *Client) backoff(try int) time.Duration {
	d := c.baseBackoff << try
	jitter := time.Duration(rand.Int64N(int64(d)/2 + 1))
	return d + jitter
}

// send performs one HTTP exchange. It returns a suggested wait before the
// next attempt when the server provided one.
func (c *Client) send(ctx context.Context, ep Endpoint, body []byte, req Request) (*Response, json.RawMessage, time.Duration, error) {
	url := strings.TrimRight(ep.BaseURL, "/") + "/v1/systemone"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, nil, 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+ep.APIKey)
	httpReq.Header.Set("User-Agent", "veto/1 (+https://github.com/shvgroups/veto)")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("jev: %s: %w", ep.Name, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, nil, 0, fmt.Errorf("jev: %s: read response: %w", ep.Name, err)
	}
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(raw))
		if len(msg) > 500 {
			msg = msg[:500]
		}
		return nil, nil, retryAfter(resp.Header.Get("Retry-After")), &APIError{Endpoint: ep.Name, StatusCode: resp.StatusCode, Body: msg}
	}
	var out Response
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, nil, 0, fmt.Errorf("jev: %s: decode response: %w", ep.Name, err)
	}
	if err := out.Validate(req); err != nil {
		return nil, nil, 0, fmt.Errorf("%w (endpoint %s)", err, ep.Name)
	}
	return &out, raw, 0, nil
}

func retryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if secs, err := strconv.ParseFloat(v, 64); err == nil && secs >= 0 {
		return time.Duration(secs * float64(time.Second))
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

func sleep(ctx context.Context, d time.Duration) bool {
	if dl, ok := ctx.Deadline(); ok && time.Until(dl) < d {
		return false
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
