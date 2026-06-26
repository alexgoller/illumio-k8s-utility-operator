# SegmentationPolicy

Namespaced. A NetworkPolicy-style allow-list for inbound traffic to the namespace's own application. The operator compiles each policy into one owned Illumio ruleset via the same backend as `SegmentationIntent`, and provisions it according to the cluster's `ClusterProfile.spec.provisioningMode`.

Short name: `segpol`. Category: `illumio`.

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `podSelector` | LabelSelector | no | Must be empty (`{}`). The provider is always the namespace's own app (derived from the namespace's Illumio `app` label). A non-empty `podSelector` is rejected. |
| `policyTypes` | []string | no | Must be `["Ingress"]` if specified. Egress is not supported and causes rejection. Defaults to `["Ingress"]` when omitted. |
| `ingress` | []IngressRule | yes | List of ingress rules. Each rule allows traffic from the listed peers on the listed ports. |
| `ingress[].from` | []NetworkPolicyPeer | yes | Consumer peers. At least one peer is required per rule. Each peer must have a `podSelector` or `namespaceSelector` (or both). An empty `from` list is rejected. |
| `ingress[].from[].podSelector` | LabelSelector | no | Selects consumers by their Illumio labels using `matchLabels`. `matchExpressions` is not supported and causes rejection. |
| `ingress[].from[].podSelector.matchLabels` | map[string]string | no | Illumio label key/value pairs for the consumer (e.g. `{app: checkout, env: prod}`). All key/value pairs must already exist in the PCE. |
| `ingress[].from[].namespaceSelector` | LabelSelector | no | Selects consumers by the Illumio labels of their namespace using `matchLabels`. `matchExpressions` is not supported and causes rejection. |
| `ingress[].from[].namespaceSelector.matchLabels` | map[string]string | no | Illumio label key/value pairs for the consumer's namespace. |
| `ingress[].ports` | []NetworkPolicyPort | no | Ports the consumer may reach. When omitted, all ports are allowed. |
| `ingress[].ports[].port` | integer | yes | TCP or UDP port number. |
| `ingress[].ports[].protocol` | string | no | `TCP` or `UDP`. Defaults to `TCP`. |
| `enforcement` | string | no | Requests a namespace enforcement mode. One of `idle`, `visibility_only`, `full`. The namespace's effective enforcement is the strictest of the admin baseline and all policy CRs in the namespace. See [Effective enforcement](#effective-enforcement). |

## Rejection rules

The following configurations are rejected immediately (before any PCE call), setting `Ready=False` with reason `Rejected`:

| Condition | Rejection message |
|-----------|-------------------|
| `policyTypes` includes `Egress` | `Egress policyType is not supported` |
| `podSelector` has any `matchLabels` or `matchExpressions` | `podSelector must be empty` |
| Any `from` peer uses `matchExpressions` | `matchExpressions are not supported in from[N].podSelector` (or `namespaceSelector`) |
| Any `from` peer has neither `podSelector` nor `namespaceSelector` | `from[N] must have podSelector or namespaceSelector` |
| An `ingress` rule has an empty `from` list | `ingress[N].from must not be empty` |
| A consumer label key/value does not exist in the PCE | `label not found in PCE: <key>=<value>` |

## Annotation

| Annotation | Value | Effect |
|------------|-------|--------|
| `microsegment.io/provision` | `approved` | When `ClusterProfile.spec.provisioningMode` is `manual`, the operator waits for this annotation before provisioning the compiled draft. Behavior is identical to `SegmentationIntent`. |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | []Condition | Standard Kubernetes conditions list. See below. |
| `workloadsAffected` | integer | Count of PCE workloads affected by the last provisioning operation. |
| `effectiveEnforcement` | string | The namespace's resolved enforcement mode (`idle`, `visibility_only`, or `full`) after applying the strictest-wins algorithm across the admin baseline and all policy CRs in the namespace. |
| `enforcementSetBy` | string | Names the CR (or `admin`) that determined the effective enforcement. |
| `observedGeneration` | integer | Generation of the spec that produced the current status. |

### The `Ready` condition

| Reason | Status | Meaning |
|--------|--------|---------|
| `Compiled` | `True` | All consumer labels resolved; the Illumio ruleset has been written. |
| `Rejected` | `False` | The spec violates the supported subset, or one or more consumer labels do not exist in the PCE. The `message` field gives the specific cause. Not retried until the spec changes. |
| `ClusterProfileNotReady` | `False` | No `ClusterProfile` manages this namespace, or the `ClusterProfile` has not finished reconciling. The controller will retry. |

### The `Provisioned` condition

| Reason | Status | Meaning |
|--------|--------|---------|
| `Provisioned` | `True` | The ruleset has been provisioned to the PCE. `status.workloadsAffected` reflects the count from this operation. |
| `ProvisionPending` | `False` | The ruleset is compiled but not yet provisioned. In `manual` mode, add the `microsegment.io/provision=approved` annotation to trigger provisioning. In `draft-only` mode, this condition stays `False` permanently. |

## Effective enforcement

The `enforcement` field requests — but does not unilaterally set — the namespace's enforcement mode. The operator computes the **effective enforcement** as the strictest value across:

1. The admin baseline (`enforcementMode` from the matching `ClusterProfile` namespace rule).
2. The `spec.enforcement` value on every `SegmentationPolicy` and `SegmentationIntent` in the namespace.

Strictness order: `idle` < `visibility_only` < `full`. The winning value is applied to the namespace's Container Workload Profile (CWP). It is reflected in `status.effectiveEnforcement`; `status.enforcementSetBy` names the CR (or `admin`) that provided the winning value.

**Rules and enforcement are independent.** Setting `spec.enforcement: full` does not affect what traffic is allowed — it only determines whether non-allowed traffic is blocked. Rules determine what is allowed; enforcement determines whether the allow-list is enforced.

## Print columns

`kubectl get segpol` (or `kubectl get segmentationpolicies`) shows:

| Column | Source |
|--------|--------|
| `READY` | `status.conditions[type=="Ready"].status` |
| `PROVISIONED` | `status.conditions[type=="Provisioned"].status` |
| `ENFORCEMENT` | `status.effectiveEnforcement` |

## Example

```yaml
apiVersion: microsegment.io/v1alpha1
kind: SegmentationPolicy
metadata:
  name: payments-ingress
  namespace: payments
spec:
  podSelector: {}
  policyTypes:
    - Ingress
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
        - podSelector:
            matchLabels:
              app: ledger
              env: prod
      ports:
        - port: 5432
          protocol: TCP
    - from:
        - namespaceSelector:
            matchLabels:
              team: platform
      # no ports — allows all ports from workloads in platform namespaces
  enforcement: visibility_only
```

See the [NetworkPolicy-style guide](../guides/networkpolicy-style.md) for compilation details, rejection examples, and enforcement behavior.
See the [Segmentation policy guide](../guides/segmentation-policy.md) for the intent-style alternative.
