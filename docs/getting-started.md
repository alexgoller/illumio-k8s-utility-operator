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

## 4. Manage namespaces

Add `systemNamespaces` and `namespaceRules` to your `ClusterProfile` to control how each namespace's Container Workload Profile is configured on the PCE:

```yaml
spec:
  systemNamespaces:
    manage: true
    labels: { role: control, env: prod }
    enforcementMode: visibility_only
  namespaceRules:
    - match: { labels: { "microsegment.io/managed": "true" } }
      managed: true
      assignLabels:
        app: { fromNamespaceLabel: app.kubernetes.io/part-of }
        env: { fromNamespaceLabel: app.kubernetes.io/environment }
      enforcementMode: visibility_only
    - match: { namePattern: "*" }
      managed: false
```

After applying, check the `MANAGED-NS` count:

```bash
kubectl get cprof
# NAME           CLUSTER        CLUSTERID   ONBOARDED   MANAGED-NS
# this-cluster   ocp-prod-01    a1b2c3d4…   True        47
```

See the [Namespace management guide](guides/namespace-management.md) for the full precedence model, per-namespace annotation overrides, and troubleshooting tips.

## 5. Write segmentation policy

Once a namespace is managed by the `ClusterProfile`, app teams can declare which consumers are allowed to reach their application using a `SegmentationIntent`:

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

```bash
kubectl apply -f payments-ingress.yaml
kubectl get segintent -n payments
# NAME               READY   PROVISIONED   AFFECTED
# payments-ingress   True    True          12
```

The operator compiles the intent into one Illumio ruleset scoped to the `payments` namespace's app and provisions it according to `ClusterProfile.spec.provisioningMode`. Consumer labels in `from` must already exist in the PCE (Kubelink creates them from running workloads). Rules only **block** non-allowed traffic when the namespace's enforcement mode is `full`.

See the [Segmentation policy guide](guides/segmentation-policy.md) for compilation details, provisioning modes, and enforcement notes.
