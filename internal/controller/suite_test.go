/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	microsegmentv1alpha1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	ctx       context.Context
	cancel    context.CancelFunc
	testEnv   *envtest.Environment
	cfg       *rest.Config
	k8sClient client.Client
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	var err error
	err = microsegmentv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	// Retrieve the first found binary directory to allow running tests from IDEs
	if getFirstFoundEnvTestBinaryDir() != "" {
		testEnv.BinaryAssetsDirectory = getFirstFoundEnvTestBinaryDir()
	}

	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	By("starting the controller manager")
	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).NotTo(HaveOccurred())

	err = (&PCEConnectionReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
		NewPCEClient: func(cfg pce.Config) PCEPinger {
			if cfg.APIKey == apiKeyBadValue {
				return fakePinger{err: errAuth}
			}
			if cfg.APIKey == "rate" {
				return fakePinger{err: &pce.RateLimitError{RetryAfter: 30 * time.Second}}
			}
			return fakePinger{err: nil}
		},
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&ClusterProfileReconciler{
		Client:              k8sManager.GetClient(),
		Scheme:              k8sManager.GetScheme(),
		OperatorNamespace:   operatorNamespaceForTest,
		NewOnboardingClient: func(pce.Config) OnboardingClient { return fakeOnboardingClient{} },
		Recorder:            k8sManager.GetEventRecorder("clusterprofile-controller"),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&SegmentationIntentReconciler{
		Client:          k8sManager.GetClient(),
		Scheme:          k8sManager.GetScheme(),
		NewPolicyClient: func(pce.Config) PolicyClient { return fakePolicyClient{} },
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&SegmentationPolicyReconciler{
		Client:          k8sManager.GetClient(),
		Scheme:          k8sManager.GetScheme(),
		NewPolicyClient: func(pce.Config) PolicyClient { return fakePolicyClient{} },
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "manager failed to start")
	}()
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	Eventually(func() error {
		return testEnv.Stop()
	}, time.Minute, time.Second).Should(Succeed())
})

const operatorNamespaceForTest = "default"

// cwpUpdatesMu guards the last-recorded CWP update for race-safe assertions.
var (
	cwpUpdatesMu  sync.Mutex
	lastCWPUpdate *pce.CWPUpdate
	lastCWPHref   string
)

// fakeOnboardingClient returns deterministic onboarding results for envtest.
type fakeOnboardingClient struct{}

func (fakeOnboardingClient) FindContainerClusterByName(context.Context, string) (*pce.ContainerCluster, error) {
	return nil, nil // not found → controller creates
}
func (fakeOnboardingClient) CreateContainerCluster(_ context.Context, name, _ string, _ pce.Owner) (*pce.ContainerCluster, error) {
	return &pce.ContainerCluster{Href: "/orgs/1/container_clusters/uuid-ob", Name: name, ContainerClusterToken: "tok-ob"}, nil
}
func (fakeOnboardingClient) FindPairingProfileByName(context.Context, string) (*pce.PairingProfile, error) {
	return nil, nil
}
func (fakeOnboardingClient) CreatePairingProfile(_ context.Context, pp pce.PairingProfile) (*pce.PairingProfile, error) {
	pp.Href = "/orgs/1/pairing_profiles/9"
	return &pp, nil
}
func (fakeOnboardingClient) GeneratePairingKey(context.Context, string) (string, error) {
	return "act-ob", nil
}
func (fakeOnboardingClient) EnsureLabel(_ context.Context, key, value string, _ pce.Owner) (*pce.Label, error) {
	return &pce.Label{Href: "/orgs/1/labels/" + key + "-" + value, Key: key, Value: value}, nil
}
func (fakeOnboardingClient) ListContainerWorkloadProfiles(_ context.Context, _ string) ([]pce.ContainerWorkloadProfile, error) {
	return []pce.ContainerWorkloadProfile{
		{Href: "/orgs/1/container_clusters/uuid-ob/container_workload_profiles/p1", Namespace: cwpTestNamespace, Managed: false},
	}, nil
}
func (fakeOnboardingClient) UpdateContainerWorkloadProfile(_ context.Context, href string, u pce.CWPUpdate) error {
	cwpUpdatesMu.Lock()
	copy := u
	lastCWPUpdate = &copy
	lastCWPHref = href
	cwpUpdatesMu.Unlock()
	return nil
}

// fakePolicyClient is a test double for PolicyClient used by the
// SegmentationIntent controller tests.
type fakePolicyClient struct{}

func (fakePolicyClient) FindLabel(_ context.Context, key, value string) (*pce.Label, error) {
	if value == "does-not-exist" {
		return nil, pce.ErrLabelNotFound
	}
	return &pce.Label{Href: "/orgs/1/labels/" + key + "-" + value, Key: key, Value: value}, nil
}
func (fakePolicyClient) FindRuleSetByOwner(_ context.Context, owner pce.Owner) (*pce.RuleSet, error) {
	deleteRuleSetMu.Lock()
	href := ruleSetsByOwner[owner.Reference]
	deleteRuleSetMu.Unlock()
	if href == "" {
		return nil, nil
	}
	return &pce.RuleSet{Href: href}, nil
}
func (fakePolicyClient) CreateRuleSet(_ context.Context, rs pce.RuleSet) (*pce.RuleSet, error) {
	rs.Href = "/orgs/1/sec_policy/draft/rule_sets/" + rs.ExternalDataReference
	if rs.Href == "/orgs/1/sec_policy/draft/rule_sets/" {
		rs.Href = "/orgs/1/sec_policy/draft/rule_sets/843"
	}
	deleteRuleSetMu.Lock()
	if ruleSetsByOwner == nil {
		ruleSetsByOwner = map[string]string{}
	}
	ruleSetsByOwner[rs.ExternalDataReference] = rs.Href
	deleteRuleSetMu.Unlock()
	return &rs, nil
}
func (fakePolicyClient) DeleteRuleSet(_ context.Context, href string) error {
	deleteRuleSetMu.Lock()
	lastDeletedRuleSetHref = href
	deleteRuleSetMu.Unlock()
	return nil
}
func (fakePolicyClient) ListRules(context.Context, string) ([]pce.SecRule, error) {
	return nil, nil
}
func (fakePolicyClient) CreateRule(_ context.Context, _ string, rule pce.SecRule) (*pce.SecRule, error) {
	rule.Href = "/orgs/1/sec_policy/draft/rule_sets/843/sec_rules/1"
	return &rule, nil
}
func (fakePolicyClient) DeleteRule(context.Context, string) error { return nil }
func (fakePolicyClient) ProvisionRuleSets(context.Context, []string, string) (*pce.ProvisionResult, error) {
	return &pce.ProvisionResult{Version: 80, WorkloadsAffected: 2}, nil
}

// deleteRuleSetMu guards recorded delete/create calls for race-safe assertions.
var (
	deleteRuleSetMu        sync.Mutex
	lastDeletedRuleSetHref string
	// ruleSetsByOwner maps owner.Reference (UID string) → ruleset href.
	ruleSetsByOwner map[string]string
)

// getFirstFoundEnvTestBinaryDir locates the first binary in the specified path.
// ENVTEST-based tests depend on specific binaries, usually located in paths set by
// controller-runtime. When running tests directly (e.g., via an IDE) without using
// Makefile targets, the 'BinaryAssetsDirectory' must be explicitly configured.
//
// This function streamlines the process by finding the required binaries, similar to
// setting the 'KUBEBUILDER_ASSETS' environment variable. To ensure the binaries are
// properly set up, run 'make setup-envtest' beforehand.
func getFirstFoundEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		logf.Log.Error(err, "Failed to read directory", "path", basePath)
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}
