package pce

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	testExternalDataSet = "illumio-operator"
	testExternalDataRef = "cp-uid"
)

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return NewClient(
		Config{PCEURL: "ignored", OrgID: 7, APIKey: "api_key123", APISecret: "secret123"},
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
	)
}

func TestPing_SendsBasicAuthToLabelsEndpoint(t *testing.T) {
	var gotPath, gotUser, gotPass string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotUser, gotPass, _ = r.BasicAuth()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
	})

	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping returned error: %v", err)
	}
	if gotPath != "/api/v2/orgs/7/labels" {
		t.Errorf("path = %q, want /api/v2/orgs/7/labels", gotPath)
	}
	if gotUser != "api_key123" || gotPass != "secret123" {
		t.Errorf("basic auth = %q:%q, want api_key123:secret123", gotUser, gotPass)
	}
}

func TestPing_RateLimitedReturnsRateLimitError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	err := c.Ping(context.Background())
	rl, ok := err.(*RateLimitError)
	if !ok {
		t.Fatalf("error = %T (%v), want *RateLimitError", err, err)
	}
	if rl.RetryAfter.Seconds() != 30 {
		t.Errorf("RetryAfter = %v, want 30s", rl.RetryAfter)
	}
}

func TestPing_ServerErrorReturnsAPIError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad key"}`))
	})
	err := c.Ping(context.Background())
	ae, ok := err.(*APIError)
	if !ok {
		t.Fatalf("error = %T (%v), want *APIError", err, err)
	}
	if ae.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d, want 401", ae.StatusCode)
	}
}
