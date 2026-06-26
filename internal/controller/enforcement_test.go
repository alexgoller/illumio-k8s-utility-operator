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
	enfTestNamespace  = "payments-enf"
	enfTestPCEConn    = "pce-enf"
	enfTestCredSecret = "pce-creds-enf"
	enfTestIntentName = "intent-enf"
)

var _ = Describe("Enforcement-strictest resolution", func() {
	const ns = "default"

	It("raises CWP enforcement to full when a SegmentationIntent requests it, and reports it in intent status", func() {
		ctx := context.Background()

		// Create the namespace; the ClusterProfile rule will set baseline visibility_only.
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: enfTestNamespace, Labels: map[string]string{testNSLabelPartOf: "payments-enf"}},
		})).To(Succeed())

		// Ready PCEConnection + secret.
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: enfTestCredSecret, Namespace: ns},
			Data:       map[string][]byte{keyAPIKey: []byte("k"), keyAPISecret: []byte("s")},
		})).To(Succeed())
		pc := &microv1.PCEConnection{
			ObjectMeta: metav1.ObjectMeta{Name: enfTestPCEConn},
			Spec: microv1.PCEConnectionSpec{
				PCEURL: testPCEURL, OrgID: 1,
				CredentialsSecretRef: microv1.SecretReference{Name: enfTestCredSecret, Namespace: ns},
			},
		}
		Expect(k8sClient.Create(ctx, pc)).To(Succeed())
		Eventually(func(g Gomega) {
			got := &microv1.PCEConnection{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: enfTestPCEConn}, got)).To(Succeed())
			meta.SetStatusCondition(&got.Status.Conditions, metav1.Condition{
				Type: microv1.ConditionConnected, Status: metav1.ConditionTrue, Reason: microv1.ReasonConnected, Message: "t",
			})
			g.Expect(k8sClient.Status().Update(ctx, got)).To(Succeed())
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

		// ClusterProfile: managed with baseline visibility_only + auto provisioning.
		cp := &microv1.ClusterProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "cp-enf"},
			Spec: microv1.ClusterProfileSpec{
				PCEConnectionRef: microv1.LocalObjectReference{Name: enfTestPCEConn},
				ProvisioningMode: "auto",
				Onboarding: microv1.OnboardingSpec{
					ContainerClusterName:    "ocp-enf",
					CredentialsOutputSecret: "creds-enf-out",
				},
				NamespaceRules: []microv1.NamespaceRule{
					{Match: microv1.NamespaceMatch{NamePattern: enfTestNamespace}, Managed: true,
						AssignLabels:    map[string]microv1.LabelAssignment{testLabelKeyApp: {FromNamespaceLabel: testNSLabelPartOf}},
						EnforcementMode: testEnforcementVisOnly},
				},
			},
		}
		Expect(k8sClient.Create(ctx, cp)).To(Succeed())
		// Wait for the ClusterProfile to be onboarded.
		Eventually(func(g Gomega) {
			got := &microv1.ClusterProfile{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cp-enf"}, got)).To(Succeed())
			g.Expect(meta.FindStatusCondition(got.Status.Conditions, microv1.ConditionOnboarded)).NotTo(BeNil())
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())

		// Create a SegmentationIntent requesting enforcement: full.
		si := &microv1.SegmentationIntent{
			ObjectMeta: metav1.ObjectMeta{Name: enfTestIntentName, Namespace: enfTestNamespace},
			Spec: microv1.SegmentationIntentSpec{
				Enforcement: testEnforcementFull,
				Allow: []microv1.IntentAllow{
					{From: map[string]string{testLabelKeyApp: testLabelValueCheckout}, Ports: []microv1.IntentPort{{Port: 8443, Protocol: siProtoTCP}}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, si)).To(Succeed())

		// The CWP update for payments-enf must carry enforcement_mode: full.
		// We check the per-href map so we don't race with updates for other namespaces.
		Eventually(func(g Gomega) {
			cwpEnfUpdatesMu.Lock()
			u, ok := cwpEnfUpdates["/orgs/1/container_clusters/uuid-ob/container_workload_profiles/p2"]
			cwpEnfUpdatesMu.Unlock()
			g.Expect(ok).To(BeTrue())
			g.Expect(u.EnforcementMode).To(Equal(testEnforcementFull))
		}, 20*time.Second, 250*time.Millisecond).Should(Succeed())

		// The intent's status must report effectiveEnforcement=full, enforcementSetBy=intent name.
		Eventually(func(g Gomega) {
			got := &microv1.SegmentationIntent{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: enfTestIntentName, Namespace: enfTestNamespace}, got)).To(Succeed())
			g.Expect(got.Status.EffectiveEnforcement).To(Equal(testEnforcementFull))
			g.Expect(got.Status.EnforcementSetBy).To(Equal(enfTestIntentName))
		}, 20*time.Second, 250*time.Millisecond).Should(Succeed())
	})
})
