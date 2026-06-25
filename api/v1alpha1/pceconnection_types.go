/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// SecretReference points to a Kubernetes Secret holding PCE API credentials.
type SecretReference struct {
	// Name of the Secret (keys: api_key, api_secret).
	Name string `json:"name"`
	// Namespace of the Secret. Defaults to the operator's namespace if empty.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// PCEConnectionSpec defines a connection to one Illumio PCE.
type PCEConnectionSpec struct {
	// PCEURL is the PCE host:port (443 for SaaS, 8443 typical on-prem).
	PCEURL string `json:"pceUrl"`
	// OrgID is the PCE organization id.
	OrgID int `json:"orgId"`
	// CredentialsSecretRef references the Secret with api_key / api_secret.
	CredentialsSecretRef SecretReference `json:"credentialsSecretRef"`
	// ExternalDataSet is the ownership tag stamped on PCE objects this
	// operator creates. Defaults to "illumio-operator" if empty.
	// +optional
	ExternalDataSet string `json:"externalDataSet,omitempty"`
}

// PCEConnectionStatus is the observed connection state.
type PCEConnectionStatus struct {
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// Condition types and reasons for PCEConnection.
const (
	ConditionConnected = "Connected"

	ReasonConnected      = "Connected"
	ReasonSecretMissing  = "SecretMissing"
	ReasonAuthFailed     = "AuthFailed"
	ReasonRateLimited    = "RateLimited"
	ReasonPCEUnreachable = "PCEUnreachable"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories=illumio,shortName=pceconn
// +kubebuilder:printcolumn:name="PCE",type=string,JSONPath=`.spec.pceUrl`
// +kubebuilder:printcolumn:name="Org",type=integer,JSONPath=`.spec.orgId`
// +kubebuilder:printcolumn:name="Connected",type=string,JSONPath=`.status.conditions[?(@.type=="Connected")].status`

// PCEConnection is the Schema for the pceconnections API
type PCEConnection struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of PCEConnection
	// +required
	Spec PCEConnectionSpec `json:"spec"`

	// status defines the observed state of PCEConnection
	// +optional
	Status PCEConnectionStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// PCEConnectionList contains a list of PCEConnection
type PCEConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []PCEConnection `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &PCEConnection{}, &PCEConnectionList{})
		return nil
	})
}
