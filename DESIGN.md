# Shaper Design

**Shaper uses a layered, interface-driven architecture to manage iPXE boot configurations as Kubernetes Custom Resources, serving boot scripts and machine configs over HTTP without external databases.**

## Problem Statement

Bare-metal servers require network boot configurations to provision operating systems. Operators must manage boot profiles, match machines to profiles, and serve configuration files at boot time. Existing tools like Matchbox and Foreman couple boot configuration to external databases, require imperative workflows, and resist GitOps integration. This raises a question: how can operators manage boot configurations declaratively using Kubernetes-native primitives?

Shaper answers this by storing boot profiles and machine assignments as Kubernetes CRDs. A Profile CRD defines an iPXE template and additional content (Ignition, cloud-init). An Assignment CRD maps machines to profiles by UUID and architecture. The shaper-api HTTP server resolves assignments, renders templates, and serves configuration files. A content pipeline resolves content from inline values, Kubernetes objects, or external webhooks, then applies transformations (Butane to Ignition) before serving.

## Tenets

1. **Kubernetes is the database.** Store all boot configuration state in CRDs. No external databases, no local file stores. Kubernetes provides persistence, RBAC, audit logging, and API access.
2. **Interface-driven design.** Define contracts between layers as Go interfaces. Testability and substitutability take priority over implementation convenience.
3. **Layered architecture.** Separate drivers (HTTP, TFTP, webhooks), controllers (business logic), adapters (data access), and types (domain models). Each layer depends only on layers below it.
4. **Extensible content pipeline.** Support pluggable resolvers (inline, objectRef, webhook) and transformers (Butane, webhook). New content sources and transformations require a single interface implementation.
5. **Operational simplicity.** Deploy with Helm charts. No custom operators beyond the included controller.
6. **iPXE compatibility.** Work with standard iPXE clients. Use chainloading via TFTP for initial boot, then HTTP for all subsequent requests.

## Requirements

- Serve iPXE bootstrap scripts at `/boot.ipxe` with machine-specific chain URLs.
- Match machines to boot profiles by UUID and build architecture.
- Fall back to a default assignment when no UUID-specific assignment exists.
- Resolve content from 3 sources: inline strings, Kubernetes object references with JSONPath extraction, and external webhooks with mTLS or Basic Auth.
- Transform content through a pipeline (e.g., Butane YAML to Ignition JSON).
- Expose additional content at `/content/{contentID}` endpoints for machine consumption.
- Validate and mutate CRDs via admission webhooks (exactly one content source per item, label injection).
- Serve iPXE chainload binaries via TFTP for initial network boot.
- Deploy all components via Helm charts to any Kubernetes cluster.

## Out of Scope

- **OS image hosting.** Shaper serves boot scripts and configuration files, not kernel images or root filesystems.
- **DHCP server.** Operators configure existing DHCP infrastructure to point to Shaper's TFTP server.
- **Machine lifecycle management.** Shaper does not track machine state, power management, or provisioning progress.
- **Multi-cluster federation.** Boot configurations are scoped to a single Kubernetes cluster.
- **Graphical user interface.** All interaction occurs through kubectl, Helm, and the CRD API.

## Success Criteria

- A machine completes the full boot flow (bootstrap, assignment, boot, config) in under 5 minutes from initial PXE request.
- Zero external database dependencies. All state resides in Kubernetes CRDs.
- 4 deployable components: shaper-api, shaper-controller, shaper-webhook, shaper-tftp.
- Full GitOps compatibility: Profile and Assignment CRDs can be managed through Flux, ArgoCD, or any GitOps tool.
- Unit test coverage exceeds 65% across adapter, controller, and driver packages.

## Proposed Design

### Layered Architecture

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

Each layer depends only on the layers below it. Drivers depend on controllers, controllers depend on adapters, adapters depend on types. This rule prevents circular dependencies and keeps business logic independent of transport protocols.

### iPXE Boot Flow

The boot flow has 4 phases: BOOTSTRAP, ASSIGNMENT, BOOT, and CONFIG (optional).

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
       |  (may contain /content/{contentID} URLs)         |
       |<-------------------------------------------------|
       |                                                  |
       |  Phase 3: BOOT                                   |
       |  Machine executes iPXE commands                  |
       |                                                  |
       |  Phase 4: CONFIG (optional)                      |
       |  GET /content/{contentID}?buildarch=amd64        |
       |------------------------------------------------->|
       |                                                  |
       |           +----------------------------------+   |
       |           | Content Controller               |   |
       |           | 1. Lookup Profile by UUID        |   |
       |           | 2. Resolve content source        |   |
       |           | 3. Apply transformations         |   |
       |           +----------------------------------+   |
       |                                                  |
       |  Response: Configuration file                    |
       |  (Ignition, Cloud-Init, etc.)                    |
       |<-------------------------------------------------|
       |                                                  |
       v                                                  v
   Machine boots OS
```

Phase 1 returns a cached bootstrap script that chains into Phase 2 with machine-specific parameters. Phase 2 performs assignment selection, profile lookup, content resolution, and template rendering. Phase 3 is client-side iPXE execution. Phase 4 serves additional configuration files referenced in the rendered iPXE script.

### Content Resolution Pipeline

```
+-------------------+     +-------------------+     +-------------------+
|    Resolve        |     |    Transform      |     |    Final Content  |
|                   |     |                   |     |                   |
| - Inline: return  |     | - Butane->Ignition|     | Rendered bytes    |
|   string directly | --> | - Webhook: POST   | --> | ready to serve    |
| - ObjectRef: K8s  |     |   to external     |     | via HTTP          |
|   JSONPath query  |     |   transformer     |     |                   |
| - Webhook: GET    |     | - (none): pass    |     |                   |
|   external API    |     |   through)        |     |                   |
+-------------------+     +-------------------+     +-------------------+
```

The ResolveTransformerMux routes each content item to its resolver based on `ResolverKind`, then chains zero or more transformers based on `PostTransformers`. For batch operations (iPXE template rendering), exposed content returns a `/content/{contentID}` URL instead of the resolved bytes.

### Assignment Selection Priority

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

The Assignment adapter uses Kubernetes label selectors for efficient queries. The AssignmentReconciler adds labels (`shaper.amahdha.com/buildarch-{arch}`, `uuid.shaper.amahdha.com/{uuid}`, `shaper.amahdha.com/default-assignment`) to Assignments based on their spec. This converts the selection algorithm into standard Kubernetes label-based list operations.

## Technical Design

### Data Model

**Profile CRD** (`shaper.amahdha.com/v1alpha1`):

| Field | Type | Description |
|-------|------|-------------|
| `spec.ipxeTemplate` | string | Go template for iPXE script |
| `spec.additionalContent` | []AdditionalContent | Content items for templating |
| `spec.additionalContent[].name` | string | Content identifier |
| `spec.additionalContent[].exposed` | bool | Serve via `/content/{contentID}` |
| `spec.additionalContent[].postTransformations` | []Transformer | Transformation pipeline |
| `spec.additionalContent[].inline` | *string | Direct content (mutually exclusive) |
| `spec.additionalContent[].objectRef` | *ObjectRef | K8s object reference (mutually exclusive) |
| `spec.additionalContent[].webhook` | *WebhookConfig | External webhook (mutually exclusive) |
| `status.exposedAdditionalContent` | map[string]string | Maps content names to UUIDs |

**Assignment CRD** (`shaper.amahdha.com/v1alpha1`):

| Field | Type | Description |
|-------|------|-------------|
| `spec.subjectSelectors.buildarch` | []Buildarch | Target architectures (i386, x86_64, arm32, arm64) |
| `spec.subjectSelectors.uuidList` | []string | Target machine UUIDs |
| `spec.profileName` | string | Name of Profile to assign |
| `spec.isDefault` | bool | Default assignment for buildarch |

**Internal Domain Types** (abbreviated):

```go
// internal/types
type Profile struct {
    Name, Namespace   string
    IPXETemplate      string
    AdditionalContent map[string]Content
    ContentIDToNameMap map[uuid.UUID]string
}

type Assignment struct {
    Name, Namespace  string
    ProfileName      string
    SubjectSelectors map[string][]string
}

type Content struct {
    Name             string
    ResolverKind     ResolverKind
    Inline           string
    ObjectRef        *ObjectRef
    WebhookConfig    *WebhookConfig
    PostTransformers []TransformerConfig
    Exposed          bool
    ExposedUUID      uuid.UUID
}
```

### Component Catalog

| Binary | Package | Purpose | Key Interfaces |
|--------|---------|---------|----------------|
| shaper-api | `cmd/shaper-api` | HTTP server for iPXE boot scripts and config files | `StrictServerInterface` |
| shaper-controller | `cmd/shaper-controller` | Reconciles Profile and Assignment CRDs | `reconcile.Reconciler` |
| shaper-webhook | `cmd/shaper-webhook` | Validates and mutates CRDs on admission | `admission.Handler` |
| shaper-tftp | `cmd/shaper-tftp` | TFTP server for iPXE chainload binaries | `tftp.Handler` |

### Package Catalog

**Public packages** (`pkg/`):

| Package | Purpose |
|---------|---------|
| `pkg/v1alpha1` | CRD type definitions, GroupVersion registration, label selectors |
| `pkg/generated/shaperserver` | OpenAPI-generated server stubs for the iPXE API |
| `pkg/generated/shaperclient` | OpenAPI-generated client for the iPXE API |
| `pkg/generated/resolverserver` | OpenAPI-generated server stubs for webhook resolver |
| `pkg/generated/transformerserver` | OpenAPI-generated server stubs for webhook transformer |
| `pkg/constants` | Shared constants |
| `pkg/network` | Network utilities (bridge, dnsmasq, libvirt) |
| `pkg/cloudinit` | Cloud-init configuration helpers |
| `pkg/execcontext` | Execution context utilities |
| `pkg/test/e2e` | E2E test helpers (libvirt, K8s, port-forward) |

**Internal packages** (`internal/`):

| Package | Purpose |
|---------|---------|
| `internal/adapter/assignment` | Queries Assignment CRDs via label selectors |
| `internal/adapter/profile` | Fetches and converts Profile CRDs to domain types |
| `internal/adapter/resolver` | Inline, ObjectRef, and Webhook content resolvers |
| `internal/adapter/transformer` | Butane and Webhook content transformers |
| `internal/controller/ipxe` | Assignment selection, profile rendering |
| `internal/controller/content` | Content retrieval by UUID |
| `internal/controller/resolvetransformermux` | Routes resolve/transform operations |
| `internal/controller/reconciler` | Profile and Assignment reconciliation loops |
| `internal/driver/server` | HTTP server implementing OpenAPI spec |
| `internal/driver/webhook` | Admission webhook handlers |
| `internal/driver/tftp` | TFTP file server |
| `internal/types` | Internal domain models |
| `internal/util/mocks` | Generated mocks for all interfaces |
| `internal/util/fakes` | Fake implementations for testing |
| `internal/util/certutil` | Certificate generation utilities |
| `internal/util/httputil` | HTTP server helpers |
| `internal/util/tlsutil` | TLS configuration helpers |
| `internal/util/ssh` | SSH client utilities |
| `internal/util/logging` | Structured logging setup |
| `internal/util/gracefulshutdown` | Graceful shutdown handler |

### Key Interfaces

**Controller layer:**

```go
type IPXE interface {
    FindProfileAndRender(ctx context.Context, selectors types.IPXESelectors) ([]byte, error)
    Boostrap() []byte
}

type Content interface {
    GetByID(ctx context.Context, contentID uuid.UUID, attributes types.IPXESelectors) ([]byte, error)
}

type ResolveTransformerMux interface {
    ResolveAndTransform(ctx context.Context, content types.Content, selectors types.IPXESelectors) ([]byte, error)
    ResolveAndTransformBatch(ctx context.Context, batch map[string]types.Content, selectors types.IPXESelectors, options ...ResolveTransformBatchOption) (map[string][]byte, error)
}
```

**Adapter layer:**

```go
type Assignment interface {
    FindDefaultByBuildarch(ctx context.Context, buildarch string) (types.Assignment, error)
    FindBySelectors(ctx context.Context, selectors types.IPXESelectors) (types.Assignment, error)
}

type Profile interface {
    Get(ctx context.Context, name string) (types.Profile, error)
    GetInNamespace(ctx context.Context, name, namespace string) (types.Profile, error)
    ListByContentID(ctx context.Context, contentID uuid.UUID) ([]types.Profile, error)
}

type Resolver interface {
    Resolve(ctx context.Context, content types.Content, attributes types.IPXESelectors) ([]byte, error)
}

type Transformer interface {
    Transform(ctx context.Context, cfg types.TransformerConfig, content []byte, selectors types.IPXESelectors) ([]byte, error)
}
```

## Design Patterns

**Interface-based dependency injection.** All components accept interfaces via constructor functions (e.g., `NewIPXE(assignment, profile, mux)`). This enables unit testing with generated mocks and fake implementations without touching Kubernetes or external services.

**Sentinel errors.** Exported sentinel errors (e.g., `ErrAssignmentNotFound`) enable callers to branch on error type using `errors.Is()`. Unexported errors (e.g., `errAssignmentList`) provide context about where the error occurred. Both are chained using `errors.Join()`.

**Functional options.** Optional parameters use the functional options pattern (e.g., `ResolveTransformBatchOption`). This keeps function signatures stable when adding new optional behaviors like `ReturnExposedContentURL`.

**Type conversion at adapter boundary.** Adapters convert between CRD types (`pkg/v1alpha1`) and internal domain types (`internal/types`). Controllers work exclusively with internal types. This isolates business logic from CRD API changes.

**Label-based Kubernetes queries.** The AssignmentReconciler and ProfileReconciler add labels to CRDs based on spec fields. The adapter layer uses Kubernetes label selectors to query CRDs, converting domain queries into efficient K8s API calls.

## Alternatives Considered

### Do Nothing (Use Existing Tools)

Matchbox and Foreman provide bare-metal boot configuration. Matchbox stores profiles in local files, requires a separate etcd cluster for metadata, and does not integrate with Kubernetes RBAC or GitOps workflows. Foreman requires a PostgreSQL database and provides provisioning features beyond what boot configuration needs. Both tools introduce operational dependencies that Shaper eliminates by using Kubernetes as the single source of truth.

### ConfigMaps Instead of CRDs

Storing boot profiles in ConfigMaps would avoid CRD registration. ConfigMaps lack schema validation, status subresources, and admission webhook support. Without a status field, the controller cannot track generated content UUIDs. Without admission webhooks, invalid configurations (e.g., multiple content sources on one item) reach the controller. CRDs provide all three capabilities with minimal operational overhead.

### Monolithic Binary

A single binary combining API server, controller, webhook, and TFTP server would simplify deployment. It would also prevent independent scaling (API server handles request load while the controller handles reconciliation load). Separate binaries allow operators to scale, upgrade, and configure each component independently. The 4-binary approach maps to standard Kubernetes deployment patterns.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| CRD version upgrades (v1alpha1 to v1) | Breaking API changes affect all consumers | Conversion webhooks, versioned packages, deprecation period |
| iPXE client incompatibility | Machines fail to boot | TFTP chainloading with custom-compiled iPXE binary, embedded retry logic |
| Webhook resolver/transformer unavailability | Content resolution fails at boot time | Context timeouts, error propagation, fallback to cached bootstrap |
| Kubernetes API unavailability | All operations fail | Cached bootstrap script serves Phase 1 without K8s API calls |

## Testing Strategy

**Unit tests.** Table-driven tests cover adapter, controller, and driver packages. Generated mocks (`internal/util/mocks/`) stub interface dependencies. The K8s fake client (`sigs.k8s.io/controller-runtime/pkg/client/fake`) simulates CRD operations without a cluster.

**End-to-end tests.** E2E tests use the `e2e` build tag and run against a Kind cluster with libvirt/QEMU virtual machines. Tests cover the full boot flow: TFTP chainload, DHCP, iPXE bootstrap, assignment selection, profile rendering, and content serving. A DnsmasqServer VM provides DHCP and TFTP services on a NAT network (192.168.100.0/24).

**Forge orchestration.** The `forge` tool manages build and test workflows. `forge test-all` builds all artifacts, runs lint, unit tests, and E2E tests sequentially. `forge test run unit` and `forge test run e2e` target individual stages.

**Lint.** Static analysis via `golangci-lint` enforces code style, identifies bugs, and catches common mistakes.

## FAQ

**Why 4 separate binaries instead of 1?** Each binary serves a distinct operational concern. The API server handles HTTP request load. The controller handles reconciliation load. The webhook validates CRDs at admission time. The TFTP server handles initial chainloading. Separate binaries allow independent scaling, upgrade cycles, and failure isolation.

**Why CRDs instead of ConfigMaps?** CRDs provide schema validation, a status subresource for tracking generated content UUIDs, and admission webhook support for enforcing invariants (exactly one content source per item). ConfigMaps lack all three.

**Why a content pipeline instead of serving raw content?** Machines consume different configuration formats. Flatcar Linux uses Ignition JSON, but operators write Butane YAML. The pipeline transforms Butane to Ignition at serve time. Webhook transformers extend this to arbitrary transformations without modifying Shaper.

**How does Shaper handle concurrent boot requests?** Each HTTP request creates an independent call chain through the controller and adapter layers. No shared mutable state exists beyond the cached bootstrap script (read-only after initialization). The Kubernetes API client handles concurrency internally.

## Appendix

### OpenAPI Specifications

- `api/shaper.v1.yaml` - iPXE boot API (`/boot.ipxe`, `/ipxe`, `/content/{contentID}`)
- `api/shaper-webhook-resolver.v1.yaml` - Webhook resolver request/response schema
- `api/shaper-webhook-transformer.v1.yaml` - Webhook transformer request/response schema

### Helm Charts

- `charts/shaper-crds` - Profile and Assignment CRD definitions
- `charts/shaper-api` - API server Deployment, Service, ConfigMap
- `charts/shaper-controller` - Controller Deployment with RBAC
- `charts/shaper-webhooks` - ValidatingWebhookConfiguration, MutatingWebhookConfiguration

### Related Documentation

- [README.md](README.md) - Project overview and quick start
