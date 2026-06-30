package pce

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

const testOwnerRefB = "uid-B"

func TestFindRuleSetByOwner(t *testing.T) {
	// Fixture: three rulesets — uid-A, uid-B, and one with an empty reference.
	fixture := []RuleSet{
		{Href: "/orgs/7/sec_policy/draft/rule_sets/1", Name: "rs-a", Enabled: true, Scopes: [][]RuleSetScope{}, ExternalDataSet: "ds", ExternalDataReference: "uid-A"},
		{Href: "/orgs/7/sec_policy/draft/rule_sets/2", Name: "rs-b", Enabled: true, Scopes: [][]RuleSetScope{}, ExternalDataSet: "ds", ExternalDataReference: testOwnerRefB},
		{Href: "/orgs/7/sec_policy/draft/rule_sets/3", Name: "rs-empty", Enabled: true, Scopes: [][]RuleSetScope{}, ExternalDataSet: "ds", ExternalDataReference: ""},
	}
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/orgs/7/sec_policy/draft/rule_sets" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(fixture)
	})

	// (a) Lookup uid-B — should return the second ruleset.
	got, err := c.FindRuleSetByOwner(context.Background(), Owner{Reference: testOwnerRefB})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a ruleset, got nil")
	}
	if got.ExternalDataReference != testOwnerRefB {
		t.Errorf("ExternalDataReference = %q, want %s", got.ExternalDataReference, testOwnerRefB)
	}
	if got.Href != "/orgs/7/sec_policy/draft/rule_sets/2" {
		t.Errorf("Href = %q, want /orgs/7/sec_policy/draft/rule_sets/2", got.Href)
	}

	// (b) Lookup uid-Z (no match) — should return (nil, nil).
	got, err = c.FindRuleSetByOwner(context.Background(), Owner{Reference: "uid-Z"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for unknown reference, got %+v", got)
	}

	// (c) Security invariant: empty reference must NOT match the ruleset with an
	// empty external_data_reference. An empty owner reference is not a valid
	// ownership claim and must never match anything.
	got, err = c.FindRuleSetByOwner(context.Background(), Owner{Reference: ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("empty owner reference must not match any ruleset, got %+v", got)
	}
}

const (
	testLabelHref14 = "/orgs/7/labels/14"
	testRuleResolve = "workloads"
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
		Scopes:          [][]RuleSetScope{{{Label: LabelRef{Href: testLabelHref14}}}},
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
		Enabled:           true,
		ResolveLabelsAs:   ResolveLabelsAs{Providers: []string{testRuleResolve}, Consumers: []string{testRuleResolve}},
		Providers:         []Actor{{Label: &LabelRef{Href: testLabelHref14}}},
		Consumers:         []Actor{{Label: &LabelRef{Href: "/orgs/7/labels/15"}}},
		IngressServices:   []IngressService{{Proto: 6, Port: 8443}},
		UnscopedConsumers: true,
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if rule.Href != "/orgs/7/sec_policy/draft/rule_sets/843/sec_rules/9" {
		t.Errorf("href = %q", rule.Href)
	}
	if posted.ResolveLabelsAs.Providers[0] != testRuleResolve || !posted.UnscopedConsumers {
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

func TestAllWorkloadsActor_MarshalsToAms(t *testing.T) {
	b, err := json.Marshal(AllWorkloadsActor())
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"actors":"ams"}` {
		t.Fatalf("ams actor = %s, want {\"actors\":\"ams\"}", b)
	}
	// A label actor must still marshal as a label, not actors.
	lb, _ := json.Marshal(Actor{Label: &LabelRef{Href: "/h/1"}})
	if string(lb) != `{"label":{"href":"/h/1"}}` {
		t.Fatalf("label actor = %s", lb)
	}
}
