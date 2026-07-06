# Illumio K8s Utility Operator

A Kubernetes operator that automates the **PCE-side** of running Illumio Core on Kubernetes/OpenShift — and lets your teams **live in `kubectl`** instead of the Illumio console.

## What it does

- **Onboard or adopt** — register a cluster with the PCE, or **adopt** one that's already paired (e.g. by the Illumio Helm chart).
- **Container Workload Profiles** — manage which namespaces are governed and how they're labeled, from declarative rules.
- **Segmentation policy** — app teams express Illumio policy as Kubernetes resources (`SegmentationIntent` / `SegmentationPolicy`), compiled to PCE rulesets.
- **Traffic visibility (`PolicyInsight`)** — a **what-if preflight**: which observed flows a policy would block, from the PCE's draft decision — *before* you enforce.
- **Rule visibility (`RuleView`)** — the **current Illumio rules** protecting your app, in `kubectl`, **including rules authored outside Kubernetes**.

The operator is a client of the Illumio PCE REST API. It does **not** deploy the C-VEN/Kubelink agents — that stays with the official Helm chart.

## The typical workflow

```
  1. Connect            2. Onboard / Adopt      3. Label
  PCEConnection   ──▶   ClusterProfile     ──▶  namespaceRules → CWP labels
  (PCE creds)           (create or adopt)       (app / env per namespace)
        │                                              │
        ▼                                              ▼
  4. Author policy       5. Preflight (traffic)    6. See current rules    7. Enforce
  SegmentationIntent  ─▶ PolicyInsight         ─▶  RuleView            ─▶  enforcement: full
  / SegmentationPolicy   "what would break?"       "what protects me?"     (strictest-wins)
```

1. **Connect** a `PCEConnection` with your PCE URL + credentials.
2. **Onboard** a fresh cluster or **adopt** an already-paired one with a `ClusterProfile`.
3. **Label** namespaces (Container Workload Profiles) via `namespaceRules` — new namespaces are picked up automatically.
4. **Author** allow-list policy as `SegmentationIntent` (or NetworkPolicy-shaped `SegmentationPolicy`).
5. **Preflight** the impact with `PolicyInsight` — see the flows a policy *would* block, from observed traffic, before enforcing.
6. **See the current rules** with `RuleView` — everything governing your app, including rules made in the Illumio UI.
7. **Enforce** by moving the namespace to `full` — confident nothing legitimate breaks.

Steps 5–6 are **read-only** and let you verify from `kubectl` without opening the Illumio UI.

## Start here

- **[Getting Started](getting-started.md)** — connect → onboard → label → policy → visibility, end to end.
- **[Concepts](concepts.md)** — the Illumio + Kubernetes model.
- **Guides** — [Policy preflight](guides/preflight.md), [Live rule view](guides/ruleview.md), [Onboarding](guides/onboarding.md), [Namespace management](guides/namespace-management.md), [Security & credentials](guides/security.md).
