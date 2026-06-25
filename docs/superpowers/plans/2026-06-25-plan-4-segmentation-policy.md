# Illumio K8s Utility Operator — Plan 4: Segmentation Policy (SegmentationIntent front-end)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let application teams express Illumio segmentation policy as a Kubernetes resource (`SegmentationIntent`) — never touching the Illumio console or Terraform. The operator compiles each intent into an Illumio **ruleset + rules** (label-based, pod-resolved), writes them to the PCE **draft**, and **provisions** them per the cluster's provisioning mode, with guardrails that keep an app team's policy scoped to **their own** namespace.

**Architecture:** Extends Plans 1–3. `internal/pce` gains a policy client (rulesets, rules, provisioning) under `sec_policy/draft`. A namespaced `SegmentationIntent` CRD is compiled by a pure IR (`CompileIntent` → `DesiredPolicy` → PCE objects). A namespaced reconciler resolves the namespace's own provider labels (reusing Plan 3's `ComputeDesiredCWP`) and the consumer labels (must already exist in the PCE), enforces guardrails, reconciles one owned ruleset + its rules in draft, provisions the change subset per `ClusterProfile.spec.provisioningMode`, reports `workloadsAffected`, and cleans up via a finalizer.

**Tech Stack:** Go, kubebuilder v4 + controller-runtime, envtest, `net/http/httptest`; MkDocs for docs.

## Global Constraints

- **Module path:** `github.com/alexgoller/illumio-k8s-utility-operator`. **API group:** `microsegment.io`; **version:** `v1alpha1`.
- **Target platform:** Illumio Core for Kubernetes in **CLAS** mode, **PCE 24.5+**.
- **Policy API facts (verified against PCE 24.5 REST / illumio-py / terraform-provider):**
  - Security-policy objects live under `/orgs/{org}/sec_policy/{pversion}/...`; **mutations target `draft`**. `href` values in bodies omit the `/api/v2` prefix.
  - **Services can be inlined** in a rule — no Service object needed: `ingress_services: [{ "proto": 6, "port": 8080 }]` (`proto` numeric IANA: 6=TCP, 17=UDP; `port` optional for all-ports; `to_port` for ranges).
  - **Ruleset:** `POST /orgs/{org}/sec_policy/draft/rule_sets` body `{ "name", "enabled": true, "scopes": [[ {"label":{"href":...}} ]] }`. `scopes` is an **array of arrays**; a single inner array of `{"label":{"href":...}}` entries is a valid app scope. `[[]]` = all (never emit that for app-team policy).
  - **Rule:** `POST .../rule_sets/{id}/sec_rules` body requires `resolve_labels_as` `{ "providers":["workloads"], "consumers":["workloads"] }` (use `["workloads"]` for pods — CLAS-safe), `providers` and `consumers` (each ≥1 actor `{"label":{"href":...}}`), `ingress_services` (≥1), and `unscoped_consumers` (bool — `true` when consumers are outside the ruleset scope, which is our case).
  - **Provision:** `POST /orgs/{org}/sec_policy` body `{ "update_description", "change_subset": { "rule_sets": [{"href": <draft-href>}] } }` → response `{ "version", "workloads_affected" }`. Omitting `change_subset` provisions ALL pending — **never do that**; always scope to our own ruleset hrefs.
  - **Delete:** `DELETE <draft rule/ruleset href>`, then provision the removal (include the href in `change_subset`, or it's covered by a provision-all which we avoid → we provision our ruleset href explicitly).
  - **Labels need no provisioning**; rulesets/rules **do**. Rule authoring/provisioning is **independent of enforcement mode** (rules are computed; enforcement decides whether non-allowed traffic is blocked — enforcement stays admin-driven via Plan 3 CWPs; per-policy enforcement-strictest is Plan 5).
- **Ownership tagging:** the operator stamps `external_data_set` (operator id) + `external_data_reference` (CR UID) on the **ruleset** it creates, so it owns exactly one ruleset per `SegmentationIntent` and provisions only that.
- **Guardrails (design §5):** the **provider** is always the namespace's own resolved labels (an app team can only protect their own app). **Consumers** must resolve to **existing** PCE labels (created by Kubelink from real workloads); unknown → CR `Rejected`, never created. Never emit `[[]]` (all) scopes or estate-wide rules.
- **Commits:** conventional-commit messages; commit at the end of every task; end messages with the repo's `Co-Authored-By` trailer.

---

### Task 1: PCE policy client (rulesets, rules, provisioning)

**Files:**
- Create: `internal/pce/policy.go`
- Test: `internal/pce/policy_test.go`

**Interfaces:**
- Consumes: `Client.do`, `Client.orgPath`, `LabelRef` (Plans 1–3).
- Produces:
  - `type RuleSetScope struct { Label LabelRef }` (json `label`)
  - `type Actor struct { Label *LabelRef }` (json `label,omitempty`)
  - `type IngressService struct { Proto int; Port int }` (json `proto`, `port,omitempty`)
  - `type ResolveLabelsAs struct { Providers []string; Consumers []string }` (json `providers`/`consumers`)
  - `type RuleSet struct { Href, Name string; Enabled bool; Scopes [][]RuleSetScope; ExternalDataSet, ExternalDataReference string }`
  - `type SecRule struct { Href string; Enabled bool; ResolveLabelsAs ResolveLabelsAs; Providers, Consumers []Actor; IngressServices []IngressService; UnscopedConsumers bool }`
  - `type ProvisionResult struct { Href string; Version int; WorkloadsAffected int }`
  - `func (c *Client) FindRuleSetByOwner(ctx, owner Owner) (*RuleSet, error)` — list draft rule_sets, return the one whose `external_data_reference` matches (or nil)
  - `func (c *Client) CreateRuleSet(ctx, rs RuleSet) (*RuleSet, error)`
  - `func (c *Client) DeleteRuleSet(ctx, href string) error`
  - `func (c *Client) ListRules(ctx, ruleSetHref string) ([]SecRule, error)`
  - `func (c *Client) CreateRule(ctx, ruleSetHref string, rule SecRule) (*SecRule, error)`
  - `func (c *Client) DeleteRule(ctx, ruleHref string) error`
  - `func (c *Client) ProvisionRuleSets(ctx, ruleSetHrefs []string, description string) (*ProvisionResult, error)`

- [ ] **Step 1: Write the failing test**

Create `internal/pce/policy_test.go`:

```go
package pce

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestCreateRuleSet_PostsScopesAndOwnership(t *testing.T) {
	var posted RuleSet
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/orgs/7/sec_policy/draft/rule_sets" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&posted)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"href":"/orgs/7/sec_policy/draft/rule_sets/843","name":"rs","enabled":true}`))
	})
	rs, err := c.CreateRuleSet(context.Background(), RuleSet{
		Name: "rs", Enabled: true,
		Scopes:          [][]RuleSetScope{{{Label: LabelRef{Href: "/orgs/7/labels/14"}}}},
		ExternalDataSet: "illumio-operator", ExternalDataReference: "cr-uid",
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if rs.Href != "/orgs/7/sec_policy/draft/rule_sets/843" {
		t.Errorf("href = %q", rs.Href)
	}
	if len(posted.Scopes) != 1 || len(posted.Scopes[0]) != 1 || posted.Scopes[0][0].Label.Href != "/orgs/7/labels/14" {
		t.Errorf("scopes = %+v", posted.Scopes)
	}
	if posted.ExternalDataReference != "cr-uid" {
		t.Errorf("ownership = %+v", posted)
	}
}

func TestCreateRule_PostsResolveLabelsAndInlinePort(t *testing.T) {
	var posted SecRule
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/orgs/7/sec_policy/draft/rule_sets/843/sec_rules" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&posted)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"href":"/orgs/7/sec_policy/draft/rule_sets/843/sec_rules/9"}`))
	})
	rule, err := c.CreateRule(context.Background(), "/orgs/7/sec_policy/draft/rule_sets/843", SecRule{
		Enabled:         true,
		ResolveLabelsAs: ResolveLabelsAs{Providers: []string{"workloads"}, Consumers: []string{"workloads"}},
		Providers:       []Actor{{Label: &LabelRef{Href: "/orgs/7/labels/14"}}},
		Consumers:       []Actor{{Label: &LabelRef{Href: "/orgs/7/labels/15"}}},
		IngressServices: []IngressService{{Proto: 6, Port: 8443}},
		UnscopedConsumers: true,
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if rule.Href != "/orgs/7/sec_policy/draft/rule_sets/843/sec_rules/9" {
		t.Errorf("href = %q", rule.Href)
	}
	if posted.ResolveLabelsAs.Providers[0] != "workloads" || !posted.UnscopedConsumers {
		t.Errorf("posted = %+v", posted)
	}
	if len(posted.IngressServices) != 1 || posted.IngressServices[0].Proto != 6 || posted.IngressServices[0].Port != 8443 {
		t.Errorf("ingress = %+v", posted.IngressServices)
	}
}

func TestProvisionRuleSets_PostsChangeSubsetAndReadsAffected(t *testing.T) {
	var body map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/orgs/7/sec_policy" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"href":"/orgs/7/sec_policy/80","version":80,"workloads_affected":3}`))
	})
	res, err := c.ProvisionRuleSets(context.Background(),
		[]string{"/orgs/7/sec_policy/draft/rule_sets/843"}, "deploy app policy")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if res.WorkloadsAffected != 3 || res.Version != 80 {
		t.Errorf("res = %+v", res)
	}
	cs, _ := body["change_subset"].(map[string]any)
	rsList, _ := cs["rule_sets"].([]any)
	if len(rsList) != 1 {
		t.Fatalf("change_subset.rule_sets = %+v", cs["rule_sets"])
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/pce/ -run "RuleSet|Rule_|Provision" -v`
Expected: compile failure — `undefined: RuleSet`, etc.

- [ ] **Step 3: Write the implementation**

Create `internal/pce/policy.go`:

```go
package pce

import (
	"context"
	"net/http"
)

// RuleSetScope is one scope entry: {"label":{"href":...}}.
type RuleSetScope struct {
	Label LabelRef `json:"label"`
}

// Actor is a rule provider/consumer: {"label":{"href":...}}.
type Actor struct {
	Label *LabelRef `json:"label,omitempty"`
}

// IngressService is an inline port/proto (no Service object needed).
type IngressService struct {
	Proto int `json:"proto"`
	Port  int `json:"port,omitempty"`
}

// ResolveLabelsAs controls how provider/consumer labels resolve. Use
// ["workloads"] for pods/container workloads.
type ResolveLabelsAs struct {
	Providers []string `json:"providers"`
	Consumers []string `json:"consumers"`
}

// RuleSet is an Illumio ruleset (draft).
type RuleSet struct {
	Href                  string           `json:"href,omitempty"`
	Name                  string           `json:"name"`
	Enabled               bool             `json:"enabled"`
	Scopes                [][]RuleSetScope `json:"scopes"`
	ExternalDataSet       string           `json:"external_data_set,omitempty"`
	ExternalDataReference string           `json:"external_data_reference,omitempty"`
}

// SecRule is an Illumio security rule (draft).
type SecRule struct {
	Href              string           `json:"href,omitempty"`
	Enabled           bool             `json:"enabled"`
	ResolveLabelsAs   ResolveLabelsAs  `json:"resolve_labels_as"`
	Providers         []Actor          `json:"providers"`
	Consumers         []Actor          `json:"consumers"`
	IngressServices   []IngressService `json:"ingress_services"`
	UnscopedConsumers bool             `json:"unscoped_consumers"`
}

// ProvisionResult is the response to a provisioning request.
type ProvisionResult struct {
	Href              string `json:"href"`
	Version           int    `json:"version"`
	WorkloadsAffected int    `json:"workloads_affected"`
}

const secPolicyDraft = "/sec_policy/draft"

// FindRuleSetByOwner returns the draft ruleset owned by the given CR (matched
// on external_data_reference), or (nil, nil).
func (c *Client) FindRuleSetByOwner(ctx context.Context, owner Owner) (*RuleSet, error) {
	var sets []RuleSet
	if err := c.do(ctx, http.MethodGet, c.orgPath(secPolicyDraft+"/rule_sets"), nil, &sets); err != nil {
		return nil, err
	}
	for i := range sets {
		if sets[i].ExternalDataReference == owner.Reference && owner.Reference != "" {
			return &sets[i], nil
		}
	}
	return nil, nil
}

// CreateRuleSet creates a draft ruleset.
func (c *Client) CreateRuleSet(ctx context.Context, rs RuleSet) (*RuleSet, error) {
	var created RuleSet
	if err := c.do(ctx, http.MethodPost, c.orgPath(secPolicyDraft+"/rule_sets"), rs, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

// DeleteRuleSet deletes a draft ruleset by href.
func (c *Client) DeleteRuleSet(ctx context.Context, href string) error {
	return c.do(ctx, http.MethodDelete, href, nil, nil)
}

// ListRules lists the rules in a ruleset (ruleSetHref is a draft href).
func (c *Client) ListRules(ctx context.Context, ruleSetHref string) ([]SecRule, error) {
	var rules []SecRule
	if err := c.do(ctx, http.MethodGet, ruleSetHref+"/sec_rules", nil, &rules); err != nil {
		return nil, err
	}
	return rules, nil
}

// CreateRule creates a rule under a ruleset.
func (c *Client) CreateRule(ctx context.Context, ruleSetHref string, rule SecRule) (*SecRule, error) {
	var created SecRule
	if err := c.do(ctx, http.MethodPost, ruleSetHref+"/sec_rules", rule, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

// DeleteRule deletes a rule by href.
func (c *Client) DeleteRule(ctx context.Context, ruleHref string) error {
	return c.do(ctx, http.MethodDelete, ruleHref, nil, nil)
}

// ProvisionRuleSets provisions exactly the given draft rulesets (never all).
func (c *Client) ProvisionRuleSets(ctx context.Context, ruleSetHrefs []string, description string) (*ProvisionResult, error) {
	refs := make([]map[string]string, 0, len(ruleSetHrefs))
	for _, h := range ruleSetHrefs {
		refs = append(refs, map[string]string{"href": h})
	}
	body := map[string]any{
		"update_description": description,
		"change_subset":      map[string]any{"rule_sets": refs},
	}
	var res ProvisionResult
	if err := c.do(ctx, http.MethodPost, c.orgPath("/sec_policy"), body, &res); err != nil {
		return nil, err
	}
	return &res, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/pce/ -v`
Expected: PASS (all pce tests, including the three new policy tests).

- [ ] **Step 5: Commit**

```bash
git add internal/pce/policy.go internal/pce/policy_test.go
git commit -m "feat(pce): add policy client (rulesets, rules, provisioning)"
```

---

### Task 2: SegmentationIntent CRD

**Files:**
- Create: `api/v1alpha1/segmentationintent_types.go`
- Test: `api/v1alpha1/segmentationintent_types_test.go`
- Generated: `zz_generated.deepcopy.go`, `config/crd/bases/microsegment.io_segmentationintents.yaml`, RBAC.

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `IntentPort{ Port int; Protocol string }` (`protocol` enum TCP;UDP, default TCP)
  - `IntentAllow{ From map[string]string; Ports []IntentPort }` (`from` = Illumio label key→value of the consumer)
  - `SegmentationIntentSpec{ Allow []IntentAllow }`
  - `SegmentationIntentStatus{ Conditions []metav1.Condition; WorkloadsAffected int; ObservedGeneration int64 }`
  - condition/reason constants: `ConditionReady = "Ready"`, `ConditionProvisioned = "Provisioned"`; reasons `ReasonCompiled`, `ReasonRejected`, `ReasonClusterProfileNotReady`, `ReasonProvisioned`, `ReasonProvisionPending`.
  - annotation `AnnotationProvisionApprove = "microsegment.io/provision"` (value `approved`)

- [ ] **Step 1: Write the failing test**

Create `api/v1alpha1/segmentationintent_types_test.go`:

```go
package v1alpha1

import "testing"

func TestSegmentationIntent_Shape(t *testing.T) {
	si := SegmentationIntent{
		Spec: SegmentationIntentSpec{
			Allow: []IntentAllow{
				{From: map[string]string{"app": "checkout", "env": "prod"}, Ports: []IntentPort{{Port: 8443, Protocol: "TCP"}}},
			},
		},
	}
	if si.Spec.Allow[0].From["app"] != "checkout" {
		t.Errorf("from app = %q", si.Spec.Allow[0].From["app"])
	}
	if ConditionReady != "Ready" || AnnotationProvisionApprove != "microsegment.io/provision" {
		t.Errorf("constants wrong")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./api/v1alpha1/ -run TestSegmentationIntent -v`
Expected: compile failure — `undefined: SegmentationIntent`.

- [ ] **Step 3: Write the types**

Create `api/v1alpha1/segmentationintent_types.go`:

```go
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IntentPort is a port/protocol a consumer is allowed to reach.
type IntentPort struct {
	Port int `json:"port"`
	// +kubebuilder:validation:Enum=TCP;UDP
	// +kubebuilder:default=TCP
	// +optional
	Protocol string `json:"protocol,omitempty"`
}

// IntentAllow allows a consumer (referenced by existing Illumio labels) to
// reach this namespace's app on the given ports.
type IntentAllow struct {
	// From is the consumer's Illumio labels (key -> value), e.g.
	// {"app":"checkout","env":"prod"}. They must already exist in the PCE.
	From map[string]string `json:"from"`
	// Ports the consumer may reach. Empty means all ports.
	// +optional
	Ports []IntentPort `json:"ports,omitempty"`
}

// SegmentationIntentSpec is an app team's allow-list for their namespace's app.
type SegmentationIntentSpec struct {
	// Allow is the set of permitted inbound flows to this namespace's app.
	Allow []IntentAllow `json:"allow"`
}

// SegmentationIntentStatus is the observed state.
type SegmentationIntentStatus struct {
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// WorkloadsAffected is the count from the last provisioning.
	// +optional
	WorkloadsAffected int `json:"workloadsAffected,omitempty"`
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// Condition types/reasons for SegmentationIntent.
const (
	ConditionReady       = "Ready"
	ConditionProvisioned = "Provisioned"

	ReasonCompiled               = "Compiled"
	ReasonRejected               = "Rejected"
	ReasonClusterProfileNotReady = "ClusterProfileNotReady"
	ReasonProvisioned            = "Provisioned"
	ReasonProvisionPending       = "ProvisionPending"

	// AnnotationProvisionApprove on a SegmentationIntent approves a pending
	// provision when provisioningMode is manual (value "approved").
	AnnotationProvisionApprove = "microsegment.io/provision"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:categories=illumio,shortName=segintent
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Provisioned",type=string,JSONPath=`.status.conditions[?(@.type=="Provisioned")].status`
// +kubebuilder:printcolumn:name="Affected",type=integer,JSONPath=`.status.workloadsAffected`

// SegmentationIntent is an app team's Illumio allow-list for their namespace.
type SegmentationIntent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SegmentationIntentSpec   `json:"spec,omitempty"`
	Status SegmentationIntentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SegmentationIntentList contains a list of SegmentationIntent.
type SegmentationIntentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SegmentationIntent `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *scheme.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &SegmentationIntent{}, &SegmentationIntentList{})
		return nil
	})
}
```

> **NOTE on `init()`:** match the existing registration pattern in `clusterprofile_types.go`/`pceconnection_types.go` exactly (this repo's `SchemeBuilder` is a `runtime.NewSchemeBuilder`, so registration is `SchemeBuilder.Register(func(s *runtime.Scheme) error { s.AddKnownTypes(SchemeGroupVersion, ...); return nil })`). Use whatever import alias those files use for the runtime scheme — do **not** invent a `scheme` package. `SegmentationIntent` is **namespaced** (no `scope=Cluster` marker).

- [ ] **Step 4: Regenerate and verify**

```bash
make generate
make manifests
go test ./api/v1alpha1/ -run TestSegmentationIntent -v
make build
```

Expected: deepcopy + CRD generated; `config/crd/bases/microsegment.io_segmentationintents.yaml` is **namespaced** (no `scope: Cluster`), category `illumio`, shortName `segintent`; test PASS; build clean.

- [ ] **Step 5: Commit**

```bash
git add api/v1alpha1/ config/crd/ config/rbac/
git commit -m "feat(api): add SegmentationIntent CRD"
```

---

### Task 3: Policy IR — compile intent to ruleset + rules (pure)

**Files:**
- Create: `internal/controller/policyir.go`
- Test: `internal/controller/policyir_test.go`

**Interfaces:**
- Consumes: `pce.RuleSet`, `pce.SecRule`, `pce.RuleSetScope`, `pce.Actor`, `pce.IngressService`, `pce.ResolveLabelsAs`, `pce.LabelRef`, `pce.Owner` (Task 1).
- Produces:
  - `type ResolvedAllow struct { ConsumerHrefs []string; Ports []pce.IngressService }`
  - `func RuleSetName(namespace, crName string) string` — deterministic name (e.g. `k8s-<namespace>-<crName>`)
  - `func BuildRuleSet(namespace, crName string, providerHrefs []string, owner pce.Owner) pce.RuleSet` — scope = provider labels
  - `func BuildRules(providerHrefs []string, allows []ResolvedAllow) []pce.SecRule` — one rule per allow; providers=providerHrefs, consumers=allow.ConsumerHrefs, ingress=allow.Ports, resolve_labels_as workloads, unscoped_consumers=true
  - `func protoNumber(protocol string) int` — TCP→6, UDP→17

- [ ] **Step 1: Write the failing test**

Create `internal/controller/policyir_test.go`:

```go
package controller

import (
	"testing"

	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

func TestBuildRuleSet_ScopesToProviderAndStampsOwner(t *testing.T) {
	rs := BuildRuleSet("payments", "ingress", []string{"/orgs/1/labels/14"}, pce.Owner{DataSet: "illumio-operator", Reference: "uid-1"})
	if rs.Name != RuleSetName("payments", "ingress") {
		t.Errorf("name = %q", rs.Name)
	}
	if len(rs.Scopes) != 1 || len(rs.Scopes[0]) != 1 || rs.Scopes[0][0].Label.Href != "/orgs/1/labels/14" {
		t.Fatalf("scopes = %+v", rs.Scopes)
	}
	if rs.ExternalDataReference != "uid-1" || !rs.Enabled {
		t.Errorf("rs = %+v", rs)
	}
}

func TestBuildRules_OneRulePerAllow(t *testing.T) {
	rules := BuildRules(
		[]string{"/orgs/1/labels/14"},
		[]ResolvedAllow{
			{ConsumerHrefs: []string{"/orgs/1/labels/15"}, Ports: []pce.IngressService{{Proto: 6, Port: 8443}}},
			{ConsumerHrefs: []string{"/orgs/1/labels/16"}, Ports: []pce.IngressService{{Proto: 6, Port: 5432}}},
		},
	)
	if len(rules) != 2 {
		t.Fatalf("rules = %d, want 2", len(rules))
	}
	r := rules[0]
	if r.Providers[0].Label.Href != "/orgs/1/labels/14" || r.Consumers[0].Label.Href != "/orgs/1/labels/15" {
		t.Errorf("rule actors = %+v", r)
	}
	if r.ResolveLabelsAs.Providers[0] != "workloads" || !r.UnscopedConsumers {
		t.Errorf("rule resolve/unscoped = %+v", r)
	}
	if len(r.IngressServices) != 1 || r.IngressServices[0].Port != 8443 {
		t.Errorf("ingress = %+v", r.IngressServices)
	}
}

func TestProtoNumber(t *testing.T) {
	if protoNumber("TCP") != 6 || protoNumber("UDP") != 17 || protoNumber("") != 6 {
		t.Errorf("protoNumber wrong")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/controller/ -run "BuildRuleSet|BuildRules|ProtoNumber" -v`
Expected: compile failure — `undefined: BuildRuleSet`.

- [ ] **Step 3: Write the implementation**

Create `internal/controller/policyir.go`:

```go
package controller

import (
	"fmt"

	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

// ResolvedAllow is an IntentAllow after consumer labels are resolved to hrefs.
type ResolvedAllow struct {
	ConsumerHrefs []string
	Ports         []pce.IngressService
}

// RuleSetName is the deterministic name of the ruleset for a CR.
func RuleSetName(namespace, crName string) string {
	return fmt.Sprintf("k8s-%s-%s", namespace, crName)
}

// BuildRuleSet builds the desired ruleset, scoped to the provider labels and
// stamped with ownership.
func BuildRuleSet(namespace, crName string, providerHrefs []string, owner pce.Owner) pce.RuleSet {
	scope := make([]pce.RuleSetScope, 0, len(providerHrefs))
	for _, h := range providerHrefs {
		scope = append(scope, pce.RuleSetScope{Label: pce.LabelRef{Href: h}})
	}
	return pce.RuleSet{
		Name:                  RuleSetName(namespace, crName),
		Enabled:               true,
		Scopes:                [][]pce.RuleSetScope{scope},
		ExternalDataSet:       owner.DataSet,
		ExternalDataReference: owner.Reference,
	}
}

// BuildRules builds one rule per allow entry: providers = the namespace's app
// labels; consumers = the allow's resolved labels; inline ports; pod resolution.
func BuildRules(providerHrefs []string, allows []ResolvedAllow) []pce.SecRule {
	providers := make([]pce.Actor, 0, len(providerHrefs))
	for _, h := range providerHrefs {
		href := h
		providers = append(providers, pce.Actor{Label: &pce.LabelRef{Href: href}})
	}
	rules := make([]pce.SecRule, 0, len(allows))
	for _, a := range allows {
		consumers := make([]pce.Actor, 0, len(a.ConsumerHrefs))
		for _, h := range a.ConsumerHrefs {
			href := h
			consumers = append(consumers, pce.Actor{Label: &pce.LabelRef{Href: href}})
		}
		rules = append(rules, pce.SecRule{
			Enabled:           true,
			ResolveLabelsAs:   pce.ResolveLabelsAs{Providers: []string{"workloads"}, Consumers: []string{"workloads"}},
			Providers:         providers,
			Consumers:         consumers,
			IngressServices:   a.Ports,
			UnscopedConsumers: true,
		})
	}
	return rules
}

// protoNumber maps a k8s protocol string to its IANA number (default TCP).
func protoNumber(protocol string) int {
	if protocol == "UDP" {
		return 17
	}
	return 6
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/controller/ -run "BuildRuleSet|BuildRules|ProtoNumber" -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/controller/policyir.go internal/controller/policyir_test.go
git commit -m "feat(controller): add pure policy IR (intent -> ruleset + rules)"
```

---

### Task 4: SegmentationIntent reconciler (resolve, guardrails, reconcile draft, auto-provision)

**Files:**
- Create: `internal/controller/segmentationintent_controller.go`
- Create: `internal/controller/policy_client.go`
- Test: `internal/controller/segmentationintent_controller_test.go`
- Modify: `internal/controller/suite_test.go` (register the reconciler with a fake policy client; add a managed ClusterProfile + namespace)
- Modify: `cmd/main.go` (wire the reconciler)

**Interfaces:**
- Consumes: `pce.RuleSet`/`SecRule`/`ProvisionResult`/`Owner`/`Label`/`IngressService`, `BuildRuleSet`/`BuildRules`/`ResolvedAllow`/`protoNumber`/`ComputeDesiredCWP` (Tasks 1, 3 + Plan 3); `SegmentationIntentSpec`, `ClusterProfileSpec`, condition reasons.
- Produces:
  - `type PolicyClient interface { FindLabel(ctx, key, value string) (*pce.Label, error); FindRuleSetByOwner(ctx, owner pce.Owner) (*pce.RuleSet, error); CreateRuleSet(ctx, rs pce.RuleSet) (*pce.RuleSet, error); DeleteRuleSet(ctx, href string) error; ListRules(ctx, ruleSetHref string) ([]pce.SecRule, error); CreateRule(ctx, ruleSetHref string, rule pce.SecRule) (*pce.SecRule, error); DeleteRule(ctx, ruleHref string) error; ProvisionRuleSets(ctx, hrefs []string, desc string) (*pce.ProvisionResult, error) }`
  - `type PolicyClientFactory func(cfg pce.Config) PolicyClient`; `DefaultPolicyClientFactory`
  - `SegmentationIntentReconciler{ client.Client; Scheme; NewPolicyClient PolicyClientFactory }`

**Reconcile behavior (this task: happy path + guardrails + auto/draft, NO finalizer/manual yet — those are Task 5):**
1. Get the `SegmentationIntent`. Find the cluster's `ClusterProfile` (list; the one that is Onboarded). If none ready → `Ready=False / ClusterProfileNotReady`, requeue 30s. Resolve the PCE `Config` from its `PCEConnection` (reuse the resolve pattern).
2. **Provider labels (guardrail):** compute the namespace's desired CWP labels via `ComputeDesiredCWP(ns.Name, ns.Labels, ns.Annotations, cp.Spec.NamespaceRules, cp.Spec.SystemNamespaces)` (fetch the Namespace object). If the namespace is **not managed** or has **no labels**, reject (`Rejected`, "namespace is not managed / has no Illumio identity"). Resolve each provider label `(key,value)` to an existing PCE label href via `FindLabel`; if any is missing, reject (the namespace's own labels should exist once Kubelink labeled it — requeue/Rejected with a clear message). These hrefs are the **provider** and the ruleset **scope**.
3. **Consumers (guardrail):** for each `allow`, resolve every `from` label `(key,value)` to an existing PCE label href via `FindLabel`. If any is missing → `Rejected` ("no Illumio label <key>=<value> in PCE"); do **not** create labels. Build `ResolvedAllow{ConsumerHrefs, Ports}` (ports via `protoNumber`).
4. **Compile:** `owner := pce.Owner{DataSet: <externalDataSet>, Reference: string(si.UID)}`; `desiredRS := BuildRuleSet(ns, si.Name, providerHrefs, owner)`; `desiredRules := BuildRules(providerHrefs, resolvedAllows)`.
5. **Reconcile ruleset:** `FindRuleSetByOwner(owner)`; if nil → `CreateRuleSet(desiredRS)`. Use the resulting ruleset href.
6. **Reconcile rules:** `ListRules(rsHref)`; delete all existing rules owned by this ruleset, then create the desired rules. (Simple replace-all is acceptable for v1 — a ruleset is small and single-owner; a smarter diff can come later.)
7. **Provision (auto/draft):** read `cp.Spec.ProvisioningMode` (default `manual`). For this task implement: `auto` → `ProvisionRuleSets([rsHref], "<ns>/<name>")`, set `Provisioned=True`, `status.WorkloadsAffected`. `draft-only` → `Provisioned=False / ProvisionPending` (no provision). (`manual` → treat like draft for now; Task 5 adds the approval gate.)
8. Set `Ready=True / Compiled`; `ObservedGeneration`; status update; requeue 10m.

> **Replace-all rules note:** deleting+recreating rules every reconcile is acceptable for v1 but means a provision shows churn. Task 5 can add diffing; keep it simple here. Always provision AFTER the draft is consistent.

- [ ] **Step 1: Write the failing test** — create `segmentationintent_controller_test.go`

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

var _ = Describe("SegmentationIntent controller", func() {
	const opNS = "default"

	It("compiles an intent to a ruleset, provisions (auto), and reports affected", func() {
		ctx := context.Background()

		// Managed namespace with an app label.
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "payments-si", Labels: map[string]string{"app.kubernetes.io/part-of": "payments"}},
		})).To(Succeed())

		// PCEConnection (Connected) + secret.
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "pce-creds-si", Namespace: opNS},
			Data:       map[string][]byte{keyAPIKey: []byte("k"), keyAPISecret: []byte("s")},
		})).To(Succeed())
		pc := &microv1.PCEConnection{
			ObjectMeta: metav1.ObjectMeta{Name: "pce-si"},
			Spec:       microv1.PCEConnectionSpec{PCEURL: testPCEURL, OrgID: 1, CredentialsSecretRef: microv1.SecretReference{Name: "pce-creds-si", Namespace: opNS}},
		}
		Expect(k8sClient.Create(ctx, pc)).To(Succeed())
		Eventually(func(g Gomega) {
			got := &microv1.PCEConnection{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "pce-si"}, got)).To(Succeed())
			meta.SetStatusCondition(&got.Status.Conditions, metav1.Condition{Type: microv1.ConditionConnected, Status: metav1.ConditionTrue, Reason: microv1.ReasonConnected, Message: "t"})
			g.Expect(k8sClient.Status().Update(ctx, got)).To(Succeed())
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

		// ClusterProfile: managed app namespace, auto provisioning, onboarded.
		cp := &microv1.ClusterProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "cp-si"},
			Spec: microv1.ClusterProfileSpec{
				PCEConnectionRef: microv1.LocalObjectReference{Name: "pce-si"},
				ProvisioningMode: "auto",
				Onboarding:       microv1.OnboardingSpec{ContainerClusterName: "ocp-si", CredentialsOutputSecret: "creds-si-out"},
				NamespaceRules: []microv1.NamespaceRule{
					{Match: microv1.NamespaceMatch{NamePattern: "payments-si"}, Managed: true,
						AssignLabels: map[string]microv1.LabelAssignment{"app": {FromNamespaceLabel: "app.kubernetes.io/part-of"}}},
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
			ObjectMeta: metav1.ObjectMeta{Name: "ingress", Namespace: "payments-si"},
			Spec: microv1.SegmentationIntentSpec{Allow: []microv1.IntentAllow{
				{From: map[string]string{"app": "checkout"}, Ports: []microv1.IntentPort{{Port: 8443, Protocol: "TCP"}}},
			}},
		}
		Expect(k8sClient.Create(ctx, si)).To(Succeed())

		Eventually(func(g Gomega) {
			got := &microv1.SegmentationIntent{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ingress", Namespace: "payments-si"}, got)).To(Succeed())
			g.Expect(meta.IsStatusConditionTrue(got.Status.Conditions, microv1.ConditionReady)).To(BeTrue())
			g.Expect(meta.IsStatusConditionTrue(got.Status.Conditions, microv1.ConditionProvisioned)).To(BeTrue())
			g.Expect(got.Status.WorkloadsAffected).To(Equal(2))
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())
	})

	It("rejects an intent whose consumer label does not exist in the PCE", func() {
		ctx := context.Background()
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "team-b-si", Labels: map[string]string{"app.kubernetes.io/part-of": "teamb"}},
		})).To(Succeed())
		si := &microv1.SegmentationIntent{
			ObjectMeta: metav1.ObjectMeta{Name: "bad", Namespace: "team-b-si"},
			Spec: microv1.SegmentationIntentSpec{Allow: []microv1.IntentAllow{
				{From: map[string]string{"app": "does-not-exist"}, Ports: []microv1.IntentPort{{Port: 80}}},
			}},
		}
		Expect(k8sClient.Create(ctx, si)).To(Succeed())
		Eventually(func(g Gomega) {
			got := &microv1.SegmentationIntent{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "bad", Namespace: "team-b-si"}, got)).To(Succeed())
			c := meta.FindStatusCondition(got.Status.Conditions, microv1.ConditionReady)
			g.Expect(c).NotTo(BeNil())
			g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(c.Reason).To(Equal(microv1.ReasonRejected))
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())
	})
})
```

- [ ] **Step 2: Extend the suite fake + register the reconciler**

In `internal/controller/suite_test.go`, add a fake policy client and register the reconciler. The fake resolves the namespace's own label (`app=payments`) and the consumer `app=checkout` as existing, but returns not-found for `app=does-not-exist`; records created rules; provisions returning `workloads_affected: 2`:

```go
type fakePolicyClient struct{}

func (fakePolicyClient) FindLabel(_ context.Context, key, value string) (*pce.Label, error) {
	if value == "does-not-exist" {
		return nil, pce.ErrLabelNotFound
	}
	return &pce.Label{Href: "/orgs/1/labels/" + key + "-" + value, Key: key, Value: value}, nil
}
func (fakePolicyClient) FindRuleSetByOwner(context.Context, pce.Owner) (*pce.RuleSet, error) { return nil, nil }
func (fakePolicyClient) CreateRuleSet(_ context.Context, rs pce.RuleSet) (*pce.RuleSet, error) {
	rs.Href = "/orgs/1/sec_policy/draft/rule_sets/843"
	return &rs, nil
}
func (fakePolicyClient) DeleteRuleSet(context.Context, string) error { return nil }
func (fakePolicyClient) ListRules(context.Context, string) ([]pce.SecRule, error) { return nil, nil }
func (fakePolicyClient) CreateRule(_ context.Context, _ string, rule pce.SecRule) (*pce.SecRule, error) {
	rule.Href = "/orgs/1/sec_policy/draft/rule_sets/843/sec_rules/1"
	return &rule, nil
}
func (fakePolicyClient) DeleteRule(context.Context, string) error { return nil }
func (fakePolicyClient) ProvisionRuleSets(context.Context, []string, string) (*pce.ProvisionResult, error) {
	return &pce.ProvisionResult{Version: 80, WorkloadsAffected: 2}, nil
}
```

Register (alongside the others):

```go
err = (&SegmentationIntentReconciler{
	Client:          k8sManager.GetClient(),
	Scheme:          k8sManager.GetScheme(),
	NewPolicyClient: func(pce.Config) PolicyClient { return fakePolicyClient{} },
}).SetupWithManager(k8sManager)
Expect(err).ToNot(HaveOccurred())
```

> The "rejects" test relies on the fake returning `ErrLabelNotFound` for `does-not-exist`. The reconciler must treat `team-b-si` as managed (the suite ClusterProfile `cp-si` only matches `payments-si`; add a rule to `cp-si` or a second ClusterProfile so `team-b-si` is managed with `app=teamb` — OR set systemNamespaces off and make the reject test's failure come from the namespace having an app label resolved fine but the consumer missing). Simplest: extend `cp-si.spec.namespaceRules` with a rule `{match:{namePattern:"team-b-si"}, managed:true, assignLabels:{app:{fromNamespaceLabel:"app.kubernetes.io/part-of"}}}` so the provider resolves and the rejection is purely due to the missing consumer label.

- [ ] **Step 3: Run to verify it fails** — `make test` → compile failure (`undefined: SegmentationIntentReconciler`, `PolicyClient`).

- [ ] **Step 4: Write the policy client interface** — create `internal/controller/policy_client.go`:

```go
package controller

import (
	"context"

	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

// PolicyClient is the subset of the PCE client the SegmentationIntent
// controller needs. The real *pce.Client satisfies it.
type PolicyClient interface {
	FindLabel(ctx context.Context, key, value string) (*pce.Label, error)
	FindRuleSetByOwner(ctx context.Context, owner pce.Owner) (*pce.RuleSet, error)
	CreateRuleSet(ctx context.Context, rs pce.RuleSet) (*pce.RuleSet, error)
	DeleteRuleSet(ctx context.Context, href string) error
	ListRules(ctx context.Context, ruleSetHref string) ([]pce.SecRule, error)
	CreateRule(ctx context.Context, ruleSetHref string, rule pce.SecRule) (*pce.SecRule, error)
	DeleteRule(ctx context.Context, ruleHref string) error
	ProvisionRuleSets(ctx context.Context, hrefs []string, desc string) (*pce.ProvisionResult, error)
}

var _ PolicyClient = (*pce.Client)(nil)

// PolicyClientFactory builds a PolicyClient (injectable for tests).
type PolicyClientFactory func(cfg pce.Config) PolicyClient

// DefaultPolicyClientFactory wraps the real PCE client.
func DefaultPolicyClientFactory(cfg pce.Config) PolicyClient { return pce.NewClient(cfg) }
```

- [ ] **Step 5: Write the reconciler** — create `internal/controller/segmentationintent_controller.go`:

```go
package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

const (
	siRequeueNotReady = 30 * time.Second
	siRequeueHealthy  = 10 * time.Minute
)

// SegmentationIntentReconciler reconciles a SegmentationIntent.
type SegmentationIntentReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	NewPolicyClient PolicyClientFactory
}

// +kubebuilder:rbac:groups=microsegment.io,resources=segmentationintents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=microsegment.io,resources=segmentationintents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=microsegment.io,resources=segmentationintents/finalizers,verbs=update
// +kubebuilder:rbac:groups=microsegment.io,resources=clusterprofiles,verbs=get;list;watch
// +kubebuilder:rbac:groups=microsegment.io,resources=pceconnections,verbs=get;list;watch

func (r *SegmentationIntentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var si microv1.SegmentationIntent
	if err := r.Get(ctx, req.NamespacedName, &si); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if r.NewPolicyClient == nil {
		r.NewPolicyClient = DefaultPolicyClientFactory
	}

	cp, cfg, eds, ready, transientErr := r.resolveClusterProfile(ctx)
	if transientErr != nil {
		return ctrl.Result{}, transientErr
	}
	if !ready {
		return r.fail(ctx, &si, microv1.ConditionReady, microv1.ReasonClusterProfileNotReady,
			"no Onboarded ClusterProfile / PCEConnection available", siRequeueNotReady)
	}

	pclient := r.NewPolicyClient(cfg)
	owner := pce.Owner{DataSet: eds, Reference: string(si.UID)}

	// Provider labels = the namespace's own resolved CWP labels (guardrail).
	providerHrefs, reason, msg, ok, err := r.resolveProvider(ctx, &si, cp, pclient, owner)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !ok {
		return r.fail(ctx, &si, microv1.ConditionReady, reason, msg, siRequeueHealthy)
	}

	// Consumers must resolve to existing PCE labels (guardrail).
	resolved, reason, msg, ok, err := r.resolveAllows(ctx, &si, pclient)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !ok {
		return r.fail(ctx, &si, microv1.ConditionReady, reason, msg, siRequeueHealthy)
	}

	// Compile + reconcile the owned ruleset and its rules in draft.
	rsHref, err := r.reconcileRuleSet(ctx, &si, pclient, owner, providerHrefs, resolved)
	if err != nil {
		return ctrl.Result{}, err
	}

	meta.SetStatusCondition(&si.Status.Conditions, metav1.Condition{
		Type: microv1.ConditionReady, Status: metav1.ConditionTrue, Reason: microv1.ReasonCompiled,
		Message: "compiled to Illumio ruleset",
	})

	// Provision per the cluster's mode (this task: auto + draft-only; manual == draft for now).
	if cp.Spec.ProvisioningMode == "auto" {
		res, perr := pclient.ProvisionRuleSets(ctx, []string{rsHref}, fmt.Sprintf("%s/%s", si.Namespace, si.Name))
		if perr != nil {
			return ctrl.Result{}, perr
		}
		si.Status.WorkloadsAffected = res.WorkloadsAffected
		meta.SetStatusCondition(&si.Status.Conditions, metav1.Condition{
			Type: microv1.ConditionProvisioned, Status: metav1.ConditionTrue, Reason: microv1.ReasonProvisioned,
			Message: fmt.Sprintf("provisioned; %d workloads affected", res.WorkloadsAffected),
		})
	} else {
		meta.SetStatusCondition(&si.Status.Conditions, metav1.Condition{
			Type: microv1.ConditionProvisioned, Status: metav1.ConditionFalse, Reason: microv1.ReasonProvisionPending,
			Message: "draft written; provisioning is " + cp.Spec.ProvisioningMode,
		})
	}

	si.Status.ObservedGeneration = si.Generation
	if err := r.Status().Update(ctx, &si); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: siRequeueHealthy}, nil
}

// resolveClusterProfile finds an Onboarded ClusterProfile and loads its PCE config.
func (r *SegmentationIntentReconciler) resolveClusterProfile(ctx context.Context) (*microv1.ClusterProfile, pce.Config, string, bool, error) {
	var list microv1.ClusterProfileList
	if err := r.List(ctx, &list); err != nil {
		return nil, pce.Config{}, "", false, err
	}
	for i := range list.Items {
		cp := &list.Items[i]
		if !meta.IsStatusConditionTrue(cp.Status.Conditions, microv1.ConditionOnboarded) {
			continue
		}
		var conn microv1.PCEConnection
		if err := r.Get(ctx, types.NamespacedName{Name: cp.Spec.PCEConnectionRef.Name}, &conn); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return nil, pce.Config{}, "", false, err
		}
		if !meta.IsStatusConditionTrue(conn.Status.Conditions, microv1.ConditionConnected) {
			continue
		}
		var secret corev1.Secret
		key := types.NamespacedName{Name: conn.Spec.CredentialsSecretRef.Name, Namespace: conn.Spec.CredentialsSecretRef.Namespace}
		if err := r.Get(ctx, key, &secret); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return nil, pce.Config{}, "", false, err
		}
		apiKey, apiSecret := string(secret.Data["api_key"]), string(secret.Data["api_secret"])
		if apiKey == "" || apiSecret == "" {
			continue
		}
		eds := conn.Spec.ExternalDataSet
		if eds == "" {
			eds = "illumio-operator"
		}
		return cp, pce.Config{PCEURL: conn.Spec.PCEURL, OrgID: conn.Spec.OrgID, APIKey: apiKey, APISecret: apiSecret}, eds, true, nil
	}
	return nil, pce.Config{}, "", false, nil
}

// resolveProvider derives the namespace's own labels and resolves them to hrefs.
func (r *SegmentationIntentReconciler) resolveProvider(ctx context.Context, si *microv1.SegmentationIntent, cp *microv1.ClusterProfile, pclient PolicyClient, owner pce.Owner) ([]string, string, string, bool, error) {
	var ns corev1.Namespace
	if err := r.Get(ctx, types.NamespacedName{Name: si.Namespace}, &ns); err != nil {
		return nil, microv1.ReasonRejected, "namespace not found", false, client.IgnoreNotFound(err)
	}
	desired := ComputeDesiredCWP(ns.Name, ns.Labels, ns.Annotations, cp.Spec.NamespaceRules, cp.Spec.SystemNamespaces)
	if !desired.Managed || len(desired.Labels) == 0 {
		return nil, microv1.ReasonRejected, "namespace is not managed or has no Illumio labels; an admin must manage it via ClusterProfile", false, nil
	}
	hrefs := make([]string, 0, len(desired.Labels))
	for key, value := range desired.Labels {
		lbl, err := pclient.FindLabel(ctx, key, value)
		if err != nil {
			if errors.Is(err, pce.ErrLabelNotFound) {
				return nil, microv1.ReasonRejected, fmt.Sprintf("namespace label %s=%s not yet in the PCE", key, value), false, nil
			}
			return nil, "", "", false, err
		}
		hrefs = append(hrefs, lbl.Href)
	}
	return hrefs, "", "", true, nil
}

// resolveAllows resolves consumer labels to existing PCE hrefs (never creates).
func (r *SegmentationIntentReconciler) resolveAllows(ctx context.Context, si *microv1.SegmentationIntent, pclient PolicyClient) ([]ResolvedAllow, string, string, bool, error) {
	out := make([]ResolvedAllow, 0, len(si.Spec.Allow))
	for _, a := range si.Spec.Allow {
		consumerHrefs := make([]string, 0, len(a.From))
		for key, value := range a.From {
			lbl, err := pclient.FindLabel(ctx, key, value)
			if err != nil {
				if errors.Is(err, pce.ErrLabelNotFound) {
					return nil, microv1.ReasonRejected, fmt.Sprintf("no Illumio label %s=%s in the PCE", key, value), false, nil
				}
				return nil, "", "", false, err
			}
			consumerHrefs = append(consumerHrefs, lbl.Href)
		}
		ports := make([]pce.IngressService, 0, len(a.Ports))
		for _, p := range a.Ports {
			ports = append(ports, pce.IngressService{Proto: protoNumber(p.Protocol), Port: p.Port})
		}
		out = append(out, ResolvedAllow{ConsumerHrefs: consumerHrefs, Ports: ports})
	}
	return out, "", "", true, nil
}

// reconcileRuleSet ensures the owned ruleset exists and its rules match desired
// (replace-all). Returns the ruleset href.
func (r *SegmentationIntentReconciler) reconcileRuleSet(ctx context.Context, si *microv1.SegmentationIntent, pclient PolicyClient, owner pce.Owner, providerHrefs []string, resolved []ResolvedAllow) (string, error) {
	existing, err := pclient.FindRuleSetByOwner(ctx, owner)
	if err != nil {
		return "", err
	}
	var rsHref string
	if existing == nil {
		created, cerr := pclient.CreateRuleSet(ctx, BuildRuleSet(si.Namespace, si.Name, providerHrefs, owner))
		if cerr != nil {
			return "", cerr
		}
		rsHref = created.Href
	} else {
		rsHref = existing.Href
		// Remove existing rules (replace-all).
		rules, lerr := pclient.ListRules(ctx, rsHref)
		if lerr != nil {
			return "", lerr
		}
		for i := range rules {
			if derr := pclient.DeleteRule(ctx, rules[i].Href); derr != nil {
				return "", derr
			}
		}
	}
	for _, rule := range BuildRules(providerHrefs, resolved) {
		if _, cerr := pclient.CreateRule(ctx, rsHref, rule); cerr != nil {
			return "", cerr
		}
	}
	return rsHref, nil
}

func (r *SegmentationIntentReconciler) fail(ctx context.Context, si *microv1.SegmentationIntent, condType, reason, msg string, requeue time.Duration) (ctrl.Result, error) {
	meta.SetStatusCondition(&si.Status.Conditions, metav1.Condition{
		Type: condType, Status: metav1.ConditionFalse, Reason: reason, Message: msg,
	})
	si.Status.ObservedGeneration = si.Generation
	if err := r.Status().Update(ctx, si); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: requeue}, nil
}

func (r *SegmentationIntentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&microv1.SegmentationIntent{}).
		Complete(r)
}
```

- [ ] **Step 6: Wire main** — in `cmd/main.go`, register:

```go
if err := (&controller.SegmentationIntentReconciler{
	Client:          mgr.GetClient(),
	Scheme:          mgr.GetScheme(),
	NewPolicyClient: controller.DefaultPolicyClientFactory,
}).SetupWithManager(mgr); err != nil {
	setupLog.Error(err, "unable to create controller", "controller", "SegmentationIntent")
	os.Exit(1)
}
```

- [ ] **Step 7: Run tests/build/lint** — `make test` (both specs pass), `make build`, `make lint` (0 issues; extract constants on goconst). Use a fresh linter (`rm -f bin/golangci-lint && make lint`) to match CI.

- [ ] **Step 8: Commit**

```bash
git add internal/controller/ cmd/main.go config/rbac/
git commit -m "feat(controller): compile SegmentationIntent to provisioned Illumio policy"
```

---

### Task 5: Provisioning modes (manual) + finalizer cleanup

**Files:**
- Modify: `internal/controller/segmentationintent_controller.go`
- Modify: `internal/controller/segmentationintent_controller_test.go` (add manual + delete specs)

**Interfaces:**
- Consumes: Task 4 reconciler; `AnnotationProvisionApprove`.
- Produces: `manual` provisioning gate (provision only when the CR is annotated `microsegment.io/provision: approved`); a finalizer that deletes the owned ruleset (draft) and provisions the removal on CR deletion.

- [ ] **Step 1: Write the failing tests** — add to `segmentationintent_controller_test.go`:
  - **manual:** a ClusterProfile with `provisioningMode: manual`; a SegmentationIntent without the approve annotation → `Provisioned=False / ProvisionPending`; then add annotation `microsegment.io/provision: approved` → eventually `Provisioned=True`. (The fake's `ProvisionRuleSets` returns affected>0.)
  - **delete:** create + reconcile an intent (finalizer added), then delete it → eventually the object is gone (finalizer ran). Assert via a recorder on the fake that `DeleteRuleSet` was called (mutex-guarded recorder, same pattern as Plan 3's CWP recorder).

- [ ] **Step 2: Add the finalizer + manual gate**

Define `const segIntentFinalizer = "microsegment.io/segmentationintent"`.

In `Reconcile`, near the top (after Get), handle deletion and ensure the finalizer:

```go
	if !si.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&si, segIntentFinalizer) {
			if err := r.finalize(ctx, &si); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(&si, segIntentFinalizer)
			if err := r.Update(ctx, &si); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}
	if controllerutil.AddFinalizer(&si, segIntentFinalizer) {
		if err := r.Update(ctx, &si); err != nil {
			return ctrl.Result{}, err
		}
	}
```

(Import `"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"`.)

Change the provisioning block so `manual` provisions only when approved:

```go
	approved := si.Annotations[microv1.AnnotationProvisionApprove] == "approved"
	doProvision := cp.Spec.ProvisioningMode == "auto" ||
		(cp.Spec.ProvisioningMode == "manual" && approved)
	if doProvision {
		// ... existing provision + Provisioned=True ...
	} else {
		// Provisioned=False / ProvisionPending (message names the mode and, for
		// manual, that it awaits the microsegment.io/provision=approved annotation)
	}
```

Add `finalize` — best-effort delete + provision of the removal (so the rules actually stop applying):

```go
func (r *SegmentationIntentReconciler) finalize(ctx context.Context, si *microv1.SegmentationIntent) error {
	cp, cfg, eds, ready, err := r.resolveClusterProfile(ctx)
	if err != nil {
		return err
	}
	if !ready {
		// Can't reach the PCE; allow deletion to proceed rather than blocking k8s
		// object removal indefinitely. (Orphaned draft ruleset is cleaned on next
		// full reconcile or manually.)
		return nil
	}
	_ = cp
	pclient := r.NewPolicyClient(cfg)
	owner := pce.Owner{DataSet: eds, Reference: string(si.UID)}
	rs, err := pclient.FindRuleSetByOwner(ctx, owner)
	if err != nil || rs == nil {
		return err
	}
	if err := pclient.DeleteRuleSet(ctx, rs.Href); err != nil {
		return err
	}
	_, err = pclient.ProvisionRuleSets(ctx, []string{rs.Href}, "delete "+si.Namespace+"/"+si.Name)
	return err
}
```

> Note: provisioning a deleted draft object (by its draft href) commits the deletion — this matches the PCE "deletions must be provisioned" rule.

- [ ] **Step 3: Run tests/build/lint** — `make test` (manual + delete specs pass), `make build`, `rm -f bin/golangci-lint && make lint` (0 issues).

- [ ] **Step 4: Commit**

```bash
git add internal/controller/
git commit -m "feat(controller): manual provisioning gate and finalizer cleanup for SegmentationIntent"
```

---

### Task 6: Segmentation policy documentation

**Files:**
- Create: `docs/guides/segmentation-policy.md`
- Create: `docs/reference/segmentationintent.md`
- Modify: `docs/getting-started.md`, `mkdocs.yml` (nav)
- Run: `make docs-api` (regenerate `docs/reference/api.md`).

> **Dispatch note:** documentation subagent. Content only; cross-check every field against `segmentationintent_types.go`.

- [ ] **Step 1: Write the guide** — `docs/guides/segmentation-policy.md` covering: the app-team mental model ("allow these consumers to reach my app's pods on these ports"); a complete `SegmentationIntent` example; how the operator compiles it (one owned Illumio ruleset scoped to the namespace's app label, label-based rules resolved as workloads, ports inlined); the **guardrails** (the provider is always your namespace's own app — you can't protect another team's app; consumers must reference Illumio labels that already exist, i.e. apps Kubelink has labeled; unknown labels are rejected); the **provisioning modes** (`auto` provisions immediately; `manual` writes a draft and waits for `kubectl annotate segmentationintent <name> microsegment.io/provision=approved`; `draft-only` never provisions — set via `ClusterProfile.spec.provisioningMode`); that **enforcement is separate** (rules only block when the namespace is in `full` enforcement — set via the namespace's CWP, Plan 3); how to read status (`Ready`/`Provisioned` conditions, `Affected` column / `status.workloadsAffected`); and that deleting the CR removes and re-provisions the ruleset. Example:

```yaml
apiVersion: microsegment.io/v1alpha1
kind: SegmentationIntent
metadata:
  name: payments-ingress
  namespace: payments
spec:
  allow:
    - from: { app: checkout, env: prod }
      ports:
        - { port: 8443, protocol: TCP }
    - from: { app: ledger, env: prod }
      ports:
        - { port: 5432, protocol: TCP }
```

- [ ] **Step 2: Write the reference** — `docs/reference/segmentationintent.md`: spec (`allow[].from` map of Illumio label key→value; `allow[].ports[].{port,protocol}`), the `microsegment.io/provision` annotation, status conditions (`Ready` reasons `Compiled`/`Rejected`/`ClusterProfileNotReady`; `Provisioned` reasons `Provisioned`/`ProvisionPending`) and `workloadsAffected`, in the existing table style.

- [ ] **Step 3: getting-started + nav** — add a "Write segmentation policy" step to `docs/getting-started.md`; add `- Segmentation policy: guides/segmentation-policy.md` (Guides) and `- SegmentationIntent: reference/segmentationintent.md` (API Reference) to `mkdocs.yml`.

- [ ] **Step 4: Regenerate + build** — `make docs-api`; `mkdocs build --strict` (or rely on Docs CI). `docs/reference/api.md` must include `SegmentationIntent`.

- [ ] **Step 5: Commit**

```bash
git add docs/ mkdocs.yml
git commit -m "docs: add segmentation policy guide and SegmentationIntent reference"
```

---

## Self-Review

**Spec coverage (design spec §2, §4.5, §5, §6, §7, §8, §10):**
- §4.5 `SegmentationIntent` (namespaced, `allow[].from`/`ports`) → Task 2. ✓
- §7 two-front-ends/one-backend — the IR + reconciler are the shared backend; `SegmentationIntent` is the first front-end (the `SegmentationPolicy` NetworkPolicy-style front-end reuses this IR in Plan 5) → Tasks 1, 3, 4. ✓
- §5 guardrails — provider locked to the namespace's own labels; consumers must resolve to existing PCE labels (never created); no `[[]]`/estate-wide scope → Task 4 `resolveProvider`/`resolveAllows`/`BuildRuleSet`. ✓
- §8 ownership tagging (one ruleset per CR, scoped provisioning of only that ruleset) → Tasks 1, 3, 4. ✓
- §6/§8 provisioning (draft → provision the change_subset of our ruleset; `workloads_affected` surfaced) with modes auto/manual/draft-only → Tasks 4 (auto/draft) + 5 (manual gate). ✓
- §10 status conditions + finalizer cleanup → Tasks 4, 5. ✓
- Docs from the start → Task 6. ✓

**Out of Plan 4 scope (deferred to Plan 5):** the `SegmentationPolicy` (NetworkPolicy-style) front-end over the same IR; the per-policy **enforcement-strictest** resolution that raises the namespace's CWP enforcement (spec §5.1); rule-level diffing (Plan 4 uses replace-all). Plan 4's enforcement stays admin-driven (Plan 3 CWPs); writing rules is independent of enforcement mode (rules compute; enforcement blocks).

**Placeholder scan:** No `TBD`/"handle edge cases" — code steps are complete. The `init()` registration note (Task 2) and the suite-fake managed-namespace note (Task 4 Step 2) give explicit resolutions, not placeholders.

**Type consistency:** `pce.RuleSet`/`SecRule`/`RuleSetScope`/`Actor`/`IngressService`/`ResolveLabelsAs`/`ProvisionResult`/`Owner`/`Label`; `BuildRuleSet`/`BuildRules`/`RuleSetName`/`ResolvedAllow`/`protoNumber`; `PolicyClient`/`PolicyClientFactory`/`DefaultPolicyClientFactory`; `SegmentationIntentSpec`/`IntentAllow`/`IntentPort`; condition reasons; reuses `ComputeDesiredCWP` (Plan 3) and the `FindLabel`/`ErrLabelNotFound`/`pce.Config` (Plan 1) without redefining them.
