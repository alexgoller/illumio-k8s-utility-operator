package controller

import (
	"testing"

	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

const (
	testLabelHref14 = "/orgs/1/labels/14"
	testLabelHref15 = "/orgs/1/labels/15"
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
		nil,
		"",
		[]ResolvedAllow{
			{ConsumerHrefs: []string{testLabelHref15}, Ports: []pce.IngressService{{Proto: 6, Port: 8443}}},
			{ConsumerHrefs: []string{"/orgs/1/labels/16"}, Ports: []pce.IngressService{{Proto: 6, Port: 5432}}},
		},
	)
	if len(rules) != 2 {
		t.Fatalf("rules = %d, want 2", len(rules))
	}
	r := rules[0]
	// Provider is "All Workloads in scope" (ams) — the scope is not repeated as a label.
	if len(r.Providers) != 1 || r.Providers[0].Actors != pce.ActorAllWorkloads || r.Providers[0].Label != nil {
		t.Errorf("provider actor = %+v, want ams", r.Providers)
	}
	if r.Consumers[0].Label.Href != testLabelHref15 {
		t.Errorf("consumer actor = %+v", r.Consumers)
	}
	if r.ResolveLabelsAs.Providers[0] != resolveWorkloads || !r.UnscopedConsumers {
		t.Errorf("rule resolve/unscoped = %+v", r)
	}
	if len(r.IngressServices) != 1 || r.IngressServices[0].Port != 8443 {
		t.Errorf("ingress = %+v", r.IngressServices)
	}
}

func TestBuildRules_IntraScopeAndAllWorkloads(t *testing.T) {
	const allSvc = "/orgs/1/sec_policy/draft/services/all"
	rules := BuildRules(nil, allSvc, []ResolvedAllow{
		// any-any intra-namespace: consumer = ams, intra-scope.
		{AllWorkloads: true, IntraScope: true},
		// role-based intra-scope: label consumer, intra-scope.
		{ConsumerHrefs: []string{testLabelHref15}, IntraScope: true},
		// cross-app extra-scope: label consumer, unscoped.
		{ConsumerHrefs: []string{"/orgs/1/labels/16"}},
	})
	if len(rules) != 3 {
		t.Fatalf("rules = %d, want 3", len(rules))
	}
	// any-any: ams consumer, intra-scope (unscoped_consumers=false).
	if len(rules[0].Consumers) != 1 || rules[0].Consumers[0].Actors != pce.ActorAllWorkloads || rules[0].UnscopedConsumers {
		t.Errorf("any-any rule = %+v", rules[0])
	}
	// no explicit ports → All Services referenced by href, never null/inline.
	if len(rules[0].IngressServices) != 1 || rules[0].IngressServices[0].Href != allSvc || rules[0].IngressServices[0].Proto != 0 {
		t.Errorf("all-ports rule should reference All Services by href: %+v", rules[0].IngressServices)
	}
	// role intra: label consumer, intra-scope.
	if rules[1].Consumers[0].Label == nil || rules[1].UnscopedConsumers {
		t.Errorf("role-intra rule = %+v", rules[1])
	}
	// cross-app: label consumer, extra-scope.
	if rules[2].Consumers[0].Label == nil || !rules[2].UnscopedConsumers {
		t.Errorf("cross-app rule = %+v", rules[2])
	}
}

func TestBuildRules_ProviderNarrowing(t *testing.T) {
	// No narrow hrefs → provider is ams (whole app).
	wide := BuildRules(nil, "", []ResolvedAllow{{ConsumerHrefs: []string{testLabelHref15}, Ports: []pce.IngressService{{Proto: 6, Port: 80}}}})
	if wide[0].Providers[0].Actors != pce.ActorAllWorkloads {
		t.Errorf("no narrowing should be ams provider: %+v", wide[0].Providers)
	}
	// Narrow hrefs → provider is the label(s), not ams.
	narrow := BuildRules([]string{"/orgs/1/labels/backend"}, "", []ResolvedAllow{{ConsumerHrefs: []string{testLabelHref15}, Ports: []pce.IngressService{{Proto: 6, Port: 80}}}})
	if len(narrow[0].Providers) != 1 || narrow[0].Providers[0].Label == nil || narrow[0].Providers[0].Label.Href != "/orgs/1/labels/backend" {
		t.Errorf("narrowed provider should be the label: %+v", narrow[0].Providers)
	}
}

func TestProtoNumber(t *testing.T) {
	if protoNumber("TCP") != 6 || protoNumber("UDP") != 17 || protoNumber("") != 6 {
		t.Errorf("protoNumber wrong")
	}
}
