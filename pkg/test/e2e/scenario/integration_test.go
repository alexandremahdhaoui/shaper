//go:build e2e

package scenario

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadRealScenarios tests loading the actual scenario YAML files from test/e2e/scenarios
func TestLoadRealScenarios(t *testing.T) {
	// Get the project root (assuming we're in pkg/test/e2e/scenario)
	projectRoot := filepath.Join("..", "..", "..", "..")
	scenariosDir := filepath.Join(projectRoot, "test", "e2e", "scenarios")

	scenarios := []struct {
		name     string
		filename string
	}{
		{
			name:     "basic boot scenario",
			filename: "basic-boot.yaml",
		},
		{
			name:     "assignment match scenario",
			filename: "assignment-match.yaml",
		},
		{
			name:     "multi-vm scenario",
			filename: "multi-vm.yaml",
		},
	}

	loader := NewLoader(scenariosDir)

	for _, tc := range scenarios {
		t.Run(tc.name, func(t *testing.T) {
			scenario, err := loader.Load(tc.filename)
			require.NoError(t, err, "Failed to load %s", tc.filename)
			require.NotNil(t, scenario)

			// Validate basic structure
			assert.NotEmpty(t, scenario.Name, "Scenario name should not be empty")
			assert.NotEmpty(t, scenario.Description, "Scenario description should not be empty")
			assert.NotEmpty(t, scenario.Architecture, "Architecture should not be empty")
			assert.NotEmpty(t, scenario.VMs, "VMs list should not be empty")
			assert.NotEmpty(t, scenario.Assertions, "Assertions list should not be empty")
		})
	}
}

// TestLoadBasicBootScenario validates the basic-boot.yaml scenario in detail
func TestLoadBasicBootScenario(t *testing.T) {
	projectRoot := filepath.Join("..", "..", "..", "..")
	scenariosDir := filepath.Join(projectRoot, "test", "e2e", "scenarios")

	loader := NewLoader(scenariosDir)
	scenario, err := loader.Load("basic-boot.yaml")
	require.NoError(t, err)
	require.NotNil(t, scenario)

	// Validate top-level fields
	assert.Equal(t, "Basic Single VM Boot Test", scenario.Name)
	assert.Contains(t, scenario.Description, "Validates basic PXE boot flow")
	assert.Equal(t, "x86_64", scenario.Architecture)
	assert.Equal(t, []string{"basic", "smoke", "boot"}, scenario.Tags)

	// Validate VMs
	assert.Len(t, scenario.VMs, 1)
	assert.Equal(t, "test-vm-basic", scenario.VMs[0].Name)
	assert.Equal(t, "1024", scenario.VMs[0].Memory)
	assert.Equal(t, 1, scenario.VMs[0].VCPUs)
	assert.Equal(t, []string{"network"}, scenario.VMs[0].BootOrder)

	// Validate resources
	assert.Len(t, scenario.Resources, 2)
	assert.Equal(t, "Profile", scenario.Resources[0].Kind)
	assert.Equal(t, "default-profile", scenario.Resources[0].Name)
	assert.Equal(t, "Assignment", scenario.Resources[1].Kind)
	assert.Equal(t, "default-assignment", scenario.Resources[1].Name)

	// Validate assertions
	assert.Len(t, scenario.Assertions, 4)
	assert.Equal(t, "dhcp_lease", scenario.Assertions[0].Type)
	assert.Equal(t, "tftp_boot", scenario.Assertions[1].Type)
	assert.Equal(t, "http_boot_called", scenario.Assertions[2].Type)
	assert.Equal(t, "profile_match", scenario.Assertions[3].Type)
	assert.Equal(t, "default-profile", scenario.Assertions[3].Expected)

	// Validate timeouts
	dhcpTimeout, err := scenario.Timeouts.DHCPLease.Duration()
	require.NoError(t, err)
	assert.Equal(t, "30s", dhcpTimeout.String())

	// Validate expected outcome
	require.NotNil(t, scenario.ExpectedOutcome)
	assert.Equal(t, "passed", scenario.ExpectedOutcome.Status)
}

// TestLoadAssignmentMatchScenario validates the assignment-match.yaml scenario in detail
func TestLoadAssignmentMatchScenario(t *testing.T) {
	projectRoot := filepath.Join("..", "..", "..", "..")
	scenariosDir := filepath.Join(projectRoot, "test", "e2e", "scenarios")

	loader := NewLoader(scenariosDir)
	scenario, err := loader.Load("assignment-match.yaml")
	require.NoError(t, err)
	require.NotNil(t, scenario)

	// Validate top-level fields
	assert.Equal(t, "Assignment Selector Matching Test", scenario.Name)
	assert.Equal(t, "x86_64", scenario.Architecture)

	// Validate VM with explicit UUID
	assert.Len(t, scenario.VMs, 1)
	assert.Equal(t, "test-vm-uuid-match", scenario.VMs[0].Name)
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", scenario.VMs[0].UUID)
	assert.Equal(t, "52:54:00:12:34:56", scenario.VMs[0].MACAddress)

	// Validate assertions include assignment_match
	var hasAssignmentMatch bool
	for _, assertion := range scenario.Assertions {
		if assertion.Type == "assignment_match" {
			hasAssignmentMatch = true
			assert.Equal(t, "uuid-specific-assignment", assertion.Expected)
		}
	}
	assert.True(t, hasAssignmentMatch, "Should have assignment_match assertion")
}

// TestLoadMultiVMScenario validates the multi-vm.yaml scenario in detail
func TestLoadMultiVMScenario(t *testing.T) {
	projectRoot := filepath.Join("..", "..", "..", "..")
	scenariosDir := filepath.Join(projectRoot, "test", "e2e", "scenarios")

	loader := NewLoader(scenariosDir)
	scenario, err := loader.Load("multi-vm.yaml")
	require.NoError(t, err)
	require.NotNil(t, scenario)

	// Validate top-level fields
	assert.Equal(t, "Multi-VM Test with Different Profiles", scenario.Name)
	assert.Equal(t, []string{"multi-vm", "parallel", "roles"}, scenario.Tags)

	// Validate multiple VMs
	assert.Len(t, scenario.VMs, 2)

	// Worker VM
	assert.Equal(t, "test-vm-worker", scenario.VMs[0].Name)
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", scenario.VMs[0].UUID)
	assert.Equal(t, "2048", scenario.VMs[0].Memory)
	assert.Equal(t, 2, scenario.VMs[0].VCPUs)
	assert.Equal(t, "worker", scenario.VMs[0].Labels["role"])

	// Control plane VM
	assert.Equal(t, "test-vm-control", scenario.VMs[1].Name)
	assert.Equal(t, "22222222-2222-2222-2222-222222222222", scenario.VMs[1].UUID)
	assert.Equal(t, "4096", scenario.VMs[1].Memory)
	assert.Equal(t, 4, scenario.VMs[1].VCPUs)
	assert.Equal(t, "control-plane", scenario.VMs[1].Labels["role"])

	// Validate resources (2 Profiles + 2 Assignments = 4 total)
	assert.Len(t, scenario.Resources, 4)

	// Validate assertions (4 assertions per VM = 8 total)
	assert.Len(t, scenario.Assertions, 8)

	// Verify both VMs have profile_match assertions
	var workerProfileMatch, controlProfileMatch bool
	for _, assertion := range scenario.Assertions {
		if assertion.Type == "profile_match" {
			if assertion.VM == "test-vm-worker" {
				workerProfileMatch = true
				assert.Equal(t, "worker-profile", assertion.Expected)
			}
			if assertion.VM == "test-vm-control" {
				controlProfileMatch = true
				assert.Equal(t, "control-plane-profile", assertion.Expected)
			}
		}
	}
	assert.True(t, workerProfileMatch, "Worker VM should have profile_match assertion")
	assert.True(t, controlProfileMatch, "Control VM should have profile_match assertion")
}
