# Installation

## Requirements

- A Kubernetes or OpenShift cluster (kubectl configured).
- Helm 3 (for the recommended Helm install path).
- Access to an Illumio PCE (24.5+) with an API key/secret and your org ID.
- Go 1.26+ and `make` only if building from source.

## Option 1: Helm (recommended)

The operator's Helm chart is published to **GitHub Container Registry** as an OCI artifact at `oci://ghcr.io/alexgoller/charts/illumio-k8s-utility-operator`, and the operator image at `ghcr.io/alexgoller/illumio-k8s-utility-operator`. This is the fastest way to get running — it installs the CRDs, the operator deployment, and optionally a `PCEConnection` and `ClusterProfile` in a single command. (The same chart lives in-repo at `dist/chart`; use `./dist/chart` instead of the OCI ref if you're building from source.)

### Install the operator and connect to a PCE

```bash
helm install illumio-operator oci://ghcr.io/alexgoller/charts/illumio-k8s-utility-operator --version 0.1.2 \
  --namespace illumio-operator \
  --create-namespace \
  --set pce.url=mypce.example.com:8443 \
  --set pce.orgId=3 \
  --set pce.apiKey=api_1234567890abcdef \
  --set pce.apiSecret=your-api-secret
```

This creates the `illumio-operator` namespace, installs all four CRDs (PCEConnection, ClusterProfile, SegmentationIntent, SegmentationPolicy), deploys the operator, and renders a `PCEConnection` named `default`.

### Also onboard the cluster in the same install

Add `--set onboarding.enabled=true` and supply a Container Cluster name:

```bash
helm install illumio-operator oci://ghcr.io/alexgoller/charts/illumio-k8s-utility-operator --version 0.1.2 \
  --namespace illumio-operator \
  --create-namespace \
  --set pce.url=mypce.example.com:8443 \
  --set pce.orgId=3 \
  --set pce.apiKey=api_1234567890abcdef \
  --set pce.apiSecret=your-api-secret \
  --set onboarding.enabled=true \
  --set onboarding.containerClusterName=ocp-prod-01
```

The operator will create the PCE Container Cluster, generate a node Pairing Profile and pairing key, and write the credentials to a Secret named `illumio-cluster-creds` in the `illumio-operator` namespace. See the [Onboarding guide](guides/onboarding.md) for details on reading status and consuming those credentials.

### Using an existing Secret for PCE credentials

If your credentials are already managed by an external secret manager (e.g., Vault, External Secrets Operator), point the chart at the existing Secret instead of supplying raw values:

```bash
helm install illumio-operator oci://ghcr.io/alexgoller/charts/illumio-k8s-utility-operator --version 0.1.2 \
  --namespace illumio-operator \
  --create-namespace \
  --set pce.url=mypce.example.com:8443 \
  --set pce.orgId=3 \
  --set pce.existingSecret=my-pce-credentials
```

The Secret must exist in the release namespace and contain the keys `api_key` and `api_secret`.

### Using a values file

A ready-to-edit example lives at `dist/chart/values-example.yaml` (and is bundled inside the chart). Fill in your PCE details and pass it with `-f`:

```bash
helm install illumio-operator \
  oci://ghcr.io/alexgoller/charts/illumio-k8s-utility-operator --version 0.1.2 \
  -n illumio-operator --create-namespace \
  -f values-example.yaml
```

### Key Helm values

| Value | Default | Description |
|-------|---------|-------------|
| `pce.url` | `""` | PCE endpoint `host:port` (e.g. `mypce.example.com:8443`; `443` for SaaS). |
| `pce.orgId` | `0` | PCE organization ID. |
| `pce.apiKey` | `""` | PCE API key. Ignored when `pce.existingSecret` is set. |
| `pce.apiSecret` | `""` | PCE API secret. Ignored when `pce.existingSecret` is set. |
| `pce.existingSecret` | `""` | Name of a pre-existing Secret with `api_key`/`api_secret`. |
| `pce.connectionName` | `default` | Name of the `PCEConnection` resource to create. |
| `onboarding.enabled` | `false` | When `true`, creates a `ClusterProfile` for this cluster. |
| `onboarding.name` | `this-cluster` | `metadata.name` of the rendered `ClusterProfile`. Because `ClusterProfile` is cluster-scoped, each cluster must use a unique name to avoid resource collisions. |
| `onboarding.containerClusterName` | `""` | Required when `onboarding.enabled=true`. |
| `onboarding.credentialsOutputSecret` | `illumio-cluster-creds` | Name of the Secret the operator writes credentials into. |
| `onboarding.nodePairingProfile.existingName` | `""` | Reuse this existing PCE Pairing Profile by name. |
| `onboarding.nodePairingProfile.labels` | `{}` | Illumio label key/value pairs for the created Pairing Profile. |
| `onboarding.nodePairingProfile.enforcementMode` | `idle` | Enforcement mode: `idle`, `visibility_only`, or `full`. |

## Option 2: kubectl apply (pre-built manifest)

The committed `dist/install.yaml` contains all four CRDs and the operator deployment, pinned to the published image `ghcr.io/alexgoller/illumio-k8s-utility-operator`. Apply it directly with `kubectl apply -f dist/install.yaml`, or rebuild it against your own image:

```bash
make build-installer IMG=<your-registry>/illumio-k8s-utility-operator:<tag>
```

This regenerates `dist/install.yaml` with all four CRDs and the operator deployment, bound to the image you specify. Then apply it:

```bash
kubectl apply -f dist/install.yaml
```

## Option 3: make install / make deploy (development)

For local development against a cluster where your kubeconfig is already configured:

```bash
make install        # installs CRDs into the cluster
make deploy IMG=<your-registry>/illumio-k8s-utility-operator:dev
```

## Uninstall

### Helm

```bash
helm uninstall illumio-operator --namespace illumio-operator
```

Note: `helm uninstall` does **not** remove the CRDs (Helm never deletes resources from a chart's `crds/` directory). To also remove the CRDs (and any custom resources using them):

```bash
kubectl delete crd clusterprofiles.microsegment.io pceconnections.microsegment.io segmentationintents.microsegment.io segmentationpolicies.microsegment.io
```

### make

```bash
make undeploy
make uninstall
```
