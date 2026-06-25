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

Create a `ClusterProfile` to register this cluster with the PCE:

```yaml
apiVersion: microsegment.io/v1alpha1
kind: ClusterProfile
metadata:
  name: this-cluster
spec:
  pceConnectionRef:
    name: prod-pce
  onboarding:
    containerClusterName: ocp-prod-01
    credentialsOutputSecret: illumio-cluster-creds
```

```bash
kubectl apply -f clusterprofile.yaml
kubectl get cprof   # watch ONBOARDED become True
```

Once `ONBOARDED` is `True`, the operator has written a Secret named `illumio-cluster-creds` in the `illumio-operator` namespace containing `pce_url`, `cluster_id`, `cluster_token`, and `cluster_code`. Use these to configure the Illumio C-VEN agent.

See the [Onboarding guide](guides/onboarding.md) for node Pairing Profile options, how to consume the credentials with Helm or Flux, and important caveats about pre-existing clusters.
