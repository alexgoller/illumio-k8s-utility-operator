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
	siPCEConnName   = "pce-si"
	siTeamBNS       = "team-b-si"
	siBadIntentName = "bad"
	siPaymentsNS    = "payments-si"
)

var _ = Describe("SegmentationIntent controller", func() {
	const opNS = "default"

	It("compiles an intent to a ruleset, provisions (auto), and reports affected", func() {
		ctx := context.Background()

		// Managed namespace with an app label.
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: siPaymentsNS, Labels: map[string]string{testNSLabelPartOf: "payments"}},
		})).To(Succeed())

		// PCEConnection (Connected) + secret.
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "pce-creds-si", Namespace: opNS},
			Data:       map[string][]byte{keyAPIKey: []byte("k"), keyAPISecret: []byte("s")},
		})).To(Succeed())
		pc := &microv1.PCEConnection{
			ObjectMeta: metav1.ObjectMeta{Name: siPCEConnName},
			Spec:       microv1.PCEConnectionSpec{PCEURL: testPCEURL, OrgID: 1, CredentialsSecretRef: microv1.SecretReference{Name: "pce-creds-si", Namespace: opNS}},
		}
		Expect(k8sClient.Create(ctx, pc)).To(Succeed())
		Eventually(func(g Gomega) {
			got := &microv1.PCEConnection{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: siPCEConnName}, got)).To(Succeed())
			meta.SetStatusCondition(&got.Status.Conditions, metav1.Condition{Type: microv1.ConditionConnected, Status: metav1.ConditionTrue, Reason: microv1.ReasonConnected, Message: "t"})
			g.Expect(k8sClient.Status().Update(ctx, got)).To(Succeed())
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

		// ClusterProfile: managed app namespace, auto provisioning, onboarded.
		cp := &microv1.ClusterProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "cp-si"},
			Spec: microv1.ClusterProfileSpec{
				PCEConnectionRef: microv1.LocalObjectReference{Name: siPCEConnName},
				ProvisioningMode: "auto",
				Onboarding:       microv1.OnboardingSpec{ContainerClusterName: "ocp-si", CredentialsOutputSecret: "creds-si-out"},
				NamespaceRules: []microv1.NamespaceRule{
					{Match: microv1.NamespaceMatch{NamePattern: siPaymentsNS}, Managed: true,
						AssignLabels: map[string]microv1.LabelAssignment{testLabelKeyApp: {FromNamespaceLabel: testNSLabelPartOf}}},
					{Match: microv1.NamespaceMatch{NamePattern: siTeamBNS}, Managed: true,
						AssignLabels: map[string]microv1.LabelAssignment{testLabelKeyApp: {FromNamespaceLabel: testNSLabelPartOf}}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, cp)).To(Succeed())
		Eventually(func(g Gomega) {
			got := &microv1.ClusterProfile{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cp-si"}, got)).To(Succeed())
			g.Expect(meta.FindStatusCondition(got.Status.Conditions, microv1.ConditionOnboarded)).NotTo(BeNil())
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())

		si := &microv1.SegmentationIntent{
			ObjectMeta: metav1.ObjectMeta{Name: "ingress", Namespace: siPaymentsNS},
			Spec: microv1.SegmentationIntentSpec{Allow: []microv1.IntentAllow{
				{From: map[string]string{testLabelKeyApp: testLabelValueCheckout}, Ports: []microv1.IntentPort{{Port: 8443, Protocol: "TCP"}}},
			}},
		}
		Expect(k8sClient.Create(ctx, si)).To(Succeed())

		Eventually(func(g Gomega) {
			got := &microv1.SegmentationIntent{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ingress", Namespace: siPaymentsNS}, got)).To(Succeed())
			g.Expect(meta.IsStatusConditionTrue(got.Status.Conditions, microv1.ConditionReady)).To(BeTrue())
			g.Expect(meta.IsStatusConditionTrue(got.Status.Conditions, microv1.ConditionProvisioned)).To(BeTrue())
			g.Expect(got.Status.WorkloadsAffected).To(Equal(2))
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())
	})

	It("rejects an intent whose consumer label does not exist in the PCE", func() {
		ctx := context.Background()
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: siTeamBNS, Labels: map[string]string{testNSLabelPartOf: "teamb"}},
		})).To(Succeed())
		si := &microv1.SegmentationIntent{
			ObjectMeta: metav1.ObjectMeta{Name: siBadIntentName, Namespace: siTeamBNS},
			Spec: microv1.SegmentationIntentSpec{Allow: []microv1.IntentAllow{
				{From: map[string]string{testLabelKeyApp: "does-not-exist"}, Ports: []microv1.IntentPort{{Port: 80}}},
			}},
		}
		Expect(k8sClient.Create(ctx, si)).To(Succeed())
		Eventually(func(g Gomega) {
			got := &microv1.SegmentationIntent{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: siBadIntentName, Namespace: siTeamBNS}, got)).To(Succeed())
			c := meta.FindStatusCondition(got.Status.Conditions, microv1.ConditionReady)
			g.Expect(c).NotTo(BeNil())
			g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(c.Reason).To(Equal(microv1.ReasonRejected))
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())
	})
})
