package controller

import (
	"testing"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
)

func TestAutoCreatableKey(t *testing.T) {
	for _, k := range []string{microv1.LabelKeyRole, microv1.LabelKeyApp, microv1.LabelKeyEnv, microv1.LabelKeyLoc} {
		if !autoCreatableKey(k) {
			t.Errorf("%q should be auto-creatable", k)
		}
	}
	for _, k := range []string{"custom", "ap", "team", ""} {
		if autoCreatableKey(k) {
			t.Errorf("%q must NOT be auto-creatable", k)
		}
	}
}
