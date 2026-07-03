# CNI-enforced policy from Illumio intent — idea draft (napkin)

> **Status: IDEA / DRAFT — not an approved design, not scoped for build.** Written on paper to
> think through the enforcement posture and a possible MVP. Nothing here is committed.

## The one-liner

Keep Illumio's label model and visibility, but **move enforcement off the C-VEN's host iptables
onto the cluster's CNI** — translate Illumio segmentation intent into Kubernetes `NetworkPolicy`
(eventually CNI-native policy) that the CNI enforces — and **bridge pod→VM egress by feeding
Illumio's label-known VM IPs into that NetworkPolicy**.

## Why (the actual motivation)

The C-VEN enforces in-cluster by programming **iptables on the node**. Many Kubernetes customers
**don't want that**:

- It collides conceptually (and sometimes literally) with kube-proxy / the CNI's own dataplane.
- iptables enforcement at scale has performance and operational baggage; eBPF CNIs are the modern norm.
- Platform teams want enforcement in the plane they already run and trust (Calico/Cilium), not a
  second agent rewriting host firewall rules.

So the value isn't "Illumio vs NetworkPolicy" — it's **"author + observe in Illumio, enforce with
your CNI."** Illumio stays the source of policy intent and the flow-visibility brain; the CNI becomes
the dataplane.

## Enforcement posture (Q1) — essentially decided by the motivation

For an **already-onboarded C-VEN cluster**:

- **C-VEN → `visibility_only`** (no iptables enforcement). We *already* control this via the CWP
  enforcement mode — flipping a namespace to `visibility_only` is an existing operator capability.
- **CNI enforces** the translated `NetworkPolicy`.
- Illumio keeps: the label model (app/env/role via CWP/LabelMap), flow visibility (Explorer), and
  policy authorship (SegmentationIntent/Policy). The CNI keeps: packet enforcement.

This is the "CNI instead of C-VEN" posture, and it's the natural fit because we manage CWP
enforcement already. Defense-in-depth (both enforce) is explicitly *not* the goal here — the whole
point is to stop enforcing on iptables.

### The VM ↔ cluster picture — and where we add real value

The "cross-domain gap" is smaller than it first looks, because of how Illumio already handles
VM↔cluster traffic — and one direction of it is where **this operator can contribute unique value**.

**How Illumio does it today (VM side):**

- **VMs run the standard VEN**, which enforces on the VM. For a policy that references a Kubernetes
  namespace (by its Illumio label set), the VM's VEN programs rules against the **cluster node IPs**
  — because from outside, pods in that namespace are reached via the cluster's node IPs.
- **The C-VEN is the reporting tool that publishes the cluster node IPs** into Illumio, so the VM
  VENs learn what the cluster's IPs are. Crucially, this reporting role **survives `visibility_only`**
  — turning off iptables *enforcement* does not turn off *reporting*. So the C-VEN keeps publishing
  node IPs and flows while the CNI takes over enforcement.

**The two directions, and who handles each:**

| Direction | Enforced by | Our operator's role |
|---|---|---|
| **VM → pod** (VM is the source) | the **VM's VEN**, against the cluster node IPs the C-VEN publishes | none — Illumio already does this |
| **pod → VM** (egress from the cluster) | the **CNI**, via `NetworkPolicy` egress | **this is our value-add** ⬇ |

**The value-add (pod → VM egress):** NetworkPolicy egress can't select by Illumio label, but it *can*
use `ipBlock` CIDRs — and **Illumio knows the VM IPs by label**. So the operator can resolve the
intent's egress target (an Illumio label set) to the **actual VM IPs from the PCE** and emit
`NetworkPolicy` **egress `ipBlock`** rules, keeping them in sync as VM IPs change. That lets a pod
egress to "the payments VMs" by label-driven intent, enforced natively by the CNI, with no iptables
on the node.

**The symmetry is the nice part:** the C-VEN publishes **cluster IPs outward** so VM VENs can reach
pods; our operator publishes **VM IPs inward** into the CNI's NetworkPolicy so pods can reach VMs.
Together that covers k8s↔VM east-west without host iptables on the cluster nodes.

### The remaining (smaller) honest gap

With the above, **k8s↔VM is largely covered** (VM→pod by the VM VEN, pod→VM by our `ipBlock` egress).
What's still genuinely hard for NetworkPolicy:

- **Cross-cluster pod↔pod** — the other cluster's pods sit behind *its* node IPs; expressible only as
  brittle node-IP `ipBlock`s. Defer.
- **Truly external / non-Illumio peers** — no label source, no known IPs. Out of scope.

## Possible MVP (Q3) — smallest useful, honest slice

Two capabilities, both from the existing **`SegmentationPolicy`** CRD (already NetworkPolicy-shaped):

1. **Intra-cluster east-west (ingress), standard `NetworkPolicy`** — table stakes; any NP generator
   does this. Translation is close to 1:1 for in-cluster cases.
2. **pod → VM egress via label-resolved `ipBlock`** — the **differentiator**. Resolve the intent's
   egress target (Illumio label set) to the VMs' actual IPs *from the PCE*, emit NP egress `ipBlock`
   rules, and keep them current as VM IPs change. This is unique value: nobody else knows the VM IPs
   by label and keeps them fresh in the cluster's NetworkPolicy.

Shared plumbing for both:

- **Off-cluster peers we can't resolve: skipped and reported** in CR status — never silently dropped.
- **Preflight-gated rollout.** Emitting an NP that selects pods flips them to default-deny for that
  direction — so we *must* run a `PolicyInsight` preflight first and only emit once it shows no legit
  traffic would break. Reuses the preflight we already built; it's the safety story.
- Operator **owns** the emitted NP objects (labels/annotations), reconciles them, and garbage-collects
  on CR delete. Idempotent, drift-correcting. The `ipBlock` sets re-reconcile as PCE workload IPs move.

Explicit non-goals for the MVP: CNI-specific policy (Cilium/Calico CRDs, L7, FQDN), audit mode,
cross-*cluster* pod↔pod, and any change to how intent is authored. (Egress is **in** scope now — but
only the VM-`ipBlock` form above, not general egress.)

## How it fits what already exists

- **`SegmentationPolicy` CRD** (NetworkPolicy-shaped) is the natural front-end — likely a
  backend selector (`enforcement.backend: illumio | networkPolicy`) rather than a new CRD.
- **CWP enforcement control** already lets us set `visibility_only` — the posture flip is not new code.
- **`PolicyInsight` preflight** is the de-risking mechanism for turning NP on.
- **The label problem is shared** with preflight: NP needs *k8s* pod/namespace labels, while intent
  is in *Illumio* labels. This is the make-or-break and stays the top open question.

## Open questions (unresolved on purpose)

1. **Label mapping (make-or-break):** how do Illumio labels (app/env/role) become k8s `podSelector` /
   `namespaceSelector` matches? Do real workloads carry equivalent k8s labels, or do we need a
   configurable Illumio→k8s label map? Needs a look at real clusters before any build.
2. **Cross-domain gap:** accept intra-cluster-only, or design a split model where the C-VEN still
   enforces the cross-domain slice?
3. **CNI target:** standard `NetworkPolicy` only (portable) vs Cilium/Calico CRDs (richer, eBPF, audit
   mode, egress/FQDN/L7)?
4. **Who owns the posture flip:** does the operator set the C-VEN to `visibility_only`, or is that an
   admin decision it only *reports* on?
5. **Trust / correctness:** if the CNI enforces and Illumio only observes, how do we reconcile
   Illumio's draft policy view with what the CNI actually enforces? (Two policy planes, one truth.)
6. **VM-IP churn & scale (for the `ipBlock` egress path):** how often do the target VM IPs move, and
   how fast must we re-reconcile the NP `ipBlock` sets? Can we aggregate into CIDRs, and what are the
   practical `ipBlock` count limits per NetworkPolicy on the target CNIs?
7. **Source of VM IPs:** query the PCE for workloads by the intent's egress label set → their IPs.
   Confirm the PCE API returns current interface IPs for VEN-managed VMs filtered by label.

## Bottom line

Feasible and compelling for the **no-iptables** use case that's driving it. Two payoffs: intra-cluster
east-west via NetworkPolicy (table stakes), and — the real differentiator — **pod→VM egress by
resolving Illumio's label-known VM IPs into NP `ipBlock` and keeping them fresh**. That closes most of
the k8s↔VM story (VM→pod is already handled by the VM VEN against C-VEN-published node IPs), leaving
only cross-cluster/external as genuine gaps. The hard part remains the **label mapping** for the
in-cluster selectors. Everything else leans on machinery we already have (CWP enforcement control, the
NetworkPolicy-shaped CRD, preflight, the PCE client). Worth a real design + spike **if** the
label-mapping checks out on actual clusters — and the VM-`ipBlock` egress bridge is the piece worth
prototyping first, since it's the unique value.
