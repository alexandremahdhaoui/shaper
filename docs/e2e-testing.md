# Shaper E2E Testing Framework

Complete guide to the Shaper end-to-end testing framework for validating iPXE boot flows.

## Table of Contents

1. [Introduction](#introduction)
2. [Quick Start](#quick-start)
3. [Architecture](#architecture)
4. [Test Scenarios](#test-scenarios)
5. [Assertions](#assertions)
6. [Infrastructure](#infrastructure)
7. [Running Tests](#running-tests)
8. [Debugging](#debugging)
9. [Advanced Topics](#advanced-topics)
10. [Contributing](#contributing)

## Introduction

### What is the E2E Framework?

The Shaper E2E testing framework provides comprehensive end-to-end testing for the complete iPXE boot flow, from initial PXE boot to configuration retrieval. It automates the complex process of setting up test infrastructure, provisioning VMs, deploying Shaper components, and validating boot behavior.

### Why Use It?

**Comprehensive Testing**: Unlike unit or integration tests, E2E tests validate the complete system including:
- Network infrastructure (bridges, DHCP, TFTP)
- Kubernetes cluster deployment
- VM provisioning and boot sequence
- Shaper API behavior with real iPXE clients
- Profile and Assignment CRD matching logic
- Configuration resolution and transformation

**Reproducible Environments**: Every test runs in an isolated, cleanly provisioned environment that can be torn down and recreated on demand.

**Declarative Test Definition**: Tests are defined in simple YAML files that describe what to test, not how to test it.

**CI/CD Integration**: Forge integration enables seamless test execution in continuous integration pipelines.

### Key Features

- **Declarative YAML scenarios**: Define VMs, resources, and assertions in readable YAML
- **Parallel VM orchestration**: Multiple VMs boot and test simultaneously
- **Automated infrastructure**: Bridge networks, DHCP, TFTP, KIND clusters all provisioned automatically
- **Multiple assertion types**: Verify DHCP, TFTP, HTTP, Profile matching, and more
- **Detailed reporting**: Human-readable and JSON reports with metrics and timelines
- **Forge integration**: Managed via `forge test e2e` commands
- **Extensible design**: Add custom assertions and formatters

## Quick Start

Get your first E2E test running in 5 minutes.

### Prerequisites

Ensure you have these tools installed:

```bash
# Check prerequisites
go version          # Go 1.24+
kind version        # Kubernetes in Docker
kubectl version     # Kubernetes CLI
virsh version       # Libvirt (for VMs)
sudo -v             # Requires sudo for network setup
```

### Run Your First Test

```bash
# Navigate to project root
cd /home/alexandremahdhaoui/go/src/github.com/alexandremahdhaoui/shaper

# Create test environment (provisions infrastructure)
forge test e2e create

# Run basic boot scenario
forge test e2e run --scenario basic-boot

# Check results
cat ~/.shaper/e2e/artifacts/<test-id>/report.txt

# View detailed JSON report
jq . ~/.shaper/e2e/artifacts/<test-id>/report.json

# Cleanup
forge test e2e delete <test-id>
```

### What Just Happened?

1. **Infrastructure provisioned**: Bridge network (`br-shaper`), dnsmasq for DHCP/TFTP, libvirt network
2. **KIND cluster created**: Kubernetes cluster with Shaper CRDs and controllers deployed
3. **Test VM booted**: Virtual machine configured to network boot
4. **Resources applied**: Default Profile and Assignment CRDs created
5. **Assertions validated**: DHCP lease, TFTP boot, HTTP calls, Profile matching verified
6. **Report generated**: Human-readable and JSON reports saved to artifacts directory

## Architecture

### High-Level Components

The E2E framework consists of seven major components:

```
┌─────────────────────────────────────────────────────────────┐
│                    Forge Integration Layer                   │
│                   (forge test e2e commands)                  │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│                     Test Coordinator                         │
│              (Orchestrates complete test flow)               │
└──┬────────┬────────┬────────┬────────┬────────┬─────────────┘
   │        │        │        │        │        │
   ▼        ▼        ▼        ▼        ▼        ▼
┌──────┐ ┌─────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐
│Loader│ │Infra│ │  VM  │ │ K8s  │ │ Test │ │Report│
│      │ │ Mgr │ │ Orch │ │Apply │ │ Exec │ │      │
└──────┘ └─────┘ └──────┘ └──────┘ └──────┘ └──────┘
```

### Component Responsibilities

**Scenario Loader**: Parses and validates YAML test scenarios

**Infrastructure Manager**: Provisions bridges, DHCP, TFTP, libvirt networks, KIND clusters

**VM Orchestrator**: Creates and manages test VMs (supports parallel provisioning)

**Resource Applier**: Applies Kubernetes resources (Profiles, Assignments, ConfigMaps, Secrets)

**Test Executor**: Runs assertions and collects results

**Reporter**: Generates human-readable and JSON reports

### Test Execution Flow

```
1. LOAD SCENARIO
   ↓
2. PROVISION INFRASTRUCTURE
   ├─ Create bridge network
   ├─ Start dnsmasq (DHCP/TFTP)
   ├─ Create libvirt network
   ├─ Create KIND cluster
   └─ Deploy Shaper components
   ↓
3. APPLY KUBERNETES RESOURCES
   ├─ Profiles
   ├─ Assignments
   ├─ ConfigMaps
   └─ Secrets
   ↓
4. PROVISION & BOOT VMs (PARALLEL)
   ├─ VM 1 → Network boot → iPXE
   ├─ VM 2 → Network boot → iPXE
   └─ VM N → Network boot → iPXE
   ↓
5. EXECUTE ASSERTIONS
   ├─ DHCP lease checks
   ├─ TFTP boot checks
   ├─ HTTP endpoint checks
   ├─ Profile/Assignment matching
   └─ Config retrieval validation
   ↓
6. REPORT RESULTS
   ├─ Console output (human-readable)
   └─ JSON report (CI/CD)
```

### Integration with Existing Code

The E2E framework leverages existing Shaper packages:

- **pkg/network**: BridgeManager, DnsmasqManager, LibvirtNetworkManager
- **pkg/vmm**: VM creation and management
- **pkg/test/kind**: KIND cluster provisioning and Shaper deployment
- **pkg/cloudinit**: VM initialization

## Test Scenarios

### Scenario Format

Test scenarios are defined in YAML files under `test/e2e/scenarios/`. Each scenario specifies:

- **Metadata**: Name, description, tags, architecture
- **VMs**: Virtual machines to provision
- **Resources**: Kubernetes resources to apply
- **Assertions**: Validations to perform
- **Timeouts**: Operation timeout configuration
- **Expected outcome**: Documentation of expected result

### Scenario Structure

```yaml
name: "Human-readable test name"
description: |
  Multi-line description explaining what this scenario tests.

tags: ["category", "priority"]
architecture: "x86_64"

vms:
  - name: "vm-name"
    uuid: "explicit-uuid"       # Optional, auto-generated if omitted
    macAddress: "52:54:00:..."  # Optional, auto-generated if omitted
    memory: "1024"              # MB
    vcpus: 1
    bootOrder: ["network"]

resources:
  - kind: "Profile"
    name: "my-profile"
    namespace: "shaper-system"
    yaml: |
      apiVersion: shaper.amahdha.com/v1alpha1
      kind: Profile
      metadata:
        name: my-profile
        namespace: shaper-system
        labels:
          profile: "custom"
      spec:
        ipxe: |
          #!ipxe
          echo Custom iPXE script
          shell

assertions:
  - type: "dhcp_lease"
    vm: "vm-name"
    description: "VM obtains DHCP lease"

  - type: "profile_match"
    vm: "vm-name"
    expected: "my-profile"
    description: "Correct Profile is returned"

timeouts:
  dhcpLease: "30s"
  httpBoot: "120s"

expectedOutcome:
  status: "passed"
  description: "What success looks like"
```

### Available Scenarios

The framework includes these example scenarios:

**basic-boot.yaml** (⭐ Beginner)
- Single VM, default assignment
- Validates basic PXE boot flow
- Good starting point for new tests

**assignment-match.yaml** (⭐⭐ Intermediate)
- Tests Assignment subject selector matching
- Explicit UUID/MAC matching
- Custom Assignment configuration

**multi-vm.yaml** (⭐⭐⭐ Advanced)
- Two VMs with different roles
- Parallel VM provisioning
- Role-based Assignment selection

**profile-selection.yaml** (⭐⭐ Intermediate)
- Multiple Profiles with labels
- Tests Profile label selector logic

**config-retrieval.yaml** (⭐⭐⭐ Advanced)
- Tests /config/{uuid} endpoint
- Butane transformation pipeline
- Content resolution and serving

### Creating Custom Scenarios

See `test/e2e/scenarios/README.md` for detailed scenario creation guide including:
- Step-by-step creation process
- Template expansion with VM data
- Best practices and tips
- Troubleshooting common issues

## Assertions

### Available Assertion Types

**dhcp_lease**
- **Purpose**: Verify VM obtained DHCP lease
- **Check**: Parses dnsmasq lease file for VM's MAC address
- **Timeout**: Configurable (default 30s)

```yaml
- type: "dhcp_lease"
  vm: "test-vm"
  description: "VM obtains DHCP lease from dnsmasq"
```

**tftp_boot**
- **Purpose**: Verify VM fetched boot file via TFTP
- **Check**: Parses dnsmasq TFTP logs for file requests
- **Timeout**: Configurable (default 60s)

```yaml
- type: "tftp_boot"
  vm: "test-vm"
  description: "VM fetches boot file via TFTP"
```

**http_boot_called**
- **Purpose**: Verify VM called shaper-API HTTP endpoint
- **Check**: Reads shaper-API pod logs for HTTP requests from VM
- **Timeout**: Configurable (default 120s)

```yaml
- type: "http_boot_called"
  vm: "test-vm"
  description: "VM calls shaper-API /boot.ipxe or /ipxe"
```

**assignment_match**
- **Purpose**: Verify correct Assignment was selected
- **Check**: Parses shaper-API response to validate Assignment name
- **Expected**: Assignment name

```yaml
- type: "assignment_match"
  vm: "test-vm"
  expected: "custom-assignment"
  description: "Custom Assignment is selected"
```

**profile_match**
- **Purpose**: Verify correct Profile was returned
- **Check**: Validates Profile name in API response
- **Expected**: Profile name

```yaml
- type: "profile_match"
  vm: "test-vm"
  expected: "custom-profile"
  description: "Custom Profile is returned"
```

**http_endpoint_accessible**
- **Purpose**: Verify specific HTTP endpoint is accessible
- **Check**: Makes HTTP request to endpoint from test host
- **Expected**: Endpoint URL

```yaml
- type: "http_endpoint_accessible"
  vm: "test-vm"
  expected: "http://shaper-api/config/some-uuid"
  description: "Config endpoint is accessible"
```

**config_content_match**
- **Purpose**: Verify configuration content matches expected values
- **Check**: Retrieves config and validates content
- **Expected**: Content pattern or hash

```yaml
- type: "config_content_match"
  vm: "test-vm"
  expected: "ignition-version-3.4.0"
  description: "Ignition config is correct version"
```

### Assertion Execution

Assertions are executed sequentially after VMs boot. Each assertion:

1. **Polls with timeout**: Retries until success or timeout
2. **Records result**: Success/failure, actual vs expected, duration
3. **Collects logs**: Relevant log snippets for debugging
4. **Continues on failure**: Does not halt remaining assertions

## Infrastructure

### Network Setup

The framework provisions a complete network stack:

**Bridge Network** (`br-shaper`)
- Virtual bridge connecting test VMs to KIND cluster
- Default CIDR: `192.168.100.0/24`
- Created using BridgeManager (`pkg/network`)

**Dnsmasq Service**
- Provides DHCP server for VM IP allocation
- Provides TFTP server for initial boot file serving
- DHCP range: `192.168.100.100-192.168.100.200`
- Lease file: `<tempdir>/dnsmasq.leases`
- Logs: `<tempdir>/dnsmasq.log`

**Libvirt Network**
- Connects VMs to bridge network
- Enables VM-to-KIND communication
- NAT mode for external access

### KIND Cluster

**Cluster Configuration**:
- Name: `shaper-e2e-<test-id>`
- Nodes: 1 control-plane node
- Network: Connected to bridge via CNI configuration
- Kubeconfig: Saved to `<tempdir>/kubeconfig`

**Shaper Deployment**:
- Namespace: `shaper-system`
- CRDs: Installed from `charts/shaper-crds/`
- Components:
  - shaper-api: HTTP server for iPXE scripts
  - shaper-controller: CRD reconciliation
  - shaper-webhook: Admission webhooks

### TFTP Boot Files

The framework prepares TFTP boot files:

- **undionly.kpxe**: Initial chainload file for network boot
- Served from: `<tempdir>/tftp/`
- Downloaded automatically if not present

### VM Provisioning

VMs are provisioned using `pkg/vmm`:

**Default Configuration**:
- Boot order: Network first
- Network: Connected to libvirt network
- Console: Serial and VNC available
- State: Running (auto-start)

**Resource Allocation**:
- Memory: Configurable per VM (default 1024MB)
- vCPUs: Configurable per VM (default 1)
- Disk: Optional (diskless boot supported)

**Parallel Provisioning**:
- Multiple VMs created in parallel using goroutines
- Reduces total test time for multi-VM scenarios
- Error handling collects all failures

### Environment Persistence

Test environments are persisted to disk:

**Location**: `~/.shaper/e2e/testenvs/<test-id>.json`

**Contains**:
- Infrastructure IDs (bridge, dnsmasq, KIND cluster)
- Paths (kubeconfig, TFTP root, temp dirs)
- Timestamps (created at, last used)
- VM metadata

**Lifecycle**:
- Created by `forge test e2e create`
- Reused for multiple test runs
- Deleted by `forge test e2e delete <test-id>`

## Running Tests

### Using Forge (Recommended)

**List test environments**:
```bash
forge test e2e list
```

Example output:
```
ID              SCENARIO         STATUS    CREATED
abc123          basic-boot       ready     2 minutes ago
def456          multi-vm         running   5 minutes ago
```

**Create test environment**:
```bash
# Create without running tests (for inspection)
forge test e2e create

# Create and run specific scenario
forge test e2e create --scenario basic-boot
```

**Run tests**:
```bash
# Run all scenarios
forge test e2e run

# Run specific scenario
forge test e2e run --scenario basic-boot

# Run in existing environment
forge test e2e run --test-id abc123 --scenario assignment-match
```

**Get environment details**:
```bash
forge test e2e get <test-id>
```

Example output:
```json
{
  "id": "abc123",
  "scenario": "basic-boot",
  "status": "ready",
  "infrastructure": {
    "bridge": "br-shaper",
    "kindCluster": "shaper-e2e-abc123",
    "kubeconfig": "/tmp/shaper-e2e-abc123/kubeconfig"
  }
}
```

**Delete environment**:
```bash
# Delete specific environment
forge test e2e delete <test-id>

# Delete all environments
forge test e2e delete --all
```

### Using Go Test

For development and debugging:

```bash
# Run specific scenario
export SHAPER_E2E_SCENARIO=test/e2e/scenarios/basic-boot.yaml
go test ./test/e2e -v -run TestE2EScenario

# Run with custom timeout
go test ./test/e2e -v -timeout 30m

# Run with race detector (slower but finds concurrency bugs)
go test ./test/e2e -v -race
```

### Using shaper-e2e Binary

Build and run standalone binary:

```bash
# Build binary
forge build shaper-e2e-binary

# Run scenario
./bin/shaper-e2e run --scenario test/e2e/scenarios/basic-boot.yaml

# Create environment only
./bin/shaper-e2e setup --scenario test/e2e/scenarios/basic-boot.yaml

# Cleanup
./bin/shaper-e2e teardown --test-id <test-id>
```

### Test Options

**Verbosity**:
```bash
# Minimal output
forge test e2e run --quiet

# Verbose output
forge test e2e run --verbose

# Debug output (includes all logs)
forge test e2e run --debug
```

**Report formats**:
```bash
# Human-readable (default)
forge test e2e run --format human

# JSON (for CI/CD)
forge test e2e run --format json

# JUnit XML
forge test e2e run --format junit
```

**Cleanup behavior**:
```bash
# Cleanup on success, keep on failure (default)
forge test e2e run --cleanup-on-success

# Always cleanup
forge test e2e run --cleanup-always

# Never cleanup (for debugging)
forge test e2e run --no-cleanup
```

## Debugging

### Reading Logs

**Framework logs**:
```bash
# Framework execution log
cat ~/.shaper/e2e/artifacts/<test-id>/framework.log

# View in real-time
tail -f ~/.shaper/e2e/artifacts/<test-id>/framework.log
```

**Infrastructure logs**:
```bash
# Dnsmasq logs (DHCP/TFTP)
cat ~/.shaper/e2e/artifacts/<test-id>/dnsmasq.log

# KIND cluster logs
kubectl --kubeconfig <kubeconfig> get events -A
```

**Shaper component logs**:
```bash
# shaper-API logs
kubectl --kubeconfig <kubeconfig> logs -n shaper-system deployment/shaper-api

# shaper-controller logs
kubectl --kubeconfig <kubeconfig> logs -n shaper-system deployment/shaper-controller

# Follow logs in real-time
kubectl --kubeconfig <kubeconfig> logs -n shaper-system -f deployment/shaper-api
```

**VM console logs**:
```bash
# Serial console output
cat ~/.shaper/e2e/artifacts/<test-id>/vms/<vm-name>/serial.log

# Connect to VM console (if still running)
virsh console <vm-name>
```

### Common Issues

#### Test Environment Creation Fails

**Symptom**: `failed to create bridge` or `KIND cluster creation failed`

**Diagnosis**:
```bash
# Check if bridge already exists
ip addr show br-shaper

# Check for conflicting KIND clusters
kind get clusters

# Check libvirt connection
virsh list --all
```

**Solution**:
```bash
# Clean up existing resources
sudo ip link delete br-shaper 2>/dev/null
kind delete cluster --name shaper-e2e-*
sudo pkill dnsmasq

# Retry creation
forge test e2e create
```

#### VM Doesn't Obtain DHCP Lease

**Symptom**: `assertion failed: dhcp_lease timeout`

**Diagnosis**:
```bash
# Check dnsmasq is running
ps aux | grep dnsmasq

# Check dnsmasq lease file
cat <tempdir>/dnsmasq.leases

# Check VM network interface
virsh domiflist <vm-name>

# Check bridge connectivity
ip addr show br-shaper
```

**Solution**:
- Verify firewall allows DHCP (UDP 67/68)
- Check dnsmasq logs for errors
- Verify VM MAC address is correct
- Restart dnsmasq if needed

#### Assignment Not Matched

**Symptom**: `expected assignment 'X' but got 'default'`

**Diagnosis**:
```bash
# Check Assignments exist
kubectl --kubeconfig <kubeconfig> get assignments -n shaper-system

# Describe Assignment
kubectl --kubeconfig <kubeconfig> describe assignment <name> -n shaper-system

# Check shaper-API logs for selection logic
kubectl --kubeconfig <kubeconfig> logs -n shaper-system deployment/shaper-api | grep -i assignment
```

**Solution**:
- Verify Assignment `subjectSelectors` match VM UUID/MAC
- Verify Assignment `buildArch` matches VM architecture
- Check Assignment was created before VM boot
- Verify shaper-controller reconciled Assignment (added labels)

#### Profile Not Found

**Symptom**: `profile not found` or `expected profile 'X' but got 'Y'`

**Diagnosis**:
```bash
# Check Profiles exist
kubectl --kubeconfig <kubeconfig> get profiles -n shaper-system

# Check Profile status (should have UUIDs)
kubectl --kubeconfig <kubeconfig> get profile <name> -n shaper-system -o yaml

# Check Assignment profileSelectors
kubectl --kubeconfig <kubeconfig> get assignment <name> -n shaper-system -o yaml
```

**Solution**:
- Verify Profile exists and has correct labels
- Verify Assignment `profileSelectors` match Profile labels
- Wait for shaper-controller to reconcile Profile (adds status UUIDs)
- Check shaper-API logs for Profile retrieval errors

#### Shaper Components Not Ready

**Symptom**: `timeout waiting for resources` or HTTP errors

**Diagnosis**:
```bash
# Check pod status
kubectl --kubeconfig <kubeconfig> get pods -n shaper-system

# Check pod events
kubectl --kubeconfig <kubeconfig> describe pod -n shaper-system <pod-name>

# Check CRDs installed
kubectl --kubeconfig <kubeconfig> get crds | grep shaper
```

**Solution**:
```bash
# Wait for pods to be ready
kubectl --kubeconfig <kubeconfig> wait --for=condition=Ready --timeout=300s -n shaper-system pod -l app=shaper-api

# Check CRD installation
kubectl --kubeconfig <kubeconfig> apply -f charts/shaper-crds/templates/crds/

# Restart deployments if needed
kubectl --kubeconfig <kubeconfig> rollout restart -n shaper-system deployment/shaper-api
```

### Debugging Techniques

**Keep environment for inspection**:
```bash
# Run test without cleanup
forge test e2e run --no-cleanup --scenario basic-boot

# Inspect infrastructure
kubectl --kubeconfig <kubeconfig> get all -A
virsh list --all
ip addr show br-shaper

# Access KIND cluster
export KUBECONFIG=<kubeconfig>
kubectl get pods -A

# When done, cleanup manually
forge test e2e delete <test-id>
```

**Increase timeouts for debugging**:
```yaml
# In scenario YAML
timeouts:
  dhcpLease: "300s"    # 5 minutes instead of 30s
  httpBoot: "600s"     # 10 minutes instead of 2 minutes
  vmProvision: "900s"  # 15 minutes instead of 3 minutes
```

**Enable verbose logging**:
```bash
# Set log level
export SHAPER_E2E_LOG_LEVEL=debug

# Run with verbose output
forge test e2e run --verbose --scenario basic-boot
```

**Manual VM console access**:
```bash
# Connect to VM serial console
virsh console <vm-name>

# View VM XML configuration
virsh dumpxml <vm-name>

# Check VM network interfaces
virsh domiflist <vm-name>
```

## Advanced Topics

### Multi-VM Tests

**Parallel VM provisioning** reduces total test time:

```yaml
vms:
  - name: "worker-1"
    memory: "2048"
    vcpus: 2
    labels:
      role: "worker"

  - name: "worker-2"
    memory: "2048"
    vcpus: 2
    labels:
      role: "worker"

  - name: "control-plane"
    memory: "4096"
    vcpus: 4
    labels:
      role: "control-plane"
```

VMs are created in parallel using goroutines, significantly reducing provisioning time.

**Role-based Assignments**:

```yaml
# Worker assignment
- kind: "Assignment"
  name: "worker-assignment"
  namespace: "shaper-system"
  yaml: |
    spec:
      subjectSelectors:
        matchLabels:
          role: "worker"
      profileSelectors:
        matchLabels:
          profile: "worker"

# Control-plane assignment
- kind: "Assignment"
  name: "control-plane-assignment"
  namespace: "shaper-system"
  yaml: |
    spec:
      subjectSelectors:
        matchLabels:
          role: "control-plane"
      profileSelectors:
        matchLabels:
          profile: "control-plane"
```

### Custom Assertions

To add custom assertion types:

1. **Implement Asserter interface**:

```go
package orchestration

type CustomAsserter struct {
    // configuration fields
}

func (a *CustomAsserter) Assert(ctx context.Context, env *infrastructure.Environment, vm *VMInstance, expected string) (*AssertionResult, error) {
    // Custom assertion logic
    // Poll, check, validate

    return &AssertionResult{
        Type:        "custom_check",
        VM:          vm.Name,
        Expected:    expected,
        Actual:      actualValue,
        Success:     matches,
        Description: "Custom validation description",
        Duration:    time.Since(start),
    }, nil
}

func (a *CustomAsserter) Type() string {
    return "custom_check"
}
```

2. **Register in asserter factory** (`pkg/test/e2e/orchestration/asserters.go`):

```go
func NewAsserter(assertionType string) (Asserter, error) {
    switch assertionType {
    case "dhcp_lease":
        return &DHCPLeaseAsserter{}, nil
    case "custom_check":
        return &CustomAsserter{}, nil
    default:
        return nil, ErrUnknownAssertionType
    }
}
```

3. **Use in scenarios**:

```yaml
assertions:
  - type: "custom_check"
    vm: "test-vm"
    expected: "some-value"
    description: "Custom validation"
```

### Template Expansion

Resource YAML supports Go template expansion with VM data:

```yaml
resources:
  - kind: "Assignment"
    name: "vm-specific-assignment"
    namespace: "shaper-system"
    yaml: |
      apiVersion: shaper.amahdha.com/v1alpha1
      kind: Assignment
      metadata:
        name: assignment-{{.VMs["test-vm"].Name}}
        namespace: shaper-system
      spec:
        subjectSelectors:
          matchLabels:
            uuid: "{{.VMs["test-vm"].UUID}}"
            mac: "{{.VMs["test-vm"].MACAddress}}"
        profileSelectors:
          matchLabels:
            profile: "custom"
```

**Available template variables**:
- `{{.VMs["vm-name"].Name}}`: VM name
- `{{.VMs["vm-name"].UUID}}`: VM UUID
- `{{.VMs["vm-name"].MACAddress}}`: VM MAC address
- `{{.VMs["vm-name"].IP}}`: VM IP address (after DHCP)

### Test Grid Integration

For CI/CD pipelines, leverage JSON reports:

```bash
# Run test and generate JSON report
forge test e2e run --format json --output results.json --scenario basic-boot

# Parse results in CI
jq '.status' results.json  # "passed" or "failed"
jq '.passRate' results.json  # Pass percentage

# Upload to test grid
curl -X POST https://testgrid.example.com/api/results \
  -H "Content-Type: application/json" \
  -d @results.json
```

**JSON report schema**:

```json
{
  "version": "1.0",
  "testID": "abc123",
  "scenarioName": "Basic Boot Test",
  "success": true,
  "status": "passed",
  "duration": "2m15s",
  "totalAssertions": 4,
  "passedAssertions": 4,
  "failedAssertions": 0,
  "passRate": 100.0,
  "vms": [
    {
      "name": "test-vm",
      "uuid": "550e8400-e29b-41d4-a716-446655440000",
      "status": "passed",
      "metrics": {
        "provisionTime": 15.3,
        "dhcpLeaseTime": 2.1,
        "httpBootTime": 3.5
      },
      "assertions": [...]
    }
  ]
}
```

### Performance Metrics

The framework collects detailed performance metrics:

**VM-level metrics**:
- Provision time: VM creation duration
- DHCP lease time: Time to obtain IP
- TFTP boot time: Time to fetch boot file
- HTTP boot time: Time to call shaper-API
- First response time: Time to first API response

**Accessing metrics**:

```bash
# View metrics in JSON report
jq '.vms[].metrics' report.json

# Extract specific metric
jq '.vms[] | select(.name=="test-vm") | .metrics.httpBootTime' report.json
```

**Example metrics**:

```json
{
  "provisionTime": 15.3,
  "dhcpLeaseTime": 2.1,
  "tftpBootTime": 1.8,
  "httpBootTime": 3.5,
  "firstResponseTime": 4.2
}
```

### Custom Report Formatters

To add custom report formats:

1. **Implement Formatter interface**:

```go
package reporting

type CustomFormatter struct{}

func (f *CustomFormatter) Format(results *orchestration.TestResults) ([]byte, error) {
    // Custom formatting logic
    // Could be XML, CSV, HTML, etc.
    return customBytes, nil
}

func (f *CustomFormatter) ContentType() string {
    return "text/custom"
}
```

2. **Register formatter**:

```go
func NewFormatter(format ReportFormat) (Formatter, error) {
    switch format {
    case ReportFormatHuman:
        return &HumanFormatter{}, nil
    case ReportFormatJSON:
        return &JSONFormatter{}, nil
    case ReportFormatCustom:
        return &CustomFormatter{}, nil
    default:
        return nil, ErrUnknownFormat
    }
}
```

## Contributing

### Adding New Scenarios

1. **Create scenario YAML** in `test/e2e/scenarios/`:

```bash
cp test/e2e/scenarios/basic-boot.yaml test/e2e/scenarios/my-scenario.yaml
```

2. **Customize scenario**:
   - Update name, description, tags
   - Modify VM configuration
   - Define Kubernetes resources
   - Add assertions

3. **Validate scenario**:

```bash
# Check YAML syntax
yamllint test/e2e/scenarios/my-scenario.yaml

# Validate scenario structure
go test ./pkg/test/e2e/scenario -v -run TestLoadExampleScenarios
```

4. **Test scenario**:

```bash
forge test e2e run --scenario my-scenario
```

5. **Document scenario** in `test/e2e/scenarios/README.md`:
   - Add to "Available Scenarios" section
   - Describe purpose and features
   - Indicate complexity level

### Extending the Framework

**Adding assertion types**:
- Implement `Asserter` interface in `pkg/test/e2e/orchestration/asserters.go`
- Register in asserter factory
- Add unit tests
- Document in this guide

**Adding infrastructure components**:
- Extend `InfrastructureManager` in `pkg/test/e2e/infrastructure/manager.go`
- Leverage existing managers from `pkg/network/`
- Ensure idempotent Create/Delete operations
- Track resources for cleanup

**Adding report formats**:
- Implement `Formatter` interface in `pkg/test/e2e/reporting/formatters.go`
- Register in formatter factory
- Add format flag to CLI
- Document format specification

### Testing Framework Code

**Unit tests**:
```bash
# Test scenario loading
go test ./pkg/test/e2e/scenario -v

# Test infrastructure management
go test ./pkg/test/e2e/infrastructure -v

# Test orchestration
go test ./pkg/test/e2e/orchestration -v
```

**Integration tests**:
```bash
# Run with e2e build tag
go test -tags=e2e ./pkg/test/e2e/... -v
```

**Coverage**:
```bash
go test ./pkg/test/e2e/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Code Style

Follow Shaper project conventions:

- **Interface-driven design**: Define interfaces for all major components
- **Context propagation**: Accept `context.Context` for all I/O operations
- **Error handling**: Use `errors.Join` for multi-error scenarios, define sentinel errors
- **Table-driven tests**: Use `[]struct{name, input, expected}` pattern
- **Documentation**: Add godoc comments for exported types and functions

### Pull Request Checklist

When contributing to the E2E framework:

- [ ] Scenario YAML is valid and well-documented
- [ ] Code follows project patterns (interfaces, errors, context)
- [ ] Unit tests added for new functionality
- [ ] Integration tests pass (if applicable)
- [ ] Documentation updated (this guide and scenario README)
- [ ] Example usage provided for new features
- [ ] Backward compatibility maintained (or breaking changes documented)

## Further Reading

- **Scenario Creation Guide**: `test/e2e/scenarios/README.md`
- **Framework Architecture**: `.ai/plan/e2e-framework/architecture.md`
- **Report Format Specification**: `.ai/plan/e2e-framework/reporting-format.md`
- **Shaper CRD Documentation**: `charts/shaper-crds/README.md`
- **iPXE Boot Flow**: Main project README

## Support

For issues, questions, or contributions:

- **Issues**: GitHub Issues for bug reports and feature requests
- **Discussions**: GitHub Discussions for questions and ideas
- **Contributing**: See CONTRIBUTING.md for contribution guidelines

---

**Framework Version**: 1.0
**Last Updated**: 2025-01-17
