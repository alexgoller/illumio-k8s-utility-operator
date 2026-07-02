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
	testInsightNS        = "insight-app"
	testPCEConnPI        = "pce-pi"
	testLabelValueLedger = "ledger"
)

var _ = Describe("PolicyInsight preflight", func() {
	const opNS = "default"

	It("computes would-block-inbound and blocked-egress findings on request", func() {
		ctx := context.Background()

		// PCEConnection (Connected) + secret.
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "pce-creds-pi", Namespace: opNS},
			Data:       map[string][]byte{keyAPIKey: []byte("k"), keyAPISecret: []byte("s")},
		})).To(Succeed())
		pc := &microv1.PCEConnection{
			ObjectMeta: metav1.ObjectMeta{Name: testPCEConnPI},
			Spec:       microv1.PCEConnectionSpec{PCEURL: testPCEURL, OrgID: 1, CredentialsSecretRef: microv1.SecretReference{Name: "pce-creds-pi", Namespace: opNS}},
		}
		Expect(k8sClient.Create(ctx, pc)).To(Succeed())
		Eventually(func(g Gomega) {
			got := &microv1.PCEConnection{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testPCEConnPI}, got)).To(Succeed())
			meta.SetStatusCondition(&got.Status.Conditions, metav1.Condition{Type: microv1.ConditionConnected, Status: metav1.ConditionTrue, Reason: microv1.ReasonConnected, Message: "t"})
			g.Expect(k8sClient.Status().Update(ctx, got)).To(Succeed())
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

		// ClusterProfile that manages insight-* namespaces with app+env scope labels.
		cp := &microv1.ClusterProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "cp-pi"},
			Spec: microv1.ClusterProfileSpec{
				PCEConnectionRef: microv1.LocalObjectReference{Name: testPCEConnPI},
				Onboarding:       microv1.OnboardingSpec{ContainerClusterName: "c-pi", CredentialsOutputSecret: "creds-pi-out"},
				NamespaceRules: []microv1.NamespaceRule{
					{Match: microv1.NamespaceMatch{NamePattern: "insight-*"}, Managed: true,
						AssignLabels: map[string]microv1.LabelAssignment{
							testLabelKeyApp: {Value: "myapp"},
							testLabelKeyEnv: {Value: testLabelValueProd},
						}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, cp)).To(Succeed())
		Eventually(func(g Gomega) {
			got := &microv1.ClusterProfile{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cp-pi"}, got)).To(Succeed())
			meta.SetStatusCondition(&got.Status.Conditions, metav1.Condition{Type: microv1.ConditionOnboarded, Status: metav1.ConditionTrue, Reason: "Onboarded", Message: "t"})
			g.Expect(k8sClient.Status().Update(ctx, got)).To(Succeed())
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

		// The managed namespace + a PolicyInsight requesting a preflight.
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testInsightNS}})).To(Succeed())
		Expect(k8sClient.Create(ctx, &microv1.PolicyInsight{
			ObjectMeta: metav1.ObjectMeta{Name: "preflight", Namespace: testInsightNS},
			Spec:       microv1.PolicyInsightSpec{LookbackDays: 7},
		})).To(Succeed())

		Eventually(func(g Gomega) {
			got := &microv1.PolicyInsight{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "preflight", Namespace: testInsightNS}, got)).To(Succeed())
			c := meta.FindStatusCondition(got.Status.Conditions, microv1.ConditionReady)
			g.Expect(c).NotTo(BeNil())
			g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(c.Reason).To(Equal(microv1.ReasonComputed))
			g.Expect(got.Status.InboundBlockedCount).To(Equal(1))
			g.Expect(got.Status.EgressBlockedCount).To(Equal(1))
			g.Expect(got.Status.WouldBlockInbound[0].Peer[testLabelKeyApp]).To(Equal(testLabelValueCheckout))
			g.Expect(got.Status.WouldBlockInbound[0].Port).To(Equal(8443))
			g.Expect(got.Status.BlockedEgress[0].Port).To(Equal(5432))
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())
	})
})
