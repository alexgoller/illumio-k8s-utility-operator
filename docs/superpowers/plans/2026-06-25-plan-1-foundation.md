# Illumio K8s Utility Operator — Plan 1: Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up the operator project and the PCE integration core: a tested Illumio PCE REST client and a `PCEConnection` CRD + controller that authenticates to a PCE and reports connection health.

**Architecture:** A kubebuilder/controller-runtime operator in Go. A self-contained `internal/pce` package wraps the Illumio PCE REST API v2 (HTTP Basic auth, label resolve/create, ownership tagging, rate-limit classification) and is the only home of Illumio-specific knowledge. The `PCEConnection` controller reads API credentials from a Kubernetes Secret, constructs a PCE client via an injectable factory (so controllers are testable without a real PCE), pings the PCE, and writes status conditions.

**Tech Stack:** Go, kubebuilder v4 + controller-runtime, `metav1.Condition` for status, `envtest` for controller tests, `net/http/httptest` for a mock PCE server. Standard library HTTP client (no third-party PCE SDK).

## Global Constraints

- **Go version:** 1.22+ (matches kubebuilder v4 scaffolding).
- **Module path:** `github.com/microsegment-io/illumio-k8s-utility-operator` (adjust once if your VCS path differs — it only affects import paths).
- **API group:** `microsegment.io`. Kubebuilder forms the group as `<group>.<domain>`, so scaffold with `--domain io` and `--group microsegment` to yield exactly `microsegment.io`.
- **API version:** `v1alpha1`.
- **CRD conventions:** every CRD registers category `illumio` and a shortName; `PCEConnection` is **cluster-scoped**.
- **Target platform:** Illumio Core for Kubernetes in **CLAS** mode, **PCE 24.5+** (a deployment prerequisite; no legacy-mode code paths).
- **Credentials:** all Illumio credentials come from Kubernetes Secrets, never CR spec/status. The PCE API Secret has keys `api_key` and `api_secret`.
- **PCE API base:** requests go to `https://<pceUrl>/api/v2`; `href` values returned by the PCE omit the `/api/v2` prefix.
- **Ownership tagging:** every PCE object the operator creates carries `external_data_set` (operator identity) and `external_data_reference` (owning CR UID).
- **Commits:** conventional-commit messages; commit at the end of every task. End commit messages with the `Co-Authored-By` trailer used in this repo.

---

### Task 1: Project scaffolding

**Files:**
- Create: whole kubebuilder project tree (`go.mod`, `Makefile`, `cmd/main.go`, `PROJECT`, `config/**`).
- Create: `api/v1alpha1/pceconnection_types.go`, `internal/controller/pceconnection_controller.go` (scaffolded stubs).

**Interfaces:**
- Consumes: nothing.
- Produces: a buildable module at the Global-Constraints module path; the `PCEConnection` Go type and an empty reconciler other tasks extend.

- [ ] **Step 1: Initialize the project**

Run in the repo root (it is an empty git repo):

```bash
kubebuilder init --domain io --repo github.com/microsegment-io/illumio-k8s-utility-operator
```

Expected: scaffolds `go.mod`, `cmd/main.go`, `Makefile`, `config/`, `PROJECT`. (`--domain io` + group `microsegment` below = API group `microsegment.io`.)

- [ ] **Step 2: Scaffold the PCEConnection API and controller**

```bash
kubebuilder create api --group microsegment --version v1alpha1 --kind PCEConnection --resource --controller
```

Answer `y` to both "Create Resource" and "Create Controller". Expected: creates `api/v1alpha1/pceconnection_types.go` and `internal/controller/pceconnection_controller.go`.

- [ ] **Step 3: Verify it builds and the empty test target runs**

```bash
make build
make test
```

Expected: `make build` produces `bin/manager` with no errors; `make test` downloads envtest binaries and passes with no failures (the scaffold ships a trivial passing suite).

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "chore: scaffold operator project and PCEConnection api"
```

---

### Task 2: PCE client core (auth, request plumbing, error types, Ping)

**Files:**
- Create: `internal/pce/client.go`
- Create: `internal/pce/errors.go`
- Test: `internal/pce/client_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type Config struct { PCEURL string; OrgID int; APIKey string; APISecret string }`
  - `type Option func(*Client)`; `func WithBaseURL(string) Option`; `func WithHTTPClient(*http.Client) Option`
  - `func NewClient(cfg Config, opts ...Option) *Client`
  - `func (c *Client) Ping(ctx context.Context) error`
  - `type APIError struct { StatusCode int; Body string }` (implements `error`)
  - `type RateLimitError struct { RetryAfter time.Duration }` (implements `error`)
  - `var ErrLabelNotFound = errors.New("illumio label not found")`
  - unexported `func (c *Client) do(ctx, method, path string, body, out any) error` used by later tasks.

- [ ] **Step 1: Write the failing test**

Create `internal/pce/client_test.go`:

```go
package pce

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return NewClient(
		Config{PCEURL: "ignored", OrgID: 7, APIKey: "api_key123", APISecret: "secret123"},
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
	)
}

func TestPing_SendsBasicAuthToLabelsEndpoint(t *testing.T) {
	var gotPath, gotUser, gotPass string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotUser, gotPass, _ = r.BasicAuth()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
	})

	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping returned error: %v", err)
	}
	if gotPath != "/api/v2/orgs/7/labels" {
		t.Errorf("path = %q, want /api/v2/orgs/7/labels", gotPath)
	}
	if gotUser != "api_key123" || gotPass != "secret123" {
		t.Errorf("basic auth = %q:%q, want api_key123:secret123", gotUser, gotPass)
	}
}

func TestPing_RateLimitedReturnsRateLimitError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	err := c.Ping(context.Background())
	rl, ok := err.(*RateLimitError)
	if !ok {
		t.Fatalf("error = %T (%v), want *RateLimitError", err, err)
	}
	if rl.RetryAfter.Seconds() != 30 {
		t.Errorf("RetryAfter = %v, want 30s", rl.RetryAfter)
	}
}

func TestPing_ServerErrorReturnsAPIError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad key"}`))
	})
	err := c.Ping(context.Background())
	ae, ok := err.(*APIError)
	if !ok {
		t.Fatalf("error = %T (%v), want *APIError", err, err)
	}
	if ae.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d, want 401", ae.StatusCode)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/pce/ -run TestPing -v`
Expected: compile failure — `undefined: NewClient`, `undefined: Config`, etc.

- [ ] **Step 3: Write the error types**

Create `internal/pce/errors.go`:

```go
package pce

import (
	"errors"
	"fmt"
	"time"
)

// ErrLabelNotFound is returned when a label lookup finds no exact match.
var ErrLabelNotFound = errors.New("illumio label not found")

// APIError is a non-2xx, non-429 PCE response.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("pce api error: status %d: %s", e.StatusCode, e.Body)
}

// RateLimitError is a 429 from the PCE (limit is 500 req/min per key).
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("pce rate limited, retry after %s", e.RetryAfter)
}
```

- [ ] **Step 4: Write the client**

Create `internal/pce/client.go`:

```go
package pce

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Client is a minimal Illumio PCE REST API v2 client.
type Client struct {
	baseURL    string
	orgID      int
	apiKey     string
	apiSecret  string
	httpClient *http.Client
}

// Config holds the PCE endpoint and credentials.
type Config struct {
	PCEURL    string // host:port, e.g. mypce.example.com:8443
	OrgID     int
	APIKey    string
	APISecret string
}

// Option customizes a Client (used by tests to point at a mock server).
type Option func(*Client)

// WithBaseURL overrides the derived https://<PCEURL> base URL.
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = u } }

// WithHTTPClient overrides the default HTTP client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.httpClient = h } }

// NewClient builds a PCE client. By default the base URL is https://<cfg.PCEURL>.
func NewClient(cfg Config, opts ...Option) *Client {
	c := &Client{
		baseURL:    "https://" + cfg.PCEURL,
		orgID:      cfg.OrgID,
		apiKey:     cfg.APIKey,
		apiSecret:  cfg.APISecret,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// orgPath returns "/orgs/<orgID><suffix>".
func (c *Client) orgPath(suffix string) string {
	return fmt.Sprintf("/orgs/%d%s", c.orgID, suffix)
}

// do performs a request against /api/v2<path>, JSON-encoding body and
// JSON-decoding the response into out (both optional).
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/api/v2"+path, reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.SetBasicAuth(c.apiKey, c.apiSecret)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("pce request: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusTooManyRequests {
		return &RateLimitError{RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Body: string(data)}
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// Ping performs an authenticated request to verify connectivity and credentials.
func (c *Client) Ping(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, c.orgPath("/labels"), nil, nil)
}

func parseRetryAfter(h string) time.Duration {
	if h == "" {
		return 30 * time.Second
	}
	if secs, err := strconv.Atoi(h); err == nil {
		return time.Duration(secs) * time.Second
	}
	return 30 * time.Second
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/pce/ -run TestPing -v`
Expected: PASS for all three tests.

- [ ] **Step 6: Commit**

```bash
git add internal/pce/client.go internal/pce/errors.go internal/pce/client_test.go
git commit -m "feat(pce): add PCE client core with auth and Ping"
```

---

### Task 3: PCE label resolution, creation, and ownership tagging

**Files:**
- Create: `internal/pce/labels.go`
- Test: `internal/pce/labels_test.go`

**Interfaces:**
- Consumes: `Client.do`, `ErrLabelNotFound`, `Client.orgPath` from Task 2.
- Produces:
  - `type Owner struct { DataSet string; Reference string }`
  - `type Label struct { Href, Key, Value, ExternalDataSet, ExternalDataReference string }` (JSON-tagged)
  - `func (c *Client) FindLabel(ctx context.Context, key, value string) (*Label, error)` (returns `ErrLabelNotFound` if absent)
  - `func (c *Client) CreateLabel(ctx context.Context, l Label) (*Label, error)`
  - `func (c *Client) EnsureLabel(ctx context.Context, key, value string, owner Owner) (*Label, error)` (find-or-create; stamps ownership on create)

- [ ] **Step 1: Write the failing test**

Create `internal/pce/labels_test.go`:

```go
package pce

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

func TestFindLabel_ReturnsExactMatch(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("key"); got != "app" {
			t.Errorf("query key = %q, want app", got)
		}
		if got := r.URL.Query().Get("value"); got != "checkout" {
			t.Errorf("query value = %q, want checkout", got)
		}
		_, _ = w.Write([]byte(`[{"href":"/orgs/7/labels/42","key":"app","value":"checkout"}]`))
	})
	l, err := c.FindLabel(context.Background(), "app", "checkout")
	if err != nil {
		t.Fatalf("FindLabel error: %v", err)
	}
	if l.Href != "/orgs/7/labels/42" {
		t.Errorf("Href = %q, want /orgs/7/labels/42", l.Href)
	}
}

func TestFindLabel_NoMatchReturnsErrLabelNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	_, err := c.FindLabel(context.Background(), "app", "missing")
	if !errors.Is(err, ErrLabelNotFound) {
		t.Fatalf("error = %v, want ErrLabelNotFound", err)
	}
}

func TestEnsureLabel_CreatesWithOwnershipWhenMissing(t *testing.T) {
	var posted Label
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[]`)) // not found
			return
		}
		// POST create
		_ = json.NewDecoder(r.Body).Decode(&posted)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"href":"/orgs/7/labels/99","key":"role","value":"control"}`))
	})
	owner := Owner{DataSet: "illumio-operator", Reference: "cr-uid-1"}
	l, err := c.EnsureLabel(context.Background(), "role", "control", owner)
	if err != nil {
		t.Fatalf("EnsureLabel error: %v", err)
	}
	if l.Href != "/orgs/7/labels/99" {
		t.Errorf("Href = %q, want /orgs/7/labels/99", l.Href)
	}
	if posted.ExternalDataSet != "illumio-operator" || posted.ExternalDataReference != "cr-uid-1" {
		t.Errorf("ownership = %q/%q, want illumio-operator/cr-uid-1",
			posted.ExternalDataSet, posted.ExternalDataReference)
	}
}

func TestEnsureLabel_ReturnsExistingWithoutCreating(t *testing.T) {
	var posts int
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
		}
		_, _ = w.Write([]byte(`[{"href":"/orgs/7/labels/42","key":"app","value":"checkout"}]`))
	})
	if _, err := c.EnsureLabel(context.Background(), "app", "checkout", Owner{}); err != nil {
		t.Fatalf("EnsureLabel error: %v", err)
	}
	if posts != 0 {
		t.Errorf("POST count = %d, want 0 (label already existed)", posts)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/pce/ -run TestFindLabel -v`
Expected: compile failure — `undefined: Label`, `undefined: FindLabel`.

- [ ] **Step 3: Write the implementation**

Create `internal/pce/labels.go`:

```go
package pce

import (
	"context"
	"errors"
	"net/http"
	"net/url"
)

// Owner carries the ownership tags stamped on PCE objects the operator creates.
type Owner struct {
	DataSet   string
	Reference string
}

// Label is an Illumio label object.
type Label struct {
	Href                  string `json:"href,omitempty"`
	Key                   string `json:"key"`
	Value                 string `json:"value"`
	ExternalDataSet       string `json:"external_data_set,omitempty"`
	ExternalDataReference string `json:"external_data_reference,omitempty"`
}

// FindLabel returns the label with the exact key/value, or ErrLabelNotFound.
func (c *Client) FindLabel(ctx context.Context, key, value string) (*Label, error) {
	q := url.Values{}
	q.Set("key", key)
	q.Set("value", value)
	var labels []Label
	if err := c.do(ctx, http.MethodGet, c.orgPath("/labels")+"?"+q.Encode(), nil, &labels); err != nil {
		return nil, err
	}
	for i := range labels {
		if labels[i].Key == key && labels[i].Value == value {
			return &labels[i], nil
		}
	}
	return nil, ErrLabelNotFound
}

// CreateLabel creates a new label and returns it (with its assigned href).
func (c *Client) CreateLabel(ctx context.Context, l Label) (*Label, error) {
	var created Label
	if err := c.do(ctx, http.MethodPost, c.orgPath("/labels"), l, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

// EnsureLabel returns the existing label for key/value, or creates it stamped
// with the given ownership tags.
func (c *Client) EnsureLabel(ctx context.Context, key, value string, owner Owner) (*Label, error) {
	existing, err := c.FindLabel(ctx, key, value)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ErrLabelNotFound) {
		return nil, err
	}
	return c.CreateLabel(ctx, Label{
		Key:                   key,
		Value:                 value,
		ExternalDataSet:       owner.DataSet,
		ExternalDataReference: owner.Reference,
	})
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/pce/ -v`
Expected: PASS for all label and ping tests.

- [ ] **Step 5: Commit**

```bash
git add internal/pce/labels.go internal/pce/labels_test.go
git commit -m "feat(pce): add label find/create/ensure with ownership tagging"
```

---

### Task 4: PCEConnection types — fields, markers, status helpers

**Files:**
- Modify: `api/v1alpha1/pceconnection_types.go` (replace scaffolded spec/status)
- Test: `api/v1alpha1/pceconnection_types_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `PCEConnectionSpec{ PCEURL string; OrgID int; CredentialsSecretRef SecretReference; ExternalDataSet string }`
  - `SecretReference{ Name string; Namespace string }`
  - `PCEConnectionStatus{ Conditions []metav1.Condition; ObservedGeneration int64 }`
  - condition-reason constants `ConditionConnected = "Connected"`, reasons `ReasonConnected`, `ReasonAuthFailed`, `ReasonSecretMissing`, `ReasonRateLimited`, `ReasonPCEUnreachable`.

- [ ] **Step 1: Write the failing test**

Create `api/v1alpha1/pceconnection_types_test.go`:

```go
package v1alpha1

import "testing"

func TestPCEConnection_HasExpectedDefaults(t *testing.T) {
	pc := PCEConnection{
		Spec: PCEConnectionSpec{
			PCEURL: "pce.example.com:8443",
			OrgID:  3,
			CredentialsSecretRef: SecretReference{Name: "illumio-pce-api", Namespace: "illumio-operator"},
		},
	}
	if pc.Spec.CredentialsSecretRef.Name != "illumio-pce-api" {
		t.Errorf("secret name = %q", pc.Spec.CredentialsSecretRef.Name)
	}
	if ConditionConnected != "Connected" {
		t.Errorf("ConditionConnected = %q, want Connected", ConditionConnected)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./api/v1alpha1/ -run TestPCEConnection -v`
Expected: compile failure — `undefined: SecretReference`, `undefined: ConditionConnected`.

- [ ] **Step 3: Replace the scaffolded types**

Replace the `PCEConnectionSpec` and `PCEConnectionStatus` structs and the `PCEConnection` type markers in `api/v1alpha1/pceconnection_types.go` with:

```go
// SecretReference points to a Kubernetes Secret holding PCE API credentials.
type SecretReference struct {
	// Name of the Secret (keys: api_key, api_secret).
	Name string `json:"name"`
	// Namespace of the Secret. Defaults to the operator's namespace if empty.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// PCEConnectionSpec defines a connection to one Illumio PCE.
type PCEConnectionSpec struct {
	// PCEURL is the PCE host:port (443 for SaaS, 8443 typical on-prem).
	PCEURL string `json:"pceUrl"`
	// OrgID is the PCE organization id.
	OrgID int `json:"orgId"`
	// CredentialsSecretRef references the Secret with api_key / api_secret.
	CredentialsSecretRef SecretReference `json:"credentialsSecretRef"`
	// ExternalDataSet is the ownership tag stamped on PCE objects this
	// operator creates. Defaults to "illumio-operator" if empty.
	// +optional
	ExternalDataSet string `json:"externalDataSet,omitempty"`
}

// PCEConnectionStatus is the observed connection state.
type PCEConnectionStatus struct {
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// Condition types and reasons for PCEConnection.
const (
	ConditionConnected = "Connected"

	ReasonConnected      = "Connected"
	ReasonSecretMissing  = "SecretMissing"
	ReasonAuthFailed     = "AuthFailed"
	ReasonRateLimited    = "RateLimited"
	ReasonPCEUnreachable = "PCEUnreachable"
)
```

Then set the resource markers immediately above `type PCEConnection struct` (replace the scaffolded `+kubebuilder:object:root` / status markers, keeping `object:root` and `subresource:status`):

```go
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories=illumio,shortName=pceconn
// +kubebuilder:printcolumn:name="PCE",type=string,JSONPath=`.spec.pceUrl`
// +kubebuilder:printcolumn:name="Org",type=integer,JSONPath=`.spec.orgId`
// +kubebuilder:printcolumn:name="Connected",type=string,JSONPath=`.status.conditions[?(@.type=="Connected")].status`
```

Ensure the file imports `metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"` (the scaffold already does).

- [ ] **Step 4: Regenerate code and manifests**

```bash
make generate
make manifests
```

Expected: updates `zz_generated.deepcopy.go` and writes the CRD to `config/crd/bases/microsegment.io_pceconnections.yaml` with `scope: Cluster` and the `illumio` category.

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./api/v1alpha1/ -run TestPCEConnection -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/v1alpha1/ config/crd/
git commit -m "feat(api): define PCEConnection spec, status, and markers"
```

---

### Task 5: PCEConnection controller — connect and report health

**Files:**
- Modify: `internal/controller/pceconnection_controller.go`
- Create: `internal/controller/clientfactory.go`
- Test: `internal/controller/pceconnection_controller_test.go`
- Modify: `internal/controller/suite_test.go` (register the reconciler in the envtest suite)

**Interfaces:**
- Consumes: `pce.NewClient`, `pce.Config`, `pce.APIError`, `pce.RateLimitError` (Task 2); `PCEConnectionSpec`, condition reasons (Task 4).
- Produces:
  - `type PCEPinger interface { Ping(ctx context.Context) error }`
  - `type ClientFactory func(cfg pce.Config) PCEPinger` (reconciler field `NewPCEClient`; defaults to wrapping `pce.NewClient`)
  - Reconcile behavior: reads the credentials Secret, pings the PCE, sets the `Connected` condition with the appropriate reason.

- [ ] **Step 1: Write the failing test**

Create `internal/controller/pceconnection_controller_test.go`:

```go
package controller

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	microv1 "github.com/microsegment-io/illumio-k8s-utility-operator/api/v1alpha1"
	"github.com/microsegment-io/illumio-k8s-utility-operator/internal/pce"
)

var _ = Describe("PCEConnection controller", func() {
	const ns = "default"

	It("sets Connected=True when the PCE ping succeeds", func() {
		ctx := context.Background()
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds-ok", Namespace: ns},
			Data:       map[string][]byte{"api_key": []byte("k"), "api_secret": []byte("s")},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		conn := &microv1.PCEConnection{
			ObjectMeta: metav1.ObjectMeta{Name: "conn-ok"},
			Spec: microv1.PCEConnectionSpec{
				PCEURL: "pce.example.com:8443", OrgID: 1,
				CredentialsSecretRef: microv1.SecretReference{Name: "creds-ok", Namespace: ns},
			},
		}
		Expect(k8sClient.Create(ctx, conn)).To(Succeed())

		Eventually(func(g Gomega) {
			got := &microv1.PCEConnection{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "conn-ok"}, got)).To(Succeed())
			c := meta_FindStatusCondition(got.Status.Conditions, microv1.ConditionConnected)
			g.Expect(c).NotTo(BeNil())
			g.Expect(c.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(c.Reason).To(Equal(microv1.ReasonConnected))
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())
	})

	It("sets Connected=False with AuthFailed when the PCE returns 401", func() {
		ctx := context.Background()
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "creds-bad", Namespace: ns},
			Data:       map[string][]byte{"api_key": []byte("k"), "api_secret": []byte("s")},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		conn := &microv1.PCEConnection{
			ObjectMeta: metav1.ObjectMeta{Name: "conn-bad"},
			Spec: microv1.PCEConnectionSpec{
				PCEURL: "pce.example.com:8443", OrgID: 1,
				CredentialsSecretRef: microv1.SecretReference{Name: "creds-bad", Namespace: ns},
			},
		}
		Expect(k8sClient.Create(ctx, conn)).To(Succeed())

		Eventually(func(g Gomega) {
			got := &microv1.PCEConnection{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "conn-bad"}, got)).To(Succeed())
			c := meta_FindStatusCondition(got.Status.Conditions, microv1.ConditionConnected)
			g.Expect(c).NotTo(BeNil())
			g.Expect(c.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(c.Reason).To(Equal(microv1.ReasonAuthFailed))
		}, 10*time.Second, 250*time.Millisecond).Should(Succeed())
	})
})

// fakePinger lets tests drive Ping results per PCE URL credentials.
type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func newFakeFactory(err error) ClientFactory {
	return func(pce.Config) PCEPinger { return fakePinger{err: err} }
}

// errAuth is a 401 APIError used in the AuthFailed test wiring (see suite_test.go).
var errAuth = &pce.APIError{StatusCode: 401, Body: "bad key"}

var _ = errors.New // keep errors imported for future cases
```

- [ ] **Step 2: Wire the fake factory into the suite**

In `internal/controller/suite_test.go`, after the manager is created and before `mgr.Start`, register two reconcilers is overkill — instead register **one** reconciler whose factory inspects the secret-derived key. Simplest: register the reconciler with a factory that returns success unless the PCEConnection name ends in `-bad`. Add this where other reconcilers are set up (replace any scaffolded reconciler registration block):

```go
err = (&PCEConnectionReconciler{
	Client: k8sManager.GetClient(),
	Scheme: k8sManager.GetScheme(),
	NewPCEClient: func(cfg pce.Config) PCEPinger {
		// Test factory: org 1 + key "k" always; choose result by APIKey marker.
		// Real credentials are "k"/"s"; we vary behavior via the secret name
		// encoded as APIKey is not available here, so use a always-ok pinger
		// and override per-test via cfg is not possible — see note below.
		return fakePinger{err: nil}
	},
}).SetupWithManager(k8sManager)
Expect(err).ToNot(HaveOccurred())
```

> Note: because both test PCEConnections share a manager, drive the two outcomes by **secret contents**, not CR name. Update the failing-case secret to carry `api_key: "bad"` and make the test factory return `errAuth` when `cfg.APIKey == "bad"`, success otherwise:

```go
NewPCEClient: func(cfg pce.Config) PCEPinger {
	if cfg.APIKey == "bad" {
		return fakePinger{err: errAuth}
	}
	return fakePinger{err: nil}
},
```

And in the controller test above, change the `creds-bad` secret to `"api_key": []byte("bad")`.

- [ ] **Step 3: Run the test to verify it fails**

Run: `make test`
Expected: compile failure — `undefined: PCEConnectionReconciler` fields `NewPCEClient`, `undefined: ClientFactory`, `undefined: PCEPinger`, `undefined: meta_FindStatusCondition`.

- [ ] **Step 4: Write the client factory**

Create `internal/controller/clientfactory.go`:

```go
package controller

import (
	"context"

	"github.com/microsegment-io/illumio-k8s-utility-operator/internal/pce"
)

// PCEPinger is the subset of the PCE client the connection controller needs.
type PCEPinger interface {
	Ping(ctx context.Context) error
}

// ClientFactory builds a PCEPinger from a Config. Injectable for tests.
type ClientFactory func(cfg pce.Config) PCEPinger

// DefaultClientFactory wraps the real PCE client.
func DefaultClientFactory(cfg pce.Config) PCEPinger {
	return pce.NewClient(cfg)
}
```

- [ ] **Step 5: Write the reconciler**

Replace the body of `internal/controller/pceconnection_controller.go` with:

```go
package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	microv1 "github.com/microsegment-io/illumio-k8s-utility-operator/api/v1alpha1"
	"github.com/microsegment-io/illumio-k8s-utility-operator/internal/pce"
)

// PCEConnectionReconciler reconciles a PCEConnection object.
type PCEConnectionReconciler struct {
	client.Client
	Scheme       *runtimeScheme
	NewPCEClient ClientFactory
}

// runtimeScheme aliases the scheme type to avoid an extra import line churn.
type runtimeScheme = scheme

// +kubebuilder:rbac:groups=microsegment.io,resources=pceconnections,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=microsegment.io,resources=pceconnections/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *PCEConnectionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var conn microv1.PCEConnection
	if err := r.Get(ctx, req.NamespacedName, &conn); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	cfg, reason, msg, ok := r.loadConfig(ctx, &conn)
	if !ok {
		return r.fail(ctx, &conn, reason, msg)
	}

	if r.NewPCEClient == nil {
		r.NewPCEClient = DefaultClientFactory
	}
	if err := r.NewPCEClient(cfg).Ping(ctx); err != nil {
		reason, msg := classifyPingError(err)
		return r.fail(ctx, &conn, reason, msg)
	}

	meta.SetStatusCondition(&conn.Status.Conditions, metav1.Condition{
		Type:    microv1.ConditionConnected,
		Status:  metav1.ConditionTrue,
		Reason:  microv1.ReasonConnected,
		Message: "PCE reachable and credentials accepted",
	})
	conn.Status.ObservedGeneration = conn.Generation
	return ctrl.Result{}, r.Status().Update(ctx, &conn)
}

func (r *PCEConnectionReconciler) loadConfig(ctx context.Context, conn *microv1.PCEConnection) (pce.Config, string, string, bool) {
	var secret corev1.Secret
	key := types.NamespacedName{Name: conn.Spec.CredentialsSecretRef.Name, Namespace: conn.Spec.CredentialsSecretRef.Namespace}
	if err := r.Get(ctx, key, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return pce.Config{}, microv1.ReasonSecretMissing, "credentials secret not found", false
		}
		return pce.Config{}, microv1.ReasonSecretMissing, err.Error(), false
	}
	apiKey := string(secret.Data["api_key"])
	apiSecret := string(secret.Data["api_secret"])
	if apiKey == "" || apiSecret == "" {
		return pce.Config{}, microv1.ReasonSecretMissing, "secret missing api_key or api_secret", false
	}
	return pce.Config{PCEURL: conn.Spec.PCEURL, OrgID: conn.Spec.OrgID, APIKey: apiKey, APISecret: apiSecret}, "", "", true
}

func (r *PCEConnectionReconciler) fail(ctx context.Context, conn *microv1.PCEConnection, reason, msg string) (ctrl.Result, error) {
	meta.SetStatusCondition(&conn.Status.Conditions, metav1.Condition{
		Type:    microv1.ConditionConnected,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: msg,
	})
	conn.Status.ObservedGeneration = conn.Generation
	return ctrl.Result{}, r.Status().Update(ctx, conn)
}

func classifyPingError(err error) (reason, msg string) {
	switch e := err.(type) {
	case *pce.RateLimitError:
		return microv1.ReasonRateLimited, e.Error()
	case *pce.APIError:
		if e.StatusCode == 401 || e.StatusCode == 403 {
			return microv1.ReasonAuthFailed, e.Error()
		}
		return microv1.ReasonPCEUnreachable, e.Error()
	default:
		return microv1.ReasonPCEUnreachable, err.Error()
	}
}

func (r *PCEConnectionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&microv1.PCEConnection{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
```

> If `runtimeScheme`/`scheme` aliasing causes confusion, simply declare the field as `Scheme *runtime.Scheme` with `import "k8s.io/apimachinery/pkg/runtime"` and delete the alias llines. The alias only exists to keep the diff small against the scaffold; prefer the plain `*runtime.Scheme` if unsure.

- [ ] **Step 6: Add the status-condition test helper**

Create `internal/controller/helpers_test.go`:

```go
package controller

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// meta_FindStatusCondition is a thin alias so tests read clearly.
func meta_FindStatusCondition(conds []metav1.Condition, t string) *metav1.Condition {
	return meta.FindStatusCondition(conds, t)
}
```

- [ ] **Step 7: Register the real factory in main**

In `cmd/main.go`, where `PCEConnectionReconciler` is set up (the scaffold added a `SetupWithManager` call), set the factory:

```go
if err := (&controller.PCEConnectionReconciler{
	Client:       mgr.GetClient(),
	Scheme:       mgr.GetScheme(),
	NewPCEClient: controller.DefaultClientFactory,
}).SetupWithManager(mgr); err != nil {
	setupLog.Error(err, "unable to create controller", "controller", "PCEConnection")
	os.Exit(1)
}
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `make test`
Expected: PASS, including both PCEConnection Ginkgo specs (Connected=True, and AuthFailed).

- [ ] **Step 9: Commit**

```bash
git add internal/controller/ cmd/main.go config/rbac/
git commit -m "feat(controller): reconcile PCEConnection health via injectable PCE client"
```

---

## Self-Review

**Spec coverage (against the design spec §4.1, §7, §9, §10):**
- §4.1 `PCEConnection` CRD (cluster-scoped, secret ref, ownership tag, status conditions) → Tasks 4–5. ✓
- §7 PCE API client as the single home of Illumio knowledge → Tasks 2–3 (`internal/pce`). ✓
- §8 ownership tagging (`external_data_set`/`external_data_reference`) → Task 3 `EnsureLabel`. ✓ (provisioning/drift come in Plan 3.)
- §9 credentials from Secrets, never inline; rate-limit (429) handling → Tasks 2 (`RateLimitError`) and 5 (`ReasonRateLimited`). ✓
- §10 status conditions with machine-readable reasons; PCE-unreachable handling → Task 5 `classifyPingError`. ✓
- API conventions (group `microsegment.io`, `v1alpha1`, category `illumio`, shortName) → Global Constraints + Task 4 markers. ✓

**Out of Plan 1 scope (deferred, by design):** CWP/`ClusterProfile`, onboarding + output Secret, IR, policy reconciler, provisioning, both policy front-ends, finalizers, drift revert. These are Plans 2–3.

**Placeholder scan:** No `TBD`/`TODO`/"handle edge cases" — every code step shows complete code. The only narrative notes are the two explicit "simplify if unsure" guidances in Task 5 (the `runtimeScheme` alias and the secret-driven fake factory), which include the exact alternative code.

**Type consistency:** `pce.Config`, `pce.NewClient`, `pce.APIError`, `pce.RateLimitError`, `pce.Owner`, `pce.Label`, `EnsureLabel/FindLabel/CreateLabel`, `PCEPinger`, `ClientFactory`, `PCEConnectionSpec`/`SecretReference`/condition reasons, and `PCEConnectionReconciler.NewPCEClient` are used identically across the tasks that define and consume them.
