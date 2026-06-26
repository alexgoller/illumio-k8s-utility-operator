package v1alpha1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSegmentationPolicy_Shape(t *testing.T) {
	sp := SegmentationPolicy{
		Spec: SegmentationPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []string{testPolicyTypeIngress},
			Enforcement: testEnforcementFull,
			Ingress: []IngressRule{
				{
					From:  []NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{testLabelKeyApp: testLabelValueCheckout}}}},
					Ports: []NetworkPolicyPort{{Port: 8443, Protocol: "TCP"}},
				},
			},
		},
	}
	if sp.Spec.Ingress[0].From[0].PodSelector.MatchLabels[testLabelKeyApp] != testLabelValueCheckout {
		t.Errorf("from app = %q", sp.Spec.Ingress[0].From[0].PodSelector.MatchLabels[testLabelKeyApp])
	}
	if sp.Spec.Enforcement != testEnforcementFull {
		t.Errorf("enforcement = %q", sp.Spec.Enforcement)
	}
}
