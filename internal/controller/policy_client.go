package controller

import (
	"context"

	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

// PolicyClient is the subset of the PCE client the SegmentationIntent
// controller needs. The real *pce.Client satisfies it.
type PolicyClient interface {
	FindLabel(ctx context.Context, key, value string) (*pce.Label, error)
	FindRuleSetByOwner(ctx context.Context, owner pce.Owner) (*pce.RuleSet, error)
	CreateRuleSet(ctx context.Context, rs pce.RuleSet) (*pce.RuleSet, error)
	DeleteRuleSet(ctx context.Context, href string) error
	ListRules(ctx context.Context, ruleSetHref string) ([]pce.SecRule, error)
	CreateRule(ctx context.Context, ruleSetHref string, rule pce.SecRule) (*pce.SecRule, error)
	DeleteRule(ctx context.Context, ruleHref string) error
	ProvisionRuleSets(ctx context.Context, hrefs []string, desc string) (*pce.ProvisionResult, error)
}

var _ PolicyClient = (*pce.Client)(nil)

// PolicyClientFactory builds a PolicyClient (injectable for tests).
type PolicyClientFactory func(cfg pce.Config) PolicyClient

// DefaultPolicyClientFactory wraps the real PCE client.
func DefaultPolicyClientFactory(cfg pce.Config) PolicyClient { return pce.NewClient(cfg) }
