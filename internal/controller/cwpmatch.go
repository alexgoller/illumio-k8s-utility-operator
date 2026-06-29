package controller

import (
	"maps"
	"path"
	"strings"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
)

const (
	// enforcementIdle is invalid for a managed Container Workload Profile: the
	// PCE rejects managed+idle (container_workload_profile_invalid_managed_idle).
	// "managed" means managed by Illumio; idle is meaningless on pods, so a
	// managed namespace floors to enforcementVisibilityOnly.
	enforcementIdle = "idle"
	// enforcementVisibilityOnly is the minimum (and default) enforcement for a
	// managed namespace: workloads are managed and traffic is logged, not blocked.
	enforcementVisibilityOnly = "visibility_only"
)

// DesiredCWP is the computed desired Container Workload Profile config for a namespace.
type DesiredCWP struct {
	Managed         bool
	Labels          map[string]string // Illumio label key -> value
	EnforcementMode string
}

// defaultSystemPatterns is the built-in list of system namespace globs.
// kube-* covers kube-system, kube-public, and kube-node-lease.
var defaultSystemPatterns = []string{
	"openshift-*", "kube-*", "default",
}

// ComputeDesiredCWP resolves the desired CWP for a namespace. Precedence:
// systemNamespaces > user rules (first match) > unmanaged default; then
// per-namespace annotations override the result.
func ComputeDesiredCWP(name string, nsLabels, nsAnnotations map[string]string, rules []microv1.NamespaceRule, sys microv1.SystemNamespacesSpec) DesiredCWP {
	d := DesiredCWP{Managed: false, Labels: map[string]string{}, EnforcementMode: ""}

	// 1. First matching user rule sets the base (lowest precedence among matchers).
	for i := range rules {
		if ruleMatches(rules[i].Match, name, nsLabels) {
			d.Managed = rules[i].Managed
			d.Labels = resolveAssignLabels(rules[i].AssignLabels, nsLabels)
			d.EnforcementMode = rules[i].EnforcementMode
			break
		}
	}

	// 2. System-namespace spec overrides user rules (higher precedence).
	if sys.Manage && matchesAnyPattern(name, systemPatterns(sys)) {
		d.Managed = true
		d.Labels = copyLabels(sys.Labels)
		d.EnforcementMode = sys.EnforcementMode
	}

	// 3. Annotation overrides.
	applyAnnotationOverrides(&d, nsAnnotations)

	// 4. A managed namespace is managed by Illumio, and the PCE rejects a managed
	// CWP with idle enforcement (container_workload_profile_invalid_managed_idle).
	// Floor an unset or idle mode to visibility_only (logs traffic, does not block).
	if d.Managed && (d.EnforcementMode == "" || d.EnforcementMode == enforcementIdle) {
		d.EnforcementMode = enforcementVisibilityOnly
	}
	return d
}

func systemPatterns(sys microv1.SystemNamespacesSpec) []string {
	if len(sys.Patterns) > 0 {
		return sys.Patterns
	}
	return defaultSystemPatterns
}

func matchesAnyPattern(name string, patterns []string) bool {
	for _, p := range patterns {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}

func ruleMatches(m microv1.NamespaceMatch, name string, nsLabels map[string]string) bool {
	if m.NamePattern != "" {
		if ok, _ := path.Match(m.NamePattern, name); !ok {
			return false
		}
	}
	for k, v := range m.Labels {
		if nsLabels[k] != v {
			return false
		}
	}
	return true
}

func resolveAssignLabels(assign map[string]microv1.LabelAssignment, nsLabels map[string]string) map[string]string {
	out := map[string]string{}
	for key, a := range assign {
		switch {
		case a.Value != "":
			out[key] = a.Value
		case a.FromNamespaceLabel != "":
			if v, ok := nsLabels[a.FromNamespaceLabel]; ok && v != "" {
				out[key] = v
			}
		}
	}
	return out
}

func applyAnnotationOverrides(d *DesiredCWP, annos map[string]string) {
	if v, ok := annos[microv1.AnnotationManaged]; ok {
		d.Managed = strings.EqualFold(v, "true")
	}
	if v, ok := annos[microv1.AnnotationEnforcement]; ok && v != "" {
		d.EnforcementMode = v
	}
	for k, v := range annos {
		if strings.HasPrefix(k, microv1.AnnotationLabelPrefix) && v != "" {
			labelKey := strings.TrimPrefix(k, microv1.AnnotationLabelPrefix)
			if labelKey != "" {
				if d.Labels == nil {
					d.Labels = map[string]string{}
				}
				d.Labels[labelKey] = v
			}
		}
	}
}

func copyLabels(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return out
}
