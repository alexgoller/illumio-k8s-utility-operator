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
- [PolicyInsight](#policyinsight)
- [PolicyInsightList](#policyinsightlist)
- [SegmentationIntent](#segmentationintent)
- [SegmentationIntentList](#segmentationintentlist)
- [SegmentationPolicy](#segmentationpolicy)
- [SegmentationPolicyList](#segmentationpolicylist)



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
| `policyScopeLabels` _string array_ | PolicyScopeLabels limits which of a namespace's assigned Illumio labels<br />form the per-namespace ruleset scope. An Illumio ruleset scope may be any<br />number of labels; for Kubernetes namespaces app+env is the right scope<br />almost always, and loc is a poor scope choice. When empty, the scope<br />defaults to app+env. Labels assigned to the namespace CWP that are not in<br />this set (e.g. loc) are still applied to workloads for visibility, but do<br />not become part of the ruleset scope. |  | Optional: \{\} <br /> |
| `unknownLabelMode` _string_ | UnknownLabelMode is the default policy when a referenced Illumio label is<br />not yet in the PCE: strict (reject), skip (omit the actor/rule and report),<br />or create (mint role/app/env/loc labels). Overridable per-namespace and<br />per-CR via the microsegment.io/unknown-label-mode annotation. Defaults to strict. |  | Enum: [strict skip create] <br />Optional: \{\} <br /> |
| `systemNamespaces` _[SystemNamespacesSpec](#systemnamespacesspec)_ | SystemNamespaces manages OpenShift/Kubernetes system namespaces out of the box. |  | Optional: \{\} <br /> |
| `namespaceRules` _[NamespaceRule](#namespacerule) array_ | NamespaceRules are evaluated in order; the first match wins. For namespaces<br />that match the SystemNamespaces patterns, SystemNamespaces takes precedence<br />and overrides any matching NamespaceRule. For all other namespaces,<br />the first matching NamespaceRule governs. |  | Optional: \{\} <br /> |




#### FlowFinding



FlowFinding is one observed flow the current draft policy would block.



_Appears in:_
- [PolicyInsightStatus](#policyinsightstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `peer` _object (keys:string, values:string)_ | Peer is the Illumio labels of the other end (consumer for inbound, provider<br />for egress). May be empty for an unlabeled / off-cluster peer (see PeerIP). |  | Optional: \{\} <br /> |
| `peerIP` _string_ | PeerIP is the other end's IP when it has no workload/labels (e.g. off-cluster). |  | Optional: \{\} <br /> |
| `port` _integer_ | Port is the destination port of the flow. |  |  |
| `protocol` _string_ | Protocol is TCP or UDP. |  | Optional: \{\} <br /> |
| `connections` _integer_ | Connections is the observed connection count over the window. |  | Optional: \{\} <br /> |
| `decision` _string_ | Decision is the draft policy decision that flagged this flow<br />(blocked or potentially_blocked). |  | Optional: \{\} <br /> |
| `lastDetected` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#time-v1-meta)_ | LastDetected is when the flow was last observed. |  | Optional: \{\} <br /> |


#### IngressRule



IngressRule allows traffic from the listed peers on the listed ports.



_Appears in:_
- [SegmentationPolicySpec](#segmentationpolicyspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `from` _[NetworkPolicyPeer](#networkpolicypeer) array_ |  |  |  |
| `ports` _[NetworkPolicyPort](#networkpolicyport) array_ |  |  |  |


#### IntentAllow



IntentAllow allows a consumer to reach this namespace's app on the given ports.
The consumer is EITHER From (a cross-app consumer, extra-scope) OR
FromIntraNamespace (a consumer within this namespace, intra-scope) — set one.



_Appears in:_
- [SegmentationIntentSpec](#segmentationintentspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `from` _object (keys:string, values:string)_ | From is a cross-app consumer's Illumio labels (key -> value), e.g.<br />\{"app":"checkout","env":"prod"\}. They must already exist in the PCE.<br />This is an extra-scope source (it may live in any app). |  | Optional: \{\} <br /> |
| `fromIntraNamespace` _object (keys:string, values:string)_ | FromIntraNamespace is a consumer WITHIN this namespace's scope, narrowed by<br />Illumio labels (typically \{"role":"frontend"\}). This is an intra-scope source<br />(same app), so the scope is not repeated. Empty map = all workloads in scope. |  | Optional: \{\} <br /> |
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


#### NetworkPolicyPeer



NetworkPolicyPeer is a consumer selector (a supported subset of k8s NetworkPolicyPeer).



_Appears in:_
- [IngressRule](#ingressrule)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `podSelector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#labelselector-v1-meta)_ |  |  | Optional: \{\} <br /> |
| `namespaceSelector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#labelselector-v1-meta)_ |  |  | Optional: \{\} <br /> |


#### NetworkPolicyPort



NetworkPolicyPort is a port/protocol.



_Appears in:_
- [IngressRule](#ingressrule)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `port` _integer_ |  |  |  |
| `protocol` _string_ |  | TCP | Enum: [TCP UDP] <br />Optional: \{\} <br /> |


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


#### ObservationWindow



ObservationWindow is the time range the preflight analyzed.



_Appears in:_
- [PolicyInsightStatus](#policyinsightstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `from` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#time-v1-meta)_ |  |  | Optional: \{\} <br /> |
| `to` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#time-v1-meta)_ |  |  | Optional: \{\} <br /> |


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




#### PolicyInsight



PolicyInsight is an on-request what-if preflight for a namespace: it reports
the flows the current draft policy would block (inbound) and the egress flows
that are denied, from observed PCE traffic. Read-only — it authors no policy.



_Appears in:_
- [PolicyInsightList](#policyinsightlist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `microsegment.io/v1alpha1` | | |
| `kind` _string_ | `PolicyInsight` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[PolicyInsightSpec](#policyinsightspec)_ |  |  |  |


#### PolicyInsightList



PolicyInsightList contains a list of PolicyInsight.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `microsegment.io/v1alpha1` | | |
| `kind` _string_ | `PolicyInsightList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[PolicyInsight](#policyinsight) array_ |  |  |  |


#### PolicyInsightSpec



PolicyInsightSpec requests a what-if preflight for the namespace it lives in.
The preflight runs ON REQUEST ONLY (on create, on spec change, or when the
microsegment.io/refresh annotation changes) — never on a timer. PCE traffic
queries are expensive; the operator computes once per request and then idles.



_Appears in:_
- [PolicyInsight](#policyinsight)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `lookbackDays` _integer_ | LookbackDays is the observation window (in days, ending now) the preflight<br />queries the PCE for. Defaults to 7. | 7 | Maximum: 90 <br />Minimum: 1 <br />Optional: \{\} <br /> |




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
| `provider` _object (keys:string, values:string)_ | Provider optionally narrows the protected provider to a sub-set of this<br />namespace's app, by Illumio labels (e.g. \{"role":"backend"\}). The labels must<br />already exist in the PCE. Default (empty): the whole namespace app. |  | Optional: \{\} <br /> |
| `allow` _[IntentAllow](#intentallow) array_ | Allow is the set of permitted inbound flows to this namespace's app. |  | Optional: \{\} <br /> |
| `allowIntraNamespace` _boolean_ | AllowIntraNamespace is a shortcut: when true, allow all workloads in this<br />namespace to reach each other on all ports (an intra-scope allow-all). Combine<br />with Allow for finer cross-app rules, or use alone for "allow any-any here". |  | Optional: \{\} <br /> |
| `enforcement` _string_ | Enforcement requests a namespace enforcement mode. The operator applies<br />the strictest mode requested across all policy CRs in the namespace (on<br />top of the admin baseline). One of idle, visibility_only, full. |  | Enum: [idle visibility_only full] <br />Optional: \{\} <br /> |




#### SegmentationPolicy



SegmentationPolicy is a NetworkPolicy-style Illumio allow-list for a namespace.



_Appears in:_
- [SegmentationPolicyList](#segmentationpolicylist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `microsegment.io/v1alpha1` | | |
| `kind` _string_ | `SegmentationPolicy` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[SegmentationPolicySpec](#segmentationpolicyspec)_ |  |  |  |


#### SegmentationPolicyList



SegmentationPolicyList contains a list of SegmentationPolicy.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `microsegment.io/v1alpha1` | | |
| `kind` _string_ | `SegmentationPolicyList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[SegmentationPolicy](#segmentationpolicy) array_ |  |  |  |


#### SegmentationPolicySpec



SegmentationPolicySpec mirrors a supported subset of k8s NetworkPolicy.



_Appears in:_
- [SegmentationPolicy](#segmentationpolicy)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `podSelector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.30/#labelselector-v1-meta)_ | PodSelector selects the pods to which this policy applies. |  | Optional: \{\} <br /> |
| `ingress` _[IngressRule](#ingressrule) array_ | Ingress rules (the only supported direction). |  |  |
| `policyTypes` _string array_ | PolicyTypes must be ["Ingress"]. |  | Optional: \{\} <br /> |
| `enforcement` _string_ | Enforcement requests a namespace enforcement mode (see SegmentationIntent). |  | Enum: [idle visibility_only full] <br />Optional: \{\} <br /> |




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


