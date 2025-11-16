# E2E Test Scenarios

This directory contains YAML-based test scenarios for the Shaper E2E testing framework. Each scenario defines a complete test case including infrastructure, VMs, Kubernetes resources, and assertions.

## Scenario Structure

Each scenario is a YAML file with the following structure:

```yaml
name: "Human-readable test name"
description: |
  Multi-line description explaining what this scenario tests
  and why it's important.

tags: ["category", "feature", "priority"]  # For filtering and organization

architecture: "x86_64"  # or "aarch64"

# Virtual machines to provision
vms:
  - name: "vm-name"
    uuid: "optional-explicit-uuid"  # Auto-generated if omitted
    macAddress: "optional-mac"      # Auto-generated if omitted
    memory: "1024"                  # MB
    vcpus: 1
    bootOrder: ["network"]
    labels:                         # Optional metadata
      role: "worker"

# Kubernetes resources to create (CRDs, ConfigMaps, Secrets)
resources:
  - kind: "Profile"
    name: "resource-name"
    namespace: "shaper-system"
    yaml: |
      # Full Kubernetes YAML definition
      apiVersion: shaper.amahdha.com/v1alpha1
      kind: Profile
      # ...

# Test assertions to validate
assertions:
  - type: "dhcp_lease"           # Check DHCP lease obtained
    vm: "vm-name"
    description: "What this assertion validates"

  - type: "profile_match"        # Verify correct Profile returned
    vm: "vm-name"
    expected: "profile-name"

# Timeouts for various operations (optional, defaults provided)
timeouts:
  dhcpLease: "30s"
  httpBoot: "120s"
  # ...

# Expected test outcome
expectedOutcome:
  status: "passed"
  description: "What success looks like"
```

## Available Scenarios

### 1. **basic-boot.yaml** - Basic Single VM Boot
**Purpose:** Smoke test for basic PXE boot flow
**Features:** Single VM, default assignment, DHCP/TFTP/HTTP boot
**Complexity:** ⭐ (Beginner)
**Use when:** Testing basic infrastructure or as a starting point

### 2. **assignment-match.yaml** - Assignment Selector Matching
**Purpose:** Validate Assignment subject selectors work correctly
**Features:** Explicit UUID/MAC matching, custom Assignment
**Complexity:** ⭐⭐ (Intermediate)
**Use when:** Testing Assignment matching logic

### 3. **multi-vm.yaml** - Multiple VMs with Different Profiles
**Purpose:** Test parallel VM provisioning with different configurations
**Features:** 2 VMs (worker + control-plane), role-based Assignments
**Complexity:** ⭐⭐⭐ (Advanced)
**Use when:** Testing multi-VM scenarios or role-based configurations

### 4. **profile-selection.yaml** - Profile Selection by Labels
**Purpose:** Validate Profile selection using label selectors
**Features:** Multiple Profiles, label-based selection
**Complexity:** ⭐⭐ (Intermediate)
**Use when:** Testing Profile label selector logic

### 5. **config-retrieval.yaml** - Configuration File Retrieval
**Purpose:** Test /config/{uuid} endpoint and content transformation
**Features:** Ignition config, Butane transformer, content resolution
**Complexity:** ⭐⭐⭐ (Advanced)
**Use when:** Testing config serving and transformation pipeline

## Running Scenarios

### Using Forge (Recommended)

```bash
# List available test environments
forge test e2e list

# Create test environment (infrastructure + kind cluster)
forge test e2e create

# Run a specific scenario
forge test e2e run --scenario basic-boot

# Run all scenarios
forge test e2e run

# Clean up test environment
forge test e2e delete
```

### Manual Execution

```bash
# Set up test environment
cd /home/alexandremahdhaoui/go/src/github.com/alexandremahdhaoui/shaper
export SHAPER_E2E_SCENARIO=test/e2e/scenarios/basic-boot.yaml

# Run E2E test
go test ./test/e2e -v -run TestE2EScenario

# Or use the e2e binary
./bin/shaper-e2e run --scenario test/e2e/scenarios/basic-boot.yaml
```

## Creating New Scenarios

### Step 1: Copy a Template

Start with the closest existing scenario:
- Simple test → Copy `basic-boot.yaml`
- Assignment matching → Copy `assignment-match.yaml`
- Multi-VM → Copy `multi-vm.yaml`
- Config testing → Copy `config-retrieval.yaml`

### Step 2: Modify VM Configuration

```yaml
vms:
  - name: "my-test-vm"
    uuid: "your-uuid-here"  # Generate with: uuidgen
    macAddress: "52:54:00:XX:XX:XX"  # Ensure uniqueness
    memory: "2048"
    vcpus: 2
    bootOrder: ["network"]
```

**Tips:**
- UUIDs must be unique per scenario
- MAC addresses must start with `52:54:00` (QEMU OUI)
- Memory is in MB (1024 = 1GB)
- Use realistic resource values (1-4GB RAM, 1-4 vCPUs for testing)

### Step 3: Define Kubernetes Resources

```yaml
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
          app: "my-app"
      spec:
        ipxe: |
          #!ipxe
          echo My custom iPXE script
          shell
```

**Tips:**
- Always use `namespace: shaper-system` for CRDs
- Use meaningful labels for Profile/Assignment selection
- Test iPXE scripts are small (just echo + shell is fine)
- For additionalContent, use inline content for testing

### Step 4: Add Assertions

```yaml
assertions:
  # Basic connectivity
  - type: "dhcp_lease"
    vm: "my-test-vm"
    description: "VM obtains DHCP lease"

  # Boot flow
  - type: "http_boot_called"
    vm: "my-test-vm"
    description: "VM calls shaper-API"

  # Configuration matching
  - type: "assignment_match"
    vm: "my-test-vm"
    expected: "my-assignment"
    description: "Correct Assignment is matched"

  - type: "profile_match"
    vm: "my-test-vm"
    expected: "my-profile"
    description: "Correct Profile is returned"
```

**Available Assertion Types:**
- `dhcp_lease`: VM obtained DHCP lease
- `tftp_boot`: VM fetched boot file via TFTP
- `http_boot_called`: VM called shaper-API HTTP endpoint
- `assignment_match`: Expected Assignment was matched
- `profile_match`: Expected Profile was returned
- `http_endpoint_accessible`: Specific HTTP endpoint is accessible
- `config_content_match`: Config content matches expected values

### Step 5: Validate and Test

```bash
# Validate YAML syntax
yamllint test/e2e/scenarios/my-scenario.yaml

# Validate scenario structure (if validation test exists)
go test ./pkg/test/e2e/scenario -v -run TestLoadExampleScenarios

# Run the scenario
forge test e2e run --scenario my-scenario
```

## Troubleshooting

### Scenario Fails to Load

**Error:** `failed to parse scenario YAML`

**Solutions:**
- Check YAML syntax with `yamllint`
- Ensure proper indentation (use spaces, not tabs)
- Validate embedded YAML in `resources[].yaml` field

### VM Doesn't Boot

**Error:** `timeout waiting for DHCP lease`

**Check:**
1. Bridge network exists: `ip addr show br-shaper`
2. Dnsmasq is running: `ps aux | grep dnsmasq`
3. VM has network interface: Check libvirt domain XML
4. Firewall rules allow DHCP (UDP 67/68)

**Debug:**
```bash
# Check dnsmasq logs
tail -f /tmp/shaper-e2e-*/dnsmasq.log

# Check VM console output
virsh console <vm-name>
```

### Assignment Not Matched

**Error:** `expected assignment 'X' but got 'Y'` or `default assignment`

**Check:**
1. VM UUID matches Assignment `subjectSelectors.matchLabels.uuid`
2. Assignment `buildArch` matches VM architecture
3. Assignment resource was created before VM boot
4. Check shaper-API logs for assignment selection logic

**Debug:**
```bash
# List all Assignments
kubectl get assignments -n shaper-system

# Describe specific Assignment
kubectl describe assignment <name> -n shaper-system

# Check shaper-API logs
kubectl logs -n shaper-system deployment/shaper-api
```

### Profile Not Found

**Error:** `expected profile 'X' but got 'Y'` or `profile not found`

**Check:**
1. Profile resource exists in scenario `resources`
2. Assignment `profileSelectors` match Profile `labels`
3. Profile was created and ready before VM boot

**Debug:**
```bash
# List all Profiles
kubectl get profiles -n shaper-system

# Describe specific Profile
kubectl describe profile <name> -n shaper-system

# Check if Profile is ready
kubectl get profile <name> -n shaper-system -o yaml
```

### Config Retrieval Fails

**Error:** `failed to retrieve config from /config/{uuid}`

**Check:**
1. Profile has `additionalContent` defined
2. Profile status contains UUID mappings
3. shaper-API is serving /config endpoint
4. Content resolver/transformer configured correctly

**Debug:**
```bash
# Check Profile status for UUIDs
kubectl get profile <name> -n shaper-system -o jsonpath='{.status}'

# Test config endpoint manually
curl http://shaper-api.shaper-system.svc.cluster.local/config/<uuid>

# Check shaper-API logs for content resolution
kubectl logs -n shaper-system deployment/shaper-api | grep -i config
```

### Timeouts

**Error:** `timeout waiting for <operation>`

**Solutions:**
1. Increase timeout in scenario YAML:
   ```yaml
   timeouts:
     vmProvision: "300s"  # Increase from default 180s
     httpBoot: "180s"     # Increase from default 120s
   ```

2. Check resource constraints (CPU, memory) on test host
3. Check network latency (especially for HTTP/TFTP operations)

### Test Environment Issues

**Error:** `failed to create kind cluster` or `bridge already exists`

**Solutions:**
```bash
# Clean up existing test environment
forge test e2e delete

# Manually clean up if needed
kind delete cluster --name shaper-e2e
sudo ip link delete br-shaper
sudo pkill dnsmasq
```

## Best Practices

1. **Start Simple:** Begin with `basic-boot.yaml` and incrementally add complexity
2. **One Feature Per Scenario:** Each scenario should test one specific feature or flow
3. **Use Meaningful Names:** VM names, resource names should indicate their purpose
4. **Add Comments:** Use YAML comments to explain complex configurations
5. **Keep iPXE Scripts Minimal:** For testing, simple echo + shell is sufficient
6. **Explicit UUIDs for Matching:** When testing selectors, use explicit UUIDs/MACs
7. **Descriptive Assertions:** Write clear `description` fields for each assertion
8. **Tag Appropriately:** Use tags to organize scenarios (smoke, regression, feature-specific)
9. **Document Expected Behavior:** Use `expectedOutcome.description` to explain success criteria

## Contributing

When adding new scenarios:

1. Follow the existing naming convention: `feature-name.yaml`
2. Add entry to this README under "Available Scenarios"
3. Ensure scenario validates with `go test ./pkg/test/e2e/scenario`
4. Test scenario runs successfully at least once
5. Include comments in YAML for complex configurations
6. Update assertion types documentation if adding new assertion types

## Further Reading

- [Shaper E2E Framework Architecture](../../../.ai/plan/e2e-framework/architecture.md)
- [Test Reporting Format](../../../.ai/plan/e2e-framework/reporting-format.md)
- [iPXE Boot Flow](../README.md)
- [Shaper CRD Documentation](../../../charts/shaper-crds/README.md)
