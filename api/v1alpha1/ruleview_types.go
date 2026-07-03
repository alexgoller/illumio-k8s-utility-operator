package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// RuleViewSpec configures how the operator mirrors the current Illumio rules that
// protect this namespace's app (where the app is the provider). Read-only: the
// operator never authors or edits rules from a RuleView.
type RuleViewSpec struct {
	// RefreshIntervalMinutes is the periodic sync cadence. The operator re-queries
	// the PCE Rule Search API every interval (and immediately on a
	// microsegment.io/refresh annotation change). Defaults to 5.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1440
	// +kubebuilder:default=5
	// +optional
	RefreshIntervalMinutes int `json:"refreshIntervalMinutes,omitempty"`
	// PolicyVersion selects which policy store to mirror: active (enforced) or draft.
	// +kubebuilder:validation:Enum=active;draft
	// +kubebuilder:default=active
	// +optional
	PolicyVersion string `json:"policyVersion,omitempty"`
}

// RuleSummary is one Illumio rule that protects this namespace's app.
type RuleSummary struct {
	// Href is the PCE rule href.
	Href string `json:"href"`
	// RulesetName is the ruleset the rule belongs to.
	// +optional
	RulesetName string `json:"rulesetName,omitempty"`
	// OwnedBy is "operator" when this operator authored the rule's ruleset, or
	// "external" when it was authored outside Kubernetes (e.g. the Illumio UI).
	OwnedBy string `json:"ownedBy"`
	// Type is allow, deny, or override_deny.
	// +optional
	Type string `json:"type,omitempty"`
	// Enabled reports whether the rule is enabled.
	// +optional
	Enabled bool `json:"enabled,omitempty"`
	// Consumers are the rule's sources, rendered as label-set strings
	// (e.g. "app=checkout;env=prod"), "ams" (All Workloads), or "ip_list:<name>".
	// +optional
	Consumers []string `json:"consumers,omitempty"`
	// Services are the allowed services, rendered as "<port>/<proto>" or "All Services".
	// +optional
	Services []string `json:"services,omitempty"`
}

// RuleView owned-by values.
const (
	RuleOwnedByOperator = "operator"
	RuleOwnedByExternal = "external"
)

// RuleView condition reasons (reuses ConditionReady).
const (
	// ReasonSynced marks Ready=True after a successful rule-search sync.
	ReasonSynced = "Synced"
)

// RuleViewStatus holds the last-synced view of the current rules.
type RuleViewStatus struct {
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ObservedAt is when the last successful sync ran.
	// +optional
	ObservedAt *metav1.Time `json:"observedAt,omitempty"`
	// RuleCount is the total number of rules found (before capping).
	// +optional
	RuleCount int `json:"ruleCount,omitempty"`
	// OwnedCount is how many rules this operator authored.
	// +optional
	OwnedCount int `json:"ownedCount,omitempty"`
	// ExternalCount is how many rules were authored outside Kubernetes.
	// +optional
	ExternalCount int `json:"externalCount,omitempty"`
	// Truncated is true when the listed rules were capped (counts stay exact).
	// +optional
	Truncated bool `json:"truncated,omitempty"`
	// Rules is the (capped) list of rules protecting this namespace's app.
	// +optional
	Rules []RuleSummary `json:"rules,omitempty"`
	// ObservedGeneration is the spec generation the current status reflects.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// ObservedRefresh is the microsegment.io/refresh value honored by the last sync.
	// +optional
	ObservedRefresh string `json:"observedRefresh,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=illumio,shortName=rview
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Rules",type=integer,JSONPath=`.status.ruleCount`
// +kubebuilder:printcolumn:name="Owned",type=integer,JSONPath=`.status.ownedCount`
// +kubebuilder:printcolumn:name="External",type=integer,JSONPath=`.status.externalCount`
// +kubebuilder:printcolumn:name="Synced",type=date,JSONPath=`.status.observedAt`

// RuleView is a read-only, periodically-synced mirror of the current Illumio
// rules that protect this namespace's app (where the app is the provider),
// including rules authored outside Kubernetes.
type RuleView struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              RuleViewSpec   `json:"spec,omitempty"`
	Status            RuleViewStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RuleViewList contains a list of RuleView.
type RuleViewList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RuleView `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &RuleView{}, &RuleViewList{})
		return nil
	})
}
