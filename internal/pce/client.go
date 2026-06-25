package pce

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Client is a minimal Illumio PCE REST API v2 client.
type Client struct {
	baseURL    string
	orgID      int
	apiKey     string
	apiSecret  string
	httpClient *http.Client
}

// Config holds the PCE endpoint and credentials.
type Config struct {
	PCEURL    string // host:port, e.g. mypce.example.com:8443
	OrgID     int
	APIKey    string
	APISecret string
}

// Option customizes a Client (used by tests to point at a mock server).
type Option func(*Client)

// WithBaseURL overrides the derived https://<PCEURL> base URL.
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = u } }

// WithHTTPClient overrides the default HTTP client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.httpClient = h } }

// NewClient builds a PCE client. By default the base URL is https://<cfg.PCEURL>.
func NewClient(cfg Config, opts ...Option) *Client {
	c := &Client{
		baseURL:    "https://" + cfg.PCEURL,
		orgID:      cfg.OrgID,
		apiKey:     cfg.APIKey,
		apiSecret:  cfg.APISecret,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// orgPath returns "/orgs/<orgID><suffix>".
func (c *Client) orgPath(suffix string) string {
	return fmt.Sprintf("/orgs/%d%s", c.orgID, suffix)
}

// do performs a request against /api/v2<path>, JSON-encoding body and
// JSON-decoding the response into out (both optional).
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/api/v2"+path, reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.SetBasicAuth(c.apiKey, c.apiSecret)
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("pce request: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusTooManyRequests {
		return &RateLimitError{RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Body: string(data)}
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// Ping performs an authenticated request to verify connectivity and credentials.
func (c *Client) Ping(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, c.orgPath("/labels"), nil, nil)
}

func parseRetryAfter(h string) time.Duration {
	if h == "" {
		return 30 * time.Second
	}
	if secs, err := strconv.Atoi(h); err == nil {
		return time.Duration(secs) * time.Second
	}
	return 30 * time.Second
}
