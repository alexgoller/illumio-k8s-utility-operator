# Preflight — PCE traffic API spike findings

> Spike (research, no code) for Phase A of [issue #47](https://github.com/alexgoller/illumio-k8s-utility-operator/issues/47).
> Researched 2026-07-02 from the Illumio Core Explorer REST API docs (24.2). Goal: confirm the
> operator can query observed flows filtered by label and read a **draft ("what-if") policy
> decision** — the load-bearing dependency for preflight.

## Verdict: feasible ✅

The Explorer async traffic-flow API gives us exactly what preflight needs, **including a distinct
draft policy decision** (added in Core **23.2.10**). No CNI instrumentation required — the PCE
already has the flows, labeled, with both reported and draft decisions.

## Endpoints (async, three-step)

```
POST [org]/traffic_flows/async_queries            → returns a query {href, uuid, status}
GET  [org]/traffic_flows/async_queries/:uuid       → poll until status == "completed"
GET  [org]/traffic_flows_async/queries/:uuid/download  → download the result flows
```

⚠️ The 24.2 doc renders the download path inconsistently (`traffic_flows_async/queries/...` and a
typo'd `dowload`). **Live-confirm the exact download path + whether results are a single JSON blob
or paged** before finalizing the client.

## Request body (what we send)

| Field | Use for preflight |
|---|---|
| `sources` | `{include:[[{label:{href}}...]]}` — set to the namespace scope (app+env) for the **egress** query |
| `destinations` | set to the namespace scope for the **inbound** query (response calls these `providers`) |
| `services` | leave open (all ports) or narrow later; entries are `{port, to_port, proto, process_name, ...}` |
| `policy_decisions` | filter to `["potentially_blocked","blocked"]` to fetch only the problem flows |
| `start_date` / `end_date` | ISO 8601; our lookback window (default 7d) |
| `max_results` | integer, **limit 200,000** — we cap far lower and set `truncated` |
| `data_sources` | optional (`server`/`endpoint`/`flowlink`/`scanner`) |

**Direction is expressed by which side carries the scope labels:** destinations=scope → inbound to
the app; sources=scope → egress from the app. So Phase A runs **two label-filtered queries**
(inbound, egress) rather than one broad query + client-side classification — lighter and precise.

## Response flow schema (what we read)

Each flow object includes:
- `src` / `dst` — IP, plus the workload object with `href`, `name`, and a **labels array** (`key`,
  `value`, `href`), OS, hostname, enforcement mode.
- `service` — `{port, proto}` (proto 6=TCP, 17=UDP).
- `num_connections` — integer.
- `policy_decision` — **reported** decision (as enforced by the VEN).
- **`draft_policy_decision`** — the **what-if** decision under current draft policy. *(Core 23.2.10+.)*
- `timestamp_range` — `{first_detected, last_detected}`.

Decision values: `allowed`, `potentially_blocked`, `blocked`, `unknown`.

## What this means for the design

- The `internal/pce/traffic.go` client (`QueryTraffic`) implements the POST→poll→download dance.
  It should accept a direction (which side gets the scope labels) and a decision filter, so the
  controller issues the inbound and egress queries separately.
- The classifier reads **`draft_policy_decision`** (not `policy_decision`) — preflight is about what
  the *draft/what-if* policy would do, which is the whole point. (Reported decision is useful later
  for C/drift.)
- Confirms Phase A evaluates the **draft policy already in the PCE**. Injecting a fully hypothetical
  ruleset is not supported by this API shape (the draft decision reflects the PCE's current draft),
  so hypothetical what-if stays deferred to B (epic open question #2) — consistent with the design.

## Open items to confirm against the live PCE (like the deny spike)

1. **PCE version ≥ Core 23.2.10** — required for `draft_policy_decision`. Confirm the target US2 PCE
   version. If older, preflight can still show *reported* blocked flows but not the draft what-if
   (degraded mode).
2. **Exact download path + result format** — single JSON array vs paged; the 24.2 doc is
   inconsistent. Capture a real request/response to fix the client and to seed the replay tests.
3. **Does `draft_policy_decision` reflect operator-created draft rulesets** (draft/manual mode) as
   expected? Create a draft `SegmentationIntent`, observe a flow it would block, and confirm the
   flow's `draft_policy_decision` is `potentially_blocked`/`blocked`.
4. **Query latency / result volume** for a busy namespace over 7d — informs the `max_results` cap and
   the user-facing "computing" UX. **Not** a requeue cadence: preflight is **on-request only** and
   must never poll the PCE on a timer (these queries are expensive and load the PCE).

## Recommended next step

Proceed to implement `QueryTraffic` once (1) and (2) are confirmed live. The design
(`2026-07-02-policy-preflight-design.md`) holds; only field names and the download path may need
small adjustment from the captured live response.

## Sources
- [Illumio Core 24.2 — Explorer REST API (`traffic_flows/async_queries`)](https://product-docs-repo.illumio.com/Tech-Docs/Core/24.2/REST-APIs/out/en/rest-apis/visualization/explorer.html)
- [Illumio Core — Work with Explorer (Reported vs Draft View)](https://product-docs-repo.illumio.com/Tech-Docs/Core/22.2/Visualization/out/en/visualization/explorer/work-with-explorer.html)
