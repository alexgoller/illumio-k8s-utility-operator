package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

// PolicyInsightReconciler runs an on-request what-if preflight for a namespace.
// It NEVER queries the PCE on a timer — a run happens only on create, spec
// change, or a change to the microsegment.io/refresh annotation.
type PolicyInsightReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	NewInsightClient InsightClientFactory
	// Now, MaxResults, and MaxFindings are injectable for tests.
	Now func() time.Time
	// MaxResults caps the raw flows downloaded per query (default 10000).
	MaxResults int
	// MaxFindings caps the listed findings per direction for etcd object-size
	// safety (default 500). The true counts remain in the summary + *Count fields.
	MaxFindings int
}

// +kubebuilder:rbac:groups=microsegment.io,resources=policyinsights,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=microsegment.io,resources=policyinsights/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=microsegment.io,resources=clusterprofiles,verbs=get;list;watch
// +kubebuilder:rbac:groups=microsegment.io,resources=pceconnections,verbs=get;list;watch

func (r *PolicyInsightReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var pi microv1.PolicyInsight
	if err := r.Get(ctx, req.NamespacedName, &pi); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if r.NewInsightClient == nil {
		r.NewInsightClient = DefaultInsightClientFactory
	}
	if r.Now == nil {
		r.Now = time.Now
	}

	refresh := pi.Annotations[microv1.AnnotationRefresh]

	// On-request gate: if this exact request (generation + refresh token) was
	// already computed, do nothing — no PCE call, no requeue.
	if pi.Status.ObservedGeneration == pi.Generation &&
		pi.Status.ObservedRefresh == refresh &&
		meta.FindStatusCondition(pi.Status.Conditions, microv1.ConditionReady) != nil {
		return ctrl.Result{}, nil
	}

	cp, cfg, _, ready, err := resolveClusterProfile(ctx, r.Client, pi.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !ready {
		return r.fail(ctx, &pi, refresh, microv1.ReasonClusterProfileNotReady,
			"no Onboarded ClusterProfile / connected PCEConnection for this namespace; re-request after it is ready")
	}

	var ns corev1.Namespace
	if err := r.Get(ctx, types.NamespacedName{Name: pi.Namespace}, &ns); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	scope := scopeLabelValues(ns, cp)
	if len(scope) == 0 {
		return r.fail(ctx, &pi, refresh, microv1.ReasonNoScopeLabels,
			"namespace is not managed or has no scope labels (app/env) to preflight")
	}

	pclient := r.NewInsightClient(cfg)
	scopeHrefs := make([]string, 0, len(scope))
	for k, v := range scope {
		lbl, lerr := pclient.FindLabel(ctx, k, v)
		if lerr != nil {
			if errors.Is(lerr, pce.ErrLabelNotFound) {
				return r.fail(ctx, &pi, refresh, microv1.ReasonNoScopeLabels,
					fmt.Sprintf("scope label %s=%s not yet in the PCE", k, v))
			}
			return r.fail(ctx, &pi, refresh, microv1.ReasonQueryFailed, "resolve scope label: "+lerr.Error())
		}
		scopeHrefs = append(scopeHrefs, lbl.Href)
	}

	lookback := pi.Spec.LookbackDays
	if lookback <= 0 {
		lookback = 7
	}
	to := r.Now().UTC()
	from := to.AddDate(0, 0, -lookback)
	maxResults := r.MaxResults
	if maxResults <= 0 {
		maxResults = 10000
	}

	// Inbound = scope on destination; outbound = scope on source.
	inFlows, inTrunc, err := pclient.QueryTraffic(ctx, pce.TrafficQuery{
		QueryName: "preflight-inbound-" + pi.Namespace, DestinationLabelHrefs: scopeHrefs,
		From: from, To: to, MaxResults: maxResults,
	})
	if err != nil {
		return r.fail(ctx, &pi, refresh, microv1.ReasonQueryFailed, "inbound traffic query: "+err.Error())
	}
	outFlows, outTrunc, err := pclient.QueryTraffic(ctx, pce.TrafficQuery{
		QueryName: "preflight-outbound-" + pi.Namespace, SourceLabelHrefs: scopeHrefs,
		From: from, To: to, MaxResults: maxResults,
	})
	if err != nil {
		return r.fail(ctx, &pi, refresh, microv1.ReasonQueryFailed, "outbound traffic query: "+err.Error())
	}

	inboundFull := classifyFlows(inFlows, directionInbound)
	outboundFull := classifyFlows(outFlows, directionOutbound)
	inSummary := summarizeFlows(inFlows)
	outSummary := summarizeFlows(outFlows)

	// Cap the listed findings so a pathological namespace (a gateway talking to
	// thousands of distinct peers) can't produce a status object that exceeds the
	// etcd/apiserver ~1.5MB limit. The *Count fields and summary keep the true totals.
	maxFindings := r.MaxFindings
	if maxFindings <= 0 {
		maxFindings = 500
	}
	inbound, inFindTrunc := capFindings(inboundFull, maxFindings)
	outbound, outFindTrunc := capFindings(outboundFull, maxFindings)

	fromT, toT := metav1.NewTime(from), metav1.NewTime(to)
	pi.Status.ObservedWindow = &microv1.ObservationWindow{From: &fromT, To: &toT}
	pi.Status.Summary = &microv1.PreflightSummary{Inbound: inSummary, Outbound: outSummary}
	pi.Status.FlowsAnalyzed = len(inFlows) + len(outFlows)
	pi.Status.Truncated = inTrunc || outTrunc
	pi.Status.WouldBlockInbound = inbound
	pi.Status.WouldBlockInboundTruncated = inFindTrunc
	pi.Status.WouldBlockOutbound = outbound
	pi.Status.WouldBlockOutboundTruncated = outFindTrunc
	pi.Status.InboundBlockedCount = len(inboundFull)
	pi.Status.OutboundBlockedCount = len(outboundFull)
	meta.SetStatusCondition(&pi.Status.Conditions, metav1.Condition{
		Type: microv1.ConditionReady, Status: metav1.ConditionTrue, Reason: microv1.ReasonComputed,
		Message: fmt.Sprintf("inbound: %d allowed / %d potentially-blocked / %d blocked; outbound: %d allowed / %d potentially-blocked / %d blocked",
			inSummary.Allowed, inSummary.PotentiallyBlocked, inSummary.Blocked,
			outSummary.Allowed, outSummary.PotentiallyBlocked, outSummary.Blocked),
	})
	pi.Status.ObservedGeneration = pi.Generation
	pi.Status.ObservedRefresh = refresh
	// No RequeueAfter — preflight is on-request only, never periodic.
	if err := r.Status().Update(ctx, &pi); err != nil {
		// Do NOT return the error: an error requeues, which would re-run the
		// (expensive) PCE traffic query. Log and stop; the user re-requests
		// (refresh) or a later change re-triggers. Findings are already capped,
		// so an oversized-object write failure should not happen here.
		log.FromContext(ctx).Error(err, "failed to write PolicyInsight status; not re-querying the PCE",
			"namespace", pi.Namespace, "name", pi.Name)
		return ctrl.Result{}, nil
	}
	return ctrl.Result{}, nil
}

// fail records a Ready=False condition and the request token so the reconcile
// does not spin; the user re-requests (refresh) to try again. No requeue.
func (r *PolicyInsightReconciler) fail(ctx context.Context, pi *microv1.PolicyInsight, refresh, reason, msg string) (ctrl.Result, error) {
	meta.SetStatusCondition(&pi.Status.Conditions, metav1.Condition{
		Type: microv1.ConditionReady, Status: metav1.ConditionFalse, Reason: reason, Message: msg,
	})
	pi.Status.ObservedGeneration = pi.Generation
	pi.Status.ObservedRefresh = refresh
	return ctrl.Result{}, r.Status().Update(ctx, pi)
}

func (r *PolicyInsightReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&microv1.PolicyInsight{}).
		Complete(r)
}
