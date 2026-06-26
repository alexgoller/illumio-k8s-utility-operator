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
			Ports: []microv1.NetworkPolicyPort{{Port: 8443, Protocol: "TCP"}},
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
