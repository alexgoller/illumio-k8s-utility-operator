# API Reference

## Packages
- [microsegment.io/v1alpha1](#microsegmentiov1alpha1)


## microsegment.io/v1alpha1

Package v1alpha1 contains API Schema definitions for the microsegment v1alpha1 API group.

### Resource Types
- [ClusterProfile](#clusterprofile)
- [ClusterProfileList](#clusterprofilelist)
- [PCEConnection](#pceconnection)
- [PCEConnectionList](#pceconnectionlist)



#### ClusterProfile



ClusterProfile onboards a Kubernetes cluster to an Illumio PCE.



_Appears in:_
- [ClusterProfileList](#clusterprofilelist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `microsegment.io/v1alpha1` | | |
| `kind` _string_ | `ClusterProfile` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ClusterProfileSpec](#clusterprofilespec)_ |  |  |  |


#### ClusterProfileList



ClusterProfileList contains a list of ClusterProfile.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `microsegment.io/v1alpha1` | | |
| `kind` _string_ | `ClusterProfileList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[ClusterProfile](#clusterprofile) array_ |  |  |  |


#### ClusterProfileSpec



ClusterProfileSpec is the desired onboarding state for this cluster.



_Appears in:_
- [ClusterProfile](#clusterprofile)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `pceConnectionRef` _[LocalObjectReference](#localobjectreference)_ | PCEConnectionRef references the PCEConnection to use. |  |  |
| `onboarding` _[OnboardingSpec](#onboardingspec)_ | Onboarding configures PCE cluster onboarding. |  |  |
| `provisioningMode` _string_ | ProvisioningMode is the default policy provisioning mode for resources in<br />this cluster. One of: auto, manual, draft-only. Consumed by later<br />policy reconciliation; defaults to manual. | manual | Enum: [auto manual draft-only] <br />Optional: \{\} <br /> |




#### LocalObjectReference



LocalObjectReference references a cluster-scoped object by name.



_Appears in:_
- [ClusterProfileSpec](#clusterprofilespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ |  |  |  |


#### NodePairingProfileSpec



NodePairingProfileSpec configures the pairing profile the C-VEN uses to pair
the cluster's nodes. Either reuse an existing profile by name, or have the
operator create one with the given node labels and enforcement mode.



_Appears in:_
- [OnboardingSpec](#onboardingspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `existingName` _string_ | ExistingName: if set, use this existing PCE pairing profile (by name)<br />instead of creating one. Labels/EnforcementMode are ignored in that case. |  | Optional: \{\} <br /> |
| `labels` _object (keys:string, values:string)_ | Labels to apply to nodes paired with this profile, as Illumio<br />label-key -> value (e.g. \{"role": "node", "env": "prod"\}). The operator<br />resolves each to an Illumio label href (create-if-missing). |  | Optional: \{\} <br /> |
| `enforcementMode` _string_ | EnforcementMode for a created pairing profile. One of idle,<br />visibility_only, full. Defaults to idle. | idle | Enum: [idle visibility_only full] <br />Optional: \{\} <br /> |


#### OnboardingSpec



OnboardingSpec configures how the cluster is onboarded to the PCE.



_Appears in:_
- [ClusterProfileSpec](#clusterprofilespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `containerClusterName` _string_ | ContainerClusterName is the name of the PCE Container Cluster object to<br />ensure exists for this cluster. |  |  |
| `credentialsOutputSecret` _string_ | CredentialsOutputSecret is the name of the Secret (in the operator's<br />namespace) the operator writes the agent credentials into:<br />pce_url, cluster_id, cluster_token, cluster_code. |  |  |
| `nodePairingProfile` _[NodePairingProfileSpec](#nodepairingprofilespec)_ | NodePairingProfile configures the pairing profile the C-VEN uses to pair<br />the cluster's nodes. |  | Optional: \{\} <br /> |


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


