package v1alpha1

import "testing"

func TestSegmentationIntent_Shape(t *testing.T) {
	si := SegmentationIntent{
		Spec: SegmentationIntentSpec{
			Allow: []IntentAllow{
				{From: map[string]string{"app": "checkout", "env": "prod"}, Ports: []IntentPort{{Port: 8443, Protocol: "TCP"}}},
			},
		},
	}
	if si.Spec.Allow[0].From["app"] != "checkout" {
		t.Errorf("from app = %q", si.Spec.Allow[0].From["app"])
	}
	if ConditionReady != "Ready" || AnnotationProvisionApprove != "microsegment.io/provision" {
		t.Errorf("constants wrong")
	}
}
