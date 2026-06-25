# Getting Started

## Prerequisites

- A Kubernetes or OpenShift cluster.
- Access to an Illumio PCE (24.5+), with an API key/secret and your org ID.

## 1. Install the operator

See [Installation](installation.md).

## 2. Configure a PCE connection

Create a Secret with your PCE API credentials and a `PCEConnection`:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: illumio-pce-api
  namespace: illumio-operator
type: Opaque
stringData:
  api_key: "api_1234567890abcdef"
  api_secret: "your-api-secret"
---
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

Check the connection:

```bash
kubectl get pceconnections
kubectl get pceconnection prod-pce -o jsonpath='{.status.conditions[?(@.type=="Connected")].status}'
```

## 3. Onboard the cluster

(See the Onboarding guide once Plan 2 lands.)
