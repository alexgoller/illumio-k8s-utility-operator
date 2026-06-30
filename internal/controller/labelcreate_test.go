package controller

import "testing"

func TestAutoCreatableKey(t *testing.T) {
	for _, k := range []string{"role", "app", "env", "loc"} {
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
