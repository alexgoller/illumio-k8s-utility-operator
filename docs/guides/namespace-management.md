# Namespace (CWP) Management

The operator configures the **Container Workload Profile (CWP)** for every namespace in the cluster. If you have ever managed Illumio for Kubernetes by hand, the CWP is where most of the day-to-day pain lives — this guide explains what a CWP is, why managing them manually does not scale, and how the operator turns that toil into a few declarative rules in Git.

## What a Container Workload Profile actually is

When you pair a Kubernetes or OpenShift cluster to the PCE, Illumio represents it as a **Container Cluster**. Inside that Container Cluster, every Kubernetes **namespace** maps to one **Container Workload Profile**. The CWP is the single control point that decides, *for all container workloads in that namespace*:

| The CWP controls | Meaning |
|---|---|
| **Managed vs unmanaged** | Whether the PCE tracks and can enforce the workloads in the namespace at all. Unmanaged = invisible to policy. |
| **Illumio labels** | The Role/App/Environment/Location (RAEL) labels stamped on **every** workload in the namespace. This is the workload's identity, and what policy scope is written against. |
| **Enforcement / visibility** | `idle`, `visibility_only`, or `full` for the namespace's workloads. |

Crucially, a CWP assigns labels **uniformly across the whole namespace** — every pod in `payments` gets the same CWP labels. (Per-workload differentiation is a separate mechanism; see [Scope vs role](#scope-vs-role-what-the-cwp-should-and-shouldnt-label) below.)

## Why CWPs are a pain by hand

The CWP model is sound, but operating it manually in the PCE console is where teams lose time:

- **It is click-ops, per namespace.** Out of the box a new namespace's CWP carries no app/env identity. Someone has to open the PCE, find the profile, and assign labels — for *every* namespace.
- **It does not scale.** A real cluster has dozens to hundreds of namespaces. Multiply that across clusters and environments and the labeling backlog never reaches zero.
- **It drifts constantly.** Namespaces are created and deleted by CI/CD all day. Every new namespace is unlabeled until a human notices. The PCE has no idea what your Kubernetes namespace labels say.
- **A single default is too blunt.** You can set one default label assignment for the whole cluster, but then every namespace gets the *same* `app`/`env` — useless for differentiating apps and environments.
- **Getting `managed` wrong is risky in both directions.** Mark too much managed under `full` and you can block real traffic; leave things unmanaged and they are invisible to policy and flow maps.
- **Enforcement transitions are finicky.** A *managed* CWP can never be `idle` — the PCE rejects that combination — so naive automation trips over it.
- **There is no native GitOps.** The desired state lives in someone's head or a runbook, not in source control.

### What the operator does instead

You describe the desired CWP state once, declaratively, in a `ClusterProfile`, and the operator reconciles **every** namespace's CWP continuously:

1. Lists all namespaces in the cluster.
2. Resolves the desired CWP configuration for each using the **precedence model** below — including **deriving Illumio labels from the namespace's own Kubernetes labels** (`fromNamespaceLabel`), which the PCE UI cannot do.
3. Updates each CWP via the PCE API (managed flag, labels, enforcement).
4. Reconciles again whenever namespaces or the `ClusterProfile` change, so new namespaces are labeled automatically and drift self-heals.
5. Records `status.managedNamespaces` and emits a `CWPConfigured` event per namespace.

> **Timing note:** Illumio Kubelink creates the CWP for a namespace when the C-VEN stack first discovers it. The operator does **not** create CWPs — it only updates ones Kubelink has already created. A namespace whose CWP does not exist yet is skipped and picked up automatically on a later reconcile. If a namespace never gets labeled, check that the C-VEN agent and Kubelink are running.

## Scope vs role: what the CWP should and shouldn't label

This is the single most important design decision when adopting the operator, and it follows directly from the CWP being **namespace-uniform**.

An Illumio ruleset **scope** can be any number of labels. For Kubernetes namespaces the right scope is **`app` + `env`** almost every time (that is the operator's default) — it identifies the namespace's application and environment. `loc` is **not** a good scope label, and `role` is never scope (it distinguishes services *within* a namespace, inside a rule). The scope-label set is configurable — see [`policyScopeLabels`](#limiting-the-scope-labels) below.

Because a CWP labels every workload in the namespace identically, it can only own **namespace-uniform** dimensions:

| Dimension | Who should assign it | Why |
|---|---|---|
| **App, Env** (the default scope) | **This operator** — the CWP | Namespace-wide identity. Every workload in `payments`/`prod` shares them. The CWP is exactly the right granularity, and these become the ruleset scope. |
| **Loc, other custom keys** | The operator (CWP) *if you want them on workloads* | Fine to assign for visibility, but keep them **out of the ruleset scope** (they are not in `policyScopeLabels` by default). |
| **Role** (per-service tier) | **The Illumio C-VEN `LabelMap`** (per-workload) | `frontend` vs `backend` differ *within* one namespace. A CWP cannot express that — it is uniform. The C-VEN's `LabelMap` maps a per-pod Kubernetes label to the Illumio `role`. |

> **Best practice — divide the label set, never overlap it.**
> The operator owns the **scope** labels (`app`+`env` by default) via the CWP. You **rely on the C-VEN `LabelMap` for `role`** (and any other per-workload key). The two systems must never write the *same* Illumio key — that is two controllers fighting over one dimension.

### Limiting the scope labels

By default a namespace's ruleset is scoped to its `app` and `env` labels. If a namespace CWP also carries other labels (e.g. `loc`), they stay on the workloads for visibility but are **not** part of the ruleset scope. To change which assigned labels form the scope, set `policyScopeLabels` on the `ClusterProfile`:

```yaml
spec:
  policyScopeLabels: [app, env]   # the default; loc is intentionally excluded
```

Set it explicitly (e.g. `[app, env, tier]`) to scope on a different set. Leaving it empty keeps the `app`+`env` default. A namespace that carries none of the scope labels is rejected for policy — it has nothing to scope its ruleset by.

This division is also *what makes intra-namespace, service-to-service policy possible*: the operator supplies the scope (`app=payments, env=prod`) and the `LabelMap` supplies the in-namespace distinction (`role=frontend` / `role=backend`), so you can write "allow `role=frontend` → `role=backend` within `app=payments, env=prod`."

The operator actively guards this boundary: if a `LabelMap` is detected writing a key the `ClusterProfile` also assigns, it raises a `LabelMapOverlap` warning (it never overrides — warn-only). See the [LabelMap coexistence guide](labelmap-and-the-operator.md) for the full division of labor and how to read the warning.

## Precedence model

The desired configuration for a namespace is resolved in three layers, applied in order from highest to lowest priority:

1. **Per-namespace annotation overrides** — annotations on the namespace object itself always take final precedence (see [Namespace annotations](#namespace-annotations)).
2. **`systemNamespaces` config** — when `systemNamespaces.manage` is `true`, namespaces whose name matches any of the system patterns receive the `systemNamespaces` configuration. This **overrides any matching `namespaceRule`** for those namespaces.
3. **`namespaceRules` (first match)** — for all other namespaces, rules are evaluated in order; the first rule whose `match` criteria are satisfied governs the namespace. A namespace that matches no rule receives no CWP update.

A managed namespace with no `enforcementMode` set (neither from a rule nor from an annotation) defaults to `visibility_only`. A managed Container Workload Profile can never be `idle` — the PCE rejects that combination — so an `idle` mode on a managed namespace is also raised to `visibility_only`.

## System namespaces

The `systemNamespaces` stanza is a convenience for managing Kubernetes and OpenShift infrastructure namespaces without having to enumerate them in `namespaceRules`.

When `systemNamespaces.patterns` is empty, the following default patterns apply:

- `openshift-*`
- `kube-*`
- `default`
- `kube-system`
- `kube-public`
- `kube-node-lease`

You can supply a custom `patterns` list to override these defaults.

### Configuring via Helm

When the chart renders the `ClusterProfile` (`onboarding.enabled: true`), the system-namespace set is tunable from `values.yaml` — no rebuild required. The shipped defaults cover OpenShift and Kubernetes infrastructure (the bare `openshift` project is listed explicitly because it is not matched by `openshift-*`):

```yaml
systemNamespaces:
  manage: true                       # off by default; turn on to manage them
  enforcementMode: visibility_only
  labels: { app: openshift, role: infra }
  patterns:
    - openshift
    - "openshift-*"
    - "kube-*"                       # kube-system, kube-public, kube-node-lease
    - default
namespaceRules: []                   # optional, written verbatim into the spec
```

These values are written into the chart-managed `ClusterProfile` spec. If you manage your own `ClusterProfile` directly (not via the chart), set `systemNamespaces` in that resource instead.

> System namespaces are the one place the operator legitimately assigns `role` (e.g. `role: infra`) at the namespace level, because infrastructure pods do not need per-service `role` differentiation. For **application** namespaces, leave `role` to the `LabelMap`.

## Namespace rules

`namespaceRules` is an ordered list of rules. Each rule has:

- `match` — criteria to select namespaces:
  - `namePattern` — a glob (Go `path.Match` syntax, e.g. `team-*`). Empty matches any name.
  - `labels` — a map of k8s label key/value pairs that must all be present on the namespace.
- `managed` — whether to mark the CWP as PCE-managed.
- `assignLabels` — Illumio label key → assignment. Two assignment forms:
  - `value: "prod"` — assign a fixed value.
  - `fromNamespaceLabel: "app.kubernetes.io/part-of"` — read the value from the namespace's own k8s label. If the label is absent on the namespace, this assignment is skipped (the Illumio label is left unchanged).
- `enforcementMode` — one of `idle`, `visibility_only`, `full`.

!!! note "Only `idle`, `visibility_only`, and `full` are valid for container workload profiles."
    The `selective` mode is not supported for CWPs.

> **Keep `assignLabels` to scope keys.** For application namespaces, assign the scope labels (`app`/`env`) here and do **not** assign `role` — that belongs to the C-VEN `LabelMap` so it can vary per workload. Other keys like `loc` may be assigned for visibility but stay out of the ruleset scope. See [Scope vs role](#scope-vs-role-what-the-cwp-should-and-shouldnt-label).

## Namespace annotations

Individual namespaces can override the rule-derived configuration by adding annotations:

| Annotation | Values | Effect |
|------------|--------|--------|
| `microsegment.io/managed` | `"true"` or `"false"` | Override the managed flag. |
| `microsegment.io/enforcement` | `idle`, `visibility_only`, `full` | Override the enforcement mode. |
| `microsegment.io/label.<key>` | any string | Override the value of Illumio label `<key>`. E.g. `microsegment.io/label.env=staging`. |

Annotations are evaluated after all rule resolution, so they always win.

## Complete example

The following `ClusterProfile` is a recommended starting point for OpenShift clusters. It:

- Marks all system namespaces (`openshift-*`, `kube-*`, etc.) as managed with `visibility_only` enforcement and fixed Illumio labels.
- Marks namespaces that carry the `microsegment.io/managed: "true"` label as managed, deriving Illumio **scope** labels (`app`/`env`) from standard k8s labels — and leaving `role` to the `LabelMap`.
- Marks everything else as unmanaged (no CWP update for unlabelled namespaces).

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
    labels: { app: openshift, role: infra }   # infra may carry role at ns level
    enforcementMode: visibility_only
    # patterns default to openshift-*, kube-*, default, kube-system, kube-public, kube-node-lease
  namespaceRules:
    - match: { labels: { "microsegment.io/managed": "true" } }
      managed: true
      assignLabels:                            # scope only — no role here
        app: { fromNamespaceLabel: app.kubernetes.io/part-of }
        env: { fromNamespaceLabel: app.kubernetes.io/environment }
      enforcementMode: visibility_only
    - match: { namePattern: "*" }
      managed: false
```

The catch-all `namePattern: "*"` rule at the end explicitly marks every remaining namespace as unmanaged, so unlabelled application namespaces are never enrolled by accident.

## Observing results

### Managed namespace count

```bash
kubectl get clusterprofiles
```

The `MANAGED-NS` column shows how many namespaces currently have a managed CWP:

```
NAME           CLUSTER        CLUSTERID   ONBOARDED   MANAGED-NS
this-cluster   ocp-prod-01    a1b2c3d4…   True        47
```

To read the count directly:

```bash
kubectl get clusterprofile this-cluster -o jsonpath='{.status.managedNamespaces}'
```

### Per-namespace CWP events

The operator emits a `CWPConfigured` event on each namespace after updating its CWP. Inspect them with:

```bash
kubectl describe namespace <ns-name>
```

Look for events of type `Normal` with reason `CWPConfigured`:

```
Events:
  Type    Reason         Age    From                               Message
  ----    ------         ----   ----                               -------
  Normal  CWPConfigured  2m5s   clusterprofile-controller          CWP configured: managed=true enforcement=visibility_only
```

### Verifying a specific namespace

```bash
# Check which annotations are set
kubectl get namespace <ns-name> -o jsonpath='{.metadata.annotations}' | jq .

# Watch the CWP update event stream
kubectl get events --field-selector involvedObject.name=<ns-name>,involvedObject.kind=Namespace
```

## Troubleshooting

**Namespace not being updated**

If a namespace is expected to be managed but does not appear in the count or events, the most likely cause is that Kubelink has not yet created a CWP for it. The operator skips namespaces with no CWP and retries on the next reconcile. Check that the C-VEN agent and Kubelink are running in the cluster.

**Wrong enforcement mode**

Check annotation precedence first. An annotation on the namespace itself overrides everything:

```bash
kubectl get namespace <ns-name> -o jsonpath='{.metadata.annotations.microsegment\.io/enforcement}'
```

**`fromNamespaceLabel` assignment not applied**

The assignment is silently skipped if the referenced k8s label is absent from the namespace. Verify the label exists:

```bash
kubectl get namespace <ns-name> --show-labels
```

**Workloads have the wrong `role`, or a `LabelMapOverlap` warning appears**

`role` should come from the C-VEN `LabelMap`, not the CWP. If a `ClusterProfile` and a `LabelMap` both assign the same key, the operator raises a `LabelMapOverlap` warning on the `ClusterProfile`. See [LabelMap coexistence](labelmap-and-the-operator.md#the-golden-rule-dont-let-both-write-the-same-key).
