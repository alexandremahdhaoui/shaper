# Shaper Architecture

**A layered, interface-driven architecture for Kubernetes-native iPXE boot configuration management.**

> "Understanding how Shaper's layers connect helped me contribute my first PR in a day.
> The separation between adapters and controllers makes testing straightforward."
> - New Contributor

## Problem

Shaper's codebase follows a layered architecture that can be overwhelming for newcomers.
This document explains how components connect, how requests flow through the system,
and where to find specific functionality. Use this as your map when exploring or modifying Shaper.

## Contents

- [How is the codebase structured?](#how-is-the-codebase-structured)
- [How does the iPXE boot flow work?](#how-does-the-ipxe-boot-flow-work)
- [What are the main components?](#what-are-the-main-components)
- [How do the CRDs work?](#how-do-the-crds-work)
- [What code patterns are used?](#what-code-patterns-are-used)
- [How is testing organized?](#how-is-testing-organized)
- [Links](#links)

---

## How is the codebase structured?

Shaper uses a **layered architecture** with dependencies flowing inward:

```
+------------------------------------------------------------------+
|                      Driver Layer                                 |
|   HTTP handlers, webhooks, external interfaces                    |
|   internal/driver/server, internal/driver/webhook                 |
+------------------------------------------------------------------+
                              |
                              v
+------------------------------------------------------------------+
|                    Controller Layer                               |
|   Business logic, orchestration, template rendering               |
|   internal/controller/ipxe, content, resolvetransformermux        |
+------------------------------------------------------------------+
                              |
                              v
+------------------------------------------------------------------+
|                     Adapter Layer                                 |
|   Data access, K8s integration, external services                 |
|   internal/adapter/assignment, profile, resolver, transformer     |
+------------------------------------------------------------------+
                              |
                              v
+------------------------------------------------------------------+
|                      Types Layer                                  |
|   Domain models, CRD types                                        |
|   internal/types, pkg/v1alpha1                                    |
+------------------------------------------------------------------+
```

**Dependency Rule**: Each layer depends only on the layers below it. Drivers depend on
controllers, controllers depend on adapters, adapters depend on types.

**Package Structure**:
```
shaper/
+-- cmd/                    # Binary entry points
|   +-- shaper-api/         # REST API server
|   +-- shaper-controller/  # K8s controller
|   +-- shaper-webhook/     # Admission webhooks
|   +-- shaper-tftp/        # TFTP server
+-- internal/               # Private application code
|   +-- adapter/            # Data access (K8s, webhooks)
|   +-- controller/         # Business logic
|   +-- driver/             # HTTP handlers, webhooks
|   +-- types/              # Domain models
|   +-- util/               # Utilities
+-- pkg/                    # Public packages
|   +-- v1alpha1/           # CRD types
|   +-- generated/          # OpenAPI generated code
+-- charts/                 # Helm charts
    +-- shaper-crds/        # CRD definitions
    +-- shaper-api/         # API server deployment
    +-- shaper-webhooks/    # Admission webhooks
    +-- shaper-controller/  # Controller deployment
```

---

## How does the iPXE boot flow work?

The boot flow has four phases: BOOTSTRAP, ASSIGNMENT, BOOT, and CONFIG (optional).

```
+---------------+                                +------------------+
|   Booting     |                                |    shaper-api    |
|   Machine     |                                |    HTTP Server   |
+---------------+                                +------------------+
       |                                                  |
       |  Phase 1: BOOTSTRAP                              |
       |  GET /boot.ipxe                                  |
       |------------------------------------------------->|
       |                                                  |
       |  Response: Cached bootstrap script               |
       |  #!ipxe                                          |
       |  dhcp                                            |
       |  chain /ipxe?uuid=${uuid}&buildarch=${buildarch} |
       |<-------------------------------------------------|
       |                                                  |
       |  Phase 2: ASSIGNMENT                             |
       |  GET /ipxe?uuid=XXX&buildarch=amd64              |
       |------------------------------------------------->|
       |                                                  |
       |           +----------------------------------+   |
       |           | IPXE Controller                  |   |
       |           | 1. Query Assignment by selectors |   |
       |           | 2. Fallback to default if none   |   |
       |           | 3. Get Profile from Assignment   |   |
       |           | 4. Resolve additional content    |   |
       |           | 5. Render iPXE template          |   |
       |           +----------------------------------+   |
       |                                                  |
       |  Response: Rendered iPXE script                  |
       |  (may contain /config/{uuid} URLs)               |
       |<-------------------------------------------------|
       |                                                  |
       |  Phase 3: BOOT                                   |
       |  Machine executes iPXE commands                  |
       |                                                  |
       |  Phase 4: CONFIG (optional)                      |
       |  GET /config/{uuid}?buildarch=amd64              |
       |------------------------------------------------->|
       |                                                  |
       |           +----------------------------------+   |
       |           | Content Controller              |   |
       |           | 1. Lookup Profile by UUID       |   |
       |           | 2. Resolve content source       |   |
       |           | 3. Apply transformations        |   |
       |           +----------------------------------+   |
       |                                                  |
       |  Response: Configuration file                    |
       |  (Ignition, Cloud-Init, etc.)                    |
       |<-------------------------------------------------|
       |                                                  |
       v                                                  v
   Machine boots OS
```

**Assignment Selection Logic**:
1. Try to find Assignment matching machine UUID and buildarch
2. If not found, try to find default Assignment for buildarch
3. If still not found, return error

**Content Resolution Pipeline**:
1. Determine source type (Inline, ObjectRef, or Webhook)
2. Resolve content from source
3. Apply post-transformations (e.g., Butane to Ignition)
4. Return final content

---

## What are the main components?

### Binaries (cmd/)

| Binary | Status | Purpose |
|--------|--------|---------|
| shaper-api | Implemented | REST API for iPXE boot scripts and configs |
| shaper-controller | Implemented | K8s controller for Profile/Assignment reconciliation |
| shaper-webhook | Implemented | Admission webhooks for CRD validation/mutation |
| shaper-tftp | Implemented | TFTP server for initial chainloading |

### Driver Layer (internal/driver/)

**HTTP Server Driver** (`internal/driver/server/server.go`)
- Implements OpenAPI-generated `StrictServerInterface`
- Handles `/boot.ipxe`, `/ipxe`, and `/config/{uuid}` endpoints
- Delegates to IPXE and Content controllers

**Webhook Driver** (`internal/driver/webhook/`)
- Validates Profile content sources (exactly one per content)
- Validates Assignment UUIDs, buildarch, default rules
- Mutates Assignments to add UUID and buildarch labels
- Mutates Profiles to add UUID labels for exposed content

**TFTP Driver** (`internal/driver/tftp/`)
- Serves iPXE chainload files via TFTP protocol
- Enables initial network boot before HTTP handoff

### Controller Layer (internal/controller/)

**IPXE Controller** (`internal/controller/ipxe.go`)
```go
type IPXE interface {
    FindProfileAndRender(ctx context.Context, selectors types.IPXESelectors) ([]byte, error)
    Bootstrap() []byte
}
```
- Orchestrates assignment selection and profile rendering
- Caches bootstrap script for performance
- Uses Go templates for iPXE script rendering

**Content Controller** (`internal/controller/content.go`)
```go
type Content interface {
    GetByID(ctx context.Context, contentID uuid.UUID, attributes types.IPXESelectors) ([]byte, error)
}
```
- Retrieves configuration files by UUID
- Delegates resolution and transformation to ResolveTransformerMux

**ResolveTransformerMux** (`internal/controller/resolvetransformermux.go`)
```go
type ResolveTransformerMux interface {
    ResolveAndTransform(ctx context.Context, content types.Content, selectors types.IPXESelectors) ([]byte, error)
    ResolveAndTransformBatch(ctx context.Context, contents map[string]types.Content, selectors types.IPXESelectors, opts ...ResolveTransformBatchOption) (map[string][]byte, error)
}
```
- Routes to appropriate resolver (Inline, ObjectRef, Webhook)
- Chains multiple transformers (e.g., Butane to Ignition)
- Supports batch processing for multiple contents

**Reconcilers** (`internal/controller/reconciler/`)
- **ProfileReconciler**: Generates UUIDs for exposed content, updates Profile status
- **AssignmentReconciler**: Adds subject selector labels to Assignments

### Adapter Layer (internal/adapter/)

**Assignment Adapter** (`internal/adapter/assignment.go`)
```go
type Assignment interface {
    FindDefaultByBuildarch(ctx context.Context, buildarch string) (types.Assignment, error)
    FindBySelectors(ctx context.Context, selectors types.IPXESelectors) (types.Assignment, error)
}
```
- Queries Assignment CRDs using label selectors
- Returns `ErrAssignmentNotFound` when no match

**Profile Adapter** (`internal/adapter/profile.go`)
```go
type Profile interface {
    Get(ctx context.Context, name string) (types.Profile, error)
    GetInNamespace(ctx context.Context, name, namespace string) (types.Profile, error)
    ListByContentID(ctx context.Context, contentID uuid.UUID) ([]types.Profile, error)
}
```
- Converts CRD types to internal domain types
- Parses JSONPath expressions for ObjectRef

**Resolvers** (`internal/adapter/resolver.go`)
```go
type Resolver interface {
    Resolve(ctx context.Context, content types.Content, attributes types.IPXESelectors) ([]byte, error)
}
```
- **InlineResolver**: Returns content directly from spec
- **ObjectRefResolver**: Fetches from K8s objects (ConfigMaps, Secrets) using JSONPath
- **WebhookResolver**: Calls external HTTP endpoints with mTLS support

**Transformers** (`internal/adapter/transformer.go`)
```go
type Transformer interface {
    Transform(ctx context.Context, cfg types.TransformerConfig, content []byte, selectors types.IPXESelectors) ([]byte, error)
}
```
- **ButaneTransformer**: Converts Butane YAML to Ignition JSON
- **WebhookTransformer**: Calls external transformation webhooks

---

## How do the CRDs work?

### Profile CRD

**Purpose**: Defines an iPXE boot profile with template and additional content.

**API Version**: `shaper.amahdha.com/v1alpha1`

**Spec Fields**:
| Field | Type | Description |
|-------|------|-------------|
| `spec.ipxeTemplate` | string | Go template for iPXE script |
| `spec.additionalContent` | []AdditionalContent | Content that can be templated into iPXE |

**AdditionalContent Fields**:
| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Content identifier for templating |
| `exposed` | bool | If true, content available via `/config/{uuid}` |
| `postTransformations` | []Transformer | Transformations to apply |
| `inline` | *string | Direct content (mutually exclusive) |
| `objectRef` | *ObjectRef | K8s object reference (mutually exclusive) |
| `webhook` | *WebhookConfig | External webhook (mutually exclusive) |

**Status Fields**:
| Field | Type | Description |
|-------|------|-------------|
| `status.exposedAdditionalContent` | map[string]string | Maps content names to UUIDs |

**Example**:
```yaml
apiVersion: shaper.amahdha.com/v1alpha1
kind: Profile
metadata:
  name: flatcar-linux
spec:
  ipxeTemplate: |
    #!ipxe
    kernel http://stable.release.flatcar-linux.net/amd64-usr/current/flatcar_production_pxe.vmlinuz
    initrd http://stable.release.flatcar-linux.net/amd64-usr/current/flatcar_production_pxe_image.cpio.gz
    initrd {{ .AdditionalContent.ignition }}
    boot
  additionalContent:
    - name: ignition
      exposed: true
      postTransformations:
        - butaneToIgnition: true
      inline: |
        variant: flatcar
        version: 1.0.0
        storage:
          files:
            - path: /etc/hostname
              contents:
                inline: flatcar-node
```

### Assignment CRD

**Purpose**: Maps machines to profiles based on selectors.

**API Version**: `shaper.amahdha.com/v1alpha1`

**Spec Fields**:
| Field | Type | Description |
|-------|------|-------------|
| `spec.subjectSelectors.buildarch` | []Buildarch | Target architectures (i386, x86_64, arm32, arm64) |
| `spec.subjectSelectors.uuidList` | []string | Target machine UUIDs |
| `spec.profileName` | string | Name of Profile to assign |
| `spec.isDefault` | bool | Default assignment for buildarch |

**Example**:
```yaml
apiVersion: shaper.amahdha.com/v1alpha1
kind: Assignment
metadata:
  name: default-amd64
spec:
  subjectSelectors:
    buildarch:
      - x86_64
  profileName: flatcar-linux
  isDefault: true
---
apiVersion: shaper.amahdha.com/v1alpha1
kind: Assignment
metadata:
  name: specific-machine
spec:
  subjectSelectors:
    buildarch:
      - x86_64
    uuidList:
      - "550e8400-e29b-41d4-a716-446655440000"
  profileName: custom-profile
  isDefault: false
```

### Content Resolution Types

**ObjectRef**: Reference to any K8s object with JSONPath extraction.
```yaml
objectRef:
  group: ""
  version: v1
  resource: configmaps
  namespace: default
  name: boot-config
  jsonpath: ".data.config"
```

**ObjectRef JSONPath Examples**:
| Source Type | JSONPath | Description |
|-------------|----------|-------------|
| ConfigMap data | `.data.config` | Plain text value from data field |
| ConfigMap binary | `.binaryData.config` | Base64-encoded binary data |
| Secret data | `.data.tls\\.crt` | Escaped dot for keys with periods |
| Nested object | `.data.nested.key` | Nested structure navigation |
| Custom Resource | `.spec.template.data` | Any custom resource field |

**JSONPath Escaping Rules**:
- Keys containing dots must escape the dot: `.data.client\\.key` matches key `client.key`
- Keys containing slashes use standard notation: `.data.ca-bundle`
- Array access is supported: `.items[0].data`

**WebhookConfig**: External HTTP endpoint with optional mTLS or Basic Auth.
```yaml
webhook:
  url: https://config-server.example.com/generate
  mTLSRef:
    group: ""
    version: v1
    resource: secrets
    namespace: default
    name: mtls-creds
    clientKeyJSONPath: ".data.client\\.key"
    clientCertJSONPath: ".data.client\\.crt"
    caBundleJSONPath: ".data.ca\\.crt"
    tlsInsecureSkipVerify: false
```

**WebhookConfig mTLS Explanation**:
- `clientKeyJSONPath`: Path to PEM-encoded client private key
- `clientCertJSONPath`: Path to PEM-encoded client certificate
- `caBundleJSONPath`: Path to CA bundle for server verification (optional)
- `tlsInsecureSkipVerify`: Skip server certificate verification (use only for testing)

The webhook request includes machine selectors (UUID, buildarch) as query parameters,
allowing dynamic content generation based on the requesting machine.

**WebhookConfig Basic Auth Alternative**:
```yaml
webhook:
  url: https://config-server.example.com/generate
  basicAuthRef:
    group: ""
    version: v1
    resource: secrets
    namespace: default
    name: basic-auth-creds
    usernameJSONPath: ".data.username"
    passwordJSONPath: ".data.password"
```

### Assignment Selection Priority

When a machine requests an iPXE boot script, the Assignment adapter uses a priority-based
selection algorithm:

```
Selection Priority (highest to lowest):
+-----------------------------------------------+
| 1. Exact UUID match + buildarch match         |
|    - Machine UUID in Assignment.uuidList      |
|    - Buildarch in Assignment.buildarch        |
+-----------------------------------------------+
            |
            v (not found)
+-----------------------------------------------+
| 2. Default Assignment for buildarch           |
|    - Assignment.isDefault = true              |
|    - Buildarch in Assignment.buildarch        |
+-----------------------------------------------+
            |
            v (not found)
+-----------------------------------------------+
| 3. Error: No assignment found                 |
|    - Returns ErrAssignmentNotFound            |
+-----------------------------------------------+
```

**Label-Based Selection**: The AssignmentReconciler controller adds labels to Assignments
based on their spec, enabling efficient Kubernetes label-based queries:
- `shaper.amahdha.com/buildarch-{arch}`: For buildarch filtering (i386, x86_64, arm32, arm64)
- `uuid.shaper.amahdha.com/{uuid}`: For UUID-based filtering
- `shaper.amahdha.com/default-assignment`: For default assignment queries

### Profile Status Lifecycle

The ProfileReconciler manages the Profile status through these states:

```
Profile Created/Updated
        |
        v
+------------------+
| Reconcile Start  |
+------------------+
        |
        v
+----------------------------------+
| Check spec.additionalContent     |
| for exposed: true items          |
+----------------------------------+
        |
        v
+----------------------------------+
| For each exposed content:        |
| - If UUID exists in status: skip |
| - If UUID missing: generate new  |
+----------------------------------+
        |
        v
+----------------------------------+
| Update status.exposedAdditional  |
| Content with name->UUID mapping  |
+----------------------------------+
        |
        v
+------------------+
| Reconcile End    |
+------------------+
```

**Status Fields**:
- UUIDs are generated once and persist across reconciliations
- UUIDs are only removed if the corresponding content is removed from spec
- Content accessible at `/config/{uuid}` endpoint using status UUID

---

## What code patterns are used?

### Interface-Based Design

All components use interfaces for testability:

```go
// Constructor injection
func NewIPXE(assignment adapter.Assignment, profile adapter.Profile, mux ResolveTransformerMux) IPXE

// Interface allows mocking
type MockAssignment struct{ mock.Mock }
func (m *MockAssignment) FindBySelectors(ctx context.Context, sel types.IPXESelectors) (types.Assignment, error) {
    args := m.Called(ctx, sel)
    return args.Get(0).(types.Assignment), args.Error(1)
}
```

### Error Handling with errors.Join

Shaper uses Go 1.20+ `errors.Join` for error chains:

```go
var (
    ErrAssignmentNotFound = errors.New("assignment not found")  // Exported: check with errors.Is
    errAssignmentList     = errors.New("listing assignment")     // Unexported: context only
)

// Usage
return errors.Join(err, ErrAssignmentNotFound, errAssignmentList)

// Checking
if errors.Is(err, ErrAssignmentNotFound) {
    // Handle not found
}
```

### Context Propagation

All I/O operations accept `context.Context`:

```go
func (p *Profile) Get(ctx context.Context, name string) (types.Profile, error) {
    if err := p.client.Get(ctx, key, obj); err != nil {
        return types.Profile{}, err
    }
    // ...
}
```

### Functional Options

Optional parameters use the functional options pattern:

```go
type ResolveTransformBatchOption func(*resolveTransformBatchConfig)

func WithRenderOption(render bool) ResolveTransformBatchOption {
    return func(cfg *resolveTransformBatchConfig) {
        cfg.render = render
    }
}

// Usage
mux.ResolveAndTransformBatch(ctx, contents, selectors, WithRenderOption(true))
```

### Type Conversion in Adapters

Adapters convert between CRD types (`pkg/v1alpha1`) and internal types (`internal/types`):

```go
// internal/adapter/profile.go
func (p *v1a1Profile) Get(ctx context.Context, name string) (types.Profile, error) {
    obj := new(v1alpha1.Profile)
    if err := p.client.Get(ctx, key, obj); err != nil {
        return types.Profile{}, err
    }
    return fromV1alpha1.toProfile(obj)  // Convert to internal type
}
```

Controllers work exclusively with internal types, never CRD types directly.

### Sentinel Error Pattern

Shaper uses exported sentinel errors for type checking and unexported errors for context:

```go
// Exported: callers check with errors.Is()
var ErrAssignmentNotFound = errors.New("assignment not found")

// Unexported: provides context about where the error occurred
var (
    errAssignmentFindDefault     = errors.New("finding default assignment")
    errAssignmentFindBySelectors = errors.New("error finding assignment by selectors")
    errAssignmentList            = errors.New("listing assignment")
)

// Usage: wrap errors with context
func (a *assignment) FindDefaultByBuildarch(ctx context.Context, buildarch string) (types.Assignment, error) {
    if err := a.client.List(ctx, list, opts...); err != nil {
        // Chain: original error + sentinel + context
        return types.Assignment{}, errors.Join(err, ErrAssignmentNotFound, errAssignmentFindDefault)
    }
    // ...
}

// Caller checks sentinel
if errors.Is(err, adapter.ErrAssignmentNotFound) {
    // Fall back to default assignment
    return a.assignment.FindDefaultByBuildarch(ctx, selectors.Buildarch)
}
```

### Context Timeout Pattern

All external calls should respect context deadlines:

```go
func (r *WebhookResolver) Resolve(ctx context.Context, content types.Content, selectors types.IPXESelectors) ([]byte, error) {
    // Create HTTP request with context
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    if err != nil {
        return nil, err
    }

    // Context cancellation propagates to HTTP client
    resp, err := r.client.Do(req)
    if err != nil {
        // err will be context.DeadlineExceeded if timeout
        // err will be context.Canceled if parent canceled
        return nil, err
    }
    defer resp.Body.Close()
    // ...
}
```

---

## How is testing organized?

### Test Structure

```
+-- internal/adapter/
|   +-- assignment.go
|   +-- assignment_test.go       # Unit tests
+-- internal/controller/
|   +-- ipxe.go
|   +-- ipxe_test.go             # Unit tests with mocks
+-- test/
    +-- integration/             # K8s integration tests
    +-- e2e/                     # End-to-end tests
```

### Testing Patterns

**Table-Driven Tests**:
```go
func TestAssignment_FindBySelectors(t *testing.T) {
    tests := []struct {
        name          string
        selectors     types.IPXESelectors
        expected      types.Assignment
        expectedError error
    }{
        {name: "finds by UUID", selectors: types.IPXESelectors{UUID: "test"}, ...},
        {name: "returns not found", selectors: types.IPXESelectors{}, expectedError: ErrAssignmentNotFound},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

**Mocking with mockery**:
Mocks are generated in `internal/util/mocks/` for all interfaces.

**K8s Fake Client**:
```go
fakeClient := fake.NewClientBuilder().
    WithScheme(scheme).
    WithObjects(&profile).
    Build()
adapter := NewProfile(fakeClient, "default")
```

### Running Tests

```bash
# Unit tests
forge test unit run

# Integration tests (requires K8s)
go test -v -tags=integration ./test/integration/...

# E2E tests
forge test e2e run

# All tests
forge test-all

# Coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Test Coverage by Package

| Package | Coverage | Notes |
|---------|----------|-------|
| internal/adapter/* | ~70% | Table-driven tests |
| internal/controller/* | ~75% | Mocked dependencies |
| internal/driver/server | ~65% | HTTP handler tests |
| internal/driver/webhook | ~70% | Validation logic tests |
| internal/util/* | ~80% | Utility function tests |

---

## Links

### Internal Documentation

- [README](README.md) - Project overview and quick start
- [API Deployment](docs/api-deployment.md) - Production deployment guide
- [Webhook Deployment](docs/webhook-deployment.md) - Admission webhook setup
- [E2E Tests](test/e2e/README.md) - End-to-end testing guide
- [CLAUDE.md](CLAUDE.md) - Development workflow and commands

### Helm Charts

- `charts/shaper-crds` - CRD definitions
- `charts/shaper-api` - API server deployment
- `charts/shaper-webhooks` - Admission webhook deployment
- `charts/shaper-controller` - Controller deployment

### External Resources

- [iPXE Documentation](https://ipxe.org/docs) - iPXE scripting reference
- [Butane Specification](https://coreos.github.io/butane/) - Butane config format
- [controller-runtime](https://pkg.go.dev/sigs.k8s.io/controller-runtime) - K8s controller framework
- [Kubernetes CRDs](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) - Custom Resource concepts

### API Specifications

- `api/shaper.v1.yaml` - Main iPXE API
- `api/shaper-webhook-resolver.v1.yaml` - Webhook resolver API
- `api/shaper-webhook-transformer.v1.yaml` - Webhook transformer API

---

**Last Updated**: 2025-12-15
