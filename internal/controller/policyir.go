package controller

import (
	"fmt"

	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

// ResolvedAllow is an IntentAllow after consumer labels are resolved to hrefs.
type ResolvedAllow struct {
	ConsumerHrefs []string
	Ports         []pce.IngressService
}

// RuleSetName is the deterministic name of the ruleset for a CR.
func RuleSetName(namespace, crName string) string {
	return fmt.Sprintf("k8s-%s-%s", namespace, crName)
}

// BuildRuleSet builds the desired ruleset, scoped to the provider labels and
// stamped with ownership.
func BuildRuleSet(namespace, crName string, providerHrefs []string, owner pce.Owner) pce.RuleSet {
	scope := make([]pce.RuleSetScope, 0, len(providerHrefs))
	for _, h := range providerHrefs {
		scope = append(scope, pce.RuleSetScope{Label: pce.LabelRef{Href: h}})
	}
	return pce.RuleSet{
		Name:                  RuleSetName(namespace, crName),
		Enabled:               true,
		Scopes:                [][]pce.RuleSetScope{scope},
		ExternalDataSet:       owner.DataSet,
		ExternalDataReference: owner.Reference,
	}
}

// BuildRules builds one rule per allow entry: providers = the namespace's app
// labels; consumers = the allow's resolved labels; inline ports; pod resolution.
func BuildRules(providerHrefs []string, allows []ResolvedAllow) []pce.SecRule {
	providers := make([]pce.Actor, 0, len(providerHrefs))
	for _, h := range providerHrefs {
		href := h
		providers = append(providers, pce.Actor{Label: &pce.LabelRef{Href: href}})
	}
	rules := make([]pce.SecRule, 0, len(allows))
	for _, a := range allows {
		consumers := make([]pce.Actor, 0, len(a.ConsumerHrefs))
		for _, h := range a.ConsumerHrefs {
			href := h
			consumers = append(consumers, pce.Actor{Label: &pce.LabelRef{Href: href}})
		}
		rules = append(rules, pce.SecRule{
			Enabled:           true,
			ResolveLabelsAs:   pce.ResolveLabelsAs{Providers: []string{"workloads"}, Consumers: []string{"workloads"}},
			Providers:         providers,
			Consumers:         consumers,
			IngressServices:   a.Ports,
			UnscopedConsumers: true,
		})
	}
	return rules
}

// protoNumber maps a k8s protocol string to its IANA number (default TCP).
func protoNumber(protocol string) int {
	if protocol == "UDP" {
		return 17
	}
	return 6
}
