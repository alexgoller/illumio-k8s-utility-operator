# Cluster Onboarding

A `ClusterProfile` connects this Kubernetes cluster to the Illumio PCE as a **Container Cluster**. There are **two paths**, chosen with `onboarding.mode`:

## Two onboarding paths

| Path | `onboarding.mode` | Use when | What the operator does |
|------|------------------|----------|------------------------|
| **Create** (default) | `create` | The cluster is **not yet onboarded**. | Creates the Container Cluster, a node Pairing Profile, generates the pairing key, and writes the credentials Secret. |
| **Adopt** | `adopt` | The cluster is **already onboarded** — e.g. the C-VEN was paired by the Illumio helm chart or another process. | Finds the existing Container Cluster **by name**, records its href, and marks `Onboarded=True`. **No pairing, no pairing key, no credentials Secret** — the cluster is already paired. |

In **both** paths, once `Onboarded=True` everything downstream — namespace/CWP labeling, `SegmentationIntent`/`SegmentationPolicy`, and `PolicyInsight` preflight — works the same. Adopt simply skips the pairing steps that don't apply to an already-paired cluster.

```yaml
# Path 2 — adopt an already-onboarded cluster
apiVersion: microsegment.io/v1alpha1
kind: ClusterProfile
metadata: { name: this-cluster }
spec:
  pceConnectionRef: { name: prod-pce }
  onboarding:
    mode: adopt
    containerClusterName: ocp-prod-01   # the name of the EXISTING PCE Container Cluster
    # credentialsOutputSecret + nodePairingProfile are not needed in adopt mode
```

Via Helm: `--set onboarding.enabled=true --set onboarding.mode=adopt --set onboarding.containerClusterName=<existing-cluster>`.

### Adopting an already-onboarded cluster (walkthrough)

The common case: the C-VEN was installed and paired by the **Illumio helm chart** (or a prior manual pairing), so a Container Cluster already exists in the PCE. You want this operator to *manage* that cluster (labels, policy, preflight) without re-pairing it.

1. **Find the existing Container Cluster's name** in the PCE (Infrastructure → Container Clusters), e.g. `ocp-prod-01`.
2. **Create the `PCEConnection`** (or install the chart with PCE creds) as usual — adoption still needs API access to the PCE.
3. **Create the `ClusterProfile` in `adopt` mode** with `containerClusterName` set to that exact name (above). No `credentialsOutputSecret`, no `nodePairingProfile` — those are for pairing, which already happened.
4. **Verify:**
   ```bash
   kubectl get clusterprofile this-cluster
   # NAME           CLUSTER       CLUSTERID   ONBOARDED   MANAGED-NS
   # this-cluster   ocp-prod-01   a1b2c3d4…   True        47
   kubectl get clusterprofile this-cluster -o jsonpath='{.status.conditions[?(@.type=="Onboarded")].message}{"\n"}'
   # existing container cluster adopted
   ```
   `ONBOARDED=True` with the message **"existing container cluster adopted"** confirms the operator matched and recorded the existing cluster (its href/UUID appear in `status.containerClusterHref`/`containerClusterID`). From here, `namespaceRules`, `SegmentationIntent`/`Policy`, and `PolicyInsight` all work exactly as in create mode.

**What adopt does *not* do:** it never creates a Container Cluster or Pairing Profile, never generates a pairing key, and never writes a credentials Secret. It leaves the existing pairing untouched.

**Managing namespaces on an adopted cluster:** add `namespaceRules` / `systemNamespaces` to the same `ClusterProfile` — CWP labeling, enforcement, and automatic pickup of new namespaces work identically to create mode. See [Namespace management → what triggers a reconcile](namespace-management.md#what-triggers-a-reconcile-and-new-namespace-behavior).

**Troubleshooting adopt:** if `Onboarded=False` with reason `OnboardFailed` and a message like *"container cluster … was not found"*, the `containerClusterName` doesn't match an existing cluster in the PCE. Fix the name (it is case- and exact-match by name) and the operator retries. The controller re-checks on its healthy cadence, so a cluster that appears later is picked up automatically.

## What the operator does (create mode)

When you create a `ClusterProfile` in the default `create` mode:

1. Reads the referenced `PCEConnection` and waits for it to be `Connected`.
2. Calls the PCE API to create a **Container Cluster** with the specified name. If a cluster with that name already exists and the operator has not yet recorded its href, onboarding fails — use `adopt` mode instead (see [Pre-existing cluster in create mode](#pre-existing-cluster-in-create-mode)).
3. Ensures a **node Pairing Profile** exists — either creating one with the supplied labels and enforcement mode, or reusing an existing profile by name.
4. Calls the PCE to **generate a pairing key** for that profile.
5. Writes the credentials to a Kubernetes Secret in the operator's namespace (key: `credentialsOutputSecret`).
6. Updates the `ClusterProfile` status with the cluster's UUID and sets the `Onboarded` condition to `True`.

The operator does **not** deploy the Illumio C-VEN agent itself. The output Secret is meant to be consumed by a separate Helm release for the agent (see [Using the credentials with Helm](#using-the-credentials-with-helm)).

## Applying a ClusterProfile

### Minimal example

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

The operator creates a Pairing Profile with default settings (`enforcementMode: idle`) and no node labels.

### With node labels and enforcement mode

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
    nodePairingProfile:
      labels:
        role: node
        env: prod
      enforcementMode: visibility_only
```

The operator resolves each label key/value to an Illumio label href (creating the label on the PCE if it does not yet exist), then creates the Pairing Profile with those labels applied.

### Reusing an existing Pairing Profile

If a Pairing Profile already exists in the PCE and you want the operator to use it instead of creating a new one, supply its name via `existingName`. The `labels` and `enforcementMode` fields are ignored in this case.

```yaml
spec:
  onboarding:
    containerClusterName: ocp-prod-01
    credentialsOutputSecret: illumio-cluster-creds
    nodePairingProfile:
      existingName: my-existing-profile
```

## Checking status

```bash
# Short summary (uses the cprof short name)
kubectl get cprof

# Example output:
# NAME           CLUSTER        CLUSTERID                              ONBOARDED
# this-cluster   ocp-prod-01    a1b2c3d4-e5f6-7890-abcd-ef1234567890   True
```

To inspect the full status conditions:

```bash
kubectl get clusterprofile this-cluster -o jsonpath='{.status.conditions}' | jq .
```

The `Onboarded` condition will report `status: "True"` once the cluster has been successfully registered and a pairing key issued.

For the PCE Container Cluster UUID:

```bash
kubectl get clusterprofile this-cluster -o jsonpath='{.status.containerClusterID}'
```

## The output Secret

Once onboarding succeeds, the operator writes (or overwrites) the Secret named by `credentialsOutputSecret` in the operator's namespace. It contains four keys:

| Key | Description |
|-----|-------------|
| `pce_url` | The PCE endpoint (`host:port`). |
| `cluster_id` | The Container Cluster UUID. |
| `cluster_token` | The pairing token for the C-VEN agent. |
| `cluster_code` | The activation code generated by the PCE. |

Inspect the Secret:

```bash
kubectl get secret illumio-cluster-creds -n illumio-operator -o jsonpath='{.data}' | jq 'map_values(@base64d)'
```

## Using the credentials with Helm

### GitOps / Flux (recommended)

Reference the operator-managed Secret in a Flux `HelmRelease` `valuesFrom` block so that the C-VEN Helm chart receives the credentials automatically once the `ClusterProfile` is onboarded:

```yaml
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: illumio-cven
  namespace: illumio-operator
spec:
  chart:
    spec:
      chart: illumio-cven
      sourceRef:
        kind: HelmRepository
        name: illumio
  valuesFrom:
    - kind: Secret
      name: illumio-cluster-creds
      valuesKey: pce_url
      targetPath: pce.url
    - kind: Secret
      name: illumio-cluster-creds
      valuesKey: cluster_id
      targetPath: pce.clusterID
    - kind: Secret
      name: illumio-cluster-creds
      valuesKey: cluster_token
      targetPath: pce.token
    - kind: Secret
      name: illumio-cluster-creds
      valuesKey: cluster_code
      targetPath: pce.activationCode
```

### Manual helm install

```bash
PCE_URL=$(kubectl get secret illumio-cluster-creds -n illumio-operator -o jsonpath='{.data.pce_url}' | base64 -d)
CLUSTER_ID=$(kubectl get secret illumio-cluster-creds -n illumio-operator -o jsonpath='{.data.cluster_id}' | base64 -d)
CLUSTER_TOKEN=$(kubectl get secret illumio-cluster-creds -n illumio-operator -o jsonpath='{.data.cluster_token}' | base64 -d)
CLUSTER_CODE=$(kubectl get secret illumio-cluster-creds -n illumio-operator -o jsonpath='{.data.cluster_code}' | base64 -d)

helm install illumio-cven <cven-chart> \
  --set pce.url="$PCE_URL" \
  --set pce.clusterID="$CLUSTER_ID" \
  --set pce.token="$CLUSTER_TOKEN" \
  --set pce.activationCode="$CLUSTER_CODE"
```

## Pre-existing cluster in `create` mode

In **`create`** mode, if a Container Cluster with the requested name already exists on the PCE **and this `ClusterProfile` has not yet recorded its href**, the operator sets `Onboarded=False` with reason `OnboardFailed` and stops — it will not create a duplicate, and the original's one-time pairing token cannot be recovered.

The intended fix is **`onboarding.mode: adopt`** (see [Two onboarding paths](#two-onboarding-paths)): it manages the existing cluster in place without needing the token. Alternatively, delete the Container Cluster from the PCE and let `create` mode make a fresh one.

## Output Secret ownership

The output Secret named by `credentialsOutputSecret` is **operator-managed**. The operator creates or updates it on every successful reconcile: `pce_url` and `cluster_id` are always (re)written; `cluster_token` is written once when the cluster is freshly created and preserved on subsequent reconciles; `cluster_code` is (re)written on every reconcile (a new pairing key is generated each time). Do not edit this Secret manually — the operator will overwrite your changes.

## One-command install with the Helm chart

The operator's own Helm chart can optionally create the `ClusterProfile` as part of a single install:

```bash
helm install illumio-operator ./dist/chart \
  --namespace illumio-operator \
  --create-namespace \
  --set pce.url=mypce.example.com:8443 \
  --set pce.orgId=3 \
  --set pce.apiKey=api_1234567890abcdef \
  --set pce.apiSecret=your-api-secret \
  --set onboarding.enabled=true \
  --set onboarding.containerClusterName=ocp-prod-01
```

See [Installation](../installation.md) for the full Helm install reference.
