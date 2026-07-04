package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	allows, cerr := CompileIntent(si.Spec)
	if cerr != nil {
		meta.SetStatusCondition(&si.Status.Conditions, metav1.Condition{
			Type:    microv1.ConditionReady,
			Status:  metav1.ConditionFalse,
			Reason:  microv1.ReasonRejected,
			Message: cerr.Error(),
		})
		si.Status.ObservedGeneration = si.Generation
		return ctrl.Result{}, r.Status().Update(ctx, &si)
	}

	res, err := ReconcilePolicy(ctx, r.Client, r.NewPolicyClient, si.Namespace, si.Name, si.UID, si.Annotations, si.Spec.Provider, allows)
	if err != nil {
		if d, ok := pceRateLimit(err); ok {
			meta.SetStatusCondition(&si.Status.Conditions, metav1.Condition{
				Type: microv1.ConditionReady, Status: metav1.ConditionFalse, Reason: microv1.ReasonRateLimited, Message: err.Error()})
			si.Status.ObservedGeneration = si.Generation
			if uerr := r.Status().Update(ctx, &si); uerr != nil {
				return ctrl.Result{}, uerr
			}
			return ctrl.Result{RequeueAfter: d}, nil
		}
		return ctrl.Result{}, err
	}

	applyBackendResult(&si.Status.Conditions, res)
	si.Status.WorkloadsAffected = res.WorkloadsAffected
	si.Status.EffectiveEnforcement = res.EffectiveEnforcement
	si.Status.EnforcementSetBy = res.EnforcementSetBy
	si.Status.UnknownLabelMode = res.UnknownLabelMode
	si.Status.UnknownLabelModeSetBy = res.UnknownLabelModeSetBy
	si.Status.DeferredLabels = res.DeferredLabels
	si.Status.CreatedLabels = res.CreatedLabels
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
