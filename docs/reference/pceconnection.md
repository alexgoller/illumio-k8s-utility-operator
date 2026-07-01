# PCEConnection

Cluster-scoped. Defines a connection to one Illumio PCE.

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `pceUrl` | string | yes | PCE host:port (e.g. `mypce.example.com:8443`; `443` for SaaS). |
| `orgId` | integer | yes | PCE organization ID. |
| `credentialsSecretRef.name` | string | yes | Secret holding `api_key` and `api_secret`. |
| `credentialsSecretRef.namespace` | string | no | Secret namespace (defaults to the operator's namespace). |
| `externalDataSet` | string | no | Ownership tag stamped on PCE objects the operator creates (default `illumio-operator`). |

## Status

A `Connected` condition reports reachability/auth. Reasons: `Connected`, `SecretMissing`, `AuthFailed`, `RateLimited`, `PCEUnreachable`.

## Reading the credentials Secret

The Secret in `credentialsSecretRef` holds `api_key` and `api_secret`, base64-encoded under `.data` (the Helm chart names it `illumio-pce-api` unless you set `pce.existingSecret`). To inspect it:

```bash
kubectl get secret illumio-pce-api -n illumio-operator \
  -o jsonpath='{.data}' | jq 'map_values(@base64d)'
```

## Example

```yaml
apiVersion: microsegment.io/v1alpha1
kind: PCEConnection
metadata:
  name: prod-pce
spec:
  pceUrl: mypce.example.com:8443
  orgId: 3
  credentialsSecretRef:
    name: illumio-pce-api
    namespace: illumio-operator
```
