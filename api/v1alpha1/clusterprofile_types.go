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

// NamespaceMatch selects namespaces by name glob and/or required k8s labels.
type NamespaceMatch struct {
	// NamePattern is a glob (path.Match syntax, e.g. "openshift-*"). Empty matches any name.
	// +optional
	NamePattern string `json:"namePattern,omitempty"`
	// Labels that must all be present on the namespace (subset match).
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// LabelAssignment assigns an Illumio label value: a fixed Value, or a value
// read from one of the namespace's own k8s labels.
type LabelAssignment struct {
	// +optional
	Value string `json:"value,omitempty"`
	// +optional
	FromNamespaceLabel string `json:"fromNamespaceLabel,omitempty"`
}

// NamespaceRule maps matching namespaces to a desired CWP configuration.
type NamespaceRule struct {
	Match NamespaceMatch `json:"match"`
	// Managed marks the namespace's CWP as PCE-managed.
	Managed bool `json:"managed"`
	// AssignLabels maps Illumio label keys (role/app/env/loc/custom) to values.
	// +optional
	AssignLabels map[string]LabelAssignment `json:"assignLabels,omitempty"`
	// EnforcementMode for the namespace. One of idle, visibility_only, full.
	// +kubebuilder:validation:Enum=idle;visibility_only;full
	// +optional
	EnforcementMode string `json:"enforcementMode,omitempty"`
}

// SystemNamespacesSpec is a convenience to manage the cluster's system
// namespaces (OpenShift/Kubernetes) out of the box. User NamespaceRules take
// precedence over these defaults.
type SystemNamespacesSpec struct {
	// Manage turns on management of system namespaces.
	// +optional
	Manage bool `json:"manage,omitempty"`
	// Patterns of system namespace name globs. Defaults (when empty) to:
	// openshift-*, kube-*, default, kube-system, kube-public, kube-node-lease.
	// +optional
	Patterns []string `json:"patterns,omitempty"`
	// Labels assigned to system-namespace CWPs.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// EnforcementMode for system namespaces. Defaults to idle.
	// +kubebuilder:validation:Enum=idle;visibility_only;full
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
	// SystemNamespaces manages OpenShift/Kubernetes system namespaces out of the box.
	// +optional
	SystemNamespaces SystemNamespacesSpec `json:"systemNamespaces,omitempty"`
	// NamespaceRules are evaluated in order; the first match wins. They take
	// precedence over SystemNamespaces.
	// +optional
	NamespaceRules []NamespaceRule `json:"namespaceRules,omitempty"`
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
	// ManagedNamespaces is the number of namespaces whose CWP is managed.
	// +optional
	ManagedNamespaces int `json:"managedNamespaces,omitempty"`
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

// Namespace annotation keys for per-namespace CWP overrides.
const (
	AnnotationManaged     = "microsegment.io/managed"     // "true"/"false"
	AnnotationEnforcement = "microsegment.io/enforcement" // idle|visibility_only|full
	AnnotationLabelPrefix = "microsegment.io/label."      // e.g. microsegment.io/label.env=prod
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories=illumio,shortName=cprof
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.onboarding.containerClusterName`
// +kubebuilder:printcolumn:name="ClusterID",type=string,JSONPath=`.status.containerClusterID`
// +kubebuilder:printcolumn:name="Onboarded",type=string,JSONPath=`.status.conditions[?(@.type=="Onboarded")].status`
// +kubebuilder:printcolumn:name="Managed-NS",type=integer,JSONPath=`.status.managedNamespaces`

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
