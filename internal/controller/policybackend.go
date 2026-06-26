package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

const (
	siRequeueNotReady = 30 * time.Second
	siRequeueHealthy  = 10 * time.Minute

	provisionModeAuto   = "auto"
	provisionModeManual = "manual"
)

// Condition carries a condition status + reason + message for a BackendResult.
type Condition struct {
	Status  metav1.ConditionStatus
	Reason  string
	Message string
}

// BackendResult holds the outcome of ReconcilePolicy.
type BackendResult struct {
	Ready                *Condition
	Provisioned          *Condition
	WorkloadsAffected    int
	Requeue              time.Duration
	EffectiveEnforcement string
	EnforcementSetBy     string
}

// applyBackendResult writes the Ready and Provisioned conditions from res onto
// conds. It is used by both policy front-end controllers.
func applyBackendResult(conds *[]metav1.Condition, res BackendResult) {
	if res.Ready != nil {
		meta.SetStatusCondition(conds, metav1.Condition{
			Type:    microv1.ConditionReady,
			Status:  res.Ready.Status,
			Reason:  res.Ready.Reason,
			Message: res.Ready.Message,
		})
	}
	if res.Provisioned != nil {
		meta.SetStatusCondition(conds, metav1.Condition{
			Type:    microv1.ConditionProvisioned,
			Status:  res.Provisioned.Status,
			Reason:  res.Provisioned.Reason,
			Message: res.Provisioned.Message,
		})
	}
}

// ReconcilePolicy is the shared policy reconciliation backend. It orchestrates:
//
//  1. Resolve the ClusterProfile for the namespace (→ Ready=False if not found).
//  2. Resolve the provider labels (namespace own labels → PCE hrefs).
//  3. Resolve the consumer labels in allows (→ Ready=False/Rejected if missing).
//  4. Reconcile the owned ruleset + rules in draft.
//  5. Provision per the ClusterProfile provisioning mode and the manual-gate
//     annotation.
//
// Guardrail rejections are returned as a BackendResult with Ready=False (not as
// the error return). The error return is reserved for transient errors that
// warrant an immediate requeue.
func ReconcilePolicy(
	ctx context.Context,
	k8s client.Client,
	factory PolicyClientFactory,
	namespace, crName string,
	uid types.UID,
	annotations map[string]string,
	allows []CompiledAllow,
) (BackendResult, error) {
	cp, cfg, eds, ready, transientErr := resolveClusterProfile(ctx, k8s, namespace)
	if transientErr != nil {
		return BackendResult{}, transientErr
	}
	if !ready {
		return BackendResult{
			Ready: &Condition{
				Status:  metav1.ConditionFalse,
				Reason:  microv1.ReasonClusterProfileNotReady,
				Message: "no Onboarded ClusterProfile / PCEConnection available",
			},
			Requeue: siRequeueNotReady,
		}, nil
	}

	// Compute the effective enforcement for the namespace (admin baseline + policy CRs).
	var effEnforcement, enfSetBy string
	{
		var nsObj corev1.Namespace
		if err := k8s.Get(ctx, types.NamespacedName{Name: namespace}, &nsObj); err == nil {
			baseline := ComputeDesiredCWP(namespace, nsObj.Labels, nsObj.Annotations, cp.Spec.NamespaceRules, cp.Spec.SystemNamespaces).EnforcementMode
			effEnforcement, enfSetBy, _ = EffectiveEnforcement(ctx, k8s, namespace, baseline)
		}
	}

	pclient := factory(cfg)
	owner := pce.Owner{DataSet: eds, Reference: string(uid)}

	providerHrefs, reason, msg, ok, err := resolveProvider(ctx, k8s, namespace, cp, pclient)
	if err != nil {
		return BackendResult{}, err
	}
	if !ok {
		return BackendResult{
			Ready: &Condition{
				Status:  metav1.ConditionFalse,
				Reason:  reason,
				Message: msg,
			},
			Requeue: siRequeueHealthy,
		}, nil
	}

	resolved, reason, msg, ok, err := resolveAllows(ctx, allows, pclient)
	if err != nil {
		return BackendResult{}, err
	}
	if !ok {
		return BackendResult{
			Ready: &Condition{
				Status:  metav1.ConditionFalse,
				Reason:  reason,
				Message: msg,
			},
			Requeue: siRequeueHealthy,
		}, nil
	}

	rsHref, err := reconcileRuleSet(ctx, pclient, crName, namespace, owner, providerHrefs, resolved)
	if err != nil {
		return BackendResult{}, err
	}

	// Determine provisioning.
	approved := annotations[microv1.AnnotationProvisionApprove] == "approved"
	doProvision := cp.Spec.ProvisioningMode == provisionModeAuto ||
		(cp.Spec.ProvisioningMode == provisionModeManual && approved)

	readyCond := &Condition{
		Status:  metav1.ConditionTrue,
		Reason:  microv1.ReasonCompiled,
		Message: "compiled to Illumio ruleset",
	}

	if doProvision {
		res, perr := pclient.ProvisionRuleSets(ctx, []string{rsHref}, fmt.Sprintf("%s/%s", namespace, crName))
		if perr != nil {
			return BackendResult{}, perr
		}
		return BackendResult{
			Ready: readyCond,
			Provisioned: &Condition{
				Status:  metav1.ConditionTrue,
				Reason:  microv1.ReasonProvisioned,
				Message: fmt.Sprintf("provisioned; %d workloads affected", res.WorkloadsAffected),
			},
			WorkloadsAffected:    res.WorkloadsAffected,
			Requeue:              siRequeueHealthy,
			EffectiveEnforcement: effEnforcement,
			EnforcementSetBy:     enfSetBy,
		}, nil
	}

	var pendingMsg string
	if cp.Spec.ProvisioningMode == provisionModeManual {
		pendingMsg = "draft written; awaiting " + microv1.AnnotationProvisionApprove + "=approved annotation"
	} else {
		pendingMsg = "draft written; provisioning is " + cp.Spec.ProvisioningMode
	}
	return BackendResult{
		Ready: readyCond,
		Provisioned: &Condition{
			Status:  metav1.ConditionFalse,
			Reason:  microv1.ReasonProvisionPending,
			Message: pendingMsg,
		},
		Requeue:              siRequeueHealthy,
		EffectiveEnforcement: effEnforcement,
		EnforcementSetBy:     enfSetBy,
	}, nil
}

// FinalizePolicy deletes the owned ruleset and provisions the removal. Best-effort:
// if the PCE is unreachable the error is surfaced to the caller (which may choose
// to allow deletion to proceed rather than blocking k8s object removal indefinitely).
func FinalizePolicy(
	ctx context.Context,
	k8s client.Client,
	factory PolicyClientFactory,
	namespace string,
	uid types.UID,
) error {
	cp, cfg, eds, ready, err := resolveClusterProfile(ctx, k8s, namespace)
	if err != nil {
		return err
	}
	if !ready {
		// Can't reach the PCE; allow deletion to proceed rather than blocking k8s
		// object removal indefinitely. Orphaned draft ruleset is cleaned on next
		// full reconcile or manually.
		return nil
	}
	_ = cp
	pclient := factory(cfg)
	owner := pce.Owner{DataSet: eds, Reference: string(uid)}
	rs, err := pclient.FindRuleSetByOwner(ctx, owner)
	if err != nil || rs == nil {
		return err
	}
	if err := pclient.DeleteRuleSet(ctx, rs.Href); err != nil {
		return err
	}
	_, err = pclient.ProvisionRuleSets(ctx, []string{rs.Href}, "delete "+namespace+"/"+string(uid))
	return err
}

// resolveClusterProfile finds an Onboarded ClusterProfile whose namespace rules
// cover nsName, and loads its PCE config. In production there is one ClusterProfile
// per cluster; nsName filters in multi-CP test environments.
func resolveClusterProfile(ctx context.Context, k8s client.Client, nsName string) (*microv1.ClusterProfile, pce.Config, string, bool, error) {
	var nsList corev1.Namespace
	if err := k8s.Get(ctx, types.NamespacedName{Name: nsName}, &nsList); err != nil {
		return nil, pce.Config{}, "", false, client.IgnoreNotFound(err)
	}
	var list microv1.ClusterProfileList
	if err := k8s.List(ctx, &list); err != nil {
		return nil, pce.Config{}, "", false, err
	}
	for i := range list.Items {
		cp := &list.Items[i]
		if !meta.IsStatusConditionTrue(cp.Status.Conditions, microv1.ConditionOnboarded) {
			continue
		}
		// Only use this CP if it manages the intent's namespace.
		desired := ComputeDesiredCWP(nsName, nsList.Labels, nsList.Annotations, cp.Spec.NamespaceRules, cp.Spec.SystemNamespaces)
		if !desired.Managed {
			continue
		}
		var conn microv1.PCEConnection
		if err := k8s.Get(ctx, types.NamespacedName{Name: cp.Spec.PCEConnectionRef.Name}, &conn); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return nil, pce.Config{}, "", false, err
		}
		if !meta.IsStatusConditionTrue(conn.Status.Conditions, microv1.ConditionConnected) {
			continue
		}
		var secret corev1.Secret
		key := types.NamespacedName{Name: conn.Spec.CredentialsSecretRef.Name, Namespace: conn.Spec.CredentialsSecretRef.Namespace}
		if err := k8s.Get(ctx, key, &secret); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return nil, pce.Config{}, "", false, err
		}
		apiKey, apiSecret := string(secret.Data["api_key"]), string(secret.Data["api_secret"])
		if apiKey == "" || apiSecret == "" {
			continue
		}
		eds := conn.Spec.ExternalDataSet
		if eds == "" {
			eds = defaultExternalDataSet
		}
		return cp, pce.Config{PCEURL: conn.Spec.PCEURL, OrgID: conn.Spec.OrgID, APIKey: apiKey, APISecret: apiSecret}, eds, true, nil
	}
	return nil, pce.Config{}, "", false, nil
}

// resolveProvider derives the namespace's own labels and resolves them to hrefs.
func resolveProvider(ctx context.Context, k8s client.Client, nsName string, cp *microv1.ClusterProfile, pclient PolicyClient) ([]string, string, string, bool, error) {
	var ns corev1.Namespace
	if err := k8s.Get(ctx, types.NamespacedName{Name: nsName}, &ns); err != nil {
		return nil, microv1.ReasonRejected, "namespace not found", false, client.IgnoreNotFound(err)
	}
	desired := ComputeDesiredCWP(ns.Name, ns.Labels, ns.Annotations, cp.Spec.NamespaceRules, cp.Spec.SystemNamespaces)
	if !desired.Managed || len(desired.Labels) == 0 {
		return nil, microv1.ReasonRejected, "namespace is not managed or has no Illumio labels; an admin must manage it via ClusterProfile", false, nil
	}
	hrefs := make([]string, 0, len(desired.Labels))
	for key, value := range desired.Labels {
		lbl, err := pclient.FindLabel(ctx, key, value)
		if err != nil {
			if errors.Is(err, pce.ErrLabelNotFound) {
				return nil, microv1.ReasonRejected, fmt.Sprintf("namespace label %s=%s not yet in the PCE", key, value), false, nil
			}
			return nil, "", "", false, err
		}
		hrefs = append(hrefs, lbl.Href)
	}
	return hrefs, "", "", true, nil
}

// resolveAllows resolves consumer labels in each CompiledAllow to existing PCE
// hrefs (never creates). Returns the resolved list or a rejection reason.
func resolveAllows(ctx context.Context, allows []CompiledAllow, pclient PolicyClient) ([]ResolvedAllow, string, string, bool, error) {
	out := make([]ResolvedAllow, 0, len(allows))
	for _, a := range allows {
		consumerHrefs := make([]string, 0, len(a.From))
		for key, value := range a.From {
			lbl, err := pclient.FindLabel(ctx, key, value)
			if err != nil {
				if errors.Is(err, pce.ErrLabelNotFound) {
					return nil, microv1.ReasonRejected, fmt.Sprintf("no Illumio label %s=%s in the PCE", key, value), false, nil
				}
				return nil, "", "", false, err
			}
			consumerHrefs = append(consumerHrefs, lbl.Href)
		}
		out = append(out, ResolvedAllow{ConsumerHrefs: consumerHrefs, Ports: a.Ports})
	}
	return out, "", "", true, nil
}

// reconcileRuleSet ensures the owned ruleset exists and its rules match desired
// (replace-all). Returns the ruleset href.
func reconcileRuleSet(ctx context.Context, pclient PolicyClient, crName, namespace string, owner pce.Owner, providerHrefs []string, resolved []ResolvedAllow) (string, error) {
	existing, err := pclient.FindRuleSetByOwner(ctx, owner)
	if err != nil {
		return "", err
	}
	var rsHref string
	if existing == nil {
		created, cerr := pclient.CreateRuleSet(ctx, BuildRuleSet(namespace, crName, providerHrefs, owner))
		if cerr != nil {
			return "", cerr
		}
		rsHref = created.Href
	} else {
		rsHref = existing.Href
		// Remove existing rules (replace-all). Note: the DRAFT ruleset is
		// transiently partial while rules are deleted and recreated here, but
		// the ENFORCED (provisioned/active) policy is untouched — ProvisionRuleSets
		// only runs after the draft is fully rebuilt below.
		rules, lerr := pclient.ListRules(ctx, rsHref)
		if lerr != nil {
			return "", lerr
		}
		for i := range rules {
			if derr := pclient.DeleteRule(ctx, rules[i].Href); derr != nil {
				return "", derr
			}
		}
	}
	for _, rule := range BuildRules(providerHrefs, resolved) {
		if _, cerr := pclient.CreateRule(ctx, rsHref, rule); cerr != nil {
			return "", cerr
		}
	}
	return rsHref, nil
}
