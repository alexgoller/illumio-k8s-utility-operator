package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
)

const (
	cwpTestNamespace  = "payments-cwp"
	cwpTestPCEConn    = "pce-cwp"
	cwpTestCredSecret = "pce-creds-cwp"
	cwpTestLabelEnv   = "env"
)

var _ = Describe("ClusterProfile CWP reconcile", func() {
	const ns = "default"

	It("marks a matched namespace's CWP managed and counts it", func() {
		ctx := context.Background()

		// A namespace the rule will match.
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: cwpTestNamespace},
		})).To(Succeed())

		// Ready PCEConnection + secret.
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: cwpTestCredSecret, Namespace: ns},
			Data:       map[string][]byte{keyAPIKey: []byte("k"), keyAPISecret: []byte("s")},
		})).To(Succeed())
		pc := &microv1.PCEConnection{
			ObjectMeta: metav1.ObjectMeta{Name: cwpTestPCEConn},
			Spec: microv1.PCEConnectionSpec{
				PCEURL: testPCEURL, OrgID: 1,
				CredentialsSecretRef: microv1.SecretReference{Name: cwpTestCredSecret, Namespace: ns},
			},
		}
		Expect(k8sClient.Create(ctx, pc)).To(Succeed())
		Eventually(func(g Gomega) {
			got := &microv1.PCEConnection{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cwpTestPCEConn}, got)).To(Succeed())
			meta.SetStatusCondition(&got.Status.Conditions, metav1.Condition{
				Type: microv1.ConditionConnected, Status: metav1.ConditionTrue, Reason: microv1.ReasonConnected, Message: "t",
			})
			g.Expect(k8sClient.Status().Update(ctx, got)).To(Succeed())
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

		cp := &microv1.ClusterProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "cp-cwp"},
			Spec: microv1.ClusterProfileSpec{
				PCEConnectionRef: microv1.LocalObjectReference{Name: cwpTestPCEConn},
				Onboarding: microv1.OnboardingSpec{
					ContainerClusterName: "ocp-cwp", CredentialsOutputSecret: "creds-cwp-out",
				},
				NamespaceRules: []microv1.NamespaceRule{
					{Match: microv1.NamespaceMatch{NamePattern: cwpTestNamespace}, Managed: true,
						AssignLabels: map[string]microv1.LabelAssignment{cwpTestLabelEnv: {Value: testLabelValueProd}}, EnforcementMode: testEnforcementVisOnly},
				},
			},
		}
		Expect(k8sClient.Create(ctx, cp)).To(Succeed())

		// The fake onboarding client (suite_test.go) returns a CWP for namespace
		// "payments-cwp"; assert the reconcile counts it as managed.
		Eventually(func(g Gomega) {
			got := &microv1.ClusterProfile{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cp-cwp"}, got)).To(Succeed())
			g.Expect(meta.FindStatusCondition(got.Status.Conditions, microv1.ConditionOnboarded)).NotTo(BeNil())
			g.Expect(got.Status.ManagedNamespaces).To(BeNumerically(">=", 1))
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())
	})
})
