# Illumio K8s Utility Operator — Plan 5: NetworkPolicy-style front-end + enforcement-strictest

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the policy design. (1) Add a second front-end, `SegmentationPolicy`, that speaks the Kubernetes `NetworkPolicy` mental model (ingress + selectors + ports) and compiles — through a **shared policy backend** — into the same Illumio ruleset machinery Plan 4 built. (2) Add the per-policy **enforcement-strictest** resolution: each policy CR may request an enforcement mode, and the operator raises the namespace's CWP enforcement to the strictest requested across all its policy CRs (on top of the admin baseline), reporting the effective state back on each CR.

**Architecture:** Extract Plan 4's reconcile logic into a shared backend (package-level functions over a `PolicyClient`) consumed by both the `SegmentationIntent` and the new `SegmentationPolicy` controllers, so the hard parts (provider guardrail, consumer resolution, ruleset/rule reconcile, provisioning, finalizer) live once. Each front-end compiles its CR into a front-end-agnostic `[]CompiledAllow`; the NetworkPolicy compiler enforces a **supported subset** (ingress-only, label selectors → Illumio labels) and rejects unmappable constructs loudly. Enforcement resolution adds a pure `StrictestEnforcement` helper, a shared `EffectiveEnforcement` lister, an extension to Plan 3's CWP reconcile (raise enforcement from policy CRs), and status reporting + watches.

**Tech Stack:** Go, kubebuilder v4 + controller-runtime, envtest, `net/http/httptest`; MkDocs for docs.

## Global Constraints

- **Module path:** `github.com/alexgoller/illumio-k8s-utility-operator`. **API group:** `microsegment.io`; **version:** `v1alpha1`.
- **Target platform:** Illumio Core for Kubernetes in **CLAS** mode, **PCE 24.5+**.
- **Guardrails carry over (design §5):** provider always = the CR's own namespace labels; consumers must resolve to existing PCE labels (never created); no estate-wide scopes; provisioning always scoped to the operator's own ruleset (never provision-all). The new front-end MUST go through the same backend so these hold identically.
- **Enforcement model:** valid container modes are `idle`, `visibility_only`, `full` (strictness order `idle < visibility_only < full`; `selective` unsupported). The **effective** namespace enforcement = strictest of (admin baseline from `ClusterProfile.namespaceRules`/`systemNamespaces`) and (every policy CR's `enforcement` in that namespace). Writing Illumio rules is independent of enforcement; enforcement decides whether non-allowed traffic is blocked.
- **NetworkPolicy supported subset (reject everything else loudly):** `spec.policyTypes` must be `["Ingress"]` (Egress unsupported); `spec.podSelector` must be empty (the policy applies to the whole namespace's app — per-pod providers unsupported); each `ingress[].from[]` must use `podSelector.matchLabels` and/or `namespaceSelector.matchLabels` (mapped to Illumio labels); `matchExpressions`, `ipBlock`, and empty/all `from` are rejected. Ports map to `{port, protocol}`.
- **Lint:** before claiming lint clean, run `rm -f bin/golangci-lint && make lint` (a stale cached linter masks new staticcheck findings — CI rebuilds fresh).
- **Commits:** conventional-commit messages; commit at the end of every task; end messages with the repo's `Co-Authored-By` trailer.

---

### Task 1: Add `enforcement` to policy specs + the `SegmentationPolicy` CRD

**Files:**
- Modify: `api/v1alpha1/segmentationintent_types.go` (add `Enforcement` to spec; `EffectiveEnforcement`/`EnforcementSetBy` to status; `ReasonEnforcementRaised` if needed)
- Create: `api/v1alpha1/segmentationpolicy_types.go`
- Test: `api/v1alpha1/segmentationpolicy_types_test.go`
- Generated: deepcopy, CRDs, RBAC.

**Interfaces:**
- Produces:
  - `SegmentationIntentSpec` gains `Enforcement string` (enum idle;visibility_only;full, optional); `SegmentationIntentStatus` gains `EffectiveEnforcement string`, `EnforcementSetBy string`.
  - `NetworkPolicyPeer{ PodSelector *metav1.LabelSelector; NamespaceSelector *metav1.LabelSelector }`
  - `NetworkPolicyPort{ Port int; Protocol string }`
  - `IngressRule{ From []NetworkPolicyPeer; Ports []NetworkPolicyPort }`
  - `SegmentationPolicySpec{ PodSelector metav1.LabelSelector; Ingress []IngressRule; PolicyTypes []string; Enforcement string }`
  - `SegmentationPolicyStatus` (same shape as SegmentationIntentStatus: conditions, workloadsAffected, effectiveEnforcement, enforcementSetBy, observedGeneration)
  - `SegmentationPolicy`/`SegmentationPolicyList` (namespaced, category illumio, shortName segpol)

- [ ] **Step 1: Write the failing test** — `api/v1alpha1/segmentationpolicy_types_test.go`:

```go
package v1alpha1

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSegmentationPolicy_Shape(t *testing.T) {
	sp := SegmentationPolicy{
		Spec: SegmentationPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []string{"Ingress"},
			Enforcement: "full",
			Ingress: []IngressRule{
				{
					From:  []NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "checkout"}}}},
					Ports: []NetworkPolicyPort{{Port: 8443, Protocol: "TCP"}},
				},
			},
		},
	}
	if sp.Spec.Ingress[0].From[0].PodSelector.MatchLabels["app"] != "checkout" {
		t.Errorf("from app = %q", sp.Spec.Ingress[0].From[0].PodSelector.MatchLabels["app"])
	}
	if sp.Spec.Enforcement != "full" {
		t.Errorf("enforcement = %q", sp.Spec.Enforcement)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `go test ./api/v1alpha1/ -run TestSegmentationPolicy -v` → RED.

- [ ] **Step 3: Add the `enforcement` fields to SegmentationIntent**

In `segmentationintent_types.go`, add to `SegmentationIntentSpec`:

```go
	// Enforcement requests a namespace enforcement mode. The operator applies
	// the strictest mode requested across all policy CRs in the namespace (on
	// top of the admin baseline). One of idle, visibility_only, full.
	// +kubebuilder:validation:Enum=idle;visibility_only;full
	// +optional
	Enforcement string `json:"enforcement,omitempty"`
```

Add to `SegmentationIntentStatus`:

```go
	// EffectiveEnforcement is the namespace's resolved enforcement mode.
	// +optional
	EffectiveEnforcement string `json:"effectiveEnforcement,omitempty"`
	// EnforcementSetBy names what set the effective enforcement (a CR name or "admin").
	// +optional
	EnforcementSetBy string `json:"enforcementSetBy,omitempty"`
```

- [ ] **Step 4: Create the SegmentationPolicy types** — `api/v1alpha1/segmentationpolicy_types.go`:

```go
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NetworkPolicyPeer is a consumer selector (a supported subset of k8s NetworkPolicyPeer).
type NetworkPolicyPeer struct {
	// +optional
	PodSelector *metav1.LabelSelector `json:"podSelector,omitempty"`
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`
}

// NetworkPolicyPort is a port/protocol.
type NetworkPolicyPort struct {
	Port int `json:"port"`
	// +kubebuilder:validation:Enum=TCP;UDP
	// +kubebuilder:default=TCP
	// +optional
	Protocol string `json:"protocol,omitempty"`
}

// IngressRule allows traffic from the listed peers on the listed ports.
type IngressRule struct {
	From  []NetworkPolicyPeer  `json:"from"`
	Ports []NetworkPolicyPort  `json:"ports,omitempty"`
}

// SegmentationPolicySpec mirrors a supported subset of k8s NetworkPolicy.
type SegmentationPolicySpec struct {
	// PodSelector must be empty: the policy applies to the whole namespace's app.
	// +optional
	PodSelector metav1.LabelSelector `json:"podSelector,omitempty"`
	// Ingress rules (the only supported direction).
	Ingress []IngressRule `json:"ingress"`
	// PolicyTypes must be ["Ingress"].
	// +optional
	PolicyTypes []string `json:"policyTypes,omitempty"`
	// Enforcement requests a namespace enforcement mode (see SegmentationIntent).
	// +kubebuilder:validation:Enum=idle;visibility_only;full
	// +optional
	Enforcement string `json:"enforcement,omitempty"`
}

// SegmentationPolicyStatus is the observed state.
type SegmentationPolicyStatus struct {
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// +optional
	WorkloadsAffected int `json:"workloadsAffected,omitempty"`
	// +optional
	EffectiveEnforcement string `json:"effectiveEnforcement,omitempty"`
	// +optional
	EnforcementSetBy string `json:"enforcementSetBy,omitempty"`
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=illumio,shortName=segpol
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Provisioned",type=string,JSONPath=`.status.conditions[?(@.type=="Provisioned")].status`
// +kubebuilder:printcolumn:name="Enforcement",type=string,JSONPath=`.status.effectiveEnforcement`

// SegmentationPolicy is a NetworkPolicy-style Illumio allow-list for a namespace.
type SegmentationPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SegmentationPolicySpec   `json:"spec,omitempty"`
	Status SegmentationPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SegmentationPolicyList contains a list of SegmentationPolicy.
type SegmentationPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SegmentationPolicy `json:"items"`
}

func init() {
	// Match the existing registration pattern in this package (runtime.NewSchemeBuilder).
	SchemeBuilder.Register(func(s *runtimeScheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &SegmentationPolicy{}, &SegmentationPolicyList{})
		return nil
	})
}
```

> **`init()` note:** replicate the EXACT registration form already in `segmentationintent_types.go`/`clusterprofile_types.go` (use the real `*runtime.Scheme` and the existing import alias — do not introduce a `runtimeScheme` type or a `scheme` package; the literal above is a placeholder for whatever those files use).

- [ ] **Step 5: Regenerate + verify** — `make generate`, `make manifests`, `go test ./api/v1alpha1/ -run "TestSegmentation" -v` (both intent + policy), `make build`. Confirm `config/crd/bases/microsegment.io_segmentationpolicies.yaml` is namespaced, category illumio, shortName segpol; and `segmentationintents.yaml` gains the `enforcement` spec field + `effectiveEnforcement`/`enforcementSetBy` status fields.

- [ ] **Step 6: Commit** — `git add api/v1alpha1/ config/`; `git commit -m "feat(api): add SegmentationPolicy CRD and enforcement fields"`.

---

### Task 2: Front-end compilers + strictest-enforcement helper (pure)

**Files:**
- Modify: `internal/controller/policyir.go` (add `CompiledAllow`, `CompileIntent`, `CompilePolicy`, `StrictestEnforcement`)
- Test: `internal/controller/policycompile_test.go`

**Interfaces:**
- Produces:
  - `type CompiledAllow struct { From map[string]string; Ports []pce.IngressService }`
  - `func CompileIntent(allows []microv1.IntentAllow) []CompiledAllow`
  - `func CompilePolicy(spec microv1.SegmentationPolicySpec) ([]CompiledAllow, error)` — applies the supported subset; returns a descriptive error (→ Rejected) for any unsupported construct
  - `func StrictestEnforcement(modes ...string) string` — returns the strictest non-empty mode (idle<visibility_only<full); "" if none

- [ ] **Step 1: Write the failing test** — `internal/controller/policycompile_test.go`:

```go
package controller

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
)

func TestCompilePolicy_SupportedSubset(t *testing.T) {
	spec := microv1.SegmentationPolicySpec{
		PolicyTypes: []string{"Ingress"},
		Ingress: []microv1.IngressRule{{
			From:  []microv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "checkout", "env": "prod"}}}},
			Ports: []microv1.NetworkPolicyPort{{Port: 8443, Protocol: "TCP"}},
		}},
	}
	allows, err := CompilePolicy(spec)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(allows) != 1 || allows[0].From["app"] != "checkout" || allows[0].From["env"] != "prod" {
		t.Fatalf("allows = %+v", allows)
	}
	if len(allows[0].Ports) != 1 || allows[0].Ports[0].Proto != 6 || allows[0].Ports[0].Port != 8443 {
		t.Errorf("ports = %+v", allows[0].Ports)
	}
}

func TestCompilePolicy_RejectsUnsupported(t *testing.T) {
	cases := map[string]microv1.SegmentationPolicySpec{
		"egress policyType": {PolicyTypes: []string{"Ingress", "Egress"}, Ingress: []microv1.IngressRule{{From: []microv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "x"}}}}}}},
		"non-empty podSelector": {PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"role": "db"}}, Ingress: []microv1.IngressRule{{From: []microv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "x"}}}}}}},
		"matchExpressions": {Ingress: []microv1.IngressRule{{From: []microv1.NetworkPolicyPeer{{PodSelector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "app", Operator: metav1.LabelSelectorOpExists}}}}}}}},
		"empty from":       {Ingress: []microv1.IngressRule{{From: nil}}},
	}
	for name, spec := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := CompilePolicy(spec); err == nil {
				t.Fatalf("expected rejection for %s", name)
			} else if strings.TrimSpace(err.Error()) == "" {
				t.Fatalf("rejection error must be descriptive")
			}
		})
	}
}

func TestStrictestEnforcement(t *testing.T) {
	if StrictestEnforcement("idle", "full", "visibility_only") != "full" {
		t.Errorf("want full")
	}
	if StrictestEnforcement("", "visibility_only", "idle") != "visibility_only" {
		t.Errorf("want visibility_only")
	}
	if StrictestEnforcement("", "") != "" {
		t.Errorf("want empty")
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `go test ./internal/controller/ -run "CompilePolicy|StrictestEnforcement" -v` → RED.

- [ ] **Step 3: Implement** — append to `internal/controller/policyir.go`:

```go
// CompiledAllow is a front-end-agnostic allow entry: consumer labels (key->value)
// and proto-resolved ports.
type CompiledAllow struct {
	From  map[string]string
	Ports []pce.IngressService
}

// CompileIntent lowers a SegmentationIntent's allow list to CompiledAllow.
func CompileIntent(allows []microv1.IntentAllow) []CompiledAllow {
	out := make([]CompiledAllow, 0, len(allows))
	for _, a := range allows {
		ports := make([]pce.IngressService, 0, len(a.Ports))
		for _, p := range a.Ports {
			ports = append(ports, pce.IngressService{Proto: protoNumber(p.Protocol), Port: p.Port})
		}
		out = append(out, CompiledAllow{From: a.From, Ports: ports})
	}
	return out
}

// CompilePolicy lowers a SegmentationPolicy (supported NetworkPolicy subset) to
// CompiledAllow, returning a descriptive error for any unsupported construct.
func CompilePolicy(spec microv1.SegmentationPolicySpec) ([]CompiledAllow, error) {
	for _, t := range spec.PolicyTypes {
		if t != "Ingress" {
			return nil, fmt.Errorf("unsupported policyType %q: only Ingress is supported", t)
		}
	}
	if len(spec.PodSelector.MatchLabels) > 0 || len(spec.PodSelector.MatchExpressions) > 0 {
		return nil, fmt.Errorf("spec.podSelector must be empty: the policy applies to the whole namespace's app")
	}
	out := make([]CompiledAllow, 0, len(spec.Ingress))
	for i, ing := range spec.Ingress {
		if len(ing.From) == 0 {
			return nil, fmt.Errorf("ingress[%d].from must list at least one peer (allow-all is not supported)", i)
		}
		from := map[string]string{}
		for j, peer := range ing.From {
			for _, sel := range []*metav1.LabelSelector{peer.PodSelector, peer.NamespaceSelector} {
				if sel == nil {
					continue
				}
				if len(sel.MatchExpressions) > 0 {
					return nil, fmt.Errorf("ingress[%d].from[%d]: matchExpressions are not supported; use matchLabels", i, j)
				}
				for k, v := range sel.MatchLabels {
					from[k] = v
				}
			}
			if peer.PodSelector == nil && peer.NamespaceSelector == nil {
				return nil, fmt.Errorf("ingress[%d].from[%d]: a podSelector or namespaceSelector is required (ipBlock is not supported)", i, j)
			}
		}
		if len(from) == 0 {
			return nil, fmt.Errorf("ingress[%d].from: no matchLabels found", i)
		}
		ports := make([]pce.IngressService, 0, len(ing.Ports))
		for _, p := range ing.Ports {
			ports = append(ports, pce.IngressService{Proto: protoNumber(p.Protocol), Port: p.Port})
		}
		out = append(out, CompiledAllow{From: from, Ports: ports})
	}
	return out, nil
}

var enforcementRank = map[string]int{"": 0, "idle": 1, "visibility_only": 2, "full": 3}

// StrictestEnforcement returns the strictest non-empty mode, or "" if none.
func StrictestEnforcement(modes ...string) string {
	best := ""
	for _, m := range modes {
		if enforcementRank[m] > enforcementRank[best] {
			best = m
		}
	}
	return best
}
```

(Add imports `"fmt"` and `metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"` and `microv1 ".../api/v1alpha1"` to `policyir.go` if not already present.)

- [ ] **Step 4: Run the tests** — `go test ./internal/controller/ -run "Compile|Strictest" -v` → PASS.

- [ ] **Step 5: Commit** — `git add internal/controller/policyir.go internal/controller/policycompile_test.go`; `git commit -m "feat(controller): add NetworkPolicy compiler (supported subset) and strictest-enforcement helper"`.

---

### Task 3: Extract the shared policy backend

**Files:**
- Create: `internal/controller/policybackend.go`
- Modify: `internal/controller/segmentationintent_controller.go` (use the shared backend)
- Test: existing `segmentationintent_controller_test.go` must stay green (no new test needed; this is a refactor).

**Interfaces:**
- Produces (package-level functions taking the k8s client + a `PolicyClient` factory):
  - `type BackendResult struct { Ready *Condition; Provisioned *Condition; WorkloadsAffected int; Requeue time.Duration }` where `Condition{ Status metav1.ConditionStatus; Reason, Message string }`
  - `func ReconcilePolicy(ctx, k8s client.Client, factory PolicyClientFactory, namespace, crName string, uid types.UID, annotations map[string]string, allows []CompiledAllow) (BackendResult, error)` — does: resolveClusterProfile(namespace) → not-ready ⇒ Ready=False/ClusterProfileNotReady; resolveProvider; resolveAllows(allows); reconcileRuleSet; provision per mode/annotation; returns conditions. The returned `error` is a transient error (requeue); guardrail rejections come back as `Ready=False/Rejected` in the result, not an error.
  - `func FinalizePolicy(ctx, k8s client.Client, factory PolicyClientFactory, namespace string, uid types.UID) error`

**Refactor approach:** move `resolveClusterProfile`, `resolveProvider`, `resolveAllows` (changed to accept `[]CompiledAllow`), `reconcileRuleSet`, and `finalize` out of the `SegmentationIntentReconciler` methods into package-level functions in `policybackend.go` that take `(ctx, k8s, factory, ...)`. The `SegmentationIntentReconciler.Reconcile` becomes: Get CR → finalizer handling (calls `FinalizePolicy`) → `CompileIntent(si.Spec.Allow)` → `ReconcilePolicy(...)` → map `BackendResult` onto `si.Status` conditions → `Status().Update`.

- [ ] **Step 1: Create `policybackend.go`** with the package-level functions. Lift the bodies of the existing `SegmentationIntentReconciler.resolve*`/`reconcileRuleSet`/`finalize` methods verbatim, changing the receiver to explicit `k8s client.Client` and `factory PolicyClientFactory` parameters, and changing `resolveAllows` to take `[]CompiledAllow` (resolve `a.From` via `FindLabel`, build `ResolvedAllow{ConsumerHrefs, Ports: a.Ports}`). `ReconcilePolicy` orchestrates them and returns a `BackendResult`; `FinalizePolicy` wraps `finalize`. Provisioning reads `annotations[microv1.AnnotationProvisionApprove]` for the manual gate. Keep `var _ PolicyClient = (*pce.Client)(nil)` (already in `policy_client.go`).

- [ ] **Step 2: Rewrite `SegmentationIntentReconciler.Reconcile`** to delegate to `ReconcilePolicy`/`FinalizePolicy`, compiling via `CompileIntent`, and mapping the `BackendResult` to its status conditions (Ready + Provisioned + WorkloadsAffected + RequeueAfter). Remove the now-moved private methods from the intent controller.

- [ ] **Step 3: Run the existing SegmentationIntent specs** — `make test` (the Plan 4 specs: auto-provision happy path, consumer-missing rejection, manual gate, delete/finalizer) MUST still pass unchanged. `make build`; `rm -f bin/golangci-lint && make lint`.

- [ ] **Step 4: Commit** — `git add internal/controller/`; `git commit -m "refactor(controller): extract shared policy backend used by both front-ends"`.

---

### Task 4: SegmentationPolicy controller

**Files:**
- Create: `internal/controller/segmentationpolicy_controller.go`
- Test: `internal/controller/segmentationpolicy_controller_test.go`
- Modify: `internal/controller/suite_test.go` (register; reuse the existing `fakePolicyClient`)
- Modify: `cmd/main.go` (wire)

**Interfaces:**
- Consumes: `ReconcilePolicy`/`FinalizePolicy`/`BackendResult`/`CompilePolicy` (Tasks 2, 3); `PolicyClientFactory`.
- Produces: `SegmentationPolicyReconciler{ client.Client; Scheme; NewPolicyClient PolicyClientFactory }` whose `Reconcile`: Get CR → finalizer (shared `segPolicyFinalizer`) → `CompilePolicy(sp.Spec)` (compile error ⇒ Ready=False/Rejected, no backend call) → `ReconcilePolicy(...)` → map result to status.

- [ ] **Step 1: Write the failing test** — `segmentationpolicy_controller_test.go`: mirror the SegmentationIntent happy-path spec but with a `SegmentationPolicy` (ingress from podSelector `app=checkout`, port 8443, `policyTypes: [Ingress]`, the managed `payments-si`-style namespace + onboarded `cp-si` ClusterProfile reused from the intent suite) → `Ready=True` + `Provisioned=True`. Add a rejection spec: a `SegmentationPolicy` with `policyTypes: [Ingress, Egress]` → `Ready=False / Rejected` (compile rejection, before any PCE call). Reuse the existing `fakePolicyClient` (it resolves all labels except `does-not-exist` and provisions affected=2).

- [ ] **Step 2: Register in the suite** — add the `SegmentationPolicyReconciler` registration alongside the others (same `NewPolicyClient: func(pce.Config) PolicyClient { return fakePolicyClient{} }`). The happy-path namespace must be managed by the suite's `cp-si` ClusterProfile — reuse a namespace its rules manage (or add a rule), same as Plan 4 Task 4.

- [ ] **Step 3: Run to verify it fails** — `make test` → compile failure (`undefined: SegmentationPolicyReconciler`).

- [ ] **Step 4: Write the controller** — `segmentationpolicy_controller.go`. Structure mirrors the refactored `SegmentationIntentReconciler`:

```go
package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
)

const segPolicyFinalizer = "microsegment.io/segmentationpolicy"

// SegmentationPolicyReconciler reconciles a SegmentationPolicy.
type SegmentationPolicyReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	NewPolicyClient PolicyClientFactory
}

// +kubebuilder:rbac:groups=microsegment.io,resources=segmentationpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=microsegment.io,resources=segmentationpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=microsegment.io,resources=segmentationpolicies/finalizers,verbs=update

func (r *SegmentationPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var sp microv1.SegmentationPolicy
	if err := r.Get(ctx, req.NamespacedName, &sp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if r.NewPolicyClient == nil {
		r.NewPolicyClient = DefaultPolicyClientFactory
	}

	if !sp.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&sp, segPolicyFinalizer) {
			if err := FinalizePolicy(ctx, r.Client, r.NewPolicyClient, sp.Namespace, sp.UID); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(&sp, segPolicyFinalizer)
			if err := r.Update(ctx, &sp); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}
	if controllerutil.AddFinalizer(&sp, segPolicyFinalizer) {
		if err := r.Update(ctx, &sp); err != nil {
			return ctrl.Result{}, err
		}
	}

	allows, cerr := CompilePolicy(sp.Spec)
	if cerr != nil {
		meta.SetStatusCondition(&sp.Status.Conditions, metav1.Condition{
			Type: microv1.ConditionReady, Status: metav1.ConditionFalse, Reason: microv1.ReasonRejected, Message: cerr.Error(),
		})
		sp.Status.ObservedGeneration = sp.Generation
		return ctrl.Result{}, r.Status().Update(ctx, &sp)
	}

	res, err := ReconcilePolicy(ctx, r.Client, r.NewPolicyClient, sp.Namespace, sp.Name, sp.UID, sp.Annotations, allows)
	if err != nil {
		return ctrl.Result{}, err
	}
	applyBackendResult(&sp.Status.Conditions, res)
	sp.Status.WorkloadsAffected = res.WorkloadsAffected
	sp.Status.ObservedGeneration = sp.Generation
	if err := r.Status().Update(ctx, &sp); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: res.Requeue}, nil
}

func (r *SegmentationPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).For(&microv1.SegmentationPolicy{}).Complete(r)
}
```

> Add a small shared `applyBackendResult(conds *[]metav1.Condition, res BackendResult)` helper in `policybackend.go` that sets the Ready and Provisioned conditions from the result (used by both controllers — refactor the intent controller to use it too).

- [ ] **Step 5: Wire main** — register `SegmentationPolicyReconciler` in `cmd/main.go` (mirror the SegmentationIntent registration).

- [ ] **Step 6: Run tests/build/lint** — `make test` (both new specs + all prior pass), `make build`, `rm -f bin/golangci-lint && make lint`.

- [ ] **Step 7: Commit** — `git add internal/controller/ cmd/main.go config/rbac/`; `git commit -m "feat(controller): add SegmentationPolicy (NetworkPolicy-style) controller over the shared backend"`.

---

### Task 5: Enforcement-strictest resolution

**Files:**
- Create: `internal/controller/enforcement.go` (the shared `EffectiveEnforcement` lister)
- Modify: `internal/controller/clusterprofile_controller.go` (raise CWP enforcement from policy CRs; watch policy CRs)
- Modify: both policy controllers (report `EffectiveEnforcement`/`EnforcementSetBy` in status)
- Test: `internal/controller/enforcement_test.go` + an envtest assertion

**Interfaces:**
- Produces:
  - `func EffectiveEnforcement(ctx, k8s client.Client, namespace, baseline string) (mode string, setBy string, err error)` — lists `SegmentationIntent` + `SegmentationPolicy` in the namespace, returns the strictest of `baseline` and each CR's `spec.enforcement` (via `StrictestEnforcement`), and `setBy` = the CR name that set it (or `"admin"` if the baseline won / nothing raised it).

- [ ] **Step 1: Write the failing test** — `enforcement_test.go` (pure-ish, using a fake client list is heavier; prefer an envtest in the controller suite). Minimal unit test of `StrictestEnforcement` is already in Task 2; here add an envtest spec: a managed namespace with admin baseline `visibility_only` (via the ClusterProfile rule) plus a `SegmentationIntent` with `enforcement: full` → the namespace's CWP is updated to `full` and the intent's `status.effectiveEnforcement == "full"`, `enforcementSetBy == <intent name>`. (Drive via the existing CWP envtest harness + the fake clients.)

- [ ] **Step 2: Implement `EffectiveEnforcement`** — `internal/controller/enforcement.go`:

```go
package controller

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
)

// EffectiveEnforcement returns the strictest enforcement for a namespace: the
// admin baseline raised by any policy CR's spec.enforcement. setBy names the CR
// that set the result, or "admin" if the baseline was not raised.
func EffectiveEnforcement(ctx context.Context, k8s client.Client, namespace, baseline string) (string, string, error) {
	mode, setBy := baseline, "admin"
	var intents microv1.SegmentationIntentList
	if err := k8s.List(ctx, &intents, client.InNamespace(namespace)); err != nil {
		return "", "", err
	}
	for i := range intents.Items {
		if raised := StrictestEnforcement(mode, intents.Items[i].Spec.Enforcement); raised != mode {
			mode, setBy = raised, intents.Items[i].Name
		}
	}
	var policies microv1.SegmentationPolicyList
	if err := k8s.List(ctx, &policies, client.InNamespace(namespace)); err != nil {
		return "", "", err
	}
	for i := range policies.Items {
		if raised := StrictestEnforcement(mode, policies.Items[i].Spec.Enforcement); raised != mode {
			mode, setBy = raised, policies.Items[i].Name
		}
	}
	return mode, setBy, nil
}
```

- [ ] **Step 3: Raise CWP enforcement in the ClusterProfile reconcile** — in `clusterprofile_controller.go` `reconcileNamespaceCWPs`, after computing `desired := ComputeDesiredCWP(...)` for a managed namespace, call `EffectiveEnforcement(ctx, r.Client, nsObj.Name, desired.EnforcementMode)` and set `desired.EnforcementMode` to the returned strictest mode before building the CWP update. (This makes the CWP reflect the strictest of admin baseline + policy CRs.) Also add a Namespace-independent watch: `Watches(&microv1.SegmentationIntent{}, ...)` and `Watches(&microv1.SegmentationPolicy{}, ...)` that enqueue all ClusterProfiles, so an enforcement change on a policy CR re-runs the CWP sweep. Add RBAC to read segmentationintents/segmentationpolicies.

- [ ] **Step 4: Report effective enforcement on policy CRs** — in `ReconcilePolicy` (or the controllers), after the backend resolves the ClusterProfile, compute `EffectiveEnforcement(ctx, k8s, namespace, baselineFromCP)` where the baseline is the namespace's admin enforcement from `ComputeDesiredCWP`, and surface `mode`/`setBy` in the `BackendResult` so each controller writes `status.EffectiveEnforcement`/`status.EnforcementSetBy`. (Add those fields to `BackendResult`.)

- [ ] **Step 5: Run tests/build/lint** — `make test` (the new enforcement envtest + all prior), `make build`, `rm -f bin/golangci-lint && make lint`.

- [ ] **Step 6: Commit** — `git add internal/controller/ config/rbac/`; `git commit -m "feat(controller): raise namespace enforcement to the strictest requested across policy CRs"`.

---

### Task 6: Documentation

**Files:**
- Create: `docs/guides/networkpolicy-style.md`
- Create: `docs/reference/segmentationpolicy.md`
- Modify: `docs/guides/segmentation-policy.md` (cross-link the two front-ends; document the `enforcement` field + effective-enforcement behavior)
- Modify: `docs/reference/segmentationintent.md` (add the `enforcement` spec field + `effectiveEnforcement`/`enforcementSetBy` status)
- Modify: `docs/getting-started.md`, `mkdocs.yml`
- Run: `make docs-api`.

> Dispatch to the documentation subagent. Cross-check every field against the Go types.

- [ ] **Step 1: NetworkPolicy-style guide** — `docs/guides/networkpolicy-style.md`: who it's for (k8s engineers who know `NetworkPolicy`); a complete `SegmentationPolicy` example (ingress + podSelector/namespaceSelector matchLabels + ports); the **supported subset** and exactly what is rejected (Egress, non-empty `podSelector`, `matchExpressions`, `ipBlock`, empty `from`) with the rejection surfacing as `Ready=False / Rejected`; that it compiles to the **same** Illumio ruleset machinery as `SegmentationIntent` (same guardrails — provider is your namespace's app, consumers must exist); and that the choice between the two front-ends is mental-model preference. Note that selectors map to **Illumio labels** (matchLabels become label key/value lookups), not raw pod selection.

- [ ] **Step 2: SegmentationPolicy reference** — `docs/reference/segmentationpolicy.md`: spec (`podSelector` must be empty; `ingress[].from[].{podSelector,namespaceSelector}.matchLabels`; `ingress[].ports[].{port,protocol}`; `policyTypes`; `enforcement`), status (conditions, `workloadsAffected`, `effectiveEnforcement`, `enforcementSetBy`), in the table style.

- [ ] **Step 3: Enforcement docs** — in both reference pages and the segmentation-policy guide, document the `enforcement` field and the **strictest-wins** behavior: a policy CR may request `idle`/`visibility_only`/`full`; the namespace's effective enforcement is the strictest of the admin baseline and all policy CRs; the result shows in `status.effectiveEnforcement`/`enforcementSetBy` and on the namespace's CWP. Reiterate that enforcement determines blocking, rules determine what's allowed.

- [ ] **Step 4: getting-started + nav** — add the NetworkPolicy-style option to the policy step; add nav entries `Guides → NetworkPolicy-style: guides/networkpolicy-style.md` and `API Reference → SegmentationPolicy: reference/segmentationpolicy.md`.

- [ ] **Step 5: Regenerate + build** — `make docs-api`; `mkdocs build --strict` (or rely on Docs CI). `docs/reference/api.md` includes `SegmentationPolicy`.

- [ ] **Step 6: Commit** — `git add docs/ mkdocs.yml`; `git commit -m "docs: add NetworkPolicy-style front-end guide, SegmentationPolicy reference, and enforcement docs"`.

---

## Self-Review

**Spec coverage (design spec §4.5, §5, §5.1, §7):**
- §7 two-front-ends/one-backend — the shared backend (`policybackend.go`) is used by both `SegmentationIntent` and `SegmentationPolicy` → Tasks 3, 4. ✓
- §4.5 `SegmentationPolicy` NetworkPolicy-style front-end with a documented supported subset (reject unmappable loudly) → Tasks 1, 2, 4. ✓
- §5 guardrails preserved identically (both front-ends go through the same backend; provider locked, consumers must exist) → Tasks 3, 4. ✓
- §5.1 per-policy enforcement resolved to the strictest per namespace, applied to the CWP, with effective state + source reported on each CR → Tasks 1, 2 (`StrictestEnforcement`), 5. ✓
- Docs from the start → Task 6. ✓

**This completes the design.** All four CRDs (`PCEConnection`, `ClusterProfile`, `SegmentationIntent`, `SegmentationPolicy`) and both policy front-ends ship; CWP management, onboarding, and provisioning are in place.

**Out of scope / future (noted, not in this plan):** per-change manual approval (the v1 manual gate is standing-approval; finer control needs rule-diffing to replace replace-all); `ipBlock`/IP-list policy; egress policy; `matchExpressions` selectors.

**Placeholder scan:** No `TBD`/"handle edge cases" — code steps are complete. The `init()` registration note (Task 1) and the refactor "lift the bodies verbatim" instruction (Task 3) are explicit, not placeholders.

**Type consistency:** `CompiledAllow`/`CompileIntent`/`CompilePolicy`/`StrictestEnforcement`; `ReconcilePolicy`/`FinalizePolicy`/`BackendResult`/`applyBackendResult`; `SegmentationPolicySpec`/`IngressRule`/`NetworkPolicyPeer`/`NetworkPolicyPort`; the `Enforcement`/`EffectiveEnforcement`/`EnforcementSetBy` fields; `EffectiveEnforcement` helper — used consistently across the tasks that define and consume them. Reuses Plan 4's `ResolvedAllow`/`BuildRuleSet`/`BuildRules`/`PolicyClient`/`pce.*` and Plan 3's `ComputeDesiredCWP` without redefining them.
