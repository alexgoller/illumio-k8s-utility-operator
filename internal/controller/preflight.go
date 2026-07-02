package controller

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

// preflight flow directions.
const (
	directionInbound  = "inbound"  // peer is the source (consumer) reaching this app
	directionOutbound = "outbound" // peer is the destination (provider) this app reaches

	protoNameTCP = "TCP"
	protoNameUDP = "UDP"
)

// scopeLabelValues returns the namespace's scope-label key→value map (the subset
// of its assigned CWP labels that form the ruleset scope; empty if unmanaged or
// no scope labels apply).
func scopeLabelValues(ns corev1.Namespace, cp *microv1.ClusterProfile) map[string]string {
	desired := ComputeDesiredCWP(ns.Name, ns.Labels, ns.Annotations, cp.Spec.NamespaceRules, cp.Spec.SystemNamespaces)
	if !desired.Managed {
		return nil
	}
	return scopeLabelSubset(desired.Labels, cp.Spec.ScopeLabelKeys())
}

// protoName maps an IP protocol number to a name for display.
func protoName(proto int) string {
	switch proto {
	case 6:
		return protoNameTCP
	case 17:
		return protoNameUDP
	default:
		return fmt.Sprintf("%d", proto)
	}
}

// draftBlocked reports whether a flow's DRAFT policy decision would block it.
func draftBlocked(decision string) bool {
	return decision == pce.DecisionBlocked || decision == pce.DecisionPotentiallyBlocked
}

// summarizeFlows counts observed flows by their DRAFT policy decision.
func summarizeFlows(flows []pce.TrafficFlow) microv1.DecisionCounts {
	var c microv1.DecisionCounts
	for i := range flows {
		switch flows[i].DraftPolicyDecision {
		case pce.DecisionAllowed:
			c.Allowed++
		case pce.DecisionPotentiallyBlocked:
			c.PotentiallyBlocked++
		case pce.DecisionBlocked:
			c.Blocked++
		default:
			c.Unknown++
		}
		c.Total++
	}
	return c
}

// classifyFlows converts observed flows into findings for the given direction,
// keeping only those the DRAFT policy would block. For inbound the peer is the
// flow source (consumer); for outbound the peer is the flow destination (provider).
// Findings are de-duplicated by (peer, port, proto) and sorted for stable output.
func classifyFlows(flows []pce.TrafficFlow, direction string) []microv1.FlowFinding {
	type key struct {
		peer  string
		port  int
		proto int
	}
	seen := map[key]*microv1.FlowFinding{}
	order := []key{}
	for i := range flows {
		f := &flows[i]
		if !draftBlocked(f.DraftPolicyDecision) {
			continue
		}
		var peer map[string]string
		var peerIP string
		if direction == directionInbound {
			peer, peerIP = f.SrcLabels, f.SrcIP
		} else {
			peer, peerIP = f.DstLabels, f.DstIP
		}
		k := key{peer: peerKey(peer, peerIP), port: f.Port, proto: f.Protocol}
		if existing, ok := seen[k]; ok {
			existing.Connections += f.Connections
			continue
		}
		ld := metav1.NewTime(f.LastDetected)
		finding := &microv1.FlowFinding{
			Peer:         peer,
			PeerIP:       peerIP,
			Port:         f.Port,
			Protocol:     protoName(f.Protocol),
			Connections:  f.Connections,
			Decision:     f.DraftPolicyDecision,
			LastDetected: &ld,
		}
		seen[k] = finding
		order = append(order, k)
	}
	out := make([]microv1.FlowFinding, 0, len(order))
	for _, k := range order {
		out = append(out, *seen[k])
	}
	slices.SortFunc(out, func(a, b microv1.FlowFinding) int {
		if c := cmp.Compare(peerKey(a.Peer, a.PeerIP), peerKey(b.Peer, b.PeerIP)); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Port, b.Port); c != 0 {
			return c
		}
		return cmp.Compare(a.Protocol, b.Protocol)
	})
	return out
}

// capFindings bounds a findings list to n entries (for etcd object-size safety),
// keeping the highest-connection flows. Returns the (possibly shortened) list and
// whether it was truncated. n<=0 means no cap.
func capFindings(f []microv1.FlowFinding, n int) ([]microv1.FlowFinding, bool) {
	if n <= 0 || len(f) <= n {
		return f, false
	}
	out := make([]microv1.FlowFinding, len(f))
	copy(out, f)
	slices.SortFunc(out, func(a, b microv1.FlowFinding) int {
		if c := cmp.Compare(b.Connections, a.Connections); c != 0 { // desc by connections
			return c
		}
		if c := cmp.Compare(peerKey(a.Peer, a.PeerIP), peerKey(b.Peer, b.PeerIP)); c != 0 {
			return c
		}
		return cmp.Compare(a.Port, b.Port)
	})
	return out[:n], true
}

// peerKey is a stable string identity for a peer (labels or IP).
func peerKey(labels map[string]string, ip string) string {
	if len(labels) == 0 {
		return "ip:" + ip
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(labels[k])
		b.WriteString(";")
	}
	return b.String()
}
