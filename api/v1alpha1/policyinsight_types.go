package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// PolicyInsightSpec requests a what-if preflight for the namespace it lives in.
// The preflight runs ON REQUEST ONLY (on create, on spec change, or when the
// microsegment.io/refresh annotation changes) — never on a timer. PCE traffic
// queries are expensive; the operator computes once per request and then idles.
type PolicyInsightSpec struct {
	// LookbackDays is the observation window (in days, ending now) the preflight
	// queries the PCE for. Defaults to 7.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=90
	// +kubebuilder:default=7
	// +optional
	LookbackDays int `json:"lookbackDays,omitempty"`
}

// FlowFinding is one observed flow the current draft policy would block.
type FlowFinding struct {
	// Peer is the Illumio labels of the other end (consumer for inbound, provider
	// for egress). May be empty for an unlabeled / off-cluster peer (see PeerIP).
	// +optional
	Peer map[string]string `json:"peer,omitempty"`
	// PeerIP is the other end's IP when it has no workload/labels (e.g. off-cluster).
	// +optional
	PeerIP string `json:"peerIP,omitempty"`
	// Port is the destination port of the flow.
	Port int `json:"port"`
	// Protocol is TCP or UDP.
	// +optional
	Protocol string `json:"protocol,omitempty"`
	// Connections is the observed connection count over the window.
	// +optional
	Connections int `json:"connections,omitempty"`
	// Decision is the draft policy decision that flagged this flow
	// (blocked or potentially_blocked).
	// +optional
	Decision string `json:"decision,omitempty"`
	// LastDetected is when the flow was last observed.
	// +optional
	LastDetected *metav1.Time `json:"lastDetected,omitempty"`
}

// ObservationWindow is the time range the preflight analyzed.
type ObservationWindow struct {
	// +optional
	From *metav1.Time `json:"from,omitempty"`
	// +optional
	To *metav1.Time `json:"to,omitempty"`
}

// PolicyInsightStatus holds the last computed preflight findings.
type PolicyInsightStatus struct {
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ObservedWindow is the time range analyzed by the last run.
	// +optional
	ObservedWindow *ObservationWindow `json:"observedWindow,omitempty"`
	// FlowsAnalyzed is the number of flows the last run examined.
	// +optional
	FlowsAnalyzed int `json:"flowsAnalyzed,omitempty"`
	// InboundBlockedCount is len(WouldBlockInbound) (for the print column).
	// +optional
	InboundBlockedCount int `json:"inboundBlockedCount,omitempty"`
	// EgressBlockedCount is len(BlockedEgress) (for the print column).
	// +optional
	EgressBlockedCount int `json:"egressBlockedCount,omitempty"`
	// Truncated is true when the flow result was capped (findings are partial).
	// +optional
	Truncated bool `json:"truncated,omitempty"`
	// WouldBlockInbound are flows TO this namespace's app the draft policy would
	// block at full enforcement (allow-list gaps).
	// +optional
	WouldBlockInbound []FlowFinding `json:"wouldBlockInbound,omitempty"`
	// BlockedEgress are flows FROM this namespace's workloads that are denied
	// (surfaced for awareness; this operator does not author egress policy).
	// +optional
	BlockedEgress []FlowFinding `json:"blockedEgress,omitempty"`
	// ObservedGeneration is the spec generation the current status reflects.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// ObservedRefresh is the microsegment.io/refresh annotation value honored by
	// the last run (used to detect an on-demand re-run request).
	// +optional
	ObservedRefresh string `json:"observedRefresh,omitempty"`
}

// PolicyInsight condition reasons (reuses ConditionReady).
const (
	// ReasonComputed marks Ready=True after a successful preflight run.
	ReasonComputed = "Computed"
	// ReasonQueryFailed marks Ready=False when the PCE traffic query failed.
	ReasonQueryFailed = "QueryFailed"
	// ReasonNoScopeLabels marks Ready=False when the namespace has no scope labels
	// to query by (not managed, or none of PolicyScopeLabels assigned).
	ReasonNoScopeLabels = "NoScopeLabels"

	// AnnotationRefresh, when changed, requests an on-demand preflight re-run.
	AnnotationRefresh = "microsegment.io/refresh"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=illumio,shortName=insight
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Inbound-Blocked",type=integer,JSONPath=`.status.inboundBlockedCount`
// +kubebuilder:printcolumn:name="Egress-Blocked",type=integer,JSONPath=`.status.egressBlockedCount`

// PolicyInsight is an on-request what-if preflight for a namespace: it reports
// the flows the current draft policy would block (inbound) and the egress flows
// that are denied, from observed PCE traffic. Read-only — it authors no policy.
type PolicyInsight struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PolicyInsightSpec   `json:"spec,omitempty"`
	Status            PolicyInsightStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PolicyInsightList contains a list of PolicyInsight.
type PolicyInsightList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PolicyInsight `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &PolicyInsight{}, &PolicyInsightList{})
		return nil
	})
}
