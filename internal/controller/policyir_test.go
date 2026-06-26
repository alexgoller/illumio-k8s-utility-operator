package controller

import (
	"testing"

	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

const (
	testLabelHref14 = "/orgs/1/labels/14"
)

func TestBuildRuleSet_ScopesToProviderAndStampsOwner(t *testing.T) {
	rs := BuildRuleSet("payments", "ingress", []string{testLabelHref14}, pce.Owner{DataSet: defaultExternalDataSet, Reference: "uid-1"})
	if rs.Name != RuleSetName("payments", "ingress") {
		t.Errorf("name = %q", rs.Name)
	}
	if len(rs.Scopes) != 1 || len(rs.Scopes[0]) != 1 || rs.Scopes[0][0].Label.Href != testLabelHref14 {
		t.Fatalf("scopes = %+v", rs.Scopes)
	}
	if rs.ExternalDataReference != "uid-1" || !rs.Enabled {
		t.Errorf("rs = %+v", rs)
	}
}

func TestBuildRules_OneRulePerAllow(t *testing.T) {
	rules := BuildRules(
		[]string{testLabelHref14},
		[]ResolvedAllow{
			{ConsumerHrefs: []string{"/orgs/1/labels/15"}, Ports: []pce.IngressService{{Proto: 6, Port: 8443}}},
			{ConsumerHrefs: []string{"/orgs/1/labels/16"}, Ports: []pce.IngressService{{Proto: 6, Port: 5432}}},
		},
	)
	if len(rules) != 2 {
		t.Fatalf("rules = %d, want 2", len(rules))
	}
	r := rules[0]
	if r.Providers[0].Label.Href != testLabelHref14 || r.Consumers[0].Label.Href != "/orgs/1/labels/15" {
		t.Errorf("rule actors = %+v", r)
	}
	if r.ResolveLabelsAs.Providers[0] != resolveWorkloads || !r.UnscopedConsumers {
		t.Errorf("rule resolve/unscoped = %+v", r)
	}
	if len(r.IngressServices) != 1 || r.IngressServices[0].Port != 8443 {
		t.Errorf("ingress = %+v", r.IngressServices)
	}
}

func TestProtoNumber(t *testing.T) {
	if protoNumber("TCP") != 6 || protoNumber("UDP") != 17 || protoNumber("") != 6 {
		t.Errorf("protoNumber wrong")
	}
}
