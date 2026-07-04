package controller

import (
	"context"
	"errors"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

// robustnessStubClient is a minimal OnboardingClient that fails the CWP update
// for one href and records the rest, so we can assert one namespace's PCE
// failure does not stop the others.
type robustnessStubClient struct {
	cwps     []pce.ContainerWorkloadProfile
	failHref string
	updated  map[string]bool
}

func (s *robustnessStubClient) FindContainerClusterByName(context.Context, string) (*pce.ContainerCluster, error) {
	return nil, nil
}
func (s *robustnessStubClient) CreateContainerCluster(context.Context, string, string) (*pce.ContainerCluster, error) {
	return nil, nil
}
func (s *robustnessStubClient) FindPairingProfileByName(context.Context, string) (*pce.PairingProfile, error) {
	return nil, nil
}
func (s *robustnessStubClient) CreatePairingProfile(_ context.Context, pp pce.PairingProfile) (*pce.PairingProfile, error) {
	return &pp, nil
}
func (s *robustnessStubClient) GeneratePairingKey(context.Context, string) (string, error) {
	return "", nil
}
func (s *robustnessStubClient) EnsureLabel(_ context.Context, key, value string, _ pce.Owner) (*pce.Label, error) {
	return &pce.Label{Href: "/labels/" + key + "-" + value, Key: key, Value: value}, nil
}
func (s *robustnessStubClient) ListContainerWorkloadProfiles(context.Context, string) ([]pce.ContainerWorkloadProfile, error) {
	return s.cwps, nil
}
func (s *robustnessStubClient) UpdateContainerWorkloadProfile(_ context.Context, href string, _ pce.CWPUpdate) error {
	if href == s.failHref {
		return errors.New("pce api error: status 406")
	}
	s.updated[href] = true
	return nil
}

// TestReconcileNamespaceCWPs_OneFailureDoesNotBlockOthers verifies that a single
// namespace's PCE failure neither aborts the loop nor hides the others: every
// other namespace is still applied, the managed count reflects only the namespaces
// actually managed (the failed one is not counted), and the aggregated error names
// the failing namespace.
func TestReconcileNamespaceCWPs_OneFailureDoesNotBlockOthers(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := microv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	k := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-good"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-bad"}},
	).Build()

	stub := &robustnessStubClient{
		cwps: []pce.ContainerWorkloadProfile{
			{Href: "h-good", Namespace: "ns-good", Managed: false},
			{Href: "h-bad", Namespace: "ns-bad", Managed: false},
		},
		failHref: "h-bad",
		updated:  map[string]bool{},
	}

	r := &ClusterProfileReconciler{Client: k, Scheme: scheme}
	cp := &microv1.ClusterProfile{
		Status: microv1.ClusterProfileStatus{ContainerClusterID: "cid"},
		Spec: microv1.ClusterProfileSpec{
			NamespaceRules: []microv1.NamespaceRule{{
				Match:           microv1.NamespaceMatch{NamePattern: "ns-*"},
				Managed:         true,
				AssignLabels:    map[string]microv1.LabelAssignment{microv1.LabelKeyEnv: {Value: "test"}},
				EnforcementMode: testEnforcementVisOnly,
			}},
		},
	}

	managed, pending, err := r.reconcileNamespaceCWPs(context.Background(), cp, stub, pce.Owner{})

	if managed != 1 {
		t.Errorf("managed = %d, want 1 (ns-good applied; ns-bad failed and is not counted)", managed)
	}
	if pending != 0 {
		t.Errorf("pending = %d, want 0 (both namespaces have CWPs)", pending)
	}
	if err == nil {
		t.Fatal("expected an aggregated error for the failing namespace, got nil")
	}
	if !strings.Contains(err.Error(), "ns-bad") {
		t.Errorf("aggregated error should name ns-bad, got %v", err)
	}
	if !stub.updated["h-good"] {
		t.Error("ns-good must still be applied even though ns-bad failed")
	}
}
