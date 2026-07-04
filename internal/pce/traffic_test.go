package pce_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce/pcetest"
)

// testScopeLabelHref is a stand-in provider/destination scope label href reused
// across the PCE HTTP round-trip tests.
const testScopeLabelHref = "/orgs/1/labels/9"

func TestQueryTraffic_AsyncFlowAndParsing(t *testing.T) {
	s := pcetest.New(t)
	// POST create → poll (completed on first read, no sleep) → download.
	s.JSON("POST", "/orgs/1/traffic_flows/async_queries", 201,
		`{"href":"/orgs/1/traffic_flows/async_queries/q1","status":"working"}`)
	s.JSON("GET", "/orgs/1/traffic_flows/async_queries/q1", 200,
		`{"href":"/orgs/1/traffic_flows/async_queries/q1","status":"completed"}`)
	s.JSON("GET", "/orgs/1/traffic_flows/async_queries/q1/download", 200, `[
	  {"src":{"ip":"10.0.0.5","workload":{"labels":[{"key":"app","value":"checkout"}]}},
	   "dst":{"ip":"10.1.0.9","workload":{"labels":[{"key":"app","value":"payments"}]}},
	   "service":{"port":8443,"proto":6},"num_connections":42,
	   "policy_decision":"allowed","draft_policy_decision":"potentially_blocked",
	   "timestamp_range":{"last_detected":"2026-07-03T10:00:00Z"}}
	]`)

	c := s.Client(1)
	flows, truncated, err := c.QueryTraffic(context.Background(), pce.TrafficQuery{
		DestinationLabelHrefs: []string{testScopeLabelHref}, MaxResults: 100,
	})
	if err != nil {
		t.Fatalf("QueryTraffic err: %v", err)
	}
	if truncated {
		t.Error("should not be truncated (1 < MaxResults)")
	}
	if len(flows) != 1 {
		t.Fatalf("got %d flows, want 1", len(flows))
	}
	f := flows[0]
	if f.DraftPolicyDecision != pce.DecisionPotentiallyBlocked || f.PolicyDecision != pce.DecisionAllowed {
		t.Errorf("decisions = draft:%q reported:%q", f.DraftPolicyDecision, f.PolicyDecision)
	}
	if f.Port != 8443 || f.Protocol != 6 || f.Connections != 42 {
		t.Errorf("flow port/proto/conns = %d/%d/%d", f.Port, f.Protocol, f.Connections)
	}
	if f.SrcLabels["app"] != "checkout" || f.DstLabels["app"] != "payments" {
		t.Errorf("labels src=%v dst=%v", f.SrcLabels, f.DstLabels)
	}
	if f.LastDetected.IsZero() {
		t.Error("last_detected did not parse")
	}

	// Request-body contract: the destination scope label href is sent under destinations.include.
	var body struct {
		Destinations struct {
			Include [][]struct {
				Label struct {
					Href string `json:"href"`
				} `json:"label"`
			} `json:"include"`
		} `json:"destinations"`
		MaxResults int `json:"max_results"`
	}
	if err := json.Unmarshal(s.LastBody("POST", "/orgs/1/traffic_flows/async_queries"), &body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if len(body.Destinations.Include) != 1 || len(body.Destinations.Include[0]) != 1 ||
		body.Destinations.Include[0][0].Label.Href != testScopeLabelHref {
		t.Errorf("destinations.include did not carry the scope label href: %+v", body.Destinations.Include)
	}
	if body.MaxResults != 100 {
		t.Errorf("max_results = %d, want 100", body.MaxResults)
	}
}

func TestQueryTraffic_CreateErrorSurfaces(t *testing.T) {
	s := pcetest.New(t)
	s.JSON("POST", "/orgs/1/traffic_flows/async_queries", 406, `{"error":"bad query"}`)
	if _, _, err := s.Client(1).QueryTraffic(context.Background(), pce.TrafficQuery{}); err == nil {
		t.Fatal("expected error on 406 create")
	}
}
