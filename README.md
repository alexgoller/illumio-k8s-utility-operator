# Illumio Kubernetes Utility Operator

**Manage Illumio segmentation for Kubernetes and OpenShift the GitOps way — onboard clusters, label workloads, and author micro-segmentation policy as native Kubernetes resources.**

[![Docs](https://img.shields.io/badge/docs-live-blue)](https://alexgoller.github.io/illumio-k8s-utility-operator/)
[![Release](https://img.shields.io/github/v/release/alexgoller/illumio-k8s-utility-operator?sort=semver)](https://github.com/alexgoller/illumio-k8s-utility-operator/releases)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://www.apache.org/licenses/LICENSE-2.0)

📖 **[Documentation site → alexgoller.github.io/illumio-k8s-utility-operator](https://alexgoller.github.io/illumio-k8s-utility-operator/)**

---

## What it does

The Illumio K8s Utility Operator connects a Kubernetes or OpenShift cluster to an Illumio **PCE** and lets platform and application teams drive Illumio from the cluster instead of clicking through the PCE console:

- **🔌 Connect** — register one or more PCEs with a single `PCEConnection` resource and store credentials in a Kubernetes Secret (or reuse one from Vault / External Secrets).
- **🚀 Onboard or adopt** — create the PCE **Container Cluster**, node **Pairing Profile**, and pairing key from a `ClusterProfile` — or **adopt** a cluster that's already paired (e.g. by the Illumio helm chart) with `onboarding.mode: adopt`.
- **🏷️ Label** — apply Illumio labels to namespaces and workloads (Container Workload Profiles) from declarative namespace rules, with safe handling for system namespaces.
- **🛡️ Segment** — author micro-segmentation policy as `SegmentationIntent` (allow-list style) or `SegmentationPolicy` (Kubernetes `NetworkPolicy` style). Both compile to the **same Illumio ruleset backend**.
- **🚦 Enforce & provision** — control enforcement (`idle` → `visibility_only` → `full`) per namespace and provision rulesets automatically or behind a manual approval gate.

Everything is a Kubernetes custom resource, so it lives in Git, flows through your CI/CD, and reconciles continuously.

## Who it's for

| Audience | Why it helps |
|---|---|
| **Platform / cluster teams** | One operator onboards the cluster and keeps Illumio labels in sync with namespaces — no per-app PCE work. |
| **Application teams** | Express "who can talk to my service" in YAML next to the app, in a model they already know (`NetworkPolicy`) or a simple allow-list. |
| **Illumio PS & consultants** | Repeatable, auditable, GitOps-friendly onboarding and policy authoring for customer clusters — drop in a chart, set PCE creds, and the cluster appears in the PCE with labels and rulesets driven from source control. |

## How it works

```
   ┌────────────────────┐         ┌──────────────────────────────────────┐
   │  Your Git repo /    │ apply   │  Kubernetes / OpenShift cluster       │
   │  CI-CD pipeline      │────────▶│                                       │
   └────────────────────┘         │   ┌──────────────────────────────┐    │
                                   │   │  Illumio K8s Utility Operator │    │
   PCEConnection ─────────────────┼──▶│                              │    │
   ClusterProfile  (onboard+label)│   │  reconciles CRs ──────────────┼────┼──┐
   SegmentationIntent / Policy ───┼──▶│                              │    │  │ REST
                                   │   └──────────────────────────────┘    │  │ (API key)
                                   └──────────────────────────────────────┘  │
                                                                              ▼
                                            ┌───────────────────────────────────────┐
                                            │  Illumio PCE                            │
                                            │  Container Cluster · Pairing Profile ·  │
                                            │  Labels / CWPs · Rulesets · Enforcement │
                                            └───────────────────────────────────────┘
```

You declare intent in Kubernetes; the operator translates it into Container Clusters, labels, and rulesets in the PCE and keeps them reconciled.

## The four custom resources

| CRD | Short name | Scope | What it does |
|-----|-----------|-------|--------------|
| `PCEConnection` | `pceconn` | Cluster | Holds the PCE URL, org ID, and a reference to the credentials Secret. The connection every other resource uses. |
| `ClusterProfile` | `clusterprofile` | Cluster | Onboards the cluster to the PCE and defines namespace **label rules**, system-namespace handling, the **enforcement baseline**, and the **provisioning mode**. |
| `SegmentationIntent` | `segintent` | Namespaced | Illumio-native **allow-list** policy for the namespace it lives in. |
| `SegmentationPolicy` | `segpol` | Namespaced | The same policy expressed in Kubernetes **`NetworkPolicy`** shape (`ingress` / `from` / `ports`). |

`SegmentationIntent` and `SegmentationPolicy` compile to the same backend and have identical capabilities — pick whichever matches your team's mental model.

---

## Quickstart

### Requirements

- A Kubernetes or OpenShift cluster (`kubectl` configured).
- **Helm 3** (recommended install path).
- An Illumio **PCE (24.5+)** with an API key/secret and your **org ID**.

### 1. Install the operator and connect to your PCE

The operator and its Helm chart are published to GitHub Container Registry:

- Chart: `oci://ghcr.io/alexgoller/charts/illumio-k8s-utility-operator`
- Image: `ghcr.io/alexgoller/illumio-k8s-utility-operator`

```bash
helm install illumio-operator \
  oci://ghcr.io/alexgoller/charts/illumio-k8s-utility-operator --version 0.1.17 \
  --namespace illumio-operator --create-namespace \
  --set pce.url=mypce.example.com:8443 \
  --set pce.orgId=3 \
  --set pce.apiKey=api_1234567890abcdef \
  --set pce.apiSecret=your-api-secret
```

This installs the CRDs, deploys the operator, and creates a `PCEConnection` named `default`. Already manage secrets elsewhere? Use `--set pce.existingSecret=my-pce-credentials` (a Secret with keys `api_key` / `api_secret`) instead of the raw values.

### 2. Onboard the cluster

Add onboarding to the same install (or a `helm upgrade`) to create the PCE Container Cluster and pairing key:

```bash
helm upgrade illumio-operator \
  oci://ghcr.io/alexgoller/charts/illumio-k8s-utility-operator --version 0.1.17 \
  --namespace illumio-operator --reuse-values \
  --set onboarding.enabled=true \
  --set onboarding.containerClusterName=ocp-prod-01
```

The operator creates the Container Cluster, generates a node Pairing Profile and pairing key, and writes the credentials to a Secret (`illumio-cluster-creds`) in the `illumio-operator` namespace. See the [Onboarding guide](docs/guides/onboarding.md) for status fields and how to consume the pairing key.

### 3. Write your first policy

Allow everything inside a namespace to talk to itself:

```yaml
apiVersion: microsegment.io/v1alpha1
kind: SegmentationIntent
metadata:
  name: myapp-internal
  namespace: myapp
spec:
  allowIntraNamespace: true
```

```bash
kubectl apply -f myapp-internal.yaml
kubectl get segintent -n myapp
# NAME             READY   PROVISIONED   AFFECTED
# myapp-internal   True    True          8
```

> Installing from source instead? Use `./dist/chart` in place of the OCI ref, or `make install && make deploy IMG=...`. Full options (values file, existing secrets, OpenShift notes) are in the [Installation guide](docs/installation.md).

---

## Policy

The operator lets app teams express Illumio segmentation as Kubernetes custom resources, eliminating the need to write rulesets in the PCE console by hand. Two CRDs are available — choose the one that fits your team's mental model.

### CRDs at a glance

| CRD | Short name | Shape | Best for |
|-----|------------|-------|----------|
| `SegmentationIntent` | `segintent` | Intent-style `allow[]` list | Teams new to Illumio, or who prefer an explicit allow-list |
| `SegmentationPolicy` | `segpol` | NetworkPolicy `ingress`/`from`/`ports` | Teams already familiar with Kubernetes `NetworkPolicy` |

Both CRDs compile to the **same Illumio ruleset backend** and have identical capabilities. The choice is purely a matter of style.

### Key concepts

**Your namespace, your rules.** Each policy CR protects the namespace it lives in. The provider is always your namespace's own app (derived from its Illumio `app` label set by the `ClusterProfile`). You cannot write rules that protect another team's namespace.

**Intra-scope vs extra-scope consumers.** An Illumio ruleset is scoped to your namespace's app. Consumers from *within* that scope (same-namespace workloads) are **intra-scope**; consumers from other apps are **extra-scope**. Both types are supported.

**Rules and enforcement are independent.** A policy CR declares *what* is allowed. The namespace's effective enforcement — `idle`, `visibility_only`, or `full` — determines whether non-allowed traffic is blocked. Only `full` enforcement blocks. Effective enforcement is computed as the strictest value across the `ClusterProfile` admin baseline and all policy CRs in the namespace.

**Unknown labels are configurable.** By default (`strict` mode), a consumer label that does not yet exist in the PCE causes the CR to be rejected. Set `microsegment.io/unknown-label-mode: skip` on the CR (or namespace) to silently omit the unknown consumer and keep the CR `Ready`, or `create` to mint the label in the PCE automatically. Provider labels are always resolved strictly.

### How-tos

#### 1. Allow any-any within a namespace

The simplest policy: all workloads in the namespace can reach each other on all ports.

**SegmentationIntent:**

```yaml
apiVersion: microsegment.io/v1alpha1
kind: SegmentationIntent
metadata:
  name: myapp-internal
  namespace: myapp
spec:
  allowIntraNamespace: true
```

**SegmentationPolicy equivalent:**

```yaml
apiVersion: microsegment.io/v1alpha1
kind: SegmentationPolicy
metadata:
  name: myapp-internal
  namespace: myapp
spec:
  ingress:
    - from:
        - podSelector: {}   # all pods in this namespace
```

Apply and verify (in `manual` provisioning mode, approve first):

```bash
kubectl apply -f myapp-internal.yaml

# Approve provisioning (if ClusterProfile.spec.provisioningMode is "manual")
kubectl annotate segintent myapp-internal microsegment.io/provision=approved -n myapp

kubectl get segintent -n myapp
# NAME             READY   PROVISIONED   AFFECTED
# myapp-internal   True    True          8
```

#### 2. Cross-app ingress (extra-scope)

Allow the `checkout` app in `prod` to reach the `payments` service on port 8443.

**SegmentationIntent:**

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
```

**SegmentationPolicy equivalent:**

```yaml
apiVersion: microsegment.io/v1alpha1
kind: SegmentationPolicy
metadata:
  name: payments-ingress
  namespace: payments
spec:
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              app: checkout
              env: prod
      ports:
        - port: 8443
          protocol: TCP
```

#### 3. Service-to-service within a namespace (intra-scope, narrowed)

Protect only `role=backend` pods, and allow only `role=frontend` pods to reach them.

**SegmentationIntent:**

```yaml
apiVersion: microsegment.io/v1alpha1
kind: SegmentationIntent
metadata:
  name: backend-access
  namespace: payments
spec:
  provider: { role: backend }
  allow:
    - fromIntraNamespace: { role: frontend }
      ports:
        - { port: 8443, protocol: TCP }
```

**SegmentationPolicy equivalent:**

```yaml
apiVersion: microsegment.io/v1alpha1
kind: SegmentationPolicy
metadata:
  name: backend-access
  namespace: payments
spec:
  podSelector:
    matchLabels:
      role: backend
  ingress:
    - from:
        - podSelector:
            matchLabels:
              role: frontend
      ports:
        - port: 8443
          protocol: TCP
```

#### 4. Tolerate not-yet-existing consumer labels

If the consumer workload does not exist in the PCE yet, use `skip` mode to keep the CR `Ready` and retry on each reconcile.

```yaml
apiVersion: microsegment.io/v1alpha1
kind: SegmentationIntent
metadata:
  name: payments-ingress
  namespace: payments
  annotations:
    microsegment.io/unknown-label-mode: skip
spec:
  allow:
    - from: { app: future-service, env: prod }
      ports:
        - { port: 8443, protocol: TCP }
```

Check which labels are deferred:

```bash
kubectl get segintent payments-ingress -n payments -o jsonpath='{.status.deferredLabels}'
# ["app=future-service"]
```

---

## Documentation

| Start here | |
|---|---|
| [Getting started](docs/getting-started.md) | End-to-end first run: connect, onboard, label, segment. |
| [Installation](docs/installation.md) | All Helm options, existing-secret, values file, source build. |
| [Concepts](docs/concepts.md) | The Illumio + Kubernetes model the operator implements. |

| Guides | |
|---|---|
| [Onboarding](docs/guides/onboarding.md) | Container Cluster, Pairing Profile, reading the pairing key. |
| [Namespace management](docs/guides/namespace-management.md) | Label rules, system namespaces, Container Workload Profiles. |
| [Policy concepts](docs/guides/policy-concepts.md) | Scope, rules vs enforcement, provisioning, unknown labels — **start here for policy**. |
| [Segmentation policy](docs/guides/segmentation-policy.md) | `SegmentationIntent` compilation and provisioning modes. |
| [NetworkPolicy-style](docs/guides/networkpolicy-style.md) | `SegmentationPolicy` selector mapping in depth. |
| [Policy preflight](docs/guides/preflight.md) | `PolicyInsight` — what-if preflight from observed PCE flows before you enforce. |
| [Security & credentials](docs/guides/security.md) | Protecting PCE API keys (one key per cluster), RBAC, TLS, etcd, supply chain. |
| [LabelMap & the operator](docs/guides/labelmap-and-the-operator.md) | Coexisting with Illumio's per-workload LabelMap. |

| Reference | |
|---|---|
| [PCEConnection](docs/reference/pceconnection.md) · [ClusterProfile](docs/reference/clusterprofile.md) · [SegmentationIntent](docs/reference/segmentationintent.md) · [SegmentationPolicy](docs/reference/segmentationpolicy.md) · [PolicyInsight](docs/reference/policyinsight.md) · [RuleView](docs/reference/ruleview.md) | Full field and status documentation. |

## Compatibility

| Component | Supported |
|---|---|
| Illumio PCE | 24.5+ (on-prem or SaaS) |
| Kubernetes | 1.25+ |
| OpenShift | 4.x |
| Helm | 3.x |

## Status & roadmap

The operator is in active development (`v0.1.x`, API group `microsegment.io/v1alpha1`). Shipped and live-verified against a real PCE today:

- ✅ PCE connection and credential management
- ✅ Cluster onboarding (Container Cluster + Pairing Profile + key) **or adopt** an already-onboarded cluster
- ✅ Namespace / Container Workload Profile labeling, incl. helm-tunable system namespaces
- ✅ `SegmentationIntent` and `SegmentationPolicy` → Illumio rulesets (intra-/extra-scope)
- ✅ Per-namespace enforcement and auto/manual/draft provisioning
- ✅ Configurable unknown-label handling (`strict` / `skip` / `create`)
- ✅ LabelMap overlap detection (warns when Illumio's LabelMap labels the same dimension)
- ✅ On-request policy preflight (`PolicyInsight`) — what-if from observed PCE flows + draft decisions
- ✅ Live rule view (`RuleView`) — the current Illumio rules protecting your app in `kubectl`, incl. rules authored outside k8s

On the roadmap:

- 🚧 **Deny / override-deny rules** (Track 2 — design complete, pending PCE Core 25.x deny-rules verification)
- 🚧 **Egress policy** (Track 6 — designed)
- 🚧 **Selective enforcement** (Track 3 — designed)

See `docs/superpowers/specs/` for the design notes and roadmap.

## Contributing

Issues and pull requests are welcome. For local development:

```bash
make test        # unit + envtest suite
make lint        # golangci-lint
make manifests generate   # regenerate CRDs/code after API changes
```

Run `make help` for all targets. The project is built with [Kubebuilder](https://book.kubebuilder.io/introduction.html).

## License

Copyright 2026. Licensed under the [Apache License, Version 2.0](https://www.apache.org/licenses/LICENSE-2.0).

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
