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
	testEnforcementFull    = "full"
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
			EnforcementMode: testEnforcementFull,
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
			want:   DesiredCWP{Managed: true, Labels: map[string]string{testLabelKeyEnv: testLabelValueProd, testLabelKeyApp: testLabelValueCheckout}, EnforcementMode: testEnforcementFull},
		},
		{
			name:   "non-system, catch-all unmanaged",
			nsName: testNamespaceTeamA,
			want:   DesiredCWP{Managed: false, Labels: map[string]string{}, EnforcementMode: ""},
		},
		{
			// Explicit idle on a managed namespace is invalid in the PCE, so it is
			// floored to visibility_only (the annotation still overrides managed + label).
			name:   "annotation managed + explicit idle floors to visibility_only",
			nsName: testNamespaceTeamA,
			annos:  map[string]string{microv1.AnnotationManaged: "true", microv1.AnnotationEnforcement: enforcementIdle, microv1.AnnotationLabelPrefix + testLabelKeyEnv: "dev"},
			want:   DesiredCWP{Managed: true, Labels: map[string]string{testLabelKeyEnv: "dev"}, EnforcementMode: testEnforcementVisOnly},
		},
		{
			name:   "managed with no enforcement defaults to visibility_only",
			nsName: testNamespaceTeamA,
			annos:  map[string]string{microv1.AnnotationManaged: "true"},
			want:   DesiredCWP{Managed: true, Labels: map[string]string{}, EnforcementMode: testEnforcementVisOnly},
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

// TestComputeDesiredCWP_ManagedIdleFloorsToVisibilityOnly is a regression test
// for the PCE 406 container_workload_profile_invalid_managed_idle: a managed CWP
// must never be idle. systemNamespaces (and user rules) that request idle while
// managed must be floored to visibility_only. A managed CWP is never emitted with
// idle enforcement.
func TestComputeDesiredCWP_ManagedIdleFloorsToVisibilityOnly(t *testing.T) {
	cases := []struct {
		name  string
		rules []microv1.NamespaceRule
		sys   microv1.SystemNamespacesSpec
		ns    string
	}{
		{
			name: "system namespace managed + idle",
			sys:  microv1.SystemNamespacesSpec{Manage: true, EnforcementMode: enforcementIdle},
			ns:   "kube-system",
		},
		{
			name:  "user rule managed + idle",
			rules: []microv1.NamespaceRule{{Match: microv1.NamespaceMatch{NamePattern: "illumio-*"}, Managed: true, EnforcementMode: enforcementIdle}},
			ns:    "illumio-system",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeDesiredCWP(tc.ns, nil, nil, tc.rules, tc.sys)
			if !got.Managed {
				t.Fatalf("expected managed, got %+v", got)
			}
			if got.EnforcementMode != testEnforcementVisOnly {
				t.Fatalf("managed CWP must never be idle: got enforcement %q, want %s", got.EnforcementMode, testEnforcementVisOnly)
			}
		})
	}
}
