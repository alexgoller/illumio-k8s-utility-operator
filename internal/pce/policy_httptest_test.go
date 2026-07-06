package pce_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce/pcetest"
)

// TestCreateRule_IngressServicesNonNull guards the historical 406 regressions:
// ingress_services must be a non-null array, and "All Services" must be sent by
// href (never an inline {proto:-1}).
func TestCreateRule_IngressServicesNonNull(t *testing.T) {
	s := pcetest.New(t)
	rsHref := "/orgs/1/sec_policy/draft/rule_sets/42"
	s.JSON("POST", rsHref+"/sec_rules", 201, `{"href":"/orgs/1/sec_policy/draft/rule_sets/42/sec_rules/1"}`)

	rule := pce.SecRule{
		Providers:       []pce.Actor{pce.AllWorkloadsActor()},
		Consumers:       []pce.Actor{pce.AllWorkloadsActor()},
		IngressServices: []pce.IngressService{{Href: "/orgs/1/sec_policy/draft/services/all"}},
	}
	if _, err := s.Client(1).CreateRule(context.Background(), rsHref, rule); err != nil {
		t.Fatalf("CreateRule err: %v", err)
	}

	raw := s.LastBody("POST", rsHref+"/sec_rules")
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	is, ok := body["ingress_services"]
	if !ok || string(is) == "null" {
		t.Fatalf("ingress_services must be a non-null array, got: %s", string(is))
	}
	var svcs []map[string]any
	if err := json.Unmarshal(is, &svcs); err != nil || len(svcs) != 1 {
		t.Fatalf("ingress_services should be a 1-element array: %s (%v)", string(is), err)
	}
	if svcs[0]["href"] != "/orgs/1/sec_policy/draft/services/all" {
		t.Errorf("All Services must be referenced by href, got %+v", svcs[0])
	}
	if _, hasProto := svcs[0]["proto"]; hasProto {
		t.Errorf("All Services must NOT be sent as an inline proto entry: %+v", svcs[0])
	}
}

func TestProvisionRuleSets_SendsChangeSubset(t *testing.T) {
	s := pcetest.New(t)
	s.JSON("POST", "/orgs/1/sec_policy", 201, `{"href":"/sp/1","version":80,"workloads_affected":3}`)

	res, err := s.Client(1).ProvisionRuleSets(context.Background(), []string{"/rs/42"}, "delete ns/uid")
	if err != nil {
		t.Fatalf("ProvisionRuleSets err: %v", err)
	}
	if res.WorkloadsAffected != 3 || res.Version != 80 {
		t.Errorf("result = %+v, want workloads=3 version=80", res)
	}

	var body struct {
		ChangeSubset struct {
			RuleSets []struct {
				Href string `json:"href"`
			} `json:"rule_sets"`
		} `json:"change_subset"`
	}
	if err := json.Unmarshal(s.LastBody("POST", "/orgs/1/sec_policy"), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.ChangeSubset.RuleSets) != 1 || body.ChangeSubset.RuleSets[0].Href != "/rs/42" {
		t.Errorf("change_subset.rule_sets = %+v, want the provided href", body.ChangeSubset.RuleSets)
	}
}

func TestFindRuleSetByOwner_MatchesExternalDataReference(t *testing.T) {
	s := pcetest.New(t)
	s.JSON("GET", "/orgs/1/sec_policy/draft/rule_sets", 200, `[
	  {"href":"/rs/1","name":"other","external_data_reference":"someone-else"},
	  {"href":"/rs/2","name":"mine","external_data_reference":"cp-uid","update_type":"create"}
	]`)
	rs, err := s.Client(1).FindRuleSetByOwner(context.Background(), pce.Owner{DataSet: "illumio-operator", Reference: "cp-uid"})
	if err != nil {
		t.Fatalf("FindRuleSetByOwner err: %v", err)
	}
	if rs == nil || rs.Href != "/rs/2" {
		t.Fatalf("expected to match /rs/2 by external_data_reference, got %+v", rs)
	}
	if rs.UpdateType != pce.RuleSetUpdateTypeDelete && rs.UpdateType != "create" {
		t.Errorf("update_type not parsed: %q", rs.UpdateType)
	}
}

func TestFindServiceByName_Parses(t *testing.T) {
	s := pcetest.New(t)
	s.JSON("GET", "/orgs/1/sec_policy/draft/services", 200,
		`[{"href":"/svc/1","name":"Other"},{"href":"/svc/all","name":"All Services"}]`)
	svc, err := s.Client(1).FindServiceByName(context.Background(), pce.AllServicesName)
	if err != nil {
		t.Fatalf("FindServiceByName err: %v", err)
	}
	if svc == nil || svc.Href != "/svc/all" {
		t.Fatalf("expected All Services href, got %+v", svc)
	}
}
