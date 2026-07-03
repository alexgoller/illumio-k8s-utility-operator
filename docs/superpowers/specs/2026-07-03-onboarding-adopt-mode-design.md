# Onboarding: create vs adopt (two paths) — design

**Goal:** support two onboarding paths so the operator works whether or not the cluster is already
paired to the PCE.

- **Path 1 — create (default, unchanged):** cluster not yet onboarded → operator creates the PCE
  Container Cluster, the node Pairing Profile, generates the pairing key, and writes the credentials
  Secret. For greenfield clusters.
- **Path 2 — adopt (new):** cluster **already onboarded** (by the Illumio helm chart or another
  process) → operator finds the existing Container Cluster **by name**, records its href/ID, marks
  `Onboarded=True`, and **skips pairing, the key, and the credentials Secret**. Everything downstream
  (CWP labeling, SegmentationIntent/Policy, PolicyInsight preflight) then works unchanged, since it
  keys off `Onboarded` + `ContainerClusterHref`.

Today path 2 is a **hard failure**: if the named cluster already exists, onboarding sets
`OnboardFailed` and stops (`the one-time token cannot be recovered`). Adopt is exactly the case where
that doesn't matter — the C-VEN is already paired, so we never needed the token.

## Decisions (agreed)

- **Explicit `onboarding.mode: create | adopt`** — the user picks the path; no silent adoption.
  Defaults to `create` (preserves current behavior).
- **Adopt matches by name** (`onboarding.containerClusterName`). Adopt-by-href/ID is a possible later
  addition, out of scope here.
- **Standalone** feature (own spec → implement → release).

## API changes (`OnboardingSpec`)

- Add `Mode string` — `+kubebuilder:validation:Enum=create;adopt`, `+kubebuilder:default=create`,
  optional.
- Make `CredentialsOutputSecret` **optional** (it is meaningless in adopt mode). The controller still
  **requires it in create mode** (fail with a clear message if empty), preserving today's contract.
- `nodePairingProfile` is already optional and is ignored in adopt mode.

## Controller changes (`clusterprofile_controller.go`)

Add `mode := onboardMode(&cp)` (returns `cp.Spec.Onboarding.Mode` or `create`). In the
container-cluster block (`if cp.Status.ContainerClusterHref == ""`):

- **adopt:** `FindContainerClusterByName`. If not found → `onboardFail`
  (`"onboarding.mode is adopt but container cluster %q was not found; create it first or use mode create"`,
  requeue healthy — it may appear later). If found → record `ContainerClusterHref`/`ID`, persist
  status. **Skip** the pairing-profile / pairing-key / credentials-Secret block.
- **create:** unchanged, except the "already exists" message now points to adopt:
  `"…already exists; set onboarding.mode: adopt to manage it in place, or delete it."` Also require
  `CredentialsOutputSecret`.

Gate the pairing-profile + key + `writeCredentialsSecret` steps behind `mode == create`. The shared
tail (CWP reconcile, `Onboarded=True`, LabelMap check, status) runs for both; the `Onboarded`
condition message reflects the mode (`"cluster onboarded; credentials published"` vs
`"existing container cluster adopted"`).

## Testing (envtest + unit)

- **Unit:** `onboardMode` default/explicit.
- **Envtest:**
  - adopt + existing cluster → `Onboarded=True`, href recorded, **no credentials Secret written**,
    CWPs reconcile.
  - adopt + missing cluster → `Onboarded=False`, reason `OnboardFailed`, adopt message.
  - create + existing cluster → `OnboardFailed` with the "set mode: adopt" hint.
  - (existing create happy-path test stays green.)
- The `fakeOnboardingClient.FindContainerClusterByName` returns an existing cluster for a magic name
  prefix (e.g. `existing-*`) so tests can drive both branches.

## Docs / chart

- **Onboarding guide:** lead with the two paths; adopt example.
- **ClusterProfile reference:** document `onboarding.mode`; note `credentialsOutputSecret` optional in
  adopt.
- **Chart:** render `onboarding.mode` from values (`onboarding.mode: create` default).

## Non-goals

Adopt-by-href/ID; migrating a create-mode profile to adopt; reconciling drift between an
externally-managed pairing profile and ours.
