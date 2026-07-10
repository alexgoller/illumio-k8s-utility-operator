# SegmentationPolicy

Namespaced. A NetworkPolicy-style allow-list for inbound traffic to the namespace's own application. The operator compiles each policy into one owned Illumio ruleset via the same backend as `SegmentationIntent`, and provisions it according to the cluster's `ClusterProfile.spec.provisioningMode`.

Short name: `segpol`. Category: `illumio`.

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `podSelector` | LabelSelector | no | Narrows the protected provider to a sub-set of this namespace's app. `matchLabels` identifies a specific workload group (e.g. `{role: backend}`). An empty `podSelector: {}` means the whole namespace app. `matchExpressions` is not supported and causes rejection. |
| `policyTypes` | []string | no | Must be `["Ingress"]` if specified. Egress is not supported and causes rejection. Defaults to `["Ingress"]` when omitted. |
| `ingress` | []IngressRule | yes | List of ingress rules. Each rule allows traffic from the listed peers on the listed ports. |
| `ingress[].from` | []NetworkPolicyPeer | yes | Consumer peers. At least one peer is required per rule. Each peer must have exactly one of `podSelector` or `namespaceSelector` — setting both on the same peer is rejected. An empty `from` list is rejected. |
| `ingress[].from[].podSelector` | LabelSelector | no | Intra-scope (same-namespace) consumer selector. `matchLabels` narrows by Illumio labels (e.g. `{role: frontend}`). An empty `podSelector: {}` means all workloads in this namespace (intra-scope any-any). `matchExpressions` is not supported and causes rejection. |
| `ingress[].from[].namespaceSelector` | LabelSelector | no | Extra-scope (cross-app) consumer selector. `matchLabels` identifies another app by its Illumio labels (e.g. `{app: checkout, env: prod}`). `matchExpressions` is not supported and causes rejection. |
| `ingress[].ports` | []NetworkPolicyPort | no | Ports the consumer may reach. When omitted, all ports are allowed (compiled as "All Services"). |
| `ingress[].ports[].port` | integer | yes (in port) | TCP or UDP port number. |
| `ingress[].ports[].protocol` | string | no | `TCP` or `UDP`. Defaults to `TCP`. |
| `enforcement` | string | no | Requests a namespace enforcement mode. One of `idle`, `visibility_only`, `selective`, `full`. Participates in the strictest-wins calculation — does not unilaterally switch enforcement. See [Effective enforcement](#effective-enforcement). |

### How selectors map to Illumio concepts

| Peer selector | Illumio scope | Equivalent SegmentationIntent field |
|---------------|---------------|-------------------------------------|
| `podSelector: {}` | Intra-scope, All Workloads | `allowIntraNamespace: true` |
| `podSelector: {matchLabels: {role: frontend}}` | Intra-scope, narrowed by role | `allow[].fromIntraNamespace: {role: frontend}` |
| `namespaceSelector: {matchLabels: {app: checkout}}` | Extra-scope (another app) | `allow[].from: {app: checkout}` |
| top-level `podSelector: {matchLabels: {role: backend}}` | Provider narrowing | `spec.provider: {role: backend}` |

### Rejection rules

The operator rejects (sets `Ready=False`, reason `Rejected`) a policy that violates any of these rules:

| Condition | Rejection message |
|-----------|-------------------|
| `policyTypes` includes `Egress` | `Egress policyType is not supported` |
| `podSelector` has `matchExpressions` | `matchExpressions are not supported in podSelector` |
| A `from` peer has both `podSelector` and `namespaceSelector` | `from[N]: podSelector and namespaceSelector cannot both be set` |
| A `from` peer has neither `podSelector` nor `namespaceSelector` | `from[N] must have podSelector or namespaceSelector` |
| Any `from` peer uses `matchExpressions` | `matchExpressions are not supported in from[N].podSelector` (or `namespaceSelector`) |
| An `ingress` rule has an empty `from` list | `ingress[N].from must not be empty` |
| A consumer label key/value does not exist in the PCE (strict mode) | `label not found in PCE: <key>=<value>` |

## Annotations

| Annotation | Value | Effect |
|------------|-------|--------|
| `microsegment.io/provision` | `approved` | When `ClusterProfile.spec.provisioningMode` is `manual`, the operator waits for this annotation before provisioning the compiled draft. Behavior is identical to `SegmentationIntent`. Set with `kubectl annotate segpol <name> microsegment.io/provision=approved -n <ns>`. Remove with `kubectl annotate segpol <name> microsegment.io/provision- -n <ns>`. |
| `microsegment.io/unknown-label-mode` | `strict`, `skip`, or `create` | Per-CR override for how unresolvable consumer labels are handled. Overrides the namespace annotation and `ClusterProfile.spec.unknownLabelMode`. Provider (`podSelector`) labels are always resolved strictly. |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | []Condition | Standard Kubernetes conditions. See below. |
| `workloadsAffected` | integer | Count of PCE workloads affected by the last provisioning operation. |
| `effectiveEnforcement` | string | The namespace's resolved enforcement mode (`idle`, `visibility_only`, `selective`, or `full`) after applying the strictest-wins algorithm. Shown as the `ENFORCEMENT` print column. |
| `enforcementSetBy` | string | Names the CR (or `admin`) that determined the effective enforcement. `admin` means the `ClusterProfile` admin baseline was strictest. |
| `unknownLabelMode` | string | The effective unknown-label mode used when this CR was compiled (`strict`, `skip`, or `create`). |
| `unknownLabelModeSetBy` | string | Source of the resolved mode: `cr`, `namespace`, `clusterprofile`, or `default`. |
| `deferredLabels` | []string | In `skip` mode, the `key=value` consumer labels that were not found in the PCE and were omitted from the compiled rules. Retried on every reconcile. |
| `createdLabels` | []string | In `create` mode, the `key=value` labels that were minted in the PCE while compiling this CR. |
| `observedGeneration` | integer | Generation of the spec that produced the current status. |

### The `Ready` condition

| Reason | Status | Meaning |
|--------|--------|---------|
| `Compiled` | `True` | All consumer labels resolved; the Illumio ruleset has been written. |
| `Rejected` | `False` | The spec violates the supported subset, or one or more consumer labels do not exist in the PCE (strict mode). The `message` field gives the specific cause. Not retried until the spec changes. |
| `ClusterProfileNotReady` | `False` | No `ClusterProfile` manages this namespace, or the `ClusterProfile` has not finished reconciling. The controller retries automatically. |
| `PCEStateConflict` | `False` | The operator-owned ruleset has an **unprovisioned pending deletion** in the PCE (someone deleted it in the PCE but did not provision the change). The operator defers rather than overriding the pending change — resolve it in the PCE (provision or revert the deletion) and the operator recovers automatically. The controller re-checks periodically. |

### The `Provisioned` condition

| Reason | Status | Meaning |
|--------|--------|---------|
| `Provisioned` | `True` | The ruleset has been provisioned to the PCE. `status.workloadsAffected` reflects the count from this operation. |
| `ProvisionPending` | `False` | The ruleset is compiled but not yet provisioned. In `manual` mode, add the `microsegment.io/provision=approved` annotation to trigger provisioning. In `draft-only` mode, this condition stays `False` permanently. |

## Effective enforcement

The `enforcement` field requests — but does not unilaterally set — the namespace's enforcement mode. The operator computes the **effective enforcement** as the strictest value across:

1. The admin baseline (`enforcementMode` from the matching `ClusterProfile` namespace rule).
2. The `spec.enforcement` value on every `SegmentationPolicy` and `SegmentationIntent` in the namespace.

Strictness order: `idle` < `visibility_only` < `selective` < `full`. The winning value is applied to the namespace's Container Workload Profile (CWP). It is reflected in `status.effectiveEnforcement`; `status.enforcementSetBy` names the CR (or `admin`) that provided the winning value.

**Rules and enforcement are independent.** Setting `spec.enforcement: full` does not affect what traffic is allowed — it only determines whether non-allowed traffic is blocked. Rules determine what is allowed; enforcement determines whether the allow-list is enforced.

## Print columns

`kubectl get segpol` (or `kubectl get segmentationpolicies`) shows:

| Column | Source |
|--------|--------|
| `READY` | `status.conditions[type=="Ready"].status` |
| `PROVISIONED` | `status.conditions[type=="Provisioned"].status` |
| `ENFORCEMENT` | `status.effectiveEnforcement` |

## Examples

### Cross-app ingress (extra-scope, namespaceSelector)

```yaml
apiVersion: microsegment.io/v1alpha1
kind: SegmentationPolicy
metadata:
  name: payments-ingress
  namespace: payments
spec:
  policyTypes:
    - Ingress
  ingress:
    - from:
        - namespaceSelector:
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
      # no ports — allows all ports from workloads in platform namespaces (All Services)
  enforcement: visibility_only
```

Verify:

```bash
kubectl get segpol -n payments
# NAME               READY   PROVISIONED   ENFORCEMENT
# payments-ingress   True    True          visibility_only
```

### Allow any-any within the namespace (intra-scope shortcut)

```yaml
apiVersion: microsegment.io/v1alpha1
kind: SegmentationPolicy
metadata:
  name: myapp-internal
  namespace: myapp
spec:
  ingress:
    - from:
        - podSelector: {}   # all pods in this namespace — intra-scope any-any
```

### Service-to-service within a namespace (intra-scope, narrowed)

```yaml
apiVersion: microsegment.io/v1alpha1
kind: SegmentationPolicy
metadata:
  name: backend-access
  namespace: payments
spec:
  podSelector:                           # narrow the provider to role=backend
    matchLabels:
      role: backend
  ingress:
    - from:
        - podSelector:
            matchLabels:
              role: frontend             # frontend → backend, intra-scope
      ports:
        - port: 8443
          protocol: TCP
```

### Tolerate not-yet-existing consumer labels (skip mode)

```yaml
apiVersion: microsegment.io/v1alpha1
kind: SegmentationPolicy
metadata:
  name: payments-ingress
  namespace: payments
  annotations:
    microsegment.io/unknown-label-mode: skip   # omit unknown consumers; retry each reconcile
spec:
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              app: future-service
              env: prod
      ports:
        - port: 8443
          protocol: TCP
```

Verify deferred labels:

```bash
kubectl get segpol payments-ingress -n payments -o jsonpath='{.status.deferredLabels}'
# ["app=future-service"]
```

See the [NetworkPolicy-style guide](../guides/networkpolicy-style.md) for compilation details, rejection examples, and enforcement behavior.
See the [Segmentation policy guide](../guides/segmentation-policy.md) for the intent-style alternative.
See the [SegmentationIntent reference](segmentationintent.md) for the intent-style CRD field documentation.
