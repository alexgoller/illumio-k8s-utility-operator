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

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

// RuleViewReconciler mirrors the current Illumio rules that protect a namespace's
// app (provider side) into a read-only RuleView, re-syncing on a periodic cadence
// and on a microsegment.io/refresh annotation change. It never authors rules.
type RuleViewReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	NewRuleViewClient RuleViewClientFactory
	// Now and MaxRules are injectable for tests.
	Now      func() time.Time
	MaxRules int
}

// +kubebuilder:rbac:groups=microsegment.io,resources=ruleviews,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=microsegment.io,resources=ruleviews/status,verbs=get;update;patch

func (r *RuleViewReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var rv microv1.RuleView
	if err := r.Get(ctx, req.NamespacedName, &rv); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if r.NewRuleViewClient == nil {
		r.NewRuleViewClient = DefaultRuleViewClientFactory
	}
	if r.Now == nil {
		r.Now = time.Now
	}

	interval := time.Duration(rv.Spec.RefreshIntervalMinutes) * time.Minute
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	refresh := rv.Annotations[microv1.AnnotationRefresh]

	// Periodic time-gate: if we synced this generation+refresh less than an interval
	// ago, requeue for the remainder without re-querying (also absorbs the reconcile
	// triggered by our own status write).
	if rv.Status.ObservedGeneration == rv.Generation &&
		rv.Status.ObservedRefresh == refresh &&
		rv.Status.ObservedAt != nil &&
		meta.FindStatusCondition(rv.Status.Conditions, microv1.ConditionReady) != nil {
		if elapsed := r.Now().Sub(rv.Status.ObservedAt.Time); elapsed < interval {
			return ctrl.Result{RequeueAfter: interval - elapsed}, nil
		}
	}

	cp, cfg, eds, ready, err := resolveClusterProfile(ctx, r.Client, rv.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !ready {
		return r.fail(ctx, &rv, refresh, interval, microv1.ReasonClusterProfileNotReady,
			"no Onboarded ClusterProfile / connected PCEConnection for this namespace")
	}

	var ns corev1.Namespace
	if err := r.Get(ctx, types.NamespacedName{Name: rv.Namespace}, &ns); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	scope := scopeLabelValues(ns, cp)
	if len(scope) == 0 {
		return r.fail(ctx, &rv, refresh, interval, microv1.ReasonNoScopeLabels,
			"namespace is not managed or has no scope labels (app/env)")
	}

	pclient := r.NewRuleViewClient(cfg)
	scopeHrefs := make([]string, 0, len(scope))
	for k, v := range scope {
		lbl, lerr := pclient.FindLabel(ctx, k, v)
		if lerr != nil {
			if errors.Is(lerr, pce.ErrLabelNotFound) {
				return r.fail(ctx, &rv, refresh, interval, microv1.ReasonNoScopeLabels,
					fmt.Sprintf("scope label %s=%s not yet in the PCE", k, v))
			}
			return r.fail(ctx, &rv, refresh, interval, microv1.ReasonQueryFailed, "resolve scope label: "+lerr.Error())
		}
		scopeHrefs = append(scopeHrefs, lbl.Href)
	}

	found, err := pclient.SearchRules(ctx, pce.RuleSearchQuery{ProviderLabelHrefs: scopeHrefs, PolicyVersion: rv.Spec.PolicyVersion})
	if err != nil {
		return r.fail(ctx, &rv, refresh, interval, microv1.ReasonQueryFailed, "rule search: "+err.Error())
	}

	maxRules := r.MaxRules
	if maxRules <= 0 {
		maxRules = 200
	}
	rules, owned, external, trunc := mapRules(found, eds, maxRules)

	now := metav1.NewTime(r.Now())
	rv.Status.ObservedAt = &now
	rv.Status.RuleCount = len(found)
	rv.Status.OwnedCount = owned
	rv.Status.ExternalCount = external
	rv.Status.Truncated = trunc
	rv.Status.Rules = rules
	meta.SetStatusCondition(&rv.Status.Conditions, metav1.Condition{
		Type: microv1.ConditionReady, Status: metav1.ConditionTrue, Reason: microv1.ReasonSynced,
		Message: fmt.Sprintf("%d rules protect this app (%d owned, %d external)", len(found), owned, external),
	})
	rv.Status.ObservedGeneration = rv.Generation
	rv.Status.ObservedRefresh = refresh
	if err := r.Status().Update(ctx, &rv); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: interval}, nil
}

// fail records a Ready=False condition and stamps observedAt so the periodic gate
// applies (retry on the next interval, not a tight loop). The user can bump the
// refresh annotation to retry sooner.
func (r *RuleViewReconciler) fail(ctx context.Context, rv *microv1.RuleView, refresh string, interval time.Duration, reason, msg string) (ctrl.Result, error) {
	now := metav1.NewTime(r.Now())
	rv.Status.ObservedAt = &now
	meta.SetStatusCondition(&rv.Status.Conditions, metav1.Condition{
		Type: microv1.ConditionReady, Status: metav1.ConditionFalse, Reason: reason, Message: msg,
	})
	rv.Status.ObservedGeneration = rv.Generation
	rv.Status.ObservedRefresh = refresh
	if err := r.Status().Update(ctx, rv); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: interval}, nil
}

func (r *RuleViewReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&microv1.RuleView{}).
		Complete(r)
}
