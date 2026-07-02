package v1alpha1

import (
	"reflect"
	"testing"
)

func TestScopeLabelKeys(t *testing.T) {
	appEnv := []string{LabelKeyApp, LabelKeyEnv}

	// Unset → app+env default.
	if got := (ClusterProfileSpec{}).ScopeLabelKeys(); !reflect.DeepEqual(got, appEnv) {
		t.Errorf("default ScopeLabelKeys = %v, want %v", got, appEnv)
	}
	// Empty slice → app+env default.
	if got := (ClusterProfileSpec{PolicyScopeLabels: []string{}}).ScopeLabelKeys(); !reflect.DeepEqual(got, appEnv) {
		t.Errorf("empty ScopeLabelKeys = %v, want %v", got, appEnv)
	}
	// Explicit set is honored verbatim.
	explicit := []string{LabelKeyApp, LabelKeyEnv, "tier"}
	if got := (ClusterProfileSpec{PolicyScopeLabels: explicit}).ScopeLabelKeys(); !reflect.DeepEqual(got, explicit) {
		t.Errorf("explicit ScopeLabelKeys = %v, want %v", got, explicit)
	}
}
