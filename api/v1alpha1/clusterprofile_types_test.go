package v1alpha1

import "testing"

func TestClusterProfile_Shape(t *testing.T) {
	cp := ClusterProfile{
		Spec: ClusterProfileSpec{
			PCEConnectionRef: LocalObjectReference{Name: "prod-pce"},
			Onboarding: OnboardingSpec{
				ContainerClusterName:    "ocp-prod",
				CredentialsOutputSecret: "illumio-cluster-creds",
			},
			ProvisioningMode: "manual",
		},
	}
	if cp.Spec.Onboarding.ContainerClusterName != "ocp-prod" {
		t.Errorf("clusterName = %q", cp.Spec.Onboarding.ContainerClusterName)
	}
	if ConditionOnboarded != "Onboarded" {
		t.Errorf("ConditionOnboarded = %q", ConditionOnboarded)
	}
}
