package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// IntentPort is a port/protocol a consumer is allowed to reach.
type IntentPort struct {
	Port int `json:"port"`
	// +kubebuilder:validation:Enum=TCP;UDP
	// +kubebuilder:default=TCP
	// +optional
	Protocol string `json:"protocol,omitempty"`
}

// IntentAllow allows a consumer to reach this namespace's app on the given ports.
// The consumer is EITHER From (a cross-app consumer, extra-scope) OR
// FromIntraNamespace (a consumer within this namespace, intra-scope) — set one.
type IntentAllow struct {
	// From is a cross-app consumer's Illumio labels (key -> value), e.g.
	// {"app":"checkout","env":"prod"}. They must already exist in the PCE.
	// This is an extra-scope source (it may live in any app).
	// +optional
	From map[string]string `json:"from,omitempty"`
	// FromIntraNamespace is a consumer WITHIN this namespace's scope, narrowed by
	// Illumio labels (typically {"role":"frontend"}). This is an intra-scope source
	// (same app), so the scope is not repeated. Empty map = all workloads in scope.
	// +optional
	FromIntraNamespace map[string]string `json:"fromIntraNamespace,omitempty"`
	// Ports the consumer may reach. Empty means all ports.
	// +optional
	Ports []IntentPort `json:"ports,omitempty"`
}

// SegmentationIntentSpec is an app team's allow-list for their namespace's app.
type SegmentationIntentSpec struct {
	// Provider optionally narrows the protected provider to a sub-set of this
	// namespace's app, by Illumio labels (e.g. {"role":"backend"}). The labels must
	// already exist in the PCE. Default (empty): the whole namespace app.
	// +optional
	Provider map[string]string `json:"provider,omitempty"`
	// Allow is the set of permitted inbound flows to this namespace's app.
	// +optional
	Allow []IntentAllow `json:"allow,omitempty"`
	// AllowIntraNamespace is a shortcut: when true, allow all workloads in this
	// namespace to reach each other on all ports (an intra-scope allow-all). Combine
	// with Allow for finer cross-app rules, or use alone for "allow any-any here".
	// +optional
	AllowIntraNamespace bool `json:"allowIntraNamespace,omitempty"`
	// Enforcement requests a namespace enforcement mode. The operator applies
	// the strictest mode requested across all policy CRs in the namespace (on
	// top of the admin baseline). One of idle, visibility_only, full.
	// +kubebuilder:validation:Enum=idle;visibility_only;full
	// +optional
	Enforcement string `json:"enforcement,omitempty"`
}

// SegmentationIntentStatus is the observed state.
type SegmentationIntentStatus struct {
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// WorkloadsAffected is the count from the last provisioning.
	// +optional
	WorkloadsAffected int `json:"workloadsAffected,omitempty"`
	// EffectiveEnforcement is the namespace's resolved enforcement mode.
	// +optional
	EffectiveEnforcement string `json:"effectiveEnforcement,omitempty"`
	// EnforcementSetBy names what set the effective enforcement (a CR name or "admin").
	// +optional
	EnforcementSetBy string `json:"enforcementSetBy,omitempty"`
	// UnknownLabelMode is the effective mode used to resolve referenced labels.
	// +optional
	UnknownLabelMode string `json:"unknownLabelMode,omitempty"`
	// UnknownLabelModeSetBy names where the mode came from (cr|namespace|clusterprofile|default).
	// +optional
	UnknownLabelModeSetBy string `json:"unknownLabelModeSetBy,omitempty"`
	// DeferredLabels are key=value consumer labels skipped because they do not yet exist (skip mode).
	// +optional
	DeferredLabels []string `json:"deferredLabels,omitempty"`
	// CreatedLabels are key=value labels minted while resolving (create mode).
	// +optional
	CreatedLabels []string `json:"createdLabels,omitempty"`
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// Condition types/reasons for SegmentationIntent.
const (
	ConditionReady       = "Ready"
	ConditionProvisioned = "Provisioned"

	ReasonCompiled               = "Compiled"
	ReasonRejected               = "Rejected"
	ReasonClusterProfileNotReady = "ClusterProfileNotReady"
	ReasonProvisioned            = "Provisioned"
	ReasonProvisionPending       = "ProvisionPending"

	// AnnotationProvisionApprove on a SegmentationIntent approves a pending
	// provision when provisioningMode is manual (value "approved").
	AnnotationProvisionApprove = "microsegment.io/provision"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=illumio,shortName=segintent
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Provisioned",type=string,JSONPath=`.status.conditions[?(@.type=="Provisioned")].status`
// +kubebuilder:printcolumn:name="Affected",type=integer,JSONPath=`.status.workloadsAffected`

// SegmentationIntent is an app team's Illumio allow-list for their namespace.
type SegmentationIntent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SegmentationIntentSpec   `json:"spec,omitempty"`
	Status SegmentationIntentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SegmentationIntentList contains a list of SegmentationIntent.
type SegmentationIntentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SegmentationIntent `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &SegmentationIntent{}, &SegmentationIntentList{})
		return nil
	})
}
