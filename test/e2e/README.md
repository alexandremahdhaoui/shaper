# E2E Tests

**Validate Shaper's complete iPXE boot flow with real VMs**

End-to-end tests verify DHCP, TFTP, HTTP, Assignment matching, and Profile rendering.

## Contents

- [Quick Start](#quick-start)
- [How to debug?](#how-to-debug)
- [Environment Variables](#environment-variables)

## Quick Start

```bash
# Run all E2E tests
forge test run e2e

# Manage test environments
forge test create-env e2e      # Create without running tests
forge test list-env e2e        # List environments
forge test delete-env e2e ID   # Delete by ID
```

## How to debug?

**Q: Permission denied for libvirt?**
A: Add user to group: `sudo usermod -aG libvirt $USER`

**Q: KIND cluster not starting?**
A: Verify Docker is running: `docker ps`

**Q: Tests skip with "could not set up port-forward"?**
A: Ensure shaper-api is deployed: `kubectl -n shaper-system get pods`

**Q: How to check VM status?**
A: Use virsh: `virsh list --all`

**Q: How to check network?**
A: Use virsh: `virsh net-list --all`

## Environment Variables

Variables set by `forge test run e2e`:

| Variable | Description |
|----------|-------------|
| `TESTENV_VM_PXECLIENT_IP` | PXE client VM IP address |
| `TESTENV_KEY_VMSSH_PRIVATE_PATH` | SSH private key path |
| `TESTENV_NETWORK_TESTNETWORK_IP` | Network gateway IP |
| `KUBECONFIG` | KIND cluster kubeconfig |

## Test Files

- `test/e2e/ipxe_boot_test.go` - Main E2E tests
- `pkg/test/e2e/testenv_config.go` - Config loader

## Links

- [Main README](../../README.md)
- [Architecture](../../ARCHITECTURE.md)
