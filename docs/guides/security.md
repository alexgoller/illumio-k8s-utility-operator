# Security & PCE credential hardening

This guide is a security review of the operator and a hardening playbook — with a focus on
**protecting the PCE API keys** and running **one key per cluster**. It reflects the operator as
shipped; where something is a recommendation or a known gap, it says so.

## What's sensitive (trust boundaries)

| Asset | Where | Why it matters |
|---|---|---|
| **PCE API key + secret** | k8s Secret referenced by `PCEConnection.credentialsSecretRef` (keys `api_key` / `api_secret`) | Full API access to your PCE org, at that key's permission scope. The crown jewels. |
| **Onboarding output Secret** (`illumio-cluster-creds`) | operator namespace | Holds `cluster_token` (the C-VEN pairing token) + `cluster_code`. A pairing token lets a workload pair to your cluster's Container Cluster. |
| **CR status data** | etcd | `PolicyInsight` / `RuleView` status reveal your policy and observed flows. |

Trust boundaries: **app teams** author *namespaced* CRs (`SegmentationIntent`, `SegmentationPolicy`,
`PolicyInsight`, `RuleView`) that only affect their own namespace. **Cluster admins** own the
*cluster-scoped* `PCEConnection` and `ClusterProfile` — these hold the PCE connection and are
powerful. Split them with Kubernetes RBAC (see [Kubernetes RBAC](#kubernetes-rbac-who-can-do-what)).

---

## Protecting the PCE API keys

### 1. One key per cluster (do this)

Give **each cluster its own PCE API key** (ideally its own PCE **service account**), never a shared
org-wide key. Why:

- **Blast radius.** A compromised cluster leaks only *its* key. You revoke/rotate that one key without
  touching any other cluster.
- **Attribution.** Every PCE change the operator makes is attributable to that cluster's key in the
  PCE audit log.
- **Least privilege per environment.** Prod and dev clusters can hold keys with different scopes.

Each cluster runs its own `PCEConnection` pointing at its own credentials Secret:

```yaml
apiVersion: microsegment.io/v1alpha1
kind: PCEConnection
metadata: { name: default }
spec:
  pceUrl: mypce.example.com:8443
  orgId: 3
  credentialsSecretRef:
    name: illumio-pce-api       # this cluster's key only
    namespace: illumio-operator
```

### 2. Least-privilege PCE service account

Scope the key in the PCE to only what the operator uses. Depending on which features you enable, it
needs: **labels** (read + create), **container clusters** and **pairing profiles** (read + create —
create only in `onboarding.mode: create`), **rulesets / rules** (CRUD) and **provisioning**,
**container workload profiles** (read + update), and **read** for **traffic (Explorer)** and **rule
search** (for `PolicyInsight` / `RuleView`). Don't grant org-admin. In **`onboarding.mode: adopt`**
the key needs no cluster/pairing *create* permission at all.

### 3. Never commit the key; bring your own Secret

The chart can take the key two ways:

- ❌ `--set pce.apiKey=… --set pce.apiSecret=…` renders a chart-managed Secret (`illumio-pce-api`).
  The value then lives in your Helm release/history and anywhere you stored the command. **Avoid for
  anything but a throwaway test.**
- ✅ `--set pce.existingSecret=<name>` — you create the Secret out-of-band with a **secret manager**
  (HashiCorp Vault + Vault Agent / CSI, External Secrets Operator, Sealed Secrets, SOPS). The key
  never touches Git or Helm values.

```bash
helm upgrade illumio-operator oci://ghcr.io/alexgoller/charts/illumio-k8s-utility-operator \
  --version 0.1.24 -n illumio-operator \
  --set pce.url=mypce.example.com:8443 --set pce.orgId=3 \
  --set pce.existingSecret=illumio-pce-api      # Secret managed by Vault/ESO, keys api_key/api_secret
```

### 4. Rotation is seamless

The operator **reads the credentials Secret on every reconcile** — it does not cache the key. So
**rotating a key is just updating the Secret**: point the PCE service account at a new key, write the
new `api_key`/`api_secret` into the Secret (your secret manager can do this automatically), and the
operator picks it up on the next reconcile. No restart, no reinstall. Rotate on a schedule and
immediately on suspected compromise.

### 5. Restrict who can read the Secret

- Keep the credentials Secret in the **operator's namespace** and lock that namespace down.
- Grant `get`/`list` on Secrets in that namespace to **only** the operator's ServiceAccount and
  cluster admins — not app teams. The operator ships an option to scope its own RBAC to a single
  namespace: `--set rbac.namespaced=true` (see the caveat in [Operator RBAC](#operator-rbac-least-privilege)).
- Enable **etcd encryption at rest** so the key isn't stored in plaintext in etcd (this protects the
  onboarding token and CR status too).

---

## The onboarding output Secret

In `onboarding.mode: create`, the operator writes `illumio-cluster-creds` (`pce_url`, `cluster_id`,
`cluster_token`, `cluster_code`). The **`cluster_token`** is a pairing secret — treat it like the API
key: keep it in the operator namespace, restrict read access, and encrypt etcd at rest. It is
consumed by the separate C-VEN/Kubelink Helm release. **`onboarding.mode: adopt` writes no output
Secret at all** — one fewer secret to protect on already-paired clusters.

---

## Operator RBAC (least privilege)

The operator's Kubernetes permissions are rendered by the chart as either a **`ClusterRole`**
(default) or a namespace-scoped **`Role`** via `--set rbac.namespaced=true`.

- Default (**ClusterRole**) grants, among the CRDs, `secrets: get/list/create/patch/update` — which is
  **cluster-wide** Secret access. The operator only genuinely needs the PCE credentials Secret and its
  own output Secret, so cluster-wide `list` on Secrets is broader than required.
- **Recommendation:** if you keep the PCE credentials + output Secret in the operator namespace, run
  with `rbac.namespaced=true` to confine Secret (and other) access to that namespace.
- **Caveat:** namespace/CWP labeling lists *all* namespaces (a cluster-scoped resource), so a fully
  namespaced role limits day-2 namespace management. Pick the mode that matches how you use the
  operator: onboarding/policy-only workloads can run namespaced; full CWP labeling needs cluster read
  on namespaces.
- The operator does **not** need `delete` or `watch` on Secrets. Review the rendered role
  (`kubectl get clusterrole <release>-manager-role -o yaml`) against your policy.

---

## Kubernetes RBAC: who can do what

Because a `PCEConnection` / `ClusterProfile` effectively wields your PCE key, restrict them:

- **Cluster admins only** may `create`/`update` `pceconnections` and `clusterprofiles`
  (cluster-scoped). Bind these verbs to an admin group, not to developers.
- **App teams** get `create`/`update` on the namespaced CRs (`segmentationintents`,
  `segmentationpolicies`, `policyinsights`, `ruleviews`) **in their own namespaces** — self-service
  segmentation without any PCE credential access. A namespaced `RoleBinding` per team achieves this.
- `PolicyInsight` and `RuleView` are read-only against the PCE, but their *results* (flows, rules) can
  be sensitive — treat read access to those CRs as you would any policy-visibility grant.

---

## Transport security (operator → PCE)

- The operator talks to the PCE over **HTTPS with standard TLS verification** — there is **no
  insecure / skip-verify option**. Connections to a PCE with an untrusted cert simply fail (fail
  closed).
- **Private/internal PCE CA:** the operator trusts the CA bundle in its container image (system
  roots). There is currently **no setting to inject a custom CA bundle**, so a PCE fronted by a
  private CA requires that CA to be trusted by the image (e.g. a mounted CA bundle / custom image).
  This is a known gap — track it if your PCE uses an internal CA.
- The PCE API key is sent as HTTP Basic auth **over that TLS channel** (the standard Illumio scheme).
- The operator **does not log** the API key, secret, or pairing token.

## Metrics & webhooks

- The metrics endpoint is served **securely (HTTPS + authn/authz) by default** (`--metrics-secure`);
  keep it on. Restrict scrape access to your monitoring stack.
- The operator ships **no admission webhooks**, so there is no webhook TLS surface to manage.

## Supply chain

- Images are published to **`ghcr.io/alexgoller/illumio-k8s-utility-operator`** and the chart to
  `ghcr.io/alexgoller/charts`. For production, **pin the image by digest** (not just a tag) and, if
  your policy requires it, verify provenance/signatures before admitting the image.

---

## Hardening checklist

- [ ] **One PCE key per cluster**, each a least-privilege PCE service account.
- [ ] Key delivered via **`pce.existingSecret`** from a secret manager — never `--set` in committed values.
- [ ] Credentials + output Secrets live in the **operator namespace**, read-restricted by RBAC.
- [ ] **etcd encryption at rest** enabled.
- [ ] Key **rotation** automated (operator picks up Secret changes with no restart).
- [ ] `rbac.namespaced=true` where your usage allows; otherwise the rendered ClusterRole reviewed.
- [ ] `pceconnections` / `clusterprofiles` restricted to **cluster admins**; namespaced CRs to app teams per-namespace.
- [ ] Metrics endpoint left **secure**; scrape access restricted.
- [ ] Images **pinned by digest** for production.
- [ ] Private-CA PCE: CA trusted by the operator image (no in-chart custom-CA setting yet).
