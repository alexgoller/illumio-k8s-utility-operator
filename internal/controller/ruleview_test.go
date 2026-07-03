package controller

import (
	"testing"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

const (
	testEDS         = "illumio-operator"
	testRulesetName = "payments-ingress"
)

func TestMapRules_OwnedVsExternal(t *testing.T) {
	found := []pce.FoundRule{
		// operator-owned: ruleset external_data_set == eds
		{Href: "/r/1", RulesetName: testRulesetName, RulesetExternalDataSet: testEDS, Enabled: true,
			Type: pce.RuleTypeAllow, Consumers: []pce.Actor{{Label: &pce.LabelRef{Href: "/orgs/1/labels/9"}}},
			Services: []pce.IngressService{{Port: 8443, Proto: 6}}},
		// external: authored outside k8s
		{Href: "/r/2", RulesetName: "admin-baseline", RulesetExternalDataSet: "someone-else", Enabled: true,
			Type: pce.RuleTypeAllow, Consumers: []pce.Actor{pce.AllWorkloadsActor()},
			Services: []pce.IngressService{{Href: "/svc/all"}}},
	}
	rules, owned, external, trunc := mapRules(found, testEDS, 200)
	if owned != 1 || external != 1 || trunc {
		t.Fatalf("owned=%d external=%d trunc=%v, want 1/1/false", owned, external, trunc)
	}
	// owned sorts first.
	if rules[0].OwnedBy != microv1.RuleOwnedByOperator || rules[0].RulesetName != testRulesetName {
		t.Errorf("rules[0] = %+v, want operator/payments-ingress", rules[0])
	}
	if rules[0].Services[0] != "8443/TCP" {
		t.Errorf("service render = %q, want 8443/TCP", rules[0].Services[0])
	}
	if rules[1].OwnedBy != microv1.RuleOwnedByExternal {
		t.Errorf("rules[1].OwnedBy = %q, want external", rules[1].OwnedBy)
	}
	if rules[1].Consumers[0] != pce.ActorAllWorkloads || rules[1].Services[0] != "All Services" {
		t.Errorf("external rule render = %+v", rules[1])
	}
}

func TestMapRules_CapTruncates(t *testing.T) {
	found := make([]pce.FoundRule, 5)
	for i := range found {
		found[i] = pce.FoundRule{Href: "/r", RulesetExternalDataSet: "x"}
	}
	rules, _, external, trunc := mapRules(found, testEDS, 2)
	if !trunc || len(rules) != 2 {
		t.Fatalf("trunc=%v len=%d, want true/2", trunc, len(rules))
	}
	if external != 5 {
		t.Errorf("external count = %d, want 5 (exact, over all found)", external)
	}
}

func TestMapRules_EmptyEDSMeansExternal(t *testing.T) {
	// When the operator has no data set, nothing is flagged as owned.
	found := []pce.FoundRule{{Href: "/r/1", RulesetExternalDataSet: ""}}
	_, owned, external, _ := mapRules(found, "", 200)
	if owned != 0 || external != 1 {
		t.Errorf("owned=%d external=%d, want 0/1", owned, external)
	}
}
