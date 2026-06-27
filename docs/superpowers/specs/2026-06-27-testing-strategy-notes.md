# Testing & integration-testing strategy — notes (WIP)

**Status:** Brainstorm paused. Decisions captured below; **one open decision** (the hermetic
fidelity layer) remains before this becomes a full spec + implementation plan.
**Date:** 2026-06-27

---

## Where we are today (current test landscape)

| Layer | What it covers | Hermetic? | Fidelity risk |
|---|---|---|---|
| **Unit — `internal/pce`** | REST client wire behavior, tested against an `httptest` mock PCE | yes | the mock encodes our *assumptions* about the PCE |
| **Pure unit — `internal/controller`** | `cwpmatch`, `policyir`, `policycompile`, `enforcement` helpers | yes | none (pure functions) |
| **envtest — `internal/controller`** | real kube-apiserver + the actual reconcilers, with **fake PCE clients** (`fakeOnboardingClient`, `fakePolicyClient`) | yes | fakes can drift from real PCE behavior |
| **e2e (Kind) — `test/e2e`** | kubebuilder scaffold: deploys the operator, checks the manager pod runs | yes | no PCE, no real flow |

## The gap (the hard part)

The fakes/mocks **encode our assumptions about the PCE**, so they cannot catch the exact bug class
we hit repeatedly while building: wrong API shape, draft→active/provisioning semantics, `429`
handling, label-href resolution, ownership-scoped provisioning. They also don't exercise the
**Kubelink→CWP handoff** (the operator configures CWPs Kubelink creates) or the **Helm credential
handoff**. True integration testing must validate the operator against *real or faithfully-simulated*
PCE behavior, and ultimately a real cluster with the agents running.

---

## Decisions captured

### Environment (decided)
- A **real test cluster + Illumio agents (C-VEN/Kubelink) + PCE** is available for the highest-fidelity
  end-to-end tests — we can validate the *whole* chain, including the Kubelink→CWP handoff and actual
  enforcement, not just the PCE API.
- **But:** the cluster is **manually set up**, **likely EKS**, and a **nightly run cannot be
  guaranteed**.

### Consequence (decided)
- The full **real-cluster + agents + PCE** suite is **on-demand / manual**, not a dependable scheduled
  CI job. It must be:
  - **self-contained and env-gated** (skips cleanly unless `RUN_REAL_E2E=1` + PCE/cluster creds/env are
    present),
  - **documented prerequisites** (EKS exists; Kubelink/C-VEN installed; PCE reachable; API key/org;
    kubeconfig) — since the cluster is manual,
  - triggered by a `make` target and/or GitHub Actions **`workflow_dispatch`**,
  - **never blocking PRs**, isolation-safe (unique object names + reliable teardown so re-runs on a
    persistent lab don't collide).
- Because real-env runs aren't guaranteed, **everyday PR confidence stays hermetic** — which forces a
  hermetic answer to the "fakes drift from the real PCE" risk (the open decision below).

---

## OPEN DECISION — the hermetic PCE-fidelity layer

How do we get PCE **behavioral** fidelity in always-on (hermetic) CI without a guaranteed live PCE?

- **A — Stateful PCE simulator (RECOMMENDED).** Replace the one-off fakes with a small in-memory PCE
  HTTP server that models the *semantics* we depend on: draft objects, `provision` moving draft→active
  and returning `workloads_affected`, label find/create + href assignment, ownership-scoped
  provisioning, `429`s. The integration suite runs the real `pce.Client`/operator against it over HTTP.
  *Pros:* deterministic, fast, no secrets, exercises real JSON + state transitions; catches the
  draft/provision/ownership/429 bug class the dumb fakes miss. *Cons:* the simulator is *our model* of
  the PCE, so it must be **validated periodically by contract tests against a real PCE** (manual, when
  the lab is up).
- **B — Recorded cassettes (go-vcr).** Capture real PCE responses once; replay in CI. *Pros:* real
  response shapes. *Cons:* brittle (any request-shape change forces a re-record), can't model state
  transitions like provisioning, needs a real PCE to (re)record — high maintenance.
- **C — Keep the simple fakes; lean entirely on manual real-env runs.** Cheapest now, but leaves
  everyday CI blind to PCE-contract correctness — exactly the gap we're worried about.

**Recommendation:** **A** (simulator) as the hermetic fidelity layer, **+ manual contract tests** to
keep the simulator honest against the real PCE, **+ the on-demand EKS e2e** for the full chain.

---

## Proposed test pyramid (pending the open decision)

1. **Unit + pure** (have) — every PR.
2. **envtest with the PCE simulator** (upgrade from one-off fakes, if A) — every PR; exercises real
   reconcile + real client JSON against realistic PCE state.
3. **Operator-boots Kind e2e** (have, thin) — every PR; could grow to drive a CR against the simulator.
4. **Real-PCE contract tests** (new) — **manual / env-gated**: assert the simulator's assumptions match
   the live PCE (and pin exact JSON shapes). Not in PR CI.
5. **Full real-cluster + agents + PCE e2e** (new) — **manual / `workflow_dispatch`** on the EKS lab:
   apply CRs → assert PCE ends up with the right objects **and** the agents enforce → clean up.

---

## Next steps when resuming

1. Decide the hermetic fidelity layer: **A (simulator)** / B (cassettes) / C (keep fakes). _(Leaning A.)_
2. Finish the brainstorm → write the full spec → `writing-plans` → implement test-first (TDD).
3. Scope the first increment (likely: build the PCE simulator + re-point the controller integration
   suite at it; then the env-gated real-e2e skeleton with prerequisite docs).

> Housekeeping note (unrelated): a stale `.claude/worktrees/agent-*` copy of the repo is lingering in
> the tree — worth pruning so it stops showing up in `find`/test sweeps.
