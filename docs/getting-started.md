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

!!! tip "Reading the PCE credentials back out"
    Values in a Secret are base64-encoded under `.data`. To verify what the operator is
    using — handy when the Secret was created by the Helm chart (`illumio-pce-api` by
    default) rather than by hand — decode it:

    ```bash
    # All keys at once
    kubectl get secret illumio-pce-api -n illumio-operator \
      -o jsonpath='{.data}' | jq 'map_values(@base64d)'
    # { "api_key": "api_1234567890abcdef", "api_secret": "your-api-secret" }

    # A single key
    kubectl get secret illumio-pce-api -n illumio-operator \
      -o jsonpath='{.data.api_key}' | base64 -d
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

Once `ONBOARDED` is `True`, the operator has written a Secret named `illumio-cluster-creds` in the `illumio-operator` namespace containing `pce_url`, `cluster_id`, `cluster_token`, and `cluster_code` — the values the Illumio C-VEN agent needs to pair.

!!! tip "Getting the onboarding credentials out"
    Read the whole Secret, or pull individual keys to feed the C-VEN agent install:

    ```bash
    # All four keys, decoded
    kubectl get secret illumio-cluster-creds -n illumio-operator \
      -o jsonpath='{.data}' | jq 'map_values(@base64d)'

    # Individual values (e.g. into shell vars for a Helm install)
    CLUSTER_ID=$(kubectl get secret illumio-cluster-creds -n illumio-operator -o jsonpath='{.data.cluster_id}'    | base64 -d)
    CLUSTER_TOKEN=$(kubectl get secret illumio-cluster-creds -n illumio-operator -o jsonpath='{.data.cluster_token}' | base64 -d)
    CLUSTER_CODE=$(kubectl get secret illumio-cluster-creds -n illumio-operator -o jsonpath='{.data.cluster_code}'  | base64 -d)
    ```

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

Once a namespace is managed by the `ClusterProfile`, app teams can declare which consumers are allowed to reach their application. There are two equivalent front-ends — choose the one that fits your team's mental model.

**Intent style** (`SegmentationIntent`) — a flat allow-list:

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

**NetworkPolicy style** (`SegmentationPolicy`) — familiar if you already use Kubernetes `NetworkPolicy`:

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
```

Both compile to the same Illumio ruleset via the same backend. Consumer labels (`from` entries) must already exist in the PCE — Kubelink creates them from running workloads. Rules only **block** non-allowed traffic when the namespace's effective enforcement mode is `full`.

```bash
kubectl apply -f payments-ingress.yaml
kubectl get segintent -n payments   # or: kubectl get segpol -n payments
# NAME               READY   PROVISIONED   AFFECTED
# payments-ingress   True    True          12
```

See the [Segmentation policy guide](guides/segmentation-policy.md) for compilation details, provisioning modes, and enforcement notes.
See the [NetworkPolicy-style guide](guides/networkpolicy-style.md) for the `SegmentationPolicy` front-end, including the supported subset and rejection rules.
