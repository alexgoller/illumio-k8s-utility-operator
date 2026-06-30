# Track 2 — Deny semantics: API spike findings

> Spike (research, no code) for issue #22 / roadmap Track 2. Researched 2026-06-30 from the
> Illumio Core REST API docs (24.2 / 24.5 / 25.4). Goal: decide HOW the operator should emit
> "override deny" and "default deny" to the PCE.

## Illumio's deny model (three rule types)

Illumio rules come in three types, with a fixed precedence:

| Type | Effect | Precedence |
|---|---|---|
| **Override Deny** | Blocks the traffic regardless of any other rule; **cannot** be overridden by an Allow. | Highest |
| **Allow** | Permits traffic. | Middle |
| **Deny** | Blocks traffic, **but** an Allow rule (in the same or another policy) overrides it → allowed. | Lowest |

So: **Override Deny > Allow > Deny.** (From Core 25.2.10 all three support label exclusion.)

This maps directly to the roadmap's two needs:
- **"Override deny" (deny that wins over allow)** → an **Override Deny** rule.
- **Deny (overridable)** → a **Deny** rule (softer; an allow elsewhere can punch through).

## Two mechanisms — use Deny Rules, not Enforcement Boundaries

| | **Deny Rules** (recommended) | **Enforcement Boundaries** (legacy) |
|---|---|---|
| Where | A rule **inside a ruleset** | A separate `/sec_policy/{ver}/enforcement_boundaries` object |
| Endpoint | `{ruleset_href}/deny_rules` (POST/PUT/DELETE/GET, parallel to `/sec_rules`) | `.../enforcement_boundaries` |
| Payload | **Same shape as an allow rule** (providers, consumers, ingress_services, enabled, …) | providers/consumers/ingress_services |
| Status | Modern; **replacing** Enforcement Boundaries | Being deprecated; tied to `selective` enforcement / "port enforcement" |
| Version | Core **25.x** | older (≤24.x) |

**Recommendation:** target **Deny Rules** (`{ruleset}/deny_rules`). They live in the ruleset the operator already owns/provisions, share the allow-rule JSON shape (so we reuse `pce.SecRule` + the All-Workloads/All-Services/scope machinery built in Tracks 4–5), and provision via the same `change_subset: {rule_sets:[…]}` call. Enforcement Boundaries would be a whole separate object model AND are deprecated.

## What this means for `internal/pce` + the controller

Small, additive — deny rules reuse almost everything:
- **pce client:** add `CreateDenyRule(ctx, ruleSetHref, rule)` → `POST {ruleSetHref}/deny_rules`, plus `ListDenyRules`/`DeleteDenyRule` (parallel to the existing `sec_rules` calls) for the replace-all rebuild.
- **`reconcileRuleSet`:** when rebuilding the ruleset, also delete+recreate the deny rules (replace-all, same as allow rules today).
- **`BuildRules`** already produces the right actor/service shape — a deny rule is the same body, just POSTed to `/deny_rules`.
- **Provisioning:** unchanged — deny rules are part of the ruleset, committed by the existing `ProvisionRuleSets` (change_subset on rule_sets).

## "Default / bottom deny" = enforcement, not a rule

In Illumio, **`full` enforcement is already default-deny** (anything not explicitly allowed is blocked). So a per-namespace "default deny" posture is the existing **enforcement** knob (`enforcement: full` / the namespace annotation / the ClusterProfile baseline — see policy-concepts.md §5), NOT a new rule. A roadmap `defaultDeny` field, if added, should just be sugar that sets the namespace's effective enforcement to `full`. An explicit catch-all Override-Deny rule is the hard-block alternative but is drastic.

## Open questions to resolve before implementing

1. **Override-deny representation.** Is "override deny" a **field on a deny rule** (e.g. `override: true`) or a distinct sub-type? The 24.5/25.4 docs expose `deny_rules` + `override_deny` in Rule Search but don't show the create-time field. **Confirm** by either (a) reading the 25.x `deny_rules` POST schema, or (b) creating an override-deny rule in the PCE UI and `GET`ing it to see the JSON. **This is the one byte the spike couldn't pin down.**
2. **PCE version gating.** Deny Rules need Core **25.x**. Confirm the target US2 PCE supports `GET {ruleset}/deny_rules` (try it, or check the PCE version). If it's on 24.x, either require an upgrade or fall back to Enforcement Boundaries (extra work, deprecated — avoid).
3. **CRD surface.** Proposed: `SegmentationIntent.spec.deny []IntentDeny` (same shape as `allow`: `from`/`fromIntraNamespace` + ports) → compiles to **Override Deny** rules (the strong form the user asked for). Optionally a separate softer `deny` vs `overrideDeny`. `SegmentationPolicy` (NetworkPolicy-shaped) has no deny concept, so deny likely stays Illumio-native (Intent-only) — confirm.

## Recommended next step

Implement **Override Deny rules via `{ruleset}/deny_rules`** once (1) and (2) are confirmed:
1. Spike-verify the override field against the live US2 PCE (create one in the UI, GET it).
2. Confirm the PCE is on Core 25.x.
Then: `pce` deny-rule CRUD → `spec.deny` on SegmentationIntent → compile to override-deny → reconcile (replace-all) → tests + docs.

## Sources
- [Illumio — Rules (Allow / Deny / Override Deny precedence)](https://product-docs-repo.illumio.com/Tech-Docs/Core/25.2/Security-Policy/out/en/security-policy-guide-25-2-10/create-security-policy/rules.html)
- [Illumio — Rules REST API (`deny_rules` endpoint), Core 25.4](https://product-docs-repo.illumio.com/Tech-Docs/Core/25.4/REST-APIs/out/en/rest-apis-25-4/policy/rules.html)
- [Illumio — Enforcement Boundaries REST API (legacy deny mechanism)](https://product-docs-repo.illumio.com/Tech-Docs/Core/24.2/REST-APIs/out/en/rest-apis/rulesets-and-rules/enforcement-boundaries.html)
