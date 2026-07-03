package controller

import (
	"context"

	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

// RuleViewClient is the subset of the PCE client the RuleView controller needs:
// resolve scope labels to hrefs, and run rule search.
type RuleViewClient interface {
	FindLabel(ctx context.Context, key, value string) (*pce.Label, error)
	SearchRules(ctx context.Context, q pce.RuleSearchQuery) ([]pce.FoundRule, error)
}

var _ RuleViewClient = (*pce.Client)(nil)

// RuleViewClientFactory builds a RuleViewClient (injectable for tests).
type RuleViewClientFactory func(cfg pce.Config) RuleViewClient

// DefaultRuleViewClientFactory wraps the real PCE client.
func DefaultRuleViewClientFactory(cfg pce.Config) RuleViewClient { return pce.NewClient(cfg) }
