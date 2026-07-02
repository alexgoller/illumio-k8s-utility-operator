# Policy What-If Preflight (Phase A) — design

> Phase **A** of the epic *"leverage PCE flow data + draft policy decisions to compute policy
> that works"* ([issue #47](https://github.com/alexgoller/illumio-k8s-utility-operator/issues/47)).
> B (recommend rules from flows) and C (continuous drift) are out of scope here but the surface
> introduced by A is designed to extend to them.

## Goal

For a managed namespace, tell the user the **impact of enforcing policy before they enforce it**,
using traffic the PCE has already observed and its **draft ("what-if") policy decision**:

- **Would-be-blocked inbound** — flows *to* the namespace's app that the current draft policy would
  block at `full` enforcement. These are gaps in the allow-list: enforcing now would break them.
- **Blocked egress** — flows *from* the namespace's workloads that are (or would be) denied. Our
  ruleset model is ingress-centric and does not *author* egress, but the user must still **see**
  outbound the app needs but cannot make.

This is the safety net that makes later rule generation (B) trustworthy. Illumio's UI Policy
Generator is **deprecated**, so preflight is built **API-first** on the Explorer / traffic query API.

## Hard constraint: on-request only, never periodic

PCE traffic (Explorer) queries are **expensive** and have real load impact on the PCE. Preflight
**must never run on a reconcile timer or background poll.** A run is triggered **only by an explicit
user request** — creating a `PolicyInsight`, changing its spec, or bumping a refresh annotation. The
controller computes exactly once per request and then sits idle (no `RequeueAfter`). This shapes the
whole design and applies to every later phase (B/C) too: any flow query is user-initiated.

## Non-goals (Phase A)

- Generating or writing allow rules (that is B).
- Injecting a *hypothetical* policy into the query. A evaluates the **draft policy already in the
  PCE** (whatever the operator has compiled, incl. draft/manual-mode rulesets). Hypothetical
  what-if is an open question deferred to B (epic open question #2).
- Authoring egress policy. A only *surfaces* blocked egress.
- Auto-changing enforcement. A informs; the human decides.

## Architecture

Four units with clear boundaries:

### 1. PCE traffic client — `internal/pce/traffic.go`

A new client capability to query observed flows. Illumio's traffic query is **asynchronous**:
`POST` a query → poll for completion → download results. The method wraps that:

```go
// TrafficQuery selects observed flows for a scope + time window.
type TrafficQuery struct {
    Labels    []string  // scope label hrefs (the namespace app+env); matches src OR dst
    From, To  time.Time // observation window (e.g. last 7d)
    MaxResults int
}

// TrafficFlow is one aggregated observed flow with its policy decision.
type TrafficFlow struct {
    SrcLabels map[string]string // resolved key->value (may be partial / workload)
    DstLabels map[string]string
    SrcIP, DstIP string
    Port      int
    Protocol  int               // 6=TCP, 17=UDP
    Decision  string            // policy decision: allowed / potentially_blocked / blocked / unknown
    Connections int
    LastDetected time.Time
}

func (c *Client) QueryTraffic(ctx context.Context, q TrafficQuery) ([]TrafficFlow, bool /*truncated*/, error)
```

**Exact endpoint, request/response schema, the draft-vs-active decision field, and the direction
signal are confirmed by the spike** (`2026-07-02-preflight-traffic-api-spike.md`) before this is
implemented. The signature above is the target; field names adjust to the real API.

### 2. Classifier — `internal/controller/preflight.go` (pure)

A pure function over the flows + the namespace scope labels:

```go
func classifyFlows(flows []TrafficFlow, scope map[string]string) PreflightFindings
```

- A flow whose **dst** matches the scope (inbound) with decision `blocked`/`potentially_blocked`
  → `WouldBlockInbound`.
- A flow whose **src** matches the scope (egress) with decision `blocked`/`potentially_blocked`
  → `BlockedEgress`.
- `allowed` flows are ignored. Findings are de-duplicated and sorted (stable output).

Pure and table-testable — no PCE or k8s dependency.

### 3. Surface — a new `PolicyInsight` CRD (namespaced, read-only status)

The reusable "first pattern." The user **opts in** by creating a `PolicyInsight` in their namespace
(a request for a preflight); the operator populates status. It exists **independent of any
`SegmentationIntent`** — you want insight *before* you have a policy — and is the natural home for
blocked-egress (which is not tied to an inbound CR). B adds `suggestedRules[]`; C keeps it fresh.

```yaml
apiVersion: microsegment.io/v1alpha1
kind: PolicyInsight
metadata:
  name: preflight
  namespace: payments
  annotations:
    microsegment.io/refresh: "2026-07-02T10:00:00Z"   # bump to re-run on demand (on-request only)
spec:
  lookbackWindow: 7d          # optional; default 7d
status:
  observedWindow: { from: ..., to: ... }
  flowsAnalyzed: 1240
  truncated: false
  wouldBlockInbound:          # []FlowFinding
    - peer: { app: checkout, env: prod }
      port: 8443, protocol: TCP, connections: 312, lastDetected: ...
  blockedEgress:
    - peer: { app: ledger, env: prod }      # or peerIP for off-cluster
      port: 5432, protocol: TCP, connections: 12, lastDetected: ...
  conditions:
    - type: Ready   # Computed / PCEConnectionNotReady / QueryFailed / ClusterProfileNotReady
```

`FlowFinding`: `{ peer map[string]string, peerIP string, port int, protocol string, connections int,
lastDetected metav1.Time }`. Print columns: `INBOUND-BLOCKED`, `EGRESS-BLOCKED`, `WINDOW`.

### 4. Controller — `PolicyInsightReconciler`

Reconciles `PolicyInsight` objects **on request only**: resolve the ClusterProfile + scope labels for
the namespace (reusing `resolveClusterProfile`/`ScopeLabelKeys`), run `QueryTraffic` for the window,
`classifyFlows`, write status — then **return with no `RequeueAfter`** so it never re-queries on a
timer.

A run happens exactly once per request. A request is:
- **create** a `PolicyInsight`, or
- **change its spec** (observed via `metadata.generation` vs `status.observedGeneration`), or
- **bump the refresh annotation** `microsegment.io/refresh=<any-new-value>` (re-run without editing
  spec; the operator records the honored token in status to detect the next bump).

If nothing changed (generation and refresh token already reflected in status), the reconcile is a
**no-op** — it does not touch the PCE. Missing ClusterProfile/PCE → `Ready=False` with the matching
reason and **no automatic retry** (the user re-requests); transient PCE/query failures may use a
single bounded retry, but never a standing periodic requeue.

## Data flow

```
PolicyInsight (user-created, ns=payments)
   → resolve ClusterProfile + scope labels (app=payments, env=prod)
   → QueryTraffic(labels=[app,env hrefs], window=7d, both directions)   [async: POST→poll→fetch]
   → classifyFlows(flows, scope)  → {wouldBlockInbound, blockedEgress}
   → write PolicyInsight.status
```

## Error handling

- **Async query failure / timeout** → `Ready=False` reason `QueryFailed`. At most a *single bounded*
  retry — never a standing periodic requeue. The user re-requests (refresh annotation) to try again.
- **No ClusterProfile / PCE not connected** → `Ready=False` reason `ClusterProfileNotReady` /
  `PCEConnectionNotReady`. No automatic re-query; the user re-requests once the dependency is ready.
- **Large result sets** → cap at `MaxResults`, set `status.truncated=true`, and `log()`/note it so
  truncation never reads as "nothing more to see." Scope every query by namespace labels + bounded
  window to keep volume sane.
- **No flows observed** → empty findings, `Ready=True` (valid answer: nothing seen in the window).

## Testing

- **Classifier**: table-driven unit tests over synthetic `TrafficFlow` sets (inbound blocked, egress
  blocked, allowed ignored, dedupe, scope-mismatch ignored, empty).
- **Traffic client**: unit tests against a stubbed HTTP server replaying recorded async-query
  responses (captured during the spike).
- **Controller**: envtest with a fake traffic client returning canned flows → assert `PolicyInsight`
  status is populated and conditions are set; PCE-not-ready path → `Ready=False`.

## Dependency / sequencing

1. **Spike (first):** confirm the Explorer / `traffic_flows/async_queries` API against a live PCE —
   endpoint, label + time filtering, the **draft** decision field, and the direction signal.
   Findings → `2026-07-02-preflight-traffic-api-spike.md`. This is load-bearing; the client shape
   above is provisional until confirmed.
2. Traffic client (`QueryTraffic`) with recorded-response tests.
3. `PolicyInsight` CRD + deepcopy + chart CRD sync + RBAC.
4. Classifier + controller + envtest.
5. Docs (guide + reference) and a chart bump.
