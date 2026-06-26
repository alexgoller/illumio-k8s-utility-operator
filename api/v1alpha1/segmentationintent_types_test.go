package v1alpha1

import "testing"

func TestSegmentationIntent_Shape(t *testing.T) {
	si := SegmentationIntent{
		Spec: SegmentationIntentSpec{
			Allow: []IntentAllow{
				{From: map[string]string{testLabelKeyApp: testLabelValueCheckout, "env": testLabelValueProd}, Ports: []IntentPort{{Port: 8443, Protocol: "TCP"}}},
			},
		},
	}
	if si.Spec.Allow[0].From[testLabelKeyApp] != testLabelValueCheckout {
		t.Errorf("from app = %q", si.Spec.Allow[0].From[testLabelKeyApp])
	}
	if ConditionReady != "Ready" || AnnotationProvisionApprove != "microsegment.io/provision" {
		t.Errorf("constants wrong")
	}
}
