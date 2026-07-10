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
		"egress policyType":            {PolicyTypes: []string{testPolicyTypeIngress, "Egress"}, Ingress: []microv1.IngressRule{{From: []microv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{testLabelKeyApp: "x"}}}}}}},
		"podSelector matchExpressions": {PodSelector: metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: testLabelKeyRole, Operator: metav1.LabelSelectorOpExists}}}, Ingress: []microv1.IngressRule{{From: []microv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{testLabelKeyApp: "x"}}}}}}},
		"matchExpressions":             {Ingress: []microv1.IngressRule{{From: []microv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: testLabelKeyApp, Operator: metav1.LabelSelectorOpExists}}}}}}}},
		"empty from":                   {Ingress: []microv1.IngressRule{{From: nil}}},
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
				{NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{testLabelKeyTier: "web"}}},
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
	if allows[1].From[testLabelKeyTier] != "web" {
		t.Errorf("allows[1].From[tier] = %q, want %q", allows[1].From[testLabelKeyTier], "web")
	}
}

func TestCompilePolicy_IntraVsExtraScope(t *testing.T) {
	spec := microv1.SegmentationPolicySpec{
		Ingress: []microv1.IngressRule{{
			From: []microv1.NetworkPolicyPeer{
				{PodSelector: &metav1.LabelSelector{}},                                                           // empty → ams intra
				{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{testLabelKeyRole: "fe"}}},     // role intra
				{NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{testLabelKeyApp: "x"}}}, // cross-app extra
			},
		}},
	}
	allows, err := CompilePolicy(spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(allows) != 3 {
		t.Fatalf("allows = %d, want 3", len(allows))
	}
	if !allows[0].AllWorkloads || !allows[0].IntraScope {
		t.Errorf("empty podSelector should be ams intra-scope: %+v", allows[0])
	}
	if allows[1].From[testLabelKeyRole] != "fe" || !allows[1].IntraScope {
		t.Errorf("role podSelector should be intra-scope: %+v", allows[1])
	}
	if allows[2].From[testLabelKeyApp] != "x" || allows[2].IntraScope {
		t.Errorf("namespaceSelector should be extra-scope: %+v", allows[2])
	}
}

func TestCompilePolicy_RejectsBothSelectors(t *testing.T) {
	spec := microv1.SegmentationPolicySpec{Ingress: []microv1.IngressRule{{From: []microv1.NetworkPolicyPeer{
		{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{testLabelKeyRole: "fe"}}, NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{testLabelKeyApp: "x"}}},
	}}}}
	if _, err := CompilePolicy(spec); err == nil {
		t.Fatal("expected rejection when both podSelector and namespaceSelector are set")
	}
}

func TestCompileIntent_ShortcutAndIntraExtra(t *testing.T) {
	// any-any shortcut
	allows, err := CompileIntent(microv1.SegmentationIntentSpec{AllowIntraNamespace: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(allows) != 1 || !allows[0].AllWorkloads || !allows[0].IntraScope {
		t.Fatalf("shortcut = %+v", allows)
	}
	// fromIntraNamespace (intra) + from (extra)
	allows, err = CompileIntent(microv1.SegmentationIntentSpec{Allow: []microv1.IntentAllow{
		{FromIntraNamespace: map[string]string{testLabelKeyRole: "fe"}},
		{From: map[string]string{testLabelKeyApp: testLabelValueCheckout}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if allows[0].From[testLabelKeyRole] != "fe" || !allows[0].IntraScope {
		t.Errorf("fromIntraNamespace should be intra-scope: %+v", allows[0])
	}
	if allows[1].From[testLabelKeyApp] != testLabelValueCheckout || allows[1].IntraScope {
		t.Errorf("from should be extra-scope: %+v", allows[1])
	}
	// errors: empty intent, both sources, neither source
	if _, err := CompileIntent(microv1.SegmentationIntentSpec{}); err == nil {
		t.Error("empty intent should be rejected")
	}
	if _, err := CompileIntent(microv1.SegmentationIntentSpec{Allow: []microv1.IntentAllow{{From: map[string]string{testLabelKeyApp: "x"}, FromIntraNamespace: map[string]string{testLabelKeyRole: "y"}}}}); err == nil {
		t.Error("both from and fromIntraNamespace should be rejected")
	}
	if _, err := CompileIntent(microv1.SegmentationIntentSpec{Allow: []microv1.IntentAllow{{}}}); err == nil {
		t.Error("an allow with neither source should be rejected")
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
	// selective ranks between visibility_only and full.
	if StrictestEnforcement(testEnforcementVisOnly, "selective") != "selective" {
		t.Errorf("selective must beat visibility_only")
	}
	if StrictestEnforcement("selective", testEnforcementFull) != testEnforcementFull {
		t.Errorf("full must beat selective")
	}
	if StrictestEnforcement("idle", "selective") != "selective" {
		t.Errorf("selective must beat idle")
	}
}
