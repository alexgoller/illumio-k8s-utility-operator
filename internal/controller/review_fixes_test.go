package controller

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

// --- H1: a managed namespace with no CWP is pending, not counted as managed ---

func TestReconcileNamespaceCWPs_PendingWhenNoCWP(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = microv1.AddToScheme(scheme)
	k := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-nocwp"}},
	).Build()

	// Kubelink hasn't created this namespace's CWP yet: ListContainerWorkloadProfiles is empty.
	stub := &robustnessStubClient{cwps: nil, updated: map[string]bool{}}
	r := &ClusterProfileReconciler{Client: k, Scheme: scheme}
	cp := &microv1.ClusterProfile{
		Status: microv1.ClusterProfileStatus{ContainerClusterID: "cid"},
		Spec: microv1.ClusterProfileSpec{NamespaceRules: []microv1.NamespaceRule{{
			Match: microv1.NamespaceMatch{NamePattern: "ns-*"}, Managed: true,
			AssignLabels: map[string]microv1.LabelAssignment{microv1.LabelKeyEnv: {Value: testLabelValueProd}},
		}}},
	}
	managed, pending, err := r.reconcileNamespaceCWPs(context.Background(), cp, stub, pce.Owner{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if managed != 0 || pending != 1 {
		t.Errorf("managed=%d pending=%d, want 0/1 (CWP not created yet)", managed, pending)
	}
}

// --- H2: FinalizePolicy is idempotent on an already pending-deletion ruleset ---

type finalizeFakeClient struct {
	updateType    string
	deleteErr     error
	deleteCalled  bool
	provisionErr  error
	provisionCall bool
}

func (c *finalizeFakeClient) FindRuleSetByOwner(context.Context, pce.Owner) (*pce.RuleSet, error) {
	return &pce.RuleSet{Href: "/rs/1", UpdateType: c.updateType}, nil
}
func (c *finalizeFakeClient) DeleteRuleSet(context.Context, string) error {
	c.deleteCalled = true
	return c.deleteErr
}
func (c *finalizeFakeClient) ProvisionRuleSets(context.Context, []string, string) (*pce.ProvisionResult, error) {
	c.provisionCall = true
	return &pce.ProvisionResult{}, c.provisionErr
}
func (*finalizeFakeClient) FindLabel(context.Context, string, string) (*pce.Label, error) {
	return &pce.Label{}, nil
}
func (*finalizeFakeClient) EnsureLabel(context.Context, string, string, pce.Owner) (*pce.Label, error) {
	return &pce.Label{}, nil
}
func (*finalizeFakeClient) FindServiceByName(context.Context, string) (*pce.Service, error) {
	return &pce.Service{}, nil
}
func (*finalizeFakeClient) CreateRuleSet(_ context.Context, rs pce.RuleSet) (*pce.RuleSet, error) {
	return &rs, nil
}
func (*finalizeFakeClient) ListRules(context.Context, string) ([]pce.SecRule, error) { return nil, nil }
func (*finalizeFakeClient) CreateRule(_ context.Context, _ string, r pce.SecRule) (*pce.SecRule, error) {
	return &r, nil
}
func (*finalizeFakeClient) DeleteRule(context.Context, string) error { return nil }

func finalizeTestClient(t *testing.T) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = microv1.AddToScheme(scheme)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "fin-ns"}}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "fin-creds", Namespace: operatorNamespaceForTest},
		Data: map[string][]byte{"api_key": []byte("k"), "api_secret": []byte("s")}}
	conn := &microv1.PCEConnection{ObjectMeta: metav1.ObjectMeta{Name: "fin-pce"},
		Spec: microv1.PCEConnectionSpec{PCEURL: "pce:8443", OrgID: 1,
			CredentialsSecretRef: microv1.SecretReference{Name: "fin-creds", Namespace: operatorNamespaceForTest}}}
	meta.SetStatusCondition(&conn.Status.Conditions, metav1.Condition{Type: microv1.ConditionConnected, Status: metav1.ConditionTrue, Reason: microv1.ReasonConnected, Message: "t"})
	cp := &microv1.ClusterProfile{ObjectMeta: metav1.ObjectMeta{Name: "fin-cp"},
		Spec: microv1.ClusterProfileSpec{PCEConnectionRef: microv1.LocalObjectReference{Name: "fin-pce"},
			NamespaceRules: []microv1.NamespaceRule{{Match: microv1.NamespaceMatch{NamePattern: "fin-*"}, Managed: true}}}}
	meta.SetStatusCondition(&cp.Status.Conditions, metav1.Condition{Type: microv1.ConditionOnboarded, Status: metav1.ConditionTrue, Reason: microv1.ReasonOnboarded, Message: "t"})
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, secret, conn, cp).Build()
}

func TestFinalizePolicy_PendingDeleteSkipsDeleteButProvisions(t *testing.T) {
	k := finalizeTestClient(t)
	fc := &finalizeFakeClient{updateType: pce.RuleSetUpdateTypeDelete}
	err := FinalizePolicy(context.Background(), k, func(pce.Config) PolicyClient { return fc }, "fin-ns", "uid-1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if fc.deleteCalled {
		t.Error("DeleteRuleSet must NOT be called on an already pending-delete ruleset")
	}
	if !fc.provisionCall {
		t.Error("ProvisionRuleSets should still be called to complete the deletion")
	}
}

func TestFinalizePolicy_NotFoundIsSuccess(t *testing.T) {
	k := finalizeTestClient(t)
	// Normal ruleset, but the provision returns 404 (already gone) → treated as success.
	fc := &finalizeFakeClient{updateType: "", provisionErr: &pce.APIError{StatusCode: 404}}
	if err := FinalizePolicy(context.Background(), k, func(pce.Config) PolicyClient { return fc }, "fin-ns", "uid-2"); err != nil {
		t.Fatalf("404 on provision should be idempotent success, got %v", err)
	}
	if !fc.deleteCalled {
		t.Error("DeleteRuleSet should be called for a non-pending ruleset")
	}
}

// --- H3: rate-limit classification ---

func TestPCEFailReasonAndRequeue(t *testing.T) {
	rl := &pce.RateLimitError{RetryAfter: 12 * time.Second}
	if d, ok := pceRateLimit(rl); !ok || d != 12*time.Second {
		t.Errorf("pceRateLimit(rl) = %v,%v, want 12s,true", d, ok)
	}
	if r := pceFailReason(rl); r != microv1.ReasonRateLimited {
		t.Errorf("pceFailReason(rl) = %q, want RateLimited", r)
	}
	other := &pce.APIError{StatusCode: 500}
	if _, ok := pceRateLimit(other); ok {
		t.Error("non-rate-limit must not classify as rate limit")
	}
	if r := pceFailReason(other); r != microv1.ReasonQueryFailed {
		t.Errorf("pceFailReason(other) = %q, want QueryFailed", r)
	}
}
