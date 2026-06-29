package pce

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestCreateContainerCluster_ReturnsTokenAndOmitsExternalData(t *testing.T) {
	// The PCE container_clusters POST schema rejects external_data_set /
	// external_data_reference with a 406 input_validation_error. Assert on the
	// raw JSON body (not a typed struct) that those keys are never sent.
	var raw map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v2/orgs/7/container_clusters" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&raw)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"href":"/orgs/7/container_clusters/1b85-uuid","name":"ocp-prod","container_cluster_token":"7_abc123"}`))
	})
	cl, err := c.CreateContainerCluster(context.Background(), "ocp-prod", "managed by operator")
	if err != nil {
		t.Fatalf("CreateContainerCluster error: %v", err)
	}
	if cl.ContainerClusterToken != "7_abc123" {
		t.Errorf("token = %q, want 7_abc123", cl.ContainerClusterToken)
	}
	if raw["name"] != "ocp-prod" {
		t.Errorf("posted name = %v", raw["name"])
	}
	if _, ok := raw["external_data_set"]; ok {
		t.Errorf("posted body must not contain external_data_set (PCE rejects it): %v", raw)
	}
	if _, ok := raw["external_data_reference"]; ok {
		t.Errorf("posted body must not contain external_data_reference (PCE rejects it): %v", raw)
	}
	if ContainerClusterUUID(cl.Href) != "1b85-uuid" {
		t.Errorf("uuid = %q, want 1b85-uuid", ContainerClusterUUID(cl.Href))
	}
}

func TestFindContainerClusterByName(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"href":"/orgs/7/container_clusters/a","name":"other"},{"href":"/orgs/7/container_clusters/b","name":"ocp-prod"}]`))
	})
	got, err := c.FindContainerClusterByName(context.Background(), "ocp-prod")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if got == nil || got.Href != "/orgs/7/container_clusters/b" {
		t.Fatalf("got = %+v, want cluster b", got)
	}
}

func TestFindContainerClusterByName_NoneReturnsNilNil(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	got, err := c.FindContainerClusterByName(context.Background(), "missing")
	if err != nil || got != nil {
		t.Fatalf("got=%+v err=%v, want nil,nil", got, err)
	}
}
