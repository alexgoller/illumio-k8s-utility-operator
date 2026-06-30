# Track 1 — Configurable Unknown-Label Handling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make an unknown referenced Illumio label a configurable outcome (`strict` reject / `skip` / `create`) instead of always rejecting the whole policy CR.

**Architecture:** A single `unknownLabelMode` is resolved per CR (CR annotation → namespace annotation → `ClusterProfile` default → `strict`) and threaded into the shared policy backend's `resolveAllows`, which both `SegmentationIntent` and `SegmentationPolicy` controllers already call. `skip` omits unresolved consumers and reports them; `create` mints the label (gated to the standard RAEL keys). Outcomes surface on each CR's status.

**Tech Stack:** Go, controller-runtime, kubebuilder, envtest, the existing `internal/pce` client and `internal/controller` policy backend.

## Global Constraints

- Default behaviour is **`strict`** — fully backward-compatible; existing CRs and tests must be unaffected when no mode is set.
- The mode is a **general label-resolution policy** (applies to any consumer reference; provider resolution stays strict for now — provider is the namespace's own managed app, a different failure class).
- `create` may only auto-create the standard Illumio label keys **`role`, `app`, `env`, `loc`**; any other key in `create` mode is a rejection (a typo'd *key* must not mint a junk dimension).
- After any change to `api/v1alpha1`, run `make manifests generate` and copy `config/crd/bases/microsegment.io_*.yaml` to `dist/chart/crds/` (they are kept byte-identical; verify with `diff`).
- `make lint` must report `0 issues` (golangci-lint v2.12.2, includes `goconst` — promote a literal to a constant if it appears 3×).

## File Structure

- `api/v1alpha1/clusterprofile_types.go` — add `UnknownLabelMode` field + mode constants.
- `api/v1alpha1/segmentationintent_types.go` / `segmentationpolicy_types.go` — add status fields.
- `api/v1alpha1/labelmode.go` (new) — mode constants + `AnnotationUnknownLabelMode` + `ResolveUnknownLabelMode` helper (pure, unit-tested).
- `internal/controller/policy_client.go` — add `EnsureLabel` to `PolicyClient`.
- `internal/controller/policybackend.go` — `resolveAllows` gains the mode + returns deferred/created; backend threads mode + populates status.
- `internal/controller/segmentationintent_controller.go` / `segmentationpolicy_controller.go` — pass CR annotations; write new status fields.
- `docs/guides/segmentation-policy.md` — document the modes.

---

### Task 1: `unknownLabelMode` mode constants + `ResolveUnknownLabelMode` helper

**Files:**
- Create: `api/v1alpha1/labelmode.go`
- Test: `api/v1alpha1/labelmode_test.go`

**Interfaces:**
- Produces: `const (UnknownLabelStrict = "strict"; UnknownLabelSkip = "skip"; UnknownLabelCreate = "create")`; `const AnnotationUnknownLabelMode = "microsegment.io/unknown-label-mode"`; `func ResolveUnknownLabelMode(cpDefault string, nsAnnotations, crAnnotations map[string]string) (mode, setBy string)`.

- [ ] **Step 1: Write the failing test**

```go
package v1alpha1

import "testing"

func TestResolveUnknownLabelMode(t *testing.T) {
	cases := []struct {
		name, cpDefault string
		ns, cr          map[string]string
		wantMode, wantBy string
	}{
		{"default empty -> strict", "", nil, nil, UnknownLabelStrict, "default"},
		{"clusterprofile default", "skip", nil, nil, UnknownLabelSkip, "clusterprofile"},
		{"namespace overrides cp", "skip", map[string]string{AnnotationUnknownLabelMode: "create"}, nil, UnknownLabelCreate, "namespace"},
		{"cr overrides namespace", "skip", map[string]string{AnnotationUnknownLabelMode: "create"}, map[string]string{AnnotationUnknownLabelMode: "strict"}, UnknownLabelStrict, "cr"},
		{"invalid value ignored -> falls through", "", nil, map[string]string{AnnotationUnknownLabelMode: "bogus"}, UnknownLabelStrict, "default"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mode, by := ResolveUnknownLabelMode(tc.cpDefault, tc.ns, tc.cr)
			if mode != tc.wantMode || by != tc.wantBy {
				t.Fatalf("got %q/%q, want %q/%q", mode, by, tc.wantMode, tc.wantBy)
			}
		})
	}
}
```

- [ ] **Step 2: Run test, verify it fails**

Run: `go test ./api/v1alpha1/ -run TestResolveUnknownLabelMode -v`
Expected: FAIL (undefined: `ResolveUnknownLabelMode`).

- [ ] **Step 3: Implement**

```go
package v1alpha1

// Unknown-label handling modes (see docs/superpowers/specs/2026-06-30-policy-model-roadmap-design.md, Track 1).
const (
	UnknownLabelStrict = "strict" // reject the whole CR (default)
	UnknownLabelSkip   = "skip"   // omit the unknown actor/rule, report it
	UnknownLabelCreate = "create" // mint the label (standard keys only), then use it
)

// AnnotationUnknownLabelMode overrides the unknown-label mode on a namespace or a policy CR.
const AnnotationUnknownLabelMode = "microsegment.io/unknown-label-mode"

func validMode(m string) bool {
	return m == UnknownLabelStrict || m == UnknownLabelSkip || m == UnknownLabelCreate
}

// ResolveUnknownLabelMode picks the effective mode, most-specific wins:
// CR annotation > namespace annotation > ClusterProfile default > strict.
// setBy is one of "cr", "namespace", "clusterprofile", "default".
func ResolveUnknownLabelMode(cpDefault string, nsAnnotations, crAnnotations map[string]string) (mode, setBy string) {
	if v := crAnnotations[AnnotationUnknownLabelMode]; validMode(v) {
		return v, "cr"
	}
	if v := nsAnnotations[AnnotationUnknownLabelMode]; validMode(v) {
		return v, "namespace"
	}
	if validMode(cpDefault) {
		return cpDefault, "clusterprofile"
	}
	return UnknownLabelStrict, "default"
}
```

- [ ] **Step 4: Run test, verify it passes**

Run: `go test ./api/v1alpha1/ -run TestResolveUnknownLabelMode -v` → PASS.

- [ ] **Step 5: Commit**

```bash
git add api/v1alpha1/labelmode.go api/v1alpha1/labelmode_test.go
git commit -m "feat(api): unknown-label mode constants + most-specific-wins resolver"
```

---

### Task 2: `ClusterProfile.spec.unknownLabelMode` field + CRD regen

**Files:**
- Modify: `api/v1alpha1/clusterprofile_types.go` (next to `ProvisioningMode`, ~line 114)
- Regen: `config/crd/bases/microsegment.io_clusterprofiles.yaml`, `dist/chart/crds/microsegment.io_clusterprofiles.yaml`

**Interfaces:**
- Produces: `ClusterProfileSpec.UnknownLabelMode string` (json `unknownLabelMode`).

- [ ] **Step 1: Add the field** (after `ProvisioningMode`)

```go
	// UnknownLabelMode is the default policy when a referenced Illumio label is
	// not yet in the PCE: strict (reject), skip (omit the actor/rule and report),
	// or create (mint role/app/env/loc labels). Overridable per-namespace and
	// per-CR via the microsegment.io/unknown-label-mode annotation. Defaults to strict.
	// +kubebuilder:validation:Enum=strict;skip;create
	// +optional
	UnknownLabelMode string `json:"unknownLabelMode,omitempty"`
```

- [ ] **Step 2: Regenerate manifests and sync the chart CRD**

```bash
make manifests generate
/bin/cp -f config/crd/bases/microsegment.io_clusterprofiles.yaml dist/chart/crds/microsegment.io_clusterprofiles.yaml
diff -q config/crd/bases/microsegment.io_clusterprofiles.yaml dist/chart/crds/microsegment.io_clusterprofiles.yaml && echo "in sync"
```
Expected: `in sync`, and `git diff` shows `unknownLabelMode` with `enum: [strict, skip, create]` in both CRD files.

- [ ] **Step 3: Build**

Run: `go build ./...` → no output (success).

- [ ] **Step 4: Commit**

```bash
git add api/v1alpha1/clusterprofile_types.go config/crd/bases/ dist/chart/crds/
git commit -m "feat(api): ClusterProfile.spec.unknownLabelMode (default strict)"
```

---

### Task 3: `PolicyClient.EnsureLabel` + auto-creatable-key guard

**Files:**
- Modify: `internal/controller/policy_client.go`
- Modify: `internal/controller/suite_test.go` (fake `fakePolicyClient` — add `EnsureLabel`)
- Create: `internal/controller/labelcreate.go` + `labelcreate_test.go`

**Interfaces:**
- Produces: `PolicyClient.EnsureLabel(ctx, key, value string, owner pce.Owner) (*pce.Label, error)`; `func autoCreatableKey(key string) bool`.

- [ ] **Step 1: Add `EnsureLabel` to the interface** (`policy_client.go`, inside `PolicyClient`)

```go
	EnsureLabel(ctx context.Context, key, value string, owner pce.Owner) (*pce.Label, error)
```
(`*pce.Client` already implements `EnsureLabel` — see `internal/pce/labels.go` — so the `var _ PolicyClient = (*pce.Client)(nil)` assertion still holds.)

- [ ] **Step 2: Add `EnsureLabel` to the test fake** (`suite_test.go`, on `fakePolicyClient`)

```go
func (fakePolicyClient) EnsureLabel(_ context.Context, key, value string, _ pce.Owner) (*pce.Label, error) {
	return &pce.Label{Href: "/orgs/1/labels/created-" + key + "-" + value, Key: key, Value: value}, nil
}
```

- [ ] **Step 3: Write the failing test for the key guard** (`labelcreate_test.go`)

```go
package controller

import "testing"

func TestAutoCreatableKey(t *testing.T) {
	for _, k := range []string{"role", "app", "env", "loc"} {
		if !autoCreatableKey(k) {
			t.Errorf("%q should be auto-creatable", k)
		}
	}
	for _, k := range []string{"custom", "ap", "team", ""} {
		if autoCreatableKey(k) {
			t.Errorf("%q must NOT be auto-creatable", k)
		}
	}
}
```

- [ ] **Step 4: Run, verify fail**

Run: `go test ./internal/controller/ -run TestAutoCreatableKey` → FAIL (undefined).

- [ ] **Step 5: Implement** (`labelcreate.go`)

```go
package controller

// autoCreatableKeys are the standard Illumio label dimensions the operator may
// mint in unknown-label "create" mode. Any other key is rejected so a typo'd key
// cannot spawn a junk label dimension.
var autoCreatableKeys = map[string]bool{"role": true, "app": true, "env": true, "loc": true}

func autoCreatableKey(key string) bool { return autoCreatableKeys[key] }
```

- [ ] **Step 6: Run, verify pass; build**

Run: `go test ./internal/controller/ -run TestAutoCreatableKey` → PASS.
Run: `go build ./...` → success.

- [ ] **Step 7: Commit**

```bash
git add internal/controller/policy_client.go internal/controller/suite_test.go internal/controller/labelcreate.go internal/controller/labelcreate_test.go
git commit -m "feat(controller): PolicyClient.EnsureLabel + auto-creatable-key guard"
```

---

### Task 4: `resolveAllows` honours the mode (strict/skip/create)

**Files:**
- Modify: `internal/controller/policybackend.go` (`resolveAllows`, ~line 312)
- Test: `internal/controller/policybackend_labelmode_test.go` (new)

**Interfaces:**
- Consumes: `PolicyClient.FindLabel`, `PolicyClient.EnsureLabel`, `autoCreatableKey`, `microv1.UnknownLabel*`.
- Produces: `func resolveAllows(ctx, allows []CompiledAllow, pclient PolicyClient, mode string, owner pce.Owner) (res []ResolvedAllow, deferred, created []string, reason, msg string, ok bool, err error)`.

- [ ] **Step 1: Write failing tests** (table over modes; uses a local stub PolicyClient whose `FindLabel` returns `ErrLabelNotFound` for `app=missing`, found otherwise)

```go
package controller

import (
	"context"
	"testing"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

type stubLabelClient struct{ fakePolicyClient } // embeds the suite fake; override FindLabel
func (stubLabelClient) FindLabel(_ context.Context, key, value string) (*pce.Label, error) {
	if value == "missing" {
		return nil, pce.ErrLabelNotFound
	}
	return &pce.Label{Href: "/h/" + key + "-" + value, Key: key, Value: value}, nil
}

func TestResolveAllows_Modes(t *testing.T) {
	allows := []CompiledAllow{
		{From: map[string]string{"app": "known"}},
		{From: map[string]string{"app": "missing"}},
	}
	ctx := context.Background()
	c := stubLabelClient{}

	// strict: unknown -> not ok
	if _, _, _, _, _, ok, _ := resolveAllows(ctx, allows, c, microv1.UnknownLabelStrict, pce.Owner{}); ok {
		t.Fatal("strict must reject on unknown label")
	}
	// skip: ok, the unknown allow is dropped, deferred lists it
	res, deferred, _, _, _, ok, _ := resolveAllows(ctx, allows, c, microv1.UnknownLabelSkip, pce.Owner{})
	if !ok || len(res) != 1 || len(deferred) != 1 || deferred[0] != "app=missing" {
		t.Fatalf("skip: res=%d deferred=%v ok=%v", len(res), deferred, ok)
	}
	// create: ok, the missing label is minted, created lists it, both allows kept
	res, _, created, _, _, ok, _ := resolveAllows(ctx, allows, c, microv1.UnknownLabelCreate, pce.Owner{})
	if !ok || len(res) != 2 || len(created) != 1 || created[0] != "app=missing" {
		t.Fatalf("create: res=%d created=%v ok=%v", len(res), created, ok)
	}
	// create with a non-standard key -> reject
	bad := []CompiledAllow{{From: map[string]string{"team": "missing"}}}
	if _, _, _, _, _, ok, _ := resolveAllows(ctx, bad, c, microv1.UnknownLabelCreate, pce.Owner{}); ok {
		t.Fatal("create must reject auto-creating a non-standard key")
	}
}
```

- [ ] **Step 2: Run, verify fail**

Run: `go test ./internal/controller/ -run TestResolveAllows_Modes` → FAIL (signature mismatch / undefined behavior).

- [ ] **Step 3: Reimplement `resolveAllows`**

```go
// resolveAllows resolves consumer labels per the unknown-label mode. Returns the
// resolved allows, the "key=value" labels deferred (skip) or created (create),
// and a rejection (ok=false) for strict-mode unknowns or create of a non-standard key.
func resolveAllows(ctx context.Context, allows []CompiledAllow, pclient PolicyClient, mode string, owner pce.Owner) ([]ResolvedAllow, []string, []string, string, string, bool, error) {
	out := make([]ResolvedAllow, 0, len(allows))
	var deferred, created []string
	for _, a := range allows {
		consumerHrefs := make([]string, 0, len(a.From))
		skippedAny := false
		for key, value := range a.From {
			lbl, err := pclient.FindLabel(ctx, key, value)
			if err == nil {
				consumerHrefs = append(consumerHrefs, lbl.Href)
				continue
			}
			if !errors.Is(err, pce.ErrLabelNotFound) {
				return nil, nil, nil, "", "", false, err
			}
			kv := key + "=" + value
			switch mode {
			case microv1.UnknownLabelSkip:
				deferred = append(deferred, kv)
				skippedAny = true
			case microv1.UnknownLabelCreate:
				if !autoCreatableKey(key) {
					return nil, nil, nil, microv1.ReasonRejected,
						fmt.Sprintf("cannot auto-create label with non-standard key %q (create mode allows role/app/env/loc only); pre-create %s in the PCE", key, kv), false, nil
				}
				lbl, cerr := pclient.EnsureLabel(ctx, key, value, owner)
				if cerr != nil {
					return nil, nil, nil, "", "", false, cerr
				}
				consumerHrefs = append(consumerHrefs, lbl.Href)
				created = append(created, kv)
			default: // strict
				return nil, nil, nil, microv1.ReasonRejected, fmt.Sprintf("no Illumio label %s in the PCE", kv), false, nil
			}
		}
		// In skip mode, drop an allow whose consumers were all unresolved.
		if skippedAny && len(consumerHrefs) == 0 {
			continue
		}
		out = append(out, ResolvedAllow{ConsumerHrefs: consumerHrefs, Ports: a.Ports})
	}
	return out, deferred, created, "", "", true, nil
}
```

- [ ] **Step 4: Run, verify pass**

Run: `go test ./internal/controller/ -run TestResolveAllows_Modes -v` → PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/controller/policybackend.go internal/controller/policybackend_labelmode_test.go
git commit -m "feat(controller): resolveAllows honours strict/skip/create unknown-label modes"
```

---

### Task 5: Status fields on both policy CRDs

**Files:**
- Modify: `api/v1alpha1/segmentationintent_types.go` (`SegmentationIntentStatus`)
- Modify: `api/v1alpha1/segmentationpolicy_types.go` (`SegmentationPolicyStatus`)
- Regen: `config/crd/bases/*segmentation*.yaml` + `dist/chart/crds/*segmentation*.yaml`

**Interfaces:**
- Produces, on both status structs: `UnknownLabelMode string`, `UnknownLabelModeSetBy string`, `DeferredLabels []string`, `CreatedLabels []string` (all `+optional`, json `unknownLabelMode`/`unknownLabelModeSetBy`/`deferredLabels`/`createdLabels`).

- [ ] **Step 1: Add the four fields to each status struct**

```go
	// UnknownLabelMode is the effective mode used to resolve referenced labels.
	// +optional
	UnknownLabelMode string `json:"unknownLabelMode,omitempty"`
	// UnknownLabelModeSetBy names where the mode came from (cr|namespace|clusterprofile|default).
	// +optional
	UnknownLabelModeSetBy string `json:"unknownLabelModeSetBy,omitempty"`
	// DeferredLabels are key=value consumer labels skipped because they do not yet exist (skip mode).
	// +optional
	DeferredLabels []string `json:"deferredLabels,omitempty"`
	// CreatedLabels are key=value labels minted while resolving (create mode).
	// +optional
	CreatedLabels []string `json:"createdLabels,omitempty"`
```

- [ ] **Step 2: Regenerate + sync chart CRDs**

```bash
make manifests generate
for f in microsegment.io_segmentationintents.yaml microsegment.io_segmentationpolicies.yaml; do /bin/cp -f config/crd/bases/$f dist/chart/crds/$f; done
git diff --stat config/crd/bases dist/chart/crds
```
Expected: both segmentation CRDs show the four new status properties.

- [ ] **Step 3: Build** → `go build ./...` success.

- [ ] **Step 4: Commit**

```bash
git add api/v1alpha1/segmentation*_types.go api/v1alpha1/zz_generated.deepcopy.go config/crd/bases/ dist/chart/crds/
git commit -m "feat(api): unknown-label status fields on SegmentationIntent/Policy"
```

---

### Task 6: Thread the mode through the backend + both controllers; write status

**Files:**
- Modify: `internal/controller/policybackend.go` (backend entry ~line 112–143; `BackendResult`)
- Modify: `internal/controller/segmentationintent_controller.go` (~line 59) and `segmentationpolicy_controller.go` (~line 56)
- Test: `internal/controller/clusterprofile_cwp_test.go` neighbours — add an envtest in `segmentationintent_controller_test.go`

**Interfaces:**
- Consumes: `microv1.ResolveUnknownLabelMode`, the new `resolveAllows` signature, the new status fields.
- `BackendResult` gains: `UnknownLabelMode, UnknownLabelModeSetBy string`, `DeferredLabels, CreatedLabels []string` so each controller copies them onto its CR status.

- [ ] **Step 1: Resolve the mode in the backend and pass it down.** Where the backend currently calls `resolveAllows(ctx, allows, pclient)` (~line 130), resolve the mode first and capture the new returns:

```go
	mode, setBy := microv1.ResolveUnknownLabelMode(cp.Spec.UnknownLabelMode, nsAnnotations, crAnnotations)
	resolved, deferred, created, reason, msg, ok, err := resolveAllows(ctx, allows, pclient, mode, owner)
```
Add `nsAnnotations, crAnnotations map[string]string` parameters to the backend entry function and populate `BackendResult.UnknownLabelMode = mode`, `…SetBy = setBy`, `…DeferredLabels = deferred`, `…CreatedLabels = created` on every return path that builds a result (including the success path). The namespace annotations are already fetched for enforcement (~line 106) — reuse them.

- [ ] **Step 2: Pass CR annotations from each controller.** In `segmentationintent_controller.go` (~line 59) and `segmentationpolicy_controller.go` (~line 56), pass `si.Annotations` / `sp.Annotations` into the backend call, and after it returns copy the four fields onto `si.Status` / `sp.Status` before the status update.

- [ ] **Step 3: Write an envtest** (`segmentationintent_controller_test.go`) — apply a `ClusterProfile` with `spec.unknownLabelMode: skip`, a managed namespace, and an intent whose `from` references a non-existent label; assert `Ready=True`, `status.deferredLabels` contains the missing `key=value`, and no ruleset rule was created for it. (Follow the existing envtest harness in `clusterprofile_cwp_test.go` — ready PCEConnection + fake clients.)

- [ ] **Step 4: Run the controller suite**

Run: `go test ./internal/controller/ -run TestControllers -v` (or the package's Ginkgo entry) → PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/controller/policybackend.go internal/controller/segmentationintent_controller.go internal/controller/segmentationpolicy_controller.go internal/controller/segmentationintent_controller_test.go
git commit -m "feat(controller): thread unknown-label mode to status on both policy CRDs"
```

---

### Task 7: Documentation

**Files:**
- Modify: `docs/guides/segmentation-policy.md` (the "Guardrails" / "How compilation works" sections)

- [ ] **Step 1: Document the three modes**, the precedence (CR → namespace → ClusterProfile default → strict), the `microsegment.io/unknown-label-mode` annotation, the `create` standard-keys-only rule, and the new status fields (`deferredLabels`, `createdLabels`, `unknownLabelMode`, `unknownLabelModeSetBy`). Update the existing "unknown label → Rejected" guardrail text to say that is the **`strict`** (default) behaviour, with `skip`/`create` as alternatives.

- [ ] **Step 2: Commit**

```bash
git add docs/guides/segmentation-policy.md
git commit -m "docs: document configurable unknown-label handling"
```

---

## Final verification

- [ ] `make manifests generate` → no further diff (generated code in sync).
- [ ] `diff -q` each `config/crd/bases/*.yaml` against its `dist/chart/crds/*.yaml` → identical.
- [ ] `go test ./...` → all pass.
- [ ] `make lint` → `0 issues`.
- [ ] Bump `dist/chart/Chart.yaml` to the next patch version, open a PR.

## Self-review notes

- **Spec coverage:** modes (T1,4), precedence (T1), ClusterProfile default (T2), annotation override (T1+T6), create key-allowlist (T3,4), status surface (T5,6), docs (T7). Provider-side resolution intentionally stays `strict` (provider = the namespace's own managed app; a missing provider label is a different, admin-side failure — out of scope, noted in the spec).
- **Type consistency:** `resolveAllows` new signature `(…, mode string, owner pce.Owner) ([]ResolvedAllow, []string, []string, string, string, bool, error)` is consumed only by the backend (Task 6) which is updated in lockstep.
- **Backward compatibility:** empty `unknownLabelMode` + no annotations → `ResolveUnknownLabelMode` returns `strict` → identical to today; existing tests unchanged.
