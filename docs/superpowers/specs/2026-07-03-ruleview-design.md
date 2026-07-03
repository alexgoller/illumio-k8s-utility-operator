# RuleView — current Illumio rules, in kubectl (design)

**Goal:** let a Kubernetes user see the Illumio rules that currently govern their namespace's app —
**including rules authored outside Kubernetes** — without opening the Illumio UI. A read-only,
namespaced `RuleView` CRD mirrors the PCE **Rule Search** result for the namespace's **provider**
scope.

This completes the "live entirely in kubectl" loop: author (`SegmentationIntent`/`Policy`) → preflight
impact (`PolicyInsight`) → **see current effective rules (`RuleView`)** → labels/enforcement
(`ClusterProfile`).

## Decisions (agreed)

- **Periodic sync + on-demand refresh** (Rule Search is a cheap policy-config query, unlike Explorer
  flow queries — safe on a timer).
- **Provider-side** rules (where the app is the provider — ingress *to* it). Consumer-side is a later
  addition.
- Name **`RuleView`** (short name `rview`).
- Cap the listed rules for etcd safety; keep exact counts.

## CRD

```yaml
apiVersion: microsegment.io/v1alpha1
kind: RuleView
metadata: { name: current, namespace: payments }
spec:
  refreshIntervalMinutes: 5     # periodic cadence; default 5, min 1, max 1440
  policyVersion: active         # active (default) | draft
status:
  observedAt: 2026-07-03T14:00:00Z
  ruleCount: 7
  ownedCount: 4                 # authored via this operator
  externalCount: 3             # authored OUTSIDE Kubernetes
  truncated: false
  rules:                       # capped list (default 200)
    - href: /orgs/1/sec_policy/active/rule_sets/42/sec_rules/9
      rulesetName: payments-ingress
      ownedBy: operator         # operator | external
      type: allow               # allow | deny | override_deny
      enabled: true
      consumers: ["app=checkout;env=prod"]
      services: ["8443/TCP"]
  conditions:
    - type: Ready   # reasons: Synced | ClusterProfileNotReady | NoScopeLabels | QueryFailed
```

`RuleSummary` fields: `href`, `rulesetName`, `ownedBy`, `type`, `enabled`, `consumers []string`
(label-set strings or `ams`/`ip_list:<name>`), `services []string` (`<port>/<proto>` or
`All Services`). Print columns: `READY`, `RULES` (`ruleCount`), `OWNED`, `EXTERNAL`, and `SYNCED`
(observedAt age).

## Architecture

Mirrors `PolicyInsight` (read-only report CRD + ClusterProfile/scope resolution + cap-for-etcd).

1. **PCE client — `internal/pce/rulesearch.go`:** `SearchRules(ctx, RuleSearchQuery) ([]FoundRule, error)`.
   - `RuleSearchQuery{ ProviderLabelHrefs []string; PolicyVersion string }`.
   - `FoundRule{ Href, RulesetHref, RulesetName, RulesetExternalDataSet string; Enabled bool; Type string; Consumers []Actor; Services []IngressService }`.
   - Wraps `POST /orgs/{org}/sec_policy/{version}/rule_search` with providers = the scope labels.
   - **Live-confirm (spike):** exact request/response field paths, and whether the response carries the
     ruleset's `external_data_set` (needed for owned/external — otherwise a secondary ruleset lookup).
2. **Mapper — `internal/controller/ruleview.go` (pure):**
   `mapRules(found []pce.FoundRule, eds string, cap int) (rules []microv1.RuleSummary, counts, truncated)`.
   Flags `ownedBy=operator` when `RulesetExternalDataSet == eds` (the operator's data set), else
   `external`. Renders actors/services to display strings. Sorted, capped.
3. **Controller — `RuleViewReconciler`:** periodic (`RequeueAfter`) + on-demand. A **time-gate**
   (`now − observedAt ≥ interval`, unless the refresh annotation / generation changed) makes the
   status-write-triggered reconcile a no-op, so it re-queries on cadence, not in a loop. Resolves the
   ClusterProfile + provider scope labels (reusing `resolveClusterProfile` / `ScopeLabelKeys` /
   `scopeLabelValues`), runs `SearchRules`, maps, writes status.
4. **Client interface — `RuleViewClient` (FindLabel + SearchRules) + factory.**

## Data flow

```
RuleView (ns=payments)  → resolve ClusterProfile + provider scope labels (app,env → hrefs)
  → SearchRules(providers=scope hrefs, version=active)
  → mapRules(found, eds, cap=200)  → owned/external counts + capped list
  → write status; RequeueAfter = refreshIntervalMinutes
```

## Error handling

- No ClusterProfile / PCE not connected → `Ready=False` `ClusterProfileNotReady`; retry on cadence.
- No scope labels → `Ready=False` `NoScopeLabels`.
- Rule Search failure → `Ready=False` `QueryFailed`; retry on cadence (cheap query, so a periodic
  retry is fine — unlike the flow-query preflight).
- Large result → cap at 200, `truncated=true`; counts stay exact.

## Testing

- **Mapper unit tests:** owned-vs-external flagging by `external_data_set`; actor/service rendering;
  cap + truncation; counts.
- **Client:** stubbed HTTP server replaying a recorded `rule_search` response.
- **Controller envtest:** fake search client returns a mix of owned + external rules → assert status
  counts, `ownedBy`, and the periodic-gate no-op.

## Docs / chart

- Reference `docs/reference/ruleview.md`; a short guide section; nav + README + `api.md`.
- Chart: manager-role RBAC for `ruleviews`; CRD synced to `dist/chart/crds/`.

## Non-goals

Consumer-side (egress) rules; editing rules from k8s (read-only); resolving label groups / IP lists to
members; deny-rule authoring (view only shows type).
