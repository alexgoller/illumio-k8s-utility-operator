package controller

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// meta_FindStatusCondition is a thin alias so tests read clearly.
func meta_FindStatusCondition(conds []metav1.Condition, t string) *metav1.Condition {
	return meta.FindStatusCondition(conds, t)
}
