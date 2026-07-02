# Policy Preflight (PolicyInsight)

**See what a segmentation policy would block — before you enforce it.** A `PolicyInsight` runs a
what-if preflight for a namespace: it asks the PCE for the traffic it has actually observed, reads
the **draft ("what-if") policy decision** for each flow, and reports what the current draft policy
would block. Nothing is enforced and no policy is authored — it is a read-only report you request.

This is the Kubernetes-native answer to a question `NetworkPolicy` can't answer on its own: *"if I
turn this on, what breaks?"* The PCE already has the flows — labeled, org-wide, with a draft
decision — so no CNI instrumentation is needed.

!!! warning "On request only — never on a timer"
    PCE traffic (Explorer) queries are expensive. A preflight runs **only** when you create a
    `PolicyInsight`, change its spec, or bump its `microsegment.io/refresh` annotation. The operator
    computes once per request and then idles — it never polls the PCE in the background.

## When to use it

- **Before going to `full` enforcement** — confirm your allow-list actually covers the real traffic,
  so you don't break flows the moment you enforce.
- **Before removing or tightening a policy** — see which flows currently rely on it.
- **To discover what a namespace actually talks to** — the summary breaks all observed flows down by
  decision, and the findings list the ones that would be blocked.

## Requirements

- The namespace is managed by a `ClusterProfile` and carries scope labels (`app`/`env`) — see
  [Namespace management](namespace-management.md#scope-vs-role-what-the-cwp-should-and-shouldnt-label).
- The PCE is **Core 23.2.10+** (for the `draft_policy_decision` field). On older PCEs the query still
  runs but the what-if signal is unavailable.
- The PCE has observed traffic for the namespace in the lookback window (the C-VEN must be reporting
  flows; `visibility_only` or `full` enforcement records them, `idle` does not).

## Run a preflight

Create a `PolicyInsight` in the namespace you want to analyze:

```bash
kubectl apply -f - <<'YAML'
apiVersion: microsegment.io/v1alpha1
kind: PolicyInsight
metadata:
  name: preflight
  namespace: payments
spec:
  lookbackDays: 7        # optional, 1–90; defaults to 7
YAML
```

The operator runs once and writes the result to `status`. Watch it:

```bash
kubectl get policyinsight -n payments
# NAME        READY   IN-ALLOWED   IN-BLOCKED   EG-ALLOWED   EG-BLOCKED
# preflight   True    118          2            40           1
```

### Re-run on demand

Because preflight never re-runs on its own, trigger a fresh run by bumping the refresh annotation
(or by editing `spec.lookbackDays`):

```bash
kubectl annotate policyinsight preflight -n payments \
  microsegment.io/refresh="$(date +%s)" --overwrite
```

## Read the outcome

The full result is in `status`:

```bash
kubectl get policyinsight preflight -n payments -o yaml
```

```yaml
status:
  conditions:
    - type: Ready
      status: "True"
      reason: Computed
      message: "inbound: 118 allowed / 2 potentially-blocked / 0 blocked; egress: 40 allowed / 1 potentially-blocked / 0 blocked"
  observedWindow: { from: 2026-06-25T…, to: 2026-07-02T… }
  summary:
    inbound: { allowed: 118, potentiallyBlocked: 2, blocked: 0, total: 120 }
    egress:  { allowed: 40,  potentiallyBlocked: 1, blocked: 0, total: 41 }
  wouldBlockInbound:
    - peer: { app: checkout, env: prod }
      port: 8443
      protocol: TCP
      connections: 312
      decision: potentially_blocked
      lastDetected: 2026-07-01T…
  blockedEgress:
    - peer: { app: ledger, env: prod }
      port: 5432
      protocol: TCP
      connections: 12
      decision: potentially_blocked
```

Two complementary views:

| Where | What it tells you |
|---|---|
| **`status.summary`** | The **decision breakdown** for *all* observed flows, per direction: `allowed` / `potentiallyBlocked` / `blocked` / `total`. Allowed flows are **counted** here, not listed. |
| **`status.wouldBlockInbound`** | The **inbound flows to act on** — consumers reaching your app that the draft policy would block at `full`. Each is a gap in your allow-list. |
| **`status.blockedEgress`** | Outbound flows *from* your app that are denied. Surfaced for awareness — this operator authors inbound policy, not egress (see [Policy concepts](policy-concepts.md)). |

**Decision meanings** (from the draft/what-if policy):

- `allowed` — the draft policy permits it. Nothing to do.
- `potentially_blocked` — allowed today, but the draft policy would block it at `full`. **These are
  your allow-list gaps.**
- `blocked` — already blocked by the draft policy.

## The workflow: preflight → tighten → enforce

1. **Preflight** the namespace while it is still permissive (`visibility_only`).
2. **Read `wouldBlockInbound`** — each finding is a consumer/port you'd break at `full`.
3. **Author a `SegmentationIntent`** (or `SegmentationPolicy`) that allows those real flows:
   ```yaml
   apiVersion: microsegment.io/v1alpha1
   kind: SegmentationIntent
   metadata: { name: payments-ingress, namespace: payments }
   spec:
     allow:
       - from: { app: checkout, env: prod }   # from a wouldBlockInbound finding
         ports: [{ port: 8443, protocol: TCP }]
   ```
4. **Re-run the preflight** (bump the refresh annotation). `wouldBlockInbound` should shrink as your
   rules cover the traffic.
5. When `In-Blocked` reaches 0 (or only expected denials remain), **move the namespace to `full`**
   with confidence.

> This is the manual form of the roadmap's "recommend policy from flows" (Phase B). Today you read
> the findings and write the rules; a future release will draft candidate rules for you.

## Safety notes

- **Read-only.** A `PolicyInsight` never changes policy, labels, or enforcement. Deleting it removes
  only the report.
- **No PCE load when idle.** After a run it sits still; it only queries the PCE when you request a
  run. Keep `lookbackDays` reasonable — a wider window is a heavier query.
- **Draft decision reflects the current draft policy in the PCE**, including rulesets the operator
  compiled in `draft-only`/`manual` mode. So you can compile a draft `SegmentationIntent`, preflight
  it, and see its effect *before* provisioning.
- **Bounded status size.** The listed findings are capped (500 per direction, highest-connection
  first) so even a very noisy namespace can't produce a status object that exceeds the
  etcd/apiserver object limit. If capping happens, `wouldBlockInboundTruncated` /
  `blockedEgressTruncated` is set, and the **true totals stay accurate** in `inboundBlockedCount` /
  `egressBlockedCount` and the `summary`.

## Troubleshooting

Check the `Ready` condition reason:

```bash
kubectl get policyinsight preflight -n payments \
  -o jsonpath='{.status.conditions[?(@.type=="Ready")].reason}: {.status.conditions[?(@.type=="Ready")].message}{"\n"}'
```

| Reason | Meaning / fix |
|---|---|
| `Computed` | Success — findings are in `status`. |
| `NoScopeLabels` | The namespace isn't managed, or its `app`/`env` scope labels aren't assigned / not in the PCE. Manage it via `ClusterProfile` and ensure the labels exist. |
| `ClusterProfileNotReady` | No Onboarded `ClusterProfile` / connected `PCEConnection` for this namespace. Fix that, then re-request (bump `microsegment.io/refresh`). |
| `QueryFailed` | The PCE traffic query failed; the message names the failing step. Re-request to retry. If it persists, confirm the PCE version and that Explorer/traffic queries are enabled. |

**No findings but you expected some?** Confirm the C-VEN is reporting flows for the namespace and
that the window (`lookbackDays`) covers when the traffic happened. `idle` namespaces record no flows.

## See also

- [PolicyInsight reference](../reference/policyinsight.md) — full field and status documentation.
- [Policy concepts](policy-concepts.md) — scope, rules vs enforcement, the ingress-centric model.
- [Segmentation policy](segmentation-policy.md) — authoring the intent the preflight informs.
