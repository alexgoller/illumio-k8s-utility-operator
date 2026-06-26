package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
)

const segIntentFinalizer = "microsegment.io/segmentationintent"

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
			if err := FinalizePolicy(ctx, r.Client, r.NewPolicyClient, si.Namespace, si.UID); err != nil {
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

	allows := CompileIntent(si.Spec.Allow)

	res, err := ReconcilePolicy(ctx, r.Client, r.NewPolicyClient, si.Namespace, si.Name, si.UID, si.Annotations, allows)
	if err != nil {
		return ctrl.Result{}, err
	}

	applyBackendResult(&si.Status.Conditions, res)
	si.Status.WorkloadsAffected = res.WorkloadsAffected
	si.Status.EffectiveEnforcement = res.EffectiveEnforcement
	si.Status.EnforcementSetBy = res.EnforcementSetBy
	si.Status.ObservedGeneration = si.Generation
	if err := r.Status().Update(ctx, &si); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: res.Requeue}, nil
}

func (r *SegmentationIntentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&microv1.SegmentationIntent{}).
		Complete(r)
}
