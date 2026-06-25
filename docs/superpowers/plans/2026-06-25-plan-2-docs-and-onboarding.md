# Illumio K8s Utility Operator — Plan 2: Documentation Infrastructure + Cluster Onboarding

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up first-class project documentation (in-repo Markdown that also builds a published MkDocs Material site, plus auto-generated CRD API reference) and implement cluster **onboarding**: a `ClusterProfile` CRD whose controller ensures the Illumio **Container Cluster** and a **node Pairing Profile** (with the right node labels, or a reused existing profile) exist in the PCE and publishes the resulting credentials as a Kubernetes Secret for the Helm-deployed agents to consume. Make installation as easy as possible via an **operator Helm chart** that renders the PCE Secret + `PCEConnection` (+ optional `ClusterProfile`) from values, so a full install is one command.

**Architecture:** Extends the Plan 1 operator. The `internal/pce` package gains container-cluster and pairing-profile clients (same HTTP/auth/ownership conventions as Plan 1). A new cluster-scoped `ClusterProfile` CRD drives an onboarding reconciler that talks to the PCE via an injectable client interface (testable under envtest with a fake), captures the one-time container-cluster token, generates a pairing key, and writes `pce_url`/`cluster_id`/`cluster_token`/`cluster_code` into an output Secret. Documentation is a first-class deliverable: a `docs/` Markdown tree builds a MkDocs site published to GitHub Pages, and a CRD API reference is generated from the Go types.

**Tech Stack:** Go, kubebuilder v4 + controller-runtime, envtest, `net/http/httptest`; MkDocs Material (Python) for the docs site; `github.com/elastic/crd-ref-docs` for CRD reference; GitHub Actions for docs build/deploy.

## Global Constraints

- **Module path:** `github.com/alexgoller/illumio-k8s-utility-operator`.
- **API group:** `microsegment.io`; **API version:** `v1alpha1`.
- **CRD conventions:** every CRD registers category `illumio` and a shortName; `ClusterProfile` is **cluster-scoped** (shortName `cprof`).
- **Target platform:** Illumio Core for Kubernetes in **CLAS** mode, **PCE 24.5+**.
- **PCE API base:** requests target `https://<pceUrl>/api/v2<path>`; `href` values returned by the PCE omit the `/api/v2` prefix (so an href like `/orgs/1/pairing_profiles/5` is passed straight to the client's `do` as the path).
- **Ownership tagging:** every PCE object the operator *creates* carries `external_data_set` (from `PCEConnection.spec.externalDataSet`, default `illumio-operator`) and `external_data_reference` (owning CR UID).
- **Onboarding facts (verified against PCE 24.5 REST API / illumio-py / terraform-provider):**
  - `POST /orgs/{org}/container_clusters` body `{name, description?}` → response includes `href` and the **one-time** `container_cluster_token`. The cluster UUID is the last path segment of `href`. The token is returned **only at creation** — persist immediately.
  - `POST /orgs/{org}/pairing_profiles` body requires `enabled` (bool); other fields `name, description, enforcement_mode, visibility_level, allowed_uses_per_key, key_lifespan, labels` → response includes `href`. The `labels` field is an array of label-href refs: `"labels": [ { "href": "/orgs/1/labels/224" } ]`.
  - `POST /orgs/{org}/pairing_profiles/{id}/pairing_key` with body `{}` → response field **`activation_code`** (this is the Helm `cluster_code`).
  - None of container clusters, pairing profiles, or pairing keys require `sec_policy` provisioning — they are immediate management-API objects.
  - `enforcement_mode` is a **string enum**; container-supported values are `idle`, `visibility_only`, `full`.
- **Helm handoff:** the operator only *publishes* the output Secret. It never runs the Illumio agent Helm chart. The Secret keys are `pce_url`, `cluster_id`, `cluster_token`, `cluster_code`.
- **Node pairing profile:** the pairing profile + pairing key (`cluster_code`) is what the C-VEN uses to pair the cluster **nodes** as Illumio workloads. The operator must create it with the **right node labels set** (so nodes get a correct Illumio identity), or reuse an **existing** pairing profile named by the admin. Labels are resolved key/value → Illumio label `href` (create-if-missing for admin-defined labels, same as Plan 1's `EnsureLabel`), then sent as `labels: [{href}]`.
- **Easy install (operator Helm chart):** ship a Helm chart for the *operator itself* so installation is one command. From chart values (`pce.url`, `pce.orgId`, `pce.apiKey`, `pce.apiSecret`, or `pce.existingSecret`), the chart renders the credentials Secret + a `PCEConnection` (and, when `onboarding.enabled`, a `ClusterProfile`). `existingSecret` supports externally-managed credentials (Vault/External-Secrets). Also keep `make build-installer` (`dist/install.yaml`) for a plain `kubectl apply` of the operator. PCE vars are supplied as Helm values or via the referenced Secret — never hand-assembled.
- **Docs:** authored as Markdown under `docs/`; the same tree builds a MkDocs Material site deployed to GitHub Pages. `docs/superpowers/**` (specs/plans) is internal and excluded from the published site.
- **Commits:** conventional-commit messages; commit at the end of every task; end messages with the repo's `Co-Authored-By` trailer.

---

### Task 1: Documentation infrastructure + Plan 1 backfill

**Files:**
- Create: `mkdocs.yml`
- Create: `docs/index.md`, `docs/getting-started.md`, `docs/installation.md`, `docs/concepts.md`, `docs/reference/pceconnection.md`, `docs/reference/.gitkeep`
- Create: `docs/crd-ref-docs.yaml` (crd-ref-docs config)
- Create: `requirements-docs.txt`
- Create: `.github/workflows/docs.yml`
- Modify: `Makefile` (add `docs-api`, `docs-build`, `docs-serve` targets)
- Modify: `.gitignore` (add `site/`)

**Interfaces:**
- Consumes: nothing (docs + tooling).
- Produces: a buildable docs site (`make docs-build` → `site/`), a CRD reference generator (`make docs-api`), and a Pages deploy workflow. Later feature tasks add pages under `docs/`.

> **Dispatch note for the controller:** dispatch this task to a documentation-capable subagent (e.g. the `project-marketing-docs` agent). It is content + config, not Go.

- [ ] **Step 1: Add the docs Python requirements**

Create `requirements-docs.txt`:

```
mkdocs-material==9.5.39
mkdocs==1.6.1
```

- [ ] **Step 2: Add the MkDocs config**

Create `mkdocs.yml`:

```yaml
site_name: Illumio K8s Utility Operator
site_description: A Kubernetes operator that automates Illumio PCE-side configuration (onboarding, container workload profiles, segmentation policy).
repo_url: https://github.com/alexgoller/illumio-k8s-utility-operator
repo_name: alexgoller/illumio-k8s-utility-operator
docs_dir: docs
site_dir: site

theme:
  name: material
  features:
    - navigation.sections
    - navigation.top
    - content.code.copy
    - search.suggest
  palette:
    - scheme: default
      primary: indigo
      toggle:
        icon: material/weather-night
        name: Switch to dark mode
    - scheme: slate
      primary: indigo
      toggle:
        icon: material/weather-sunny
        name: Switch to light mode

markdown_extensions:
  - admonition
  - pymdownx.highlight
  - pymdownx.superfences
  - toc:
      permalink: true

# docs/superpowers/** (internal specs & plans) is intentionally excluded.
exclude_docs: |
  superpowers/

nav:
  - Home: index.md
  - Getting Started: getting-started.md
  - Installation: installation.md
  - Concepts: concepts.md
  - API Reference:
      - PCEConnection: reference/pceconnection.md
```

- [ ] **Step 3: Add the crd-ref-docs config**

Create `docs/crd-ref-docs.yaml`:

```yaml
processor:
  ignoreFields:
    - "status$"
    - "TypeMeta$"
render:
  kubernetesVersion: "1.30"
```

- [ ] **Step 4: Write the landing/getting-started/installation/concepts pages**

Create `docs/index.md`:

```markdown
# Illumio K8s Utility Operator

A Kubernetes operator that automates the **PCE-side** of running Illumio Core on Kubernetes/OpenShift — the work normally done by hand in the Illumio console:

- **Onboarding** — register the cluster with the PCE and hand the agent credentials to Helm.
- **Container Workload Profiles** — manage which namespaces are governed and how they are labeled (coming next).
- **Segmentation policy** — let app teams express Illumio policy as Kubernetes resources (coming later).

The operator is a client of the Illumio PCE REST API. It does **not** deploy the C-VEN/Kubelink agents — that stays with the official Helm chart.

See [Getting Started](getting-started.md) and [Concepts](concepts.md).
```

Create `docs/concepts.md`:

```markdown
# Concepts

| Term | Meaning |
|------|---------|
| **PCE** | Illumio Policy Compute Engine — the policy brain the operator talks to over REST API v2. |
| **C-VEN / Kubelink** | The in-cluster Illumio agents, deployed by the official Helm chart (not by this operator). |
| **Container Cluster** | The PCE object representing your Kubernetes cluster. Onboarding creates it. |
| **Pairing Profile / pairing key** | The PCE objects that produce the activation code the C-VEN uses to pair. |
| **Container Workload Profile (CWP)** | Per-namespace policy/label configuration in the PCE (managed in a later release). |

## Custom Resources

| Kind | Scope | Purpose |
|------|-------|---------|
| `PCEConnection` | cluster | Connection + credentials to one PCE. |
| `ClusterProfile` | cluster | Onboards the cluster to the PCE and publishes agent credentials. |

Run `kubectl get illumio` to list all operator resources.
```

Create `docs/getting-started.md`:

```markdown
# Getting Started

## Prerequisites

- A Kubernetes or OpenShift cluster.
- Access to an Illumio PCE (24.5+), with an API key/secret and your org ID.

## 1. Install the operator

See [Installation](installation.md).

## 2. Configure a PCE connection

Create a Secret with your PCE API credentials and a `PCEConnection`:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: illumio-pce-api
  namespace: illumio-operator
type: Opaque
stringData:
  api_key: "api_1234567890abcdef"
  api_secret: "your-api-secret"
---
apiVersion: microsegment.io/v1alpha1
kind: PCEConnection
metadata:
  name: prod-pce
spec:
  pceUrl: mypce.example.com:8443
  orgId: 3
  credentialsSecretRef:
    name: illumio-pce-api
    namespace: illumio-operator
```

Check the connection:

```bash
kubectl get pceconnections
kubectl get pceconnection prod-pce -o jsonpath='{.status.conditions[?(@.type=="Connected")].status}'
```

## 3. Onboard the cluster

(See the Onboarding guide once Plan 2 lands.)
```

Create `docs/installation.md`:

```markdown
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
```

Create `docs/reference/pceconnection.md` documenting the Plan 1 CRD:

```markdown
# PCEConnection

Cluster-scoped. Defines a connection to one Illumio PCE.

## Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `pceUrl` | string | yes | PCE host:port (e.g. `mypce.example.com:8443`; `443` for SaaS). |
| `orgId` | integer | yes | PCE organization ID. |
| `credentialsSecretRef.name` | string | yes | Secret holding `api_key` and `api_secret`. |
| `credentialsSecretRef.namespace` | string | no | Secret namespace (defaults to the operator's namespace). |
| `externalDataSet` | string | no | Ownership tag stamped on PCE objects the operator creates (default `illumio-operator`). |

## Status

A `Connected` condition reports reachability/auth. Reasons: `Connected`, `SecretMissing`, `AuthFailed`, `RateLimited`, `PCEUnreachable`.

## Example

```yaml
apiVersion: microsegment.io/v1alpha1
kind: PCEConnection
metadata:
  name: prod-pce
spec:
  pceUrl: mypce.example.com:8443
  orgId: 3
  credentialsSecretRef:
    name: illumio-pce-api
    namespace: illumio-operator
```
```

Create an empty `docs/reference/.gitkeep` (keeps the dir even if generated files are git-ignored).

- [ ] **Step 5: Add Makefile docs targets**

Append to the `Makefile` (under a new "Documentation" section). Use a tab-indented recipe:

```makefile
##@ Documentation

.PHONY: docs-api
docs-api: ## Generate CRD API reference markdown from Go types.
	go run github.com/elastic/crd-ref-docs@latest \
		--source-path=./api \
		--config=docs/crd-ref-docs.yaml \
		--renderer=markdown \
		--output-path=docs/reference/api.md

.PHONY: docs-build
docs-build: ## Build the docs site into ./site (requires mkdocs-material).
	mkdocs build --strict

.PHONY: docs-serve
docs-serve: ## Serve the docs locally at http://localhost:8000.
	mkdocs serve
```

- [ ] **Step 6: Add the Pages deploy workflow**

Create `.github/workflows/docs.yml`:

```yaml
name: Docs

on:
  push:
    branches: [main]
  pull_request:

permissions:
  contents: read

concurrency:
  group: docs-${{ github.ref }}
  cancel-in-progress: true

jobs:
  build:
    name: Build docs
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          persist-credentials: false
      - uses: actions/setup-python@v5
        with:
          python-version: "3.12"
      - run: pip install -r requirements-docs.txt
      - run: mkdocs build --strict

  deploy:
    name: Deploy to GitHub Pages
    if: github.ref == 'refs/heads/main' && github.event_name == 'push'
    needs: build
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-python@v5
        with:
          python-version: "3.12"
      - run: pip install -r requirements-docs.txt
      - run: mkdocs gh-deploy --force
```

- [ ] **Step 7: Ignore the build output**

Add `site/` to `.gitignore` (append a line).

- [ ] **Step 8: Verify the site builds and the API reference generates**

```bash
python3 -m venv .venv-docs && . .venv-docs/bin/activate && pip install -r requirements-docs.txt
make docs-api
make docs-build
deactivate
```

Expected: `make docs-api` writes `docs/reference/api.md` (contains the `PCEConnection` types); `make docs-build` produces `site/` with `--strict` (no broken-link/nav warnings). Add `reference/api.md` to the `mkdocs.yml` nav under "API Reference" (as `- Generated API: reference/api.md`). If `.venv-docs` was created in-repo, remove it before committing (`rm -rf .venv-docs`) — do not commit it.

> If the runner/sandbox cannot install Python packages, report DONE_WITH_CONCERNS with the exact error; `make docs-api` (Go-based) must still succeed, and the CI workflow will validate the MkDocs build.

- [ ] **Step 9: Commit**

```bash
git add mkdocs.yml requirements-docs.txt docs/ .github/workflows/docs.yml Makefile .gitignore
git commit -m "docs: scaffold MkDocs site, CRD reference generator, and Pages workflow"
```

---

### Task 2: PCE container-cluster client

**Files:**
- Create: `internal/pce/containercluster.go`
- Test: `internal/pce/containercluster_test.go`

**Interfaces:**
- Consumes: `Client.do`, `Client.orgPath` (Plan 1).
- Produces:
  - `type ContainerCluster struct { Href, Name, Description, ContainerClusterToken string }` (json: `href`, `name`, `description`, `container_cluster_token`)
  - `func (c *Client) ListContainerClusters(ctx context.Context) ([]ContainerCluster, error)`
  - `func (c *Client) FindContainerClusterByName(ctx context.Context, name string) (*ContainerCluster, error)` — returns `(nil, nil)` when none matches
  - `func (c *Client) CreateContainerCluster(ctx context.Context, name, description string, owner Owner) (*ContainerCluster, error)` — POST; response carries the one-time token
  - `func ContainerClusterUUID(href string) string` — last path segment of href

- [ ] **Step 1: Write the failing test**

Create `internal/pce/containercluster_test.go`:

```go
package pce

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestCreateContainerCluster_ReturnsTokenAndStampsOwnership(t *testing.T) {
	var posted ContainerCluster
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v2/orgs/7/container_clusters" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&posted)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"href":"/orgs/7/container_clusters/1b85-uuid","name":"ocp-prod","container_cluster_token":"7_abc123"}`))
	})
	cl, err := c.CreateContainerCluster(context.Background(), "ocp-prod", "managed by operator", Owner{DataSet: "illumio-operator", Reference: "cp-uid"})
	if err != nil {
		t.Fatalf("CreateContainerCluster error: %v", err)
	}
	if cl.ContainerClusterToken != "7_abc123" {
		t.Errorf("token = %q, want 7_abc123", cl.ContainerClusterToken)
	}
	if posted.Name != "ocp-prod" {
		t.Errorf("posted name = %q", posted.Name)
	}
	if ContainerClusterUUID(cl.Href) != "1b85-uuid" {
		t.Errorf("uuid = %q, want 1b85-uuid", ContainerClusterUUID(cl.Href))
	}
}

func TestFindContainerClusterByName(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"href":"/orgs/7/container_clusters/a","name":"other"},{"href":"/orgs/7/container_clusters/b","name":"ocp-prod"}]`))
	})
	got, err := c.FindContainerClusterByName(context.Background(), "ocp-prod")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if got == nil || got.Href != "/orgs/7/container_clusters/b" {
		t.Fatalf("got = %+v, want cluster b", got)
	}
}

func TestFindContainerClusterByName_NoneReturnsNilNil(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	got, err := c.FindContainerClusterByName(context.Background(), "missing")
	if err != nil || got != nil {
		t.Fatalf("got=%+v err=%v, want nil,nil", got, err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/pce/ -run ContainerCluster -v`
Expected: compile failure — `undefined: ContainerCluster`, etc.

- [ ] **Step 3: Write the implementation**

Create `internal/pce/containercluster.go`:

```go
package pce

import (
	"context"
	"net/http"
	"strings"
)

// ContainerCluster is the PCE object representing a Kubernetes cluster.
type ContainerCluster struct {
	Href        string `json:"href,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// ContainerClusterToken is returned ONLY at creation. Persist immediately.
	ContainerClusterToken string `json:"container_cluster_token,omitempty"`
	ExternalDataSet       string `json:"external_data_set,omitempty"`
	ExternalDataReference string `json:"external_data_reference,omitempty"`
}

// ListContainerClusters returns all container clusters in the org.
func (c *Client) ListContainerClusters(ctx context.Context) ([]ContainerCluster, error) {
	var out []ContainerCluster
	if err := c.do(ctx, http.MethodGet, c.orgPath("/container_clusters"), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// FindContainerClusterByName returns the cluster with the exact name, or (nil, nil).
func (c *Client) FindContainerClusterByName(ctx context.Context, name string) (*ContainerCluster, error) {
	clusters, err := c.ListContainerClusters(ctx)
	if err != nil {
		return nil, err
	}
	for i := range clusters {
		if clusters[i].Name == name {
			return &clusters[i], nil
		}
	}
	return nil, nil
}

// CreateContainerCluster creates a cluster and returns it, including the
// one-time container_cluster_token. Ownership tags are stamped on creation.
func (c *Client) CreateContainerCluster(ctx context.Context, name, description string, owner Owner) (*ContainerCluster, error) {
	body := ContainerCluster{
		Name:                  name,
		Description:           description,
		ExternalDataSet:       owner.DataSet,
		ExternalDataReference: owner.Reference,
	}
	var created ContainerCluster
	if err := c.do(ctx, http.MethodPost, c.orgPath("/container_clusters"), body, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

// ContainerClusterUUID extracts the cluster UUID (last path segment) from an href.
func ContainerClusterUUID(href string) string {
	if i := strings.LastIndex(href, "/"); i >= 0 {
		return href[i+1:]
	}
	return href
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/pce/ -run ContainerCluster -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/pce/containercluster.go internal/pce/containercluster_test.go
git commit -m "feat(pce): add container-cluster client (create/list/find, uuid)"
```

---

### Task 3: PCE pairing-profile client

**Files:**
- Create: `internal/pce/pairingprofile.go`
- Test: `internal/pce/pairingprofile_test.go`

**Interfaces:**
- Consumes: `Client.do`, `Client.orgPath`, `Owner` (Plan 1).
- Produces:
  - `type LabelRef struct { Href string }` (json `href`)
  - `type PairingProfile struct { Href, Name, Description, EnforcementMode, VisibilityLevel, AllowedUsesPerKey, KeyLifespan string; Enabled bool; Labels []LabelRef; ExternalDataSet, ExternalDataReference string }`
  - `func (c *Client) FindPairingProfileByName(ctx, name string) (*PairingProfile, error)` — `(nil, nil)` if absent
  - `func (c *Client) CreatePairingProfile(ctx, pp PairingProfile) (*PairingProfile, error)`
  - `func (c *Client) GeneratePairingKey(ctx, profileHref string) (string, error)` — POST `{profileHref}/pairing_key` with `{}`, returns `activation_code`

- [ ] **Step 1: Write the failing test**

Create `internal/pce/pairingprofile_test.go`:

```go
package pce

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestCreatePairingProfile_PostsEnabledAndOwnership(t *testing.T) {
	var posted PairingProfile
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/orgs/7/pairing_profiles" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&posted)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"href":"/orgs/7/pairing_profiles/5","name":"pp-cven","enabled":true}`))
	})
	pp, err := c.CreatePairingProfile(context.Background(), PairingProfile{
		Name: "pp-cven", Enabled: true, EnforcementMode: "visibility_only",
		Labels:          []LabelRef{{Href: "/orgs/7/labels/224"}},
		ExternalDataSet: "illumio-operator", ExternalDataReference: "cp-uid",
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if pp.Href != "/orgs/7/pairing_profiles/5" {
		t.Errorf("href = %q", pp.Href)
	}
	if !posted.Enabled || posted.ExternalDataReference != "cp-uid" {
		t.Errorf("posted = %+v", posted)
	}
	if len(posted.Labels) != 1 || posted.Labels[0].Href != "/orgs/7/labels/224" {
		t.Errorf("posted labels = %+v, want one label href /orgs/7/labels/224", posted.Labels)
	}
}

func TestGeneratePairingKey_ReturnsActivationCode(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v2/orgs/7/pairing_profiles/5/pairing_key" {
			t.Fatalf("method/path = %s %q", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"activation_code":"act-123"}`))
	})
	code, err := c.GeneratePairingKey(context.Background(), "/orgs/7/pairing_profiles/5")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if code != "act-123" {
		t.Errorf("code = %q, want act-123", code)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/pce/ -run PairingProfile -v` and `-run PairingKey`
Expected: compile failure — `undefined: PairingProfile`, etc.

- [ ] **Step 3: Write the implementation**

Create `internal/pce/pairingprofile.go`:

```go
package pce

import (
	"context"
	"net/http"
)

// LabelRef references an Illumio label by href.
type LabelRef struct {
	Href string `json:"href"`
}

// PairingProfile is the PCE object that issues C-VEN pairing keys and assigns
// labels to the nodes paired with the generated key.
type PairingProfile struct {
	Href            string `json:"href,omitempty"`
	Name            string `json:"name,omitempty"`
	Description     string `json:"description,omitempty"`
	Enabled         bool   `json:"enabled"`
	EnforcementMode string `json:"enforcement_mode,omitempty"`
	VisibilityLevel string `json:"visibility_level,omitempty"`
	// AllowedUsesPerKey and KeyLifespan accept the literal string "unlimited".
	AllowedUsesPerKey string     `json:"allowed_uses_per_key,omitempty"`
	KeyLifespan       string     `json:"key_lifespan,omitempty"`
	Labels            []LabelRef `json:"labels,omitempty"`
	ExternalDataSet       string `json:"external_data_set,omitempty"`
	ExternalDataReference string `json:"external_data_reference,omitempty"`
}

// FindPairingProfileByName returns the profile with the exact name, or (nil, nil).
func (c *Client) FindPairingProfileByName(ctx context.Context, name string) (*PairingProfile, error) {
	var profiles []PairingProfile
	if err := c.do(ctx, http.MethodGet, c.orgPath("/pairing_profiles"), nil, &profiles); err != nil {
		return nil, err
	}
	for i := range profiles {
		if profiles[i].Name == name {
			return &profiles[i], nil
		}
	}
	return nil, nil
}

// CreatePairingProfile creates a pairing profile and returns it (with its href).
func (c *Client) CreatePairingProfile(ctx context.Context, pp PairingProfile) (*PairingProfile, error) {
	var created PairingProfile
	if err := c.do(ctx, http.MethodPost, c.orgPath("/pairing_profiles"), pp, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

// pairingKeyResponse models the generate-pairing-key response.
type pairingKeyResponse struct {
	ActivationCode string `json:"activation_code"`
}

// GeneratePairingKey generates a new pairing key (activation code) for the
// profile identified by profileHref (an href like /orgs/7/pairing_profiles/5).
func (c *Client) GeneratePairingKey(ctx context.Context, profileHref string) (string, error) {
	var resp pairingKeyResponse
	if err := c.do(ctx, http.MethodPost, profileHref+"/pairing_key", struct{}{}, &resp); err != nil {
		return "", err
	}
	return resp.ActivationCode, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/pce/ -run "PairingProfile|PairingKey" -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/pce/pairingprofile.go internal/pce/pairingprofile_test.go
git commit -m "feat(pce): add pairing-profile client and pairing-key generation"
```

---

### Task 4: ClusterProfile API types (onboarding fields)

**Files:**
- Create: `api/v1alpha1/clusterprofile_types.go`
- Test: `api/v1alpha1/clusterprofile_types_test.go`
- Generated (via `make generate manifests`): `api/v1alpha1/zz_generated.deepcopy.go`, `config/crd/bases/microsegment.io_clusterprofiles.yaml`, RBAC.

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `LocalObjectReference{ Name string }`
  - `NodePairingProfileSpec{ ExistingName string; Labels map[string]string; EnforcementMode string }`
  - `OnboardingSpec{ ContainerClusterName string; CredentialsOutputSecret string; NodePairingProfile NodePairingProfileSpec }`
  - `ClusterProfileSpec{ PCEConnectionRef LocalObjectReference; Onboarding OnboardingSpec; ProvisioningMode string }`
  - `ClusterProfileStatus{ Conditions []metav1.Condition; ContainerClusterHref string; ContainerClusterID string; ObservedGeneration int64 }`
  - condition + reason constants: `ConditionOnboarded = "Onboarded"`; reasons `ReasonOnboarded`, `ReasonPCEConnectionNotReady`, `ReasonOnboardFailed`.

> This is the onboarding-only shape of `ClusterProfile`. Plan 3 adds `namespaceRules` and label/enforcement fields for CWP management.

- [ ] **Step 1: Write the failing test**

Create `api/v1alpha1/clusterprofile_types_test.go`:

```go
package v1alpha1

import "testing"

func TestClusterProfile_Shape(t *testing.T) {
	cp := ClusterProfile{
		Spec: ClusterProfileSpec{
			PCEConnectionRef: LocalObjectReference{Name: "prod-pce"},
			Onboarding: OnboardingSpec{
				ContainerClusterName:    "ocp-prod",
				CredentialsOutputSecret: "illumio-cluster-creds",
			},
			ProvisioningMode: "manual",
		},
	}
	if cp.Spec.Onboarding.ContainerClusterName != "ocp-prod" {
		t.Errorf("clusterName = %q", cp.Spec.Onboarding.ContainerClusterName)
	}
	if ConditionOnboarded != "Onboarded" {
		t.Errorf("ConditionOnboarded = %q", ConditionOnboarded)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./api/v1alpha1/ -run TestClusterProfile -v`
Expected: compile failure — `undefined: ClusterProfile`.

- [ ] **Step 3: Write the types**

Create `api/v1alpha1/clusterprofile_types.go`:

```go
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LocalObjectReference references a cluster-scoped object by name.
type LocalObjectReference struct {
	Name string `json:"name"`
}

// NodePairingProfileSpec configures the pairing profile the C-VEN uses to pair
// the cluster's nodes. Either reuse an existing profile by name, or have the
// operator create one with the given node labels and enforcement mode.
type NodePairingProfileSpec struct {
	// ExistingName: if set, use this existing PCE pairing profile (by name)
	// instead of creating one. Labels/EnforcementMode are ignored in that case.
	// +optional
	ExistingName string `json:"existingName,omitempty"`
	// Labels to apply to nodes paired with this profile, as Illumio
	// label-key -> value (e.g. {"role": "node", "env": "prod"}). The operator
	// resolves each to an Illumio label href (create-if-missing).
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// EnforcementMode for a created pairing profile. One of idle,
	// visibility_only, full. Defaults to idle.
	// +kubebuilder:validation:Enum=idle;visibility_only;full
	// +kubebuilder:default=idle
	// +optional
	EnforcementMode string `json:"enforcementMode,omitempty"`
}

// OnboardingSpec configures how the cluster is onboarded to the PCE.
type OnboardingSpec struct {
	// ContainerClusterName is the name of the PCE Container Cluster object to
	// ensure exists for this cluster.
	ContainerClusterName string `json:"containerClusterName"`
	// CredentialsOutputSecret is the name of the Secret (in the operator's
	// namespace) the operator writes the agent credentials into:
	// pce_url, cluster_id, cluster_token, cluster_code.
	CredentialsOutputSecret string `json:"credentialsOutputSecret"`
	// NodePairingProfile configures the pairing profile the C-VEN uses to pair
	// the cluster's nodes.
	// +optional
	NodePairingProfile NodePairingProfileSpec `json:"nodePairingProfile,omitempty"`
}

// ClusterProfileSpec is the desired onboarding state for this cluster.
type ClusterProfileSpec struct {
	// PCEConnectionRef references the PCEConnection to use.
	PCEConnectionRef LocalObjectReference `json:"pceConnectionRef"`
	// Onboarding configures PCE cluster onboarding.
	Onboarding OnboardingSpec `json:"onboarding"`
	// ProvisioningMode is the default policy provisioning mode for resources in
	// this cluster. One of: auto, manual, draft-only. Consumed by later
	// policy reconciliation; defaults to manual.
	// +kubebuilder:validation:Enum=auto;manual;draft-only
	// +kubebuilder:default=manual
	// +optional
	ProvisioningMode string `json:"provisioningMode,omitempty"`
}

// ClusterProfileStatus is the observed onboarding state.
type ClusterProfileStatus struct {
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ContainerClusterHref is the href of the PCE Container Cluster object.
	// +optional
	ContainerClusterHref string `json:"containerClusterHref,omitempty"`
	// ContainerClusterID is the cluster UUID (last segment of the href).
	// +optional
	ContainerClusterID string `json:"containerClusterID,omitempty"`
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// Condition types and reasons for ClusterProfile.
const (
	ConditionOnboarded = "Onboarded"

	ReasonOnboarded             = "Onboarded"
	ReasonPCEConnectionNotReady = "PCEConnectionNotReady"
	ReasonOnboardFailed         = "OnboardFailed"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories=illumio,shortName=cprof
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.onboarding.containerClusterName`
// +kubebuilder:printcolumn:name="ClusterID",type=string,JSONPath=`.status.containerClusterID`
// +kubebuilder:printcolumn:name="Onboarded",type=string,JSONPath=`.status.conditions[?(@.type=="Onboarded")].status`

// ClusterProfile onboards a Kubernetes cluster to an Illumio PCE.
type ClusterProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterProfileSpec   `json:"spec,omitempty"`
	Status ClusterProfileStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterProfileList contains a list of ClusterProfile.
type ClusterProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterProfile `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterProfile{}, &ClusterProfileList{})
}
```

- [ ] **Step 4: Regenerate code and manifests**

```bash
make generate
make manifests
```

Expected: `zz_generated.deepcopy.go` gains `ClusterProfile*` deepcopy methods; `config/crd/bases/microsegment.io_clusterprofiles.yaml` is created with `scope: Cluster`, category `illumio`, shortName `cprof`.

- [ ] **Step 5: Run the test + build to verify**

Run: `go test ./api/v1alpha1/ -run TestClusterProfile -v` (PASS), then `make build`.

- [ ] **Step 6: Commit**

```bash
git add api/v1alpha1/ config/crd/ config/rbac/
git commit -m "feat(api): add ClusterProfile types for onboarding"
```

---

### Task 5: ClusterProfile onboarding controller

**Files:**
- Create: `internal/controller/clusterprofile_controller.go`
- Create: `internal/controller/onboarding_client.go`
- Test: `internal/controller/clusterprofile_controller_test.go`
- Modify: `internal/controller/suite_test.go` (register the reconciler with a fake onboarding client)
- Modify: `cmd/main.go` (wire the real factory)

**Interfaces:**
- Consumes: `pce.Config`, `pce.NewClient`, `pce.Owner`, `pce.ContainerCluster`, `pce.PairingProfile`, `pce.ContainerClusterUUID` (Tasks 2–3); `PCEConnectionSpec`, `ClusterProfileSpec`, condition reasons (Task 4).
- Produces:
  - `type OnboardingClient interface { FindContainerClusterByName(ctx, name) (*pce.ContainerCluster, error); CreateContainerCluster(ctx, name, desc string, owner pce.Owner) (*pce.ContainerCluster, error); FindPairingProfileByName(ctx, name) (*pce.PairingProfile, error); CreatePairingProfile(ctx, pp pce.PairingProfile) (*pce.PairingProfile, error); GeneratePairingKey(ctx, profileHref string) (string, error); EnsureLabel(ctx, key, value string, owner pce.Owner) (*pce.Label, error) }`
  - `type OnboardingClientFactory func(cfg pce.Config) OnboardingClient`
  - `func DefaultOnboardingClientFactory(cfg pce.Config) OnboardingClient` (returns `pce.NewClient(cfg)`)
  - reconcile behavior described below.

**Reconcile behavior (idempotent):**
1. Get the `ClusterProfile`. Resolve its `PCEConnection` (by `spec.pceConnectionRef.name`). If the PCEConnection is missing or its `Connected` condition is not `True`, set `Onboarded=False / PCEConnectionNotReady` and requeue after 30s.
2. Load PCE credentials from the PCEConnection's Secret (reuse the same logic shape as PCEConnection: read `api_key`/`api_secret`); build a `pce.Config`; construct the `OnboardingClient` via the factory. Determine the owner: `pce.Owner{DataSet: externalDataSet-or-"illumio-operator", Reference: string(clusterProfile.UID)}`.
3. **Ensure the container cluster.** If `status.containerClusterHref` is already set, reuse it (do not recreate). Otherwise: `FindContainerClusterByName`; if found, adopt its href (note: token is unavailable for a pre-existing cluster — set `Onboarded=False / OnboardFailed` with a message telling the admin to delete the pre-existing cluster or supply credentials manually, and stop). If not found, `CreateContainerCluster` (capturing the one-time token), and proceed to write the Secret in step 5 with that token.
4. **Ensure the node pairing profile + key.** If `spec.onboarding.nodePairingProfile.existingName` is set, `FindPairingProfileByName(existingName)`; if not found, fail (`OnboardFailed`, "named pairing profile not found"); otherwise use its href. If `existingName` is empty, ensure a profile named `<containerClusterName>-nodes`: resolve each entry of `nodePairingProfile.labels` (key→value) to an Illumio label href via `EnsureLabel(key, value, owner)` (create-if-missing for these admin-defined labels), build `[]pce.LabelRef`, and `CreatePairingProfile` (Enabled true, `EnforcementMode` from the spec defaulting to `idle`, AllowedUsesPerKey/KeyLifespan `"unlimited"`, the resolved `Labels`, owner tags) if it does not already exist. Then `GeneratePairingKey(profile.Href)` to obtain the activation code (`cluster_code`).
5. **Write the output Secret** (`spec.onboarding.credentialsOutputSecret`, in the operator's namespace) with keys `pce_url`, `cluster_id`, `cluster_token`, `cluster_code`. Create or update via controller-runtime `CreateOrUpdate`. Only overwrite `cluster_token` when freshly created (do not clobber an existing token with an empty value on later reconciles).
6. Set `status.containerClusterHref`/`containerClusterID`, `Onboarded=True / Onboarded`, `ObservedGeneration`; requeue after 10m for periodic reconciliation.

> **Idempotency rule for the token:** because the container-cluster token is returned only at creation, the controller must write the Secret in the SAME reconcile that creates the cluster. On subsequent reconciles (`status.containerClusterHref` already set), it refreshes the pairing key/Secret but never expects the token again and must not blank it.

- [ ] **Step 1: Write the failing test**

Create `internal/controller/clusterprofile_controller_test.go`:

```go
package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
)

var _ = Describe("ClusterProfile onboarding controller", func() {
	const ns = "default"

	It("onboards: creates the cluster, writes the creds Secret, sets Onboarded=True", func() {
		ctx := context.Background()

		// A ready PCEConnection + its credentials secret.
		credSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "pce-creds-ob", Namespace: ns},
			Data:       map[string][]byte{keyAPIKey: []byte("k"), keyAPISecret: []byte("s")},
		}
		Expect(k8sClient.Create(ctx, credSecret)).To(Succeed())

		pc := &microv1.PCEConnection{
			ObjectMeta: metav1.ObjectMeta{Name: "pce-ob"},
			Spec: microv1.PCEConnectionSpec{
				PCEURL: testPCEURL, OrgID: 1,
				CredentialsSecretRef: microv1.SecretReference{Name: "pce-creds-ob", Namespace: ns},
			},
		}
		Expect(k8sClient.Create(ctx, pc)).To(Succeed())
		// Force its Connected condition True (the onboarding controller checks it).
		Eventually(func(g Gomega) {
			got := &microv1.PCEConnection{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "pce-ob"}, got)).To(Succeed())
			meta.SetStatusCondition(&got.Status.Conditions, metav1.Condition{
				Type: microv1.ConditionConnected, Status: metav1.ConditionTrue,
				Reason: microv1.ReasonConnected, Message: "test",
			})
			g.Expect(k8sClient.Status().Update(ctx, got)).To(Succeed())
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())

		cp := &microv1.ClusterProfile{
			ObjectMeta: metav1.ObjectMeta{Name: "cp-ob"},
			Spec: microv1.ClusterProfileSpec{
				PCEConnectionRef: microv1.LocalObjectReference{Name: "pce-ob"},
				Onboarding: microv1.OnboardingSpec{
					ContainerClusterName:    "ocp-test",
					CredentialsOutputSecret: "cluster-creds-out",
					NodePairingProfile: microv1.NodePairingProfileSpec{
						Labels:          map[string]string{"role": "node"},
						EnforcementMode: "idle",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, cp)).To(Succeed())

		Eventually(func(g Gomega) {
			got := &microv1.ClusterProfile{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cp-ob"}, got)).To(Succeed())
			c := meta.FindStatusCondition(got.Status.Conditions, microv1.ConditionOnboarded)
			g.Expect(c).NotTo(BeNil())
			g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(got.Status.ContainerClusterID).To(Equal("uuid-ob"))
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())

		// The output Secret carries all four agent credential keys.
		Eventually(func(g Gomega) {
			s := &corev1.Secret{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cluster-creds-out", Namespace: operatorNamespaceForTest}, s)).To(Succeed())
			g.Expect(s.Data).To(HaveKey("pce_url"))
			g.Expect(string(s.Data["cluster_id"])).To(Equal("uuid-ob"))
			g.Expect(string(s.Data["cluster_token"])).To(Equal("tok-ob"))
			g.Expect(string(s.Data["cluster_code"])).To(Equal("act-ob"))
		}, 15*time.Second, 250*time.Millisecond).Should(Succeed())
	})
})
```

- [ ] **Step 2: Wire the fake onboarding client and operator namespace into the suite**

In `internal/controller/suite_test.go`, add a package-level test constant and a fake, and register the reconciler. Add near the other test wiring:

```go
const operatorNamespaceForTest = "default"

// fakeOnboardingClient returns deterministic onboarding results for envtest.
type fakeOnboardingClient struct{}

func (fakeOnboardingClient) FindContainerClusterByName(context.Context, string) (*pce.ContainerCluster, error) {
	return nil, nil // not found → controller creates
}
func (fakeOnboardingClient) CreateContainerCluster(_ context.Context, name, _ string, _ pce.Owner) (*pce.ContainerCluster, error) {
	return &pce.ContainerCluster{Href: "/orgs/1/container_clusters/uuid-ob", Name: name, ContainerClusterToken: "tok-ob"}, nil
}
func (fakeOnboardingClient) FindPairingProfileByName(context.Context, string) (*pce.PairingProfile, error) {
	return nil, nil
}
func (fakeOnboardingClient) CreatePairingProfile(_ context.Context, pp pce.PairingProfile) (*pce.PairingProfile, error) {
	pp.Href = "/orgs/1/pairing_profiles/9"
	return &pp, nil
}
func (fakeOnboardingClient) GeneratePairingKey(context.Context, string) (string, error) {
	return "act-ob", nil
}
func (fakeOnboardingClient) EnsureLabel(_ context.Context, key, value string, _ pce.Owner) (*pce.Label, error) {
	return &pce.Label{Href: "/orgs/1/labels/" + key + "-" + value, Key: key, Value: value}, nil
}
```

Register the reconciler alongside the PCEConnection one (set `OperatorNamespace: operatorNamespaceForTest` so the output Secret lands where the test looks):

```go
err = (&ClusterProfileReconciler{
	Client:            k8sManager.GetClient(),
	Scheme:            k8sManager.GetScheme(),
	OperatorNamespace: operatorNamespaceForTest,
	NewOnboardingClient: func(pce.Config) OnboardingClient { return fakeOnboardingClient{} },
}).SetupWithManager(k8sManager)
Expect(err).ToNot(HaveOccurred())
```

Ensure `suite_test.go` imports `"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"` and `"context"` (add if missing).

- [ ] **Step 3: Run to verify it fails**

Run: `make test`
Expected: compile failure — `undefined: ClusterProfileReconciler`, `OnboardingClient`, `OnboardingClientFactory`.

- [ ] **Step 4: Write the onboarding client interface**

Create `internal/controller/onboarding_client.go`:

```go
package controller

import (
	"context"

	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

// OnboardingClient is the subset of the PCE client the ClusterProfile
// controller needs. The real *pce.Client satisfies it.
type OnboardingClient interface {
	FindContainerClusterByName(ctx context.Context, name string) (*pce.ContainerCluster, error)
	CreateContainerCluster(ctx context.Context, name, description string, owner pce.Owner) (*pce.ContainerCluster, error)
	FindPairingProfileByName(ctx context.Context, name string) (*pce.PairingProfile, error)
	CreatePairingProfile(ctx context.Context, pp pce.PairingProfile) (*pce.PairingProfile, error)
	GeneratePairingKey(ctx context.Context, profileHref string) (string, error)
}

// OnboardingClientFactory builds an OnboardingClient from a Config (injectable for tests).
type OnboardingClientFactory func(cfg pce.Config) OnboardingClient

// DefaultOnboardingClientFactory wraps the real PCE client.
func DefaultOnboardingClientFactory(cfg pce.Config) OnboardingClient {
	return pce.NewClient(cfg)
}
```

- [ ] **Step 5: Write the reconciler**

Create `internal/controller/clusterprofile_controller.go`:

```go
package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	microv1 "github.com/alexgoller/illumio-k8s-utility-operator/api/v1alpha1"
	"github.com/alexgoller/illumio-k8s-utility-operator/internal/pce"
)

const (
	onboardRequeueNotReady = 30 * time.Second
	onboardRequeueHealthy  = 10 * time.Minute
)

// ClusterProfileReconciler reconciles a ClusterProfile (onboarding).
type ClusterProfileReconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	OperatorNamespace   string
	NewOnboardingClient OnboardingClientFactory
}

// +kubebuilder:rbac:groups=microsegment.io,resources=clusterprofiles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=microsegment.io,resources=clusterprofiles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=microsegment.io,resources=pceconnections,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch

func (r *ClusterProfileReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var cp microv1.ClusterProfile
	if err := r.Get(ctx, req.NamespacedName, &cp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if r.NewOnboardingClient == nil {
		r.NewOnboardingClient = DefaultOnboardingClientFactory
	}

	cfg, externalDataSet, ready, transientErr := r.resolveConnection(ctx, &cp)
	if transientErr != nil {
		return ctrl.Result{}, transientErr
	}
	if !ready {
		return r.onboardFail(ctx, &cp, microv1.ReasonPCEConnectionNotReady,
			"referenced PCEConnection is missing or not Connected", onboardRequeueNotReady)
	}

	pclient := r.NewOnboardingClient(cfg)
	owner := pce.Owner{DataSet: externalDataSet, Reference: string(cp.UID)}

	// Ensure the container cluster (capture the one-time token only on create).
	var token string
	if cp.Status.ContainerClusterHref == "" {
		existing, err := pclient.FindContainerClusterByName(ctx, cp.Spec.Onboarding.ContainerClusterName)
		if err != nil {
			return ctrl.Result{}, err
		}
		if existing != nil {
			return r.onboardFail(ctx, &cp, microv1.ReasonOnboardFailed,
				fmt.Sprintf("container cluster %q already exists in the PCE; its one-time token cannot be recovered. Delete it or supply credentials manually.", cp.Spec.Onboarding.ContainerClusterName),
				onboardRequeueHealthy)
		}
		created, err := pclient.CreateContainerCluster(ctx, cp.Spec.Onboarding.ContainerClusterName, "Managed by illumio-k8s-utility-operator", owner)
		if err != nil {
			return ctrl.Result{}, err
		}
		cp.Status.ContainerClusterHref = created.Href
		cp.Status.ContainerClusterID = pce.ContainerClusterUUID(created.Href)
		token = created.ContainerClusterToken
	}

	// Ensure the node pairing profile (cluster_code source).
	npp := cp.Spec.Onboarding.NodePairingProfile
	var pp *pce.PairingProfile
	if npp.ExistingName != "" {
		pp, err = pclient.FindPairingProfileByName(ctx, npp.ExistingName)
		if err != nil {
			return ctrl.Result{}, err
		}
		if pp == nil {
			return r.onboardFail(ctx, &cp, microv1.ReasonOnboardFailed,
				fmt.Sprintf("pairing profile %q not found in the PCE", npp.ExistingName), onboardRequeueHealthy)
		}
	} else {
		ppName := cp.Spec.Onboarding.ContainerClusterName + "-nodes"
		pp, err = pclient.FindPairingProfileByName(ctx, ppName)
		if err != nil {
			return ctrl.Result{}, err
		}
		if pp == nil {
			// Resolve the requested node labels to Illumio label hrefs.
			labels := make([]pce.LabelRef, 0, len(npp.Labels))
			for key, value := range npp.Labels {
				lbl, lerr := pclient.EnsureLabel(ctx, key, value, owner)
				if lerr != nil {
					return ctrl.Result{}, lerr
				}
				labels = append(labels, pce.LabelRef{Href: lbl.Href})
			}
			mode := npp.EnforcementMode
			if mode == "" {
				mode = "idle"
			}
			pp, err = pclient.CreatePairingProfile(ctx, pce.PairingProfile{
				Name: ppName, Enabled: true, EnforcementMode: mode,
				AllowedUsesPerKey: "unlimited", KeyLifespan: "unlimited",
				Labels:          labels,
				ExternalDataSet: owner.DataSet, ExternalDataReference: owner.Reference,
			})
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}
	code, err := pclient.GeneratePairingKey(ctx, pp.Href)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Publish the output Secret (only set cluster_token when freshly created).
	if err := r.writeCredentialsSecret(ctx, &cp, cfg.PCEURL, token, code); err != nil {
		return ctrl.Result{}, err
	}

	meta.SetStatusCondition(&cp.Status.Conditions, metav1.Condition{
		Type: microv1.ConditionOnboarded, Status: metav1.ConditionTrue,
		Reason: microv1.ReasonOnboarded, Message: "cluster onboarded; credentials published",
	})
	cp.Status.ObservedGeneration = cp.Generation
	if err := r.Status().Update(ctx, &cp); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: onboardRequeueHealthy}, nil
}

// resolveConnection finds the PCEConnection, checks its Connected condition,
// and loads credentials. Returns (cfg, externalDataSet, ready, transientErr).
func (r *ClusterProfileReconciler) resolveConnection(ctx context.Context, cp *microv1.ClusterProfile) (pce.Config, string, bool, error) {
	var conn microv1.PCEConnection
	if err := r.Get(ctx, types.NamespacedName{Name: cp.Spec.PCEConnectionRef.Name}, &conn); err != nil {
		if apierrors.IsNotFound(err) {
			return pce.Config{}, "", false, nil
		}
		return pce.Config{}, "", false, err
	}
	if !meta.IsStatusConditionTrue(conn.Status.Conditions, microv1.ConditionConnected) {
		return pce.Config{}, "", false, nil
	}
	var secret corev1.Secret
	key := types.NamespacedName{Name: conn.Spec.CredentialsSecretRef.Name, Namespace: conn.Spec.CredentialsSecretRef.Namespace}
	if err := r.Get(ctx, key, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return pce.Config{}, "", false, nil
		}
		return pce.Config{}, "", false, err
	}
	apiKey, apiSecret := string(secret.Data["api_key"]), string(secret.Data["api_secret"])
	if apiKey == "" || apiSecret == "" {
		return pce.Config{}, "", false, nil
	}
	eds := conn.Spec.ExternalDataSet
	if eds == "" {
		eds = "illumio-operator"
	}
	return pce.Config{PCEURL: conn.Spec.PCEURL, OrgID: conn.Spec.OrgID, APIKey: apiKey, APISecret: apiSecret}, eds, true, nil
}

// writeCredentialsSecret creates/updates the output Secret. cluster_token is
// only written when token != "" (it is recoverable only at cluster creation).
func (r *ClusterProfileReconciler) writeCredentialsSecret(ctx context.Context, cp *microv1.ClusterProfile, pceURL, token, code string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cp.Spec.Onboarding.CredentialsOutputSecret,
			Namespace: r.OperatorNamespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		if secret.Data == nil {
			secret.Data = map[string][]byte{}
		}
		secret.Data["pce_url"] = []byte(pceURL)
		secret.Data["cluster_id"] = []byte(cp.Status.ContainerClusterID)
		secret.Data["cluster_code"] = []byte(code)
		if token != "" {
			secret.Data["cluster_token"] = []byte(token)
		}
		return nil
	})
	return err
}

func (r *ClusterProfileReconciler) onboardFail(ctx context.Context, cp *microv1.ClusterProfile, reason, msg string, requeue time.Duration) (ctrl.Result, error) {
	meta.SetStatusCondition(&cp.Status.Conditions, metav1.Condition{
		Type: microv1.ConditionOnboarded, Status: metav1.ConditionFalse, Reason: reason, Message: msg,
	})
	cp.Status.ObservedGeneration = cp.Generation
	if err := r.Status().Update(ctx, cp); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: requeue}, nil
}

func (r *ClusterProfileReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&microv1.ClusterProfile{}).
		Complete(r)
}
```

- [ ] **Step 6: Wire the real factory and operator namespace in main**

In `cmd/main.go`, register the reconciler after the PCEConnection one. Resolve the operator namespace from `POD_NAMESPACE` (fall back to `illumio-operator`):

```go
operatorNamespace := os.Getenv("POD_NAMESPACE")
if operatorNamespace == "" {
	operatorNamespace = "illumio-operator"
}
if err := (&controller.ClusterProfileReconciler{
	Client:              mgr.GetClient(),
	Scheme:              mgr.GetScheme(),
	OperatorNamespace:   operatorNamespace,
	NewOnboardingClient: controller.DefaultOnboardingClientFactory,
}).SetupWithManager(mgr); err != nil {
	setupLog.Error(err, "unable to create controller", "controller", "ClusterProfile")
	os.Exit(1)
}
```

(Ensure `os` is imported — it already is in the scaffold.)

- [ ] **Step 7: Run the tests to verify they pass**

Run: `make test`
Expected: PASS, including the new ClusterProfile onboarding spec (Onboarded=True, all four Secret keys present). Then `make build`.

- [ ] **Step 8: Run lint**

Run: `make lint`
Expected: `0 issues.` (Extract constants for any string literal the linter flags with `goconst`; check `errcheck` on any new `Close`/`Update` calls.)

- [ ] **Step 9: Commit**

```bash
git add internal/controller/ cmd/main.go config/rbac/
git commit -m "feat(controller): onboard cluster to PCE and publish agent credentials Secret"
```

---

### Task 6: Operator Helm chart for one-command install

**Files:**
- Generate: `dist/chart/**` (kubebuilder Helm plugin — manager Deployment, RBAC, CRDs, `values.yaml`, `_helpers.tpl`)
- Create: `dist/chart/templates/illumio/pce-credentials.yaml`, `dist/chart/templates/illumio/pceconnection.yaml`, `dist/chart/templates/illumio/clusterprofile.yaml`
- Modify: `dist/chart/values.yaml` (add `pce` and `onboarding` blocks)
- Modify: `dist/chart/templates/manager/manager.yaml` (ensure `POD_NAMESPACE` env via downward API — see Step 3)
- Modify: `Makefile` (add a `helm` target that regenerates the chart)

**Interfaces:**
- Consumes: the `PCEConnection` (Plan 1) and `ClusterProfile` (Task 4) CRDs.
- Produces: a Helm chart that installs the operator AND renders the PCE credentials Secret + `PCEConnection` (+ optional `ClusterProfile`) from values, so a full install is one `helm install` command.

> Goal: `helm install illumio-operator ./dist/chart --namespace illumio-operator --create-namespace --set pce.url=mypce:8443 --set pce.orgId=3 --set pce.apiKey=… --set pce.apiSecret=…` brings up the operator, connects it to the PCE, and (with `--set onboarding.enabled=true --set onboarding.containerClusterName=…`) onboards the cluster.

- [ ] **Step 1: Generate the base chart**

```bash
kubebuilder edit --plugins=helm/v1-alpha
```

Expected: creates `dist/chart/` with `Chart.yaml`, `values.yaml`, and `templates/` for the manager, RBAC, and CRDs. Verify the scaffolding is valid:

```bash
helm lint dist/chart
helm template dist/chart >/dev/null && echo "template OK"
```

> Fallback if the Helm plugin is unavailable in the installed kubebuilder: create `dist/chart` by hand — put the CRDs from `config/crd/bases/*.yaml` under `dist/chart/crds/`, and template the operator from `make build-installer` output (`dist/install.yaml`: namespace, ServiceAccount, ClusterRole/Binding, manager Deployment). Report this as DONE_WITH_CONCERNS noting the fallback was used.

- [ ] **Step 2: Add PCE + onboarding values**

Append to `dist/chart/values.yaml`:

```yaml
# --- Illumio PCE configuration (rendered into a Secret + PCEConnection) ---
pce:
  # PCE endpoint host:port (443 for SaaS, 8443 typical on-prem).
  url: ""
  orgId: 0
  # Provide apiKey/apiSecret, OR set existingSecret to the name of a Secret
  # (in the release namespace) that already holds api_key/api_secret.
  apiKey: ""
  apiSecret: ""
  existingSecret: ""
  connectionName: default

# --- Optional cluster onboarding ---
onboarding:
  enabled: false
  name: this-cluster
  containerClusterName: ""
  credentialsOutputSecret: illumio-cluster-creds
  nodePairingProfile:
    existingName: ""
    labels: {}          # e.g. { role: node, env: prod }
    enforcementMode: idle
```

- [ ] **Step 3: Ensure the manager has POD_NAMESPACE**

The `ClusterProfile` controller resolves the operator namespace from `POD_NAMESPACE` (writes the onboarding output Secret there). In the generated manager Deployment template (`dist/chart/templates/manager/manager.yaml`), ensure the manager container has:

```yaml
        env:
          - name: POD_NAMESPACE
            valueFrom:
              fieldRef:
                fieldPath: metadata.namespace
```

(Add the `env:` block if the generated template lacks it.)

- [ ] **Step 4: Add the PCE credential Secret template**

Create `dist/chart/templates/illumio/pce-credentials.yaml`:

```yaml
{{- if not .Values.pce.existingSecret }}
apiVersion: v1
kind: Secret
metadata:
  name: illumio-pce-api
  namespace: {{ .Release.Namespace }}
type: Opaque
stringData:
  api_key: {{ required "pce.apiKey is required (or set pce.existingSecret)" .Values.pce.apiKey | quote }}
  api_secret: {{ required "pce.apiSecret is required (or set pce.existingSecret)" .Values.pce.apiSecret | quote }}
{{- end }}
```

- [ ] **Step 5: Add the PCEConnection template**

Create `dist/chart/templates/illumio/pceconnection.yaml`:

```yaml
apiVersion: microsegment.io/v1alpha1
kind: PCEConnection
metadata:
  name: {{ .Values.pce.connectionName | default "default" }}
spec:
  pceUrl: {{ required "pce.url is required" .Values.pce.url | quote }}
  orgId: {{ required "pce.orgId is required" (.Values.pce.orgId | int) }}
  credentialsSecretRef:
    name: {{ .Values.pce.existingSecret | default "illumio-pce-api" }}
    namespace: {{ .Release.Namespace }}
```

- [ ] **Step 6: Add the optional ClusterProfile template**

Create `dist/chart/templates/illumio/clusterprofile.yaml`:

```yaml
{{- if .Values.onboarding.enabled }}
apiVersion: microsegment.io/v1alpha1
kind: ClusterProfile
metadata:
  name: {{ .Values.onboarding.name | default "this-cluster" }}
spec:
  pceConnectionRef:
    name: {{ .Values.pce.connectionName | default "default" }}
  onboarding:
    containerClusterName: {{ required "onboarding.containerClusterName is required when onboarding.enabled" .Values.onboarding.containerClusterName | quote }}
    credentialsOutputSecret: {{ .Values.onboarding.credentialsOutputSecret | default "illumio-cluster-creds" | quote }}
    nodePairingProfile:
      {{- with .Values.onboarding.nodePairingProfile.existingName }}
      existingName: {{ . | quote }}
      {{- end }}
      {{- with .Values.onboarding.nodePairingProfile.labels }}
      labels:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      enforcementMode: {{ .Values.onboarding.nodePairingProfile.enforcementMode | default "idle" | quote }}
{{- end }}
```

- [ ] **Step 7: Add a Makefile target to regenerate the chart**

Append under the Documentation section (or a new "Distribution" section) in the `Makefile`:

```makefile
.PHONY: helm
helm: ## (Re)generate the operator Helm chart under dist/chart.
	kubebuilder edit --plugins=helm/v1-alpha
```

> Note in the chart's `README` or a comment that re-running `make helm` regenerates plugin-managed files only; the custom `templates/illumio/**` and the `pce`/`onboarding` values blocks are preserved (the plugin manages its own files). If a regen ever strips the custom values, re-apply Step 2.

- [ ] **Step 8: Verify rendering**

```bash
helm lint dist/chart
# Connection only:
helm template illumio-operator dist/chart \
  --set pce.url=mypce.example.com:8443 --set pce.orgId=3 \
  --set pce.apiKey=k --set pce.apiSecret=s | grep -E "kind: (Secret|PCEConnection)"
# With onboarding:
helm template illumio-operator dist/chart \
  --set pce.url=mypce.example.com:8443 --set pce.orgId=3 \
  --set pce.apiKey=k --set pce.apiSecret=s \
  --set onboarding.enabled=true --set onboarding.containerClusterName=ocp-prod | grep -E "kind: ClusterProfile"
```

Expected: `helm lint` passes; the first render contains `kind: Secret` and `kind: PCEConnection`; the second additionally contains `kind: ClusterProfile`. Confirm omitting `pce.url`/`pce.apiKey` makes `helm template` fail with the `required` message (the guard works).

- [ ] **Step 9: Commit**

```bash
git add dist/chart Makefile PROJECT
git commit -m "feat(helm): operator chart rendering PCE Secret, PCEConnection, and optional ClusterProfile"
```

---

### Task 7: Onboarding & installation documentation

**Files:**
- Create: `docs/guides/onboarding.md`
- Create: `docs/reference/clusterprofile.md`
- Modify: `docs/installation.md` (feature the one-command Helm install from Task 6)
- Modify: `mkdocs.yml` (add the new pages to nav)
- Modify: `docs/getting-started.md` (replace the "Onboard the cluster" placeholder with a real step + link)
- Run: `make docs-api` to regenerate `docs/reference/api.md` with `ClusterProfile`.

**Interfaces:**
- Consumes: the behavior implemented in Tasks 4–5.
- Produces: published onboarding documentation.

> **Dispatch note:** dispatch to the documentation subagent. Content task, no Go logic.

- [ ] **Step 1: Write the onboarding how-to**

Create `docs/guides/onboarding.md` covering: what onboarding does (creates the PCE Container Cluster + Pairing Profile, publishes a Secret), a complete `ClusterProfile` example, how to inspect status (`kubectl get clusterprofiles`, the `Onboarded` condition, `containerClusterID`), the output Secret's keys (`pce_url`, `cluster_id`, `cluster_token`, `cluster_code`), and **how Helm consumes them** — both a Flux `HelmRelease.valuesFrom` snippet referencing the Secret and a manual `helm install --set` example. Include the pre-existing-cluster caveat (token is only recoverable at creation; if the cluster already exists, delete it or supply credentials manually). Example:

```yaml
apiVersion: microsegment.io/v1alpha1
kind: ClusterProfile
metadata:
  name: this-cluster
spec:
  pceConnectionRef:
    name: prod-pce
  onboarding:
    containerClusterName: ocp-prod-01
    credentialsOutputSecret: illumio-cluster-creds
```

- [ ] **Step 2: Write the ClusterProfile reference page**

Create `docs/reference/clusterprofile.md` documenting the spec fields (`pceConnectionRef`, `onboarding.containerClusterName`, `onboarding.credentialsOutputSecret`, `provisioningMode`) and the `Onboarded` status condition with its reasons (`Onboarded`, `PCEConnectionNotReady`, `OnboardFailed`), in the same table style as `docs/reference/pceconnection.md`.

- [ ] **Step 3: Update getting-started and nav**

Replace the placeholder onboarding step in `docs/getting-started.md` with a real step that applies a `ClusterProfile` and links to the onboarding guide. Update `docs/installation.md` to lead with the **one-command Helm install** from Task 6 — `helm install illumio-operator ./dist/chart --namespace illumio-operator --create-namespace --set pce.url=... --set pce.orgId=3 --set pce.apiKey=... --set pce.apiSecret=...` (and the `--set onboarding.enabled=true --set onboarding.containerClusterName=...` variant) — and keep `make build-installer` + `kubectl apply -f dist/install.yaml` and the `make install/deploy` flow as alternatives, plus the `pce.existingSecret` option for externally-managed credentials. Add to `mkdocs.yml` nav: under a new `Guides` section add `- Onboarding: guides/onboarding.md`, and under `API Reference` add `- ClusterProfile: reference/clusterprofile.md`.

- [ ] **Step 4: Regenerate the API reference and build**

```bash
make docs-api
. .venv-docs/bin/activate 2>/dev/null && mkdocs build --strict ; deactivate 2>/dev/null || true
```

Expected: `docs/reference/api.md` now includes `ClusterProfile`; `mkdocs build --strict` succeeds with no warnings. (If Python isn't available locally, rely on the Docs CI workflow; `make docs-api` must still succeed.)

- [ ] **Step 5: Commit**

```bash
git add docs/ mkdocs.yml
git commit -m "docs: add cluster onboarding guide and ClusterProfile reference"
```

---

## Self-Review

**Spec coverage (design spec §2, §4.0–§4.2, §6, §9, §10):**
- §6 onboarding (ensure Container Cluster, node pairing profile/key, publish credentials Secret; operator never runs the agent Helm chart) → Tasks 2, 3, 5, 7. ✓
- **Node pairing profile** (create with node labels resolved to Illumio hrefs, or reuse an existing profile by name) → Tasks 3 (`Labels`/`LabelRef`), 4 (`NodePairingProfileSpec`), 5 (resolve + create/reuse). ✓
- **Easy install** (operator Helm chart rendering PCE Secret + PCEConnection + optional ClusterProfile from values; `existingSecret` for external secret managers; `make build-installer` alternative) → Task 6. ✓
- §4.2 `ClusterProfile` (cluster-scoped, pceConnectionRef, onboarding incl. node pairing, provisioningMode default) → Task 4. ✓ (`namespaceRules` + CWP labeling are Plan 3, called out explicitly.)
- §4.0 API conventions (group, version, category `illumio`, shortName) → Task 4 markers. ✓
- §8 ownership tagging on created PCE objects → Tasks 2, 3, 5 (`Owner` stamped on cluster, pairing profile, and any labels created for the pairing profile). ✓
- §9 credentials from Secrets; output credentials written to a Secret → Tasks 5, 6. ✓
- §10 status conditions with machine-readable reasons; PCEConnection-not-ready and transient handling → Task 5. ✓
- "Full documentation from the start" (in-repo Markdown + MkDocs site + CRD API reference + Pages deploy; onboarding + install guides) → Tasks 1 and 7. ✓

**Out of Plan 2 scope (deferred):** CWP namespace reconciler, namespace rules, label assignment to CWPs, enforcement resolution (Plan 3); policy IR, provisioning, front-ends (Plan 4).

**Placeholder scan:** No `TBD`/`TODO`/"handle edge cases" — every code/config step shows complete content. The two Python-availability notes (Task 1 Step 8, Task 7 Step 4) give an explicit fallback (rely on Docs CI; `make docs-api` must still pass) rather than leaving behavior undefined.

**Type consistency:** `pce.ContainerCluster`, `ContainerClusterUUID`, `pce.PairingProfile`, `pce.LabelRef`, `GeneratePairingKey`, `EnsureLabel`, `pce.Label`, `Owner`; `OnboardingClient` (incl. `EnsureLabel`)/`OnboardingClientFactory`/`DefaultOnboardingClientFactory`; `ClusterProfileSpec`/`OnboardingSpec`/`NodePairingProfileSpec`/`LocalObjectReference`/`ClusterProfileStatus`; `ConditionOnboarded` and reasons; `ClusterProfileReconciler` fields (`OperatorNamespace`, `NewOnboardingClient`) — used identically across the tasks that define and consume them. `pce.EnsureLabel`/`pce.Label` come from Plan 1. The controller reuses Plan 1 test constants (`keyAPIKey`, `keyAPISecret`, `testPCEURL`) which already exist in package `controller`.
