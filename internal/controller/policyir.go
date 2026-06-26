package controller

import (
	"fmt"
	"maps"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

const (
	// resolveWorkloads is the Illumio label resolution mode for pod workloads.
	resolveWorkloads = "workloads"
	// defaultExternalDataSet is the external_data_set stamped on PCE objects the operator creates.
	defaultExternalDataSet = "illumio-operator"
	// policyTypeIngress is the only supported policyType for SegmentationPolicy.
	policyTypeIngress = "Ingress"
	// enforcementFull is the strictest enforcement mode.
	enforcementFull = "full"
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
			ResolveLabelsAs:   pce.ResolveLabelsAs{Providers: []string{resolveWorkloads}, Consumers: []string{resolveWorkloads}},
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

// CompiledAllow is a front-end-agnostic allow entry: consumer labels (key->value)
// and proto-resolved ports.
type CompiledAllow struct {
	From  map[string]string
	Ports []pce.IngressService
}

// CompileIntent lowers a SegmentationIntent's allow list to CompiledAllow.
func CompileIntent(allows []microv1.IntentAllow) []CompiledAllow {
	out := make([]CompiledAllow, 0, len(allows))
	for _, a := range allows {
		ports := make([]pce.IngressService, 0, len(a.Ports))
		for _, p := range a.Ports {
			ports = append(ports, pce.IngressService{Proto: protoNumber(p.Protocol), Port: p.Port})
		}
		out = append(out, CompiledAllow{From: a.From, Ports: ports})
	}
	return out
}

// CompilePolicy lowers a SegmentationPolicy (supported NetworkPolicy subset) to
// CompiledAllow, returning a descriptive error for any unsupported construct.
// Each peer in a from list is emitted as a separate CompiledAllow so that
// NetworkPolicy OR semantics are preserved (peers are not merged/ANDed).
func CompilePolicy(spec microv1.SegmentationPolicySpec) ([]CompiledAllow, error) {
	for _, t := range spec.PolicyTypes {
		if t != policyTypeIngress {
			return nil, fmt.Errorf("unsupported policyType %q: only Ingress is supported", t)
		}
	}
	if len(spec.PodSelector.MatchLabels) > 0 || len(spec.PodSelector.MatchExpressions) > 0 {
		return nil, fmt.Errorf("spec.podSelector must be empty: the policy applies to the whole namespace's app")
	}
	out := make([]CompiledAllow, 0)
	for i, ing := range spec.Ingress {
		if len(ing.From) == 0 {
			return nil, fmt.Errorf("ingress[%d].from must list at least one peer (allow-all is not supported)", i)
		}
		ports := make([]pce.IngressService, 0, len(ing.Ports))
		for _, p := range ing.Ports {
			ports = append(ports, pce.IngressService{Proto: protoNumber(p.Protocol), Port: p.Port})
		}
		for j, peer := range ing.From {
			if peer.PodSelector == nil && peer.NamespaceSelector == nil {
				return nil, fmt.Errorf("ingress[%d].from[%d]: a podSelector or namespaceSelector is required (ipBlock is not supported)", i, j)
			}
			from := map[string]string{}
			for _, sel := range []*metav1.LabelSelector{peer.PodSelector, peer.NamespaceSelector} {
				if sel == nil {
					continue
				}
				if len(sel.MatchExpressions) > 0 {
					return nil, fmt.Errorf("ingress[%d].from[%d]: matchExpressions are not supported; use matchLabels", i, j)
				}
				maps.Copy(from, sel.MatchLabels)
			}
			if len(from) == 0 {
				return nil, fmt.Errorf("ingress[%d].from[%d]: no matchLabels found", i, j)
			}
			out = append(out, CompiledAllow{From: from, Ports: ports})
		}
	}
	return out, nil
}

var enforcementRank = map[string]int{"": 0, "idle": 1, "visibility_only": 2, enforcementFull: 3}

// StrictestEnforcement returns the strictest non-empty mode, or "" if none.
func StrictestEnforcement(modes ...string) string {
	best := ""
	for _, m := range modes {
		if enforcementRank[m] > enforcementRank[best] {
			best = m
		}
	}
	return best
}
