package controller

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
)

func TestOnboardMode(t *testing.T) {
	if got := onboardMode(&microv1.ClusterProfile{}); got != onboardModeCreate {
		t.Errorf("default mode = %q, want create", got)
	}
	adopt := &microv1.ClusterProfile{Spec: microv1.ClusterProfileSpec{Onboarding: microv1.OnboardingSpec{Mode: onboardModeAdopt}}}
	if got := onboardMode(adopt); got != onboardModeAdopt {
		t.Errorf("mode = %q, want adopt", got)
	}
}

var _ = Describe("ClusterProfile onboarding modes", func() {
	const opNS = "default"

	// connectPCE creates a Connected PCEConnection + secret with the given name.
	connectPCE := func(ctx context.Context, name string) {
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name + "-creds", Namespace: opNS},
			Data:       map[string][]byte{keyAPIKey: []byte("k"), keyAPISecret: []byte("s")},
		})).To(Succeed())
		Expect(k8sClient.Create(ctx, &microv1.PCEConnection{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec:       microv1.PCEConnectionSpec{PCEURL: testPCEURL, OrgID: 1, CredentialsSecretRef: microv1.SecretReference{Name: name + "-creds", Namespace: opNS}},
		})).To(Succeed())
		Eventually(func(g Gomega) {
			got := &microv1.PCEConnection{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, got)).To(Succeed())
			meta.SetStatusCondition(&got.Status.Conditions, metav1.Condition{Type: microv1.ConditionConnected, Status: metav1.ConditionTrue, Reason: microv1.ReasonConnected, Message: "t"})
			g.Expect(k8sClient.Status().Update(ctx, got)).To(Succeed())
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())
	}

	It("adopts an already-onboarded cluster (mode=adopt)", func() {
		ctx := context.Background()
		connectPCE(ctx, "pce-adopt")
		Expect(k8sClient.Create(ctx, &microv1.ClusterProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "cp-adopt"},
			Spec: microv1.ClusterProfileSpec{
				PCEConnectionRef: microv1.LocalObjectReference{Name: "pce-adopt"},
				Onboarding:       microv1.OnboardingSpec{Mode: onboardModeAdopt, ContainerClusterName: "existing-adopt-cluster"},
			},
		})).To(Succeed())

		Eventually(func(g Gomega) {
			got := &microv1.ClusterProfile{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cp-adopt"}, got)).To(Succeed())
			c := meta.FindStatusCondition(got.Status.Conditions, microv1.ConditionOnboarded)
			g.Expect(c).NotTo(BeNil())
			g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(got.Status.ContainerClusterHref).To(Equal("/orgs/1/container_clusters/adopted-uuid"))
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())
	})

	It("fails adopt when the cluster is not found", func() {
		ctx := context.Background()
		connectPCE(ctx, "pce-adopt-missing")
		Expect(k8sClient.Create(ctx, &microv1.ClusterProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "cp-adopt-missing"},
			Spec: microv1.ClusterProfileSpec{
				PCEConnectionRef: microv1.LocalObjectReference{Name: "pce-adopt-missing"},
				Onboarding:       microv1.OnboardingSpec{Mode: onboardModeAdopt, ContainerClusterName: "nope-cluster"},
			},
		})).To(Succeed())

		Eventually(func(g Gomega) {
			got := &microv1.ClusterProfile{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cp-adopt-missing"}, got)).To(Succeed())
			c := meta.FindStatusCondition(got.Status.Conditions, microv1.ConditionOnboarded)
			g.Expect(c).NotTo(BeNil())
			g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(c.Reason).To(Equal(microv1.ReasonOnboardFailed))
			g.Expect(c.Message).To(ContainSubstring("adopt"))
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())
	})

	It("fails create when the cluster already exists, pointing to adopt", func() {
		ctx := context.Background()
		connectPCE(ctx, "pce-conflict")
		Expect(k8sClient.Create(ctx, &microv1.ClusterProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "cp-conflict"},
			Spec: microv1.ClusterProfileSpec{
				PCEConnectionRef: microv1.LocalObjectReference{Name: "pce-conflict"},
				Onboarding:       microv1.OnboardingSpec{ContainerClusterName: "existing-conflict-cluster", CredentialsOutputSecret: "creds-conflict"},
			},
		})).To(Succeed())

		Eventually(func(g Gomega) {
			got := &microv1.ClusterProfile{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cp-conflict"}, got)).To(Succeed())
			c := meta.FindStatusCondition(got.Status.Conditions, microv1.ConditionOnboarded)
			g.Expect(c).NotTo(BeNil())
			g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(c.Reason).To(Equal(microv1.ReasonOnboardFailed))
			g.Expect(c.Message).To(ContainSubstring("mode: adopt"))
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())
	})
})
