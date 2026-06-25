package pce

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestCreateContainerCluster_ReturnsTokenAndStampsOwnership(t *testing.T) {
	var posted ContainerCluster
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v2/orgs/7/container_clusters" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&posted)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"href":"/orgs/7/container_clusters/1b85-uuid","name":"ocp-prod","container_cluster_token":"7_abc123"}`))
	})
	cl, err := c.CreateContainerCluster(context.Background(), "ocp-prod", "managed by operator", Owner{DataSet: testExternalDataSet, Reference: testExternalDataRef})
	if err != nil {
		t.Fatalf("CreateContainerCluster error: %v", err)
	}
	if cl.ContainerClusterToken != "7_abc123" {
		t.Errorf("token = %q, want 7_abc123", cl.ContainerClusterToken)
	}
	if posted.Name != "ocp-prod" {
		t.Errorf("posted name = %q", posted.Name)
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
