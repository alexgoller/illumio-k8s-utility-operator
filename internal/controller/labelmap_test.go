package controller

import (
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
)

// LabelMap CRD field names + label keys reused across the LabelMap tests.
const (
	lmFieldFromKey   = "fromKey"
	lmFieldToKey     = "toKey"
	testLabelKeyTier = "tier"
	testPCEConnLM    = "pce-lm"
)

func TestOperatorAssignedKeys(t *testing.T) {
	cp := &microv1.ClusterProfile{Spec: microv1.ClusterProfileSpec{
		NamespaceRules: []microv1.NamespaceRule{
			{AssignLabels: map[string]microv1.LabelAssignment{testLabelKeyApp: {Value: "x"}, testLabelKeyEnv: {Value: "y"}}},
		},
		SystemNamespaces: microv1.SystemNamespacesSpec{Labels: map[string]string{testLabelKeyEnv: "sys", testNodeLabelRole: "infra"}},
	}}
	got := operatorAssignedKeys(cp)
	for _, k := range []string{testLabelKeyApp, testLabelKeyEnv, testNodeLabelRole} {
		if !got[k] {
			t.Errorf("expected operator key %q", k)
		}
	}
	if got["nope"] {
		t.Error("unexpected key")
	}
}

func TestLabelMapWorkloadKeys(t *testing.T) {
	lm := unstructured.Unstructured{Object: map[string]any{
		"workloadLabelMap": []any{
			map[string]any{lmFieldFromKey: "environ", lmFieldToKey: testLabelKeyEnv},
			map[string]any{lmFieldFromKey: "stage", lmFieldToKey: testNodeLabelRole},
			map[string]any{lmFieldFromKey: "broken"}, // no toKey — skipped
		},
	}}
	got := labelMapWorkloadKeys([]unstructured.Unstructured{lm})
	if !got[testLabelKeyEnv] || !got[testNodeLabelRole] {
		t.Errorf("expected env+role toKeys, got %v", got)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 keys, got %v", got)
	}
}

func TestOverlapKeys(t *testing.T) {
	a := map[string]bool{testLabelKeyApp: true, testLabelKeyEnv: true}
	b := map[string]bool{testLabelKeyEnv: true, testNodeLabelRole: true}
	if got := overlapKeys(a, b); !reflect.DeepEqual(got, []string{testLabelKeyEnv}) {
		t.Errorf("overlap = %v, want [env]", got)
	}
	if got := overlapKeys(map[string]bool{testLabelKeyApp: true}, map[string]bool{testNodeLabelRole: true}); len(got) != 0 {
		t.Errorf("expected no overlap, got %v", got)
	}
}
