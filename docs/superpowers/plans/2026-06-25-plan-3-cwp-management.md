# Illumio K8s Utility Operator — Plan 3: Container Workload Profile (CWP) Management

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Automate per-namespace Container Workload Profile (CWP) configuration — the tedious work of marking namespaces `managed`, assigning Illumio labels, and setting enforcement, especially for the many OpenShift/Kubernetes system namespaces. Driven by ordered `namespaceRules` (+ a system-namespace convenience) on `ClusterProfile`, with per-namespace annotation overrides, reconciled into the PCE via the CWP REST API.

**Architecture:** Extends the Plan 2 onboarding operator. `internal/pce` gains a CWP client (list + update, with label assignment). `ClusterProfile` gains `namespaceRules` + `systemNamespaces` spec and a `managedNamespaces` status. A pure matching function computes the desired CWP for a namespace from rules/annotations. The existing **ClusterProfile controller** is extended: after onboarding, it lists Kubernetes namespaces + the cluster's CWPs and reconciles each CWP (single status writer, no multi-controller races); it watches `Namespace` objects to re-reconcile on change. Kubelink still *creates* the CWP objects; this operator *configures* them.

**Tech Stack:** Go, kubebuilder v4 + controller-runtime, envtest, `net/http/httptest`; MkDocs for docs.

## Global Constraints

- **Module path:** `github.com/alexgoller/illumio-k8s-utility-operator`. **API group:** `microsegment.io`; **version:** `v1alpha1`.
- **Target platform:** Illumio Core for Kubernetes in **CLAS** mode, **PCE 24.5+**.
- **CWP API facts (verified against PCE 24.5 REST / illumio-py / terraform-provider):**
  - List: `GET /orgs/{org}/container_clusters/{cluster_id}/container_workload_profiles` → array of `{href, name, namespace, managed, enforcement_mode, visibility_level, labels}`. The default profile has `namespace: null`.
  - Update one: `PUT <profile-href>` (the href is `/orgs/{org}/container_clusters/{cid}/container_workload_profiles/{pid}`, already without the `/api/v2` prefix) → 204. Send only the fields to change.
  - Label assignment shape: `labels: [ { "key": "role", "assignment": { "href": "/orgs/N/labels/M" } } ]` (one `assignment` object per fixed label; `restriction: [{href}]` is the allow-list form, not used here). `assign_labels` is the deprecated form — use `labels`.
  - `enforcement_mode` is a **string enum**; container-supported values: `idle`, `visibility_only`, `full` (`selective` is unsupported for containers).
  - **No `sec_policy` provisioning** is needed for CWP updates — they apply immediately. (Only creating brand-new *labels* needs provisioning; assigning an existing label's href to a CWP does not — and `EnsureLabel` from Plan 1 handles label existence.)
- **Ownership tagging:** any *labels* the operator creates via `EnsureLabel` carry `external_data_set`/`external_data_reference`. (CWP objects themselves are created by Kubelink, not us; we only update them.)
- **Single ClusterProfile assumption:** one `ClusterProfile` governs this cluster's namespaces (it maps to the one PCE Container Cluster). The CWP reconcile keys off that ClusterProfile.
- **Commits:** conventional-commit messages; commit at the end of every task; end messages with the repo's `Co-Authored-By` trailer.

---

### Task 1: PCE Container Workload Profile client

**Files:**
- Create: `internal/pce/cwp.go`
- Test: `internal/pce/cwp_test.go`

**Interfaces:**
- Consumes: `Client.do`, `Client.orgPath`, `LabelRef` (Plan 2).
- Produces:
  - `type CWPLabel struct { Key string; Assignment *LabelRef; Restriction []LabelRef }` (json `key`, `assignment`, `restriction`)
  - `type ContainerWorkloadProfile struct { Href, Name, Namespace, EnforcementMode, VisibilityLevel string; Managed bool; Labels []CWPLabel }`
  - `type CWPUpdate struct { Managed *bool; EnforcementMode string; Labels []CWPLabel }` (json `managed,omitempty`, `enforcement_mode,omitempty`, `labels,omitempty`)
  - `func (c *Client) ListContainerWorkloadProfiles(ctx context.Context, clusterID string) ([]ContainerWorkloadProfile, error)`
  - `func (c *Client) UpdateContainerWorkloadProfile(ctx context.Context, profileHref string, update CWPUpdate) error`

- [ ] **Step 1: Write the failing test**

Create `internal/pce/cwp_test.go`:

```go
package pce

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestListContainerWorkloadProfiles(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/orgs/7/container_clusters/cid-1/container_workload_profiles" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[
			{"href":"/orgs/7/container_clusters/cid-1/container_workload_profiles/p1","namespace":"payments","managed":true,"enforcement_mode":"visibility_only"},
			{"href":"/orgs/7/container_clusters/cid-1/container_workload_profiles/p0","namespace":null,"managed":false}
		]`))
	})
	got, err := c.ListContainerWorkloadProfiles(context.Background(), "cid-1")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(got) != 2 || got[0].Namespace != "payments" || !got[0].Managed {
		t.Fatalf("got = %+v", got)
	}
}

func TestUpdateContainerWorkloadProfile_PutsFieldsToHref(t *testing.T) {
	var body CWPUpdate
	var gotMethod, gotPath string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusNoContent)
	})
	managed := true
	href := "/orgs/7/container_clusters/cid-1/container_workload_profiles/p1"
	err := c.UpdateContainerWorkloadProfile(context.Background(), href, CWPUpdate{
		Managed:         &managed,
		EnforcementMode: "visibility_only",
		Labels: []CWPLabel{
			{Key: "role", Assignment: &LabelRef{Href: "/orgs/7/labels/5"}},
		},
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %s, want PUT", gotMethod)
	}
	if gotPath != "/api/v2"+href {
		t.Errorf("path = %q, want %q", gotPath, "/api/v2"+href)
	}
	if body.Managed == nil || !*body.Managed || body.EnforcementMode != "visibility_only" {
		t.Errorf("body = %+v", body)
	}
	if len(body.Labels) != 1 || body.Labels[0].Key != "role" || body.Labels[0].Assignment.Href != "/orgs/7/labels/5" {
		t.Errorf("body labels = %+v", body.Labels)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/pce/ -run "ContainerWorkloadProfile" -v`
Expected: compile failure — `undefined: ContainerWorkloadProfile`, `undefined: CWPUpdate`.

- [ ] **Step 3: Write the implementation**

Create `internal/pce/cwp.go`:

```go
package pce

import (
	"context"
	"net/http"
)

// CWPLabel is a label assignment on a Container Workload Profile. Set exactly
// one of Assignment (a fixed label) or Restriction (an allow-list).
type CWPLabel struct {
	Key         string     `json:"key"`
	Assignment  *LabelRef  `json:"assignment,omitempty"`
	Restriction []LabelRef `json:"restriction,omitempty"`
}

// ContainerWorkloadProfile is a per-namespace policy/label profile in the PCE.
// Kubelink creates one per namespace; this operator updates them.
type ContainerWorkloadProfile struct {
	Href            string     `json:"href,omitempty"`
	Name            string     `json:"name,omitempty"`
	Namespace       string     `json:"namespace,omitempty"`
	Managed         bool       `json:"managed"`
	EnforcementMode string     `json:"enforcement_mode,omitempty"`
	VisibilityLevel string     `json:"visibility_level,omitempty"`
	Labels          []CWPLabel `json:"labels,omitempty"`
}

// CWPUpdate is the body of a CWP update; only set fields are changed.
type CWPUpdate struct {
	Managed         *bool      `json:"managed,omitempty"`
	EnforcementMode string     `json:"enforcement_mode,omitempty"`
	Labels          []CWPLabel `json:"labels,omitempty"`
}

// ListContainerWorkloadProfiles lists the CWPs for a container cluster.
func (c *Client) ListContainerWorkloadProfiles(ctx context.Context, clusterID string) ([]ContainerWorkloadProfile, error) {
	var out []ContainerWorkloadProfile
	path := c.orgPath("/container_clusters/" + clusterID + "/container_workload_profiles")
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateContainerWorkloadProfile PUTs an update to the CWP identified by its
// href (an href like /orgs/N/container_clusters/C/container_workload_profiles/P).
func (c *Client) UpdateContainerWorkloadProfile(ctx context.Context, profileHref string, update CWPUpdate) error {
	return c.do(ctx, http.MethodPut, profileHref, update, nil)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/pce/ -v`
Expected: PASS (all pce tests, including the two new CWP tests).

- [ ] **Step 5: Commit**

```bash
git add internal/pce/cwp.go internal/pce/cwp_test.go
git commit -m "feat(pce): add container-workload-profile client (list/update)"
```

---

### Task 2: ClusterProfile namespaceRules + systemNamespaces API

**Files:**
- Modify: `api/v1alpha1/clusterprofile_types.go` (add new types/fields)
- Test: `api/v1alpha1/clusterprofile_namespacerules_test.go`
- Generated: `zz_generated.deepcopy.go`, `config/crd/bases/microsegment.io_clusterprofiles.yaml`, RBAC.

**Interfaces:**
- Consumes: existing `ClusterProfileSpec`/`ClusterProfileStatus` (Plan 2).
- Produces:
  - `NamespaceMatch{ NamePattern string; Labels map[string]string }`
  - `LabelAssignment{ Value string; FromNamespaceLabel string }`
  - `NamespaceRule{ Match NamespaceMatch; Managed bool; AssignLabels map[string]LabelAssignment; EnforcementMode string }`
  - `SystemNamespacesSpec{ Manage bool; Patterns []string; Labels map[string]string; EnforcementMode string }`
  - `ClusterProfileSpec` gains `NamespaceRules []NamespaceRule` and `SystemNamespaces SystemNamespacesSpec`
  - `ClusterProfileStatus` gains `ManagedNamespaces int`
  - annotation constants: `AnnotationManaged = "microsegment.io/managed"`, `AnnotationEnforcement = "microsegment.io/enforcement"`, `AnnotationLabelPrefix = "microsegment.io/label."`

- [ ] **Step 1: Write the failing test**

Create `api/v1alpha1/clusterprofile_namespacerules_test.go`:

```go
package v1alpha1

import "testing"

func TestNamespaceRule_Shape(t *testing.T) {
	cp := ClusterProfile{
		Spec: ClusterProfileSpec{
			SystemNamespaces: SystemNamespacesSpec{
				Manage:          true,
				Labels:          map[string]string{"role": "control"},
				EnforcementMode: "visibility_only",
			},
			NamespaceRules: []NamespaceRule{
				{
					Match:           NamespaceMatch{NamePattern: "payments"},
					Managed:         true,
					AssignLabels:    map[string]LabelAssignment{"env": {Value: "prod"}, "app": {FromNamespaceLabel: "app.kubernetes.io/part-of"}},
					EnforcementMode: "full",
				},
			},
		},
	}
	if cp.Spec.NamespaceRules[0].AssignLabels["env"].Value != "prod" {
		t.Errorf("env value = %q", cp.Spec.NamespaceRules[0].AssignLabels["env"].Value)
	}
	if AnnotationManaged != "microsegment.io/managed" {
		t.Errorf("AnnotationManaged = %q", AnnotationManaged)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./api/v1alpha1/ -run TestNamespaceRule -v`
Expected: compile failure — `undefined: NamespaceRule`, etc.

- [ ] **Step 3: Add the types and fields**

In `api/v1alpha1/clusterprofile_types.go`, add these types (near `OnboardingSpec`):

```go
// NamespaceMatch selects namespaces by name glob and/or required k8s labels.
type NamespaceMatch struct {
	// NamePattern is a glob (path.Match syntax, e.g. "openshift-*"). Empty matches any name.
	// +optional
	NamePattern string `json:"namePattern,omitempty"`
	// Labels that must all be present on the namespace (subset match).
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// LabelAssignment assigns an Illumio label value: a fixed Value, or a value
// read from one of the namespace's own k8s labels.
type LabelAssignment struct {
	// +optional
	Value string `json:"value,omitempty"`
	// +optional
	FromNamespaceLabel string `json:"fromNamespaceLabel,omitempty"`
}

// NamespaceRule maps matching namespaces to a desired CWP configuration.
type NamespaceRule struct {
	Match NamespaceMatch `json:"match"`
	// Managed marks the namespace's CWP as PCE-managed.
	Managed bool `json:"managed"`
	// AssignLabels maps Illumio label keys (role/app/env/loc/custom) to values.
	// +optional
	AssignLabels map[string]LabelAssignment `json:"assignLabels,omitempty"`
	// EnforcementMode for the namespace. One of idle, visibility_only, full.
	// +kubebuilder:validation:Enum=idle;visibility_only;full
	// +optional
	EnforcementMode string `json:"enforcementMode,omitempty"`
}

// SystemNamespacesSpec is a convenience to manage the cluster's system
// namespaces (OpenShift/Kubernetes) out of the box. User NamespaceRules take
// precedence over these defaults.
type SystemNamespacesSpec struct {
	// Manage turns on management of system namespaces.
	// +optional
	Manage bool `json:"manage,omitempty"`
	// Patterns of system namespace name globs. Defaults (when empty) to:
	// openshift-*, kube-*, default, kube-system, kube-public, kube-node-lease.
	// +optional
	Patterns []string `json:"patterns,omitempty"`
	// Labels assigned to system-namespace CWPs.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// EnforcementMode for system namespaces. Defaults to idle.
	// +kubebuilder:validation:Enum=idle;visibility_only;full
	// +optional
	EnforcementMode string `json:"enforcementMode,omitempty"`
}
```

Add fields to `ClusterProfileSpec` (after `ProvisioningMode`):

```go
	// SystemNamespaces manages OpenShift/Kubernetes system namespaces out of the box.
	// +optional
	SystemNamespaces SystemNamespacesSpec `json:"systemNamespaces,omitempty"`
	// NamespaceRules are evaluated in order; the first match wins. They take
	// precedence over SystemNamespaces.
	// +optional
	NamespaceRules []NamespaceRule `json:"namespaceRules,omitempty"`
```

Add to `ClusterProfileStatus` (after `ContainerClusterID`):

```go
	// ManagedNamespaces is the number of namespaces whose CWP is managed.
	// +optional
	ManagedNamespaces int `json:"managedNamespaces,omitempty"`
```

Add the annotation constants (near the condition constants):

```go
// Namespace annotation keys for per-namespace CWP overrides.
const (
	AnnotationManaged     = "microsegment.io/managed"      // "true"/"false"
	AnnotationEnforcement = "microsegment.io/enforcement"  // idle|visibility_only|full
	AnnotationLabelPrefix = "microsegment.io/label."       // e.g. microsegment.io/label.env=prod
)
```

Add a printcolumn for managed count to the `ClusterProfile` marker block:

```go
// +kubebuilder:printcolumn:name="Managed-NS",type=integer,JSONPath=`.status.managedNamespaces`
```

- [ ] **Step 4: Regenerate and verify**

```bash
make generate
make manifests
go test ./api/v1alpha1/ -run TestNamespaceRule -v
make build
```

Expected: deepcopy regenerated for the new types (incl. the maps and slices); CRD manifest gains `namespaceRules`/`systemNamespaces`/`managedNamespaces`; test PASS; build clean.

- [ ] **Step 5: Commit**

```bash
git add api/v1alpha1/ config/crd/ config/rbac/
git commit -m "feat(api): add namespaceRules, systemNamespaces, managedNamespaces to ClusterProfile"
```

---

### Task 3: Desired-CWP matching logic (pure function)

**Files:**
- Create: `internal/controller/cwpmatch.go`
- Test: `internal/controller/cwpmatch_test.go`

**Interfaces:**
- Consumes: `NamespaceRule`, `NamespaceMatch`, `LabelAssignment`, `SystemNamespacesSpec`, annotation constants (Task 2).
- Produces:
  - `type DesiredCWP struct { Managed bool; Labels map[string]string; EnforcementMode string }`
  - `func ComputeDesiredCWP(name string, nsLabels, nsAnnotations map[string]string, rules []microv1.NamespaceRule, sys microv1.SystemNamespacesSpec) DesiredCWP`

**Behavior:** when `systemNamespaces.manage` is on, namespaces matching system patterns always get the `systemNamespaces` config (it overrides user rules for them); **user `namespaceRules` (first match) govern all other namespaces**; then **per-namespace annotations override** the result. Default enforcement for a managed namespace with no enforcement set is `idle`.

- [ ] **Step 1: Write the failing test**

Create `internal/controller/cwpmatch_test.go`:

```go
package controller

import (
	"testing"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
)

func TestComputeDesiredCWP(t *testing.T) {
	sys := microv1.SystemNamespacesSpec{
		Manage: true, Labels: map[string]string{"role": "control"}, EnforcementMode: "visibility_only",
	}
	rules := []microv1.NamespaceRule{
		{
			Match:        microv1.NamespaceMatch{NamePattern: "payments"},
			Managed:      true,
			AssignLabels: map[string]microv1.LabelAssignment{"env": {Value: "prod"}, "app": {FromNamespaceLabel: "app.kubernetes.io/part-of"}},
			EnforcementMode: "full",
		},
		{Match: microv1.NamespaceMatch{NamePattern: "*"}, Managed: false},
	}

	tests := []struct {
		name    string
		nsName  string
		labels  map[string]string
		annos   map[string]string
		want    DesiredCWP
	}{
		{
			name:   "system namespace gets system defaults",
			nsName: "openshift-monitoring",
			want:   DesiredCWP{Managed: true, Labels: map[string]string{"role": "control"}, EnforcementMode: "visibility_only"},
		},
		{
			name:   "user rule wins over system + resolves fromNamespaceLabel",
			nsName: "payments",
			labels: map[string]string{"app.kubernetes.io/part-of": "checkout"},
			want:   DesiredCWP{Managed: true, Labels: map[string]string{"env": "prod", "app": "checkout"}, EnforcementMode: "full"},
		},
		{
			name:   "non-system, catch-all unmanaged",
			nsName: "team-a",
			want:   DesiredCWP{Managed: false, Labels: map[string]string{}, EnforcementMode: ""},
		},
		{
			name:   "annotation overrides managed + enforcement + label",
			nsName: "team-a",
			annos:  map[string]string{microv1.AnnotationManaged: "true", microv1.AnnotationEnforcement: "idle", microv1.AnnotationLabelPrefix + "env": "dev"},
			want:   DesiredCWP{Managed: true, Labels: map[string]string{"env": "dev"}, EnforcementMode: "idle"},
		},
		{
			name:   "managed with no enforcement defaults to idle",
			nsName: "team-a",
			annos:  map[string]string{microv1.AnnotationManaged: "true"},
			want:   DesiredCWP{Managed: true, Labels: map[string]string{}, EnforcementMode: "idle"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeDesiredCWP(tc.nsName, tc.labels, tc.annos, rules, sys)
			if got.Managed != tc.want.Managed || got.EnforcementMode != tc.want.EnforcementMode {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
			if len(got.Labels) != len(tc.want.Labels) {
				t.Fatalf("labels got %+v, want %+v", got.Labels, tc.want.Labels)
			}
			for k, v := range tc.want.Labels {
				if got.Labels[k] != v {
					t.Fatalf("label %s got %q want %q", k, got.Labels[k], v)
				}
			}
		})
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/controller/ -run TestComputeDesiredCWP -v`
Expected: compile failure — `undefined: ComputeDesiredCWP`, `undefined: DesiredCWP`.

- [ ] **Step 3: Write the implementation**

Create `internal/controller/cwpmatch.go`:

```go
package controller

import (
	"path"
	"strings"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
)

// DesiredCWP is the computed desired Container Workload Profile config for a namespace.
type DesiredCWP struct {
	Managed         bool
	Labels          map[string]string // Illumio label key -> value
	EnforcementMode string
}

var defaultSystemPatterns = []string{
	"openshift-*", "kube-*", "default", "kube-system", "kube-public", "kube-node-lease",
}

// ComputeDesiredCWP resolves the desired CWP for a namespace. Precedence:
// user rules (first match) > systemNamespaces > unmanaged default; then
// per-namespace annotations override the result.
func ComputeDesiredCWP(name string, nsLabels, nsAnnotations map[string]string, rules []microv1.NamespaceRule, sys microv1.SystemNamespacesSpec) DesiredCWP {
	d := DesiredCWP{Managed: false, Labels: map[string]string{}, EnforcementMode: ""}

	// 1. System-namespace defaults (lowest precedence of the matchers).
	if sys.Manage && matchesAnyPattern(name, systemPatterns(sys)) {
		d.Managed = true
		d.Labels = copyLabels(sys.Labels)
		d.EnforcementMode = sys.EnforcementMode
	}

	// 2. First matching user rule overrides the base.
	for i := range rules {
		if ruleMatches(rules[i].Match, name, nsLabels) {
			d.Managed = rules[i].Managed
			d.Labels = resolveAssignLabels(rules[i].AssignLabels, nsLabels)
			d.EnforcementMode = rules[i].EnforcementMode
			break
		}
	}

	// 3. Annotation overrides.
	applyAnnotationOverrides(&d, nsAnnotations)

	// 4. Default enforcement for managed namespaces.
	if d.Managed && d.EnforcementMode == "" {
		d.EnforcementMode = "idle"
	}
	return d
}

func systemPatterns(sys microv1.SystemNamespacesSpec) []string {
	if len(sys.Patterns) > 0 {
		return sys.Patterns
	}
	return defaultSystemPatterns
}

func matchesAnyPattern(name string, patterns []string) bool {
	for _, p := range patterns {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}

func ruleMatches(m microv1.NamespaceMatch, name string, nsLabels map[string]string) bool {
	if m.NamePattern != "" {
		if ok, _ := path.Match(m.NamePattern, name); !ok {
			return false
		}
	}
	for k, v := range m.Labels {
		if nsLabels[k] != v {
			return false
		}
	}
	return true
}

func resolveAssignLabels(assign map[string]microv1.LabelAssignment, nsLabels map[string]string) map[string]string {
	out := map[string]string{}
	for key, a := range assign {
		switch {
		case a.Value != "":
			out[key] = a.Value
		case a.FromNamespaceLabel != "":
			if v, ok := nsLabels[a.FromNamespaceLabel]; ok && v != "" {
				out[key] = v
			}
		}
	}
	return out
}

func applyAnnotationOverrides(d *DesiredCWP, annos map[string]string) {
	if v, ok := annos[microv1.AnnotationManaged]; ok {
		d.Managed = strings.EqualFold(v, "true")
	}
	if v, ok := annos[microv1.AnnotationEnforcement]; ok && v != "" {
		d.EnforcementMode = v
	}
	for k, v := range annos {
		if strings.HasPrefix(k, microv1.AnnotationLabelPrefix) && v != "" {
			labelKey := strings.TrimPrefix(k, microv1.AnnotationLabelPrefix)
			if labelKey != "" {
				if d.Labels == nil {
					d.Labels = map[string]string{}
				}
				d.Labels[labelKey] = v
			}
		}
	}
}

func copyLabels(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/controller/ -run TestComputeDesiredCWP -v`
Expected: PASS (all five sub-cases).

- [ ] **Step 5: Commit**

```bash
git add internal/controller/cwpmatch.go internal/controller/cwpmatch_test.go
git commit -m "feat(controller): add pure desired-CWP matching logic with annotation overrides"
```

---

### Task 4: Reconcile namespace CWPs in the ClusterProfile controller

**Files:**
- Modify: `internal/controller/onboarding_client.go` (extend the interface with CWP methods)
- Modify: `internal/controller/clusterprofile_controller.go` (add CWP reconcile + Namespace watch + Recorder)
- Modify: `internal/controller/suite_test.go` (extend the fake; add Recorder; namespaces)
- Create: `internal/controller/clusterprofile_cwp_test.go`
- Modify: `cmd/main.go` (set the Recorder)

**Interfaces:**
- Consumes: `pce.ContainerWorkloadProfile`, `pce.CWPUpdate`, `pce.CWPLabel`, `pce.LabelRef`, `pce.EnsureLabel`, `ComputeDesiredCWP`/`DesiredCWP` (Tasks 1, 3); `ClusterProfileSpec.NamespaceRules`/`SystemNamespaces`, `ClusterProfileStatus.ManagedNamespaces` (Task 2).
- Produces:
  - `OnboardingClient` gains `ListContainerWorkloadProfiles(ctx, clusterID string) ([]pce.ContainerWorkloadProfile, error)` and `UpdateContainerWorkloadProfile(ctx, profileHref string, update pce.CWPUpdate) error`.
  - `ClusterProfileReconciler` gains a `Recorder record.EventRecorder` field and a Namespace watch.
  - After a successful onboard, the reconcile reconciles every namespace's CWP and sets `status.ManagedNamespaces`.

- [ ] **Step 1: Write the failing test**

Create `internal/controller/clusterprofile_cwp_test.go`:

```go
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

var _ = Describe("ClusterProfile CWP reconcile", func() {
	const ns = "default"

	It("marks a matched namespace's CWP managed and counts it", func() {
		ctx := context.Background()

		// A namespace the rule will match.
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "payments-cwp"},
		})).To(Succeed())

		// Ready PCEConnection + secret.
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "pce-creds-cwp", Namespace: ns},
			Data:       map[string][]byte{keyAPIKey: []byte("k"), keyAPISecret: []byte("s")},
		})).To(Succeed())
		pc := &microv1.PCEConnection{
			ObjectMeta: metav1.ObjectMeta{Name: "pce-cwp"},
			Spec: microv1.PCEConnectionSpec{
				PCEURL: testPCEURL, OrgID: 1,
				CredentialsSecretRef: microv1.SecretReference{Name: "pce-creds-cwp", Namespace: ns},
			},
		}
		Expect(k8sClient.Create(ctx, pc)).To(Succeed())
		Eventually(func(g Gomega) {
			got := &microv1.PCEConnection{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "pce-cwp"}, got)).To(Succeed())
			meta.SetStatusCondition(&got.Status.Conditions, metav1.Condition{
				Type: microv1.ConditionConnected, Status: metav1.ConditionTrue, Reason: microv1.ReasonConnected, Message: "t",
			})
			g.Expect(k8sClient.Status().Update(ctx, got)).To(Succeed())
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

		cp := &microv1.ClusterProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "cp-cwp"},
			Spec: microv1.ClusterProfileSpec{
				PCEConnectionRef: microv1.LocalObjectReference{Name: "pce-cwp"},
				Onboarding: microv1.OnboardingSpec{
					ContainerClusterName: "ocp-cwp", CredentialsOutputSecret: "creds-cwp-out",
				},
				NamespaceRules: []microv1.NamespaceRule{
					{Match: microv1.NamespaceMatch{NamePattern: "payments-cwp"}, Managed: true,
						AssignLabels: map[string]microv1.LabelAssignment{"env": {Value: "prod"}}, EnforcementMode: "visibility_only"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, cp)).To(Succeed())

		// The fake onboarding client (suite_test.go) returns a CWP for namespace
		// "payments-cwp"; assert the reconcile counts it as managed.
		Eventually(func(g Gomega) {
			got := &microv1.ClusterProfile{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cp-cwp"}, got)).To(Succeed())
			g.Expect(meta.FindStatusCondition(got.Status.Conditions, microv1.ConditionOnboarded)).NotTo(BeNil())
			g.Expect(got.Status.ManagedNamespaces).To(BeNumerically(">=", 1))
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())
	})
})
```

- [ ] **Step 2: Extend the suite fake + wiring**

In `internal/controller/suite_test.go`:

(a) Add CWP methods to `fakeOnboardingClient` — it records updates and returns a CWP for the `payments-cwp` namespace:

```go
func (fakeOnboardingClient) ListContainerWorkloadProfiles(_ context.Context, _ string) ([]pce.ContainerWorkloadProfile, error) {
	return []pce.ContainerWorkloadProfile{
		{Href: "/orgs/1/container_clusters/uuid-ob/container_workload_profiles/p1", Namespace: "payments-cwp", Managed: false},
	}, nil
}
func (fakeOnboardingClient) UpdateContainerWorkloadProfile(_ context.Context, _ string, _ pce.CWPUpdate) error {
	return nil
}
```

(b) Set the `Recorder` on the registered `ClusterProfileReconciler` (use the manager's recorder):

```go
		Recorder: k8sManager.GetEventRecorderFor("clusterprofile-controller"),
```

(Add this field to the existing `ClusterProfileReconciler{...}` construction in the suite.)

- [ ] **Step 3: Run to verify it fails**

Run: `make test`
Expected: compile failure — `OnboardingClient` missing `ListContainerWorkloadProfiles`/`UpdateContainerWorkloadProfile`; `ClusterProfileReconciler` has no `Recorder` field.

- [ ] **Step 4: Extend the OnboardingClient interface**

In `internal/controller/onboarding_client.go`, add to the `OnboardingClient` interface:

```go
	ListContainerWorkloadProfiles(ctx context.Context, clusterID string) ([]pce.ContainerWorkloadProfile, error)
	UpdateContainerWorkloadProfile(ctx context.Context, profileHref string, update pce.CWPUpdate) error
```

(The `var _ OnboardingClient = (*pce.Client)(nil)` assertion already present will verify `*pce.Client` satisfies the extended interface at compile time.)

- [ ] **Step 5: Add CWP reconcile to the controller**

In `internal/controller/clusterprofile_controller.go`:

(a) Add imports: `"k8s.io/client-go/tools/record"`, `"sigs.k8s.io/controller-runtime/pkg/handler"`, `"sigs.k8s.io/controller-runtime/pkg/reconcile"`, and ensure `corev1` is imported.

(b) Add the `Recorder` field:

```go
type ClusterProfileReconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	OperatorNamespace   string
	NewOnboardingClient OnboardingClientFactory
	Recorder            record.EventRecorder
}
```

(c) Add RBAC markers (above Reconcile, alongside the existing ones):

```go
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
```

(d) In `Reconcile`, after the block that sets `Onboarded=True` succeeds (i.e. after the pairing/secret steps, just before the final `Status().Update`), insert the CWP reconcile so the managed count is part of the same status write:

```go
	// Reconcile per-namespace CWPs now that the cluster is onboarded.
	managed, cwpErr := r.reconcileNamespaceCWPs(ctx, &cp, pclient, owner)
	if cwpErr != nil {
		return ctrl.Result{}, cwpErr
	}
	cp.Status.ManagedNamespaces = managed
```

(Place this immediately before `meta.SetStatusCondition(... Onboarded ...)` / the final `Status().Update`; reuse the existing `pclient` and `owner` variables.)

(e) Add the reconcile + update-builder methods:

```go
// reconcileNamespaceCWPs evaluates every namespace against the profile's rules
// and updates each namespace's CWP in the PCE. Returns the managed count.
func (r *ClusterProfileReconciler) reconcileNamespaceCWPs(ctx context.Context, cp *microv1.ClusterProfile, pclient OnboardingClient, owner pce.Owner) (int, error) {
	if cp.Status.ContainerClusterID == "" {
		return 0, nil
	}
	var nsList corev1.NamespaceList
	if err := r.List(ctx, &nsList); err != nil {
		return 0, err
	}
	cwps, err := pclient.ListContainerWorkloadProfiles(ctx, cp.Status.ContainerClusterID)
	if err != nil {
		return 0, err
	}
	byNS := make(map[string]pce.ContainerWorkloadProfile, len(cwps))
	for _, c := range cwps {
		if c.Namespace != "" {
			byNS[c.Namespace] = c
		}
	}

	labelHref := map[string]string{} // "key|value" -> href cache
	managed := 0
	for i := range nsList.Items {
		nsObj := &nsList.Items[i]
		desired := ComputeDesiredCWP(nsObj.Name, nsObj.Labels, nsObj.Annotations, cp.Spec.NamespaceRules, cp.Spec.SystemNamespaces)
		if desired.Managed {
			managed++
		}
		cwp, ok := byNS[nsObj.Name]
		if !ok {
			// Kubelink has not created this namespace's CWP yet; reconcile later.
			continue
		}
		update, changed, lerr := r.buildCWPUpdate(ctx, pclient, owner, cwp, desired, labelHref)
		if lerr != nil {
			return managed, lerr
		}
		if !changed {
			continue
		}
		if err := pclient.UpdateContainerWorkloadProfile(ctx, cwp.Href, update); err != nil {
			return managed, err
		}
		if r.Recorder != nil {
			r.Recorder.Eventf(nsObj, corev1.EventTypeNormal, "CWPConfigured",
				"managed=%v enforcement=%s", desired.Managed, desired.EnforcementMode)
		}
	}
	return managed, nil
}

// buildCWPUpdate resolves desired labels to Illumio hrefs and diffs against the
// current CWP. Returns the update body and whether anything changed.
func (r *ClusterProfileReconciler) buildCWPUpdate(ctx context.Context, pclient OnboardingClient, owner pce.Owner, current pce.ContainerWorkloadProfile, desired DesiredCWP, labelHref map[string]string) (pce.CWPUpdate, bool, error) {
	// Resolve desired labels to hrefs (create-if-missing, cached).
	desiredLabels := make([]pce.CWPLabel, 0, len(desired.Labels))
	desiredHrefByKey := map[string]string{}
	for key, value := range desired.Labels {
		cacheKey := key + "|" + value
		href, ok := labelHref[cacheKey]
		if !ok {
			lbl, err := pclient.EnsureLabel(ctx, key, value, owner)
			if err != nil {
				return pce.CWPUpdate{}, false, err
			}
			href = lbl.Href
			labelHref[cacheKey] = href
		}
		desiredHrefByKey[key] = href
		desiredLabels = append(desiredLabels, pce.CWPLabel{Key: key, Assignment: &pce.LabelRef{Href: href}})
	}

	// Diff: managed, enforcement, and the set of (key->href) assignments.
	changed := current.Managed != desired.Managed
	enforcement := desired.EnforcementMode
	if desired.Managed && current.EnforcementMode != enforcement && enforcement != "" {
		changed = true
	}
	currentHrefByKey := map[string]string{}
	for _, l := range current.Labels {
		if l.Assignment != nil {
			currentHrefByKey[l.Key] = l.Assignment.Href
		}
	}
	if !sameLabelSet(currentHrefByKey, desiredHrefByKey) {
		changed = true
	}

	managed := desired.Managed
	update := pce.CWPUpdate{Managed: &managed, Labels: desiredLabels}
	if desired.Managed && enforcement != "" {
		update.EnforcementMode = enforcement
	}
	return update, changed, nil
}

func sameLabelSet(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
```

(f) Update `SetupWithManager` to watch namespaces:

```go
func (r *ClusterProfileReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&microv1.ClusterProfile{}).
		Watches(&corev1.Namespace{}, handler.EnqueueRequestsFromMapFunc(r.clusterProfilesForNamespace)).
		Complete(r)
}

// clusterProfilesForNamespace enqueues all ClusterProfiles when any namespace
// changes (rules apply cluster-wide).
func (r *ClusterProfileReconciler) clusterProfilesForNamespace(ctx context.Context, _ client.Object) []reconcile.Request {
	var list microv1.ClusterProfileList
	if err := r.List(ctx, &list); err != nil {
		return nil
	}
	reqs := make([]reconcile.Request, 0, len(list.Items))
	for i := range list.Items {
		reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{Name: list.Items[i].Name}})
	}
	return reqs
}
```

- [ ] **Step 6: Set the Recorder in main**

In `cmd/main.go`, add `Recorder: mgr.GetEventRecorderFor("clusterprofile-controller"),` to the `ClusterProfileReconciler{...}` construction.

- [ ] **Step 7: Run tests, build, lint**

```bash
make test
make build
make lint
```

Expected: PASS (incl. the new CWP reconcile spec asserting `ManagedNamespaces >= 1`); build clean; lint `0 issues` (extract a constant if `goconst` fires).

- [ ] **Step 8: Commit**

```bash
git add internal/controller/ cmd/main.go config/rbac/
git commit -m "feat(controller): reconcile per-namespace CWPs from ClusterProfile rules"
```

---

### Task 5: Apply deferred Plan 2 review fixes (M5, M7)

**Files:**
- Modify: `internal/controller/clusterprofile_controller.go` (M5)
- Create: `dist/chart/templates/rbac/clusterprofile-admin-role.yaml`, `clusterprofile-editor-role.yaml`, `clusterprofile-viewer-role.yaml` (M7)

**Interfaces:**
- Consumes: nothing new.
- Produces: a longer requeue interval for terminal onboarding failures; chart RBAC helper roles for `clusterprofiles`.

- [ ] **Step 1: M5 — longer requeue on terminal onboarding failure**

In `internal/controller/clusterprofile_controller.go`, add a constant and use it for the pre-existing-cluster failure (a terminal-ish state that should not re-list the PCE every 10 minutes):

```go
	onboardRequeueTerminal = time.Hour
```

Change the pre-existing-cluster `onboardFail(...)` call (the one with the "already exists … token cannot be recovered" message) to pass `onboardRequeueTerminal` instead of `onboardRequeueHealthy`. Leave the `PCEConnectionNotReady` path on its shorter interval (it self-heals when the connection becomes ready).

- [ ] **Step 2: M7 — chart helper roles for clusterprofiles**

Create three templates mirroring the existing `dist/chart/templates/rbac/pceconnection-*-role.yaml` files, substituting `clusterprofiles` for `pceconnections`. For example `clusterprofile-viewer-role.yaml`:

```yaml
{{- if .Values.rbac.enable }}
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: {{ .Chart.Name }}-clusterprofile-viewer-role
  labels:
    {{- include "chart.labels" . | nindent 4 }}
rules:
  - apiGroups:
      - microsegment.io
    resources:
      - clusterprofiles
    verbs:
      - get
      - list
      - watch
  - apiGroups:
      - microsegment.io
    resources:
      - clusterprofiles/status
    verbs:
      - get
{{- end }}
```

For the admin and editor roles, mirror the verbs used in the corresponding `pceconnection-admin-role.yaml` / `pceconnection-editor-role.yaml` (copy those files, replace `pceconnection`→`clusterprofile` and `pceconnections`→`clusterprofiles`, and keep the same `{{- if }}` guard, labels include, and metadata naming pattern). Match whatever guard/label helper the existing files use — open one and replicate it exactly so the new files are consistent.

- [ ] **Step 3: Verify**

```bash
make build
make test
make lint
helm lint dist/chart
helm template illumio-operator dist/chart --set pce.url=x --set pce.orgId=3 --set pce.apiKey=k --set pce.apiSecret=s | grep -c "clusterprofile-viewer-role"
```

Expected: build/test/lint green; `helm lint` passes; the grep returns `1` (the viewer role renders). If the existing pceconnection helper roles are gated behind a values flag that defaults off, the grep may return `0` with the default — in that case render with the same flag the pceconnection roles require and confirm all three clusterprofile roles appear.

- [ ] **Step 4: Commit**

```bash
git add internal/controller/clusterprofile_controller.go dist/chart/templates/rbac/
git commit -m "fix: longer requeue on terminal onboarding failure; add clusterprofile RBAC helper roles"
```

---

### Task 6: CWP management documentation

**Files:**
- Create: `docs/guides/namespace-management.md`
- Modify: `docs/reference/clusterprofile.md` (add namespaceRules/systemNamespaces/annotations/managedNamespaces)
- Modify: `docs/getting-started.md` (add a "manage namespaces" step)
- Modify: `mkdocs.yml` (nav)
- Run: `make docs-api` (regenerate `docs/reference/api.md`).

**Interfaces:**
- Consumes: behavior from Tasks 2–4.
- Produces: published CWP-management documentation.

> **Dispatch note:** dispatch to the documentation subagent. Content only.

- [ ] **Step 1: Write the namespace-management guide**

Create `docs/guides/namespace-management.md` covering: what CWP management does (marks namespaces managed, assigns Illumio labels, sets enforcement); the precedence model (**`systemNamespaces` config governs system-pattern namespaces when enabled and overrides user rules for them; user `namespaceRules` first-match govern all other namespaces; then per-namespace annotation overrides**); a complete `ClusterProfile` example with `systemNamespaces` (taming `openshift-*`/`kube-*` out of the box) and a couple of `namespaceRules` (incl. `fromNamespaceLabel`); the per-namespace annotations (`microsegment.io/managed`, `microsegment.io/enforcement`, `microsegment.io/label.<key>`); a note that **Kubelink must have created the CWP** (the operator configures existing CWPs and skips namespaces whose CWP doesn't exist yet, reconciling them once it appears); and how to observe results (`kubectl get clusterprofiles` → `Managed-NS` column / `status.managedNamespaces`; `kubectl describe namespace <ns>` → `CWPConfigured` events). Include a **recommended OpenShift starter block**:

```yaml
apiVersion: microsegment.io/v1alpha1
kind: ClusterProfile
metadata:
  name: this-cluster
spec:
  pceConnectionRef: { name: prod-pce }
  onboarding:
    containerClusterName: ocp-prod-01
    credentialsOutputSecret: illumio-cluster-creds
  systemNamespaces:
    manage: true
    labels: { role: control, env: prod }
    enforcementMode: visibility_only
    # patterns default to openshift-*, kube-*, default, kube-system, kube-public, kube-node-lease
  namespaceRules:
    - match: { labels: { "microsegment.io/managed": "true" } }
      managed: true
      assignLabels:
        app: { fromNamespaceLabel: app.kubernetes.io/part-of }
        env: { fromNamespaceLabel: app.kubernetes.io/environment }
      enforcementMode: visibility_only
    - match: { namePattern: "*" }
      managed: false
```

- [ ] **Step 2: Update the ClusterProfile reference**

In `docs/reference/clusterprofile.md`, add the new spec fields (`systemNamespaces.{manage,patterns,labels,enforcementMode}`, `namespaceRules[].{match.{namePattern,labels},managed,assignLabels.{value,fromNamespaceLabel},enforcementMode}`), the per-namespace annotations, and the `managedNamespaces` status field, in the existing table style.

- [ ] **Step 3: Update getting-started + nav**

Add a "Manage namespaces" step to `docs/getting-started.md` (apply rules; check the `Managed-NS` count) linking to the guide. Add `- Namespace management: guides/namespace-management.md` under the `Guides` nav section in `mkdocs.yml`.

- [ ] **Step 4: Regenerate API reference + build**

```bash
make docs-api
. .venv-docs/bin/activate 2>/dev/null && mkdocs build --strict ; deactivate 2>/dev/null || true
```

Expected: `docs/reference/api.md` includes the new types; `mkdocs build --strict` passes (or rely on Docs CI if Python is unavailable; `make docs-api` must still succeed).

- [ ] **Step 5: Commit**

```bash
git add docs/ mkdocs.yml
git commit -m "docs: add namespace (CWP) management guide and reference"
```

---

## Self-Review

**Spec coverage (design spec §2, §4.2, §4.4, §5.1, §8):**
- §2 CWP/namespace labeling (managed flag, CWP labels, enforcement) → Tasks 1–4. ✓
- §4.2 `namespaceRules` (ordered, name/label match, fixed or `fromNamespaceLabel` labels, managed, enforcement) + system-namespace defaults for `openshift-*`/`kube-*` → Tasks 2, 3. ✓
- §4.4 per-namespace annotation override with documented precedence (systemNamespaces governs system namespaces; user rules first-match govern others; annotations override) → Task 3. ✓
- §8 ownership tagging on labels the operator creates for CWPs (via `EnsureLabel`) → Task 4 `buildCWPUpdate`. ✓ (CWP objects are Kubelink-created; we only update them — no provisioning needed.)
- §10 status/visibility — `status.managedNamespaces` + per-namespace `CWPConfigured` events → Task 4. ✓
- Deferred Plan 2 review items M5 (terminal-failure requeue) and M7 (clusterprofiles helper RBAC) → Task 5. ✓
- Docs from the start → Task 6. ✓

**Enforcement note:** Plan 3 sets CWP enforcement from the namespace rule (admin-driven). The per-policy *strictest* enforcement resolution across app-team policy CRs (spec §5.1) belongs to Plan 4 (segmentation policy) and is explicitly out of scope here.

**Out of Plan 3 scope (deferred to Plan 4):** the policy IR, provisioning (draft→active), and the `SegmentationIntent`/`SegmentationPolicy` front-ends.

**Placeholder scan:** No `TBD`/`TODO`/"handle edge cases" — every code step has complete code. The two environment notes (helm role values-flag in Task 5 Step 3; Python availability in Task 6 Step 4) give explicit fallbacks.

**Type consistency:** `pce.ContainerWorkloadProfile`/`CWPUpdate`/`CWPLabel`/`LabelRef`; `ComputeDesiredCWP`/`DesiredCWP`; `NamespaceRule`/`NamespaceMatch`/`LabelAssignment`/`SystemNamespacesSpec`; annotation constants `AnnotationManaged`/`AnnotationEnforcement`/`AnnotationLabelPrefix`; `ClusterProfileStatus.ManagedNamespaces`; the extended `OnboardingClient` methods and `ClusterProfileReconciler.Recorder` — used identically across the tasks that define and consume them. Reuses Plan 1/2 symbols (`pce.EnsureLabel`, `pce.Owner`, `pce.Label`, test constants `keyAPIKey`/`keyAPISecret`/`testPCEURL`) without redefining them.
