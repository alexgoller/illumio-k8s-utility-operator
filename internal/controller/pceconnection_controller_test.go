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
	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

const (
	testPCEURL     = "pce.example.com:8443"
	keyAPIKey      = "api_key"
	keyAPISecret   = "api_secret"
	apiKeyBadValue = "bad"
)

var _ = Describe("PCEConnection controller", func() {
	const ns = "default"

	It("sets Connected=True when the PCE ping succeeds", func() {
		ctx := context.Background()
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds-ok", Namespace: ns},
			Data:       map[string][]byte{keyAPIKey: []byte("k"), keyAPISecret: []byte("s")},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		conn := &microv1.PCEConnection{
			ObjectMeta: metav1.ObjectMeta{Name: "conn-ok"},
			Spec: microv1.PCEConnectionSpec{
				PCEURL: testPCEURL, OrgID: 1,
				CredentialsSecretRef: microv1.SecretReference{Name: "creds-ok", Namespace: ns},
			},
		}
		Expect(k8sClient.Create(ctx, conn)).To(Succeed())

		Eventually(func(g Gomega) {
			got := &microv1.PCEConnection{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "conn-ok"}, got)).To(Succeed())
			c := meta.FindStatusCondition(got.Status.Conditions, microv1.ConditionConnected)
			g.Expect(c).NotTo(BeNil())
			g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(c.Reason).To(Equal(microv1.ReasonConnected))
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())
	})

	It("sets Connected=False with AuthFailed when the PCE returns 401", func() {
		ctx := context.Background()
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds-bad", Namespace: ns},
			Data:       map[string][]byte{keyAPIKey: []byte(apiKeyBadValue), keyAPISecret: []byte("s")},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		conn := &microv1.PCEConnection{
			ObjectMeta: metav1.ObjectMeta{Name: "conn-bad"},
			Spec: microv1.PCEConnectionSpec{
				PCEURL: testPCEURL, OrgID: 1,
				CredentialsSecretRef: microv1.SecretReference{Name: "creds-bad", Namespace: ns},
			},
		}
		Expect(k8sClient.Create(ctx, conn)).To(Succeed())

		Eventually(func(g Gomega) {
			got := &microv1.PCEConnection{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "conn-bad"}, got)).To(Succeed())
			c := meta.FindStatusCondition(got.Status.Conditions, microv1.ConditionConnected)
			g.Expect(c).NotTo(BeNil())
			g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(c.Reason).To(Equal(microv1.ReasonAuthFailed))
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())
	})

	It("sets Connected=False with SecretMissing when the credentials Secret does not exist", func() {
		ctx := context.Background()
		// No secret created — CredentialsSecretRef points to a non-existent secret.
		conn := &microv1.PCEConnection{
			ObjectMeta: metav1.ObjectMeta{Name: "conn-nosecret"},
			Spec: microv1.PCEConnectionSpec{
				PCEURL: testPCEURL, OrgID: 1,
				CredentialsSecretRef: microv1.SecretReference{Name: "creds-does-not-exist", Namespace: ns},
			},
		}
		Expect(k8sClient.Create(ctx, conn)).To(Succeed())

		Eventually(func(g Gomega) {
			got := &microv1.PCEConnection{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "conn-nosecret"}, got)).To(Succeed())
			c := meta.FindStatusCondition(got.Status.Conditions, microv1.ConditionConnected)
			g.Expect(c).NotTo(BeNil())
			g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(c.Reason).To(Equal(microv1.ReasonSecretMissing))
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())
	})

	It("sets Connected=False with RateLimited when the PCE returns 429", func() {
		ctx := context.Background()
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds-rate", Namespace: ns},
			Data:       map[string][]byte{keyAPIKey: []byte("rate"), keyAPISecret: []byte("s")},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		conn := &microv1.PCEConnection{
			ObjectMeta: metav1.ObjectMeta{Name: "conn-rate"},
			Spec: microv1.PCEConnectionSpec{
				PCEURL: testPCEURL, OrgID: 1,
				CredentialsSecretRef: microv1.SecretReference{Name: "creds-rate", Namespace: ns},
			},
		}
		Expect(k8sClient.Create(ctx, conn)).To(Succeed())

		Eventually(func(g Gomega) {
			got := &microv1.PCEConnection{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "conn-rate"}, got)).To(Succeed())
			c := meta.FindStatusCondition(got.Status.Conditions, microv1.ConditionConnected)
			g.Expect(c).NotTo(BeNil())
			g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(c.Reason).To(Equal(microv1.ReasonRateLimited))
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())
	})
})

// fakePinger lets tests drive Ping results per PCE URL credentials.
type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

// errAuth is a 401 APIError used in the AuthFailed test wiring (see suite_test.go).
var errAuth = &pce.APIError{StatusCode: 401, Body: "bad key"}
