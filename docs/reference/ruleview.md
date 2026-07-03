# RuleView

Namespaced, **read-only**. Mirrors the current Illumio rules that protect this namespace's app (where the app is the **provider**), pulled from the PCE **Rule Search API** — **including rules authored outside Kubernetes**. See what governs your app without opening the Illumio UI. The operator never authors or edits rules from a `RuleView`.

Short name: `rview`. Category: `illumio`.

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `refreshIntervalMinutes` | integer | no | Periodic sync cadence (1–1440). The operator re-queries every interval, and immediately on a `microsegment.io/refresh` annotation change. Defaults to `5`. |
| `policyVersion` | string | no | Which policy store to mirror: `active` (enforced, default) or `draft`. |

## Use it

```bash
kubectl apply -f - <<'YAML'
apiVersion: microsegment.io/v1alpha1
kind: RuleView
metadata: { name: current, namespace: payments }
spec: { refreshIntervalMinutes: 5 }
YAML

kubectl get ruleview -n payments
# NAME      READY   RULES   OWNED   EXTERNAL   SYNCED
# current   True    7       4       3          30s

kubectl get ruleview current -n payments -o yaml   # full rule list

# force an immediate re-sync
kubectl annotate ruleview current -n payments microsegment.io/refresh="$(date +%s)" --overwrite
```

## Status

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | []Condition | Standard conditions. See below. |
| `observedAt` | timestamp | When the last successful sync ran (the `SYNCED` column shows its age). |
| `ruleCount` | integer | Total rules found for this app (provider side). |
| `ownedCount` | integer | Rules **this operator** authored. |
| `externalCount` | integer | Rules authored **outside Kubernetes** (Illumio UI, other teams, VM-side policy). |
| `truncated` | boolean | True when the listed rules were capped (default 200); the counts stay exact. |
| `rules` | []RuleSummary | The (capped) list of rules protecting this app. |

### RuleSummary

| Field | Type | Description |
|-------|------|-------------|
| `href` | string | The PCE rule href. |
| `rulesetName` | string | The ruleset the rule belongs to. |
| `ownedBy` | string | `operator` (this operator authored the rule's ruleset) or `external` (authored outside Kubernetes). |
| `type` | string | `allow`, `deny`, or `override_deny`. |
| `enabled` | boolean | Whether the rule is enabled. |
| `consumers` | []string | Rule sources, rendered as `label:<id>`, `ams` (All Workloads), or `ip_list:<name>`. |
| `services` | []string | Allowed services, rendered as `<port>/<proto>` or `All Services`. |

### The `Ready` condition

| Reason | Status | Meaning |
|--------|--------|---------|
| `Synced` | `True` | The rule list was refreshed; findings are in `status`. |
| `NoScopeLabels` | `False` | The namespace isn't managed, or its `app`/`env` scope labels aren't assigned / not in the PCE. |
| `ClusterProfileNotReady` | `False` | No Onboarded `ClusterProfile` / connected `PCEConnection` for this namespace. |
| `QueryFailed` | `False` | The PCE Rule Search query failed; the message has detail. Retries on the next interval. |

## Notes

- **Read-only.** A `RuleView` never changes policy. It's a live-ish window onto the PCE.
- **Owned vs external** is the differentiator: `externalCount` and each `ownedBy: external` entry show rules governing your app that were **not** authored through this operator — visibility you otherwise only get in the Illumio UI.
- **Provider-side only** for now (rules where your app is the destination). Consumer-side (your egress) is a possible later addition.
- Requires the namespace to be managed by a `ClusterProfile` with scope labels (`app`/`env`).
