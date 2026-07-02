package v1alpha1

// DefaultPolicyScopeLabels is the ruleset scope-label set used when a
// ClusterProfile does not specify PolicyScopeLabels. Illumio ruleset scope may
// be any number of labels; for Kubernetes namespaces app+env is the right scope
// almost always. loc is intentionally excluded — it is a poor scope choice.
var DefaultPolicyScopeLabels = []string{LabelKeyApp, LabelKeyEnv}

// ScopeLabelKeys returns the effective Illumio label keys that form the
// per-namespace ruleset scope: the configured PolicyScopeLabels, or the
// app+env default when unset/empty.
func (s ClusterProfileSpec) ScopeLabelKeys() []string {
	if len(s.PolicyScopeLabels) == 0 {
		return DefaultPolicyScopeLabels
	}
	return s.PolicyScopeLabels
}
