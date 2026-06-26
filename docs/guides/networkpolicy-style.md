# NetworkPolicy-style segmentation with SegmentationPolicy

If you are already familiar with Kubernetes `NetworkPolicy`, `SegmentationPolicy` lets you express Illumio allow-rules in the same shape — with one `ingress` block per permitted inbound flow, `from` peers that use `podSelector`/`namespaceSelector`, and a `ports` list. The operator compiles it into exactly the same Illumio ruleset machinery as `SegmentationIntent`. Both front-ends are equal in power; the choice is mental-model preference.

!!! note "Same backend, different syntax"
    `SegmentationPolicy` and `SegmentationIntent` produce identical Illumio rulesets via the same backend. There is no feature difference. Use whichever shape your team finds more natural.

## How selectors map to Illumio labels

The critical difference from standard Kubernetes `NetworkPolicy` is that **selectors map to Illumio labels, not to pod labels**. The operator does not do pod-level selection. Instead:

- `podSelector.matchLabels` — the key/value pairs are looked up in the PCE as Illumio label key/value pairs. These identify the **consumer workload** by its Illumio labels (for example `{app: checkout, env: prod}`).
- `namespaceSelector.matchLabels` — similarly resolved against the PCE's label vocabulary for the consumer's namespace.

Only `matchLabels` is supported. `matchExpressions` is not supported and causes an immediate rejection (see [Rejection rules](#rejection-rules) below). Consumer labels must already exist in the PCE (Kubelink creates them from real running workloads); the operator never creates labels.

## Guardrails

The same guardrails that apply to `SegmentationIntent` apply here:

**The provider is always your namespace's own app.** The operator derives the provider from the namespace's Illumio `app` label (set by the `ClusterProfile` namespace rules). A `SegmentationPolicy` protects the namespace it lives in. You cannot write a policy that protects another namespace.

**Consumer labels must already exist in the PCE.** If any label key/value pair in a `from` peer does not exist in the PCE, the entire policy is `Rejected`.

**The namespace must be managed.** If no `ClusterProfile` covers the namespace, the policy is rejected with reason `ClusterProfileNotReady`.

## Complete example

The following `SegmentationPolicy` for the `payments` namespace allows:

- The `checkout` app in `prod` to reach port 8443/TCP.
- Any workload in a namespace with `team: platform` to reach port 9090/TCP.

```yaml
apiVersion: microsegment.io/v1alpha1
kind: SegmentationPolicy
metadata:
  name: payments-ingress
  namespace: payments
spec:
  podSelector: {}          # must be empty — protects all pods in the namespace
  policyTypes:
    - Ingress              # must be ["Ingress"] — egress is not supported
  ingress:
    - from:
        - podSelector:
            matchLabels:
              app: checkout
              env: prod
      ports:
        - port: 8443
          protocol: TCP
    - from:
        - namespaceSelector:
            matchLabels:
              team: platform
      ports:
        - port: 9090
          protocol: TCP
```

To allow a consumer on all ports, omit the `ports` field on the rule:

```yaml
  ingress:
    - from:
        - podSelector:
            matchLabels:
              app: monitoring
      # no ports — allows all ports from the monitoring app
```

Check the result:

```bash
kubectl get segpol -n payments
# NAME               READY   PROVISIONED   ENFORCEMENT
# payments-ingress   True    True          visibility_only
```

## Rejection rules

The operator validates the spec before making any PCE call. Policies that violate the supported subset are set to `Ready=False` with reason `Rejected` and are not retried until the spec changes.

| What you wrote | Reason rejected |
|----------------|-----------------|
| `policyTypes: [Egress]` or `policyTypes: [Ingress, Egress]` | Egress policy is not supported. |
| `podSelector` with any `matchLabels` or `matchExpressions` | The provider is always the namespace's own app — a non-empty pod selector is not allowed. |
| `matchExpressions` in any `from` peer's `podSelector` or `namespaceSelector` | Only `matchLabels` is supported; label expressions cannot be translated to PCE label lookups. |
| A `from` peer with neither `podSelector` nor `namespaceSelector` (an `ipBlock`-style peer) | IP block peers are not supported. Every peer must have at least one label selector. |
| An `ingress` rule with an empty `from` list | Each ingress rule must have at least one peer. |

Example of a rejected status:

```
Conditions:
  Type   Status  Reason    Message
  ----   ------  ------    -------
  Ready  False   Rejected  matchExpressions are not supported in from[0].podSelector
```

## Enforcement

`SegmentationPolicy` has an optional `spec.enforcement` field (`idle`, `visibility_only`, `full`). It works identically to the same field on `SegmentationIntent` — see [Enforcement mode and effective enforcement](#enforcement-mode-and-effective-enforcement) below.

### Enforcement mode and effective enforcement

Writing a `SegmentationPolicy` compiles allow-rules. **Rules only block traffic when enforcement is `full`.** In `visibility_only` or `idle` mode the rules exist in the PCE but traffic is not blocked.

The namespace's **effective enforcement** is the strictest of:

1. The admin baseline — the `enforcementMode` set by the matching `ClusterProfile` namespace rule.
2. Every policy CR (`SegmentationPolicy` and `SegmentationIntent`) in the namespace that has `spec.enforcement` set.

The order of strictness is `idle` < `visibility_only` < `full`. The winning value is applied to the namespace's Container Workload Profile (CWP) and is reported on each CR's status:

```
Enforcement:        full
EffectiveEnforcement: full
EnforcementSetBy:     payments-ingress
```

`EnforcementSetBy` names the CR that determined the effective enforcement, or `admin` if the admin baseline was strictest.

!!! warning "Rules and enforcement are independent"
    A policy CR does not need to set `spec.enforcement` to affect rules, and setting `spec.enforcement: full` does not guarantee traffic is blocked — the namespace's effective enforcement depends on all policy CRs in the namespace and the admin baseline. Always check `status.effectiveEnforcement` to see what is actually applied.

## Comparison with SegmentationIntent

| | SegmentationPolicy | SegmentationIntent |
|---|---|---|
| Shape | NetworkPolicy `ingress`/`from`/`ports` | Intent `allow[].from`/`ports` |
| Consumer identification | `podSelector.matchLabels` / `namespaceSelector.matchLabels` | `from` map[string]string |
| `matchExpressions` | Not supported (rejected) | N/A (flat map only) |
| Ports | `ingress[].ports[].{port, protocol}` | `allow[].ports[].{port, protocol}` |
| Enforcement field | `spec.enforcement` | `spec.enforcement` |
| Backend | Shared Illumio ruleset backend | Same |
| Guardrails | Provider locked to namespace app | Same |

If you prefer the intent shape, see the [Segmentation policy guide](segmentation-policy.md).

## Deleting a policy

Deleting a `SegmentationPolicy` CR removes its owned Illumio ruleset and provisions the deletion. A finalizer ensures cleanup happens before the object is removed from Kubernetes.

```bash
kubectl delete segpol payments-ingress -n payments
```

!!! note
    If the namespace is in `full` enforcement, deleting the policy removes the allow-rules and previously allowed traffic will be blocked until a replacement policy is applied.

## Next steps

- [SegmentationPolicy reference](../reference/segmentationpolicy.md) — full field and status documentation.
- [Segmentation policy guide](segmentation-policy.md) — the intent-style front-end and provisioning mode details.
- [Namespace management guide](namespace-management.md) — how to set a namespace's admin enforcement baseline.
