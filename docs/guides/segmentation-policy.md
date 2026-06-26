# Segmentation Policy

This guide explains how app teams use `SegmentationIntent` to declare which consumers may reach their application, and how the operator compiles and provisions those declarations into Illumio rulesets.

## Mental model

A `SegmentationIntent` is an **allow-list for inbound traffic to your namespace's app**. You declare: "allow these consumers (identified by their Illumio labels) to reach my pods on these ports." The operator compiles each intent into one Illumio ruleset that is scoped to your namespace and owned by that CR — nobody else can modify it.

Key point: you control access **into your own app only**. You cannot write rules that protect another team's namespace.

## Guardrails

Before writing any intent, understand the two hard constraints:

**The provider is always your namespace's own app.** The operator derives the provider from the namespace's Illumio `app` label (set by the `ClusterProfile` namespace rules). You cannot name another namespace's app as the provider; attempting to do so results in `Ready=False`.

**Consumer labels must already exist in the PCE.** The `from` map references Illumio labels that Kubelink creates from real running workloads. If any label key/value combination in `from` does not exist in the PCE, the entire intent is `Rejected` (status `Ready=False`, reason `Rejected`). The operator never creates labels; it only resolves them.

**The namespace must be managed.** The namespace must be enrolled by a `ClusterProfile` namespace rule (Plan 3 / [Namespace management](namespace-management.md)). If no `ClusterProfile` covers the namespace, the intent is rejected with reason `ClusterProfileNotReady`.

## Enforcement is separate

Writing a `SegmentationIntent` compiles rules and can provision them to the PCE, but **non-allowed traffic is only blocked when the namespace's Container Workload Profile (CWP) is in `full` enforcement mode**. In `visibility_only` mode the rules are computed and provisioned but traffic flows freely — nothing is blocked.

Enforcement mode is controlled by the `ClusterProfile`'s namespace rules or per-namespace `microsegment.io/enforcement` annotation, not by the `SegmentationIntent` itself. See the [Namespace management guide](namespace-management.md) for details.

!!! warning "Rules without `full` enforcement do not block traffic"
    If your namespace is in `visibility_only` mode, provisioning a `SegmentationIntent` has no enforcement effect. Switch the namespace to `full` enforcement via the `ClusterProfile` or the namespace annotation when you are ready to enforce.

## Example

The following intent for the `payments` namespace allows:

- The `checkout` app in `prod` to reach port 8443/TCP.
- The `ledger` app in `prod` to reach port 5432/TCP.

```yaml
apiVersion: microsegment.io/v1alpha1
kind: SegmentationIntent
metadata:
  name: payments-ingress
  namespace: payments
spec:
  allow:
    - from: { app: checkout, env: prod }
      ports:
        - { port: 8443, protocol: TCP }
    - from: { app: ledger, env: prod }
      ports:
        - { port: 5432, protocol: TCP }
```

To allow a consumer on **all ports**, omit the `ports` field:

```yaml
spec:
  allow:
    - from: { app: monitoring, env: prod }
      # no ports — allows all ports
```

## How compilation works

When a `SegmentationIntent` is applied, the operator:

1. Looks up the namespace's Illumio `app` label from its CWP — this becomes the **provider** scope of the ruleset.
2. For each `allow` entry, resolves the `from` labels to Illumio label hrefs. If any label is unknown in the PCE, the intent is **Rejected** and no ruleset is written.
3. Builds one Illumio ruleset named after the CR (e.g. `payments/payments-ingress`), containing one allow-rule per `allow` entry with the resolved consumer actors and inline ingress services (port + protocol pairs).
4. Tags the ruleset with an ownership annotation so the operator can identify and replace it on future reconciles. Only that one ruleset is touched; other rulesets in the scope are not modified.

The ruleset is **replaced in full** on every reconcile — rule-level diffing is not performed in this version.

## Provisioning modes

Provisioning is controlled by `ClusterProfile.spec.provisioningMode`. There are three modes:

### `auto`

The operator provisions the ruleset immediately after compilation. The `Provisioned` condition becomes `True` as soon as the PCE accepts the provisioning request.

### `manual`

The operator writes the ruleset as a **draft** and waits. A human (app team member or admin) must explicitly approve provisioning by annotating the CR:

```bash
kubectl annotate segmentationintent payments-ingress microsegment.io/provision=approved -n payments
```

While the annotation is present, the operator keeps the intent's policy provisioned — re-provisioning whenever the spec changes. The `Provisioned` condition transitions from `ProvisionPending` to `Provisioned`. To stop further provisioning of new changes, remove the annotation:

```bash
kubectl annotate segmentationintent payments-ingress microsegment.io/provision- -n payments
```

!!! note "Per-change approval"
    Per-change approval (re-approving each individual edit) is planned for a future release. Today the annotation is sticky: once set, the operator continues provisioning on every spec change until the annotation is removed.

### `draft-only`

The operator writes a draft but **never provisions it**. A human provisions the ruleset directly in the PCE UI or via the PCE API. The `Provisioned` condition stays `False` with reason `ProvisionPending` indefinitely. Use this mode when the PCE has a strict change-management process that requires out-of-band approval.

## Reading status

After applying an intent, check its conditions with:

```bash
kubectl get segmentationintents -n payments
```

The columns show:

```
NAME               READY   PROVISIONED   AFFECTED
payments-ingress   True    True          12
```

- `READY` — `True` when the intent compiled successfully; `False` when rejected (unknown labels, missing ClusterProfile, etc.).
- `PROVISIONED` — `True` when the ruleset has been provisioned to the PCE; `False` when pending.
- `AFFECTED` — the count of workloads affected by the last provisioning operation (`status.workloadsAffected`).

For full condition details:

```bash
kubectl describe segmentationintent payments-ingress -n payments
```

Look for the `Conditions` block:

```
Conditions:
  Type         Status  Reason           Message
  ----         ------  ------           -------
  Ready        True    Compiled         Ruleset compiled for 2 allow entries
  Provisioned  True    Provisioned      Provisioned; workloads affected: 12
```

If the intent is rejected:

```
Conditions:
  Type   Status  Reason    Message
  ----   ------  ------    -------
  Ready  False   Rejected  label not found in PCE: app=inventory (env=prod)
```

## Deleting an intent

Deleting a `SegmentationIntent` CR removes its owned Illumio ruleset and provisions the removal. A finalizer on the CR ensures the cleanup happens before the object is removed from Kubernetes. Once the PCE confirms the deletion, the finalizer is released and the CR disappears.

```bash
kubectl delete segmentationintent payments-ingress -n payments
```

!!! note
    If the namespace is in `full` enforcement, deleting the intent removes the allow-rules, which means the previously allowed traffic will be blocked until a replacement intent is applied. Plan your policy changes accordingly.

## Next steps

- See the [SegmentationIntent reference](../reference/segmentationintent.md) for full field documentation.
- See the [Namespace management guide](namespace-management.md) to set enforcement mode on your namespace.
- See [ClusterProfile](../reference/clusterprofile.md) for `provisioningMode` options.
