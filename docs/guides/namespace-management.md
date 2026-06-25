# Namespace (CWP) Management

The operator can configure the **Container Workload Profile (CWP)** for every namespace in the cluster. A CWP controls how Illumio treats the workloads in that namespace: whether they are PCE-managed, what Illumio labels are attached, and what enforcement mode applies.

## How it works

Illumio Kubelink creates one CWP per namespace when the C-VEN agent starts. The operator does **not** create CWPs itself — it only updates CWPs that Kubelink has already created. Namespaces whose CWP does not yet exist are skipped and reconciled automatically once the CWP appears.

When the operator reconciles a `ClusterProfile` it:

1. Lists all namespaces in the cluster.
2. Determines the desired CWP configuration for each namespace using the **precedence model** described below.
3. Calls the PCE API to update each CWP (managed flag, Illumio labels, enforcement mode).
4. Updates `status.managedNamespaces` with the count of namespaces whose CWP is marked managed.
5. Emits a `CWPConfigured` event on each namespace after a successful update.

## Precedence model

The desired configuration for a namespace is resolved in three layers, applied in order from highest to lowest priority:

1. **Per-namespace annotation overrides** — annotations on the namespace object itself always take final precedence (see [Namespace annotations](#namespace-annotations)).
2. **`systemNamespaces` config** — when `systemNamespaces.manage` is `true`, namespaces whose name matches any of the system patterns receive the `systemNamespaces` configuration. This **overrides any matching `namespaceRule`** for those namespaces.
3. **`namespaceRules` (first match)** — for all other namespaces, rules are evaluated in order; the first rule whose `match` criteria are satisfied governs the namespace. A namespace that matches no rule receives no CWP update.

A managed namespace with no `enforcementMode` set (neither from a rule nor from an annotation) defaults to `idle`.

## System namespaces

The `systemNamespaces` stanza is a convenience for managing Kubernetes and OpenShift infrastructure namespaces without having to enumerate them in `namespaceRules`.

When `systemNamespaces.patterns` is empty, the following default patterns apply:

- `openshift-*`
- `kube-*`
- `default`
- `kube-system`
- `kube-public`
- `kube-node-lease`

You can supply a custom `patterns` list to override these defaults.

## Namespace rules

`namespaceRules` is an ordered list of rules. Each rule has:

- `match` — criteria to select namespaces:
  - `namePattern` — a glob (Go `path.Match` syntax, e.g. `team-*`). Empty matches any name.
  - `labels` — a map of k8s label key/value pairs that must all be present on the namespace.
- `managed` — whether to mark the CWP as PCE-managed.
- `assignLabels` — Illumio label key → assignment. Two assignment forms:
  - `value: "prod"` — assign a fixed value.
  - `fromNamespaceLabel: "app.kubernetes.io/part-of"` — read the value from the namespace's own k8s label. If the label is absent on the namespace, this assignment is skipped (the Illumio label is left unchanged).
- `enforcementMode` — one of `idle`, `visibility_only`, `full`.

!!! note "Only `idle`, `visibility_only`, and `full` are valid for container workload profiles."
    The `selective` mode is not supported for CWPs.

## Namespace annotations

Individual namespaces can override the rule-derived configuration by adding annotations:

| Annotation | Values | Effect |
|------------|--------|--------|
| `microsegment.io/managed` | `"true"` or `"false"` | Override the managed flag. |
| `microsegment.io/enforcement` | `idle`, `visibility_only`, `full` | Override the enforcement mode. |
| `microsegment.io/label.<key>` | any string | Override the value of Illumio label `<key>`. E.g. `microsegment.io/label.env=staging`. |

Annotations are evaluated after all rule resolution, so they always win.

## Complete example

The following `ClusterProfile` is a recommended starting point for OpenShift clusters. It:

- Marks all system namespaces (`openshift-*`, `kube-*`, etc.) as managed with `visibility_only` enforcement and fixed Illumio labels.
- Marks namespaces that carry the `microsegment.io/managed: "true"` label as managed, deriving Illumio labels from standard k8s labels.
- Marks everything else as unmanaged (no CWP update for unlabelled namespaces).

```yaml
apiVersion: microsegment.io/v1alpha1
kind: ClusterProfile
metadata:
  name: this-cluster
spec:
  pceConnectionRef: { name: prod-pce }
  onboarding:
    containerClusterName: ocp-prod-01
    credentialsOutputSecret: illumio-cluster-creds
  systemNamespaces:
    manage: true
    labels: { role: control, env: prod }
    enforcementMode: visibility_only
    # patterns default to openshift-*, kube-*, default, kube-system, kube-public, kube-node-lease
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

The catch-all `namePattern: "*"` rule at the end explicitly marks every remaining namespace as unmanaged, so unlabelled application namespaces are never enrolled by accident.

## Observing results

### Managed namespace count

```bash
kubectl get clusterprofiles
```

The `MANAGED-NS` column shows how many namespaces currently have a managed CWP:

```
NAME           CLUSTER        CLUSTERID   ONBOARDED   MANAGED-NS
this-cluster   ocp-prod-01    a1b2c3d4…   True        47
```

To read the count directly:

```bash
kubectl get clusterprofile this-cluster -o jsonpath='{.status.managedNamespaces}'
```

### Per-namespace CWP events

The operator emits a `CWPConfigured` event on each namespace after updating its CWP. Inspect them with:

```bash
kubectl describe namespace <ns-name>
```

Look for events of type `Normal` with reason `CWPConfigured`:

```
Events:
  Type    Reason         Age    From                               Message
  ----    ------         ----   ----                               -------
  Normal  CWPConfigured  2m5s   clusterprofile-controller          CWP configured: managed=true enforcement=visibility_only
```

### Verifying a specific namespace

```bash
# Check which annotations are set
kubectl get namespace <ns-name> -o jsonpath='{.metadata.annotations}' | jq .

# Watch the CWP update event stream
kubectl get events --field-selector involvedObject.name=<ns-name>,involvedObject.kind=Namespace
```

## Troubleshooting

**Namespace not being updated**

If a namespace is expected to be managed but does not appear in the count or events, the most likely cause is that Kubelink has not yet created a CWP for it. The operator skips namespaces with no CWP and retries on the next reconcile. Check that the C-VEN agent and Kubelink are running in the cluster.

**Wrong enforcement mode**

Check annotation precedence first. An annotation on the namespace itself overrides everything:

```bash
kubectl get namespace <ns-name> -o jsonpath='{.metadata.annotations.microsegment\.io/enforcement}'
```

**`fromNamespaceLabel` assignment not applied**

The assignment is silently skipped if the referenced k8s label is absent from the namespace. Verify the label exists:

```bash
kubectl get namespace <ns-name> --show-labels
```
