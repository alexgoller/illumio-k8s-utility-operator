package pce

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

func TestFindLabel_ReturnsExactMatch(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("key"); got != "app" {
			t.Errorf("query key = %q, want app", got)
		}
		if got := r.URL.Query().Get("value"); got != "checkout" {
			t.Errorf("query value = %q, want checkout", got)
		}
		_, _ = w.Write([]byte(`[{"href":"/orgs/7/labels/42","key":"app","value":"checkout"}]`))
	})
	l, err := c.FindLabel(context.Background(), "app", "checkout")
	if err != nil {
		t.Fatalf("FindLabel error: %v", err)
	}
	if l.Href != "/orgs/7/labels/42" {
		t.Errorf("Href = %q, want /orgs/7/labels/42", l.Href)
	}
}

func TestFindLabel_NoMatchReturnsErrLabelNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	_, err := c.FindLabel(context.Background(), "app", "missing")
	if !errors.Is(err, ErrLabelNotFound) {
		t.Fatalf("error = %v, want ErrLabelNotFound", err)
	}
}

func TestEnsureLabel_CreatesWithOwnershipWhenMissing(t *testing.T) {
	var posted Label
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[]`)) // not found
			return
		}
		// POST create
		_ = json.NewDecoder(r.Body).Decode(&posted)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"href":"/orgs/7/labels/99","key":"role","value":"control"}`))
	})
	owner := Owner{DataSet: "illumio-operator", Reference: "cr-uid-1"}
	l, err := c.EnsureLabel(context.Background(), "role", "control", owner)
	if err != nil {
		t.Fatalf("EnsureLabel error: %v", err)
	}
	if l.Href != "/orgs/7/labels/99" {
		t.Errorf("Href = %q, want /orgs/7/labels/99", l.Href)
	}
	if posted.ExternalDataSet != "illumio-operator" || posted.ExternalDataReference != "cr-uid-1" {
		t.Errorf("ownership = %q/%q, want illumio-operator/cr-uid-1",
			posted.ExternalDataSet, posted.ExternalDataReference)
	}
}

func TestEnsureLabel_ReturnsExistingWithoutCreating(t *testing.T) {
	var posts int
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
		}
		_, _ = w.Write([]byte(`[{"href":"/orgs/7/labels/42","key":"app","value":"checkout"}]`))
	})
	if _, err := c.EnsureLabel(context.Background(), "app", "checkout", Owner{}); err != nil {
		t.Fatalf("EnsureLabel error: %v", err)
	}
	if posts != 0 {
		t.Errorf("POST count = %d, want 0 (label already existed)", posts)
	}
}
