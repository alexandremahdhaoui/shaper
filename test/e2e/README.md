# Shaper E2E Tests

End-to-end tests for Shaper's iPXE boot flow using libvirt VMs and KIND Kubernetes clusters.

## Overview

This E2E testing infrastructure validates Shaper's complete iPXE network boot flow:

1. **DHCP**: Client VM obtains IP address from dnsmasq
2. **TFTP**: Client fetches iPXE boot files via TFTP
3. **HTTP**: iPXE chainloads to shaper-api for boot configuration
4. **Assignment**: Shaper-api matches client (by MAC/UUID) to Assignment CRD
5. **Profile**: Shaper-api returns appropriate Profile for the client
6. **Boot**: Client boots with the configured OS image and settings

## Running E2E Tests

E2E tests are managed through the forge build system:

```bash
# Run all E2E tests (creates testenv, runs tests, cleans up)
forge test e2e run

# Create test environment without running tests
forge test e2e create

# List existing test environments
forge test e2e list

# Delete a test environment
forge test e2e delete <test-id>
```

## Test Environment

The forge testenv system automatically provisions:

- **KIND Cluster**: Local Kubernetes cluster with shaper components deployed
- **Linux Bridge**: Layer 2 network bridge for VM connectivity
- **Libvirt Network**: Virtual network attached to the bridge
- **Dnsmasq**: DHCP/TFTP server for PXE boot
- **Client VM**: Test VM that performs network boot

### Environment Variables

When running tests, forge sets these environment variables:

| Variable | Description |
|----------|-------------|
| `TESTENV_VM_PXECLIENT_IP` | IP address of the PXE client VM |
| `TESTENV_KEY_VMSSH_PRIVATE_PATH` | Path to SSH private key |
| `TESTENV_NETWORK_TESTBRIDGE_IP` | Bridge gateway IP |
| `KUBECONFIG` | Path to KIND cluster kubeconfig |

## Prerequisites

### System Requirements

- **Operating System**: Linux (Ubuntu 22.04+ or equivalent)
- **Privileges**: Root/sudo access required for network and VM operations
- **CPU**: Multi-core recommended
- **Memory**: 8GB+ RAM recommended
- **Disk**: 20GB+ free space

### Required Software

```bash
# Virtualization
sudo apt-get install -y \
    libvirt-daemon-system \
    libvirt-clients \
    qemu-kvm \
    virtinst

# Networking
sudo apt-get install -y \
    dnsmasq \
    bridge-utils

# Kubernetes (Docker + KIND + kubectl)
# See https://kind.sigs.k8s.io/docs/user/quick-start/
```

### User Permissions

Add your user to required groups:

```bash
sudo usermod -aG libvirt $USER
sudo usermod -aG docker $USER
```

## Test Structure

- `test/e2e/ipxe_boot_test.go` - Main E2E test file
- `test/e2e/assets/` - Test assets (TFTP files, configs)
- `pkg/test/e2e/testenv_config.go` - Testenv configuration loader

## Troubleshooting

### Common Issues

1. **Permission denied for libvirt**: Ensure user is in `libvirt` group
2. **KIND cluster not starting**: Check Docker is running
3. **Network bridge issues**: May require sudo for network operations

### Debug Commands

```bash
# Check KIND cluster
kubectl --kubeconfig=/tmp/test-kubeconfig get nodes

# Check bridge
ip link show

# Check libvirt networks
virsh net-list --all

# Check dnsmasq
ps aux | grep dnsmasq
```
