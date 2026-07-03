# CNI-enforced policy from Illumio intent — idea draft (napkin)

> **Status: IDEA / DRAFT — not an approved design, not scoped for build.** Written on paper to
> think through the enforcement posture and a possible MVP. Nothing here is committed.

## The one-liner

Keep Illumio's label model and visibility, but **move enforcement off the C-VEN's host iptables
onto the cluster's CNI** — translate Illumio segmentation intent into Kubernetes `NetworkPolicy`
(eventually CNI-native policy) that the CNI enforces.

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

### The honest gap in this posture

Turning the C-VEN to `visibility_only` also turns off the C-VEN's **cross-domain** enforcement —
traffic to/from **other clusters or VMs**, which `NetworkPolicy` fundamentally cannot express
(NP is cluster-local, label/IP-only). So:

- **Intra-cluster** segmentation: fully served by the CNI. ✅
- **Cross-cluster / workload-to-VM** segmentation: **unenforced** once iptables is off, because NP
  can't do it. ⚠️

That's the core tension to be honest about: customers who reject iptables enforcement are implicitly
accepting that cross-domain enforcement into/out of the cluster is no longer handled at the node.
For a *pure intra-cluster* east-west use case this is fine. For hybrid (k8s↔VM) segmentation it's a
real limitation, not a bug we can code around. Options later: emit `ipBlock` NP for known VM CIDRs
(brittle), or keep C-VEN enforcing *only* the cross-domain slice (a split model — much more complex).

## Possible MVP (Q3) — smallest useful, honest slice

Pair the posture above with the **narrowest** first cut:

- **Intra-cluster, ingress, standard `NetworkPolicy`** (portable across Calico/Cilium; no CNI CRDs yet).
- Compile from the existing **`SegmentationPolicy`** CRD — it's already NetworkPolicy-shaped, so the
  translation is close to 1:1 for the in-cluster cases.
- **Off-cluster / cross-cluster peers: skipped and reported**, not silently dropped — surface them in
  CR status so the user knows what the CNI can't cover.
- **Preflight-gated rollout.** Emitting an NP that selects pods flips them to default-deny for that
  direction — so we *must* run a `PolicyInsight` preflight first and only emit once it shows no
  legit traffic would break. This reuses the preflight we just built and is the safety story.
- Operator **owns** the emitted NP objects (labels/annotations), reconciles them, and garbage-collects
  on CR delete. Idempotent, drift-correcting.

Explicit non-goals for the MVP: egress, CNI-specific policy (Cilium/Calico CRDs, L7, FQDN), audit
mode, cross-domain, and any change to how intent is authored.

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

## Bottom line

Feasible and compelling for the **intra-cluster, no-iptables** use case — which is exactly the
customer objection driving it. The hard parts are the **label mapping** and being honest that
**cross-domain enforcement doesn't survive turning iptables off**. Everything else leans on machinery
we already have (CWP enforcement control, the NetworkPolicy-shaped CRD, preflight). Worth a real
design + spike **if** the label-mapping question checks out on actual clusters.
