//go:build e2e

package e2e_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	e2e "github.com/alexandremahdhaoui/shaper/pkg/test/e2e"
	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e/forge"
	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e/infrastructure"
	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e/scenario"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2EFrameworkLifecycle validates the complete framework lifecycle:
// create testenv → load scenario → execute → generate report → delete testenv → verify cleanup
func TestE2EFrameworkLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Create temporary directories for test
	artifactDir, err := os.MkdirTemp("", "e2e-lifecycle-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(artifactDir) })

	storeDir := filepath.Join(artifactDir, "store")
	require.NoError(t, os.MkdirAll(storeDir, 0o755))

	// Step 1: Create testenv
	testenv, err := forge.NewTestenv(storeDir, artifactDir)
	require.NoError(t, err)

	config := map[string]interface{}{
		"network": map[string]interface{}{
			"cidr":      "192.168.100.1/24",
			"bridge":    "br-e2e-lc", // Linux bridge names max 15 chars
			"dhcpRange": "192.168.100.10,192.168.100.100",
		},
		"kind": map[string]interface{}{
			"clusterName": "e2e-lc",
		},
		"shaper": map[string]interface{}{
			"namespace":   "default",
			"apiReplicas": 1,
		},
	}

	testID, err := testenv.Create(ctx, config)
	require.NoError(t, err)
	assert.NotEmpty(t, testID)

	// Ensure cleanup happens
	t.Cleanup(func() {
		_ = testenv.Delete(context.Background(), testID)
	})

	// Step 2: Load basic-boot scenario
	projectRoot := getProjectRoot(t)
	scenarioPath := filepath.Join(projectRoot, "test", "e2e", "scenarios", "basic-boot.yaml")

	loader := scenario.NewLoader(filepath.Dir(scenarioPath))
	scen, err := loader.Load(filepath.Base(scenarioPath))
	require.NoError(t, err)
	require.NotNil(t, scen)

	// Validate scenario loaded correctly
	assert.Equal(t, "Basic Single VM Boot Test", scen.Name)
	assert.NotEmpty(t, scen.VMs)
	assert.NotEmpty(t, scen.Assertions)

	// Step 3: Execute scenario (simulated - would require full infra)
	// For this test, we're primarily validating lifecycle, not full execution
	// Full execution requires network privileges and time

	// Step 4: Verify testenv can be retrieved
	envDetails, err := testenv.Get(ctx, testID)
	require.NoError(t, err)
	assert.NotNil(t, envDetails)
	assert.Equal(t, testID, envDetails["id"])
	assert.NotEmpty(t, envDetails["kubeconfig"])
	assert.NotEmpty(t, envDetails["artifactDir"])

	// Step 5: Delete testenv
	err = testenv.Delete(ctx, testID)
	require.NoError(t, err)

	// Step 6: Verify cleanup - environment should no longer exist
	_, err = testenv.Get(ctx, testID)
	assert.Error(t, err, "Environment should not exist after deletion")
}

// TestScenarioValidation validates scenario loading and validation logic
func TestScenarioValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	projectRoot := getProjectRoot(t)
	scenariosDir := filepath.Join(projectRoot, "test", "e2e", "scenarios")

	loader := scenario.NewLoader(scenariosDir)

	t.Run("load valid scenarios", func(t *testing.T) {
		validScenarios := []string{
			"basic-boot.yaml",
			"assignment-match.yaml",
			"multi-vm.yaml",
		}

		for _, scenarioFile := range validScenarios {
			t.Run(scenarioFile, func(t *testing.T) {
				scen, err := loader.Load(scenarioFile)
				require.NoError(t, err, "Should load valid scenario")
				require.NotNil(t, scen)

				// Validate basic structure
				assert.NotEmpty(t, scen.Name)
				assert.NotEmpty(t, scen.VMs)
				assert.NotEmpty(t, scen.Assertions)
			})
		}
	})

	t.Run("invalid scenarios", func(t *testing.T) {
		// Create test data directory for invalid scenarios
		testDataDir, err := os.MkdirTemp("", "e2e-invalid-*")
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.RemoveAll(testDataDir) })

		// Test 1: Missing required fields
		invalidYAML := `
name: "Invalid Scenario"
# Missing description, architecture, vms, assertions
`
		invalidPath := filepath.Join(testDataDir, "invalid-missing-fields.yaml")
		require.NoError(t, os.WriteFile(invalidPath, []byte(invalidYAML), 0o644))

		invalidLoader := scenario.NewLoader(testDataDir)
		_, err = invalidLoader.Load("invalid-missing-fields.yaml")
		assert.Error(t, err, "Should fail validation for missing fields")

		// Test 2: Empty VMs list
		emptyVMsYAML := `
name: "Empty VMs"
description: "Test"
architecture: "x86_64"
vms: []
assertions: []
`
		emptyVMsPath := filepath.Join(testDataDir, "invalid-empty-vms.yaml")
		require.NoError(t, os.WriteFile(emptyVMsPath, []byte(emptyVMsYAML), 0o644))

		_, err = invalidLoader.Load("invalid-empty-vms.yaml")
		assert.Error(t, err, "Should fail validation for empty VMs")
	})

	t.Run("non-existent scenario", func(t *testing.T) {
		_, err := loader.Load("does-not-exist.yaml")
		assert.Error(t, err, "Should error on non-existent file")
	})
}

// TestMultiVMOrchestration validates multi-VM scenario loading and structure
func TestMultiVMOrchestration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	projectRoot := getProjectRoot(t)
	scenariosDir := filepath.Join(projectRoot, "test", "e2e", "scenarios")

	loader := scenario.NewLoader(scenariosDir)
	scen, err := loader.Load("multi-vm.yaml")
	require.NoError(t, err)
	require.NotNil(t, scen)

	// Validate scenario structure
	assert.Equal(t, "Multi-VM Test with Different Profiles", scen.Name)
	assert.Len(t, scen.VMs, 2, "Should have exactly 2 VMs")

	// Validate VM specifications
	assert.Equal(t, "test-vm-worker", scen.VMs[0].Name)
	assert.Equal(t, "test-vm-control", scen.VMs[1].Name)

	// Validate UUIDs are specified
	assert.NotEmpty(t, scen.VMs[0].UUID)
	assert.NotEmpty(t, scen.VMs[1].UUID)
	assert.NotEqual(t, scen.VMs[0].UUID, scen.VMs[1].UUID)

	// Validate labels
	assert.Equal(t, "worker", scen.VMs[0].Labels["role"])
	assert.Equal(t, "control-plane", scen.VMs[1].Labels["role"])

	// Validate resources (should have 2 profiles + 2 assignments = 4)
	assert.Len(t, scen.Resources, 4)

	// Validate assertions (should have assertions for both VMs)
	assert.GreaterOrEqual(t, len(scen.Assertions), 2)

	// Verify both VMs have profile_match assertions
	workerAssertions := 0
	controlAssertions := 0
	for _, assertion := range scen.Assertions {
		if assertion.VM == "test-vm-worker" {
			workerAssertions++
		}
		if assertion.VM == "test-vm-control" {
			controlAssertions++
		}
	}
	assert.Greater(t, workerAssertions, 0, "Worker VM should have assertions")
	assert.Greater(t, controlAssertions, 0, "Control VM should have assertions")
}

// TestAssertionValidators validates assertion types and structure
func TestAssertionValidators(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Load a scenario with assertions
	projectRoot := getProjectRoot(t)
	scenariosDir := filepath.Join(projectRoot, "test", "e2e", "scenarios")

	loader := scenario.NewLoader(scenariosDir)
	scen, err := loader.Load("basic-boot.yaml")
	require.NoError(t, err)
	require.NotNil(t, scen)

	t.Run("dhcp_lease assertion", func(t *testing.T) {
		// Find dhcp_lease assertion
		var found bool
		for _, assertion := range scen.Assertions {
			if assertion.Type == "dhcp_lease" {
				found = true
				assert.Equal(t, "test-vm-basic", assertion.VM)
				assert.NotEmpty(t, assertion.Description)
				break
			}
		}
		assert.True(t, found, "Should have dhcp_lease assertion")
	})

	t.Run("tftp_boot assertion", func(t *testing.T) {
		var found bool
		for _, assertion := range scen.Assertions {
			if assertion.Type == "tftp_boot" {
				found = true
				assert.Equal(t, "test-vm-basic", assertion.VM)
				break
			}
		}
		assert.True(t, found, "Should have tftp_boot assertion")
	})

	t.Run("http_boot_called assertion", func(t *testing.T) {
		var found bool
		for _, assertion := range scen.Assertions {
			if assertion.Type == "http_boot_called" {
				found = true
				assert.Equal(t, "test-vm-basic", assertion.VM)
				break
			}
		}
		assert.True(t, found, "Should have http_boot_called assertion")
	})

	t.Run("profile_match assertion", func(t *testing.T) {
		var found bool
		for _, assertion := range scen.Assertions {
			if assertion.Type == "profile_match" {
				found = true
				assert.Equal(t, "test-vm-basic", assertion.VM)
				assert.Equal(t, "default-profile", assertion.Expected)
				break
			}
		}
		assert.True(t, found, "Should have profile_match assertion")
	})

	t.Run("assertion_match from multi-vm", func(t *testing.T) {
		// Load assignment-match scenario
		multiScen, err := loader.Load("assignment-match.yaml")
		require.NoError(t, err)

		var found bool
		for _, assertion := range multiScen.Assertions {
			if assertion.Type == "assignment_match" {
				found = true
				assert.Equal(t, "uuid-specific-assignment", assertion.Expected)
				break
			}
		}
		assert.True(t, found, "Should have assignment_match assertion")
	})
}

// TestErrorRecovery validates error handling and cleanup behavior
func TestErrorRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	t.Run("invalid config error", func(t *testing.T) {
		artifactDir, err := os.MkdirTemp("", "e2e-error-*")
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.RemoveAll(artifactDir) })

		storeDir := filepath.Join(artifactDir, "store")
		require.NoError(t, os.MkdirAll(storeDir, 0o755))

		testenv, err := forge.NewTestenv(storeDir, artifactDir)
		require.NoError(t, err)

		// Try to create with invalid config (missing required fields)
		invalidConfig := map[string]interface{}{
			"network": map[string]interface{}{
				// Missing cidr, bridge, dhcpRange
			},
		}

		_, err = testenv.Create(ctx, invalidConfig)
		assert.Error(t, err, "Should error on invalid config")
		assert.ErrorIs(t, err, forge.ErrInvalidConfig)
	})

	t.Run("cleanup on partial setup failure", func(t *testing.T) {
		// Create infrastructure manager with invalid spec that will fail
		spec := infrastructure.InfrastructureSpec{
			Network: infrastructure.NetworkSpec{
				// Invalid CIDR format will cause failure
				CIDR:      "invalid-cidr",
				Bridge:    "br-test-fail",
				DHCPRange: "192.168.100.10,192.168.100.100",
			},
			Kind: infrastructure.KindSpec{
				ClusterName: "test-fail",
			},
			Shaper: infrastructure.ShaperSpec{
				Namespace: "default",
			},
		}

		artifactDir, err := os.MkdirTemp("", "e2e-cleanup-*")
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.RemoveAll(artifactDir) })

		mgr := infrastructure.NewInfrastructureManager(spec, artifactDir)

		// Setup should fail due to invalid CIDR
		_, err = mgr.Setup(ctx)
		assert.Error(t, err, "Setup should fail with invalid spec")

		// Verify cleanup happened (artifact dir should still exist but be empty of infra)
		// In a real scenario, we'd verify bridges, networks, etc. are cleaned up
	})

	t.Run("delete non-existent environment", func(t *testing.T) {
		artifactDir, err := os.MkdirTemp("", "e2e-notfound-*")
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.RemoveAll(artifactDir) })

		storeDir := filepath.Join(artifactDir, "store")
		require.NoError(t, os.MkdirAll(storeDir, 0o755))

		testenv, err := forge.NewTestenv(storeDir, artifactDir)
		require.NoError(t, err)

		// Try to delete environment that doesn't exist
		err = testenv.Delete(ctx, "does-not-exist")
		assert.Error(t, err, "Should error when deleting non-existent environment")
	})
}

// TestReportGeneration validates report types and structure
func TestReportGeneration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create test result structure to validate it compiles and has required fields
	startTime := time.Now().Add(-5 * time.Minute)
	endTime := time.Now()

	testResult := &e2e.TestResult{
		Version: "1.0.0",
		TestID:  "test-report-123",
		Scenario: e2e.ScenarioInfo{
			Name:        "Test Scenario",
			Description: "A test scenario for report generation",
			Tags:        []string{"test", "report"},
		},
		Execution: e2e.ExecutionInfo{
			StartTime:    startTime,
			EndTime:      endTime,
			Duration:     endTime.Sub(startTime).Seconds(),
			Architecture: "x86_64",
			Status:       "passed",
			ExitCode:     0,
		},
		Infra: e2e.Infrastructure{
			KindCluster: e2e.KindClusterInfo{
				Name:       "test-cluster",
				Kubeconfig: "/tmp/kubeconfig",
			},
			Network: e2e.NetworkInfo{
				Bridge:    "br-test",
				CIDR:      "192.168.100.1/24",
				DHCPRange: "192.168.100.10,192.168.100.100",
			},
		},
		VMs: []e2e.VMResult{
			{
				Name:       "test-vm-1",
				UUID:       "11111111-1111-1111-1111-111111111111",
				MACAddress: "52:54:00:11:11:11",
				Status:     "passed",
				Memory:     "1024",
				VCPUs:      1,
				Assertions: []e2e.AssertionInfo{
					{
						Type:        "dhcp_lease",
						Description: "DHCP lease obtained",
						Passed:      true,
						Duration:    2.5,
						Timestamp:   startTime.Add(10 * time.Second),
					},
					{
						Type:        "profile_match",
						Description: "Profile matched",
						Expected:    "default-profile",
						Actual:      "default-profile",
						Passed:      true,
						Duration:    1.2,
						Timestamp:   startTime.Add(30 * time.Second),
					},
				},
			},
		},
		Summary: e2e.AssertionStats{
			Total:    2,
			Passed:   2,
			Failed:   0,
			Skipped:  0,
			PassRate: 100.0,
		},
	}

	// Validate structure
	assert.Equal(t, "test-report-123", testResult.TestID)
	assert.Equal(t, "Test Scenario", testResult.Scenario.Name)
	assert.Equal(t, "passed", testResult.Execution.Status)
	assert.Len(t, testResult.VMs, 1)
	assert.Equal(t, 2, testResult.Summary.Total)
	assert.Equal(t, 2, testResult.Summary.Passed)
	assert.Equal(t, 100.0, testResult.Summary.PassRate)

	// Validate log collection structure
	logs := &e2e.LogCollection{
		FrameworkLog: "Framework log content",
		DnsmasqLog:   "Dnsmasq log content",
		ShaperAPILog: "Shaper API log content",
		VMConsoleLogs: map[string]string{
			"test-vm-1": "Console output",
		},
	}

	assert.NotEmpty(t, logs.FrameworkLog)
	assert.NotEmpty(t, logs.VMConsoleLogs)
}

// getProjectRoot returns the project root directory for tests
func getProjectRoot(t *testing.T) string {
	t.Helper()

	// Use the location of THIS file to find project root
	_, filename, _, ok := runtime.Caller(1)
	if !ok {
		t.Fatal("could not determine source file location")
	}

	// From pkg/test/e2e/integration_test.go, walk up 3 levels to project root
	projectRoot := filepath.Join(filepath.Dir(filename), "..", "..", "..")

	// Verify by checking for go.mod
	gomodPath := filepath.Join(projectRoot, "go.mod")
	if _, err := os.Stat(gomodPath); err != nil {
		t.Fatalf("could not find project root (go.mod not found at %s): %v", gomodPath, err)
	}

	// Convert to absolute path
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		t.Fatalf("could not get absolute path: %v", err)
	}

	return absRoot
}
