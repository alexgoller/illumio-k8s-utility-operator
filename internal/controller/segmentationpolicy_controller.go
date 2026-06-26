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

const segPolicyFinalizer = "microsegment.io/segmentationpolicy"

// SegmentationPolicyReconciler reconciles a SegmentationPolicy.
type SegmentationPolicyReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	NewPolicyClient PolicyClientFactory
}

// +kubebuilder:rbac:groups=microsegment.io,resources=segmentationpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=microsegment.io,resources=segmentationpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=microsegment.io,resources=segmentationpolicies/finalizers,verbs=update

func (r *SegmentationPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var sp microv1.SegmentationPolicy
	if err := r.Get(ctx, req.NamespacedName, &sp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if r.NewPolicyClient == nil {
		r.NewPolicyClient = DefaultPolicyClientFactory
	}

	if !sp.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&sp, segPolicyFinalizer) {
			if err := FinalizePolicy(ctx, r.Client, r.NewPolicyClient, sp.Namespace, sp.UID); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(&sp, segPolicyFinalizer)
			if err := r.Update(ctx, &sp); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}
	if controllerutil.AddFinalizer(&sp, segPolicyFinalizer) {
		if err := r.Update(ctx, &sp); err != nil {
			return ctrl.Result{}, err
		}
	}

	allows, cerr := CompilePolicy(sp.Spec)
	if cerr != nil {
		meta.SetStatusCondition(&sp.Status.Conditions, metav1.Condition{
			Type:    microv1.ConditionReady,
			Status:  metav1.ConditionFalse,
			Reason:  microv1.ReasonRejected,
			Message: cerr.Error(),
		})
		sp.Status.ObservedGeneration = sp.Generation
		return ctrl.Result{}, r.Status().Update(ctx, &sp)
	}

	res, err := ReconcilePolicy(ctx, r.Client, r.NewPolicyClient, sp.Namespace, sp.Name, sp.UID, sp.Annotations, allows)
	if err != nil {
		return ctrl.Result{}, err
	}
	applyBackendResult(&sp.Status.Conditions, res)
	sp.Status.WorkloadsAffected = res.WorkloadsAffected
	sp.Status.ObservedGeneration = sp.Generation
	if err := r.Status().Update(ctx, &sp); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: res.Requeue}, nil
}

func (r *SegmentationPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).For(&microv1.SegmentationPolicy{}).Complete(r)
}
