package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// LocalObjectReference references a cluster-scoped object by name.
type LocalObjectReference struct {
	Name string `json:"name"`
}

// NodePairingProfileSpec configures the pairing profile the C-VEN uses to pair
// the cluster's nodes. Either reuse an existing profile by name, or have the
// operator create one with the given node labels and enforcement mode.
type NodePairingProfileSpec struct {
	// ExistingName: if set, use this existing PCE pairing profile (by name)
	// instead of creating one. Labels/EnforcementMode are ignored in that case.
	// +optional
	ExistingName string `json:"existingName,omitempty"`
	// Labels to apply to nodes paired with this profile, as Illumio
	// label-key -> value (e.g. {"role": "node", "env": "prod"}). The operator
	// resolves each to an Illumio label href (create-if-missing).
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// EnforcementMode for a created pairing profile. One of idle,
	// visibility_only, full. Defaults to idle.
	// +kubebuilder:validation:Enum=idle;visibility_only;full
	// +kubebuilder:default=idle
	// +optional
	EnforcementMode string `json:"enforcementMode,omitempty"`
}

// OnboardingSpec configures how the cluster is onboarded to the PCE.
type OnboardingSpec struct {
	// ContainerClusterName is the name of the PCE Container Cluster object to
	// ensure exists for this cluster.
	ContainerClusterName string `json:"containerClusterName"`
	// CredentialsOutputSecret is the name of the Secret (in the operator's
	// namespace) the operator writes the agent credentials into:
	// pce_url, cluster_id, cluster_token, cluster_code.
	CredentialsOutputSecret string `json:"credentialsOutputSecret"`
	// NodePairingProfile configures the pairing profile the C-VEN uses to pair
	// the cluster's nodes.
	// +optional
	NodePairingProfile NodePairingProfileSpec `json:"nodePairingProfile,omitempty"`
}

// ClusterProfileSpec is the desired onboarding state for this cluster.
type ClusterProfileSpec struct {
	// PCEConnectionRef references the PCEConnection to use.
	PCEConnectionRef LocalObjectReference `json:"pceConnectionRef"`
	// Onboarding configures PCE cluster onboarding.
	Onboarding OnboardingSpec `json:"onboarding"`
	// ProvisioningMode is the default policy provisioning mode for resources in
	// this cluster. One of: auto, manual, draft-only. Consumed by later
	// policy reconciliation; defaults to manual.
	// +kubebuilder:validation:Enum=auto;manual;draft-only
	// +kubebuilder:default=manual
	// +optional
	ProvisioningMode string `json:"provisioningMode,omitempty"`
}

// ClusterProfileStatus is the observed onboarding state.
type ClusterProfileStatus struct {
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ContainerClusterHref is the href of the PCE Container Cluster object.
	// +optional
	ContainerClusterHref string `json:"containerClusterHref,omitempty"`
	// ContainerClusterID is the cluster UUID (last segment of the href).
	// +optional
	ContainerClusterID string `json:"containerClusterID,omitempty"`
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// Condition types and reasons for ClusterProfile.
const (
	ConditionOnboarded = "Onboarded"

	ReasonOnboarded             = "Onboarded"
	ReasonPCEConnectionNotReady = "PCEConnectionNotReady"
	ReasonOnboardFailed         = "OnboardFailed"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories=illumio,shortName=cprof
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.onboarding.containerClusterName`
// +kubebuilder:printcolumn:name="ClusterID",type=string,JSONPath=`.status.containerClusterID`
// +kubebuilder:printcolumn:name="Onboarded",type=string,JSONPath=`.status.conditions[?(@.type=="Onboarded")].status`

// ClusterProfile onboards a Kubernetes cluster to an Illumio PCE.
type ClusterProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterProfileSpec   `json:"spec,omitempty"`
	Status ClusterProfileStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterProfileList contains a list of ClusterProfile.
type ClusterProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &ClusterProfile{}, &ClusterProfileList{})
		return nil
	})
}
