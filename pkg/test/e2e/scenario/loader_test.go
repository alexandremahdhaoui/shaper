package scenario

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoader_Load_ValidScenario(t *testing.T) {
	// Create a temporary directory for test scenarios
	tmpDir := t.TempDir()

	// Create a valid scenario YAML
	validScenario := `
name: "Test Scenario"
description: "A test scenario for unit testing"
architecture: "x86_64"

vms:
  - name: "test-vm"
    memory: "1024"
    vcpus: 1

assertions:
  - type: "dhcp_lease"
    vm: "test-vm"
    description: "VM should obtain DHCP lease"
`

	scenarioPath := filepath.Join(tmpDir, "test-scenario.yaml")
	err := os.WriteFile(scenarioPath, []byte(validScenario), 0o644)
	require.NoError(t, err)

	// Test loading with absolute path
	loader := NewLoader("")
	scenario, err := loader.Load(scenarioPath)
	require.NoError(t, err)
	require.NotNil(t, scenario)

	assert.Equal(t, "Test Scenario", scenario.Name)
	assert.Equal(t, "A test scenario for unit testing", scenario.Description)
	assert.Equal(t, "x86_64", scenario.Architecture)
	assert.Len(t, scenario.VMs, 1)
	assert.Equal(t, "test-vm", scenario.VMs[0].Name)
	assert.Len(t, scenario.Assertions, 1)
	assert.Equal(t, "dhcp_lease", scenario.Assertions[0].Type)
}

func TestLoader_Load_RelativePath(t *testing.T) {
	// Create a temporary directory for test scenarios
	tmpDir := t.TempDir()

	validScenario := `
name: "Relative Path Test"
description: "Tests relative path resolution"
architecture: "x86_64"

vms:
  - name: "test-vm"

assertions:
  - type: "dhcp_lease"
    vm: "test-vm"
`

	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	err := os.WriteFile(scenarioPath, []byte(validScenario), 0o644)
	require.NoError(t, err)

	// Test loading with relative path
	loader := NewLoader(tmpDir)
	scenario, err := loader.Load("scenario.yaml")
	require.NoError(t, err)
	require.NotNil(t, scenario)

	assert.Equal(t, "Relative Path Test", scenario.Name)
}

func TestLoader_Load_FileNotFound(t *testing.T) {
	loader := NewLoader("")
	scenario, err := loader.Load("nonexistent-scenario.yaml")
	assert.Error(t, err)
	assert.Nil(t, scenario)
	assert.Contains(t, err.Error(), "scenario file does not exist")
}

func TestLoader_Load_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()

	invalidYAML := `
name: "Invalid YAML"
description: [this is not valid YAML syntax
`

	scenarioPath := filepath.Join(tmpDir, "invalid.yaml")
	err := os.WriteFile(scenarioPath, []byte(invalidYAML), 0o644)
	require.NoError(t, err)

	loader := NewLoader("")
	scenario, err := loader.Load(scenarioPath)
	assert.Error(t, err)
	assert.Nil(t, scenario)
	assert.Contains(t, err.Error(), "failed to parse YAML")
}

func TestLoader_Load_ValidationFailure(t *testing.T) {
	tmpDir := t.TempDir()

	// Missing required fields
	invalidScenario := `
name: "Invalid Scenario"
# Missing description and architecture
vms:
  - name: "test-vm"
`

	scenarioPath := filepath.Join(tmpDir, "validation-fail.yaml")
	err := os.WriteFile(scenarioPath, []byte(invalidScenario), 0o644)
	require.NoError(t, err)

	loader := NewLoader("")
	scenario, err := loader.Load(scenarioPath)
	assert.Error(t, err)
	assert.Nil(t, scenario)
	assert.Contains(t, err.Error(), "scenario validation failed")
}

func TestLoader_LoadMultiple(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple scenario files
	scenarios := []struct {
		filename string
		content  string
		valid    bool
	}{
		{
			filename: "scenario1.yaml",
			content: `
name: "Scenario 1"
description: "First scenario"
architecture: "x86_64"
vms:
  - name: "vm1"
assertions:
  - type: "dhcp_lease"
    vm: "vm1"
`,
			valid: true,
		},
		{
			filename: "scenario2.yaml",
			content: `
name: "Scenario 2"
description: "Second scenario"
architecture: "aarch64"
vms:
  - name: "vm2"
assertions:
  - type: "tftp_boot"
    vm: "vm2"
`,
			valid: true,
		},
		{
			filename: "invalid.yaml",
			content: `
name: "Invalid"
# Missing required fields
`,
			valid: false,
		},
	}

	var paths []string
	for _, s := range scenarios {
		path := filepath.Join(tmpDir, s.filename)
		err := os.WriteFile(path, []byte(s.content), 0o644)
		require.NoError(t, err)
		paths = append(paths, path)
	}

	loader := NewLoader("")
	loadedScenarios, errs := loader.LoadMultiple(paths)

	// Should load 2 valid scenarios and have 1 error
	assert.Len(t, loadedScenarios, 2)
	assert.Len(t, errs, 1)

	assert.Equal(t, "Scenario 1", loadedScenarios[0].Name)
	assert.Equal(t, "Scenario 2", loadedScenarios[1].Name)
}

func TestLoader_ComplexScenario(t *testing.T) {
	tmpDir := t.TempDir()

	// Complex scenario with all optional fields
	complexScenario := `
name: "Complex E2E Test"
description: "A comprehensive test scenario with all features"
tags: ["complex", "integration", "multi-vm"]
architecture: "x86_64"

infrastructure:
  network:
    cidr: "192.168.100.0/24"
    bridge: "br-test"
    dhcpRange: "192.168.100.100,192.168.100.200"
  kind:
    clusterName: "test-cluster"
    version: "v1.30.0"
  shaper:
    namespace: "shaper-system"
    apiReplicas: 2

vms:
  - name: "vm1"
    uuid: "550e8400-e29b-41d4-a716-446655440000"
    macAddress: "52:54:00:12:34:56"
    memory: "2048"
    vcpus: 2
    bootOrder: ["network", "hd"]
    labels:
      env: "test"
      role: "worker"
  - name: "vm2"
    memory: "1024"
    vcpus: 1

resources:
  - kind: "Profile"
    name: "test-profile"
    namespace: "shaper-system"
    yaml: |
      apiVersion: shaper.amahdha.com/v1alpha1
      kind: Profile
      metadata:
        name: test-profile
  - kind: "Assignment"
    name: "test-assignment"
    yaml: |
      apiVersion: shaper.amahdha.com/v1alpha1
      kind: Assignment
      metadata:
        name: test-assignment

assertions:
  - type: "dhcp_lease"
    vm: "vm1"
    description: "VM1 should get DHCP lease"
  - type: "profile_match"
    vm: "vm1"
    expected: "test-profile"
    description: "VM1 should match test profile"
  - type: "http_boot_called"
    vm: "vm2"

timeouts:
  dhcpLease: "30s"
  tftpBoot: "60s"
  httpBoot: "120s"
  vmProvision: "180s"
  resourceReady: "60s"
  assertionPoll: "2s"

expectedOutcome:
  status: "passed"
  description: "All VMs boot successfully"
`

	scenarioPath := filepath.Join(tmpDir, "complex.yaml")
	err := os.WriteFile(scenarioPath, []byte(complexScenario), 0o644)
	require.NoError(t, err)

	loader := NewLoader("")
	scenario, err := loader.Load(scenarioPath)
	require.NoError(t, err)
	require.NotNil(t, scenario)

	// Validate top-level fields
	assert.Equal(t, "Complex E2E Test", scenario.Name)
	assert.Equal(t, "A comprehensive test scenario with all features", scenario.Description)
	assert.Equal(t, []string{"complex", "integration", "multi-vm"}, scenario.Tags)
	assert.Equal(t, "x86_64", scenario.Architecture)

	// Validate infrastructure
	assert.Equal(t, "192.168.100.0/24", scenario.Infrastructure.Network.CIDR)
	assert.Equal(t, "br-test", scenario.Infrastructure.Network.Bridge)
	assert.Equal(t, "192.168.100.100,192.168.100.200", scenario.Infrastructure.Network.DHCPRange)
	assert.Equal(t, "test-cluster", scenario.Infrastructure.Kind.ClusterName)
	assert.Equal(t, "v1.30.0", scenario.Infrastructure.Kind.Version)
	assert.Equal(t, "shaper-system", scenario.Infrastructure.Shaper.Namespace)
	assert.Equal(t, 2, scenario.Infrastructure.Shaper.APIReplicas)

	// Validate VMs
	assert.Len(t, scenario.VMs, 2)
	assert.Equal(t, "vm1", scenario.VMs[0].Name)
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", scenario.VMs[0].UUID)
	assert.Equal(t, "52:54:00:12:34:56", scenario.VMs[0].MACAddress)
	assert.Equal(t, "2048", scenario.VMs[0].Memory)
	assert.Equal(t, 2, scenario.VMs[0].VCPUs)
	assert.Equal(t, []string{"network", "hd"}, scenario.VMs[0].BootOrder)
	assert.Equal(t, "test", scenario.VMs[0].Labels["env"])
	assert.Equal(t, "worker", scenario.VMs[0].Labels["role"])

	// Validate resources
	assert.Len(t, scenario.Resources, 2)
	assert.Equal(t, "Profile", scenario.Resources[0].Kind)
	assert.Equal(t, "test-profile", scenario.Resources[0].Name)
	assert.Equal(t, "shaper-system", scenario.Resources[0].Namespace)

	// Validate assertions
	assert.Len(t, scenario.Assertions, 3)
	assert.Equal(t, "dhcp_lease", scenario.Assertions[0].Type)
	assert.Equal(t, "vm1", scenario.Assertions[0].VM)
	assert.Equal(t, "profile_match", scenario.Assertions[1].Type)
	assert.Equal(t, "test-profile", scenario.Assertions[1].Expected)

	// Validate timeouts
	duration, err := scenario.Timeouts.DHCPLease.Duration()
	require.NoError(t, err)
	assert.Equal(t, "30s", duration.String())

	// Validate expected outcome
	assert.NotNil(t, scenario.ExpectedOutcome)
	assert.Equal(t, "passed", scenario.ExpectedOutcome.Status)
}

func TestNewLoader_EmptyBasePath(t *testing.T) {
	loader := NewLoader("")
	assert.Equal(t, ".", loader.basePath)
}

func TestNewLoader_CustomBasePath(t *testing.T) {
	loader := NewLoader("/custom/path")
	assert.Equal(t, "/custom/path", loader.basePath)
}

func TestDefaultScenarioPath(t *testing.T) {
	path := DefaultScenarioPath()
	assert.Equal(t, "test/e2e/scenarios", path)
}

func TestDurationString_Duration(t *testing.T) {
	tests := []struct {
		name        string
		duration    DurationString
		expected    string
		expectError bool
	}{
		{
			name:        "valid duration 30s",
			duration:    "30s",
			expected:    "30s",
			expectError: false,
		},
		{
			name:        "valid duration 1m",
			duration:    "1m",
			expected:    "1m0s",
			expectError: false,
		},
		{
			name:        "valid duration 2h30m",
			duration:    "2h30m",
			expected:    "2h30m0s",
			expectError: false,
		},
		{
			name:        "empty duration",
			duration:    "",
			expected:    "0s",
			expectError: false,
		},
		{
			name:        "invalid duration",
			duration:    "invalid",
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := tt.duration.Duration()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, d.String())
			}
		})
	}
}

func TestLoadExampleScenarios(t *testing.T) {
	// Get the project root (4 levels up from this file)
	projectRoot := filepath.Join("..", "..", "..", "..")
	scenariosPath := filepath.Join(projectRoot, "test", "e2e", "scenarios")

	// Example scenarios that should exist
	exampleScenarios := []string{
		"basic-boot.yaml",
		"assignment-match.yaml",
		"multi-vm.yaml",
		"profile-selection.yaml",
		"config-retrieval.yaml",
	}

	loader := NewLoader(scenariosPath)

	for _, scenarioFile := range exampleScenarios {
		t.Run(scenarioFile, func(t *testing.T) {
			scenario, err := loader.Load(scenarioFile)
			require.NoError(t, err, "Failed to load %s", scenarioFile)
			require.NotNil(t, scenario, "Scenario should not be nil for %s", scenarioFile)

			// Validate basic fields are present
			assert.NotEmpty(t, scenario.Name, "Scenario name should not be empty")
			assert.NotEmpty(t, scenario.Description, "Scenario description should not be empty")
			assert.NotEmpty(t, scenario.Architecture, "Scenario architecture should not be empty")
			assert.NotEmpty(t, scenario.VMs, "Scenario should have at least one VM")
			assert.NotEmpty(t, scenario.Assertions, "Scenario should have at least one assertion")
		})
	}

	// Test loading all scenarios at once
	t.Run("LoadAllExampleScenarios", func(t *testing.T) {
		var paths []string
		paths = append(paths, exampleScenarios...)

		scenarios, errs := loader.LoadMultiple(paths)
		assert.Empty(t, errs, "Should load all example scenarios without errors")
		assert.Len(t, scenarios, len(exampleScenarios), "Should load all %d example scenarios", len(exampleScenarios))
	})
}
