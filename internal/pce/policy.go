package pce

import (
	"context"
	"net/http"
)

// RuleSetScope is one scope entry: {"label":{"href":...}}.
type RuleSetScope struct {
	Label LabelRef `json:"label"`
}

// Actor is a rule provider/consumer: {"label":{"href":...}}.
type Actor struct {
	Label *LabelRef `json:"label,omitempty"`
}

// IngressService is an inline port/proto (no Service object needed).
type IngressService struct {
	Proto int `json:"proto"`
	Port  int `json:"port,omitempty"`
}

// ResolveLabelsAs controls how provider/consumer labels resolve. Use
// ["workloads"] for pods/container workloads.
type ResolveLabelsAs struct {
	Providers []string `json:"providers"`
	Consumers []string `json:"consumers"`
}

// RuleSet is an Illumio ruleset (draft).
type RuleSet struct {
	Href                  string           `json:"href,omitempty"`
	Name                  string           `json:"name"`
	Enabled               bool             `json:"enabled"`
	Scopes                [][]RuleSetScope `json:"scopes"`
	ExternalDataSet       string           `json:"external_data_set,omitempty"`
	ExternalDataReference string           `json:"external_data_reference,omitempty"`
}

// SecRule is an Illumio security rule (draft).
type SecRule struct {
	Href              string           `json:"href,omitempty"`
	Enabled           bool             `json:"enabled"`
	ResolveLabelsAs   ResolveLabelsAs  `json:"resolve_labels_as"`
	Providers         []Actor          `json:"providers"`
	Consumers         []Actor          `json:"consumers"`
	IngressServices   []IngressService `json:"ingress_services"`
	UnscopedConsumers bool             `json:"unscoped_consumers"`
}

// ProvisionResult is the response to a provisioning request.
type ProvisionResult struct {
	Href              string `json:"href"`
	Version           int    `json:"version"`
	WorkloadsAffected int    `json:"workloads_affected"`
}

const secPolicyDraft = "/sec_policy/draft"

// FindRuleSetByOwner returns the draft ruleset owned by the given CR (matched
// on external_data_reference), or (nil, nil).
func (c *Client) FindRuleSetByOwner(ctx context.Context, owner Owner) (*RuleSet, error) {
	var sets []RuleSet
	if err := c.do(ctx, http.MethodGet, c.orgPath(secPolicyDraft+"/rule_sets"), nil, &sets); err != nil {
		return nil, err
	}
	for i := range sets {
		if sets[i].ExternalDataReference == owner.Reference && owner.Reference != "" {
			return &sets[i], nil
		}
	}
	return nil, nil
}

// CreateRuleSet creates a draft ruleset.
func (c *Client) CreateRuleSet(ctx context.Context, rs RuleSet) (*RuleSet, error) {
	var created RuleSet
	if err := c.do(ctx, http.MethodPost, c.orgPath(secPolicyDraft+"/rule_sets"), rs, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

// DeleteRuleSet deletes a draft ruleset by href.
func (c *Client) DeleteRuleSet(ctx context.Context, href string) error {
	return c.do(ctx, http.MethodDelete, href, nil, nil)
}

// ListRules lists the rules in a ruleset (ruleSetHref is a draft href).
func (c *Client) ListRules(ctx context.Context, ruleSetHref string) ([]SecRule, error) {
	var rules []SecRule
	if err := c.do(ctx, http.MethodGet, ruleSetHref+"/sec_rules", nil, &rules); err != nil {
		return nil, err
	}
	return rules, nil
}

// CreateRule creates a rule under a ruleset.
func (c *Client) CreateRule(ctx context.Context, ruleSetHref string, rule SecRule) (*SecRule, error) {
	var created SecRule
	if err := c.do(ctx, http.MethodPost, ruleSetHref+"/sec_rules", rule, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

// DeleteRule deletes a rule by href.
func (c *Client) DeleteRule(ctx context.Context, ruleHref string) error {
	return c.do(ctx, http.MethodDelete, ruleHref, nil, nil)
}

// ProvisionRuleSets provisions exactly the given draft rulesets (never all).
func (c *Client) ProvisionRuleSets(ctx context.Context, ruleSetHrefs []string, description string) (*ProvisionResult, error) {
	refs := make([]map[string]string, 0, len(ruleSetHrefs))
	for _, h := range ruleSetHrefs {
		refs = append(refs, map[string]string{"href": h})
	}
	body := map[string]any{
		"update_description": description,
		"change_subset":      map[string]any{"rule_sets": refs},
	}
	var res ProvisionResult
	if err := c.do(ctx, http.MethodPost, c.orgPath("/sec_policy"), body, &res); err != nil {
		return nil, err
	}
	return &res, nil
}
