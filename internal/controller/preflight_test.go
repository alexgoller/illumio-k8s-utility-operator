package controller

import (
	"testing"
	"time"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

func TestClassifyFlows_Inbound(t *testing.T) {
	now := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)
	flows := []pce.TrafficFlow{
		// draft-blocked inbound → finding (peer = source)
		{SrcLabels: map[string]string{testLabelKeyApp: testLabelValueCheckout, testLabelKeyEnv: testLabelValueProd},
			Port: 8443, Protocol: 6, DraftPolicyDecision: pce.DecisionBlocked, Connections: 300, LastDetected: now},
		// allowed → ignored
		{SrcLabels: map[string]string{testLabelKeyApp: "x"}, Port: 80, Protocol: 6,
			DraftPolicyDecision: pce.DecisionAllowed, Connections: 5, LastDetected: now},
		// potentially_blocked, no labels → finding by IP
		{SrcIP: "10.0.0.5", Port: 53, Protocol: 17,
			DraftPolicyDecision: pce.DecisionPotentiallyBlocked, Connections: 2, LastDetected: now},
		// duplicate of the first (same peer/port/proto) → merged, connections summed
		{SrcLabels: map[string]string{testLabelKeyApp: testLabelValueCheckout, testLabelKeyEnv: testLabelValueProd},
			Port: 8443, Protocol: 6, DraftPolicyDecision: pce.DecisionBlocked, Connections: 12, LastDetected: now},
	}
	got := classifyFlows(flows, directionInbound)
	if len(got) != 2 {
		t.Fatalf("expected 2 findings, got %d: %+v", len(got), got)
	}
	// Sorted: "app=checkout;env=prod;" < "ip:10.0.0.5"
	if got[0].Peer[testLabelKeyApp] != testLabelValueCheckout || got[0].Port != 8443 || got[0].Protocol != protoNameTCP {
		t.Errorf("finding[0] = %+v", got[0])
	}
	if got[0].Connections != 312 {
		t.Errorf("expected merged connections 312, got %d", got[0].Connections)
	}
	if got[1].PeerIP != "10.0.0.5" || got[1].Protocol != protoNameUDP || got[1].Decision != pce.DecisionPotentiallyBlocked {
		t.Errorf("finding[1] = %+v", got[1])
	}
}

func TestClassifyFlows_EgressUsesDestination(t *testing.T) {
	flows := []pce.TrafficFlow{
		{DstLabels: map[string]string{testLabelKeyApp: testLabelValueLedger}, Port: 5432, Protocol: 6,
			DraftPolicyDecision: pce.DecisionBlocked, Connections: 9},
	}
	got := classifyFlows(flows, directionEgress)
	if len(got) != 1 || got[0].Peer[testLabelKeyApp] != testLabelValueLedger || got[0].Port != 5432 {
		t.Fatalf("egress finding = %+v", got)
	}
}

func TestCapFindings(t *testing.T) {
	mk := func(app string, conns int) microv1.FlowFinding {
		return microv1.FlowFinding{Peer: map[string]string{testLabelKeyApp: app}, Port: 80, Connections: conns}
	}
	findings := []microv1.FlowFinding{mk("a", 5), mk("b", 50), mk("c", 20), mk("d", 1)}

	// Under the cap → unchanged, not truncated.
	got, trunc := capFindings(findings, 10)
	if trunc || len(got) != 4 {
		t.Fatalf("no-cap: trunc=%v len=%d", trunc, len(got))
	}

	// Over the cap → top-N by connections, truncated.
	got, trunc = capFindings(findings, 2)
	if !trunc || len(got) != 2 {
		t.Fatalf("cap: trunc=%v len=%d", trunc, len(got))
	}
	if got[0].Connections != 50 || got[1].Connections != 20 {
		t.Errorf("expected top-2 by connections [50,20], got [%d,%d]", got[0].Connections, got[1].Connections)
	}

	// n<=0 → no cap.
	if _, trunc := capFindings(findings, 0); trunc {
		t.Error("n=0 should not truncate")
	}
}

func TestSummarizeFlows(t *testing.T) {
	flows := []pce.TrafficFlow{
		{DraftPolicyDecision: pce.DecisionAllowed},
		{DraftPolicyDecision: pce.DecisionAllowed},
		{DraftPolicyDecision: pce.DecisionPotentiallyBlocked},
		{DraftPolicyDecision: pce.DecisionBlocked},
		{DraftPolicyDecision: pce.DecisionUnknown},
	}
	c := summarizeFlows(flows)
	if c.Allowed != 2 || c.PotentiallyBlocked != 1 || c.Blocked != 1 || c.Unknown != 1 || c.Total != 5 {
		t.Errorf("summary = %+v, want allowed=2 pot=1 blocked=1 unknown=1 total=5", c)
	}
	if got := summarizeFlows(nil); got.Total != 0 {
		t.Errorf("empty summary total = %d, want 0", got.Total)
	}
}

func TestClassifyFlows_AllAllowed(t *testing.T) {
	flows := []pce.TrafficFlow{
		{SrcLabels: map[string]string{testLabelKeyApp: "x"}, Port: 80, Protocol: 6, DraftPolicyDecision: pce.DecisionAllowed},
	}
	if got := classifyFlows(flows, directionInbound); len(got) != 0 {
		t.Errorf("expected no findings, got %+v", got)
	}
}
