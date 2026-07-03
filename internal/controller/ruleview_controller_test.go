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
	testRuleViewNS = "rview-app"
	testPCEConnRV  = "pce-rv"
)

var _ = Describe("RuleView sync", func() {
	const opNS = "default"

	It("mirrors current rules and flags owned vs external", func() {
		ctx := context.Background()

		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "pce-creds-rv", Namespace: opNS},
			Data:       map[string][]byte{keyAPIKey: []byte("k"), keyAPISecret: []byte("s")},
		})).To(Succeed())
		Expect(k8sClient.Create(ctx, &microv1.PCEConnection{
			ObjectMeta: metav1.ObjectMeta{Name: testPCEConnRV},
			Spec:       microv1.PCEConnectionSpec{PCEURL: testPCEURL, OrgID: 1, CredentialsSecretRef: microv1.SecretReference{Name: "pce-creds-rv", Namespace: opNS}},
		})).To(Succeed())
		Eventually(func(g Gomega) {
			got := &microv1.PCEConnection{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testPCEConnRV}, got)).To(Succeed())
			meta.SetStatusCondition(&got.Status.Conditions, metav1.Condition{Type: microv1.ConditionConnected, Status: metav1.ConditionTrue, Reason: microv1.ReasonConnected, Message: "t"})
			g.Expect(k8sClient.Status().Update(ctx, got)).To(Succeed())
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

		cp := &microv1.ClusterProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "cp-rv"},
			Spec: microv1.ClusterProfileSpec{
				PCEConnectionRef: microv1.LocalObjectReference{Name: testPCEConnRV},
				Onboarding:       microv1.OnboardingSpec{ContainerClusterName: "c-rv", CredentialsOutputSecret: "creds-rv-out"},
				NamespaceRules: []microv1.NamespaceRule{
					{Match: microv1.NamespaceMatch{NamePattern: "rview-*"}, Managed: true,
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
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cp-rv"}, got)).To(Succeed())
			meta.SetStatusCondition(&got.Status.Conditions, metav1.Condition{Type: microv1.ConditionOnboarded, Status: metav1.ConditionTrue, Reason: "Onboarded", Message: "t"})
			g.Expect(k8sClient.Status().Update(ctx, got)).To(Succeed())
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testRuleViewNS}})).To(Succeed())
		Expect(k8sClient.Create(ctx, &microv1.RuleView{
			ObjectMeta: metav1.ObjectMeta{Name: "current", Namespace: testRuleViewNS},
			Spec:       microv1.RuleViewSpec{RefreshIntervalMinutes: 5},
		})).To(Succeed())

		Eventually(func(g Gomega) {
			got := &microv1.RuleView{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "current", Namespace: testRuleViewNS}, got)).To(Succeed())
			c := meta.FindStatusCondition(got.Status.Conditions, microv1.ConditionReady)
			g.Expect(c).NotTo(BeNil())
			g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(c.Reason).To(Equal(microv1.ReasonSynced))
			g.Expect(got.Status.RuleCount).To(Equal(2))
			g.Expect(got.Status.OwnedCount).To(Equal(1))
			g.Expect(got.Status.ExternalCount).To(Equal(1))
			g.Expect(got.Status.Rules[0].OwnedBy).To(Equal(microv1.RuleOwnedByOperator))
			g.Expect(got.Status.ObservedAt).NotTo(BeNil())
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())
	})
})
