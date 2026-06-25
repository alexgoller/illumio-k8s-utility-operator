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
	obPCEConnectionName = "pce-ob"
	obCredSecretName    = "pce-creds-ob"
	testNodeLabelRole   = "role"
)

var _ = Describe("ClusterProfile onboarding controller", func() {
	const ns = "default"

	It("onboards: creates the cluster, writes the creds Secret, sets Onboarded=True", func() {
		ctx := context.Background()

		// A ready PCEConnection + its credentials secret.
		credSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: obCredSecretName, Namespace: ns},
			Data:       map[string][]byte{keyAPIKey: []byte("k"), keyAPISecret: []byte("s")},
		}
		Expect(k8sClient.Create(ctx, credSecret)).To(Succeed())

		pc := &microv1.PCEConnection{
			ObjectMeta: metav1.ObjectMeta{Name: obPCEConnectionName},
			Spec: microv1.PCEConnectionSpec{
				PCEURL: testPCEURL, OrgID: 1,
				CredentialsSecretRef: microv1.SecretReference{Name: obCredSecretName, Namespace: ns},
			},
		}
		Expect(k8sClient.Create(ctx, pc)).To(Succeed())
		// Force its Connected condition True (the onboarding controller checks it).
		Eventually(func(g Gomega) {
			got := &microv1.PCEConnection{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: obPCEConnectionName}, got)).To(Succeed())
			meta.SetStatusCondition(&got.Status.Conditions, metav1.Condition{
				Type: microv1.ConditionConnected, Status: metav1.ConditionTrue,
				Reason: microv1.ReasonConnected, Message: "test",
			})
			g.Expect(k8sClient.Status().Update(ctx, got)).To(Succeed())
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

		cp := &microv1.ClusterProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "cp-ob"},
			Spec: microv1.ClusterProfileSpec{
				PCEConnectionRef: microv1.LocalObjectReference{Name: "pce-ob"},
				Onboarding: microv1.OnboardingSpec{
					ContainerClusterName:    "ocp-test",
					CredentialsOutputSecret: "cluster-creds-out",
					NodePairingProfile: microv1.NodePairingProfileSpec{
						Labels:          map[string]string{testNodeLabelRole: "node"},
						EnforcementMode: enforcementIdle,
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, cp)).To(Succeed())

		Eventually(func(g Gomega) {
			got := &microv1.ClusterProfile{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cp-ob"}, got)).To(Succeed())
			c := meta.FindStatusCondition(got.Status.Conditions, microv1.ConditionOnboarded)
			g.Expect(c).NotTo(BeNil())
			g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(got.Status.ContainerClusterID).To(Equal("uuid-ob"))
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())

		// The output Secret carries all four agent credential keys.
		Eventually(func(g Gomega) {
			s := &corev1.Secret{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cluster-creds-out", Namespace: operatorNamespaceForTest}, s)).To(Succeed())
			g.Expect(s.Data).To(HaveKey("pce_url"))
			g.Expect(string(s.Data["cluster_id"])).To(Equal("uuid-ob"))
			g.Expect(string(s.Data["cluster_token"])).To(Equal("tok-ob"))
			g.Expect(string(s.Data["cluster_code"])).To(Equal("act-ob"))
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())
	})
})
