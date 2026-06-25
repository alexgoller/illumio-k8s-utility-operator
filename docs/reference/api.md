# API Reference

## Packages
- [microsegment.io/v1alpha1](#microsegmentiov1alpha1)


## microsegment.io/v1alpha1

Package v1alpha1 contains API Schema definitions for the microsegment v1alpha1 API group.

### Resource Types
- [PCEConnection](#pceconnection)
- [PCEConnectionList](#pceconnectionlist)



#### PCEConnection



PCEConnection is the Schema for the pceconnections API



_Appears in:_
- [PCEConnectionList](#pceconnectionlist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `microsegment.io/v1alpha1` | | |
| `kind` _string_ | `PCEConnection` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  | Optional: \{\} <br /> |
| `spec` _[PCEConnectionSpec](#pceconnectionspec)_ | spec defines the desired state of PCEConnection |  | Required: \{\} <br /> |


#### PCEConnectionList



PCEConnectionList contains a list of PCEConnection





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `microsegment.io/v1alpha1` | | |
| `kind` _string_ | `PCEConnectionList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[PCEConnection](#pceconnection) array_ |  |  |  |


#### PCEConnectionSpec



PCEConnectionSpec defines a connection to one Illumio PCE.



_Appears in:_
- [PCEConnection](#pceconnection)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `pceUrl` _string_ | PCEURL is the PCE host:port (443 for SaaS, 8443 typical on-prem). |  |  |
| `orgId` _integer_ | OrgID is the PCE organization id. |  |  |
| `credentialsSecretRef` _[SecretReference](#secretreference)_ | CredentialsSecretRef references the Secret with api_key / api_secret. |  |  |
| `externalDataSet` _string_ | ExternalDataSet is the ownership tag stamped on PCE objects this<br />operator creates. Defaults to "illumio-operator" if empty. |  | Optional: \{\} <br /> |




#### SecretReference



SecretReference points to a Kubernetes Secret holding PCE API credentials.



_Appears in:_
- [PCEConnectionSpec](#pceconnectionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the Secret (keys: api_key, api_secret). |  |  |
| `namespace` _string_ | Namespace of the Secret. Defaults to the operator's namespace if empty. |  | Optional: \{\} <br /> |


