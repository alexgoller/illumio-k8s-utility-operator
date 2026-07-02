package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

// pendingDeleteClient is a minimal PolicyClient whose owned ruleset carries the
// given UpdateType. Only FindRuleSetByOwner/ListRules/CreateRule are exercised by
// reconcileRuleSet in these cases.
type pendingDeleteClient struct {
	updateType string
}

func (c pendingDeleteClient) FindRuleSetByOwner(context.Context, pce.Owner) (*pce.RuleSet, error) {
	return &pce.RuleSet{Href: "/orgs/1/sec_policy/draft/rule_sets/9", UpdateType: c.updateType}, nil
}
func (pendingDeleteClient) FindLabel(context.Context, string, string) (*pce.Label, error) {
	return &pce.Label{}, nil
}
func (pendingDeleteClient) EnsureLabel(context.Context, string, string, pce.Owner) (*pce.Label, error) {
	return &pce.Label{}, nil
}
func (pendingDeleteClient) FindServiceByName(context.Context, string) (*pce.Service, error) {
	return &pce.Service{Href: "/orgs/1/sec_policy/draft/services/all"}, nil
}
func (pendingDeleteClient) CreateRuleSet(_ context.Context, rs pce.RuleSet) (*pce.RuleSet, error) {
	return &rs, nil
}
func (pendingDeleteClient) DeleteRuleSet(context.Context, string) error { return nil }
func (pendingDeleteClient) ListRules(context.Context, string) ([]pce.SecRule, error) {
	return nil, nil
}
func (pendingDeleteClient) CreateRule(_ context.Context, _ string, r pce.SecRule) (*pce.SecRule, error) {
	return &r, nil
}
func (pendingDeleteClient) DeleteRule(context.Context, string) error { return nil }
func (pendingDeleteClient) ProvisionRuleSets(context.Context, []string, string) (*pce.ProvisionResult, error) {
	return &pce.ProvisionResult{}, nil
}

func TestReconcileRuleSet_PendingDeletionDefers(t *testing.T) {
	owner := pce.Owner{DataSet: "illumio-operator", Reference: "uid-pd"}

	// Ruleset pending deletion → defer with the sentinel, do not recreate.
	_, err := reconcileRuleSet(context.Background(), pendingDeleteClient{updateType: pce.RuleSetUpdateTypeDelete},
		"cr", "ns", owner, []string{"/orgs/1/labels/app-x"}, nil, nil)
	if !errors.Is(err, errRuleSetPendingDeletion) {
		t.Fatalf("pending-delete ruleset: err = %v, want errRuleSetPendingDeletion", err)
	}

	// A normal existing ruleset (no pending delete) reconciles without the sentinel.
	href, err := reconcileRuleSet(context.Background(), pendingDeleteClient{updateType: ""},
		"cr", "ns", owner, []string{"/orgs/1/labels/app-x"}, nil, nil)
	if err != nil {
		t.Fatalf("normal ruleset: unexpected err %v", err)
	}
	if href == "" {
		t.Error("normal ruleset: expected an href")
	}
}
