package controller

import (
	"fmt"
	"maps"

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

// ResolvedAllow is a CompiledAllow after consumer labels are resolved to hrefs.
// AllWorkloads means the consumer is "All Workloads" (ams) — no labels to resolve.
// IntraScope means the consumer is within the namespace's scope (unscoped_consumers=false).
type ResolvedAllow struct {
	ConsumerHrefs []string
	AllWorkloads  bool
	IntraScope    bool
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

// BuildRules builds one rule per allow entry. The ruleset scope (BuildRuleSet)
// constrains providers to the namespace's labels. providerHrefs optionally NARROWS
// the provider to a sub-set within that scope (e.g. role=backend); when empty,
// providers are "All Workloads in scope" (ams) — the whole app, scope not repeated.
// Consumers are the allow's resolved labels (extra- or intra-scope per allow).
// allServicesHref references the built-in "All Services" service for no-ports rules.
func BuildRules(providerHrefs []string, allServicesHref string, allows []ResolvedAllow) []pce.SecRule {
	var providers []pce.Actor
	if len(providerHrefs) == 0 {
		providers = []pce.Actor{pce.AllWorkloadsActor()}
	} else {
		for _, h := range providerHrefs {
			href := h
			providers = append(providers, pce.Actor{Label: &pce.LabelRef{Href: href}})
		}
	}
	rules := make([]pce.SecRule, 0, len(allows))
	for _, a := range allows {
		var consumers []pce.Actor
		if a.AllWorkloads {
			consumers = []pce.Actor{pce.AllWorkloadsActor()}
		} else {
			consumers = make([]pce.Actor, 0, len(a.ConsumerHrefs))
			for _, h := range a.ConsumerHrefs {
				href := h
				consumers = append(consumers, pce.Actor{Label: &pce.LabelRef{Href: href}})
			}
		}
		// No explicit ports = All Services (referenced by href; no valid inline form).
		ingress := a.Ports
		if len(ingress) == 0 {
			ingress = []pce.IngressService{{Href: allServicesHref}}
		}
		rules = append(rules, pce.SecRule{
			Enabled:           true,
			ResolveLabelsAs:   pce.ResolveLabelsAs{Providers: []string{resolveWorkloads}, Consumers: []string{resolveWorkloads}},
			Providers:         providers,
			Consumers:         consumers,
			IngressServices:   ingress,
			UnscopedConsumers: !a.IntraScope,
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

// CompiledAllow is a front-end-agnostic allow entry. The consumer is either a
// label set (From) or All Workloads (AllWorkloads). IntraScope marks the consumer
// as within the namespace's scope (intra-scope rule, unscoped_consumers=false);
// otherwise it is extra-scope (cross-app).
type CompiledAllow struct {
	From         map[string]string
	AllWorkloads bool
	IntraScope   bool
	Ports        []pce.IngressService
}

// CompileIntent lowers a SegmentationIntent spec to CompiledAllow. spec.allowIntraNamespace
// is the any-any-in-namespace shortcut (intra-scope, all workloads). Each allow sets exactly
// one of From (cross-app, extra-scope) or FromIntraNamespace (same-namespace, intra-scope).
func CompileIntent(spec microv1.SegmentationIntentSpec) ([]CompiledAllow, error) {
	out := make([]CompiledAllow, 0, len(spec.Allow)+1)
	if spec.AllowIntraNamespace {
		out = append(out, CompiledAllow{AllWorkloads: true, IntraScope: true})
	}
	for i, a := range spec.Allow {
		hasFrom := len(a.From) > 0
		hasIntra := len(a.FromIntraNamespace) > 0
		if hasFrom == hasIntra { // both set, or neither
			return nil, fmt.Errorf("allow[%d]: set exactly one of from (cross-app) or fromIntraNamespace (same-namespace)", i)
		}
		ports := make([]pce.IngressService, 0, len(a.Ports))
		for _, p := range a.Ports {
			ports = append(ports, pce.IngressService{Proto: protoNumber(p.Protocol), Port: p.Port})
		}
		if hasFrom {
			out = append(out, CompiledAllow{From: a.From, Ports: ports})
		} else {
			out = append(out, CompiledAllow{From: a.FromIntraNamespace, IntraScope: true, Ports: ports})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("intent is empty: set spec.allow or spec.allowIntraNamespace")
	}
	return out, nil
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
	// spec.podSelector narrows the provider within the namespace's app (matchLabels
	// → the provider sub-set; empty = the whole app). The labels themselves are
	// resolved by the backend; here we only reject the unsupported matchExpressions.
	if len(spec.PodSelector.MatchExpressions) > 0 {
		return nil, fmt.Errorf("spec.podSelector: matchExpressions are not supported; use matchLabels to narrow the provider")
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
			if peer.PodSelector != nil && peer.NamespaceSelector != nil {
				return nil, fmt.Errorf("ingress[%d].from[%d]: set podSelector (same-namespace, intra-scope) OR namespaceSelector (cross-namespace, extra-scope), not both", i, j)
			}
			// podSelector → intra-scope (same namespace); namespaceSelector → extra-scope.
			sel, intra := peer.PodSelector, true
			if sel == nil {
				sel, intra = peer.NamespaceSelector, false
			}
			if len(sel.MatchExpressions) > 0 {
				return nil, fmt.Errorf("ingress[%d].from[%d]: matchExpressions are not supported; use matchLabels", i, j)
			}
			// An empty podSelector means "all pods in this namespace" = intra-scope All Workloads.
			if intra && len(sel.MatchLabels) == 0 {
				out = append(out, CompiledAllow{AllWorkloads: true, IntraScope: true, Ports: ports})
				continue
			}
			if len(sel.MatchLabels) == 0 {
				return nil, fmt.Errorf("ingress[%d].from[%d]: namespaceSelector requires matchLabels", i, j)
			}
			from := map[string]string{}
			maps.Copy(from, sel.MatchLabels)
			out = append(out, CompiledAllow{From: from, IntraScope: intra, Ports: ports})
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
