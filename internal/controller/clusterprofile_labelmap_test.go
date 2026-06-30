package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
)

var _ = Describe("ClusterProfile LabelMap overlap", func() {
	const opNS = "default"

	It("warns when a LabelMap writes a key the operator also assigns", func() {
		ctx := context.Background()

		// A LabelMap whose workloadLabelMap writes the Illumio key "tier".
		lm := &unstructured.Unstructured{}
		lm.SetGroupVersionKind(schema.GroupVersionKind{Group: "ic4k.illumio.com", Version: "v1alpha1", Kind: "LabelMap"})
		lm.SetName("default")
		Expect(unstructured.SetNestedSlice(lm.Object, []any{
			map[string]any{lmFieldFromKey: "stage", lmFieldToKey: testLabelKeyTier, "allowCreate": true},
		}, "workloadLabelMap")).To(Succeed())
		Expect(k8sClient.Create(ctx, lm)).To(Succeed())

		// PCEConnection (Connected) + secret.
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "pce-creds-lm", Namespace: opNS},
			Data:       map[string][]byte{keyAPIKey: []byte("k"), keyAPISecret: []byte("s")},
		})).To(Succeed())
		pc := &microv1.PCEConnection{
			ObjectMeta: metav1.ObjectMeta{Name: testPCEConnLM},
			Spec:       microv1.PCEConnectionSpec{PCEURL: testPCEURL, OrgID: 1, CredentialsSecretRef: microv1.SecretReference{Name: "pce-creds-lm", Namespace: opNS}},
		}
		Expect(k8sClient.Create(ctx, pc)).To(Succeed())
		Eventually(func(g Gomega) {
			got := &microv1.PCEConnection{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: testPCEConnLM}, got)).To(Succeed())
			meta.SetStatusCondition(&got.Status.Conditions, metav1.Condition{Type: microv1.ConditionConnected, Status: metav1.ConditionTrue, Reason: microv1.ReasonConnected, Message: "t"})
			g.Expect(k8sClient.Status().Update(ctx, got)).To(Succeed())
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

		// ClusterProfile that assigns the SAME key "tier" at the namespace level.
		cp := &microv1.ClusterProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "cp-lm"},
			Spec: microv1.ClusterProfileSpec{
				PCEConnectionRef: microv1.LocalObjectReference{Name: testPCEConnLM},
				Onboarding:       microv1.OnboardingSpec{ContainerClusterName: "ocp-lm", CredentialsOutputSecret: "creds-lm-out"},
				NamespaceRules: []microv1.NamespaceRule{
					{Match: microv1.NamespaceMatch{NamePattern: "lm-app-*"}, Managed: true,
						AssignLabels: map[string]microv1.LabelAssignment{testLabelKeyTier: {Value: "backend"}}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, cp)).To(Succeed())

		Eventually(func(g Gomega) {
			got := &microv1.ClusterProfile{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cp-lm"}, got)).To(Succeed())
			c := meta.FindStatusCondition(got.Status.Conditions, microv1.ConditionLabelMapOverlap)
			g.Expect(c).NotTo(BeNil())
			g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(c.Message).To(ContainSubstring(testLabelKeyTier))
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())
	})
})
