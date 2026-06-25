/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	microv1 "github.com/microsegment-io/illumio-k8s-utility-operator/api/v1alpha1"
	"github.com/microsegment-io/illumio-k8s-utility-operator/internal/pce"
)

// PCEConnectionReconciler reconciles a PCEConnection object.
type PCEConnectionReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	NewPCEClient ClientFactory
}

// +kubebuilder:rbac:groups=microsegment.io,resources=pceconnections,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=microsegment.io,resources=pceconnections/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *PCEConnectionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var conn microv1.PCEConnection
	if err := r.Get(ctx, req.NamespacedName, &conn); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	cfg, reason, msg, ok := r.loadConfig(ctx, &conn)
	if !ok {
		return r.fail(ctx, &conn, reason, msg)
	}

	factory := r.NewPCEClient
	if factory == nil {
		factory = DefaultClientFactory
	}
	if err := factory(cfg).Ping(ctx); err != nil {
		reason, msg := classifyPingError(err)
		return r.fail(ctx, &conn, reason, msg)
	}

	meta.SetStatusCondition(&conn.Status.Conditions, metav1.Condition{
		Type:               microv1.ConditionConnected,
		Status:             metav1.ConditionTrue,
		Reason:             microv1.ReasonConnected,
		Message:            "PCE reachable and credentials accepted",
		ObservedGeneration: conn.Generation,
	})
	conn.Status.ObservedGeneration = conn.Generation
	return ctrl.Result{}, r.Status().Update(ctx, &conn)
}

func (r *PCEConnectionReconciler) loadConfig(ctx context.Context, conn *microv1.PCEConnection) (pce.Config, string, string, bool) {
	var secret corev1.Secret
	key := types.NamespacedName{
		Name:      conn.Spec.CredentialsSecretRef.Name,
		Namespace: conn.Spec.CredentialsSecretRef.Namespace,
	}
	if err := r.Get(ctx, key, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return pce.Config{}, microv1.ReasonSecretMissing, "credentials secret not found", false
		}
		return pce.Config{}, microv1.ReasonSecretMissing, err.Error(), false
	}
	apiKey := string(secret.Data["api_key"])
	apiSecret := string(secret.Data["api_secret"])
	if apiKey == "" || apiSecret == "" {
		return pce.Config{}, microv1.ReasonSecretMissing, "secret missing api_key or api_secret", false
	}
	return pce.Config{
		PCEURL:    conn.Spec.PCEURL,
		OrgID:     conn.Spec.OrgID,
		APIKey:    apiKey,
		APISecret: apiSecret,
	}, "", "", true
}

func (r *PCEConnectionReconciler) fail(ctx context.Context, conn *microv1.PCEConnection, reason, msg string) (ctrl.Result, error) {
	meta.SetStatusCondition(&conn.Status.Conditions, metav1.Condition{
		Type:               microv1.ConditionConnected,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: conn.Generation,
	})
	conn.Status.ObservedGeneration = conn.Generation
	return ctrl.Result{}, r.Status().Update(ctx, conn)
}

func classifyPingError(err error) (reason, msg string) {
	switch e := err.(type) {
	case *pce.RateLimitError:
		return microv1.ReasonRateLimited, e.Error()
	case *pce.APIError:
		if e.StatusCode == 401 || e.StatusCode == 403 {
			return microv1.ReasonAuthFailed, e.Error()
		}
		return microv1.ReasonPCEUnreachable, e.Error()
	default:
		return microv1.ReasonPCEUnreachable, err.Error()
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *PCEConnectionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&microv1.PCEConnection{}).
		Named("pceconnection").
		Complete(r)
}
