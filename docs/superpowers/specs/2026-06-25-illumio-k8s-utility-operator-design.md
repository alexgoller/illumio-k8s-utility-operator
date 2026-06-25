# Illumio Kubernetes Utility Operator — Design Spec

**Date:** 2026-06-25
**Status:** Draft for review
**Author:** Alex Goller (with Claude)

---

## 1. Problem & Goal

Operating Illumio Core on Kubernetes/OpenShift requires a lot of manual, PCE-console-side work that does not fit how Kubernetes teams want to work:

1. **Container Workload Profile (CWP) toil.** Kubelink auto-creates a CWP per namespace, but every one defaults to `unmanaged` with no labels and no policy. On OpenShift, dozens of `openshift-*`/`kube-*` system namespaces must each be configured by hand in the PCE console — tedious and easy to get wrong.
2. **Policy adoption.** Application teams will not use the Illumio console or Terraform to express segmentation policy. They want to stay in Kubernetes (`kubectl`/GitOps). Today the only way to author Illumio policy is exactly the path they reject, so they fall back to native `NetworkPolicy` and Illumio policy goes unused.

**Goal:** a Kubernetes operator that turns declarative CRDs into reconciled PCE state via the Illumio REST API, so that:
- Platform admins prepare and manage CWPs declaratively (especially the OpenShift system namespaces) from a single source of truth.
- Application teams author Illumio segmentation policy in Kubernetes — never touching the console or Terraform — with guardrails that keep them from harming the wider estate.

---

## 2. Scope & Boundary

The operator is the **PCE-side automation brain**: a client of the **Illumio PCE REST API v2**. It reconciles desired state (CRDs) into PCE objects.

**In scope:**
- **Onboarding** — ensure the **Container Cluster** object exists in the PCE; publish the resulting credentials for the agents (see §6).
- **CWP / namespace labeling** — reconcile the auto-created CWPs: `managed` flag, **namespace (CWP) labels**, and enforcement state. Setting CWP labels across all namespaces (especially the OpenShift system namespaces) is the big initial effort the operator automates. *Pod/workload* labeling is done automatically by Kubelink's LabelMap; the operator does **not** read or manage the LabelMap (§4.3).
- **Segmentation policy** — compile application-team CRDs into PCE rulesets/rules/services and provision them.

**Explicitly out of scope:**
- **Deploying or upgrading the C-VEN / Kubelink agents.** This stays with the official **Helm chart** (`oci://quay.io/illumio/illumio`). The operator never runs Helm.
- Acting as a native `NetworkPolicy` controller or CNI. The operator does not enforce traffic itself; the C-VEN does.

**Target mode: CLAS.** The operator targets **CLAS** (Cluster Local Actor Store, Illumio Core for Kubernetes ≥ 5.0.0) exclusively. Legacy mode is not a target. This fixes the workload object model (Kubernetes Workloads, not per-Pod Container Workloads), annotation placement, and label precedence to a single, current behavior — no dual-mode branching.

**Known boundary risks (documented, not solved by the operator):**
- **Firewall coexistence.** In *exclusive* mode the C-VEN flushes non-Illumio iptables rules, which can break an iptables-based CNI / native NetworkPolicy. This is a deployment concern owned by Helm/cluster config; the operator documents and (where visible) warns, but does not manage it.

---

## 3. Personas

- **Platform / Security admin (Persona A)** — owns cluster onboarding, the namespace→CWP rules and CWP labels, and the default provisioning behavior. Cluster-scoped CRDs.
- **Application team (Persona B)** — owns segmentation policy for *their own* namespace. Namespaced CRDs. Can only compose within the vocabulary and guardrails the admin defines.

---

## 4. CRD Set

**CRDs this operator defines:**

| CRD | Scope | Persona | Purpose |
|---|---|---|---|
| `PCEConnection` | cluster | A / platform | PCE endpoint (`pceUrl`, `orgId`) + reference to the Secret holding the operator's API key/secret. One per PCE. |
| `ClusterProfile` | cluster | A | Onboarding + ordered namespace→CWP rules (`managed` flag, fixed labels for system namespaces) + default `provisioningMode`. Ships built-in defaults for `openshift-*`/`kube-*`. |
| `SegmentationPolicy` | namespaced | B | **NetworkPolicy-style** policy front-end. |
| `SegmentationIntent` | namespaced | B | **Simplified intent** policy front-end. |

> **Not our concern:** Illumio's own `LabelMap` CRD (`ic4k.illumio.com/v1alpha1`, Kubelink-owned) already publishes each deployment's k8s labels onto its pods/workloads as Illumio labels. The operator does **not** define, read, or manage it. It only labels the **CWP/namespace** level, which the LabelMap does not touch (§4.3).

`SegmentationPolicy` and `SegmentationIntent` are **two front-ends over one backend** (§7). Both ship in v1.

### 4.0 API conventions & naming
- **API group:** `microsegment.io` (product-brand group). The group — not the Kind — namespaces these CRDs and keeps them visibly distinct from Illumio's own `ic4k.illumio.com` CRDs, e.g. `kubectl api-resources | grep microsegment.io`.
- **Kinds stay clean** (`LabelMap`, `SegmentationPolicy`, …); no `Illumio` prefix (redundant with the group).
- **Category `illumio`** registered on every CRD so `kubectl get illumio` lists all operator objects; plus shortNames (e.g. `segpol`, `cprof`).
- **Version:** start at `v1alpha1`.

### 4.1 `PCEConnection` (sketch)
```yaml
apiVersion: microsegment.io/v1alpha1
kind: PCEConnection
metadata:
  name: prod-pce
spec:
  pceUrl: mypce.example.com:8443      # host:port (443 for SaaS)
  orgId: 3
  credentialsSecretRef:
    name: illumio-pce-api             # Secret with keys: api_key, api_secret
  externalDataSet: illumio-operator   # ownership tag stamped on PCE objects (see §8)
status:
  conditions: [...]                   # Connected / AuthFailed / RateLimited
```

### 4.2 `ClusterProfile` (sketch)
```yaml
apiVersion: microsegment.io/v1alpha1
kind: ClusterProfile
metadata:
  name: this-cluster
spec:
  pceConnectionRef: { name: prod-pce }

  onboarding:
    containerClusterName: ocp-prod-01
    credentialsOutputSecret: illumio-cluster-creds   # operator WRITES this (see §6)

  defaults:
    provisioningMode: manual          # auto | manual | draft-only

  # Ordered rules: first match wins. Each rule decides `managed` and the
  # CWP/namespace labels. Labels may be fixed values or derived from the
  # namespace's own k8s labels via `fromNamespaceLabel`. (Pod labels are
  # separately handled by Kubelink's LabelMap — not here.)
  namespaceRules:
    - match: { namePattern: "openshift-*" }
      managed: true
      assignLabels: { role: control, env: prod, loc: eu-west }
    - match: { namePattern: "kube-*" }
      managed: true
      assignLabels: { role: control, env: prod, loc: eu-west }
    - match: { labels: { "microsegment.io/managed": "true" } }
      managed: true
      assignLabels:
        app:  { fromNamespaceLabel: app.kubernetes.io/part-of }
        env:  { fromNamespaceLabel: app.kubernetes.io/environment }
    - match: { namePattern: "*" }        # default catch-all
      managed: false
status:
  conditions: [...]
  managedNamespaces: 37
  effectiveProfiles: [...]              # per-namespace resolved view
```

### 4.3 Workload labels vs CWP labels (division of responsibility)
There are two distinct labeling concerns in Illumio, and the operator owns only one of them:

| Concern | Who does it | How |
|---|---|---|
| **Pod / Workload labels** | **Kubelink + LabelMap** (Illumio) | The existing `ic4k.illumio.com` `LabelMap` maps each deployment's k8s labels onto its Illumio workload labels automatically. The operator does **not** read, manage, or depend on it. |
| **CWP / namespace labels** | **This operator** | The LabelMap does **not** set namespace-level CWP labels. The operator does, via `ClusterProfile.namespaceRules` — the large initial effort, especially for OpenShift system namespaces. |

**How the operator uses Illumio labels:**
- **Setting CWP labels (persona A).** For each managed namespace, the operator resolves the rule's `assignLabels` (fixed values or `fromNamespaceLabel`) to Illumio label `href`s via the PCE `/labels` API. For admin-defined CWP labels, it may **create-if-missing** (bounded by the rules, ownership-tagged §8) — e.g. `role=control` that no workload would have produced.
- **Resolving policy references (persona B).** Consumer references in a policy CR resolve to **existing** Illumio labels in the PCE (the ones Kubelink already created from real workloads). If a referenced label does not exist, the operator **never creates it** — it sets the CR to `Rejected`/pending with a clear reason and requeues.

This split keeps app teams using their normal k8s labels (no `com.illumio.*` annotations, no operator involvement in pod labeling) while the operator focuses on the CWP-level work the LabelMap leaves undone.

### 4.4 Per-namespace override
A `Namespace` object may carry annotations that override the central rule for that namespace (e.g. `microsegment.io/managed`, `microsegment.io/env`). **Precedence: central rule match → per-namespace annotation → operator default.** Resolution is deterministic and surfaced in `ClusterProfile.status.effectiveProfiles`.

### 4.5 `SegmentationIntent` (sketch — intent front-end)
```yaml
apiVersion: microsegment.io/v1alpha1
kind: SegmentationIntent
metadata:
  name: payments-ingress
  namespace: payments
spec:
  enforcement: visibility_only          # idle | visibility_only | full (per-policy; resolved per §5)
  allow:
    # 'from' references existing Illumio labels in the PCE (key: value), which
    # Kubelink already created from the consumer app's workloads (§4.3).
    - from: { app: checkout, env: prod }
      ports: [ { port: 8443, protocol: TCP } ]
    - from: { app: ledger,   env: prod }
      ports: [ { port: 5432, protocol: TCP } ]
status:
  conditions: [...]                     # Ready / Rejected(reason) / Provisioned
  effectiveEnforcement: visibility_only
  enforcementSetBy: payments-ingress
  workloadsAffected: 12
```

### 4.6 `SegmentationPolicy` (sketch — NetworkPolicy-style front-end)
Mirrors a supported **subset** of k8s `NetworkPolicy` (ingress/egress, selectors mapped to Illumio labels, ports). Constructs outside the subset are **rejected** with a clear status reason (§5). Lowers into the same IR as `SegmentationIntent`.

---

## 5. Policy Semantics & Guardrails (Persona B)

The operator is a **guardrail**, not just a translator. For every app-team CR:

1. **Provider is locked to the namespace's own identity.** The operator derives the provider (the protected app) from the namespace's resolved CWP labels. A team can only write rules protecting *their own* app's inbound — never put another app in provider position.
2. **Consumers must resolve to an existing Illumio label.** Any consumer reference (`from:`) must resolve to a label that already exists in the PCE (created by Kubelink from real workloads). Unknown label ⇒ CR **rejected**/pending with `status.conditions: Rejected(reason="no Illumio label app=foo in PCE")` and requeue. The operator **never creates** labels for policy references. Optionally, an admin allowlist can further restrict which existing labels persona B may reference.
3. **Supported-subset enforcement (fail loud).** NetworkPolicy-style constructs that cannot map faithfully to the Illumio model (e.g. `ipBlock` with `except`, certain selector combinations, egress semantics that don't translate) are **rejected**, never approximated. *A half-correct firewall rule is a vulnerability.* The supported subset is documented and versioned.
4. **No estate-wide scope.** The operator never emits `All|All|All` scopes or `unscoped_consumers: true` from an app-team CR. Scope is always bound to the namespace's labels.

### 5.1 Enforcement state resolution
Enforcement is a **per-policy field**, but Illumio enforcement for containers is a **namespace (CWP) property** — all pods in a namespace inherit one state. When multiple CRs share a namespace, the operator applies the **strictest** requested state:

```
idle  <  visibility_only  <  full
```

The resolved value is written to the CWP, and every CR in that namespace reports `status.effectiveEnforcement` and `status.enforcementSetBy` (the CR that won). `selective` is **not supported** for containers and is rejected if requested.

---

## 6. Onboarding & the Helm Handoff

The operator onboards the cluster to the PCE but **does not install the agents**. The handoff is **decoupled** because the stock Helm chart cannot consume a pre-created Secret (verified against chart v5.10.1: it templates `illumio-secret` and `illumio-ven-config` from Helm **values**; there is no `existingSecret` hook).

**Flow:**
1. Operator (via `ClusterProfile.spec.onboarding`) calls the PCE API to ensure a **Container Cluster** object exists, obtaining `cluster_id` + `cluster_token`, and ensures a pairing profile to obtain `cluster_code`.
2. Operator **writes these into a Kubernetes Secret** (`credentialsOutputSecret`) and mirrors non-sensitive identifiers into `ClusterProfile.status`. The token is one-time-visible from the PCE, so the operator captures it at creation.
3. **A separate mechanism consumes those credentials as Helm values** — the operator does not run Helm:
   - **GitOps (recommended):** Flux `HelmRelease.valuesFrom` referencing the Secret, or Argo CD with a comparable values source.
   - **Manual:** admin reads the Secret and passes values to `helm install`.

This keeps agent deployment fully owned by Helm while still automating the PCE-side onboarding.

> **Possible future enhancement (out of v1 scope):** maintain a fork/patch of the chart that adds an `existingSecret` value so the operator-written Secret is consumed directly. Not pursued in v1 to avoid owning a chart fork.

---

## 7. Internal Architecture — "Two Front-ends, One Brain"

```
SegmentationPolicy ─┐
                    ├─► Illumio IR ─► Reconciler / PCE client ─► draft writes ─► scoped provision
SegmentationIntent ─┘                      ▲
ClusterProfile ─► CWP reconciler ──────────┘  (shares PCE client + ownership tagging)
```

- **Front-end controllers** compile their CRD into a shared **Illumio Intermediate Representation (IR)** — desired PCE objects: labels, services, rulesets, rules, CWP settings. All CRD-specific knowledge lives here; the hard backend is written once.
- **Reconciler / PCE client** diffs desired IR vs actual PCE state over the REST API and applies create/update/delete to the **draft** policy version, then provisions (§8).
- **CWP reconciler** turns `ClusterProfile` + namespace state into desired CWP settings, sharing the same PCE client and ownership tagging.

Each unit is independently testable: IR compilation (pure functions), the PCE client (against a mock PCE), and the reconcile loop (envtest).

---

## 8. Provisioning, Ownership & Drift

Illumio writes land in **draft** and do nothing until **provisioned** (`provision_changes`), and provisioning is **org-global**. The operator must therefore never blindly provision; it must scope to its own changes.

**Ownership tagging.** Every PCE object the operator writes is stamped with:
- `external_data_set` = the operator identity (from `PCEConnection.spec.externalDataSet`)
- `external_data_reference` = the owning CR UID

This is the load-bearing safety mechanism. It lets the operator:
1. Only ever modify/delete objects **it authored**.
2. **Scope provisioning to its own `change_subset`** — never flushing a human's unrelated pending drafts.
3. Detect and revert **console drift** on its own objects (declarative reconciliation).

**Provisioning modes** (`provisioningMode`, default in `ClusterProfile`, overridable per-namespace via annotation):
- `auto` — write draft + provision the operator's own change_subset immediately. Full "no console".
- `manual` — write draft; provision only after an explicit approve annotation/field on the CR.
- `draft-only` — never provision; a human provisions in the PCE.

`status.workloadsAffected` (from the provision response) is surfaced **before** (dry-run/impact where available) and **after** provisioning.

**Deletion.** Finalizers remove the operator-owned PCE objects and re-provision the removal when a CR is deleted.

---

## 9. Secrets & Credential Management

**Requirement: all Illumio credentials are Kubernetes Secrets, managed — never inline in a CRD.**

- **Operator PCE API credentials.** `PCEConnection.spec.credentialsSecretRef` points to a Secret with `api_key` / `api_secret`. The operator authenticates via HTTP Basic (`api_key:api_secret`) against `/api/v2`, with `orgId` from the CR. Credentials are never stored in CR spec/status.
- **Onboarding output credentials.** `cluster_id` / `cluster_token` / `cluster_code` are written to the `credentialsOutputSecret` (§6); only non-sensitive identifiers appear in status.
- **RBAC.** Secret access is least-privilege; the operator's ServiceAccount can read its API-credential Secret and write the onboarding-output Secret in the configured namespace only.
- **Rotation.** The operator watches the credential Secret and re-reads on change (rotation without restart). A future enhancement may drive PCE-side API key rotation; v1 consumes externally-rotated keys.
- **Rate limits.** PCE caps at 500 req/min per key (429 on exceed) → exponential backoff + requeue; collection GETs cap at 500 objects → use async collections beyond that.

---

## 10. Error Handling & Status

- Every CR exposes `status.conditions` (e.g. `Ready`, `Rejected`, `Provisioned`, `Degraded`) with machine-readable reasons.
- **Validation failures** (unknown label, unsupported NetworkPolicy construct, `selective` requested) → `Rejected` with a human-readable reason; no PCE writes.
- **PCE unreachable** → `Degraded`, requeue with backoff, **no partial provisions**.
- **Conflict resolution** (enforcement) is reported transparently (`effectiveEnforcement` / `enforcementSetBy`).
- All reconciles are idempotent; ownership tagging makes repeated runs safe.

---

## 11. Implementation & Testing

- **Language/stack:** Go + Operator SDK / kubebuilder (matches the ecosystem and `illumio/cloud-operator`).
- **PCE API client package:** a thin, well-typed wrapper over the CWP, label, service, ruleset/rule, and provisioning endpoints, plus auth and ownership tagging. The single home of Illumio-specific knowledge; independently unit-tested against a mock PCE.
- **Tests:**
  - Unit: IR compilation for both front-ends; guardrail rejection cases; enforcement resolution.
  - Controller: envtest for each controller.
  - Integration: suite against a mock PCE (and optionally a real/test PCE) covering onboarding, CWP reconcile, policy provision, drift revert, deletion.
- **Targets:** Kubernetes and OpenShift; declare CLAS vs legacy support explicitly per release.

---

## 12. Open Questions / Version-Dependence to Pin Before Coding

1. **Minimum versions.** `workloadLabelMap` requires **CLAS + PCE 24.5+**; confirm the target PCE/Core-for-K8s versions and set them as hard prerequisites.
2. **`enforcement_mode` API type** in bulk vs single update (string enum vs integer seen in docs) — verify against the target PCE version.
3. **`labels` vs deprecated `assign_labels`** on the CWP API — prefer `labels`; confirm minimum PCE version.
4. **Impact/dry-run endpoint** availability (experimental) for surfacing `workloadsAffected` before provisioning.
5. **Label `href` resolution.** How the operator resolves a label to its PCE `href` (GET `/labels` + cache/watch), and behavior when a policy-referenced label doesn't exist yet — requeue vs reject (§4.3, §5).
6. **CWP label creation policy.** Confirm the operator may create-if-missing for admin-defined CWP labels (bounded) vs. requiring them to pre-exist. Default in this spec: create-if-missing for CWP labels only; never for policy references (§4.3).

---

## 13. Sequencing (high level)

1. PCE API client package + `PCEConnection` (auth, ownership tagging, rate-limit handling).
2. PCE label resolver (GET `/labels` lookup + cache; create-if-missing for CWP labels only).
3. `ClusterProfile` CWP reconciler (namespace rules, `managed` flag, CWP/namespace labels incl. `fromNamespaceLabel`, enforcement, OpenShift defaults).
4. Onboarding + credential-output Secret.
5. Illumio IR + Reconciler + provisioning (auto/manual/draft-only, scoped change_subset).
6. `SegmentationIntent` front-end + guardrails.
7. `SegmentationPolicy` front-end (supported subset) over the same backend.
8. Drift detection/revert, finalizers, status polish.
