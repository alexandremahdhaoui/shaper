# Shaper

**Kubernetes-native iPXE boot server for bare-metal provisioning.

> "I was managing PXE boot configs across 3 data centers with a Postgres backend and Ansible glue.
> Config drift was constant, and nothing fit into our GitOps pipeline.
> Shaper replaced all of that with 2 CRDs I can version-control and deploy through Flux."
> -- Infrastructure Engineer

## What problem does Shaper solve?

Bare-metal servers need network boot configurations to install operating systems.
Traditional PXE tools store boot state in external databases and config files outside version control.
This creates drift, blocks GitOps adoption, and adds operational overhead.
Shaper eliminates the external database entirely.
It stores boot profiles and machine assignments as Kubernetes Custom Resources (CRDs).
Operators manage boot infrastructure with kubectl, Helm, and Git -- the same tools they use for everything else.

## Quick Start

```bash
# Prerequisites: kubectl, helm, a running Kubernetes cluster

# Install CRDs
helm install shaper-crds ./charts/shaper-crds

# Install API server
helm install shaper-api ./charts/shaper-api -n shaper-system --create-namespace

# Create a boot profile
kubectl apply -f - <<'EOF'
apiVersion: shaper.amahdha.com/v1alpha1
kind: Profile
metadata: { name: flatcar-linux }
spec:
  ipxeTemplate: |
    #!ipxe
    kernel http://stable.release.flatcar-linux.net/amd64-usr/current/flatcar_production_pxe.vmlinuz
    initrd http://stable.release.flatcar-linux.net/amd64-usr/current/flatcar_production_pxe_image.cpio.gz
    boot
EOF

# Assign the profile as default for x86_64
kubectl apply -f - <<'EOF'
apiVersion: shaper.amahdha.com/v1alpha1
kind: Assignment
metadata: { name: default-amd64 }
spec:
  profileName: flatcar-linux
  subjectSelectors: { buildarch: [x86_64] }
  isDefault: true
EOF

# Verify the boot endpoint
curl http://localhost:30080/boot.ipxe
```

## How does it work?

```
Machine                    Shaper API                 Kubernetes
   |                           |                           |
   |--GET /boot.ipxe---------->|                           |
   |<--bootstrap script--------|                           |
   |                           |                           |
   |--GET /ipxe?uuid=X-------->|--List Assignments-------->|
   |                           |<--matching Assignment-----|
   |                           |--Get Profile------------->|
   |<--rendered iPXE script----|<--Profile spec------------|
   |                           |                           |
   |--GET /content/{uuid}----->|--resolve + transform----->|
   |<--ignition/cloud-init-----|<--config------------------|
```

A booting machine fetches `/boot.ipxe`, which returns a bootstrap iPXE script.
The bootstrap script chains to `/ipxe?uuid=X&buildarch=Y`, where Shaper finds the matching Assignment and renders the referenced Profile.
If the Profile exposes additional content (Ignition, cloud-init), the machine fetches it from `/content/{uuid}`.
For full design details, see [DESIGN.md](./DESIGN.md).

## Contents

- [How do I configure boot profiles?](#how-do-i-configure-boot-profiles)
- [How do I assign profiles to servers?](#how-do-i-assign-profiles-to-servers)
- [How do I build and test?](#how-do-i-build-and-test)
- [What components does Shaper include?](#what-components-does-shaper-include)
- [FAQ](#faq)
- [Documentation](#documentation)

## How do I configure boot profiles?

A Profile defines an iPXE boot template and optional additional content.

```yaml
apiVersion: shaper.amahdha.com/v1alpha1
kind: Profile
metadata:
  name: flatcar-linux
spec:
  ipxeTemplate: |
    #!ipxe
    kernel http://boot.example.com/vmlinuz ignition.config.url={{ .AdditionalContent.ignition }}
    initrd http://boot.example.com/initrd.img
    boot
  additionalContent:
    - name: ignition
      exposed: true
      inline: |
        variant: fcos
        version: 1.4.0
        storage:
          files:
            - path: /etc/hostname
              contents:
                inline: my-server
      postTransformations:
        - butaneToIgnition: true
```

**Content sources** (exactly 1 per content entry):

- `inline` -- content embedded directly in the Profile spec.
- `objectRef` -- reference to a Kubernetes object (ConfigMap, Secret) with JSONPath extraction.
- `webhook` -- external HTTP endpoint with optional mTLS or Basic Auth.

**Post-transformations** run after content resolution:

- `butaneToIgnition` -- converts Butane YAML to Ignition JSON.
- `webhook` -- sends content to an external transformation endpoint.

## How do I assign profiles to servers?

An Assignment maps machines to a Profile using selectors.

```yaml
apiVersion: shaper.amahdha.com/v1alpha1
kind: Assignment
metadata:
  name: dc1-servers
spec:
  subjectSelectors:
    buildarch: [x86_64]
    uuidList:
      - 47c6da67-7477-4970-aa03-84e48ff4f6ad
  profileName: flatcar-linux
  isDefault: false
```

**Matching logic** (highest to lowest priority):

1. Exact UUID match + buildarch match.
2. Default Assignment for the buildarch (`isDefault: true`).
3. No match found -- Shaper returns an error.

## How do I build and test?

Shaper uses [forge](https://github.com/alexandremahdhaoui/forge) for builds and tests.

```bash
forge build                    # Build all 4 binaries
forge build shaper-api         # Build a single binary
forge test-all                 # Run lint, unit, and e2e tests
forge test run unit            # Run unit tests only
forge test run e2e             # Run e2e tests (creates a Kind cluster)
```

## What components does Shaper include?

**Binaries (4):**

| Binary | Purpose |
|--------|---------|
| `shaper-api` | HTTP server serving iPXE boot scripts and configs |
| `shaper-controller` | Reconciles Profile and Assignment CRDs |
| `shaper-webhook` | Validates and mutates CRDs via admission webhooks |
| `shaper-tftp` | TFTP server for initial iPXE chainloading |

**Helm Charts (4):**

| Chart | Purpose |
|-------|---------|
| `charts/shaper-crds` | CRD definitions for Profile and Assignment |
| `charts/shaper-api` | API server Deployment, Service, ConfigMap |
| `charts/shaper-controller` | Controller Deployment and RBAC |
| `charts/shaper-webhooks` | Admission webhook configuration |

## FAQ

**Does Shaper require an external database?**
No. Shaper stores all state in Kubernetes CRDs. The Kubernetes API server is the only data store.

**Which boot firmware does Shaper support?**
Shaper serves iPXE scripts. Machines must chainload iPXE via DHCP/TFTP or boot from an iPXE ISO. The `shaper-tftp` binary handles the initial chainload.

**Can I manage Shaper resources with GitOps?**
Yes. Profiles and Assignments are standard Kubernetes resources. Tools like Flux and ArgoCD apply them from Git repositories.

**Which CPU architectures does Shaper support?**
Shaper supports 4 architectures: i386, x86\_64, arm32, and arm64. The `buildarch` selector in Assignments controls architecture targeting.

**What content formats can Shaper serve?**
Shaper serves any text-based content. Built-in transformation supports Butane-to-Ignition conversion. Webhook transformers handle arbitrary formats.

**How does Shaper resolve content from Kubernetes objects?**
The `objectRef` source fetches data from any Kubernetes object using JSONPath. This works with ConfigMaps, Secrets, and custom resources.

**Can I use external services for content generation?**
Yes. The `webhook` content source calls external HTTP endpoints with optional mTLS or Basic Auth. Webhook transformers post-process content through external services.

## Documentation

**Design:**

- [DESIGN.md](./DESIGN.md) -- System architecture, code patterns, CRD specifications

**Deployment:**

- [API Deployment](./docs/api-deployment.md) -- Production deployment guide
- [Webhook Deployment](./docs/webhook-deployment.md) -- Admission webhook setup

**Testing:**

- [E2E Tests](./test/e2e/README.md) -- End-to-end test guide

**External:**

- [iPXE Documentation](https://ipxe.org/docs) -- iPXE scripting reference
- [Butane Specification](https://coreos.github.io/butane/) -- Butane config format

## Contributing

Contributions are welcome. Open an issue to discuss changes before submitting a pull request.

## License

Apache License 2.0. See [LICENSE](./LICENSE) for the full text.
