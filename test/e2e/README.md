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

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Host Machine                             │
│                                                               │
│  ┌─────────────┐      ┌──────────────┐     ┌─────────────┐ │
│  │   Dnsmasq   │      │ KIND Cluster │     │  Client VMs │ │
│  │             │      │              │     │             │ │
│  │ DHCP Server │◄────►│  shaper-api  │     │   (libvirt) │ │
│  │ TFTP Server │      │     CRDs     │     │             │ │
│  │             │      │              │     │             │ │
│  └─────────────┘      └──────────────┘     └─────────────┘ │
│         │                     │                    │        │
│         └─────────────────────┴────────────────────┘        │
│                    Linux Bridge (br-shaper)                  │
│                    Libvirt Network (net-shaper)              │
└─────────────────────────────────────────────────────────────┘
```

### Components

- **Linux Bridge** (`br-shaper`): Layer 2 network bridge connecting VMs
- **Libvirt Network** (`net-shaper`): Libvirt network attached to the bridge
- **Dnsmasq**: Combined DHCP, TFTP, and DNS server for PXE boot
- **KIND Cluster**: Local Kubernetes cluster running shaper-api
- **Client VMs**: Test virtual machines that perform network boot
- **Shaper API**: Kubernetes service serving iPXE boot configurations
- **CRDs**: Custom resources (Profile, Assignment) defining boot behavior

## Prerequisites

### System Requirements

- **Operating System**: Linux (Ubuntu 22.04+ or equivalent)
- **Privileges**: Root/sudo access required
- **CPU**: Multi-core recommended (VMs + containers)
- **Memory**: 8GB+ RAM recommended
- **Disk**: 20GB+ free space for images

### Required Software

Install all dependencies:

```bash
# Virtualization
sudo apt-get update
sudo apt-get install -y \
    libvirt-daemon-system \
    libvirt-clients \
    qemu-kvm \
    qemu-utils \
    virtinst

# Networking
sudo apt-get install -y \
    dnsmasq \
    bridge-utils \
    iproute2

# Kubernetes
# Install Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh
sudo usermod -aG docker $USER

# Install KIND
curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.20.0/kind-linux-amd64
chmod +x ./kind
sudo mv ./kind /usr/local/bin/kind

# Install kubectl
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
chmod +x ./kubectl
sudo mv ./kubectl /usr/local/bin/kubectl

# Development tools
sudo apt-get install -y \
    golang-go \
    git \
    wget \
    curl
```

### Verify Installation

```bash
# Check libvirt
sudo systemctl status libvirtd
virsh version

# Check Docker
docker version

# Check KIND
kind version

# Check kubectl
kubectl version --client

# Check Go
go version
```

### System Configuration (Running without sudo)

To run E2E tests without sudo, configure proper permissions:

#### 1. Add user to libvirt group

```bash
# Add your user to the libvirt group
sudo usermod -aG libvirt $USER

# Add your user to the kvm group
sudo usermod -aG kvm $USER

# Log out and log back in, or use:
newgrp libvirt
```

#### 2. Configure network operations

Network bridge creation requires privileges. Choose one option:

**Option A: Use sudo for specific operations (Recommended)**

Create a sudoers file for network operations:

```bash
sudo visudo -f /etc/sudoers.d/shaper-e2e
```

Add:
```
# Allow user to manage network bridges for E2E tests
%libvirt ALL=(ALL) NOPASSWD: /usr/sbin/ip link add *, /usr/sbin/ip link delete *, /usr/sbin/ip addr add *, /usr/sbin/ip link set *
%libvirt ALL=(ALL) NOPASSWD: /usr/sbin/dnsmasq
```

**Option B: Use polkit rules**

Create `/etc/polkit-1/rules.d/50-libvirt.rules`:

```javascript
polkit.addRule(function(action, subject) {
    if (action.id == "org.libvirt.unix.manage" &&
        subject.isInGroup("libvirt")) {
            return polkit.Result.YES;
    }
});
```

**Option C: Use capabilities (Advanced)**

```bash
# Grant network administration capabilities to the test binary
sudo setcap cap_net_admin+ep ./shaper-e2e
```

#### 3. Verify Configuration

```bash
# Test libvirt access (should not require password)
virsh list

# Test KIND access
kind version

# If everything is configured correctly, you can run:
shaper-e2e test
```

## Quick Start

### Using the shaper-e2e CLI Tool (Recommended)

The `shaper-e2e` CLI tool provides a convenient way to manage E2E test environments:

```bash
# Build the CLI tool
go build -o shaper-e2e ./cmd/shaper-e2e/

# Run one-shot test (create → test → cleanup)
./shaper-e2e test

# Or manage environments manually:

# 1. Create environment
./shaper-e2e create
# Output: e2e-shaper-abc12345

# 2. Run tests
./shaper-e2e run e2e-shaper-abc12345

# 3. View logs
./shaper-e2e logs e2e-shaper-abc12345 dnsmasq

# 4. Cleanup
./shaper-e2e delete e2e-shaper-abc12345

# List all environments
./shaper-e2e list
```

### Run All E2E Tests (Direct)

```bash
cd /home/alexandremahdhaoui/go/src/github.com/alexandremahdhaoui/shaper

# Run complete E2E test suite
go test -v -tags=e2e ./test/e2e/

# Run with timeout
go test -v -tags=e2e -timeout=30m ./test/e2e/
```

### Run Specific Test

```bash
# Run only the main iPXE boot flow test
go test -v -tags=e2e -run TestIPXEBootFlow_E2E ./test/e2e/

# Run only infrastructure verification tests
go test -v -tags=e2e -run TestIPXEBootFlow_E2E/BasicNetworkConnectivity ./test/e2e/
go test -v -tags=e2e -run TestIPXEBootFlow_E2E/DnsmasqRunning ./test/e2e/
go test -v -tags=e2e -run TestIPXEBootFlow_E2E/KindClusterAccessible ./test/e2e/
go test -v -tags=e2e -run TestIPXEBootFlow_E2E/IPXEBootFlow ./test/e2e/
```

## CLI Tool: shaper-e2e

### Overview

The `shaper-e2e` CLI tool is the recommended way to manage E2E test environments. It provides persistent environment management, allowing you to create, inspect, test, and cleanup environments independently.

### Building

```bash
# Build the CLI tool
go build -o shaper-e2e ./cmd/shaper-e2e/

# Install system-wide (optional)
sudo cp shaper-e2e /usr/local/bin/
```

### Commands

#### create

Create a new test environment with all infrastructure:

```bash
shaper-e2e create
# Output: e2e-shaper-abc12345
```

Creates:
- Linux network bridge
- Libvirt network
- Dnsmasq DHCP/TFTP server
- KIND Kubernetes cluster
- Artifact directories

#### run

Execute iPXE boot tests in an existing environment:

```bash
shaper-e2e run e2e-shaper-abc12345
```

Runs:
- Creates test VM
- Monitors DHCP lease
- Verifies TFTP boot
- Tests HTTP boot flow

#### get

Display detailed information about an environment:

```bash
shaper-e2e get e2e-shaper-abc12345
```

Shows:
- Network configuration
- KIND cluster details
- TFTP root location
- Dnsmasq configuration

#### logs

View logs from test environment components:

```bash
# View dnsmasq leases and config
shaper-e2e logs e2e-shaper-abc12345 dnsmasq

# View KIND cluster info
shaper-e2e logs e2e-shaper-abc12345 kind
```

#### list

List all managed test environments:

```bash
shaper-e2e list
```

Output:
```
ID                    Bridge          KIND Cluster   Kubeconfig
--                    --              --             --
e2e-shaper-abc12345  br-shaper-e2e   shaper-e2e     /home/user/.shaper/...
```

#### delete

Cleanup and remove a test environment:

```bash
shaper-e2e delete e2e-shaper-abc12345
```

Removes:
- All VMs
- Dnsmasq process
- Libvirt network
- Network bridge
- KIND cluster
- Temporary files

#### test

Run a complete one-shot test (create → test → cleanup):

```bash
shaper-e2e test
```

Perfect for:
- CI/CD pipelines
- Quick validation
- Automated testing

### Environment Variables

- `SHAPER_E2E_ARTIFACTS_DIR`: Override artifact storage (default: `~/.shaper/e2e/`)
- `SHAPER_E2E_IMAGE_CACHE`: Override image cache (default: `/tmp/shaper-e2e-images`)
- `SHAPER_E2E_DEBUG`: Enable debug logging (set to `"1"`)

### Examples

**Development workflow:**

```bash
# Create environment once
ENV_ID=$(shaper-e2e create)

# Run tests multiple times during development
shaper-e2e run $ENV_ID
# ... make code changes ...
shaper-e2e run $ENV_ID
# ... make more changes ...
shaper-e2e run $ENV_ID

# Cleanup when done
shaper-e2e delete $ENV_ID
```

**CI/CD pipeline:**

```bash
# Single command for complete test
shaper-e2e test
```

**Debugging:**

```bash
# Create environment
ENV_ID=$(shaper-e2e create)

# Inspect configuration
shaper-e2e get $ENV_ID

# Check dnsmasq
shaper-e2e logs $ENV_ID dnsmasq

# Check KIND cluster
shaper-e2e logs $ENV_ID kind
export KUBECONFIG=$(shaper-e2e get $ENV_ID | grep Kubeconfig | awk '{print $2}')
kubectl get nodes

# Run tests
shaper-e2e run $ENV_ID

# Keep environment for investigation or cleanup
shaper-e2e delete $ENV_ID
```

## Test Structure

### Main Test: TestIPXEBootFlow_E2E

The primary E2E test that orchestrates the complete environment and runs sub-tests:

```go
func TestIPXEBootFlow_E2E(t *testing.T) {
    // 1. Setup complete environment (bridge, libvirt, dnsmasq, KIND)
    env, err := e2e.SetupShaperTestEnvironment(setupConfig)
    defer e2e.TeardownShaperTestEnvironment(env)

    // 2. Run sub-tests
    t.Run("BasicNetworkConnectivity", ...)
    t.Run("DnsmasqRunning", ...)
    t.Run("KindClusterAccessible", ...)
    t.Run("IPXEBootFlow", ...)
}
```

### Sub-Tests

1. **BasicNetworkConnectivity**: Verifies network bridge exists and is configured
2. **DnsmasqRunning**: Checks dnsmasq process is running and serving DHCP
3. **KindClusterAccessible**: Validates KIND cluster is up and kubectl works
4. **IPXEBootFlow**: Executes full iPXE boot test with VM

### Future Tests (Placeholders)

- `TestIPXEBootFlow_WithProfile`: Boot with actual Profile CRD
- `TestIPXEBootFlow_MultipleProfiles`: Test multiple profiles and assignments
- `TestIPXEBootFlow_NoAssignment`: Test fallback behavior without assignment

## Configuration

### Environment Setup

The test creates a complete isolated environment:

```go
setupConfig := e2e.ShaperSetupConfig{
    ArtifactDir:     "/tmp/shaper-e2e-artifacts",
    ImageCacheDir:   "/tmp/shaper-e2e-images",
    BridgeName:      "br-shaper-e2e",
    NetworkCIDR:     "192.168.100.1/24",
    DHCPRange:       "192.168.100.10,192.168.100.250",
    KindClusterName: "shaper-e2e-test",
    TFTPRoot:        "/tmp/shaper-e2e/tftp",
    IPXEBootFile:    "/path/to/undionly.kpxe", // Optional
    NumClients:      0,  // Don't pre-create VMs
    DownloadImages:  false,
}
```

### Test VM Configuration

```go
testConfig := e2e.IPXETestConfig{
    Env:         env,
    VMName:      "test-client-" + env.ID,
    BootOrder:   []string{"network"},
    MemoryMB:    1024,
    VCPUs:       1,
    BootTimeout: 2 * time.Minute,
    DHCPTimeout: 30 * time.Second,
    HTTPTimeout: 1 * time.Minute,
}
```

## Test Execution Flow

### Phase 1: Environment Setup

1. **Generate unique test ID**: `e2e-shaper-<uuid>`
2. **Create directories**: Artifacts, temp files, TFTP root
3. **Create network bridge**: Linux bridge with specified CIDR
4. **Create libvirt network**: Attached to the bridge
5. **Start dnsmasq**: DHCP/TFTP/PXE server on the bridge
6. **Create KIND cluster**: Local Kubernetes cluster
7. **Deploy shaper** (optional): Apply CRDs and deployment manifests

### Phase 2: Test Execution

1. **Create test VM**: Network-only boot (no disk)
2. **Monitor DHCP**: Watch for lease in dnsmasq.leases file
3. **Verify TFTP**: Check for boot file fetch (via logs)
4. **Monitor HTTP**: Watch for shaper-api requests (via logs)
5. **Verify Assignment**: Check correct Assignment matched
6. **Verify Profile**: Confirm correct Profile returned
7. **Collect results**: Gather logs, errors, success status

### Phase 3: Teardown

1. **Destroy test VMs**: Clean up libvirt VMs
2. **Stop dnsmasq**: Graceful process termination
3. **Delete libvirt network**: Remove virtual network
4. **Delete KIND cluster**: Remove Kubernetes cluster
5. **Delete bridge**: Remove Linux bridge
6. **Clean temp files**: Remove all test artifacts

## Debugging

### Enable Verbose Logging

```bash
# Maximum verbosity
sudo go test -v -tags=e2e ./test/e2e/ 2>&1 | tee test.log
```

### Check Component Status

```bash
# Bridge
ip link show br-shaper-e2e
ip addr show br-shaper-e2e

# Libvirt network
virsh net-list --all
virsh net-info net-e2e-shaper-<uuid>

# Dnsmasq
ps aux | grep dnsmasq
sudo cat /tmp/e2e-shaper-<uuid>/dnsmasq.leases
sudo cat /tmp/e2e-shaper-<uuid>/dnsmasq.conf

# KIND cluster
kind get clusters
kubectl --kubeconfig=/tmp/shaper-e2e-artifacts/<uuid>/kubeconfig get nodes
kubectl --kubeconfig=<path> get pods -A

# VMs
virsh list --all
virsh dominfo test-client-<uuid>
```

### Network Traffic Monitoring

```bash
# Monitor bridge traffic
sudo tcpdump -i br-shaper-e2e -n

# Monitor DHCP specifically
sudo tcpdump -i br-shaper-e2e -n port 67 or port 68

# Monitor TFTP
sudo tcpdump -i br-shaper-e2e -n port 69

# Monitor HTTP to shaper-api
sudo tcpdump -i br-shaper-e2e -n port 80 or port 8080
```

### Manual DHCP Test

```bash
# Request DHCP lease on the bridge
sudo dhclient -v br-shaper-e2e

# Check lease file
cat /tmp/e2e-shaper-<uuid>/dnsmasq.leases

# Release lease
sudo dhclient -r br-shaper-e2e
```

### Manual VM Creation

```bash
# Use the test infrastructure programmatically
cat > test_manual.go <<'EOF'
package main

import (
    "github.com/alexandremahdhaoui/shaper/pkg/test/e2e"
)

func main() {
    config := e2e.ShaperSetupConfig{
        BridgeName: "br-manual-test",
        NetworkCIDR: "192.168.200.1/24",
        DHCPRange: "192.168.200.10,192.168.200.250",
        KindClusterName: "manual-test",
        TFTPRoot: "/tmp/manual-test/tftp",
    }

    env, err := e2e.SetupShaperTestEnvironment(config)
    if err != nil {
        panic(err)
    }

    // Environment is ready - test manually
    println("Environment ready!")
    println("Bridge:", env.BridgeName)
    println("Kubeconfig:", env.Kubeconfig)

    // Clean up when done
    // defer e2e.TeardownShaperTestEnvironment(env)
}
EOF

sudo go run test_manual.go
```

## Troubleshooting

### Issue: "Permission denied" errors

**Cause**: Insufficient privileges for network/VM operations.

**Solution**:

1. Ensure user is in libvirt and kvm groups:
```bash
groups $USER
# Should show: ... libvirt kvm ...
```

2. Configure sudo for network operations (see System Configuration section)

3. Verify libvirt access:
```bash
virsh list  # Should not require password
```

### Issue: "KIND not installed"

**Cause**: KIND binary not found in PATH.

**Solution**:
```bash
# Install KIND
curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.20.0/kind-linux-amd64
chmod +x ./kind
sudo mv ./kind /usr/local/bin/kind
```

### Issue: "kubectl not installed"

**Cause**: kubectl binary not found in PATH.

**Solution**:
```bash
# Install kubectl
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
chmod +x ./kubectl
sudo mv ./kubectl /usr/local/bin/kubectl
```

### Issue: "Failed to create bridge: File exists"

**Cause**: Bridge already exists from previous test run.

**Solution**:
```bash
# Delete existing bridge
sudo ip link delete br-shaper-e2e

# Or use idempotent operations (already built-in)
```

### Issue: "Failed to start dnsmasq: Address already in use"

**Cause**: Another dnsmasq instance running on the same interface.

**Solution**:
```bash
# Find conflicting process
sudo lsof -i :67
sudo lsof -i :69

# Kill it
sudo pkill dnsmasq

# Or use specific PID
sudo kill $(cat /tmp/e2e-shaper-<uuid>/dnsmasq.pid)
```

### Issue: "KIND cluster creation failed: node(s) already exist"

**Cause**: Cluster with same name exists.

**Solution**:
```bash
# Delete existing cluster
kind delete cluster --name shaper-e2e-test

# List all clusters
kind get clusters
```

### Issue: "DHCP lease not obtained"

**Cause**: VM not receiving DHCP response.

**Possible solutions**:

1. Check dnsmasq is running:
```bash
ps aux | grep dnsmasq
```

2. Check dnsmasq logs:
```bash
sudo journalctl -u dnsmasq -f
```

3. Verify bridge has IP:
```bash
ip addr show br-shaper-e2e
```

4. Check VM is on correct network:
```bash
virsh domiflist test-client-<uuid>
```

5. Monitor DHCP traffic:
```bash
sudo tcpdump -i br-shaper-e2e -n port 67 or port 68
```

### Issue: "Libvirt connection failed"

**Cause**: Libvirtd service not running.

**Solution**:
```bash
# Start libvirtd
sudo systemctl start libvirtd

# Enable on boot
sudo systemctl enable libvirtd

# Check status
sudo systemctl status libvirtd
```

### Issue: "Permission denied accessing /var/run/libvirt/libvirt-sock"

**Cause**: User not in libvirt group.

**Solution**:
```bash
# Add user to libvirt group
sudo usermod -aG libvirt $USER

# Re-login or use
newgrp libvirt

# Or run as root
sudo -E go test ...
```

## Helper Commands

### Network Debugging

```bash
# Check bridge exists and has IP
ip link show br-shaper-e2e
ip addr show br-shaper-e2e

# Test DHCP on bridge interface
sudo dhclient -v br-shaper-e2e

# Monitor all traffic on bridge
sudo tcpdump -i br-shaper-e2e -n

# Test connectivity from bridge
ping -I br-shaper-e2e 192.168.100.1
curl --interface br-shaper-e2e http://192.168.100.1
```

### Libvirt Debugging

```bash
# List all networks
virsh net-list --all

# Show network details
virsh net-info net-e2e-shaper-<uuid>

# Show network XML
virsh net-dumpxml net-e2e-shaper-<uuid>

# List all VMs
virsh list --all

# Show VM details
virsh dominfo test-client-<uuid>

# Show VM XML
virsh dumpxml test-client-<uuid>

# VM console (if needed)
virsh console test-client-<uuid>

# Force destroy VM
virsh destroy test-client-<uuid>
virsh undefine test-client-<uuid>
```

### KIND Debugging

```bash
# List clusters
kind get clusters

# Get nodes
kind get nodes --name shaper-e2e-test

# Load image into cluster
kind load docker-image myimage:tag --name shaper-e2e-test

# Export logs
kind export logs /tmp/kind-logs --name shaper-e2e-test

# Get kubeconfig
kind get kubeconfig --name shaper-e2e-test > /tmp/kubeconfig
```

### Kubernetes Debugging

```bash
# Use test kubeconfig
export KUBECONFIG=/tmp/shaper-e2e-artifacts/<uuid>/kubeconfig

# Check cluster
kubectl cluster-info
kubectl get nodes

# Check all resources
kubectl get all -A

# Check CRDs
kubectl get crds
kubectl get profiles
kubectl get assignments

# Check shaper-api pods
kubectl get pods -n default
kubectl logs <shaper-api-pod> -n default

# Describe pod
kubectl describe pod <shaper-api-pod> -n default
```

## Development

### Running Unit Tests

```bash
# Network package tests
go test -v ./pkg/network/

# KIND package tests
go test -v ./pkg/test/kind/

# E2E package tests (not full E2E)
go test -v ./pkg/test/e2e/
```

### Running Integration Tests

```bash
# Network integration tests (require root)
sudo go test -v ./pkg/network/ -run ".*Integration"

# KIND integration tests
go test -v ./pkg/test/kind/ -run ".*Integration"
```

### Test Tags

- **Unit tests**: No build tags, always run
- **Integration tests**: Named `*_Integration`, require actual systems
- **E2E tests**: `//go:build e2e` tag, require full environment

## CI/CD Integration

### GitHub Actions Example

```yaml
name: E2E Tests

on:
  push:
    branches: [ main ]
  pull_request:
    branches: [ main ]

jobs:
  e2e:
    runs-on: ubuntu-latest

    steps:
    - uses: actions/checkout@v3

    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.21'

    - name: Install dependencies
      run: |
        sudo apt-get update
        sudo apt-get install -y libvirt-daemon-system qemu-kvm dnsmasq

        # Install KIND
        curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.20.0/kind-linux-amd64
        chmod +x ./kind
        sudo mv ./kind /usr/local/bin/kind

        # Install kubectl
        curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
        chmod +x ./kubectl
        sudo mv ./kubectl /usr/local/bin/kubectl

    - name: Start libvirtd
      run: |
        sudo systemctl start libvirtd
        sudo systemctl status libvirtd

    - name: Run E2E tests
      run: |
        sudo -E go test -v -tags=e2e -timeout=30m ./test/e2e/
```

## Advanced Usage

### Custom Network Configuration

```go
setupConfig := e2e.ShaperSetupConfig{
    BridgeName:  "br-custom",
    NetworkCIDR: "10.20.30.1/24",
    DHCPRange:   "10.20.30.100,10.20.30.200",
    // ...
}
```

### Using Real iPXE Boot Files

```bash
# Download iPXE boot files
wget http://boot.ipxe.org/undionly.kpxe -O /tmp/undionly.kpxe

# Configure test to use them
setupConfig.IPXEBootFile = "/tmp/undionly.kpxe"
```

### Testing with Actual VM Images

```bash
# Download Ubuntu cloud image
wget https://cloud-images.ubuntu.com/releases/noble/release/ubuntu-24.04-server-cloudimg-amd64.img \
    -O /tmp/ubuntu.img

# Configure test
setupConfig.ClientImagePath = "/tmp/ubuntu.img"
setupConfig.DownloadImages = true
```

### Deploying Shaper to Test Cluster

```go
setupConfig := e2e.ShaperSetupConfig{
    // ... other config ...
    CRDPaths: []string{
        "config/crd/profile.yaml",
        "config/crd/assignment.yaml",
    },
    DeploymentPath: "config/deployment/shaper-api.yaml",
}
```

## Performance

### Typical Test Duration

- Environment setup: 30-60 seconds
- Single VM boot test: 30-90 seconds
- Full E2E test: 2-3 minutes
- Teardown: 10-20 seconds

### Resource Usage

- **CPU**: 2-4 cores during test execution
- **Memory**: 2-4 GB total (VMs + containers)
- **Disk**: 5-10 GB for images and artifacts
- **Network**: Minimal external traffic

## Best Practices

1. **Always run as root**: Network operations require elevated privileges
2. **Use unique test IDs**: Avoid conflicts between parallel test runs
3. **Clean up manually if needed**: Tests should auto-cleanup, but verify
4. **Monitor system resources**: Don't run too many parallel E2E tests
5. **Cache VM images**: Set `ImageCacheDir` to avoid re-downloading
6. **Use short timeouts for CI**: Fail fast in automated environments
7. **Collect artifacts**: Save logs, configs, and screenshots for debugging

## Contributing

### Adding New Tests

1. Create test function in `test/e2e/`
2. Use `//go:build e2e` tag
3. Follow naming convention: `TestIPXEBootFlow_<Scenario>`
4. Use `e2e.SetupShaperTestEnvironment()` for setup
5. Always defer `e2e.TeardownShaperTestEnvironment()`
6. Log extensively for debugging
7. Update this README

### Test Template

```go
//go:build e2e

package e2e_test

import (
    "testing"
    "github.com/alexandremahdhaoui/shaper/pkg/test/e2e"
    "github.com/stretchr/testify/require"
)

func TestIPXEBootFlow_MyScenario(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping E2E test in short mode")
    }

    // Setup
    config := e2e.ShaperSetupConfig{
        // ... configuration ...
    }

    env, err := e2e.SetupShaperTestEnvironment(config)
    require.NoError(t, err)
    defer e2e.TeardownShaperTestEnvironment(env)

    // Test logic here
    t.Log("Testing my scenario...")

    // Assertions
    require.True(t, someCondition, "condition should be true")
}
```

## References

- [Libvirt Go Bindings](https://libvirt.org/go/libvirt.html)
- [KIND Documentation](https://kind.sigs.k8s.io/)
- [iPXE Documentation](https://ipxe.org/docs)
- [Dnsmasq Manual](https://thekelleys.org.uk/dnsmasq/doc.html)
- [Kubernetes CRDs](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/)

## License

Copyright 2025 Alexandre Mahdhaoui

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
