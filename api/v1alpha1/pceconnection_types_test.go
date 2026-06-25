package v1alpha1

import "testing"

func TestPCEConnection_HasExpectedDefaults(t *testing.T) {
	pc := PCEConnection{
		Spec: PCEConnectionSpec{
			PCEURL:               "pce.example.com:8443",
			OrgID:                3,
			CredentialsSecretRef: SecretReference{Name: "illumio-pce-api", Namespace: "illumio-operator"},
		},
	}
	if pc.Spec.CredentialsSecretRef.Name != "illumio-pce-api" {
		t.Errorf("secret name = %q", pc.Spec.CredentialsSecretRef.Name)
	}
	if ConditionConnected != "Connected" {
		t.Errorf("ConditionConnected = %q, want Connected", ConditionConnected)
	}
}
