package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// NetworkPolicyPeer is a consumer selector (a supported subset of k8s NetworkPolicyPeer).
type NetworkPolicyPeer struct {
	// +optional
	PodSelector *metav1.LabelSelector `json:"podSelector,omitempty"`
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`
}

// NetworkPolicyPort is a port/protocol.
type NetworkPolicyPort struct {
	Port int `json:"port"`
	// +kubebuilder:validation:Enum=TCP;UDP
	// +kubebuilder:default=TCP
	// +optional
	Protocol string `json:"protocol,omitempty"`
}

// IngressRule allows traffic from the listed peers on the listed ports.
type IngressRule struct {
	From  []NetworkPolicyPeer `json:"from"`
	Ports []NetworkPolicyPort `json:"ports,omitempty"`
}

// SegmentationPolicySpec mirrors a supported subset of k8s NetworkPolicy.
type SegmentationPolicySpec struct {
	// PodSelector selects the pods to which this policy applies.
	// +optional
	PodSelector metav1.LabelSelector `json:"podSelector,omitempty"`
	// Ingress rules (the only supported direction).
	Ingress []IngressRule `json:"ingress"`
	// PolicyTypes must be ["Ingress"].
	// +optional
	PolicyTypes []string `json:"policyTypes,omitempty"`
	// Enforcement requests a namespace enforcement mode (see SegmentationIntent).
	// +kubebuilder:validation:Enum=idle;visibility_only;selective;full
	// +optional
	Enforcement string `json:"enforcement,omitempty"`
}

// SegmentationPolicyStatus is the observed state.
type SegmentationPolicyStatus struct {
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// +optional
	WorkloadsAffected int `json:"workloadsAffected,omitempty"`
	// +optional
	EffectiveEnforcement string `json:"effectiveEnforcement,omitempty"`
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

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=illumio,shortName=segpol
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Provisioned",type=string,JSONPath=`.status.conditions[?(@.type=="Provisioned")].status`
// +kubebuilder:printcolumn:name="Enforcement",type=string,JSONPath=`.status.effectiveEnforcement`

// SegmentationPolicy is a NetworkPolicy-style Illumio allow-list for a namespace.
type SegmentationPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SegmentationPolicySpec   `json:"spec,omitempty"`
	Status SegmentationPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SegmentationPolicyList contains a list of SegmentationPolicy.
type SegmentationPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SegmentationPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &SegmentationPolicy{}, &SegmentationPolicyList{})
		return nil
	})
}
