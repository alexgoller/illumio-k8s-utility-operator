package controller

import (
	"reflect"
	"testing"
)

func TestScopeLabelSubset(t *testing.T) {
	assigned := map[string]string{
		testLabelKeyApp:   testNamespacePayments,
		testLabelKeyEnv:   testLabelValueProd,
		testLabelKeyTier:  "t1",
		testNodeLabelRole: "svc",
	}
	appEnv := []string{testLabelKeyApp, testLabelKeyEnv}

	// Default app+env keeps only app/env; tier and role are dropped from scope.
	got := scopeLabelSubset(assigned, appEnv)
	want := map[string]string{testLabelKeyApp: testNamespacePayments, testLabelKeyEnv: testLabelValueProd}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("app+env subset = %v, want %v", got, want)
	}

	// A custom scope set is honored.
	got = scopeLabelSubset(assigned, []string{testLabelKeyApp, testLabelKeyEnv, testLabelKeyTier})
	want = map[string]string{testLabelKeyApp: testNamespacePayments, testLabelKeyEnv: testLabelValueProd, testLabelKeyTier: "t1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("app+env+tier subset = %v, want %v", got, want)
	}

	// A scope key the namespace does not carry is simply absent.
	got = scopeLabelSubset(map[string]string{testLabelKeyApp: "x"}, appEnv)
	if !reflect.DeepEqual(got, map[string]string{testLabelKeyApp: "x"}) {
		t.Errorf("missing env → %v, want {app:x}", got)
	}

	// No overlap → empty (caller rejects).
	if got := scopeLabelSubset(map[string]string{testNodeLabelRole: "svc"}, appEnv); len(got) != 0 {
		t.Errorf("no scope keys present → %v, want empty", got)
	}
}
