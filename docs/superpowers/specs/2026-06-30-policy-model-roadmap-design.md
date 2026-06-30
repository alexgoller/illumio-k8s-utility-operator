# Policy Model Roadmap & Design

> **Status: living design doc.** Track 1 is decided and implementation-ready. Tracks 2–4
> are in-design / roadmap and need more brainstorming before a plan. Captured 2026-06-30.

## Why this exists

The operator's policy front-ends (`SegmentationIntent` Illumio-native, `SegmentationPolicy`
k8s-NetworkPolicy-shaped) compile to Illumio rulesets. This doc captures the tracks that close
the model's gaps, the decisions, and the open questions, so the work can be done deliberately
and made bulletproof.

**Tracks:** 1 — configurable unknown-label handling · 2 — deny semantics · 3 — selective
enforcement · 4 — intra-namespace (service-to-service) · 5 — scope-correct rule emission
(foundational) · 6 — egress.

## Current model (baseline)

- **Allow-only.** Each `allow`/`ingress` entry → one Illumio allow-rule. No deny.
- **Provider = the whole namespace's `app` label.** Derived from the namespace's CWP. There
  is no sub-namespace (per-service) granularity.
- **Consumer labels must already exist** in the PCE or the **whole intent is `Rejected`**
  (`Ready=False`, reason `Rejected`). The operator never creates labels — only resolves.
- **Enforcement modes:** `idle | visibility_only | full`, strictest-wins across the admin
  baseline + every policy CR in the namespace. No `selective`.
- **Labeling is namespace-level only:** `ClusterProfile` `assignLabels` / `systemNamespaces`
  set labels on the **CWP** (every workload in the namespace gets the same set). No
  per-workload labeling.
- **Rule emission is scope-naive.** `BuildRules`/`BuildRuleSet` scope the ruleset to the
  namespace's app label (good), but then **repeat the scope** in `providers` (emit the app hrefs
  again instead of `"ams"`/a partial selector) and set **`unscoped_consumers: true` on every
  rule** (everything is extra-scope). See Track 5.

## Illumio scope model (constraint all tracks must respect)

Captured in full in the project memory `illumio-ruleset-scope-model.md`. Essentials:

- A ruleset's **scope = App+Env+Loc**; the **provider is always bound by the scope** ⇒ rulesets
  are **ingress-centric** (traffic *into* the scoped workloads).
- **Intra-scope** rule: scope binds **both** providers and consumers (same-app, e.g. frontend→backend).
- **Extra-scope** rule (`unscoped_consumers: true`): scope binds **only providers**; consumers can
  be anywhere.
- **Don't repeat the scope** in actors: use a partial selector (`role`) or **"All Workloads"**
  (API `"ams"`). Meaning is contextual — provider/intra-scope-consumer → *all in scope*;
  extra-scope consumer → *all workloads org-wide* (⚠️ every VEN).
- **No separate egress rule**: outbound is the mirror (your workload as a *consumer* in the
  destination's rule). Since the provider is always in-scope, a namespace-scoped ruleset can
  natively express only **ingress** — see Track 6.

---

## Track 1 — Configurable unknown-label handling  ✅ decided

A referenced Illumio label not existing **is not** an error in itself. The PCE rule model
references label *objects* (hrefs); a label can exist with zero workloads, and the PCE simply
computes no per-workload policy until a matching workload appears. So "author the intent now,
policy lights up when the workload exists" is valid. Today's hard reject is a deliberate
*guardrail* (stop typos minting junk labels), not a correctness statement — so we make it
**configurable**.

This is a **general label-resolution policy**: it applies to *any* provider/consumer label
reference, including intra-namespace (Track 4), not just cross-app inbound.

**Knob:** `unknownLabelMode`, tri-state:

| Mode | On an unknown referenced label |
|---|---|
| `strict` (default) | Reject the whole CR — `Ready=False`, reason `Rejected`. Current behavior; fully backward-compatible. |
| `skip` | Compile rules for known labels, **omit** the unknown actor/rule; `Ready=True`; list deferred labels in status; re-resolve each reconcile so the rule appears once the label exists. **Never creates labels.** |
| `create` | `EnsureLabel` the missing label (mint it in the PCE), then write the rule. PCE computes policy when a matching workload appears. |

**Scope & precedence — most-specific wins:**
1. `ClusterProfile.spec.unknownLabelMode` — fleet default (next to `provisioningMode`). Default `strict`.
2. Namespace annotation `microsegment.io/unknown-label-mode: <mode>` — per-namespace override.
3. CR annotation `microsegment.io/unknown-label-mode: <mode>` — most specific.

Resolution order: **CR → namespace → ClusterProfile default.**

**Status surface (both policy CRDs):**
- `status.unknownLabelMode` + `unknownLabelModeSetBy` (mirrors `effectiveEnforcement` /
  `enforcementSetBy`).
- `status.deferredLabels` (skip) / `status.createdLabels` (create); a `LabelsResolved`
  condition message summarizes.

**`create`-mode safety:** gate auto-create to **known Illumio label keys** (`role/app/env/loc`
+ any key already present in the PCE), so a typo'd *key* still errors instead of spawning a
junk label dimension. Opt-in + per-namespace scoping is the primary guardrail; the key
allowlist is belt-and-suspenders (on by default).

---

## Track 4 — Intra-namespace (service-to-service) rules  🚧 in design

**Problem.** "How do my services in my namespace connect?" can't be expressed today: the
provider is implicitly "the namespace's `app`," and every pod shares that one CWP-assigned
`app` label. There's no sub-namespace identity to target, so `frontend → backend` *within*
one namespace is inexpressible.

**The operator stays namespace-level — it does NOT do per-workload labeling.** The operator's
lane is the **CWP**: it assigns namespace-wide labels (`app`/`env`, or whatever `assignLabels`
covers) to every workload in the namespace. It **cannot** set different labels for different
deployments in one namespace today, and taking that over is explicitly **not** the goal.

**Per-workload differentiation comes from outside the operator.** `role` (and any dimension
that must differ between services in the same namespace) is supplied by either:
- the Illumio **`LabelMap`** (`workloadLabelMap`) — maps per-**workload** k8s labels → Illumio
  labels (Illumio Core for Kubernetes 5.3.0+, CLAS, PCE 24.5.0+); or
- **Illumio pod annotations** — per-pod annotations the C-VEN/Kubelink reads to assign labels.

The operator neither creates nor manages these; it only **references** the resulting labels in
rules. Track 1 covers the "the `role` label isn't there yet" case uniformly.

**The operator's only responsibility toward the LabelMap is safety/coordination (small, must
be bulletproof):**
- **Detect** whether a `LabelMap` exists in the cluster.
- If it does, **emit a warning** so the two systems are visible to the operator/admin.
- **Don't double-write the same dimension — warn only.** The operator must not be the one
  enforcing here: if the `LabelMap` writes a key the operator also assigns via CWP, the operator
  **just warns**. It does not skip, strip, or otherwise change its own labeling, and it cannot
  enforce anything on the LabelMap. Advisory surface only.

**CRD impact.** Intra-namespace rules need **sub-namespace provider/consumer selection** — the
provider can no longer be only the implicit "namespace's app." A policy scopes provider and
consumer to an **arbitrary Illumio label set** *within* the namespace's `app`. `role` is the
Illumio convention (the default assumption for an in-app tier), but the selector must accept
**any label key(s)** — including custom keys that a customer uses instead of `role`. Shape is
the same general label map already used by the consumer `from`; provider defaults to the
namespace's `app` when no selector is given.

**Compiles to intra-scope rules (Track 5).** Same-namespace service-to-service rules are
**intra-scope** (`unscoped_consumers: false`), with the `role`/partial selector narrowing within
the scope and the scope *not* repeated. This depends on Track 5's scope-correct emission.

**Must be expressible in BOTH front-ends.** Intra-scope / same-namespace selection has to land in
**both** `SegmentationIntent` (Illumio-native) **and** the NetworkPolicy-shaped `SegmentationPolicy`
— a self-referential `podSelector`/`from` that resolves to the namespace's own `role`/sub-labels.
The shared backend already compiles both front-ends, so the intra-scope *emission* is one place;
the new requirement is parallel **CRD surface** (the sub-namespace provider/consumer selector) on
each front-end.

**Capability note.** `workloadLabelMap` needs CLAS + Core-for-K8s 5.3.0+ + PCE 24.5.0+. The
operator doesn't depend on it, but its overlap-warning should account for whether the LabelMap
mechanism is even present.

---

## Track 2 — Deny semantics  📐 CRD spec designed (tracked in #22; now being implemented)

We emit **allow** rules only; the `internal/pce` rule model has no deny/boundary concept yet.
Two needs, two pieces of CRD surface:

**Override deny** — a deny that wins over allow rules. Proposed CRD shape: a `deny` list on
`SegmentationIntent`, parallel to `allow`, same actor shape (`from` Illumio labels + `ports`),
compiled into Illumio deny/boundary rules that take precedence over the allow rules in the same
namespace scope.

```yaml
spec:
  allow:
    - from: { app: checkout, env: prod }
      ports: [{ port: 8443, protocol: TCP }]
  deny:                                    # reserved — wins over allow
    - from: { app: untrusted }
      ports: [{ port: 8443, protocol: TCP }]
```

**Bottom / default deny** — explicit default-deny posture for a namespace. Proposed CRD shape:
a `defaultDeny` boolean on the `ClusterProfile` namespace rule / `systemNamespaces` (and/or a
per-namespace annotation `microsegment.io/default-deny`). Note: `full` enforcement is already
default-deny in Illumio, so this field's exact relationship to enforcement mode is an open
semantic to settle during implementation.

`SegmentationPolicy` (k8s-NetworkPolicy-shaped) likely does **not** grow a `deny` field —
NetworkPolicy has no deny concept; deny stays on the Illumio-native `SegmentationIntent`.

Needs new `internal/pce` support for deny/boundary rules. **Design lives in the CRD spec now;
controller behaviour is deferred.**

## Track 3 — Selective enforcement  📐 CRD spec designed, ⛔ deferred (tracked in #22)

Add `selective` as an enforcement value (enforce specific services, rest stays visibility — a
middle ground between `visibility_only` and `full`). CRD-spec design:

- Add `selective` to the enforcement enums on `SegmentationIntent.spec.enforcement`,
  `SegmentationPolicy.spec.enforcement`, `ClusterProfile` `NamespaceRule.enforcementMode` /
  `SystemNamespacesSpec.enforcementMode`, and the `microsegment.io/enforcement` annotation.
- Strictness order becomes `idle < visibility_only < selective < full` in `enforcementRank`.

**Blocking unknown before implementation:** confirm the PCE **CWP** API accepts `selective`
(today's code/docs say it is unsupported for container workload profiles). If CWPs reject it,
the value stays a documented-but-guarded enum until/unless the PCE supports it. **Enum design
in the CRD spec now; controller behaviour deferred.**

## Track 5 — Scope-correct rule emission  🚧 foundational refactor (underpins 4 & 6)

Today's emission is scope-naive (see baseline). Align it with the Illumio scope model:

- **Always-scoped invariant.** A per-namespace policy is *always* a ruleset scoped to the
  namespace's labels (App+Env+Loc). Make this explicit and non-optional in `BuildRuleSet`.
- **Don't repeat the scope** in `providers`. Default to **"All Workloads in scope"** — REST actor
  `"ams"` — instead of re-emitting the namespace's app label hrefs; or a **partial `role` selector**
  when the rule targets specific services (Track 4).
- **Choose intra- vs extra-scope per consumer.** Same-namespace consumers (Track 4) →
  **intra-scope** (`unscoped_consumers: false`, `role`/partial selector, scope not repeated).
  Consumers from *other* apps → **extra-scope** (`unscoped_consumers: true`). Today every rule is
  forced extra-scope.

Touches `internal/controller/policyir.go` (`BuildRules`/`BuildRuleSet`) and `internal/pce`
(needs an **`"ams"` / All-Workloads actor** representation — `pce.Actor` only models `{label}`
today). This refactor is a prerequisite for honest intra-namespace (Track 4) and egress (Track 6).

## Track 6 — Egress  🚧 design (new; gated by the scope model)

**Need:** control **outbound** traffic from a namespace's app ("my app may reach X").

**Constraint:** Illumio has **no separate egress rule**; the provider is always in-scope, so a
ruleset scoped to *my* namespace expresses only **ingress to me**. Outbound is the mirror — my app
appears as a **consumer** in the *destination's* rule. So per-namespace egress can't live in the
namespace's own scoped ingress ruleset.

**Design options (to settle):**
1. **Extra-scope consumer in the destination's policy** — faithful to Illumio, but means writing
   into another scope/app's ruleset (cross-namespace ownership — conflicts with "a CR owns only its
   own ruleset").
2. **A separate egress-oriented ruleset** owned by the CR, scoped to the destination (or unscoped),
   where my app is the consumer. Keeps ownership clean; needs careful scoping to avoid `"ams"`
   extra-scope blast radius.
3. **Deny boundaries / newer egress constructs** (overlaps Track 2) for default-deny-egress postures.

**CRD shape (proposed):** an `egress`/`to` list on `SegmentationIntent` (destination Illumio labels
+ ports), mirroring `allow`. **Open** which Illumio construct backs it. Likely its own issue once
the option is chosen.

---

## Cross-cutting decisions & remaining questions

**Decided:**
- **Overlap = warn only.** The operator detects a `LabelMap` writing a key it also assigns via
  CWP and warns. It never skips, strips, or enforces; it only controls its own CWP keys.
- **Intra-namespace selection = arbitrary Illumio label keys.** `role` is the default
  convention/assumption but not hardcoded; custom keys are first-class. Provider defaults to the
  namespace's `app` when no selector is given. Same label-map shape as consumer `from`.
- **Track 1 × Track 4.** Externally-supplied `role`/custom labels are exactly the "might not
  exist yet" references Track 1's `skip`/`create` covers — one uniform resolution policy.

**Remaining (minor, impl-time):**
1. **Warning surface** — Event on the namespace, a condition on `ClusterProfile`, and/or a log
   line (likely all three; impl detail, not a model decision).
2. **Capability detection** — whether the `LabelMap`/CLAS path is present, to tune the warning.
3. **Exact CRD field name/shape** for the optional provider label selector.

## Sources

- [Illumio — Map Kubernetes Node or Workload Labels to Illumio Labels (`LabelMap`/`workloadLabelMap`)](https://product-docs-repo.illumio.com/Tech-Docs/Containers/out/en/kubernetes-and-openshift/configure-labels-for-namespaces,-pods,-and-services/map-kubernetes-node-or-workload-labels-to-illumio-labels.html)
- [Illumio Core for Kubernetes 5.3.0 — What's New (workload label mapping; CLAS; PCE 24.5.0)](https://product-docs-repo.illumio.com/Tech-Docs/Containers/out/en/illumio-core-for-kubernetes-5-3-release-notes/what-s-new-in-illumio-core-for-kubernetes-5-3-0.html)
- [Illumio — RBAC / least-privilege (for the related PCE-key track)](https://product-docs-repo.illumio.com/Tech-Docs/Core/24.2/Admin/out/en/pce-administration/access-configuration-for-pce/role-based-access-control.html)
