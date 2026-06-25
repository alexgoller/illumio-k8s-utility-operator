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
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

const (
	requeueSecretMissing = time.Minute
	requeueUnreachable   = 30 * time.Second
	requeueHealthy       = 5 * time.Minute
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

	cfg, reason, msg, transientErr, ok := r.loadConfig(ctx, &conn)
	if transientErr != nil {
		// Transient error fetching the Secret (not NotFound) — return the error so
		// controller-runtime applies exponential backoff.
		return ctrl.Result{}, transientErr
	}
	if !ok {
		return r.fail(ctx, &conn, reason, msg, ctrl.Result{RequeueAfter: requeueSecretMissing})
	}

	factory := r.NewPCEClient
	if factory == nil {
		factory = DefaultClientFactory
	}
	if err := factory(cfg).Ping(ctx); err != nil {
		result, reason, msg := classifyPingError(err)
		return r.fail(ctx, &conn, reason, msg, result)
	}

	meta.SetStatusCondition(&conn.Status.Conditions, metav1.Condition{
		Type:               microv1.ConditionConnected,
		Status:             metav1.ConditionTrue,
		Reason:             microv1.ReasonConnected,
		Message:            "PCE reachable and credentials accepted",
		ObservedGeneration: conn.Generation,
	})
	conn.Status.ObservedGeneration = conn.Generation
	return ctrl.Result{RequeueAfter: requeueHealthy}, r.Status().Update(ctx, &conn)
}

// loadConfig reads the credentials Secret and returns the pce.Config.
// It returns (cfg, "", "", nil, true) on success.
// It returns ("", reason, msg, nil, false) for expected failures (secret missing/empty).
// It returns ("", "", "", err, false) for transient errors (non-NotFound API errors).
func (r *PCEConnectionReconciler) loadConfig(ctx context.Context, conn *microv1.PCEConnection) (pce.Config, string, string, error, bool) {
	var secret corev1.Secret
	key := types.NamespacedName{
		Name:      conn.Spec.CredentialsSecretRef.Name,
		Namespace: conn.Spec.CredentialsSecretRef.Namespace,
	}
	if err := r.Get(ctx, key, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return pce.Config{}, microv1.ReasonSecretMissing, "credentials secret not found", nil, false
		}
		// Transient error — propagate so controller-runtime can backoff.
		return pce.Config{}, "", "", err, false
	}
	apiKey := string(secret.Data["api_key"])
	apiSecret := string(secret.Data["api_secret"])
	if apiKey == "" || apiSecret == "" {
		return pce.Config{}, microv1.ReasonSecretMissing, "secret missing api_key or api_secret", nil, false
	}
	return pce.Config{
		PCEURL:    conn.Spec.PCEURL,
		OrgID:     conn.Spec.OrgID,
		APIKey:    apiKey,
		APISecret: apiSecret,
	}, "", "", nil, true
}

func (r *PCEConnectionReconciler) fail(ctx context.Context, conn *microv1.PCEConnection, reason, msg string, result ctrl.Result) (ctrl.Result, error) {
	meta.SetStatusCondition(&conn.Status.Conditions, metav1.Condition{
		Type:               microv1.ConditionConnected,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: conn.Generation,
	})
	conn.Status.ObservedGeneration = conn.Generation
	return result, r.Status().Update(ctx, conn)
}

func classifyPingError(err error) (ctrl.Result, string, string) {
	switch e := err.(type) {
	case *pce.RateLimitError:
		return ctrl.Result{RequeueAfter: e.RetryAfter}, microv1.ReasonRateLimited, e.Error()
	case *pce.APIError:
		if e.StatusCode == 401 || e.StatusCode == 403 {
			// Bad credentials — no point retrying automatically.
			return ctrl.Result{}, microv1.ReasonAuthFailed, e.Error()
		}
		return ctrl.Result{RequeueAfter: requeueUnreachable}, microv1.ReasonPCEUnreachable, e.Error()
	default:
		return ctrl.Result{RequeueAfter: requeueUnreachable}, microv1.ReasonPCEUnreachable, err.Error()
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *PCEConnectionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&microv1.PCEConnection{}).
		Named("pceconnection").
		Complete(r)
}
