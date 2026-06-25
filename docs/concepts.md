# Concepts

| Term | Meaning |
|------|---------|
| **PCE** | Illumio Policy Compute Engine — the policy brain the operator talks to over REST API v2. |
| **C-VEN / Kubelink** | The in-cluster Illumio agents, deployed by the official Helm chart (not by this operator). |
| **Container Cluster** | The PCE object representing your Kubernetes cluster. Onboarding creates it. |
| **Pairing Profile / pairing key** | The PCE objects that produce the activation code the C-VEN uses to pair. |
| **Container Workload Profile (CWP)** | Per-namespace policy/label configuration in the PCE (managed in a later release). |

## Custom Resources

| Kind | Scope | Purpose |
|------|-------|---------|
| `PCEConnection` | cluster | Connection + credentials to one PCE. |
| `ClusterProfile` | cluster | Onboards the cluster to the PCE and publishes agent credentials. |

Run `kubectl get illumio` to list all operator resources.
