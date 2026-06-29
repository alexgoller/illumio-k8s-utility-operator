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
	owner := Owner{DataSet: testExternalDataSet, Reference: testExternalDataRef}
	l, err := c.EnsureLabel(context.Background(), "role", "control", owner)
	if err != nil {
		t.Fatalf("EnsureLabel error: %v", err)
	}
	if l.Href != "/orgs/7/labels/99" {
		t.Errorf("Href = %q, want /orgs/7/labels/99", l.Href)
	}
	// The label is stamped with the operator data set, but its
	// external_data_reference is the label's own key=value, NOT the owner's
	// CR-level reference (which is shared across all of a profile's objects).
	if posted.ExternalDataSet != testExternalDataSet {
		t.Errorf("ExternalDataSet = %q, want %s", posted.ExternalDataSet, testExternalDataSet)
	}
	if posted.ExternalDataReference != "role=control" {
		t.Errorf("ExternalDataReference = %q, want role=control", posted.ExternalDataReference)
	}
}

// TestEnsureLabel_DistinctLabelsGetDistinctReferences is a regression test for
// the PCE 406 external_reference_not_unique bug: a ClusterProfile creates
// several labels, all sharing owner.Reference (the CR UID). The PCE requires
// external_data_reference to be unique within the data set, so reusing the CR
// UID makes every creation after the first fail. Each label must instead carry
// a reference unique to itself.
func TestEnsureLabel_DistinctLabelsGetDistinctReferences(t *testing.T) {
	refs := map[string]struct{}{}
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[]`)) // not found -> force create
			return
		}
		var posted Label
		_ = json.NewDecoder(r.Body).Decode(&posted)
		if _, dup := refs[posted.ExternalDataReference]; dup {
			t.Errorf("duplicate external_data_reference %q (PCE would 406)", posted.ExternalDataReference)
		}
		refs[posted.ExternalDataReference] = struct{}{}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"href":"/orgs/7/labels/1","key":"k","value":"v"}`))
	})

	// All four labels share one owner (the same CR UID), exactly as the
	// namespace-CWP path does.
	owner := Owner{DataSet: testExternalDataSet, Reference: testExternalDataRef}
	pairs := [][2]string{{"app", "illumio"}, {"env", "test"}, {"app", "system"}, {"env", "k8s-ag"}}
	for _, p := range pairs {
		if _, err := c.EnsureLabel(context.Background(), p[0], p[1], owner); err != nil {
			t.Fatalf("EnsureLabel(%s=%s) error: %v", p[0], p[1], err)
		}
	}
	if len(refs) != len(pairs) {
		t.Errorf("created %d distinct references, want %d", len(refs), len(pairs))
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
