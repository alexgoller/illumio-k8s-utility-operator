package pce

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestListContainerWorkloadProfiles(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/orgs/7/container_clusters/cid-1/container_workload_profiles" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[
			{"href":"/orgs/7/container_clusters/cid-1/container_workload_profiles/p1","namespace":"payments","managed":true,"enforcement_mode":"visibility_only"},
			{"href":"/orgs/7/container_clusters/cid-1/container_workload_profiles/p0","namespace":null,"managed":false}
		]`))
	})
	got, err := c.ListContainerWorkloadProfiles(context.Background(), "cid-1")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(got) != 2 || got[0].Namespace != "payments" || !got[0].Managed {
		t.Fatalf("got = %+v", got)
	}
}

func TestUpdateContainerWorkloadProfile_PutsFieldsToHref(t *testing.T) {
	var body CWPUpdate
	var gotMethod, gotPath string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusNoContent)
	})
	managed := true
	href := "/orgs/7/container_clusters/cid-1/container_workload_profiles/p1"
	err := c.UpdateContainerWorkloadProfile(context.Background(), href, CWPUpdate{
		Managed:         &managed,
		EnforcementMode: "visibility_only",
		Labels: []CWPLabel{
			{Key: "role", Assignment: &LabelRef{Href: "/orgs/7/labels/5"}},
		},
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %s, want PUT", gotMethod)
	}
	if gotPath != "/api/v2"+href {
		t.Errorf("path = %q, want %q", gotPath, "/api/v2"+href)
	}
	if body.Managed == nil || !*body.Managed || body.EnforcementMode != "visibility_only" {
		t.Errorf("body = %+v", body)
	}
	if len(body.Labels) != 1 || body.Labels[0].Key != "role" || body.Labels[0].Assignment.Href != "/orgs/7/labels/5" {
		t.Errorf("body labels = %+v", body.Labels)
	}
}
