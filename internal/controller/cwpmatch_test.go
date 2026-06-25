package controller

import (
	"testing"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
)

func TestComputeDesiredCWP(t *testing.T) {
	sys := microv1.SystemNamespacesSpec{
		Manage: true, Labels: map[string]string{"role": "control"}, EnforcementMode: "visibility_only",
	}
	rules := []microv1.NamespaceRule{
		{
			Match:        microv1.NamespaceMatch{NamePattern: "payments"},
			Managed:      true,
			AssignLabels: map[string]microv1.LabelAssignment{"env": {Value: "prod"}, "app": {FromNamespaceLabel: "app.kubernetes.io/part-of"}},
			EnforcementMode: "full",
		},
		{Match: microv1.NamespaceMatch{NamePattern: "*"}, Managed: false},
	}

	tests := []struct {
		name    string
		nsName  string
		labels  map[string]string
		annos   map[string]string
		want    DesiredCWP
	}{
		{
			name:   "system namespace gets system defaults",
			nsName: "openshift-monitoring",
			want:   DesiredCWP{Managed: true, Labels: map[string]string{"role": "control"}, EnforcementMode: "visibility_only"},
		},
		{
			name:   "user rule wins over system + resolves fromNamespaceLabel",
			nsName: "payments",
			labels: map[string]string{"app.kubernetes.io/part-of": "checkout"},
			want:   DesiredCWP{Managed: true, Labels: map[string]string{"env": "prod", "app": "checkout"}, EnforcementMode: "full"},
		},
		{
			name:   "non-system, catch-all unmanaged",
			nsName: "team-a",
			want:   DesiredCWP{Managed: false, Labels: map[string]string{}, EnforcementMode: ""},
		},
		{
			name:   "annotation overrides managed + enforcement + label",
			nsName: "team-a",
			annos:  map[string]string{microv1.AnnotationManaged: "true", microv1.AnnotationEnforcement: "idle", microv1.AnnotationLabelPrefix + "env": "dev"},
			want:   DesiredCWP{Managed: true, Labels: map[string]string{"env": "dev"}, EnforcementMode: "idle"},
		},
		{
			name:   "managed with no enforcement defaults to idle",
			nsName: "team-a",
			annos:  map[string]string{microv1.AnnotationManaged: "true"},
			want:   DesiredCWP{Managed: true, Labels: map[string]string{}, EnforcementMode: "idle"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeDesiredCWP(tc.nsName, tc.labels, tc.annos, rules, sys)
			if got.Managed != tc.want.Managed || got.EnforcementMode != tc.want.EnforcementMode {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
			if len(got.Labels) != len(tc.want.Labels) {
				t.Fatalf("labels got %+v, want %+v", got.Labels, tc.want.Labels)
			}
			for k, v := range tc.want.Labels {
				if got.Labels[k] != v {
					t.Fatalf("label %s got %q want %q", k, got.Labels[k], v)
				}
			}
		})
	}
}
