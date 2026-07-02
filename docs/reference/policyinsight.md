# PolicyInsight

Namespaced. Requests an **on-request what-if preflight** for its namespace: the operator queries the PCE for observed traffic and its **draft (what-if) policy decision**, then reports the flows the current draft policy would block. Read-only — it authors no policy.

!!! warning "On request only — never periodic"
    PCE traffic (Explorer) queries are expensive. A preflight runs **only** when you create the `PolicyInsight`, change its spec, or bump the `microsegment.io/refresh` annotation. The operator computes once per request and then idles — it never polls the PCE on a timer.

Short name: `insight`. Category: `illumio`.

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `lookbackDays` | integer | no | Observation window in days, ending now. 1–90. Defaults to `7`. |

## Trigger a run

```bash
# create → runs once
kubectl apply -f - <<'YAML'
apiVersion: microsegment.io/v1alpha1
kind: PolicyInsight
metadata: { name: preflight, namespace: payments }
spec: { lookbackDays: 7 }
YAML

# re-run on demand
kubectl annotate policyinsight preflight -n payments microsegment.io/refresh="$(date +%s)" --overwrite

# read the findings
kubectl get policyinsight preflight -n payments -o yaml
```

## Outcome

The status gives both a **decision breakdown** (allowed / potentially-blocked / blocked, per direction — allowed flows are counted, not listed) and the **listed blocked flows** to act on:

```yaml
status:
  summary:
    inbound: { allowed: 118, potentiallyBlocked: 1, blocked: 0, total: 119 }
    outbound:  { allowed: 40,  potentiallyBlocked: 1, blocked: 0, total: 41 }
  wouldBlockInbound:
    - peer: { app: checkout, env: prod }
      port: 8443
      protocol: TCP
      connections: 312
      decision: potentially_blocked
  wouldBlockOutbound:
    - peer: { app: ledger }
      port: 5432
      decision: potentially_blocked
  conditions:
    - type: Ready
      reason: Computed
      message: "inbound: 118 allowed / 1 potentially-blocked / 0 blocked; outbound: 40 allowed / 1 potentially-blocked / 0 blocked"
```

Two things to read correctly:

- **The lists (`wouldBlockInbound` / `wouldBlockOutbound`) hold *both* `blocked` and `potentially_blocked` flows** — each record's `decision` field says which. So a `wouldBlockOutbound` entry with `decision: potentially_blocked` is expected; the list name means "flows the draft policy blocks or would block."
- **The `summary` counts individual flows; the lists are deduplicated** by `(peer, port, protocol)`. So a list can be *shorter* than the summary's `potentiallyBlocked + blocked` count (many flows collapse to one peer/port entry). The `summary` and `*Count` fields are the source of truth for totals.

`kubectl get policyinsight` shows `In-Allowed`, `In-Blocked`, `Out-Allowed`, `Out-Blocked` at a glance.

## Status

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | []Condition | Standard conditions. See below. |
| `observedWindow` | object | `{from, to}` — the time range analyzed by the last run. |
| `summary` | object | Draft-decision breakdown per direction: `inbound` and `outbound`, each `{allowed, potentiallyBlocked, blocked, unknown, total}`. Allowed flows are **counted here** (not listed individually). |
| `flowsAnalyzed` | integer | Number of flows the last run examined. |
| `truncated` | boolean | True when the flow result was capped (findings are partial). |
| `wouldBlockInbound` | []FlowFinding | Flows **to** this namespace's app the draft policy would block at `full` — allow-list gaps. Capped at 500 entries (highest-connection first) for etcd safety; `inboundBlockedCount` holds the true total. |
| `wouldBlockInboundTruncated` | boolean | True when the inbound list was capped (more distinct findings exist than are listed). |
| `wouldBlockOutbound` | []FlowFinding | Flows **from** this namespace's workloads that are denied. Surfaced for awareness; this operator does not author outbound policy. Capped like `wouldBlockInbound`. |
| `wouldBlockOutboundTruncated` | boolean | True when the outbound list was capped. |
| `inboundBlockedCount` / `outboundBlockedCount` | integer | Lengths of the above (print columns). |
| `observedGeneration` | integer | Spec generation the status reflects. |
| `observedRefresh` | string | The `microsegment.io/refresh` value honored by the last run. |

### FlowFinding

| Field | Type | Description |
|-------|------|-------------|
| `peer` | map[string]string | Illumio labels of the other end (consumer for inbound, provider for outbound). Empty for an unlabeled/off-cluster peer. |
| `peerIP` | string | The other end's IP when it has no workload labels. |
| `port` | integer | Destination port. |
| `protocol` | string | `TCP` or `UDP`. |
| `connections` | integer | Observed connection count over the window. |
| `decision` | string | The draft decision that flagged the flow (`blocked` or `potentially_blocked`). |
| `lastDetected` | timestamp | When the flow was last observed. |

### The `Ready` condition

| Reason | Status | Meaning |
|--------|--------|---------|
| `Computed` | `True` | The preflight ran; findings are in status. |
| `NoScopeLabels` | `False` | The namespace is not managed, or none of its scope labels (`app`/`env`) are assigned / present in the PCE. |
| `ClusterProfileNotReady` | `False` | No Onboarded `ClusterProfile` / connected `PCEConnection` for this namespace. Re-request after it is ready. |
| `QueryFailed` | `False` | The PCE traffic query failed. The `message` has detail. Re-request (bump `microsegment.io/refresh`) to retry. |

## Requirements

- PCE **Core 23.2.10+** for the draft policy decision (`draft_policy_decision`). On older PCEs the query still runs but the what-if signal is unavailable.
- The namespace must be managed by a `ClusterProfile` with scope labels (see [Namespace management](../guides/namespace-management.md#scope-vs-role-what-the-cwp-should-and-shouldnt-label)).
