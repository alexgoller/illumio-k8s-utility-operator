package controller

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
)

const testPolicyTypeIngress = "Ingress"

func TestCompilePolicy_SupportedSubset(t *testing.T) {
	spec := microv1.SegmentationPolicySpec{
		PolicyTypes: []string{testPolicyTypeIngress},
		Ingress: []microv1.IngressRule{{
			From:  []microv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{testLabelKeyApp: testLabelValueCheckout, testLabelKeyEnv: testLabelValueProd}}}},
			Ports: []microv1.NetworkPolicyPort{{Port: 8443, Protocol: siProtoTCP}},
		}},
	}
	allows, err := CompilePolicy(spec)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(allows) != 1 || allows[0].From[testLabelKeyApp] != testLabelValueCheckout || allows[0].From[testLabelKeyEnv] != testLabelValueProd {
		t.Fatalf("allows = %+v", allows)
	}
	if len(allows[0].Ports) != 1 || allows[0].Ports[0].Proto != 6 || allows[0].Ports[0].Port != 8443 {
		t.Errorf("ports = %+v", allows[0].Ports)
	}
}

func TestCompilePolicy_RejectsUnsupported(t *testing.T) {
	cases := map[string]microv1.SegmentationPolicySpec{
		"egress policyType":     {PolicyTypes: []string{testPolicyTypeIngress, "Egress"}, Ingress: []microv1.IngressRule{{From: []microv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{testLabelKeyApp: "x"}}}}}}},
		"non-empty podSelector": {PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{testLabelKeyRole: "db"}}, Ingress: []microv1.IngressRule{{From: []microv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{testLabelKeyApp: "x"}}}}}}},
		"matchExpressions":      {Ingress: []microv1.IngressRule{{From: []microv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: testLabelKeyApp, Operator: metav1.LabelSelectorOpExists}}}}}}}},
		"empty from":            {Ingress: []microv1.IngressRule{{From: nil}}},
	}
	for name, spec := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := CompilePolicy(spec); err == nil {
				t.Fatalf("expected rejection for %s", name)
			} else if strings.TrimSpace(err.Error()) == "" {
				t.Fatalf("rejection error must be descriptive")
			}
		})
	}
}

func TestCompilePolicy_MultiPeerOr_SameKey(t *testing.T) {
	spec := microv1.SegmentationPolicySpec{
		Ingress: []microv1.IngressRule{{
			From: []microv1.NetworkPolicyPeer{
				{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{testLabelKeyApp: "a"}}},
				{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{testLabelKeyApp: "b"}}},
			},
			Ports: []microv1.NetworkPolicyPort{{Port: 8080, Protocol: siProtoTCP}},
		}},
	}
	allows, err := CompilePolicy(spec)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(allows) != 2 {
		t.Fatalf("expected 2 allows (one per peer), got %d: %+v", len(allows), allows)
	}
	if allows[0].From[testLabelKeyApp] != "a" {
		t.Errorf("allows[0].From[app] = %q, want %q", allows[0].From[testLabelKeyApp], "a")
	}
	if allows[1].From[testLabelKeyApp] != "b" {
		t.Errorf("allows[1].From[app] = %q, want %q", allows[1].From[testLabelKeyApp], "b")
	}
}

func TestCompilePolicy_MultiPeerOr_DistinctKeys(t *testing.T) {
	spec := microv1.SegmentationPolicySpec{
		Ingress: []microv1.IngressRule{{
			From: []microv1.NetworkPolicyPeer{
				{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{testLabelKeyApp: "api"}}},
				{NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"tier": "web"}}},
			},
			Ports: []microv1.NetworkPolicyPort{{Port: 443, Protocol: siProtoTCP}},
		}},
	}
	allows, err := CompilePolicy(spec)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(allows) != 2 {
		t.Fatalf("expected 2 allows (one per peer), got %d: %+v", len(allows), allows)
	}
	if allows[0].From[testLabelKeyApp] != "api" {
		t.Errorf("allows[0].From[app] = %q, want %q", allows[0].From[testLabelKeyApp], "api")
	}
	if allows[1].From["tier"] != "web" {
		t.Errorf("allows[1].From[tier] = %q, want %q", allows[1].From["tier"], "web")
	}
}

func TestStrictestEnforcement(t *testing.T) {
	if StrictestEnforcement("idle", testEnforcementFull, testEnforcementVisOnly) != testEnforcementFull {
		t.Errorf("want full")
	}
	if StrictestEnforcement("", testEnforcementVisOnly, "idle") != testEnforcementVisOnly {
		t.Errorf("want visibility_only")
	}
	if StrictestEnforcement("", "") != "" {
		t.Errorf("want empty")
	}
}
