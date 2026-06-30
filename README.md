# illumio-k8s-utility-operator
// TODO(user): Add simple overview of use/purpose

## Description
// TODO(user): An in-depth paragraph about your project and overview of use

## Getting Started

### Prerequisites
- go version v1.26.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/illumio-k8s-utility-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/illumio-k8s-utility-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Policy

The operator lets app teams express Illumio segmentation policy as Kubernetes custom resources, eliminating the need to write rulesets in the PCE console by hand. Two CRDs are available — choose the one that fits your team's mental model.

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

### Further reading

- [Segmentation policy guide](docs/guides/segmentation-policy.md) — compilation, provisioning modes, and enforcement details for `SegmentationIntent`.
- [NetworkPolicy-style guide](docs/guides/networkpolicy-style.md) — `SegmentationPolicy` in depth, selector mapping, and rejection rules.
- [SegmentationIntent reference](docs/reference/segmentationintent.md) — complete field and status documentation.
- [SegmentationPolicy reference](docs/reference/segmentationpolicy.md) — complete field and status documentation.

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/illumio-k8s-utility-operator:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/illumio-k8s-utility-operator/<tag or branch>/dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
kubebuilder edit --plugins=helm/v2-alpha
```

2. See that a chart was generated under 'dist/chart', and users
can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore,
if you create webhooks, you need to use the above command with
the '--force' flag and manually ensure that any custom configuration
previously added to 'dist/chart/values.yaml' or 'dist/chart/manager/manager.yaml'
is manually re-applied afterwards.

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

