# Contributing to Shaper

**Contribute code, docs, and fixes to Shaper -- a Kubernetes-native iPXE boot server.**

## Quick Start

Prerequisites: Go 1.25.0, Docker, [forge](https://github.com/alexandremahdhaoui/forge).

```bash
git clone https://github.com/alexandremahdhaoui/shaper.git
cd shaper
forge build          # Build all artifacts
forge test-all       # Run lint, unit, and e2e tests
```

Read [DESIGN.md](./DESIGN.md) for architecture context before contributing.

## How do I structure commits?

Each commit uses an emoji prefix and a structured body.

**Emoji conventions (mapped to semver):**

| Emoji | Semver | Use |
|-------|--------|-----|
| `⚠` | major | Breaking changes |
| `✨` | minor | New feature |
| `🐛` | patch | Bug fix |
| `📖` | -- | Documentation |
| `🌱` | -- | Chore, test, refactor |

**Commit body format:**

```
✨ Short imperative summary (50 chars or less)

Why: Explain the motivation. What problem exists?

How: Describe the approach. What strategy did you choose?

What:

- pkg/foo/bar.go: description of change
- cmd/baz/main.go: description of change

How changes were verified:

- Unit tests for new logic (go test)
- forge test-all: all stages passed

Signed-off-by: Your Name <your@email.com>
```

`Signed-off-by` is required on all commits. Use `git commit -s` to add it automatically.

## How do I submit a pull request?

1. Open an issue to discuss the change.
2. Fork the repo and create a feature branch from `main`.
3. Write code, tests, and run `forge test-all`.
4. Choose the correct PR template from `.github/PULL_REQUEST_TEMPLATE/`:

| Template | Title prefix | Use |
|----------|-------------|-----|
| `breaking_change.md` | `⚠` | Breaking API or behavior changes |
| `compat_feature.md` | `✨` | New backward-compatible features |
| `bug_fix.md` | `🐛` | Bug fixes (reference the issue) |
| `docs.md` | `📖` | Documentation changes |
| `other.md` | `🌱` | Chores, tests, refactors |

**CI runs on every PR:**
- `golangci-lint` -- static analysis and linting.
- `modified-files` -- verifies generated code is up to date.

## How do I run tests?

Shaper uses [forge](https://github.com/alexandremahdhaoui/forge) for builds and tests.

**Test stages (run individually for fast iteration):**

```bash
forge test run lint-tags       # Verify build tags on test files
forge test run lint-licenses   # Verify license headers
forge test run lint            # golangci-lint (v1.62)
forge test run unit            # Unit tests
forge test run e2e             # E2E tests (requires Kind + Docker)
```

**Full validation (run before submitting a PR):**

```bash
forge test-all                 # Build all artifacts, run all test stages
```

E2E tests create a Kind cluster, build container images, and deploy Helm charts.
Full e2e requires Docker and optionally libvirt for VM-based boot tests.
See [test/e2e/README.md](./test/e2e/README.md) for detailed e2e test documentation.

**Code generation (run after changing OpenAPI specs or CRD types):**

```bash
forge build generate-all       # OpenAPI clients/servers, CRDs, RBAC, mocks
```

Stale generated files fail the `modified-files` CI check.

## How is the project structured?

```
shaper/
├── api/            # OpenAPI specifications (shaper, resolver, transformer)
├── build/          # Build output (binaries, iPXE artifacts)
├── charts/         # Helm charts (shaper-crds, shaper-api, shaper-controller, shaper-webhooks)
├── cmd/            # Binary entry points (4 binaries)
├── containers/     # Containerfiles for each binary
├── docs/           # Deployment guides
├── examples/       # Example configurations for binaries
├── hack/           # Build scripts, license boilerplate
├── internal/       # Internal packages (adapters, controllers, drivers, utilities)
├── pkg/            # Public packages (API types, generated clients, networking)
├── test/           # Test suites (unit, e2e)
└── forge.yaml      # Build and test configuration
```

## What does each CLI tool do?

**Data plane:**

| Binary | Purpose |
|--------|---------|
| `shaper-api` | HTTP server serving iPXE boot scripts and machine configurations |
| `shaper-tftp` | TFTP server for initial iPXE chainloading |

**Control plane:**

| Binary | Purpose |
|--------|---------|
| `shaper-controller` | Reconciles Profile and Assignment CRDs via controller-runtime |
| `shaper-webhook` | Admission webhook validating and mutating CRDs |

## What does each package do?

**Public packages (`pkg/`):**

| Package | Purpose |
|---------|---------|
| `cloudinit` | Cloud-init configuration builders for VM user data |
| `constants` | Shared context keys used across packages |
| `execcontext` | Execution context abstraction for command wrapping (e.g., sudo) |
| `generated` | Auto-generated OpenAPI client/server code (6 sub-packages) |
| `network` | Linux networking: bridges, dnsmasq, libvirt networks |
| `test` | Shared e2e test helpers and boot test infrastructure |
| `v1alpha1` | Kubernetes API types for Profile and Assignment CRDs |

**Internal packages (`internal/`):**

| Package | Purpose |
|---------|---------|
| `adapter` | Bridges Kubernetes resources to domain logic (assignments, profiles, resolvers, transformers) |
| `controller` | Request handlers: iPXE rendering, content resolution, resolver/transformer routing |
| `controller/reconciler` | CRD reconciliation: ProfileReconciler, AssignmentReconciler |
| `driver/server` | HTTP server middleware and request handlers |
| `driver/tftp` | TFTP server implementation for file serving over UDP |
| `driver/webhook` | Kubernetes admission webhook handlers |
| `k8s` | Kubernetes client initialization and configuration |
| `types` | Domain types: Profile, Content, IPXESelectors, TransformerConfig |
| `util/certutil` | Certificate loading and TLS configuration |
| `util/fakes` | Fake adapters and servers for testing |
| `util/gracefulshutdown` | Graceful shutdown orchestration for multi-server apps |
| `util/httputil` | HTTP server utilities and request middleware |
| `util/logging` | Structured logging setup (slog) with configurable levels |
| `util/mocks` | Generated mock interfaces for unit testing |
| `util/ssh` | SSH client for remote execution and file transfer |
| `util/testutil` | Test asset, certificate, and directory helpers |
| `util/tlsutil` | TLS configuration builder with mTLS support |

## How do I create a new engine?

Shaper builds and tests through [forge](https://github.com/alexandremahdhaoui/forge) engines defined in `forge.yaml`.

Custom engines chain built-in forge engines under the `engines:` section. Example from `forge.yaml`:

```yaml
engines:
  - alias: go-gen-openapi-with-license
    type: builder
    builder:
      - engine: go://go-gen-openapi
      - engine: go://generic-builder
        spec:
          command: "bash"
          args: ["./hack/add-license-headers.sh"]
```

See the [forge documentation](https://github.com/alexandremahdhaoui/forge) for engine types and configuration.

## What conventions must I follow?

**Build tags:** E2E tests use `//go:build e2e`. Unit tests use `//go:build unit`. Every test file requires a build tag. The `lint-tags` stage enforces this.

**License headers:** Every `.go` file requires the Apache 2.0 header from `hack/boilerplate.go.txt`. The `lint-licenses` stage enforces this.

**Generated files:** Run `forge build generate-all` after changing `api/*.yaml` or `pkg/v1alpha1/` types. Do not edit files under `pkg/generated/` or `internal/util/mocks/` by hand.

**Formatting:** Run `forge build format` before committing. CI blocks merges on lint failures.

## License

Apache License 2.0. See [LICENSE](./LICENSE) for the full text.
