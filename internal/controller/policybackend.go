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
	"sigs.k8s.io/controller-runtime/pkg/log"

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
	Ready                 *Condition
	Provisioned           *Condition
	WorkloadsAffected     int
	Requeue               time.Duration
	EffectiveEnforcement  string
	EnforcementSetBy      string
	UnknownLabelMode      string
	UnknownLabelModeSetBy string
	DeferredLabels        []string
	CreatedLabels         []string
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
	providerLabels map[string]string,
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
	var nsAnnotations map[string]string
	{
		var nsObj corev1.Namespace
		if err := k8s.Get(ctx, types.NamespacedName{Name: namespace}, &nsObj); err == nil {
			nsAnnotations = nsObj.Annotations
			baseline := ComputeDesiredCWP(namespace, nsObj.Labels, nsObj.Annotations, cp.Spec.NamespaceRules, cp.Spec.SystemNamespaces).EnforcementMode
			effEnforcement, enfSetBy, _ = EffectiveEnforcement(ctx, k8s, namespace, baseline)
		}
	}

	// Resolve the unknown-label mode (CR annotation > namespace annotation > ClusterProfile default).
	mode, modeSetBy := microv1.ResolveUnknownLabelMode(cp.Spec.UnknownLabelMode, nsAnnotations, annotations)

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

	resolved, deferred, created, reason, msg, ok, err := resolveAllows(ctx, allows, pclient, mode, owner)
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
			Requeue:               siRequeueHealthy,
			UnknownLabelMode:      mode,
			UnknownLabelModeSetBy: modeSetBy,
		}, nil
	}

	// Resolve the optional provider narrowing labels (strict: the targeted service
	// must already exist in the PCE). Empty = the whole namespace app (ams in scope).
	var narrowHrefs []string
	for key, value := range providerLabels {
		lbl, lerr := pclient.FindLabel(ctx, key, value)
		if lerr != nil {
			if errors.Is(lerr, pce.ErrLabelNotFound) {
				return BackendResult{
					Ready: &Condition{Status: metav1.ConditionFalse, Reason: microv1.ReasonRejected,
						Message: fmt.Sprintf("provider label %s=%s not in the PCE", key, value)},
					Requeue:               siRequeueHealthy,
					UnknownLabelMode:      mode,
					UnknownLabelModeSetBy: modeSetBy,
				}, nil
			}
			return BackendResult{}, lerr
		}
		narrowHrefs = append(narrowHrefs, lbl.Href)
	}

	rsHref, err := reconcileRuleSet(ctx, pclient, crName, namespace, owner, providerHrefs, narrowHrefs, resolved)
	if err != nil {
		if errors.Is(err, errRuleSetPendingDeletion) {
			// Defer to the human: surface the conflict, don't override the pending
			// change. Re-check periodically — clears once the admin provisions or
			// reverts the deletion in the PCE.
			return BackendResult{
				Ready: &Condition{
					Status:  metav1.ConditionFalse,
					Reason:  microv1.ReasonPCEStateConflict,
					Message: "the operator-owned Illumio ruleset has an unprovisioned pending deletion in the PCE; resolve it there (provision or revert the deletion) — the operator will not override a pending change made in the PCE",
				},
				Requeue:               siRequeueNotReady,
				EffectiveEnforcement:  effEnforcement,
				EnforcementSetBy:      enfSetBy,
				UnknownLabelMode:      mode,
				UnknownLabelModeSetBy: modeSetBy,
			}, nil
		}
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
		log.FromContext(ctx).Info("provisioned policy ruleset",
			"namespace", namespace, "cr", crName, "ruleSet", rsHref,
			"version", res.Version, "workloadsAffected", res.WorkloadsAffected)
		return BackendResult{
			Ready: readyCond,
			Provisioned: &Condition{
				Status:  metav1.ConditionTrue,
				Reason:  microv1.ReasonProvisioned,
				Message: fmt.Sprintf("provisioned; %d workloads affected", res.WorkloadsAffected),
			},
			WorkloadsAffected:     res.WorkloadsAffected,
			Requeue:               siRequeueHealthy,
			EffectiveEnforcement:  effEnforcement,
			EnforcementSetBy:      enfSetBy,
			UnknownLabelMode:      mode,
			UnknownLabelModeSetBy: modeSetBy,
			DeferredLabels:        deferred,
			CreatedLabels:         created,
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
		Requeue:               siRequeueHealthy,
		EffectiveEnforcement:  effEnforcement,
		EnforcementSetBy:      enfSetBy,
		UnknownLabelMode:      mode,
		UnknownLabelModeSetBy: modeSetBy,
		DeferredLabels:        deferred,
		CreatedLabels:         created,
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
	// The ruleset scope is only the scope-label subset of the namespace's assigned
	// labels (default app+env; loc and other assigned labels stay on the workloads
	// for visibility but are not part of the scope).
	scopeKeys := cp.Spec.ScopeLabelKeys()
	scopeLabels := scopeLabelSubset(desired.Labels, scopeKeys)
	if len(scopeLabels) == 0 {
		return nil, microv1.ReasonRejected, fmt.Sprintf("namespace has none of the policy scope labels %v assigned via ClusterProfile; assign at least one to scope its ruleset", scopeKeys), false, nil
	}
	hrefs := make([]string, 0, len(scopeLabels))
	for key, value := range scopeLabels {
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

// scopeLabelSubset keeps only the label keys that are part of the ruleset scope.
func scopeLabelSubset(labels map[string]string, scopeKeys []string) map[string]string {
	keep := make(map[string]bool, len(scopeKeys))
	for _, k := range scopeKeys {
		keep[k] = true
	}
	out := make(map[string]string, len(labels))
	for k, v := range labels {
		if keep[k] {
			out[k] = v
		}
	}
	return out
}

// resolveAllows resolves consumer labels per the unknown-label mode. Returns the
// resolved allows, the "key=value" labels deferred (skip) or created (create),
// and a rejection (ok=false) for strict-mode unknowns or create of a non-standard key.
func resolveAllows(ctx context.Context, allows []CompiledAllow, pclient PolicyClient, mode string, owner pce.Owner) ([]ResolvedAllow, []string, []string, string, string, bool, error) {
	out := make([]ResolvedAllow, 0, len(allows))
	var deferred, created []string
	for _, a := range allows {
		// All-Workloads consumer (ams): no labels to resolve.
		if a.AllWorkloads {
			out = append(out, ResolvedAllow{AllWorkloads: true, IntraScope: a.IntraScope, Ports: a.Ports})
			continue
		}
		consumerHrefs := make([]string, 0, len(a.From))
		skippedAny := false
		for key, value := range a.From {
			lbl, err := pclient.FindLabel(ctx, key, value)
			if err == nil {
				consumerHrefs = append(consumerHrefs, lbl.Href)
				continue
			}
			if !errors.Is(err, pce.ErrLabelNotFound) {
				return nil, nil, nil, "", "", false, err
			}
			kv := key + "=" + value
			switch mode {
			case microv1.UnknownLabelSkip:
				deferred = append(deferred, kv)
				skippedAny = true
			case microv1.UnknownLabelCreate:
				if !autoCreatableKey(key) {
					return nil, nil, nil, microv1.ReasonRejected,
						fmt.Sprintf("cannot auto-create label with non-standard key %q (create mode allows role/app/env/loc only); pre-create %s in the PCE", key, kv), false, nil
				}
				lbl, cerr := pclient.EnsureLabel(ctx, key, value, owner)
				if cerr != nil {
					return nil, nil, nil, "", "", false, cerr
				}
				consumerHrefs = append(consumerHrefs, lbl.Href)
				created = append(created, kv)
			default: // strict
				return nil, nil, nil, microv1.ReasonRejected, fmt.Sprintf("no Illumio label %s in the PCE", kv), false, nil
			}
		}
		// In skip mode, drop an allow whose consumers were all unresolved.
		if skippedAny && len(consumerHrefs) == 0 {
			continue
		}
		out = append(out, ResolvedAllow{ConsumerHrefs: consumerHrefs, IntraScope: a.IntraScope, Ports: a.Ports})
	}
	return out, deferred, created, "", "", true, nil
}

// reconcileRuleSet ensures the owned ruleset exists and its rules match desired
// (replace-all). Returns the ruleset href.
// errRuleSetPendingDeletion signals that the operator-owned ruleset is marked
// for deletion in the PCE draft store but the deletion was never provisioned.
// The operator defers to the human rather than recreating or overriding it.
var errRuleSetPendingDeletion = errors.New("owned ruleset has an unprovisioned pending deletion in the PCE")

func reconcileRuleSet(ctx context.Context, pclient PolicyClient, crName, namespace string, owner pce.Owner, scopeHrefs, narrowHrefs []string, resolved []ResolvedAllow) (string, error) {
	existing, err := pclient.FindRuleSetByOwner(ctx, owner)
	if err != nil {
		return "", err
	}
	// A ruleset a human marked for deletion (but did not provision) still appears
	// in the draft list; operating on it 404s. Defer to the admin instead of
	// recreating or overriding their pending change.
	if existing != nil && existing.UpdateType == pce.RuleSetUpdateTypeDelete {
		return "", errRuleSetPendingDeletion
	}
	var rsHref string
	if existing == nil {
		created, cerr := pclient.CreateRuleSet(ctx, BuildRuleSet(namespace, crName, scopeHrefs, owner))
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
	// Any rule with no explicit ports references the built-in "All Services" service
	// (it has no valid inline form), so resolve its href once if needed.
	var allServicesHref string
	for _, a := range resolved {
		if len(a.Ports) == 0 {
			svc, serr := pclient.FindServiceByName(ctx, pce.AllServicesName)
			if serr != nil {
				return "", serr
			}
			if svc == nil {
				return "", fmt.Errorf("PCE service %q not found (required for all-ports rules)", pce.AllServicesName)
			}
			allServicesHref = svc.Href
			break
		}
	}
	for _, rule := range BuildRules(narrowHrefs, allServicesHref, resolved) {
		if _, cerr := pclient.CreateRule(ctx, rsHref, rule); cerr != nil {
			return "", cerr
		}
	}
	return rsHref, nil
}
