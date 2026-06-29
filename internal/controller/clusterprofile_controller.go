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
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

const (
	onboardRequeueNotReady = 30 * time.Second
	onboardRequeueHealthy  = 10 * time.Minute
	onboardRequeueTerminal = time.Hour
)

// ClusterProfileReconciler reconciles a ClusterProfile (onboarding).
type ClusterProfileReconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	OperatorNamespace   string
	NewOnboardingClient OnboardingClientFactory
	Recorder            events.EventRecorder
}

// +kubebuilder:rbac:groups=microsegment.io,resources=clusterprofiles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=microsegment.io,resources=clusterprofiles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=microsegment.io,resources=pceconnections,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=microsegment.io,resources=segmentationintents,verbs=get;list;watch
// +kubebuilder:rbac:groups=microsegment.io,resources=segmentationpolicies,verbs=get;list;watch

func (r *ClusterProfileReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var cp microv1.ClusterProfile
	if err := r.Get(ctx, req.NamespacedName, &cp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if r.NewOnboardingClient == nil {
		r.NewOnboardingClient = DefaultOnboardingClientFactory
	}

	cfg, externalDataSet, ready, transientErr := r.resolveConnection(ctx, &cp)
	if transientErr != nil {
		return ctrl.Result{}, transientErr
	}
	if !ready {
		return r.onboardFail(ctx, &cp, microv1.ReasonPCEConnectionNotReady,
			"referenced PCEConnection is missing, not Connected, or its credentials Secret is unavailable", onboardRequeueNotReady)
	}

	pclient := r.NewOnboardingClient(cfg)
	owner := pce.Owner{DataSet: externalDataSet, Reference: string(cp.UID)}

	// Ensure the container cluster (capture the one-time token only on create).
	var token string
	if cp.Status.ContainerClusterHref == "" {
		existing, err := pclient.FindContainerClusterByName(ctx, cp.Spec.Onboarding.ContainerClusterName)
		if err != nil {
			return ctrl.Result{}, err
		}
		if existing != nil {
			return r.onboardFail(ctx, &cp, microv1.ReasonOnboardFailed,
				fmt.Sprintf("container cluster %q already exists in the PCE; its one-time token cannot be recovered. Delete it or supply credentials manually.", cp.Spec.Onboarding.ContainerClusterName),
				onboardRequeueTerminal)
		}
		created, err := pclient.CreateContainerCluster(ctx, cp.Spec.Onboarding.ContainerClusterName, "Managed by illumio-k8s-utility-operator")
		if err != nil {
			return r.onboardError(ctx, &cp,
				fmt.Sprintf("failed to create PCE container cluster %q: %v", cp.Spec.Onboarding.ContainerClusterName, err), err)
		}
		cp.Status.ContainerClusterHref = created.Href
		cp.Status.ContainerClusterID = pce.ContainerClusterUUID(created.Href)
		token = created.ContainerClusterToken

		// Persist href/ID and the one-time token immediately so that a crash
		// during the subsequent pairing-profile steps leaves the next reconcile
		// able to skip the create branch and find the token already in the Secret.
		// A crash between CreateContainerCluster returning and these two writes is
		// a sub-second unavoidable window; it results in the same dead-end, but
		// that window is orders of magnitude smaller than the entire reconcile loop.
		if err := r.writeCredentialsSecret(ctx, &cp, cfg.PCEURL, token, ""); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Status().Update(ctx, &cp); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Ensure the node pairing profile (cluster_code source).
	npp := cp.Spec.Onboarding.NodePairingProfile
	var pp *pce.PairingProfile
	var err error
	if npp.ExistingName != "" {
		pp, err = pclient.FindPairingProfileByName(ctx, npp.ExistingName)
		if err != nil {
			return ctrl.Result{}, err
		}
		if pp == nil {
			return r.onboardFail(ctx, &cp, microv1.ReasonOnboardFailed,
				fmt.Sprintf("pairing profile %q not found in the PCE", npp.ExistingName), onboardRequeueHealthy)
		}
	} else {
		ppName := cp.Spec.Onboarding.ContainerClusterName + "-nodes"
		pp, err = pclient.FindPairingProfileByName(ctx, ppName)
		if err != nil {
			return ctrl.Result{}, err
		}
		if pp == nil {
			// Resolve the requested node labels to Illumio label hrefs.
			labels := make([]pce.LabelRef, 0, len(npp.Labels))
			for key, value := range npp.Labels {
				lbl, lerr := pclient.EnsureLabel(ctx, key, value, owner)
				if lerr != nil {
					return ctrl.Result{}, lerr
				}
				labels = append(labels, pce.LabelRef{Href: lbl.Href})
			}
			mode := npp.EnforcementMode
			if mode == "" {
				mode = enforcementIdle
			}
			pp, err = pclient.CreatePairingProfile(ctx, pce.PairingProfile{
				Name: ppName, Enabled: true, EnforcementMode: mode,
				AllowedUsesPerKey: "unlimited", KeyLifespan: "unlimited",
				Labels:                labels,
				ExternalDataSet:       owner.DataSet,
				ExternalDataReference: owner.Reference,
			})
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}
	code, err := pclient.GeneratePairingKey(ctx, pp.Href)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Publish cluster_code into the output Secret. Pass an empty token so
	// writeCredentialsSecret preserves the token written in the early persist step
	// (or a no-op on reconcile paths where the cluster already existed).
	if err := r.writeCredentialsSecret(ctx, &cp, cfg.PCEURL, "", code); err != nil {
		return ctrl.Result{}, err
	}

	// Reconcile per-namespace CWPs now that the cluster is onboarded. A single
	// namespace's PCE failure must not freeze the whole status, so persist what we
	// managed regardless and only advance observedGeneration on a clean pass.
	managed, cwpErr := r.reconcileNamespaceCWPs(ctx, &cp, pclient, owner)
	cp.Status.ManagedNamespaces = managed
	meta.SetStatusCondition(&cp.Status.Conditions, metav1.Condition{
		Type: microv1.ConditionOnboarded, Status: metav1.ConditionTrue,
		Reason: microv1.ReasonOnboarded, Message: "cluster onboarded; credentials published",
	})

	if cwpErr != nil {
		// Partial failure: record it and keep observedGeneration behind so the
		// spec is retried, but still persist the namespaces we did manage.
		meta.SetStatusCondition(&cp.Status.Conditions, metav1.Condition{
			Type: microv1.ConditionNamespacesReconciled, Status: metav1.ConditionFalse,
			Reason: microv1.ReasonNamespaceErrors, Message: cwpErr.Error(),
		})
		if err := r.Status().Update(ctx, &cp); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, cwpErr
	}

	meta.SetStatusCondition(&cp.Status.Conditions, metav1.Condition{
		Type: microv1.ConditionNamespacesReconciled, Status: metav1.ConditionTrue,
		Reason: microv1.ReasonReconciled, Message: fmt.Sprintf("%d namespace(s) managed", managed),
	})
	cp.Status.ObservedGeneration = cp.Generation
	if err := r.Status().Update(ctx, &cp); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: onboardRequeueHealthy}, nil
}

// resolveConnection finds the PCEConnection, checks its Connected condition,
// and loads credentials. Returns (cfg, externalDataSet, ready, transientErr).
func (r *ClusterProfileReconciler) resolveConnection(ctx context.Context, cp *microv1.ClusterProfile) (pce.Config, string, bool, error) {
	var conn microv1.PCEConnection
	if err := r.Get(ctx, types.NamespacedName{Name: cp.Spec.PCEConnectionRef.Name}, &conn); err != nil {
		if apierrors.IsNotFound(err) {
			return pce.Config{}, "", false, nil
		}
		return pce.Config{}, "", false, err
	}
	if !meta.IsStatusConditionTrue(conn.Status.Conditions, microv1.ConditionConnected) {
		return pce.Config{}, "", false, nil
	}
	var secret corev1.Secret
	key := types.NamespacedName{Name: conn.Spec.CredentialsSecretRef.Name, Namespace: conn.Spec.CredentialsSecretRef.Namespace}
	if err := r.Get(ctx, key, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return pce.Config{}, "", false, nil
		}
		return pce.Config{}, "", false, err
	}
	apiKey, apiSecret := string(secret.Data["api_key"]), string(secret.Data["api_secret"])
	if apiKey == "" || apiSecret == "" {
		return pce.Config{}, "", false, nil
	}
	eds := conn.Spec.ExternalDataSet
	if eds == "" {
		eds = defaultExternalDataSet
	}
	return pce.Config{PCEURL: conn.Spec.PCEURL, OrgID: conn.Spec.OrgID, APIKey: apiKey, APISecret: apiSecret}, eds, true, nil
}

// writeCredentialsSecret creates/updates the output Secret. cluster_token is
// only written when token != "" (it is recoverable only at cluster creation).
// cluster_code is only written when code != "" so an early write (token-only)
// does not blank an existing code written by a later call.
func (r *ClusterProfileReconciler) writeCredentialsSecret(ctx context.Context, cp *microv1.ClusterProfile, pceURL, token, code string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cp.Spec.Onboarding.CredentialsOutputSecret,
			Namespace: r.OperatorNamespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		if secret.Data == nil {
			secret.Data = map[string][]byte{}
		}
		secret.Data["pce_url"] = []byte(pceURL)
		secret.Data["cluster_id"] = []byte(cp.Status.ContainerClusterID)
		if token != "" {
			secret.Data["cluster_token"] = []byte(token)
		}
		if code != "" {
			secret.Data["cluster_code"] = []byte(code)
		}
		return nil
	})
	return err
}

func (r *ClusterProfileReconciler) onboardFail(ctx context.Context, cp *microv1.ClusterProfile, reason, msg string, requeue time.Duration) (ctrl.Result, error) {
	meta.SetStatusCondition(&cp.Status.Conditions, metav1.Condition{
		Type: microv1.ConditionOnboarded, Status: metav1.ConditionFalse, Reason: reason, Message: msg,
	})
	cp.Status.ObservedGeneration = cp.Generation
	if err := r.Status().Update(ctx, cp); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: requeue}, nil
}

// onboardError records a failed PCE call on the Onboarded condition (so the
// failure is visible in `kubectl get clusterprofile` instead of only the logs)
// and returns the underlying error so controller-runtime applies its normal
// backoff-and-retry. Use this for PCE write failures that are expected to clear
// on their own (rate limits, transient 5xx) or that need an operator's
// attention (validation errors).
func (r *ClusterProfileReconciler) onboardError(ctx context.Context, cp *microv1.ClusterProfile, msg string, cause error) (ctrl.Result, error) {
	meta.SetStatusCondition(&cp.Status.Conditions, metav1.Condition{
		Type: microv1.ConditionOnboarded, Status: metav1.ConditionFalse, Reason: microv1.ReasonOnboardFailed, Message: msg,
	})
	cp.Status.ObservedGeneration = cp.Generation
	if err := r.Status().Update(ctx, cp); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, cause
}

func (r *ClusterProfileReconciler) SetupWithManager(mgr ctrl.Manager) error {
	enqueueAllCPs := handler.EnqueueRequestsFromMapFunc(r.clusterProfilesForNamespace)
	return ctrl.NewControllerManagedBy(mgr).
		For(&microv1.ClusterProfile{}).
		Watches(&corev1.Namespace{},
			handler.EnqueueRequestsFromMapFunc(r.clusterProfilesForNamespace),
			builder.WithPredicates(predicate.Or(predicate.LabelChangedPredicate{}, predicate.AnnotationChangedPredicate{})),
		).
		Watches(&microv1.SegmentationIntent{}, enqueueAllCPs,
			builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&microv1.SegmentationPolicy{}, enqueueAllCPs,
			builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}

// clusterProfilesForNamespace enqueues all ClusterProfiles when any namespace
// changes (rules apply cluster-wide).
func (r *ClusterProfileReconciler) clusterProfilesForNamespace(ctx context.Context, _ client.Object) []reconcile.Request {
	var list microv1.ClusterProfileList
	if err := r.List(ctx, &list); err != nil {
		return nil
	}
	reqs := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{Name: list.Items[i].Name}})
	}
	return reqs
}

// reconcileNamespaceCWPs evaluates every namespace against the profile's rules
// and updates each namespace's CWP in the PCE. It returns the managed count and
// the aggregated per-namespace errors (nil = every namespace applied cleanly). A
// single namespace's PCE failure does not abort the loop: the remaining
// namespaces are still reconciled so one bad namespace cannot freeze the rest.
func (r *ClusterProfileReconciler) reconcileNamespaceCWPs(ctx context.Context, cp *microv1.ClusterProfile, pclient OnboardingClient, owner pce.Owner) (int, error) {
	if cp.Status.ContainerClusterID == "" {
		return 0, nil
	}
	var nsList corev1.NamespaceList
	if err := r.List(ctx, &nsList); err != nil {
		return 0, err
	}
	cwps, err := pclient.ListContainerWorkloadProfiles(ctx, cp.Status.ContainerClusterID)
	if err != nil {
		return 0, err
	}
	byNS := make(map[string]pce.ContainerWorkloadProfile, len(cwps))
	for _, c := range cwps {
		if c.Namespace != "" {
			byNS[c.Namespace] = c
		}
	}

	labelHref := map[string]string{} // "key|value" -> href cache
	managed := 0
	var errs []error
	for i := range nsList.Items {
		nsObj := &nsList.Items[i]
		desired := ComputeDesiredCWP(nsObj.Name, nsObj.Labels, nsObj.Annotations, cp.Spec.NamespaceRules, cp.Spec.SystemNamespaces)
		if desired.Managed {
			managed++
			// Raise enforcement to the strictest requested by any policy CR in this namespace.
			if raised, _, eerr := EffectiveEnforcement(ctx, r.Client, nsObj.Name, desired.EnforcementMode); eerr == nil {
				desired.EnforcementMode = raised
			}
		}
		cwp, ok := byNS[nsObj.Name]
		if !ok {
			// Kubelink has not created this namespace's CWP yet; reconcile later.
			continue
		}
		update, changed, lerr := r.buildCWPUpdate(ctx, pclient, owner, cwp, desired, labelHref)
		if lerr != nil {
			errs = append(errs, fmt.Errorf("namespace %s: %w", nsObj.Name, lerr))
			continue
		}
		if !changed {
			continue
		}
		if err := pclient.UpdateContainerWorkloadProfile(ctx, cwp.Href, update); err != nil {
			errs = append(errs, fmt.Errorf("namespace %s: %w", nsObj.Name, err))
			continue
		}
		if r.Recorder != nil {
			r.Recorder.Eventf(nsObj, nil, corev1.EventTypeNormal, "CWPConfigured", "Configure",
				"managed=%v enforcement=%s", desired.Managed, desired.EnforcementMode)
		}
	}
	return managed, errors.Join(errs...)
}

// buildCWPUpdate resolves desired labels to Illumio hrefs and diffs against the
// current CWP. Returns the update body and whether anything changed.
func (r *ClusterProfileReconciler) buildCWPUpdate(ctx context.Context, pclient OnboardingClient, owner pce.Owner, current pce.ContainerWorkloadProfile, desired DesiredCWP, labelHref map[string]string) (pce.CWPUpdate, bool, error) {
	// Resolve desired labels to hrefs (create-if-missing, cached).
	desiredLabels := make([]pce.CWPLabel, 0, len(desired.Labels))
	desiredHrefByKey := map[string]string{}
	for key, value := range desired.Labels {
		cacheKey := key + "|" + value
		href, ok := labelHref[cacheKey]
		if !ok {
			lbl, err := pclient.EnsureLabel(ctx, key, value, owner)
			if err != nil {
				return pce.CWPUpdate{}, false, err
			}
			href = lbl.Href
			labelHref[cacheKey] = href
		}
		desiredHrefByKey[key] = href
		desiredLabels = append(desiredLabels, pce.CWPLabel{Key: key, Assignment: &pce.LabelRef{Href: href}})
	}

	// Diff: managed, enforcement, and the set of (key->href) assignments.
	changed := current.Managed != desired.Managed
	enforcement := desired.EnforcementMode
	if desired.Managed && current.EnforcementMode != enforcement && enforcement != "" {
		changed = true
	}
	currentHrefByKey := map[string]string{}
	for _, l := range current.Labels {
		if l.Assignment != nil {
			currentHrefByKey[l.Key] = l.Assignment.Href
		}
	}
	if !sameLabelSet(currentHrefByKey, desiredHrefByKey) {
		changed = true
	}

	managed := desired.Managed
	update := pce.CWPUpdate{Managed: &managed, Labels: desiredLabels}
	if desired.Managed && enforcement != "" {
		update.EnforcementMode = enforcement
	}
	return update, changed, nil
}

func sameLabelSet(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
