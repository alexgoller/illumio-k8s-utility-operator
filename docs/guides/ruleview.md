# Live rule view (RuleView)

**See the Illumio rules that protect your app — in `kubectl`, without opening the Illumio UI.** A
`RuleView` periodically mirrors the current rules where your namespace's app is the **provider**
(ingress *to* it), pulled from the PCE **Rule Search API** — **including rules authored outside
Kubernetes** (the Illumio console, other teams, VM-side policy). It is **read-only**: the operator
never authors or edits rules from a `RuleView`.

This is the piece that lets a Kubernetes user *live on this platform*: you already author policy
(`SegmentationIntent` / `SegmentationPolicy`) and preflight it (`PolicyInsight`) from kubectl —
`RuleView` closes the loop by showing what is **actually in effect**.

## Why it matters

From kubectl today you can only see the rules *you* authored (in a `SegmentationIntent`/`Policy`
status). You're blind to rules made elsewhere that still govern your workloads. `RuleView` surfaces
**all** of them and flags each as **operator-owned** or **external** — so you instantly see policy
applied to your app that did *not* come through Kubernetes.

## When to use it

- **Audit what protects your app** — the full provider-side rule set, in one place.
- **Spot rules made outside k8s** — `externalCount` and each `ownedBy: external` entry.
- **Confirm what you authored took effect** — your `SegmentationIntent`/`Policy` should appear as
  `ownedBy: operator`.

## Requirements

- The namespace is managed by a `ClusterProfile` and carries scope labels (`app`/`env`).
- Access to the PCE Rule Search API (any connected `PCEConnection`).

## Use it

```bash
kubectl apply -f - <<'YAML'
apiVersion: microsegment.io/v1alpha1
kind: RuleView
metadata:
  name: current
  namespace: payments
spec:
  refreshIntervalMinutes: 5     # periodic sync cadence (1–1440); default 5
  policyVersion: active         # active (enforced, default) | draft
YAML

kubectl get ruleview -n payments
# NAME      READY   RULES   OWNED   EXTERNAL   SYNCED
# current   True    7       4       3          30s
```

### Read the rules

```bash
kubectl get ruleview current -n payments -o yaml
```

```yaml
status:
  observedAt: 2026-07-03T14:00:00Z
  ruleCount: 7
  ownedCount: 4                 # authored via this operator
  externalCount: 3             # authored OUTSIDE Kubernetes
  truncated: false
  rules:
    - href: /orgs/1/sec_policy/active/rule_sets/42/sec_rules/9
      rulesetName: payments-ingress
      ownedBy: operator
      type: allow
      enabled: true
      consumers: ["label:checkout-prod"]
      services: ["8443/TCP"]
    - href: /orgs/1/.../sec_rules/88
      rulesetName: admin-baseline
      ownedBy: external          # ← made in the Illumio UI / by another team
      type: allow
      consumers: ["ams"]         # All Workloads
      services: ["All Services"]
```

| Field | Meaning |
|---|---|
| `ownedBy` | `operator` (this operator authored the rule's ruleset) or `external` (authored outside k8s). |
| `type` | `allow`, `deny`, or `override_deny`. |
| `consumers` | Rule sources — `label:<id>`, `ams` (All Workloads), or `ip_list:<name>`. |
| `services` | `<port>/<proto>` (e.g. `8443/TCP`) or `All Services`. |

## Refresh model — periodic + on-demand

`RuleView` re-syncs **automatically** on `spec.refreshIntervalMinutes` (default 5 minutes — Rule
Search is a light policy-config query, so a periodic sync is fine). For an **immediate** refresh, bump
the annotation:

```bash
kubectl annotate ruleview current -n payments microsegment.io/refresh="$(date +%s)" --overwrite
```

The `SYNCED` column (from `status.observedAt`) shows how fresh the data is.

## etcd safety

The listed rules are **capped** (default 200) so a large scope can't bloat the object — `truncated`
is set when capping happens, but **`ruleCount` / `ownedCount` / `externalCount` stay exact**. Use the
counts as the source of truth for totals.

## Troubleshooting

Check the `Ready` condition reason:

```bash
kubectl get ruleview current -n payments \
  -o jsonpath='{.status.conditions[?(@.type=="Ready")].reason}: {.status.conditions[?(@.type=="Ready")].message}{"\n"}'
```

| Reason | Meaning / fix |
|---|---|
| `Synced` | Success — the rule list is in `status`. |
| `NoScopeLabels` | The namespace isn't managed, or its `app`/`env` scope labels aren't assigned / not in the PCE. |
| `ClusterProfileNotReady` | No Onboarded `ClusterProfile` / connected `PCEConnection` for this namespace. |
| `QueryFailed` | The PCE Rule Search query failed; the message has detail. Retries on the next interval. |

## See also

- [PolicyInsight reference](../reference/policyinsight.md) — the what-if preflight that pairs with this.
- [RuleView reference](../reference/ruleview.md) — full field and status documentation.
- [Policy concepts](policy-concepts.md) — scope, providers/consumers, rules vs enforcement.
