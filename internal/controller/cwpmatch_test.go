package controller

import (
	"testing"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
)

const (
	testLabelKeyRole       = "role"
	testLabelKeyEnv        = "env"
	testLabelKeyApp        = "app"
	testLabelValueProd     = "prod"
	testLabelValueCheckout = "checkout"
	testEnforcementVisOnly = "visibility_only"
	testNamespaceTeamA     = "team-a"
	testNamespacePayments  = "payments"
	testNSLabelPartOf      = "app.kubernetes.io/part-of"
)

func TestComputeDesiredCWP(t *testing.T) {
	sys := microv1.SystemNamespacesSpec{
		Manage: true, Labels: map[string]string{testLabelKeyRole: "control"}, EnforcementMode: testEnforcementVisOnly,
	}
	rules := []microv1.NamespaceRule{
		{
			Match:           microv1.NamespaceMatch{NamePattern: testNamespacePayments},
			Managed:         true,
			AssignLabels:    map[string]microv1.LabelAssignment{testLabelKeyEnv: {Value: testLabelValueProd}, testLabelKeyApp: {FromNamespaceLabel: testNSLabelPartOf}},
			EnforcementMode: "full",
		},
		{Match: microv1.NamespaceMatch{NamePattern: "*"}, Managed: false},
	}

	tests := []struct {
		name   string
		nsName string
		labels map[string]string
		annos  map[string]string
		want   DesiredCWP
	}{
		{
			name:   "system namespace gets system defaults",
			nsName: "openshift-monitoring",
			want:   DesiredCWP{Managed: true, Labels: map[string]string{testLabelKeyRole: "control"}, EnforcementMode: testEnforcementVisOnly},
		},
		{
			name:   "user rule wins over system + resolves fromNamespaceLabel",
			nsName: testNamespacePayments,
			labels: map[string]string{testNSLabelPartOf: testLabelValueCheckout},
			want:   DesiredCWP{Managed: true, Labels: map[string]string{testLabelKeyEnv: testLabelValueProd, testLabelKeyApp: testLabelValueCheckout}, EnforcementMode: "full"},
		},
		{
			name:   "non-system, catch-all unmanaged",
			nsName: testNamespaceTeamA,
			want:   DesiredCWP{Managed: false, Labels: map[string]string{}, EnforcementMode: ""},
		},
		{
			name:   "annotation overrides managed + enforcement + label",
			nsName: testNamespaceTeamA,
			annos:  map[string]string{microv1.AnnotationManaged: "true", microv1.AnnotationEnforcement: enforcementIdle, microv1.AnnotationLabelPrefix + testLabelKeyEnv: "dev"},
			want:   DesiredCWP{Managed: true, Labels: map[string]string{testLabelKeyEnv: "dev"}, EnforcementMode: enforcementIdle},
		},
		{
			name:   "managed with no enforcement defaults to idle",
			nsName: testNamespaceTeamA,
			annos:  map[string]string{microv1.AnnotationManaged: "true"},
			want:   DesiredCWP{Managed: true, Labels: map[string]string{}, EnforcementMode: enforcementIdle},
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
