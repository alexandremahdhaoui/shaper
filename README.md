# Shaper

**Kubernetes-native iPXE boot server for bare-metal provisioning without external databases.**

## Problem

Bare-metal servers need network boot configurations. Traditional tools require external databases
and complex state management. Shaper stores everything in Kubernetes CRDs, enabling GitOps
workflows and eliminating database dependencies.

## Contents

- [Quick Start](#quick-start)
- [How does it work?](#how-does-it-work)
- [How do I define boot profiles?](#how-do-i-define-boot-profiles)
- [How do I assign profiles to servers?](#how-do-i-assign-profiles-to-servers)
- [Commands](#commands)
- [Links](#links)

## Quick Start

```bash
# Prerequisites: kubectl, helm, kind (for testing)

# Install CRDs
helm install shaper-crds ./charts/shaper-crds

# Install API server
helm install shaper-api ./charts/shaper-api -n shaper-system --create-namespace

# Verify
kubectl get profiles
kubectl get assignments
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

**Boot Flow:**

1. Machine boots, DHCP points to Shaper
2. `/boot.ipxe` returns bootstrap script
3. `/ipxe?uuid=X&buildarch=Y` finds Assignment, renders Profile
4. `/content/{uuid}` serves additional configs (ignition, cloud-init)

## How do I define boot profiles?

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

**Content Sources:** `inline`, `objectRef` (ConfigMap/Secret), `webhook` (external API)
**Transformations:** `butaneToIgnition`, `webhook`

## How do I assign profiles to servers?

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

**Matching Logic:**

1. Find Assignment with matching UUID + buildarch
2. Fall back to default Assignment for buildarch (`isDefault: true`)
3. Render referenced Profile's iPXE template

## Commands

```bash
# Build
forge build                    # All artifacts
forge build shaper-api         # API binary
forge build shaper-controller  # Controller binary

# Test
forge test-all                 # All tests (lint, unit, e2e)
forge test unit run            # Unit tests only
forge test e2e run             # E2E tests (creates Kind cluster)

# Code quality
forge build format             # Format code
forge test lint run            # Lint code
```

## Components

| Binary | Description |
|--------|-------------|
| shaper-api | HTTP server for iPXE boot scripts |
| shaper-controller | Reconciles Profile/Assignment CRDs |
| shaper-webhook | Validates/mutates CRDs |
| shaper-tftp | TFTP server for initial chainload |

## Helm Charts

| Chart | Purpose |
|-------|---------|
| `charts/shaper-crds` | CRD definitions |
| `charts/shaper-api` | API server deployment |
| `charts/shaper-controller` | Controller deployment |
| `charts/shaper-webhooks` | Admission webhooks |

## Links

- [Architecture](./ARCHITECTURE.md) - System design and code patterns
- [API Deployment](./docs/api-deployment.md) - Production deployment guide
- [Webhook Deployment](./docs/webhook-deployment.md) - Admission webhook setup
- [E2E Tests](./test/e2e/README.md) - End-to-end testing

**External:**

- [iPXE Documentation](https://ipxe.org/docs)
- [Butane Config](https://coreos.github.io/butane/)
- Inspired by [poseidon/matchbox](https://github.com/poseidon/matchbox)
