# Labeling: this operator and the Illumio LabelMap

Illumio workloads (your pods) carry **Illumio labels** along the standard dimensions —
**Role, App, Environment, Location** (RAEL) — plus optional custom keys. Those labels are what
segmentation policy is written against. In a Kubernetes cluster there are **two** systems that can
assign them, and they are designed to cover **different** parts of the label set.

## Two labeling systems, two lanes

| | This operator (CWP labels) | Illumio `LabelMap` (`workloadLabelMap`) |
|---|---|---|
| **Granularity** | **Namespace-level** — every workload in a namespace gets the same set | **Per-workload** — each pod/deployment can get different labels |
| **Source** | `ClusterProfile` `namespaceRules.assignLabels` / `systemNamespaces.labels` | A `kind: LabelMap` CR that maps Kubernetes workload labels → Illumio labels |
| **Typical keys** | **`app`**, **`env`** (the namespace's identity) | **`role`** (the per-service tier) and other custom keys |
| **Owned by** | This operator (writes the Container Workload Profile) | The Illumio C-VEN / Kubelink stack (you configure the `LabelMap`) |

The operator deliberately **stays namespace-level**. It does not — and will not — assign
per-deployment labels. Distinguishing services *within* one namespace (e.g. `frontend` vs
`backend`) is the `LabelMap`'s job (or [Illumio pod annotations](#illumio-pod-annotations)).

## How they combine

Together they give every workload a complete identity:

```
app=payments, env=prod      ← from this operator's ClusterProfile (namespace-level CWP)
role=frontend | role=backend ← from the Illumio LabelMap (per-workload, mapped from a k8s label)
```

That combined identity is what makes **intra-namespace, service-to-service** policy possible —
"allow `role=frontend` to reach `role=backend` within `app=payments`." The operator supplies the
scope (`app`/`env`); the `LabelMap` supplies the sub-namespace distinction (`role`).

## The golden rule: don't let both write the same key

The one thing that must not happen is **both systems writing the same label dimension** — that is
two controllers fighting over one key. The rule is simple:

> **The `LabelMap` should only populate keys the operator does *not* assign at the namespace level.**
> The operator owns whatever your `ClusterProfile` assigns (typically `app` and `env`); the
> `LabelMap` fills the complement (typically `role` and custom keys).

So when you author a `LabelMap`, **map your Kubernetes labels to `role`/custom keys, and leave
`app`/`env` to the operator.** If a `LabelMap` also tried to set `app` or `env`, the namespace-level
CWP assignment and the per-workload map would disagree.

!!! note "Operator-side overlap warning is on the roadmap"
    A future release will have the operator **detect** a `LabelMap` that targets a key it already
    assigns and **emit a warning** (it will never strip or override — warn-only; the operator only
    controls its own CWP keys). Until then, follow the golden rule by configuration. See the
    [policy-model roadmap](https://github.com/alexgoller/illumio-k8s-utility-operator/blob/main/docs/superpowers/specs/2026-06-30-policy-model-roadmap-design.md) (Track 4).

## Example: dividing the label set

`ClusterProfile` (this operator) — namespace identity:

```yaml
spec:
  namespaceRules:
    - match: { namePattern: "payments" }
      managed: true
      assignLabels:
        app: { value: payments }
        env: { value: prod }
      enforcementMode: visibility_only
```

Illumio `LabelMap` (configured with the C-VEN/Kubelink stack) — per-service `role`, **not**
`app`/`env`:

```yaml
apiVersion: ...               # Illumio Core for Kubernetes
kind: LabelMap
spec:
  workloadLabelMap:
    role: app.kubernetes.io/component   # k8s label -> Illumio role; leave app/env to the operator
```

Result: a `payments` pod with `app.kubernetes.io/component=backend` ends up labeled
`app=payments, env=prod, role=backend` — `app`/`env` from the operator, `role` from the `LabelMap`.

## Requirements for the `LabelMap`

Workload label mapping is an Illumio feature, independent of this operator:

- **Illumio Core for Kubernetes 5.3.0+**, **CLAS-enabled** deployment, **PCE 24.5.0+**.

If your stack predates this, per-workload labels aren't available and intra-namespace rules can't be
expressed; the operator's namespace-level labeling still works.

## Illumio pod annotations

Per-workload labels can also come from **Illumio pod annotations** that the C-VEN/Kubelink reads,
as an alternative to a `LabelMap`. The same golden rule applies: annotate the dimensions the
operator does *not* own (e.g. `role`), not `app`/`env`.

## See also

- [Namespace management](namespace-management.md) — how the operator assigns the namespace-level `app`/`env` labels.
- [Segmentation policy](segmentation-policy.md) — writing rules against these labels.
