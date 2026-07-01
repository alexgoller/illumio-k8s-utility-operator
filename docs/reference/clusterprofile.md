# ClusterProfile

Cluster-scoped. Onboards a Kubernetes cluster to an Illumio PCE by ensuring a Container Cluster and node Pairing Profile exist, generating a pairing key, and publishing the resulting credentials to a Kubernetes Secret.

Short name: `cprof`. Category: `illumio`.

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `pceConnectionRef.name` | string | yes | Name of the `PCEConnection` to use for PCE API calls. |
| `onboarding.containerClusterName` | string | yes | Name of the PCE Container Cluster object to create or reuse. |
| `onboarding.credentialsOutputSecret` | string | yes | Name of the Secret (in the operator's namespace) where the operator writes `pce_url`, `cluster_id`, `cluster_token`, and `cluster_code`. |
| `onboarding.nodePairingProfile.existingName` | string | no | Name of an existing PCE Pairing Profile to reuse. When set, `labels` and `enforcementMode` are ignored. |
| `onboarding.nodePairingProfile.labels` | map[string]string | no | Illumio label key/value pairs to assign to nodes paired with this profile. The operator resolves each to a label href (creating the label on the PCE if necessary). |
| `onboarding.nodePairingProfile.enforcementMode` | string | no | Enforcement mode for a newly created Pairing Profile. One of `idle`, `visibility_only`, `full`. Defaults to `idle`. |
| `provisioningMode` | string | no | Default provisioning mode for `SegmentationIntent`/`SegmentationPolicy` rulesets in this cluster: `auto` provisions on every change, `manual` waits for the `microsegment.io/provision=approved` annotation, `draft-only` compiles but never provisions. Defaults to `manual`. |
| `systemNamespaces.manage` | boolean | no | When `true`, namespaces matching the system patterns are configured by the `systemNamespaces` block. Takes precedence over any matching `namespaceRule`. Defaults to `false`. |
| `systemNamespaces.patterns` | []string | no | Glob patterns that identify system namespaces. Defaults when empty to: `openshift-*`, `kube-*`, `default`, `kube-system`, `kube-public`, `kube-node-lease`. |
| `systemNamespaces.labels` | map[string]string | no | Illumio label key/value pairs assigned to system-namespace CWPs. |
| `systemNamespaces.enforcementMode` | string | no | Enforcement mode for system namespaces. One of `idle`, `visibility_only`, `full`. Defaults to `idle`. |
| `namespaceRules[].match.namePattern` | string | no | Glob (Go `path.Match` syntax) matched against the namespace name. Empty matches any name. |
| `namespaceRules[].match.labels` | map[string]string | no | k8s labels that must all be present on the namespace (subset match). |
| `namespaceRules[].managed` | boolean | yes | Marks the namespace CWP as PCE-managed when `true`. |
| `namespaceRules[].assignLabels` | map[string]LabelAssignment | no | Illumio label key → assignment. Each assignment is either `{value: "..."}` (fixed) or `{fromNamespaceLabel: "..."}` (copy from a k8s label on the namespace; skipped if absent). |
| `namespaceRules[].enforcementMode` | string | no | Enforcement mode for matching namespaces. One of `idle`, `visibility_only`, `full`. |

## Namespace annotations

Individual namespaces can override the rule-derived CWP configuration:

| Annotation | Values | Effect |
|------------|--------|--------|
| `microsegment.io/managed` | `"true"` / `"false"` | Override the managed flag for this namespace. |
| `microsegment.io/enforcement` | `idle`, `visibility_only`, `full` | Override the enforcement mode for this namespace. |
| `microsegment.io/label.<key>` | any string | Override the value of Illumio label `<key>` for this namespace's CWP. |

Annotations are evaluated last and always take precedence over `systemNamespaces` and `namespaceRules`.

## Status

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | []Condition | Standard Kubernetes conditions list. See below. |
| `containerClusterHref` | string | Full PCE API href of the Container Cluster object. |
| `containerClusterID` | string | Container Cluster UUID (last segment of the href). |
| `managedNamespaces` | integer | Count of namespaces whose CWP is currently marked managed by the operator. |
| `observedGeneration` | integer | Generation of the spec that produced the current status. |

### The `Onboarded` condition

| Reason | Status | Meaning |
|--------|--------|---------|
| `Onboarded` | `True` | The Container Cluster and Pairing Profile exist on the PCE and the credentials Secret has been written. |
| `PCEConnectionNotReady` | `False` | The referenced `PCEConnection` is not yet `Connected`. The controller will retry. |
| `OnboardFailed` | `False` | A PCE API call failed during onboarding. The `message` field contains the error detail. The controller will retry with backoff. |

## Print columns

`kubectl get cprof` shows:

| Column | Source |
|--------|--------|
| `CLUSTER` | `spec.onboarding.containerClusterName` |
| `CLUSTERID` | `status.containerClusterID` |
| `ONBOARDED` | `status.conditions[type=="Onboarded"].status` |
| `MANAGED-NS` | `status.managedNamespaces` |

## Example

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
  provisioningMode: manual
```

See the [Onboarding guide](../guides/onboarding.md) for a full walkthrough.
