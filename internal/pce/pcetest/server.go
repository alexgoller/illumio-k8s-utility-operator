// Package pcetest provides an in-process mock Illumio PCE for testing the
// pce.Client (and, via the real client, the controllers) at the HTTP boundary —
// validating actual request bodies and response parsing, not just injected fakes.
package pcetest

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

// Server is a routing mock PCE. Register responses per endpoint with JSON/Route,
// then build a wired client with Client. All request bodies are recorded for
// assertions via LastBody/Count.
type Server struct {
	t      *testing.T
	srv    *httptest.Server
	mu     sync.Mutex
	routes []route
	reqs   map[string][][]byte
}

type route struct {
	method, path string
	prefix       bool
	h            http.HandlerFunc
}

// New starts a mock PCE server, cleaned up when the test ends.
func New(t *testing.T) *Server {
	t.Helper()
	s := &Server{t: t, reqs: map[string][][]byte{}}
	s.srv = httptest.NewServer(http.HandlerFunc(s.dispatch))
	t.Cleanup(s.srv.Close)
	return s
}

// URL is the base URL of the mock server.
func (s *Server) URL() string { return s.srv.URL }

// Client returns a pce.Client wired to this mock server for the given org.
func (s *Server) Client(orgID int) *pce.Client {
	return pce.NewClient(
		pce.Config{PCEURL: "mock", OrgID: orgID, APIKey: "k", APISecret: "s"},
		pce.WithBaseURL(s.srv.URL),
		pce.WithHTTPClient(s.srv.Client()),
	)
}

// Route registers a handler for method + path (the path AFTER /api/v2). A path
// ending in "*" is a prefix match.
func (s *Server) Route(method, path string, h http.HandlerFunc) {
	prefix := strings.HasSuffix(path, "*")
	path = strings.TrimSuffix(path, "*")
	s.mu.Lock()
	s.routes = append(s.routes, route{method: method, path: path, prefix: prefix, h: h})
	s.mu.Unlock()
}

// JSON registers a handler that returns status + a raw JSON body.
func (s *Server) JSON(method, path string, status int, body string) {
	s.Route(method, path, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})
}

// LastBody returns the most recent recorded request body for method + path
// (path AFTER /api/v2), or nil.
func (s *Server) LastBody(method, path string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.reqs[method+" "+path]
	if len(b) == 0 {
		return nil
	}
	return b[len(b)-1]
}

// Count returns how many times method + path was called.
func (s *Server) Count(method, path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.reqs[method+" "+path])
}

func (s *Server) dispatch(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	p := strings.TrimPrefix(r.URL.Path, "/api/v2")
	s.mu.Lock()
	s.reqs[r.Method+" "+p] = append(s.reqs[r.Method+" "+p], body)
	var h http.HandlerFunc
	for i := range s.routes {
		rt := &s.routes[i]
		if rt.method != r.Method {
			continue
		}
		if rt.prefix {
			if strings.HasPrefix(p, rt.path) {
				h = rt.h // keep scanning; a later exact match wins
			}
		} else if p == rt.path {
			h = rt.h
			break
		}
	}
	s.mu.Unlock()
	if h == nil {
		s.t.Errorf("pcetest: unhandled request %s %s", r.Method, p)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body)) // let the handler re-read if needed
	h(w, r)
}
