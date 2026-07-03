package pce

import (
	"context"
	"fmt"
	"net/http"
)

// Rule types as reported by the PCE (rule_search / rules).
const (
	RuleTypeAllow        = "allow"
	RuleTypeDeny         = "deny"
	RuleTypeOverrideDeny = "override_deny"

	// PolicyVersionActive / PolicyVersionDraft select which policy store to search.
	PolicyVersionActive = "active"
	PolicyVersionDraft  = "draft"
)

// RuleSearchQuery finds rules in which the given labels are the PROVIDER.
type RuleSearchQuery struct {
	ProviderLabelHrefs []string
	PolicyVersion      string // "active" (default) or "draft"
}

// FoundRule is one rule returned by rule search, with its ruleset context.
type FoundRule struct {
	Href                   string
	RulesetHref            string
	RulesetName            string
	RulesetExternalDataSet string // used to flag operator-owned vs external
	Enabled                bool
	Type                   string // allow / deny / override_deny
	Consumers              []Actor
	Services               []IngressService
}

// --- wire types ---

type ruleSearchBody struct {
	Providers []ruleSearchActor `json:"providers"`
}

type ruleSearchActor struct {
	Label *LabelRef `json:"label,omitempty"`
}

type wireRuleSet struct {
	Href            string `json:"href"`
	Name            string `json:"name"`
	ExternalDataSet string `json:"external_data_set"`
}

type wireFoundRule struct {
	Href            string           `json:"href"`
	Enabled         bool             `json:"enabled"`
	RuleType        string           `json:"rule_type"`
	OverrideDeny    bool             `json:"override_deny"`
	Consumers       []Actor          `json:"consumers"`
	IngressServices []IngressService `json:"ingress_services"`
	RuleSet         *wireRuleSet     `json:"rule_set"`
}

// SearchRules returns the rules where q.ProviderLabelHrefs are the provider, via
// the PCE Rule Search API. Read-only.
//
// NOTE: the rule_search request/response shape varies by PCE version; this targets
// the documented Core 24.x form and may need a small live-tuning pass (see
// docs/superpowers/specs/2026-07-03-ruleview-design.md).
func (c *Client) SearchRules(ctx context.Context, q RuleSearchQuery) ([]FoundRule, error) {
	version := q.PolicyVersion
	if version != PolicyVersionDraft {
		version = PolicyVersionActive
	}
	body := ruleSearchBody{Providers: make([]ruleSearchActor, 0, len(q.ProviderLabelHrefs))}
	for _, h := range q.ProviderLabelHrefs {
		body.Providers = append(body.Providers, ruleSearchActor{Label: &LabelRef{Href: h}})
	}

	var wire []wireFoundRule
	path := c.orgPath("/sec_policy/" + version + "/rule_search")
	if err := c.do(ctx, http.MethodPost, path, body, &wire); err != nil {
		return nil, fmt.Errorf("rule search: %w", err)
	}

	out := make([]FoundRule, 0, len(wire))
	for i := range wire {
		w := &wire[i]
		fr := FoundRule{
			Href:      w.Href,
			Enabled:   w.Enabled,
			Type:      ruleType(w),
			Consumers: w.Consumers,
			Services:  w.IngressServices,
		}
		if w.RuleSet != nil {
			fr.RulesetHref = w.RuleSet.Href
			fr.RulesetName = w.RuleSet.Name
			fr.RulesetExternalDataSet = w.RuleSet.ExternalDataSet
		}
		out = append(out, fr)
	}
	return out, nil
}

func ruleType(w *wireFoundRule) string {
	if w.OverrideDeny {
		return RuleTypeOverrideDeny
	}
	if w.RuleType != "" {
		return w.RuleType
	}
	return RuleTypeAllow
}
