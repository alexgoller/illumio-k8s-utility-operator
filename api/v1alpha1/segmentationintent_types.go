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

// IntentAllow allows a consumer (referenced by existing Illumio labels) to
// reach this namespace's app on the given ports.
type IntentAllow struct {
	// From is the consumer's Illumio labels (key -> value), e.g.
	// {"app":"checkout","env":"prod"}. They must already exist in the PCE.
	From map[string]string `json:"from"`
	// Ports the consumer may reach. Empty means all ports.
	// +optional
	Ports []IntentPort `json:"ports,omitempty"`
}

// SegmentationIntentSpec is an app team's allow-list for their namespace's app.
type SegmentationIntentSpec struct {
	// Allow is the set of permitted inbound flows to this namespace's app.
	// +kubebuilder:validation:MinItems=1
	Allow []IntentAllow `json:"allow"`
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
