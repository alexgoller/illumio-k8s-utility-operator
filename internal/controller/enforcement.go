package controller

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
)

// EffectiveEnforcement returns the strictest enforcement for a namespace: the
// admin baseline raised by any policy CR's spec.enforcement. setBy names the CR
// that set the result, or "admin" if the baseline was not raised.
func EffectiveEnforcement(ctx context.Context, k8s client.Client, namespace, baseline string) (string, string, error) {
	mode, setBy := baseline, "admin"
	var intents microv1.SegmentationIntentList
	if err := k8s.List(ctx, &intents, client.InNamespace(namespace)); err != nil {
		return "", "", err
	}
	for i := range intents.Items {
		if raised := StrictestEnforcement(mode, intents.Items[i].Spec.Enforcement); raised != mode {
			mode, setBy = raised, intents.Items[i].Name
		}
	}
	var policies microv1.SegmentationPolicyList
	if err := k8s.List(ctx, &policies, client.InNamespace(namespace)); err != nil {
		return "", "", err
	}
	for i := range policies.Items {
		if raised := StrictestEnforcement(mode, policies.Items[i].Spec.Enforcement); raised != mode {
			mode, setBy = raised, policies.Items[i].Name
		}
	}
	return mode, setBy, nil
}
