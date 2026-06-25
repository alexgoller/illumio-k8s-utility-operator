package pce

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestCreateRuleSet_PostsScopesAndOwnership(t *testing.T) {
	var posted RuleSet
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/orgs/7/sec_policy/draft/rule_sets" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&posted)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"href":"/orgs/7/sec_policy/draft/rule_sets/843","name":"rs","enabled":true}`))
	})
	rs, err := c.CreateRuleSet(context.Background(), RuleSet{
		Name: "rs", Enabled: true,
		Scopes:          [][]RuleSetScope{{{Label: LabelRef{Href: "/orgs/7/labels/14"}}}},
		ExternalDataSet: "illumio-operator", ExternalDataReference: "cr-uid",
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if rs.Href != "/orgs/7/sec_policy/draft/rule_sets/843" {
		t.Errorf("href = %q", rs.Href)
	}
	if len(posted.Scopes) != 1 || len(posted.Scopes[0]) != 1 || posted.Scopes[0][0].Label.Href != "/orgs/7/labels/14" {
		t.Errorf("scopes = %+v", posted.Scopes)
	}
	if posted.ExternalDataReference != "cr-uid" {
		t.Errorf("ownership = %+v", posted)
	}
}

func TestCreateRule_PostsResolveLabelsAndInlinePort(t *testing.T) {
	var posted SecRule
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/orgs/7/sec_policy/draft/rule_sets/843/sec_rules" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&posted)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"href":"/orgs/7/sec_policy/draft/rule_sets/843/sec_rules/9"}`))
	})
	rule, err := c.CreateRule(context.Background(), "/orgs/7/sec_policy/draft/rule_sets/843", SecRule{
		Enabled:         true,
		ResolveLabelsAs: ResolveLabelsAs{Providers: []string{"workloads"}, Consumers: []string{"workloads"}},
		Providers:       []Actor{{Label: &LabelRef{Href: "/orgs/7/labels/14"}}},
		Consumers:       []Actor{{Label: &LabelRef{Href: "/orgs/7/labels/15"}}},
		IngressServices: []IngressService{{Proto: 6, Port: 8443}},
		UnscopedConsumers: true,
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if rule.Href != "/orgs/7/sec_policy/draft/rule_sets/843/sec_rules/9" {
		t.Errorf("href = %q", rule.Href)
	}
	if posted.ResolveLabelsAs.Providers[0] != "workloads" || !posted.UnscopedConsumers {
		t.Errorf("posted = %+v", posted)
	}
	if len(posted.IngressServices) != 1 || posted.IngressServices[0].Proto != 6 || posted.IngressServices[0].Port != 8443 {
		t.Errorf("ingress = %+v", posted.IngressServices)
	}
}

func TestProvisionRuleSets_PostsChangeSubsetAndReadsAffected(t *testing.T) {
	var body map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/orgs/7/sec_policy" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"href":"/orgs/7/sec_policy/80","version":80,"workloads_affected":3}`))
	})
	res, err := c.ProvisionRuleSets(context.Background(),
		[]string{"/orgs/7/sec_policy/draft/rule_sets/843"}, "deploy app policy")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if res.WorkloadsAffected != 3 || res.Version != 80 {
		t.Errorf("res = %+v", res)
	}
	cs, _ := body["change_subset"].(map[string]any)
	rsList, _ := cs["rule_sets"].([]any)
	if len(rsList) != 1 {
		t.Fatalf("change_subset.rule_sets = %+v", cs["rule_sets"])
	}
}
