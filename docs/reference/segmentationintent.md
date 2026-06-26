# SegmentationIntent

Namespaced. Declares an allow-list of inbound flows for the namespace's own application. The operator compiles each intent into one owned Illumio ruleset and provisions it according to the cluster's `ClusterProfile.spec.provisioningMode`.

Short name: `segintent`. Category: `illumio`.

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `allow` | []IntentAllow | yes | List of permitted inbound flows. At least one entry is required. |
| `allow[].from` | map[string]string | yes | Illumio label key/value pairs identifying the consumer (e.g. `{app: checkout, env: prod}`). Every key/value pair must already exist in the PCE (created by Kubelink from real workloads). An unknown label causes the entire intent to be `Rejected`. |
| `allow[].ports` | []IntentPort | no | Ports the consumer may reach. When omitted, all ports are allowed. |
| `allow[].ports[].port` | integer | yes | TCP or UDP port number. |
| `allow[].ports[].protocol` | string | no | `TCP` or `UDP`. Defaults to `TCP`. |
| `enforcement` | string | no | Requests a namespace enforcement mode. One of `idle`, `visibility_only`, `full`. The namespace's effective enforcement is the strictest of the admin baseline (`ClusterProfile` namespace rule) and all policy CRs (`SegmentationIntent` and `SegmentationPolicy`) in the namespace. Setting this field does not unilaterally change the namespace's CWP — it participates in the strictest-wins calculation. See [Effective enforcement](#effective-enforcement). |

## Annotation

| Annotation | Value | Effect |
|------------|-------|--------|
| `microsegment.io/provision` | `approved` | When `ClusterProfile.spec.provisioningMode` is `manual`, the operator waits for this annotation before provisioning the compiled draft. Set it with `kubectl annotate segmentationintent <name> microsegment.io/provision=approved`. While the annotation is present, the operator keeps the intent's policy provisioned and re-provisions on every spec change. To stop further provisioning of new changes, remove the annotation with `kubectl annotate segmentationintent <name> microsegment.io/provision-`. Per-change approval (re-approving each individual edit) is planned for a future release. |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | []Condition | Standard Kubernetes conditions list. See below. |
| `workloadsAffected` | integer | Count of PCE workloads affected by the last provisioning operation. Displayed as the `Affected` print column. |
| `effectiveEnforcement` | string | The namespace's resolved enforcement mode (`idle`, `visibility_only`, or `full`) after applying the strictest-wins algorithm across the admin baseline and all policy CRs in the namespace. |
| `enforcementSetBy` | string | Names the CR (or `admin`) that determined the effective enforcement. `admin` means the `ClusterProfile` admin baseline was strictest. |
| `observedGeneration` | integer | Generation of the spec that produced the current status. |

## Effective enforcement

The `enforcement` spec field participates in a per-namespace strictest-wins resolution. The namespace's effective enforcement is the maximum of:

1. The admin baseline — `enforcementMode` from the matching `ClusterProfile` namespace rule.
2. `spec.enforcement` on every `SegmentationIntent` and `SegmentationPolicy` in the namespace.

Strictness order: `idle` < `visibility_only` < `full`. The winning value is applied to the namespace's Container Workload Profile (CWP). `status.effectiveEnforcement` reflects the value currently applied; `status.enforcementSetBy` names the source.

**Rules and enforcement are independent.** Rules determine what traffic is allowed. Enforcement determines whether non-allowed traffic is blocked — only `full` enforcement blocks. `visibility_only` allows all traffic while recording flows. `idle` allows all traffic without recording.

### The `Ready` condition

| Reason | Status | Meaning |
|--------|--------|---------|
| `Compiled` | `True` | All consumer labels resolved; the Illumio ruleset has been written (as draft or provisioned, depending on the provisioning mode). |
| `Rejected` | `False` | One or more consumer labels in `from` do not exist in the PCE, or the provider label could not be resolved. The `message` field identifies the unknown label. The intent is not retried until the spec changes. |
| `ClusterProfileNotReady` | `False` | No `ClusterProfile` manages this namespace, or the `ClusterProfile` has not finished reconciling. The controller will retry. |

### The `Provisioned` condition

| Reason | Status | Meaning |
|--------|--------|---------|
| `Provisioned` | `True` | The ruleset has been provisioned to the PCE. `status.workloadsAffected` reflects the count from this operation. |
| `ProvisionPending` | `False` | The ruleset is compiled but not yet provisioned. In `manual` mode, set the `microsegment.io/provision=approved` annotation to trigger provisioning. In `draft-only` mode, this condition stays `False` permanently — provision the ruleset in the PCE directly. |

## Print columns

`kubectl get segintent` (or `kubectl get segmentationintents`) shows:

| Column | Source |
|--------|--------|
| `READY` | `status.conditions[type=="Ready"].status` |
| `PROVISIONED` | `status.conditions[type=="Provisioned"].status` |
| `AFFECTED` | `status.workloadsAffected` |

## Example

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
    - from: { app: monitoring }
      # no ports — allows all ports from the monitoring app
```

See the [Segmentation policy guide](../guides/segmentation-policy.md) for compilation details, provisioning mode walkthroughs, and enforcement notes.
See the [NetworkPolicy-style guide](../guides/networkpolicy-style.md) if you prefer the `SegmentationPolicy` NetworkPolicy-shaped front-end.
See the [SegmentationPolicy reference](segmentationpolicy.md) for the NetworkPolicy-style CRD field documentation.
