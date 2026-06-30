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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	siPCEConnName   = "pce-si"
	siTeamBNS       = "team-b-si"
	siBadIntentName = "bad"
	siPaymentsNS    = "payments-si"

	// provisioning mode constants used in test specs.
	siModeAuto   = "auto"
	siModeManual = "manual"

	// protocol constant used in test port specs.
	siProtoTCP = "TCP"

	// names used across the manual provisioning test.
	siManualPCEConn    = "pce-manual"
	siManualIntentName = "manual-intent"

	// names used across the delete/finalizer test.
	siDelPCEConn    = "pce-del"
	siDelIntentName = "del-intent"

	// names used across the skip-mode unknown-label test.
	siSkipNS      = "skip-si"
	siSkipPCEConn = "pce-skip"
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
				ProvisioningMode: siModeAuto,
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
				{From: map[string]string{testLabelKeyApp: testLabelValueCheckout}, Ports: []microv1.IntentPort{{Port: 8443, Protocol: siProtoTCP}}},
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
				{From: map[string]string{testLabelKeyApp: labelValDoesNotExist}, Ports: []microv1.IntentPort{{Port: 80}}},
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

	It("skip mode: defers an unknown consumer label instead of rejecting", func() {
		ctx := context.Background()

		// Managed namespace with a resolvable app label.
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: siSkipNS, Labels: map[string]string{testNSLabelPartOf: "skipapp"}},
		})).To(Succeed())

		// PCEConnection (Connected) + secret.
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "pce-creds-skip", Namespace: opNS},
			Data:       map[string][]byte{keyAPIKey: []byte("k"), keyAPISecret: []byte("s")},
		})).To(Succeed())
		pcSkip := &microv1.PCEConnection{
			ObjectMeta: metav1.ObjectMeta{Name: siSkipPCEConn},
			Spec:       microv1.PCEConnectionSpec{PCEURL: testPCEURL, OrgID: 1, CredentialsSecretRef: microv1.SecretReference{Name: "pce-creds-skip", Namespace: opNS}},
		}
		Expect(k8sClient.Create(ctx, pcSkip)).To(Succeed())
		Eventually(func(g Gomega) {
			got := &microv1.PCEConnection{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: siSkipPCEConn}, got)).To(Succeed())
			meta.SetStatusCondition(&got.Status.Conditions, metav1.Condition{Type: microv1.ConditionConnected, Status: metav1.ConditionTrue, Reason: microv1.ReasonConnected, Message: "t"})
			g.Expect(k8sClient.Status().Update(ctx, got)).To(Succeed())
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

		// ClusterProfile with unknownLabelMode: skip, covering siSkipNS.
		cpSkip := &microv1.ClusterProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "cp-skip"},
			Spec: microv1.ClusterProfileSpec{
				PCEConnectionRef: microv1.LocalObjectReference{Name: siSkipPCEConn},
				ProvisioningMode: siModeAuto,
				UnknownLabelMode: microv1.UnknownLabelSkip,
				Onboarding:       microv1.OnboardingSpec{ContainerClusterName: "ocp-skip", CredentialsOutputSecret: "creds-skip-out"},
				NamespaceRules: []microv1.NamespaceRule{
					{Match: microv1.NamespaceMatch{NamePattern: siSkipNS}, Managed: true,
						AssignLabels: map[string]microv1.LabelAssignment{testLabelKeyApp: {FromNamespaceLabel: testNSLabelPartOf}}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, cpSkip)).To(Succeed())
		Eventually(func(g Gomega) {
			got := &microv1.ClusterProfile{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cp-skip"}, got)).To(Succeed())
			g.Expect(meta.FindStatusCondition(got.Status.Conditions, microv1.ConditionOnboarded)).NotTo(BeNil())
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())

		// Intent referencing a label that does not exist in the PCE.
		si := &microv1.SegmentationIntent{
			ObjectMeta: metav1.ObjectMeta{Name: "skip-intent", Namespace: siSkipNS},
			Spec: microv1.SegmentationIntentSpec{Allow: []microv1.IntentAllow{
				{From: map[string]string{testLabelKeyApp: labelValDoesNotExist}, Ports: []microv1.IntentPort{{Port: 80, Protocol: siProtoTCP}}},
			}},
		}
		Expect(k8sClient.Create(ctx, si)).To(Succeed())

		// skip mode → Ready=True (not Rejected) and the missing label is deferred.
		Eventually(func(g Gomega) {
			got := &microv1.SegmentationIntent{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "skip-intent", Namespace: siSkipNS}, got)).To(Succeed())
			g.Expect(meta.IsStatusConditionTrue(got.Status.Conditions, microv1.ConditionReady)).To(BeTrue())
			g.Expect(got.Status.UnknownLabelMode).To(Equal(microv1.UnknownLabelSkip))
			g.Expect(got.Status.DeferredLabels).To(ContainElement(testLabelKeyApp + "=does-not-exist"))
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())
	})

	It("stays ProvisionPending (manual) until annotation, then provisions", func() {
		ctx := context.Background()
		const manualNS = "manual-si"

		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: manualNS, Labels: map[string]string{testNSLabelPartOf: siModeManual}},
		})).To(Succeed())

		// PCEConnection (Connected) + secret for the manual test.
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "pce-creds-manual", Namespace: opNS},
			Data:       map[string][]byte{keyAPIKey: []byte("k"), keyAPISecret: []byte("s")},
		})).To(Succeed())
		pcManual := &microv1.PCEConnection{
			ObjectMeta: metav1.ObjectMeta{Name: siManualPCEConn},
			Spec: microv1.PCEConnectionSpec{
				PCEURL: testPCEURL, OrgID: 1,
				CredentialsSecretRef: microv1.SecretReference{Name: "pce-creds-manual", Namespace: opNS},
			},
		}
		Expect(k8sClient.Create(ctx, pcManual)).To(Succeed())
		Eventually(func(g Gomega) {
			got := &microv1.PCEConnection{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: siManualPCEConn}, got)).To(Succeed())
			meta.SetStatusCondition(&got.Status.Conditions, metav1.Condition{
				Type: microv1.ConditionConnected, Status: metav1.ConditionTrue,
				Reason: microv1.ReasonConnected, Message: "t",
			})
			g.Expect(k8sClient.Status().Update(ctx, got)).To(Succeed())
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

		// ClusterProfile with provisioningMode: manual.
		cpManual := &microv1.ClusterProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "cp-manual"},
			Spec: microv1.ClusterProfileSpec{
				PCEConnectionRef: microv1.LocalObjectReference{Name: siManualPCEConn},
				ProvisioningMode: siModeManual,
				Onboarding:       microv1.OnboardingSpec{ContainerClusterName: "ocp-manual", CredentialsOutputSecret: "creds-manual-out"},
				NamespaceRules: []microv1.NamespaceRule{
					{Match: microv1.NamespaceMatch{NamePattern: manualNS}, Managed: true,
						AssignLabels: map[string]microv1.LabelAssignment{testLabelKeyApp: {FromNamespaceLabel: testNSLabelPartOf}}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, cpManual)).To(Succeed())
		Eventually(func(g Gomega) {
			got := &microv1.ClusterProfile{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cp-manual"}, got)).To(Succeed())
			g.Expect(meta.FindStatusCondition(got.Status.Conditions, microv1.ConditionOnboarded)).NotTo(BeNil())
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())

		// Create the intent — no annotation yet.
		siManual := &microv1.SegmentationIntent{
			ObjectMeta: metav1.ObjectMeta{Name: siManualIntentName, Namespace: manualNS},
			Spec: microv1.SegmentationIntentSpec{Allow: []microv1.IntentAllow{
				{From: map[string]string{testLabelKeyApp: testLabelValueCheckout}, Ports: []microv1.IntentPort{{Port: 443, Protocol: siProtoTCP}}},
			}},
		}
		Expect(k8sClient.Create(ctx, siManual)).To(Succeed())

		// Should reach Ready=True, Provisioned=False/ProvisionPending.
		Eventually(func(g Gomega) {
			got := &microv1.SegmentationIntent{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: siManualIntentName, Namespace: manualNS}, got)).To(Succeed())
			g.Expect(meta.IsStatusConditionTrue(got.Status.Conditions, microv1.ConditionReady)).To(BeTrue())
			c := meta.FindStatusCondition(got.Status.Conditions, microv1.ConditionProvisioned)
			g.Expect(c).NotTo(BeNil())
			g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(c.Reason).To(Equal(microv1.ReasonProvisionPending))
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())

		// Now annotate with approved.
		Eventually(func(g Gomega) {
			got := &microv1.SegmentationIntent{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: siManualIntentName, Namespace: manualNS}, got)).To(Succeed())
			if got.Annotations == nil {
				got.Annotations = map[string]string{}
			}
			got.Annotations[microv1.AnnotationProvisionApprove] = "approved"
			g.Expect(k8sClient.Update(ctx, got)).To(Succeed())
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

		// Should now reach Provisioned=True.
		Eventually(func(g Gomega) {
			got := &microv1.SegmentationIntent{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: siManualIntentName, Namespace: manualNS}, got)).To(Succeed())
			g.Expect(meta.IsStatusConditionTrue(got.Status.Conditions, microv1.ConditionProvisioned)).To(BeTrue())
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())
	})

	It("runs the finalizer and calls DeleteRuleSet on CR deletion", func() {
		ctx := context.Background()
		const deleteNS = "delete-si"

		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: deleteNS, Labels: map[string]string{testNSLabelPartOf: "delete"}},
		})).To(Succeed())

		// Reset recorder before the test.
		deleteRuleSetMu.Lock()
		lastDeletedRuleSetHref = ""
		deleteRuleSetMu.Unlock()

		// PCEConnection + secret for the delete test.
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "pce-creds-del", Namespace: opNS},
			Data:       map[string][]byte{keyAPIKey: []byte("k"), keyAPISecret: []byte("s")},
		})).To(Succeed())
		pcDel := &microv1.PCEConnection{
			ObjectMeta: metav1.ObjectMeta{Name: siDelPCEConn},
			Spec: microv1.PCEConnectionSpec{
				PCEURL: testPCEURL, OrgID: 1,
				CredentialsSecretRef: microv1.SecretReference{Name: "pce-creds-del", Namespace: opNS},
			},
		}
		Expect(k8sClient.Create(ctx, pcDel)).To(Succeed())
		Eventually(func(g Gomega) {
			got := &microv1.PCEConnection{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: siDelPCEConn}, got)).To(Succeed())
			meta.SetStatusCondition(&got.Status.Conditions, metav1.Condition{
				Type: microv1.ConditionConnected, Status: metav1.ConditionTrue,
				Reason: microv1.ReasonConnected, Message: "t",
			})
			g.Expect(k8sClient.Status().Update(ctx, got)).To(Succeed())
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

		// ClusterProfile (auto) covering deleteNS.
		cpDel := &microv1.ClusterProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "cp-del"},
			Spec: microv1.ClusterProfileSpec{
				PCEConnectionRef: microv1.LocalObjectReference{Name: siDelPCEConn},
				ProvisioningMode: siModeAuto,
				Onboarding:       microv1.OnboardingSpec{ContainerClusterName: "ocp-del", CredentialsOutputSecret: "creds-del-out"},
				NamespaceRules: []microv1.NamespaceRule{
					{Match: microv1.NamespaceMatch{NamePattern: deleteNS}, Managed: true,
						AssignLabels: map[string]microv1.LabelAssignment{testLabelKeyApp: {FromNamespaceLabel: testNSLabelPartOf}}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, cpDel)).To(Succeed())
		Eventually(func(g Gomega) {
			got := &microv1.ClusterProfile{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cp-del"}, got)).To(Succeed())
			g.Expect(meta.FindStatusCondition(got.Status.Conditions, microv1.ConditionOnboarded)).NotTo(BeNil())
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())

		// Create intent and wait until provisioned + finalizer added.
		siDel := &microv1.SegmentationIntent{
			ObjectMeta: metav1.ObjectMeta{Name: siDelIntentName, Namespace: deleteNS},
			Spec: microv1.SegmentationIntentSpec{Allow: []microv1.IntentAllow{
				{From: map[string]string{testLabelKeyApp: testLabelValueCheckout}, Ports: []microv1.IntentPort{{Port: 80, Protocol: siProtoTCP}}},
			}},
		}
		Expect(k8sClient.Create(ctx, siDel)).To(Succeed())

		// Wait until reconciled (finalizer present + Provisioned=True).
		Eventually(func(g Gomega) {
			got := &microv1.SegmentationIntent{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: siDelIntentName, Namespace: deleteNS}, got)).To(Succeed())
			g.Expect(meta.IsStatusConditionTrue(got.Status.Conditions, microv1.ConditionProvisioned)).To(BeTrue())
			g.Expect(got.Finalizers).To(ContainElement(segIntentFinalizer))
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())

		// Reset recorder after the initial creation provisioning.
		deleteRuleSetMu.Lock()
		lastDeletedRuleSetHref = ""
		deleteRuleSetMu.Unlock()

		// Delete the intent.
		Expect(k8sClient.Delete(ctx, siDel)).To(Succeed())

		// Object should eventually disappear (finalizer ran).
		Eventually(func(g Gomega) {
			got := &microv1.SegmentationIntent{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: siDelIntentName, Namespace: deleteNS}, got)
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())

		// DeleteRuleSet must have been called.
		deleteRuleSetMu.Lock()
		deletedHref := lastDeletedRuleSetHref
		deleteRuleSetMu.Unlock()
		Expect(deletedHref).NotTo(BeEmpty(), "expected DeleteRuleSet to be called during finalizer")
	})
})
