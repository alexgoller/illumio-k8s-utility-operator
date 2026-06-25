package controller

import (
	"context"
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
	onboardRequeueNotReady = 30 * time.Second
	onboardRequeueHealthy  = 10 * time.Minute
)

// ClusterProfileReconciler reconciles a ClusterProfile (onboarding).
type ClusterProfileReconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	OperatorNamespace   string
	NewOnboardingClient OnboardingClientFactory
}

// +kubebuilder:rbac:groups=microsegment.io,resources=clusterprofiles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=microsegment.io,resources=clusterprofiles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=microsegment.io,resources=pceconnections,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch

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
			"referenced PCEConnection is missing or not Connected", onboardRequeueNotReady)
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
				onboardRequeueHealthy)
		}
		created, err := pclient.CreateContainerCluster(ctx, cp.Spec.Onboarding.ContainerClusterName, "Managed by illumio-k8s-utility-operator", owner)
		if err != nil {
			return ctrl.Result{}, err
		}
		cp.Status.ContainerClusterHref = created.Href
		cp.Status.ContainerClusterID = pce.ContainerClusterUUID(created.Href)
		token = created.ContainerClusterToken
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
				mode = "idle"
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

	// Publish the output Secret (only set cluster_token when freshly created).
	if err := r.writeCredentialsSecret(ctx, &cp, cfg.PCEURL, token, code); err != nil {
		return ctrl.Result{}, err
	}

	meta.SetStatusCondition(&cp.Status.Conditions, metav1.Condition{
		Type: microv1.ConditionOnboarded, Status: metav1.ConditionTrue,
		Reason: microv1.ReasonOnboarded, Message: "cluster onboarded; credentials published",
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
		eds = "illumio-operator"
	}
	return pce.Config{PCEURL: conn.Spec.PCEURL, OrgID: conn.Spec.OrgID, APIKey: apiKey, APISecret: apiSecret}, eds, true, nil
}

// writeCredentialsSecret creates/updates the output Secret. cluster_token is
// only written when token != "" (it is recoverable only at cluster creation).
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
		secret.Data["cluster_code"] = []byte(code)
		if token != "" {
			secret.Data["cluster_token"] = []byte(token)
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

func (r *ClusterProfileReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&microv1.ClusterProfile{}).
		Complete(r)
}
