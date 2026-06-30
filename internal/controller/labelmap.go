package controller

import (
	"context"
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
)

// labelMapListGVK is the Illumio Core-for-Kubernetes LabelMap CRD (5.3.0+).
// A LabelMap maps per-workload Kubernetes labels to Illumio labels; each
// workloadLabelMap entry's "toKey" is the Illumio label key it writes.
var labelMapListGVK = schema.GroupVersionKind{Group: "ic4k.illumio.com", Version: "v1alpha1", Kind: "LabelMapList"}

// operatorAssignedKeys returns the Illumio label keys this operator assigns at the
// namespace/CWP level (from ClusterProfile namespaceRules.assignLabels and
// systemNamespaces.labels). These are the operator-owned dimensions.
func operatorAssignedKeys(cp *microv1.ClusterProfile) map[string]bool {
	keys := map[string]bool{}
	for i := range cp.Spec.NamespaceRules {
		for k := range cp.Spec.NamespaceRules[i].AssignLabels {
			keys[k] = true
		}
	}
	for k := range cp.Spec.SystemNamespaces.Labels {
		keys[k] = true
	}
	return keys
}

// labelMapWorkloadKeys extracts the Illumio label keys (workloadLabelMap[].toKey)
// that the given LabelMap objects write at the per-workload level.
func labelMapWorkloadKeys(items []unstructured.Unstructured) map[string]bool {
	keys := map[string]bool{}
	for i := range items {
		entries, found, err := unstructured.NestedSlice(items[i].Object, "workloadLabelMap")
		if err != nil || !found {
			continue
		}
		for _, e := range entries {
			m, ok := e.(map[string]any)
			if !ok {
				continue
			}
			if tk, ok := m["toKey"].(string); ok && tk != "" {
				keys[tk] = true
			}
		}
	}
	return keys
}

// overlapKeys returns the sorted keys present in both sets.
func overlapKeys(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if b[k] {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}

// checkLabelMapOverlap detects an Illumio LabelMap that writes per-workload label
// keys this operator also assigns at the namespace level, and warns (warn-only —
// the operator never changes its own labeling). It sets the LabelMapOverlap
// condition on cp and emits a Warning event. Best-effort: if the LabelMap CRD is
// not installed, it is a no-op (no coordination needed).
func (r *ClusterProfileReconciler) checkLabelMapOverlap(ctx context.Context, cp *microv1.ClusterProfile) {
	reader := r.labelMapReader()
	ul := &unstructured.UnstructuredList{}
	ul.SetGroupVersionKind(labelMapListGVK)
	if err := reader.List(ctx, ul); err != nil {
		// CRD absent / no LabelMap support → nothing to coordinate.
		meta.SetStatusCondition(&cp.Status.Conditions, metav1.Condition{
			Type: microv1.ConditionLabelMapOverlap, Status: metav1.ConditionFalse,
			Reason: "NoLabelMap", Message: "no Illumio LabelMap detected",
		})
		return
	}
	overlapping := overlapKeys(operatorAssignedKeys(cp), labelMapWorkloadKeys(ul.Items))
	if len(overlapping) == 0 {
		meta.SetStatusCondition(&cp.Status.Conditions, metav1.Condition{
			Type: microv1.ConditionLabelMapOverlap, Status: metav1.ConditionFalse,
			Reason: "NoOverlap", Message: "LabelMap and this operator label distinct dimensions",
		})
		return
	}
	msg := fmt.Sprintf("Illumio LabelMap writes label key(s) %v that this operator also assigns via the ClusterProfile; two systems labeling the same dimension conflict — map those keys in only one place (LabelMap workloadLabelMap or ClusterProfile assignLabels), not both", overlapping)
	log.FromContext(ctx).Info("LabelMap overlap detected", "keys", overlapping)
	if r.Recorder != nil {
		r.Recorder.Eventf(cp, nil, corev1.EventTypeWarning, "LabelMapOverlap", "Coordinate", "%s", msg)
	}
	meta.SetStatusCondition(&cp.Status.Conditions, metav1.Condition{
		Type: microv1.ConditionLabelMapOverlap, Status: metav1.ConditionTrue,
		Reason: "Overlap", Message: msg,
	})
}

// labelMapReader prefers the uncached APIReader (so listing an unregistered CRD
// returns a clean error rather than failing to start an informer); falls back to
// the cached client.
func (r *ClusterProfileReconciler) labelMapReader() client.Reader {
	if r.APIReader != nil {
		return r.APIReader
	}
	return r.Client
}
