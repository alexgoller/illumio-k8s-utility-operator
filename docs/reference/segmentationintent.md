# SegmentationIntent

Namespaced. Declares an allow-list of inbound flows for the namespace's own application. The operator compiles each intent into one owned Illumio ruleset and provisions it according to the cluster's `ClusterProfile.spec.provisioningMode`.

Short name: `segintent`. Category: `illumio`.

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `provider` | map[string]string | no | Narrows the protected provider to a sub-set of this namespace's app, by Illumio labels (e.g. `{role: backend}`). Labels must already exist in the PCE (resolved strictly regardless of `unknownLabelMode`). Empty (default) means the whole namespace app. |
| `allow` | []IntentAllow | no | List of permitted inbound flows. Required if `allowIntraNamespace` is not set. |
| `allow[].from` | map[string]string | no | Cross-app (extra-scope) consumer: Illumio label key/value pairs identifying a consumer in another app (e.g. `{app: checkout, env: prod}`). Set exactly one of `from` or `fromIntraNamespace` per entry. |
| `allow[].fromIntraNamespace` | map[string]string | no | Same-namespace (intra-scope) consumer, narrowed by Illumio labels (e.g. `{role: frontend}`); requires at least one label. Set exactly one of `from` or `fromIntraNamespace` per entry. For "all workloads in this namespace", use `allowIntraNamespace: true` instead. |
| `allow[].ports` | []IntentPort | no | Ports the consumer may reach. When omitted, all ports are allowed (compiled as "All Services"). |
| `allow[].ports[].port` | integer | yes (in port) | TCP or UDP port number. |
| `allow[].ports[].protocol` | string | no | `TCP` or `UDP`. Defaults to `TCP`. |
| `allowIntraNamespace` | bool | no | Shortcut: when `true`, allows all workloads in this namespace to reach each other on all ports (an intra-scope allow-all). Use alone for "any-any within the namespace", or alongside `allow` for finer cross-app rules. |
| `enforcement` | string | no | Requests a namespace enforcement mode. One of `idle`, `visibility_only`, `full`. Participates in the strictest-wins calculation — does not unilaterally switch enforcement. See [Effective enforcement](#effective-enforcement). |

### Validation

The operator rejects (sets `Ready=False`, reason `Rejected`) an intent that violates any of these rules:

| Condition | Rejection message |
|-----------|-------------------|
| Neither `allow` nor `allowIntraNamespace` is set | `allow or allowIntraNamespace must be set` |
| An `allow` entry sets both `from` and `fromIntraNamespace` | `allow[N]: set exactly one of from or fromIntraNamespace` |
| An `allow` entry sets neither `from` nor `fromIntraNamespace` | `allow[N]: set exactly one of from or fromIntraNamespace` |
| A `provider` or `from` label key/value does not exist in the PCE (strict mode) | `label not found in PCE: <key>=<value>` |
| A `provider` label key/value does not exist in the PCE (any mode — provider is always strict) | `label not found in PCE: <key>=<value>` |

## Annotations

| Annotation | Value | Effect |
|------------|-------|--------|
| `microsegment.io/provision` | `approved` | When `ClusterProfile.spec.provisioningMode` is `manual`, the operator waits for this annotation before provisioning the compiled draft. Set with `kubectl annotate segintent <name> microsegment.io/provision=approved -n <ns>`. While the annotation is present, the operator keeps the policy provisioned and re-provisions on every spec change. Remove with `kubectl annotate segintent <name> microsegment.io/provision- -n <ns>`. |
| `microsegment.io/unknown-label-mode` | `strict`, `skip`, or `create` | Per-CR override for how unresolvable consumer labels are handled. Overrides the namespace annotation and `ClusterProfile.spec.unknownLabelMode`. Provider labels are always resolved strictly. |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | []Condition | Standard Kubernetes conditions. See below. |
| `workloadsAffected` | integer | Count of PCE workloads affected by the last provisioning operation. Shown as the `AFFECTED` print column. |
| `effectiveEnforcement` | string | The namespace's resolved enforcement mode (`idle`, `visibility_only`, or `full`) after applying the strictest-wins algorithm across the admin baseline and all policy CRs in the namespace. |
| `enforcementSetBy` | string | Names the CR (or `admin`) that determined the effective enforcement. `admin` means the `ClusterProfile` admin baseline was strictest. |
| `unknownLabelMode` | string | The effective unknown-label mode used when this CR was compiled (`strict`, `skip`, or `create`). |
| `unknownLabelModeSetBy` | string | Source of the resolved mode: `cr`, `namespace`, `clusterprofile`, or `default`. |
| `deferredLabels` | []string | In `skip` mode, the `key=value` consumer labels that were not found in the PCE and were omitted from the compiled rules. These are retried on every reconcile. |
| `createdLabels` | []string | In `create` mode, the `key=value` labels that were minted in the PCE while compiling this CR. |
| `observedGeneration` | integer | Generation of the spec that produced the current status. |

### The `Ready` condition

| Reason | Status | Meaning |
|--------|--------|---------|
| `Compiled` | `True` | All consumer labels resolved; the Illumio ruleset has been written (draft or provisioned, depending on provisioning mode). |
| `Rejected` | `False` | Validation failed or one or more consumer labels do not exist in the PCE (strict mode). The `message` field identifies the cause. Not retried until the spec changes. |
| `ClusterProfileNotReady` | `False` | No `ClusterProfile` manages this namespace, or the `ClusterProfile` has not finished reconciling. The controller retries automatically. |

### The `Provisioned` condition

| Reason | Status | Meaning |
|--------|--------|---------|
| `Provisioned` | `True` | The ruleset has been provisioned to the PCE. `status.workloadsAffected` reflects the count from this operation. |
| `ProvisionPending` | `False` | The ruleset is compiled but not yet provisioned. In `manual` mode, add the `microsegment.io/provision=approved` annotation to trigger provisioning. In `draft-only` mode, this condition stays `False` permanently — provision the ruleset in the PCE directly. |

## Effective enforcement

The `enforcement` spec field participates in a per-namespace strictest-wins resolution. The namespace's effective enforcement is the strictest of:

1. The admin baseline — `enforcementMode` from the matching `ClusterProfile` namespace rule.
2. `spec.enforcement` on every `SegmentationIntent` and `SegmentationPolicy` in the namespace.

Strictness order: `idle` < `visibility_only` < `full`. The winning value is applied to the namespace's Container Workload Profile (CWP). `status.effectiveEnforcement` reflects the value currently applied; `status.enforcementSetBy` names the source.

**Rules and enforcement are independent.** Rules determine what traffic is allowed. Enforcement determines whether non-allowed traffic is blocked — only `full` enforcement blocks. `visibility_only` allows all traffic while recording flows. `idle` allows all traffic without recording.

## Print columns

`kubectl get segintent` (or `kubectl get segmentationintents`) shows:

| Column | Source |
|--------|--------|
| `READY` | `status.conditions[type=="Ready"].status` |
| `PROVISIONED` | `status.conditions[type=="Provisioned"].status` |
| `AFFECTED` | `status.workloadsAffected` |

## Examples

### Cross-app ingress with ports (extra-scope)

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
    - from: { app: monitoring, env: prod }
      # no ports — allows all ports from monitoring (All Services)
```

### Allow any-any within the namespace (intra-scope shortcut)

```yaml
apiVersion: microsegment.io/v1alpha1
kind: SegmentationIntent
metadata:
  name: myapp-internal
  namespace: myapp
spec:
  allowIntraNamespace: true
```

### Narrow the provider + intra-scope consumer

```yaml
apiVersion: microsegment.io/v1alpha1
kind: SegmentationIntent
metadata:
  name: backend-access
  namespace: payments
spec:
  provider: { role: backend }            # protect only role=backend in this namespace
  allow:
    - fromIntraNamespace: { role: frontend }   # frontend pods → backend, intra-scope
      ports:
        - { port: 8443, protocol: TCP }
    - from: { app: checkout, env: prod }       # another app → backend, extra-scope
      ports:
        - { port: 8443, protocol: TCP }
```

### Tolerate not-yet-existing consumer labels (skip mode)

```yaml
apiVersion: microsegment.io/v1alpha1
kind: SegmentationIntent
metadata:
  name: payments-ingress
  namespace: payments
  annotations:
    microsegment.io/unknown-label-mode: skip   # omit unknown consumers; retry each reconcile
spec:
  allow:
    - from: { app: future-service, env: prod }
      ports:
        - { port: 8443, protocol: TCP }
```

Verify deferred labels:

```bash
kubectl get segintent payments-ingress -n payments -o jsonpath='{.status.deferredLabels}'
# ["app=future-service"]
```

### Manual provisioning approval

```bash
# Apply the intent (compiles to draft in manual mode)
kubectl apply -f payments-ingress.yaml

# Approve provisioning
kubectl annotate segintent payments-ingress microsegment.io/provision=approved -n payments

# Verify
kubectl get segintent -n payments
# NAME               READY   PROVISIONED   AFFECTED
# payments-ingress   True    True          12
```

See the [Segmentation policy guide](../guides/segmentation-policy.md) for compilation details, provisioning walkthroughs, and enforcement notes.
See the [NetworkPolicy-style guide](../guides/networkpolicy-style.md) if you prefer the `SegmentationPolicy` front-end.
See the [SegmentationPolicy reference](segmentationpolicy.md) for the NetworkPolicy-style CRD field documentation.
