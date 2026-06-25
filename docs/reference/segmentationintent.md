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

## Annotation

| Annotation | Value | Effect |
|------------|-------|--------|
| `microsegment.io/provision` | `approved` | When `ClusterProfile.spec.provisioningMode` is `manual`, the operator waits for this annotation before provisioning the compiled draft. Set it with `kubectl annotate segmentationintent <name> microsegment.io/provision=approved`. The operator removes the annotation after provisioning. |

## Status

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | []Condition | Standard Kubernetes conditions list. See below. |
| `workloadsAffected` | integer | Count of PCE workloads affected by the last provisioning operation. Displayed as the `Affected` print column. |
| `observedGeneration` | integer | Generation of the spec that produced the current status. |

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
