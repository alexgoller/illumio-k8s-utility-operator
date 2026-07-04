package pce_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce/pcetest"
)

func TestSearchRules_ProviderFilterAndParse(t *testing.T) {
	s := pcetest.New(t)
	s.JSON("POST", "/orgs/1/sec_policy/active/rule_search", 200, `[
	  {"href":"/orgs/1/sec_policy/active/rule_sets/42/sec_rules/9","enabled":true,"rule_type":"allow",
	   "consumers":[{"actors":"ams"}],"ingress_services":[{"port":8443,"proto":6}],
	   "rule_set":{"href":"/rs/42","name":"payments","external_data_set":"illumio-operator"}},
	  {"href":"/orgs/1/sec_policy/active/rule_sets/7/sec_rules/1","enabled":true,"override_deny":true,
	   "consumers":[{"label":{"href":"/orgs/1/labels/3"}}],
	   "rule_set":{"href":"/rs/7","name":"admin","external_data_set":"someone-else"}}
	]`)

	c := s.Client(1)
	rules, err := c.SearchRules(context.Background(), pce.RuleSearchQuery{
		ProviderLabelHrefs: []string{testScopeLabelHref}, PolicyVersion: pce.PolicyVersionActive,
	})
	if err != nil {
		t.Fatalf("SearchRules err: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("got %d rules, want 2", len(rules))
	}
	if rules[0].RulesetExternalDataSet != "illumio-operator" || rules[0].RulesetName != "payments" || rules[0].Type != pce.RuleTypeAllow {
		t.Errorf("rule[0] = %+v", rules[0])
	}
	if rules[1].Type != pce.RuleTypeOverrideDeny {
		t.Errorf("override_deny should map to Type=%q, got %q", pce.RuleTypeOverrideDeny, rules[1].Type)
	}

	// Request-body contract: the provider scope label href is sent under providers.
	var body struct {
		Providers []struct {
			Label struct {
				Href string `json:"href"`
			} `json:"label"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(s.LastBody("POST", "/orgs/1/sec_policy/active/rule_search"), &body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if len(body.Providers) != 1 || body.Providers[0].Label.Href != testScopeLabelHref {
		t.Errorf("providers did not carry the scope label href: %+v", body.Providers)
	}
}

func TestSearchRules_DefaultsToActiveVersion(t *testing.T) {
	s := pcetest.New(t)
	s.JSON("POST", "/orgs/1/sec_policy/active/rule_search", 200, `[]`)
	// Empty PolicyVersion must hit the active store.
	if _, err := s.Client(1).SearchRules(context.Background(), pce.RuleSearchQuery{}); err != nil {
		t.Fatalf("SearchRules err: %v", err)
	}
	if s.Count("POST", "/orgs/1/sec_policy/active/rule_search") != 1 {
		t.Error("empty PolicyVersion should default to the active rule_search endpoint")
	}
}
