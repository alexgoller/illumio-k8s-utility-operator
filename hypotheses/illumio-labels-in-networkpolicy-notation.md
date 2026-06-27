# Hypothesis: referencing Illumio labels inside a NetworkPolicy-like statement

> ⚠️ **HYPOTHESIS / DESIGN EXPLORATION ONLY.**
> This is a thought experiment. It is **NOT** part of the illumio-k8s-utility-operator,
> **NOT** a spec, and is **NOT to be implemented**. It explores notation options and
> their compatibility for a separate, hypothetical idea. Nothing here ships.

## The question

> "I have a Kubernetes `NetworkPolicy`-like statement, but I want to reference **Illumio
> labels** in it. What notation would I use to reference the Illumio labels — which is
> probably not compatible with that API?"

You're right to suspect incompatibility. The crux: the upstream `networking.k8s.io/v1`
`NetworkPolicy` is a **closed, versioned schema** owned by Kubernetes. You cannot add
fields to it, and you cannot make a CNI interpret a foreign identity system. So the
question splits into two very different things:

1. **Syntactic** compatibility — will the API server *accept* the YAML? (Sometimes yes.)
2. **Semantic** compatibility — will it *mean* what you want when a CNI enforces it? (No.)

The only extension surfaces a real `NetworkPolicy` exposes are **label keys** (which accept
DNS-subdomain prefixes) and **`metadata.annotations`**. Everything else is fixed. That gives
exactly three notations, two "in-place" and one honest.

---

## Notation 1 — reserved label-domain in the selectors (most faithful in-place form)

Carry the Illumio label reference in `matchLabels`, using a reserved key **prefix** to mark
it as "an Illumio label, not a k8s pod label":

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: payments-ingress
  namespace: payments
spec:
  podSelector: {}                     # the whole namespace's app
  policyTypes: [Ingress]
  ingress:
    - from:
        - podSelector:
            matchLabels:
              illumio.label/app: checkout   # <prefix>/<illumio-label-key>: <value>
              illumio.label/env: prod
      ports:
        - { protocol: TCP, port: 8443 }
```

**Notation rule:** a key under the reserved DNS-subdomain prefix `illumio.label/` is read as an
Illumio label — the segment after the slash is the Illumio label **key** (`role`/`app`/`env`/`loc`
or a custom type), the value is the Illumio label **value**. Keys *without* that prefix remain
ordinary k8s pod labels.

**Semantics (to match both NetworkPolicy and Illumio):**
- Multiple labels **within one peer** are **AND**-ed (`app=checkout` AND `env=prod`) → one Illumio
  rule actor carrying both labels.
- Multiple peers in `from[]` are **OR**-ed → one Illumio rule per peer.

**Why it's syntactically valid:** `illumio.label/app` is a legal label key (a DNS-subdomain
prefix `illumio.label` + name `app`), and `checkout` is a legal value. The API server validates it.

**Why it's semantically incompatible (the answer to your question):** if any CNI
(Calico, Cilium, kube-router, …) actually enforces this `NetworkPolicy`, it will try to select
**pods that literally carry the label `illumio.label/app=checkout`** — which they don't. So the
policy selects nothing and is **inert** — or worse, an empty/over-broad result combined with
NetworkPolicy's "select-a-pod ⇒ default-deny that direction" can isolate the *wrong* pods. The
statement therefore only means anything to a bespoke Illumio controller that **re-interprets**
the prefixed keys; to the rest of the cluster it is a NetworkPolicy that *looks* enforced but isn't.
That is exactly the kind of silent firewall mistranslation you want to avoid.

---

## Notation 2 — annotations carrying the references

Keep the `NetworkPolicy` schema pristine and put the Illumio references in annotations:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: payments-ingress
  namespace: payments
  annotations:
    illumio.com/ingress.0.from: "app=checkout,env=prod"
    illumio.com/ingress.0.ports: "tcp/8443"
spec:
  podSelector: {}
  policyTypes: [Ingress]
  ingress: []                          # left empty so no CNI acts on it
```

**Trade-offs:** maximally "compatible" (the schema is untouched), but the references are
**stringly-typed**, get **no schema validation**, and are **brittle** — the `ingress.0` index has to
stay in lockstep with a `spec.ingress` you've deliberately emptied. And an empty `spec.ingress`
with `policyTypes: [Ingress]` is itself a *default-deny-ingress* NetworkPolicy under a CNI, so you
must be sure no CNI is enforcing it. This is the "works on paper, fragile in practice" path.

---

## Notation 3 — your own Kind (the honest answer)

Because the real type is closed and the in-place hacks are semantically dangerous, the clean way
to "reference Illumio labels in a NetworkPolicy-like statement" is to stop pretending it's a
`NetworkPolicy` and define your **own Kind** that reuses the NetworkPolicy *shape* but gives the
peer a typed Illumio-label field:

```yaml
apiVersion: example.io/v1alpha1
kind: SegmentationPolicy                # NOT networking.k8s.io
spec:
  ingress:
    - from:
        - illumioLabels: { app: checkout, env: prod }   # typed, schema-validated
      ports:
        - { port: 8443, protocol: TCP }
```

- **Schema-validated** (you own the CRD), so `illumioLabels` is a first-class, documented field.
- **No CNI ambiguity** — it is plainly not an enforced `NetworkPolicy`, so nobody mistakes it for
  one; a controller is unambiguously required to give it meaning.
- "NetworkPolicy-like" in **shape and mental model**, not in **API**. This is the move that removes
  the footgun.

(For the record: the `illumio-k8s-utility-operator` this hypothesis sits next to took essentially
this path for its `SegmentationPolicy` CRD — a NetworkPolicy-shaped own-Kind whose peers carry
label selectors the operator resolves to Illumio labels. That's the practical resolution of this
exact tension.)

A variant of Notation 3 worth noting: a CRD peer could accept **both** a k8s `podSelector` and an
`illumioLabels` map, disambiguated structurally (different fields), so a single document can mix
"select these pods (mapped via a LabelMap)" and "reference this Illumio identity directly."

---

## Compatibility matrix

| Notation | Valid against the NetworkPolicy schema? | Survives CNI enforcement as intended? | Schema-validated reference? | Footgun risk |
|---|---|---|---|---|
| 1 — reserved label-domain in `matchLabels` | Yes (prefixed keys are legal) | **No** — CNI selects literal `illumio.label/...` pods → inert/misleading | Partial (key-syntax only) | **High** |
| 2 — annotations | Yes (schema untouched) | N/A (you keep `spec.ingress` inert) | **No** (free-form strings) | Medium |
| 3 — own Kind (`SegmentationPolicy`) | N/A (different `apiVersion`/`kind`) | N/A (not a NetworkPolicy) | **Yes** | **Low** |

**Bottom line:** inside a *literal* `NetworkPolicy`, Notation 1 is the most faithful *notation* —
`illumio.label/<type>: <value>` in `matchLabels` — but it is only **syntactically** compatible; it is
**semantically incompatible** with the NetworkPolicy API the moment a CNI enforces it. If the goal is
a real, safe resource, Notation 3 (own Kind) is the correct shape. Your instinct ("probably not
compatible with that API") is exactly right for Notations 1 and 2.

---

## Open questions / subtleties (if anyone ever revisited this)

- **`namespaceSelector`:** Illumio has no policy-actor "namespace" dimension separate from labels
  (a k8s namespace ≈ a Container Workload Profile). You'd either fold `namespaceSelector` matchLabels
  into the same Illumio label set or reject it — there's no faithful 1:1.
- **`matchExpressions`:** `In` with a single value maps to a label; `In` with several values smells
  like an Illumio **label group**; `NotIn`/`Exists`/`DoesNotExist` have no clean Illumio rule-actor
  equivalent (Illumio ANDs labels within an actor and uses separate exclusion actors). Safest to
  support `matchLabels` only and reject `matchExpressions`.
- **Label groups / IP lists:** extend the reserved key namespace, e.g. `illumio.labelgroup/<name>`
  or `illumio.iplist/<name>`, to reference non-label actors.
- **The protected side:** to protect *by Illumio labels* rather than "the namespace's own app," apply
  the same reserved-domain notation to `spec.podSelector` — but beware that this widens the
  "who does this policy protect" question and the same CNI-inertness applies.
- **Default-deny vs enforcement mode:** NetworkPolicy semantics are "selecting a pod isolates that
  direction." Illumio blocks only in `full` enforcement. So even with a perfect notation, the *posture*
  differs — a translated policy does nothing until enforcement is turned on, which surprises people who
  expect NetworkPolicy's immediate isolation.
- **Coexistence / iptables:** if you ever did emit Notation 1/2 as a real `NetworkPolicy`, you'd need
  to ensure the CNI's NetworkPolicy enforcement is disabled (or scoped) so the inert policy can't
  mislead, and you'd hit the usual C-VEN-vs-CNI iptables ownership concerns.

---

## Implementation mechanism — CRD (controller) or mutating webhook?

**Primary mechanism: a CRD reconciled by a controller. A mutating webhook is the *wrong* driver
and fits only as a thin admission-time adjunct.**

The work this idea actually requires is: resolve labels to PCE hrefs, create/update an Illumio
ruleset, **provision** it, surface `workloads_affected`, correct drift, and clean up on delete.
That is slow, retryable, rate-limited (PCE returns 429), eventually-consistent, side-effecting
external I/O with a status surface — i.e. textbook **reconcile-loop** work, not admission work.

### Why a mutating webhook is the wrong tool for the core job

| Constraint | Consequence for this use case |
|---|---|
| Synchronous, time-bounded (default ~10s) | Cannot reliably make PCE REST calls (auth, rulesets, provision, 429 backoff). |
| Sees one object at admission; not level-triggered | No reconciliation, no drift correction, no retry on PCE failure, no eventual consistency. |
| Should be side-effect-free (`sideEffects: None`) | Driving PCE writes from a webhook is an anti-pattern — it breaks dry-run, re-invocation, and idempotency assumptions. |
| `failurePolicy` is a blunt instrument | `Fail` ⇒ if your webhook is down, **all** matching writes (e.g. every `NetworkPolicy`) are blocked clusterwide. `Ignore` ⇒ policies silently slip through untranslated. Both are bad postures for a security feature. |
| No status subresource | Cannot report `Ready`/`Provisioned`/`workloadsAffected`/`Rejected` over time. |

### Where admission control *does* fit (as a complement to the CRD)

- **Fail-fast structural rejection** of the unsupported NetworkPolicy subset (Egress,
  `matchExpressions`, `ipBlock`, non-empty `podSelector`, empty `from`) — so the user gets an error
  at `kubectl apply` instead of discovering a `Ready=False` status later. Prefer a **CEL
  `ValidatingAdmissionPolicy`** (built-in, no webhook server to run/secure/scale) over a custom
  validating webhook where the rule is expressible in CEL; fall back to a validating webhook only
  for logic CEL can't express. Simple shape constraints can even live in the **CRD's OpenAPI /
  `+kubebuilder:validation` markers** with no admission plugin at all.
- **Defaulting** (default `protocol: TCP`, default `policyTypes: [Ingress]`) — covered by CRD
  `+kubebuilder:default` markers; a defaulting/mutating webhook is only needed for defaults that
  depend on other fields.

### The load-bearing split

> **Structural rejections → admission time** (CEL/VAP or validating webhook): pure-function checks on
> the object itself (egress, matchExpressions, ipBlock, empty from).
> **State-dependent rejections → the controller**: anything needing a lookup — "this consumer
> Illumio label doesn't exist in the PCE yet," "this namespace isn't managed," "provisioning failed."
> A webhook cannot do these safely (no external calls, no retries), so they belong in reconcile and
> surface as `Ready=False / Rejected` with a requeue.

### If you instead chose Notation 1 (a *real* `NetworkPolicy` with `illumio.label/` keys)

Then a **mutating webhook becomes relevant for a different, narrower job**: at admission, rewrite the
`illumio.label/<key>: <value>` selector keys into *real* k8s labels (via a LabelMap) so a CNI can
actually enforce them — i.e. translating Illumio notation **into Kubernetes**, not into the PCE. Even
here you still need a **controller** for the PCE-side rulesets, and you inherit all the Notation-1
semantic hazards above. So this is a mutating-webhook-*plus*-controller design, strictly more complex
and more fragile than the own-Kind CRD.

**Conclusion:** CRD + controller does the work; admission control (ideally CEL `ValidatingAdmissionPolicy`)
is an optional fail-fast guard for the structural subset; a mutating webhook is only justified if you
commit to the (discouraged) Notation-1 real-`NetworkPolicy` path, and never as the thing that talks to
the PCE.

---

## Verdict

- **Most faithful in-place notation:** `illumio.label/<illumio-key>: <value>` inside `matchLabels`
  (Notation 1). Looks native; **not** compatible with the NetworkPolicy *semantics*.
- **Safest / recommended shape:** an own-Kind, NetworkPolicy-shaped CRD with a typed `illumioLabels`
  peer field (Notation 3).
- The incompatibility you sensed is real and is **semantic**, not syntactic: the NetworkPolicy schema
  will *accept* prefixed labels, but the NetworkPolicy *API contract* (CNI selects pods by those
  labels) cannot carry an Illumio identity reference.

*End of hypothesis. Not for implementation.*
