# Illumio K8s Utility Operator

A Kubernetes operator that automates the **PCE-side** of running Illumio Core on Kubernetes/OpenShift — the work normally done by hand in the Illumio console:

- **Onboarding** — register the cluster with the PCE and hand the agent credentials to Helm.
- **Container Workload Profiles** — manage which namespaces are governed and how they are labeled.
- **Segmentation policy** — let app teams express Illumio policy as Kubernetes resources (`SegmentationIntent` / `SegmentationPolicy`), compiled to PCE rulesets.

The operator is a client of the Illumio PCE REST API. It does **not** deploy the C-VEN/Kubelink agents — that stays with the official Helm chart.

See [Getting Started](getting-started.md) and [Concepts](concepts.md).
