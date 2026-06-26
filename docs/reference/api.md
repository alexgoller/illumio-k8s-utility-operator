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
- [SegmentationIntent](#segmentationintent)
- [SegmentationIntentList](#segmentationintentlist)



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
| `systemNamespaces` _[SystemNamespacesSpec](#systemnamespacesspec)_ | SystemNamespaces manages OpenShift/Kubernetes system namespaces out of the box. |  | Optional: \{\} <br /> |
| `namespaceRules` _[NamespaceRule](#namespacerule) array_ | NamespaceRules are evaluated in order; the first match wins. For namespaces<br />that match the SystemNamespaces patterns, SystemNamespaces takes precedence<br />and overrides any matching NamespaceRule. For all other namespaces,<br />the first matching NamespaceRule governs. |  | Optional: \{\} <br /> |




#### IntentAllow



IntentAllow allows a consumer (referenced by existing Illumio labels) to
reach this namespace's app on the given ports.



_Appears in:_
- [SegmentationIntentSpec](#segmentationintentspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `from` _object (keys:string, values:string)_ | From is the consumer's Illumio labels (key -> value), e.g.<br />\{"app":"checkout","env":"prod"\}. They must already exist in the PCE. |  |  |
| `ports` _[IntentPort](#intentport) array_ | Ports the consumer may reach. Empty means all ports. |  | Optional: \{\} <br /> |


#### IntentPort



IntentPort is a port/protocol a consumer is allowed to reach.



_Appears in:_
- [IntentAllow](#intentallow)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `port` _integer_ |  |  |  |
| `protocol` _string_ |  | TCP | Enum: [TCP UDP] <br />Optional: \{\} <br /> |


#### LabelAssignment



LabelAssignment assigns an Illumio label value: a fixed Value, or a value
read from one of the namespace's own k8s labels.



_Appears in:_
- [NamespaceRule](#namespacerule)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `value` _string_ |  |  | Optional: \{\} <br /> |
| `fromNamespaceLabel` _string_ |  |  | Optional: \{\} <br /> |


#### LocalObjectReference



LocalObjectReference references a cluster-scoped object by name.



_Appears in:_
- [ClusterProfileSpec](#clusterprofilespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ |  |  |  |


#### NamespaceMatch



NamespaceMatch selects namespaces by name glob and/or required k8s labels.



_Appears in:_
- [NamespaceRule](#namespacerule)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `namePattern` _string_ | NamePattern is a glob (path.Match syntax, e.g. "openshift-*"). Empty matches any name. |  | Optional: \{\} <br /> |
| `labels` _object (keys:string, values:string)_ | Labels that must all be present on the namespace (subset match). |  | Optional: \{\} <br /> |


#### NamespaceRule



NamespaceRule maps matching namespaces to a desired CWP configuration.



_Appears in:_
- [ClusterProfileSpec](#clusterprofilespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `match` _[NamespaceMatch](#namespacematch)_ |  |  |  |
| `managed` _boolean_ | Managed marks the namespace's CWP as PCE-managed. |  |  |
| `assignLabels` _object (keys:string, values:[LabelAssignment](#labelassignment))_ | AssignLabels maps Illumio label keys (role/app/env/loc/custom) to values. |  | Optional: \{\} <br /> |
| `enforcementMode` _string_ | EnforcementMode for the namespace. One of idle, visibility_only, full. |  | Enum: [idle visibility_only full] <br />Optional: \{\} <br /> |


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


#### SegmentationIntent



SegmentationIntent is an app team's Illumio allow-list for their namespace.



_Appears in:_
- [SegmentationIntentList](#segmentationintentlist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `microsegment.io/v1alpha1` | | |
| `kind` _string_ | `SegmentationIntent` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[SegmentationIntentSpec](#segmentationintentspec)_ |  |  |  |


#### SegmentationIntentList



SegmentationIntentList contains a list of SegmentationIntent.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `microsegment.io/v1alpha1` | | |
| `kind` _string_ | `SegmentationIntentList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[SegmentationIntent](#segmentationintent) array_ |  |  |  |


#### SegmentationIntentSpec



SegmentationIntentSpec is an app team's allow-list for their namespace's app.



_Appears in:_
- [SegmentationIntent](#segmentationintent)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `allow` _[IntentAllow](#intentallow) array_ | Allow is the set of permitted inbound flows to this namespace's app. |  |  |




#### SystemNamespacesSpec



SystemNamespacesSpec is a convenience to manage the cluster's system
namespaces (OpenShift/Kubernetes) out of the box. SystemNamespaces takes
precedence over NamespaceRules for namespaces that match the system patterns.



_Appears in:_
- [ClusterProfileSpec](#clusterprofilespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `manage` _boolean_ | Manage turns on management of system namespaces. |  | Optional: \{\} <br /> |
| `patterns` _string array_ | Patterns of system namespace name globs. Defaults (when empty) to:<br />openshift-*, kube-*, default. |  | Optional: \{\} <br /> |
| `labels` _object (keys:string, values:string)_ | Labels assigned to system-namespace CWPs. |  | Optional: \{\} <br /> |
| `enforcementMode` _string_ | EnforcementMode for system namespaces. Defaults to idle. |  | Enum: [idle visibility_only full] <br />Optional: \{\} <br /> |


