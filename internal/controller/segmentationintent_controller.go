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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

const (
	siRequeueNotReady  = 30 * time.Second
	siRequeueHealthy   = 10 * time.Minute
	segIntentFinalizer = "microsegment.io/segmentationintent"

	provisionModeAuto   = "auto"
	provisionModeManual = "manual"
)

// SegmentationIntentReconciler reconciles a SegmentationIntent.
type SegmentationIntentReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	NewPolicyClient PolicyClientFactory
}

// +kubebuilder:rbac:groups=microsegment.io,resources=segmentationintents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=microsegment.io,resources=segmentationintents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=microsegment.io,resources=segmentationintents/finalizers,verbs=update
// +kubebuilder:rbac:groups=microsegment.io,resources=clusterprofiles,verbs=get;list;watch
// +kubebuilder:rbac:groups=microsegment.io,resources=pceconnections,verbs=get;list;watch

func (r *SegmentationIntentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var si microv1.SegmentationIntent
	if err := r.Get(ctx, req.NamespacedName, &si); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if r.NewPolicyClient == nil {
		r.NewPolicyClient = DefaultPolicyClientFactory
	}

	// Handle deletion: run finalizer logic, then remove the finalizer.
	if !si.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&si, segIntentFinalizer) {
			if err := r.finalize(ctx, &si); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(&si, segIntentFinalizer)
			if err := r.Update(ctx, &si); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Ensure the finalizer is present on normal reconcile.
	if controllerutil.AddFinalizer(&si, segIntentFinalizer) {
		if err := r.Update(ctx, &si); err != nil {
			return ctrl.Result{}, err
		}
	}

	cp, cfg, eds, ready, transientErr := r.resolveClusterProfile(ctx, si.Namespace)
	if transientErr != nil {
		return ctrl.Result{}, transientErr
	}
	if !ready {
		return r.fail(ctx, &si, microv1.ConditionReady, microv1.ReasonClusterProfileNotReady,
			"no Onboarded ClusterProfile / PCEConnection available", siRequeueNotReady)
	}

	pclient := r.NewPolicyClient(cfg)
	owner := pce.Owner{DataSet: eds, Reference: string(si.UID)}

	// Provider labels = the namespace's own resolved CWP labels (guardrail).
	providerHrefs, reason, msg, ok, err := r.resolveProvider(ctx, &si, cp, pclient)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !ok {
		return r.fail(ctx, &si, microv1.ConditionReady, reason, msg, siRequeueHealthy)
	}

	// Consumers must resolve to existing PCE labels (guardrail).
	resolved, reason, msg, ok, err := r.resolveAllows(ctx, &si, pclient)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !ok {
		return r.fail(ctx, &si, microv1.ConditionReady, reason, msg, siRequeueHealthy)
	}

	// Compile + reconcile the owned ruleset and its rules in draft.
	rsHref, err := r.reconcileRuleSet(ctx, &si, pclient, owner, providerHrefs, resolved)
	if err != nil {
		return ctrl.Result{}, err
	}

	meta.SetStatusCondition(&si.Status.Conditions, metav1.Condition{
		Type: microv1.ConditionReady, Status: metav1.ConditionTrue, Reason: microv1.ReasonCompiled,
		Message: "compiled to Illumio ruleset",
	})

	// Provision per the cluster's mode.
	// auto: always provision.
	// manual: provision only when annotated with microsegment.io/provision=approved.
	// draft-only: never provision.
	approved := si.Annotations[microv1.AnnotationProvisionApprove] == "approved"
	doProvision := cp.Spec.ProvisioningMode == provisionModeAuto ||
		(cp.Spec.ProvisioningMode == provisionModeManual && approved)

	if doProvision {
		res, perr := pclient.ProvisionRuleSets(ctx, []string{rsHref}, fmt.Sprintf("%s/%s", si.Namespace, si.Name))
		if perr != nil {
			return ctrl.Result{}, perr
		}
		si.Status.WorkloadsAffected = res.WorkloadsAffected
		meta.SetStatusCondition(&si.Status.Conditions, metav1.Condition{
			Type: microv1.ConditionProvisioned, Status: metav1.ConditionTrue, Reason: microv1.ReasonProvisioned,
			Message: fmt.Sprintf("provisioned; %d workloads affected", res.WorkloadsAffected),
		})
	} else {
		var pendingMsg string
		if cp.Spec.ProvisioningMode == provisionModeManual {
			pendingMsg = "draft written; awaiting " + microv1.AnnotationProvisionApprove + "=approved annotation"
		} else {
			pendingMsg = "draft written; provisioning is " + cp.Spec.ProvisioningMode
		}
		meta.SetStatusCondition(&si.Status.Conditions, metav1.Condition{
			Type: microv1.ConditionProvisioned, Status: metav1.ConditionFalse, Reason: microv1.ReasonProvisionPending,
			Message: pendingMsg,
		})
	}

	si.Status.ObservedGeneration = si.Generation
	if err := r.Status().Update(ctx, &si); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: siRequeueHealthy}, nil
}

// finalize deletes the owned ruleset and provisions the removal. Best-effort:
// if the PCE is unreachable, allow deletion to proceed rather than blocking
// k8s object removal indefinitely.
func (r *SegmentationIntentReconciler) finalize(ctx context.Context, si *microv1.SegmentationIntent) error {
	cp, cfg, eds, ready, err := r.resolveClusterProfile(ctx, si.Namespace)
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
	pclient := r.NewPolicyClient(cfg)
	owner := pce.Owner{DataSet: eds, Reference: string(si.UID)}
	rs, err := pclient.FindRuleSetByOwner(ctx, owner)
	if err != nil || rs == nil {
		return err
	}
	if err := pclient.DeleteRuleSet(ctx, rs.Href); err != nil {
		return err
	}
	_, err = pclient.ProvisionRuleSets(ctx, []string{rs.Href}, "delete "+si.Namespace+"/"+si.Name)
	return err
}

// resolveClusterProfile finds an Onboarded ClusterProfile whose namespace rules
// cover nsName, and loads its PCE config. In production there is one ClusterProfile
// per cluster; nsName filters in multi-CP test environments.
func (r *SegmentationIntentReconciler) resolveClusterProfile(ctx context.Context, nsName string) (*microv1.ClusterProfile, pce.Config, string, bool, error) {
	var nsList corev1.Namespace
	if err := r.Get(ctx, types.NamespacedName{Name: nsName}, &nsList); err != nil {
		return nil, pce.Config{}, "", false, client.IgnoreNotFound(err)
	}
	var list microv1.ClusterProfileList
	if err := r.List(ctx, &list); err != nil {
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
		if err := r.Get(ctx, types.NamespacedName{Name: cp.Spec.PCEConnectionRef.Name}, &conn); err != nil {
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
		if err := r.Get(ctx, key, &secret); err != nil {
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
func (r *SegmentationIntentReconciler) resolveProvider(ctx context.Context, si *microv1.SegmentationIntent, cp *microv1.ClusterProfile, pclient PolicyClient) ([]string, string, string, bool, error) {
	var ns corev1.Namespace
	if err := r.Get(ctx, types.NamespacedName{Name: si.Namespace}, &ns); err != nil {
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

// resolveAllows resolves consumer labels to existing PCE hrefs (never creates).
func (r *SegmentationIntentReconciler) resolveAllows(ctx context.Context, si *microv1.SegmentationIntent, pclient PolicyClient) ([]ResolvedAllow, string, string, bool, error) {
	out := make([]ResolvedAllow, 0, len(si.Spec.Allow))
	for _, a := range si.Spec.Allow {
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
		ports := make([]pce.IngressService, 0, len(a.Ports))
		for _, p := range a.Ports {
			ports = append(ports, pce.IngressService{Proto: protoNumber(p.Protocol), Port: p.Port})
		}
		out = append(out, ResolvedAllow{ConsumerHrefs: consumerHrefs, Ports: ports})
	}
	return out, "", "", true, nil
}

// reconcileRuleSet ensures the owned ruleset exists and its rules match desired
// (replace-all). Returns the ruleset href.
func (r *SegmentationIntentReconciler) reconcileRuleSet(ctx context.Context, si *microv1.SegmentationIntent, pclient PolicyClient, owner pce.Owner, providerHrefs []string, resolved []ResolvedAllow) (string, error) {
	existing, err := pclient.FindRuleSetByOwner(ctx, owner)
	if err != nil {
		return "", err
	}
	var rsHref string
	if existing == nil {
		created, cerr := pclient.CreateRuleSet(ctx, BuildRuleSet(si.Namespace, si.Name, providerHrefs, owner))
		if cerr != nil {
			return "", cerr
		}
		rsHref = created.Href
	} else {
		rsHref = existing.Href
		// Remove existing rules (replace-all).
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

func (r *SegmentationIntentReconciler) fail(ctx context.Context, si *microv1.SegmentationIntent, condType, reason, msg string, requeue time.Duration) (ctrl.Result, error) {
	meta.SetStatusCondition(&si.Status.Conditions, metav1.Condition{
		Type: condType, Status: metav1.ConditionFalse, Reason: reason, Message: msg,
	})
	si.Status.ObservedGeneration = si.Generation
	if err := r.Status().Update(ctx, si); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: requeue}, nil
}

func (r *SegmentationIntentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&microv1.SegmentationIntent{}).
		Complete(r)
}
