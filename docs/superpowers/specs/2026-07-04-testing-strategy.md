# Testing strategy

Where the operator's testing stands and where it's going. Tiers 1–2 are implemented; 3–4 are the roadmap.

## Layers today

- **Unit tests** — pure logic (policy compilers, classifiers, scope/label-mode helpers, RuleView/preflight mappers). Fast, broad.
- **envtest (Ginkgo)** — controllers run against a real API server with **fake PCE clients** injected at the client-factory boundary. Validates reconcile logic; bypasses HTTP to the PCE.
- **PCE HTTP round-trip tests** *(Tier 1 — added)* — the real `pce.Client` driven against an in-process **mock PCE** (`internal/pce/pcetest`), asserting **both** the request body we send and the response parsing.
- **e2e (Kind)** — deploys the operator into a Kind cluster; smoke-level.

## Tier 1 — PCE HTTP round-trip tests (done)

The gap Codex flagged (L10): the PCE client's request/response shapes were largely untested, and `traffic.go` / `rulesearch.go` (the two integrations never run against a live PCE) had **no tests at all**.

Added:
- **`internal/pce/traffic_test.go`** — the async Explorer flow (POST → poll → download) end-to-end, decision/label/timestamp parsing, and the request-body contract (scope label href under `destinations.include`, `max_results`). `QueryTraffic` ~82% covered.
- **`internal/pce/rulesearch_test.go`** — provider-filter request body, response parsing incl. `override_deny` → `override_deny`, ruleset `external_data_set`, and the empty-version → `active` default. `SearchRules` ~95% covered.
- **`internal/pce/policy_httptest_test.go`** — **golden request-body** guards that would have caught the historical **406s** at unit time: `ingress_services` must be a non-null array and "All Services" sent by **href** (never inline `{proto:-1}`); `ProvisionRuleSets` `change_subset`; `FindRuleSetByOwner` matching by `external_data_reference` + `update_type` parsing; `FindServiceByName`.

## Tier 2 — the mock PCE server (done)

**`internal/pce/pcetest`** — a reusable, routing, in-process mock PCE. Register canned responses per endpoint (`JSON`/`Route`, `*` prefix match), build a wired client (`Client(orgID)`), and assert recorded request bodies (`LastBody`/`Count`). It's a normal package (not `_test`), so **controller tests can use it too** — driving the *real* `pce.Client` through the reconcilers against a mock PCE, shifting from "fake the client" toward "mock the PCE."

## Tier 3 — live-PCE integration tests (next)

Opt-in tests (build tag `//go:build livepce` or env-gated `PCE_URL`/`PCE_KEY`/…) that run the real client against an **actual PCE**. The only thing that truly confirms the traffic/rulesearch shapes and folds in the deferred review items (H4 `external_data_reference` ownership, M6 async terminal states, M7 timestamp formats). Runs manually or in a gated CI job with secrets. **Needs:** a decision on how creds reach CI.

## Tier 4 — hygiene (next)

- **Coverage floor** in CI (`-coverprofile` is already produced; add a threshold gate).
- **Native Go fuzzing** for the renderers/classifiers (`renderActors`/`renderServices`, `classifyFlows`, label rendering).
- **Expand e2e** to run the operator against the `pcetest` mock in Kind for a real deploy-time smoke of the PCE flows.

## Conventions

- HTTP-boundary tests live beside the client in `internal/pce` as `package pce_test` (external) so they can import `pcetest` without an import cycle.
- Prefer **golden request-body assertions** for anything the operator sends to the PCE — the PCE rejects malformed bodies with opaque 406s that are invisible to controller-level fakes.
