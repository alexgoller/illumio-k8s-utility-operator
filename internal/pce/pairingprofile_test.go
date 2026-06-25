package pce

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestCreatePairingProfile_PostsEnabledAndOwnership(t *testing.T) {
	var posted PairingProfile
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/orgs/7/pairing_profiles" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&posted)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"href":"/orgs/7/pairing_profiles/5","name":"pp-cven","enabled":true}`))
	})
	pp, err := c.CreatePairingProfile(context.Background(), PairingProfile{
		Name: "pp-cven", Enabled: true, EnforcementMode: testEnforcementVisOnly,
		Labels:          []LabelRef{{Href: "/orgs/7/labels/224"}},
		ExternalDataSet: testExternalDataSet, ExternalDataReference: testExternalDataRef,
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if pp.Href != "/orgs/7/pairing_profiles/5" {
		t.Errorf("href = %q", pp.Href)
	}
	if !posted.Enabled || posted.ExternalDataReference != testExternalDataRef {
		t.Errorf("posted = %+v", posted)
	}
	if len(posted.Labels) != 1 || posted.Labels[0].Href != "/orgs/7/labels/224" {
		t.Errorf("posted labels = %+v, want one label href /orgs/7/labels/224", posted.Labels)
	}
}

func TestGeneratePairingKey_ReturnsActivationCode(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v2/orgs/7/pairing_profiles/5/pairing_key" {
			t.Fatalf("method/path = %s %q", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"activation_code":"act-123"}`))
	})
	code, err := c.GeneratePairingKey(context.Background(), "/orgs/7/pairing_profiles/5")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if code != "act-123" {
		t.Errorf("code = %q, want act-123", code)
	}
}
