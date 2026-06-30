# Policy: concepts and how-tos

This page is the starting point for anyone who knows Kubernetes but is new to Illumio segmentation.
It explains the mental model, defines the two policy front-ends, and walks through the most common
tasks. Read it top-to-bottom once; then use the cross-links for details.

---

## 1. Mental model: a ruleset is a scope + rules

An Illumio **ruleset** has two parts:

- **Scope** — the application the ruleset protects. Defined by the namespace's Illumio labels:
  typically `app` + `env` + `loc`. The operator derives these labels from the namespace's
  Container Workload Profile (CWP), which is configured by `ClusterProfile`.
- **Rules** — each rule says: _these **consumers** (sources) may reach these **providers**
  (destinations) on these **services** (ports)._

```
  Ruleset: scope = app=payments, env=prod
  ┌─────────────────────────────────────────────────────────────────┐
  │  Rule 1: checkout/prod  ──8443/TCP──▶  payments/prod (backend)  │
  │  Rule 2: monitoring/prod ──all ports──▶  payments/prod          │
  └─────────────────────────────────────────────────────────────────┘
```

Two things to fix in your mental model before writing any policy:

**One CR = one ruleset = one protected namespace.** A policy CR lives in a namespace and protects
that namespace's app. You cannot write rules that protect another team's namespace.

**Illumio rulesets are ingress-centric.** The provider is always inside your scope (your
namespace). The consumer can be anywhere — inside the same namespace (intra-scope) or in another
app (extra-scope). You only write rules about _who may come in_, not about where your app is
allowed to reach out.

---

## 2. Two front-ends, one backend

There are two CRDs you can use to write policy. Both compile to the same Illumio ruleset backend
with identical capabilities.

| | `SegmentationIntent` | `SegmentationPolicy` |
|---|---|---|
| Short name | `segintent` | `segpol` |
| Style | Illumio-native allow-list (`allow[]`) | NetworkPolicy-shaped (`ingress`/`from`/`ports`) |
| Consumer field | `from` (extra-scope) / `fromIntraNamespace` (intra-scope) | `namespaceSelector` (extra-scope) / `podSelector` (intra-scope) |
| Provider narrowing | `spec.provider: {role: backend}` | top-level `spec.podSelector: {matchLabels: {role: backend}}` |
| Best for | Teams new to Illumio, or who prefer an explicit allow-list | Teams already familiar with Kubernetes `NetworkPolicy` |

Pick one based on taste. The rest of this page shows both where the syntax differs.

See the [Segmentation policy guide](segmentation-policy.md) for `SegmentationIntent` in depth, and
the [NetworkPolicy-style guide](networkpolicy-style.md) for `SegmentationPolicy` in depth.

---

## 3. Intra-scope vs extra-scope consumers

This is the concept that trips people up most often.

**Intra-scope** means the consumer is a workload _inside the same namespace_ — it belongs to the
same Illumio application scope. No extra identity is needed; the scope is already established.

**Extra-scope** means the consumer is a workload in _a different application_ — it lives outside
your namespace's scope. You must identify it by its Illumio labels (e.g. `app: checkout, env: prod`).

```
  Cluster
  ┌─────────────────────────────────────────────────────────────────┐
  │  namespace: payments  (scope: app=payments, env=prod)           │
  │  ┌────────────────────────────────────────────────────────┐     │
  │  │  role=frontend ──(intra-scope)──▶  role=backend        │     │
  │  └────────────────────────────────────────────────────────┘     │
  │            ▲                                                     │
  │   (extra-scope)                                                  │
  │            │                                                     │
  │  namespace: checkout  (scope: app=checkout, env=prod)           │
  │  ┌──────────────────────────────────────────────────────────┐   │
  │  │  checkout pods                                           │   │
  │  └──────────────────────────────────────────────────────────┘   │
  └─────────────────────────────────────────────────────────────────┘
```

### Field mapping

| Consumer is... | SegmentationIntent | SegmentationPolicy |
|---|---|---|
| Intra-scope (same namespace), specific role | `allow[].fromIntraNamespace: {role: frontend}` | `ingress[].from[].podSelector: {matchLabels: {role: frontend}}` |
| Intra-scope (same namespace), any workload | `allowIntraNamespace: true` (top-level shortcut) | `ingress[].from[].podSelector: {}` (empty = all in namespace) |
| Extra-scope (another app) | `allow[].from: {app: checkout, env: prod}` | `ingress[].from[].namespaceSelector: {matchLabels: {app: checkout, env: prod}}` |

### The "All Workloads" shortcut

`allowIntraNamespace: true` (Intent) or `podSelector: {}` (Policy) compiles to an Illumio
**All Workloads** actor scoped to the namespace. It covers every current and future workload in
the namespace. Use it for the "any-any inside this namespace" pattern.

!!! warning "Use empty podSelector carefully in extra-scope"
    In the context of an `allow[].from` field (Intent) or a `namespaceSelector` peer (Policy),
    an empty or broad label set selects _all workloads organization-wide_. Be explicit: add at
    least `app` + `env` labels to any extra-scope consumer.

---

## 4. Who is protected: the provider

By default, the provider in a ruleset is the **entire namespace app** — every workload in the
namespace. That is the right default for most rules ("allow checkout to reach anything in
payments").

To protect only a _specific service_ within the namespace, narrow the provider:

**SegmentationIntent:**
```yaml
spec:
  provider: { role: backend }   # protects only role=backend pods in this namespace
  allow:
    - fromIntraNamespace: { role: frontend }
      ports:
        - { port: 8443, protocol: TCP }
```

**SegmentationPolicy:**
```yaml
spec:
  podSelector:
    matchLabels:
      role: backend             # protects only role=backend pods in this namespace
  ingress:
    - from:
        - podSelector:
            matchLabels:
              role: frontend
      ports:
        - port: 8443
          protocol: TCP
```

Provider labels must already exist in the PCE (they are always resolved strictly, regardless of
`unknownLabelMode`). The `role` label is typically supplied by the [Illumio LabelMap](labelmap-and-the-operator.md).

---

## 5. Rules vs enforcement — the most important distinction

!!! warning "Writing a rule does NOT block any traffic"
    Rules declare _what is allowed_. Traffic that is not allowed is only blocked when the
    namespace's **effective enforcement mode** is `full`. Without `full` enforcement, provisioned
    rules have zero effect on traffic — nothing is blocked.

### Enforcement modes

| Mode | Effect |
|---|---|
| `idle` | No enforcement; all traffic flows; no flow visibility. |
| `visibility_only` | All traffic flows; Illumio records flow data. Rules compiled but not enforced. |
| `full` | Default-deny: only explicitly allowed traffic flows. This is microsegmentation. |

### Effective enforcement: strictest-wins

The namespace's **effective enforcement** is the strictest value across:

1. The **ClusterProfile admin baseline** — `enforcementMode` from the matching namespace rule
   (`namespaceRules[].enforcementMode`) or `systemNamespaces.enforcementMode`.
2. The `spec.enforcement` field on **every** `SegmentationIntent` and `SegmentationPolicy` in the
   namespace.

Strictness order: `idle` < `visibility_only` < `full`.

The winning value is applied to the namespace's Container Workload Profile (CWP). Every policy CR
reports it:

- `status.effectiveEnforcement` — the mode currently applied to the namespace's CWP.
- `status.enforcementSetBy` — names the CR (or `admin`) that provided the winning value.

### How to switch a namespace to full enforcement

There are three levers. Use any one; the strictest-wins rule means any `full` source wins:

**Lever (a) — namespace annotation (instant, no CR needed):**
```bash
kubectl annotate ns <namespace> microsegment.io/enforcement=full --overwrite
```
This writes directly to the CWP. It is the fastest way to switch a single namespace.

**Lever (b) — policy CR `spec.enforcement`:**
```yaml
# Add or update any SegmentationIntent or SegmentationPolicy in the namespace:
spec:
  enforcement: full
  allow:
    - from: { app: checkout, env: prod }
      ports:
        - { port: 8443, protocol: TCP }
```
```bash
kubectl apply -f payments-ingress.yaml
kubectl get segintent -n payments
# ENFORCEMENT column (or describe) shows: effectiveEnforcement: full
```

**Lever (c) — ClusterProfile admin baseline (affects all matching namespaces):**
```yaml
# In ClusterProfile.spec.namespaceRules:
- match: { namePattern: "payments" }
  managed: true
  assignLabels:
    app: { value: payments }
    env: { value: prod }
  enforcementMode: full       # ← sets the baseline for this namespace
```

!!! danger "full enforcement = default-deny"
    Switch to `full` only after you have confirmed your allow-rules are complete and provisioned.
    Test on an application namespace first — never on control-plane namespaces like `kube-system`
    or `openshift-*`. A missed rule under `full` enforcement will silently drop traffic.

---

## 6. Provisioning: draft vs active

The operator writes rules to the Illumio **draft** policy store. Draft rules are computed and
visible in the PCE but not yet enforced. Enforcement uses the **active** policy store.

To activate draft rules they must be **provisioned** (promoted from draft to active), creating a
new policy version. The provisioning response returns the new version and how many VENs
(`workloadsAffected`) were recomputed — visible in `status.workloadsAffected`, the `AFFECTED`
print column, the `Provisioned` condition message, and the operator logs.

Provisioning is controlled by `ClusterProfile.spec.provisioningMode`:

| Mode | Behavior |
|---|---|
| `auto` | Operator provisions immediately after compilation. `Provisioned` → `True` once the PCE accepts. |
| `manual` | Operator writes the draft and waits. You approve with `kubectl annotate <cr> microsegment.io/provision=approved`. While the annotation is present the operator keeps the policy provisioned and re-provisions on every spec change. |
| `draft-only` | Operator writes the draft and never provisions. `Provisioned` stays `False` with reason `ProvisionPending` permanently. You provision directly in the PCE UI or API. |

`ClusterProfile.spec.provisioningMode` defaults to `manual`.

---

## 7. Unknown labels

Consumer labels must exist in the PCE before you can reference them. The behavior when a label is
missing is controlled by `unknownLabelMode`:

| Mode | Behavior |
|---|---|
| `strict` (default) | Reject the entire CR (`Ready=False`, reason `Rejected`). Not retried until the spec changes. |
| `skip` | Compile the rules whose labels resolve; omit the unknown consumer; keep the CR `Ready=True`. The omitted label pair is listed in `status.deferredLabels` and retried every reconcile — the rule appears automatically once the label exists. |
| `create` | Mint the missing label in the PCE (for standard keys `role`, `app`, `env`, `loc` only — a non-standard key is still rejected). Created pairs are listed in `status.createdLabels`. |

Provider labels (`spec.provider` on Intent, top-level `spec.podSelector` on Policy) are **always**
resolved strictly — `unknownLabelMode` does not apply to them.

### Setting unknownLabelMode (most-specific wins)

1. `ClusterProfile.spec.unknownLabelMode` — fleet default (defaults to `strict`).
2. Namespace annotation `microsegment.io/unknown-label-mode: <mode>` — overrides the fleet default for a namespace.
3. CR annotation `microsegment.io/unknown-label-mode: <mode>` — overrides for that specific CR.

The resolved value and its source are reported in `status.unknownLabelMode` and
`status.unknownLabelModeSetBy` (`cr` | `namespace` | `clusterprofile` | `default`).

---

## 8. How-tos

All examples work with `kubectl apply -f <file>.yaml`. Verify steps are shown after each.

### (a) Allow any-any within a namespace

```yaml
# segintent-intra.yaml
apiVersion: microsegment.io/v1alpha1
kind: SegmentationIntent
metadata:
  name: myapp-internal
  namespace: myapp
spec:
  allowIntraNamespace: true
```

```yaml
# segpol-intra.yaml  (SegmentationPolicy equivalent)
apiVersion: microsegment.io/v1alpha1
kind: SegmentationPolicy
metadata:
  name: myapp-internal
  namespace: myapp
spec:
  ingress:
    - from:
        - podSelector: {}   # all pods in this namespace
```

Verify:
```bash
kubectl get segintent -n myapp
# NAME             READY   PROVISIONED   AFFECTED
# myapp-internal   True    True          8
```

### (b) Cross-app ingress (extra-scope)

```yaml
# segintent-extra.yaml
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
    - from: { app: monitoring, env: prod }
      # no ports — allows all ports from monitoring (All Services)
```

```yaml
# segpol-extra.yaml  (SegmentationPolicy equivalent)
apiVersion: microsegment.io/v1alpha1
kind: SegmentationPolicy
metadata:
  name: payments-ingress
  namespace: payments
spec:
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              app: checkout
              env: prod
      ports:
        - port: 8443
          protocol: TCP
    - from:
        - namespaceSelector:
            matchLabels:
              app: monitoring
              env: prod
```

Verify:
```bash
kubectl get segintent -n payments
# NAME               READY   PROVISIONED   AFFECTED
# payments-ingress   True    True          12
```

### (c) Service-to-service with provider narrowing (intra-scope)

Protect only `role=backend` and allow only `role=frontend` to reach it.

```yaml
# segintent-svc.yaml
apiVersion: microsegment.io/v1alpha1
kind: SegmentationIntent
metadata:
  name: backend-access
  namespace: payments
spec:
  provider: { role: backend }
  allow:
    - fromIntraNamespace: { role: frontend }
      ports:
        - { port: 8443, protocol: TCP }
```

```yaml
# segpol-svc.yaml  (SegmentationPolicy equivalent)
apiVersion: microsegment.io/v1alpha1
kind: SegmentationPolicy
metadata:
  name: backend-access
  namespace: payments
spec:
  podSelector:
    matchLabels:
      role: backend
  ingress:
    - from:
        - podSelector:
            matchLabels:
              role: frontend
      ports:
        - port: 8443
          protocol: TCP
```

Verify (the `role` label must exist in the PCE — see [LabelMap guide](labelmap-and-the-operator.md)):
```bash
kubectl get segintent backend-access -n payments
# NAME             READY   PROVISIONED   AFFECTED
# backend-access   True    True          4
```

### (d) Tolerate not-yet-existing consumer labels (skip mode)

```yaml
# segintent-skip.yaml
apiVersion: microsegment.io/v1alpha1
kind: SegmentationIntent
metadata:
  name: payments-ingress
  namespace: payments
  annotations:
    microsegment.io/unknown-label-mode: skip   # omit unknown consumers; retry each reconcile
spec:
  allow:
    - from: { app: future-service, env: prod }
      ports:
        - { port: 8443, protocol: TCP }
```

Verify which labels are deferred:
```bash
kubectl get segintent payments-ingress -n payments -o jsonpath='{.status.deferredLabels}'
# ["app=future-service"]
```
Once the label exists in the PCE the next reconcile will include the rule automatically.

### (e) Switch a namespace to full enforcement

The fastest single-namespace lever is the namespace annotation:
```bash
kubectl annotate ns payments microsegment.io/enforcement=full --overwrite
```

Or via a policy CR:
```yaml
apiVersion: microsegment.io/v1alpha1
kind: SegmentationIntent
metadata:
  name: payments-ingress
  namespace: payments
spec:
  enforcement: full
  allow:
    - from: { app: checkout, env: prod }
      ports:
        - { port: 8443, protocol: TCP }
```

Verify effective enforcement:
```bash
kubectl describe segintent payments-ingress -n payments | grep -A2 "Effective Enforcement"
# Effective Enforcement:  full
# Enforcement Set By:     payments-ingress
```

### (f) Provision a draft (manual provisioning mode)

When `ClusterProfile.spec.provisioningMode: manual`, apply the CR (compiles to draft) then approve:

```bash
# 1. Apply the intent (compiles to draft, Provisioned=False)
kubectl apply -f payments-ingress.yaml

# 2. Check the draft is ready
kubectl get segintent -n payments
# NAME               READY   PROVISIONED   AFFECTED
# payments-ingress   True    False         0

# 3. Approve provisioning
kubectl annotate segintent payments-ingress microsegment.io/provision=approved -n payments

# 4. Verify provisioned
kubectl get segintent -n payments
# NAME               READY   PROVISIONED   AFFECTED
# payments-ingress   True    True          12
```

For `SegmentationPolicy`:
```bash
kubectl annotate segpol payments-ingress microsegment.io/provision=approved -n payments
```

For more detail on all provisioning modes, see the [Segmentation policy guide](segmentation-policy.md#provisioning-modes).

---

## 9. Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `Ready=False`, reason `Rejected`, message `label not found in PCE: key=value` | A consumer label does not exist in the PCE (strict mode). | Check spelling. Use `skip` or `create` mode if the label does not exist yet. Provider labels are always strict. |
| `Ready=False`, reason `Rejected`, message `allow[N]: set exactly one of from or fromIntraNamespace` | An `allow` entry has both fields set, or neither. | Set exactly one of `from` (extra-scope) or `fromIntraNamespace` (intra-scope) per entry. |
| `Ready=False`, reason `Rejected`, message `from[N]: podSelector and namespaceSelector cannot both be set` | A `SegmentationPolicy` peer has both selectors. | Use exactly one per peer: `podSelector` (intra) or `namespaceSelector` (extra). |
| `Ready=False`, reason `ClusterProfileNotReady` | No `ClusterProfile` namespace rule covers this namespace. | Add a matching `namespaceRules` entry (or annotation) in the `ClusterProfile`. The controller retries automatically once it exists. |
| `Provisioned=False`, reason `ProvisionPending` | `provisioningMode: manual` and the `microsegment.io/provision=approved` annotation is absent — OR `provisioningMode: draft-only`. | For `manual`: add the annotation. For `draft-only`: provision directly in the PCE. |
| Rules provisioned but traffic is NOT blocked | Effective enforcement is not `full`. Rules exist but are not enforced. | Check `status.effectiveEnforcement`. Set enforcement to `full` via namespace annotation, policy CR, or `ClusterProfile` (see [section 5](#5-rules-vs-enforcement-the-most-important-distinction)). |
| Rules provisioned, enforcement `full`, but traffic IS blocked (unintentional) | A missing allow-rule for legitimate traffic. | Add an allow-rule for the affected flow and re-provision. Check flow visibility in the PCE to identify what is being dropped. |

---

## Further reading

- [Segmentation policy guide](segmentation-policy.md) — `SegmentationIntent` in depth: compilation steps, provisioning walkthroughs, enforcement details, deleting intents.
- [NetworkPolicy-style guide](networkpolicy-style.md) — `SegmentationPolicy` in depth: selector mapping, rejection rules, comparison table.
- [Labeling and the LabelMap](labelmap-and-the-operator.md) — how `app`/`env` labels (operator) and `role` labels (LabelMap) combine to make intra-namespace rules possible.
- [Namespace management](namespace-management.md) — setting the admin enforcement baseline and per-namespace annotation overrides.
- [SegmentationIntent reference](../reference/segmentationintent.md) — complete field and status documentation.
- [SegmentationPolicy reference](../reference/segmentationpolicy.md) — complete field and status documentation.
- [ClusterProfile reference](../reference/clusterprofile.md) — `provisioningMode`, `unknownLabelMode`, and namespace rule configuration.
