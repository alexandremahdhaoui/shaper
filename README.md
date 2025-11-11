# shaper

SHAPER leverages Kubernetes to assign and expose fine-grained server configurations.

## Table of Contents

- [shaper](#shaper)
  - [Table of Contents](#table-of-contents)
  - [iPXE booting workflow](#ipxe-booting-workflow)
  - [Custom Resource Definitions](#custom-resource-definitions)
    - [Profile](#profile)
    - [Assignment](#assignment)
  - [Architecture](#architecture)
      - [Storage](#storage)
  - [Deployment](#deployment)
  - [Development](#development)
    - [Testing shaper](#testing-shaper)
      - [Running the binary in the reproducible test environment](#running-the-binary-in-the-reproducible-test-environment)
  - [Next features](#next-features)
  - [Acknowledgement](#acknowledgement)
  - [See Also](#see-also)

## iPXE booting workflow

| Phase             | Action                          | Description                                                                                                                                       |
|-------------------|---------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------|
| `[BOOTSTRAPPING]` | Call `/boot.ipxe`               | Machine starts and DHCP rule 67 specifies your `shaper` as the next server.                                                                        |
| `[ASSIGNMENT]`    | Chainload `/ipxe?labels=values` | Machine chain load this endpoint specifying labels for scheduling/assignment.                                                                     |
| `[BOOT]`          | Run `#ipxe...`                  | Machine runs the retrieved iPXE script, optionally containing uuid references to additional configuration files such as ignition or cloud-init. |
| `[OPTIONAL]`      | Fetch `/content/{contentID}`    | Fetch the optional config identified by a UUID (for exposed AdditionalContent items).                                                            |

## Custom Resource Definitions

We designed the Profile and Assignment CRDs in way that let 

### Profile

```yaml
apiVersion: shaper.amahdha.com/v1alpha1
kind: Profile
metadata:
  name: your-profile
  labels:
    assignment/ipxe-buildarch: aarch64
    assignment/extrinsic-region: us-cal
spec:
  # ipxeTemplate: string - Go template for the iPXE boot script
  ipxeTemplate: |
    #!ipxe
    echo Booting profile: your-profile
    echo Ignition config: {{ .AdditionalContent.ignitionFile }}
    echo Cloud-init config: {{ .AdditionalContent.cloudInit }}
    kernel http://boot.example.com/vmlinuz ignition.config.url={{ .AdditionalContent.ignitionFile }}
    initrd http://boot.example.com/initrd.img
    boot

  # additionalContent: []AdditionalContent - Array of additional configuration files
  additionalContent:
    # Example: Inline configuration (not exposed via /content endpoint)
    - name: config0
      exposed: false
      inline: |
        YOUR CONFIG HERE
      postTransformations: []

    # Example: Exposed ignition file with Butane transformation
    - name: ignitionFile
      exposed: true  # Will be accessible via /content/{uuid}
      inline: |
        variant: fcos
        version: 1.4.0
        storage:
          files:
            - path: /etc/hostname
              mode: 0644
              contents:
                inline: my-server
      postTransformations:
        - butaneToIgnition: true  # Transform Butane YAML to Ignition JSON

    # Example: Cloud-init from Kubernetes ConfigMap
    - name: cloudInit
      exposed: true
      objectRef:
        group: ""
        version: v1
        resource: configmaps
        namespace: default
        name: cloud-init-config
        jsonpath: .data.userdata
      postTransformations: []

    # Example: Configuration from external webhook
    - name: secretToken
      exposed: false
      webhook:
        url: https://secret-provider.example.com/token
        basicAuthRef:
          group: ""
          version: v1
          resource: secrets
          namespace: default
          name: webhook-auth
          usernameJSONPath: .data.username
          passwordJSONPath: .data.password
      postTransformations: []
```

### Assignment

Because the `shaper` should not endorse any `scheduler` or `assigner` role, but serve the purpose of other processes,
assignments should be authored by them.

This purpose is served by the `Assignment` CRD.

```yaml
apiVersion: shaper.amahdha.com/v1alpha1
kind: Assignment
metadata:
  name: your-assignment
  namespace: default
spec:
  # subjectSelectors: SubjectSelectors - Selects which machines this assignment applies to
  subjectSelectors:
    # buildarch: []Buildarch - List of CPU architectures (i386, x86_64, arm32, arm64)
    buildarch:
      - arm64
    # uuidList: []string - List of machine UUIDs (passed via iPXE ${uuid} variable)
    uuidList:
      - 47c6da67-7477-4970-aa03-84e48ff4f6ad
      - 3f5f3c39-584e-4c7c-b6ff-137e1aaa7175

  # profileName: string - Name of the Profile to assign to matching machines
  profileName: your-profile

  # isDefault: bool - If true, this assignment is used when no other assignment matches
  # Only one default assignment per buildarch is allowed
  isDefault: false
```

**Note:** When a machine boots via iPXE, it calls `/ipxe?uuid={uuid}&buildarch={buildarch}`. The shaper-api finds the best matching Assignment by:
1. Checking for assignments with matching UUID in `uuidList` and matching `buildarch`
2. If no match, falling back to the default assignment for that `buildarch` (where `isDefault: true`)
3. Once an assignment is found, the referenced Profile is used to render the iPXE boot script

## Architecture

We have controllers, admission webhooks and a REST API.

The **REST API** is an iPXE server that only serves GET requests. The API endpoints are described below:
- `/boot.ipxe` - Returns a cached bootstrap iPXE script that chainloads to `/ipxe`
- `/ipxe?uuid={uuid}&buildarch={buildarch}` - Returns the iPXE boot script for the matching Assignment and Profile
- `/content/{contentID}?uuid={uuid}&buildarch={buildarch}` - Returns additional configuration files (ignition, cloud-init, etc.) for exposed AdditionalContent items

**Admission webhooks** ensures Assignment & Profile custom resources are conform, and optionally enriched them with more
information.

**Controllers** maintain datastructures queried by the REST API.

### Binaries

The project consists of 5 main binaries:

| Binary | Status | Description | Usage |
|--------|--------|-------------|-------|
| **shaper-api** | ✅ **IMPLEMENTED** | HTTP server serving iPXE boot scripts and configuration files | Production deployment for serving PXE boot requests |
| **shaper-webhook** | ✅ **IMPLEMENTED** | Kubernetes admission webhooks for validating and mutating Assignment and Profile CRDs | Production deployment for CRD validation |
| **shaper-controller** | ❌ **TODO** | Kubernetes controller for CRD reconciliation (placeholder only) | Future: Manage Profile status and generate UUIDs for exposed content |
| **shaper-tftp** | ❌ **TODO** | TFTP server for initial chainloading (not implemented) | Future: Serve initial iPXE bootloader via TFTP |
| **shaper-e2e** | ✅ **IMPLEMENTED** | End-to-end testing binary with VM orchestration | Development/testing only |

**Building Binaries:**
```shell
# Build all binaries
forge build

# Build specific binary
forge build shaper-api-binary
forge build shaper-webhook-binary
forge build shaper-e2e-binary

# Binaries are output to ./build/bin/
```

See individual README files in `cmd/*/README.md` for detailed binary documentation.

#### Storage

The storage backend will be done through dedicated CRDs, and or ConfigMaps. There are no reason to use databases.
Even though we need to ensure great performances, we do not need such complex systems. The key-value store from etcd
with the Kubernetes API frontend is more than enough.

In case too many resources are created in the same Kubernetes cluster, we might want to create partition keys for the
kubernetes resources (CRs or CMs) and distribute them into multiple Kubernetes clusters.

## Deployment

Replicas of the REST API queries the datastructures maintained by the controllers. These communications are performed
via mTLS. Hence, cert-manager is required for a production deployment.

## Development

### Build System

This project uses **[Forge](https://github.com/alexandremahdhaoui/forge)** for AI-native build orchestration and test environment management. Forge provides a unified interface for building, testing, and managing complex multi-step workflows.

#### Quick Start with Forge

```shell
# Build all artifacts (generates code, compiles binaries, formats code)
forge build

# Build specific artifacts
forge build shaper-api-binary          # Build shaper-api binary
forge build shaper-webhook-binary      # Build shaper-webhook binary
forge build shaper-api-container       # Build container image

# Run all code generation
forge build generate-all               # Runs: go mod tidy, controller-gen, mockery

# Format code
forge build format                     # Format with gofumpt
```

#### Testing with Forge

```shell
# Run unit tests
forge test unit run

# Run integration tests (auto-creates test environment)
forge test integration run

# Run e2e tests (auto-creates kind cluster)
forge test e2e run

# Run linting
forge test lint run

# Run all tests sequentially
forge test-all
```

#### Build System Architecture

Forge orchestrates multiple specialized engines:
- **go://build-go** - Compiles Go binaries with custom ldflags
- **go://build-container** - Builds container images
- **go://generate-openapi-go** - Generates OpenAPI server/client code
- **go://generic-builder** - Runs controller-gen for CRDs, RBAC, webhooks
- **go://generate-mocks** - Generates test mocks with mockery
- **go://format-go** - Formats code with gofumpt
- **go://lint-go** - Lints code with golangci-lint
- **go://test-runner-go** - Runs Go tests with build tags
- **go://kindenv** - Creates kind Kubernetes clusters
- **go://testenv-lcr** - Manages local container registries

See `forge.yaml` for the complete build and test configuration.

### Testing shaper

#### Running the binary in the reproducible test environment

```shell
# Set up environment variables
. .envrc.example

# Create test Kubernetes cluster with Forge
forge test integration create

# Export kubeconfig (path shown in create output)
export KUBECONFIG=/path/to/test-kubeconfig

# Run shaper-api locally
go run ./cmd/shaper-api
```

## Next features

- MTLS auth (shaper side): https://ipxe.org/crypto
- Trust (client side): https://ipxe.org/cmd/imgverify

## Acknowledgement

This project was inspired by [poseidon/matchbox](https://github.com/poseidon/matchbox).

## See Also

### Binary Documentation
- [shaper-api](./cmd/shaper-api/README.md) - HTTP API server
- [shaper-controller](./cmd/shaper-controller/README.md) - Kubernetes controller (TODO)
- [shaper-tftp](./cmd/shaper-tftp/README.md) - TFTP server (TODO)
- [shaper-webhook](./cmd/shaper-webhook/README.md) - Admission webhooks

### Deployment Guides
- [API Deployment Guide](./docs/api-deployment.md) - Deploy shaper-api to Kubernetes
- [Webhook Deployment Guide](./docs/webhook-deployment.md) - Deploy shaper-webhook to Kubernetes

### Architecture
- [Architecture Documentation](./ARCHITECTURE.md) - Detailed system architecture and design
