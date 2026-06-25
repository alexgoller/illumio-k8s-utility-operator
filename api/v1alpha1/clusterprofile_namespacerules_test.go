package v1alpha1

import "testing"

func TestNamespaceRule_Shape(t *testing.T) {
	cp := ClusterProfile{
		Spec: ClusterProfileSpec{
			SystemNamespaces: SystemNamespacesSpec{
				Manage:          true,
				Labels:          map[string]string{"role": "control"},
				EnforcementMode: "visibility_only",
			},
			NamespaceRules: []NamespaceRule{
				{
					Match:           NamespaceMatch{NamePattern: "payments"},
					Managed:         true,
					AssignLabels:    map[string]LabelAssignment{"env": {Value: "prod"}, "app": {FromNamespaceLabel: "app.kubernetes.io/part-of"}},
					EnforcementMode: "full",
				},
			},
		},
	}
	if cp.Spec.NamespaceRules[0].AssignLabels["env"].Value != "prod" {
		t.Errorf("env value = %q", cp.Spec.NamespaceRules[0].AssignLabels["env"].Value)
	}
	if AnnotationManaged != "microsegment.io/managed" {
		t.Errorf("AnnotationManaged = %q", AnnotationManaged)
	}
}
