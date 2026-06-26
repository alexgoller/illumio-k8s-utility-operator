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
	spPCEConnName   = "pce-sp"
	spPaymentsNS    = "payments-sp"
	spBadPolicyName = "bad-sp"
	spRejectNS      = "reject-sp"
	spModeAuto      = "auto"
	spProtoTCP      = "TCP"
)

var _ = Describe("SegmentationPolicy controller", func() {
	const opNS = "default"

	It("compiles a SegmentationPolicy to a ruleset, provisions (auto), and reports affected", func() {
		ctx := context.Background()

		// Managed namespace with an app label.
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: spPaymentsNS, Labels: map[string]string{testNSLabelPartOf: "payments-sp"}},
		})).To(Succeed())

		// PCEConnection (Connected) + secret.
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "pce-creds-sp", Namespace: opNS},
			Data:       map[string][]byte{keyAPIKey: []byte("k"), keyAPISecret: []byte("s")},
		})).To(Succeed())
		pc := &microv1.PCEConnection{
			ObjectMeta: metav1.ObjectMeta{Name: spPCEConnName},
			Spec: microv1.PCEConnectionSpec{
				PCEURL: testPCEURL, OrgID: 1,
				CredentialsSecretRef: microv1.SecretReference{Name: "pce-creds-sp", Namespace: opNS},
			},
		}
		Expect(k8sClient.Create(ctx, pc)).To(Succeed())
		Eventually(func(g Gomega) {
			got := &microv1.PCEConnection{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: spPCEConnName}, got)).To(Succeed())
			meta.SetStatusCondition(&got.Status.Conditions, metav1.Condition{
				Type: microv1.ConditionConnected, Status: metav1.ConditionTrue,
				Reason: microv1.ReasonConnected, Message: "t",
			})
			g.Expect(k8sClient.Status().Update(ctx, got)).To(Succeed())
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

		// ClusterProfile: managed app namespace, auto provisioning, onboarded.
		cp := &microv1.ClusterProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "cp-sp"},
			Spec: microv1.ClusterProfileSpec{
				PCEConnectionRef: microv1.LocalObjectReference{Name: spPCEConnName},
				ProvisioningMode: spModeAuto,
				Onboarding:       microv1.OnboardingSpec{ContainerClusterName: "ocp-sp", CredentialsOutputSecret: "creds-sp-out"},
				NamespaceRules: []microv1.NamespaceRule{
					{Match: microv1.NamespaceMatch{NamePattern: spPaymentsNS}, Managed: true,
						AssignLabels: map[string]microv1.LabelAssignment{testLabelKeyApp: {FromNamespaceLabel: testNSLabelPartOf}}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, cp)).To(Succeed())
		Eventually(func(g Gomega) {
			got := &microv1.ClusterProfile{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cp-sp"}, got)).To(Succeed())
			g.Expect(meta.FindStatusCondition(got.Status.Conditions, microv1.ConditionOnboarded)).NotTo(BeNil())
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())

		sp := &microv1.SegmentationPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "ingress-sp", Namespace: spPaymentsNS},
			Spec: microv1.SegmentationPolicySpec{
				PolicyTypes: []string{"Ingress"},
				Ingress: []microv1.IngressRule{
					{
						From: []microv1.NetworkPolicyPeer{
							{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{testLabelKeyApp: testLabelValueCheckout}}},
						},
						Ports: []microv1.NetworkPolicyPort{{Port: 8443, Protocol: spProtoTCP}},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, sp)).To(Succeed())

		Eventually(func(g Gomega) {
			got := &microv1.SegmentationPolicy{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ingress-sp", Namespace: spPaymentsNS}, got)).To(Succeed())
			g.Expect(meta.IsStatusConditionTrue(got.Status.Conditions, microv1.ConditionReady)).To(BeTrue())
			g.Expect(meta.IsStatusConditionTrue(got.Status.Conditions, microv1.ConditionProvisioned)).To(BeTrue())
			g.Expect(got.Status.WorkloadsAffected).To(Equal(2))
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())
	})

	It("rejects a SegmentationPolicy with unsupported policyTypes (Ingress+Egress) before any PCE call", func() {
		ctx := context.Background()

		// This namespace doesn't need to be managed — the rejection happens at compile time,
		// before the backend is ever called.
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: spRejectNS, Labels: map[string]string{testNSLabelPartOf: spRejectNS}},
		})).To(Succeed())

		sp := &microv1.SegmentationPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: spBadPolicyName, Namespace: spRejectNS},
			Spec: microv1.SegmentationPolicySpec{
				PolicyTypes: []string{"Ingress", "Egress"},
				Ingress: []microv1.IngressRule{
					{
						From: []microv1.NetworkPolicyPeer{
							{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{testLabelKeyApp: testLabelValueCheckout}}},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, sp)).To(Succeed())

		Eventually(func(g Gomega) {
			got := &microv1.SegmentationPolicy{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: spBadPolicyName, Namespace: spRejectNS}, got)).To(Succeed())
			c := meta.FindStatusCondition(got.Status.Conditions, microv1.ConditionReady)
			g.Expect(c).NotTo(BeNil())
			g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(c.Reason).To(Equal(microv1.ReasonRejected))
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())
	})
})
