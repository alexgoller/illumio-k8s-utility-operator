# Installation

## Requirements

- Go 1.26+ and `make` (to build from source).
- A container registry you can push to (for your own image), or use the published image.

## Install CRDs and the operator

```bash
make install        # install CRDs into the cluster
make deploy IMG=<your-registry>/illumio-k8s-utility-operator:dev
```

## Uninstall

```bash
make undeploy
make uninstall
```
