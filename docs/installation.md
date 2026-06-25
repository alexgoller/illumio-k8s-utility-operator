# Installation

## Requirements

- A Kubernetes or OpenShift cluster (kubectl configured).
- Helm 3 (for the recommended Helm install path).
- Access to an Illumio PCE (24.5+) with an API key/secret and your org ID.
- Go 1.26+ and `make` only if building from source.

## Option 1: Helm (recommended)

The operator ships a Helm chart at `dist/chart`. This is the fastest way to get running — it installs the CRDs, the operator deployment, and optionally a `PCEConnection` and `ClusterProfile` in a single command.

### Install the operator and connect to a PCE

```bash
helm install illumio-operator ./dist/chart \
  --namespace illumio-operator \
  --create-namespace \
  --set pce.url=mypce.example.com:8443 \
  --set pce.orgId=3 \
  --set pce.apiKey=api_1234567890abcdef \
  --set pce.apiSecret=your-api-secret
```

This creates the `illumio-operator` namespace, installs both CRDs, deploys the operator, and renders a `PCEConnection` named `default`.

### Also onboard the cluster in the same install

Add `--set onboarding.enabled=true` and supply a Container Cluster name:

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

The operator will create the PCE Container Cluster, generate a node Pairing Profile and pairing key, and write the credentials to a Secret named `illumio-cluster-creds` in the `illumio-operator` namespace. See the [Onboarding guide](guides/onboarding.md) for details on reading status and consuming those credentials.

### Using an existing Secret for PCE credentials

If your credentials are already managed by an external secret manager (e.g., Vault, External Secrets Operator), point the chart at the existing Secret instead of supplying raw values:

```bash
helm install illumio-operator ./dist/chart \
  --namespace illumio-operator \
  --create-namespace \
  --set pce.url=mypce.example.com:8443 \
  --set pce.orgId=3 \
  --set pce.existingSecret=my-pce-credentials
```

The Secret must exist in the release namespace and contain the keys `api_key` and `api_secret`.

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
| `onboarding.containerClusterName` | `""` | Required when `onboarding.enabled=true`. |
| `onboarding.credentialsOutputSecret` | `illumio-cluster-creds` | Name of the Secret the operator writes credentials into. |
| `onboarding.nodePairingProfile.existingName` | `""` | Reuse this existing PCE Pairing Profile by name. |
| `onboarding.nodePairingProfile.labels` | `{}` | Illumio label key/value pairs for the created Pairing Profile. |
| `onboarding.nodePairingProfile.enforcementMode` | `idle` | Enforcement mode: `idle`, `visibility_only`, or `full`. |

## Option 2: kubectl apply (pre-built manifest)

The committed `dist/install.yaml` only carries the `PCEConnection` CRD and is **not** suitable for direct apply. First generate a complete installer that includes both CRDs:

```bash
make build-installer IMG=<your-registry>/illumio-k8s-utility-operator:<tag>
```

This writes a complete `dist/install.yaml` containing both CRDs (`PCEConnection` and `ClusterProfile`) and the operator deployment, bound to the image you specify. Then apply it:

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

Note: CRDs are kept by default (`crd.keep: true`). To also remove the CRDs:

```bash
kubectl delete crd clusterprofiles.microsegment.io pceconnections.microsegment.io
```

### make

```bash
make undeploy
make uninstall
```
