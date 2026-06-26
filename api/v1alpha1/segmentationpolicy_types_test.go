package v1alpha1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSegmentationPolicy_Shape(t *testing.T) {
	sp := SegmentationPolicy{
		Spec: SegmentationPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []string{"Ingress"},
			Enforcement: "full",
			Ingress: []IngressRule{
				{
					From:  []NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "checkout"}}}},
					Ports: []NetworkPolicyPort{{Port: 8443, Protocol: "TCP"}},
				},
			},
		},
	}
	if sp.Spec.Ingress[0].From[0].PodSelector.MatchLabels["app"] != "checkout" {
		t.Errorf("from app = %q", sp.Spec.Ingress[0].From[0].PodSelector.MatchLabels["app"])
	}
	if sp.Spec.Enforcement != "full" {
		t.Errorf("enforcement = %q", sp.Spec.Enforcement)
	}
}
