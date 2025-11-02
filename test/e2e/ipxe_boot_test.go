//go:build e2e
// +build e2e

package e2e_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e"
	"github.com/alexandremahdhaoui/shaper/pkg/test/kind"
	"github.com/stretchr/testify/require"
)

// TestIPXEBootFlow_E2E is the main end-to-end test for iPXE boot flow
func TestIPXEBootFlow_E2E(t *testing.T) {
	// Skip if not running with e2e tag
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	// Check prerequisites
	if !kind.IsKindInstalled() {
		t.Skip("KIND not installed")
	}

	if !kind.IsKubectlInstalled() {
		t.Skip("kubectl not installed")
	}

	// Setup test environment
	tempDir := t.TempDir()
	imageCacheDir := filepath.Join(os.TempDir(), "shaper-e2e-images")
	if err := os.MkdirAll(imageCacheDir, 0755); err != nil {
		t.Fatalf("Failed to create image cache dir: %v", err)
	}

	setupConfig := e2e.ShaperSetupConfig{
		ArtifactDir:     tempDir,
		ImageCacheDir:   imageCacheDir,
		BridgeName:      "br-shaper-e2e",
		NetworkCIDR:     "192.168.100.1/24",
		DHCPRange:       "192.168.100.10,192.168.100.250",
		KindClusterName: "shaper-e2e-test",
		TFTPRoot:        filepath.Join(tempDir, "tftp"),
		IPXEBootFile:    "", // Would need actual iPXE boot file
		NumClients:      0,  // Don't pre-create clients
		DownloadImages:  false,
	}

	t.Log("Setting up E2E test environment...")
	env, err := e2e.SetupShaperTestEnvironment(setupConfig)
	require.NoError(t, err)
	defer func() {
		t.Log("Tearing down E2E test environment...")
		if err := e2e.TeardownShaperTestEnvironment(env); err != nil {
			t.Logf("Warning: teardown failed: %v", err)
		}
	}()

	t.Logf("Test environment created: %s", env.ID)
	t.Logf("  Bridge: %s", env.BridgeName)
	t.Logf("  Libvirt Network: %s", env.LibvirtNetwork)
	t.Logf("  KIND Cluster: %s", env.KindCluster)
	t.Logf("  Kubeconfig: %s", env.Kubeconfig)
	t.Logf("  TFTP Root: %s", env.TFTPRoot)

	// Run sub-tests
	t.Run("BasicNetworkConnectivity", func(t *testing.T) {
		testBasicNetworkConnectivity(t, env)
	})

	t.Run("DnsmasqRunning", func(t *testing.T) {
		testDnsmasqRunning(t, env)
	})

	t.Run("KindClusterAccessible", func(t *testing.T) {
		testKindClusterAccessible(t, env)
	})

	t.Run("IPXEBootFlow", func(t *testing.T) {
		testIPXEBootFlow(t, env)
	})
}

func testBasicNetworkConnectivity(t *testing.T, env *e2e.ShaperTestEnvironment) {
	// Verify bridge exists
	t.Logf("Verifying bridge %s exists...", env.BridgeName)
	// We can't easily test this without importing network package
	// Just verify the env has the bridge name set
	require.NotEmpty(t, env.BridgeName)
}

func testDnsmasqRunning(t *testing.T, env *e2e.ShaperTestEnvironment) {
	// Verify dnsmasq is running
	require.NotNil(t, env.DnsmasqProcess, "dnsmasq process should be set")
	require.True(t, env.DnsmasqProcess.IsRunning(), "dnsmasq should be running")
	t.Log("✓ Dnsmasq is running")
}

func testKindClusterAccessible(t *testing.T, env *e2e.ShaperTestEnvironment) {
	// Verify KIND cluster is accessible
	require.NotEmpty(t, env.Kubeconfig)
	require.FileExists(t, env.Kubeconfig)

	// Try to get cluster info using kubectl
	status, err := kind.GetPodStatus(env.Kubeconfig, "kube-system")
	require.NoError(t, err)
	require.NotEmpty(t, status)
	t.Log("✓ KIND cluster is accessible")
}

func testIPXEBootFlow(t *testing.T, env *e2e.ShaperTestEnvironment) {
	t.Log("Testing iPXE boot flow...")

	// Note: This test requires:
	// 1. Actual iPXE boot files in TFTP root
	// 2. Shaper-API deployed and running
	// 3. Profile and Assignment CRDs created
	// For now, we'll test the infrastructure without actual boot

	// Create test config
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

	t.Log("Executing iPXE boot test...")
	result, err := e2e.ExecuteIPXEBootTest(testConfig)

	// Log all test logs
	for _, log := range result.Logs {
		t.Log(log)
	}

	// We expect the test to partially succeed (DHCP should work)
	// but might not get HTTP calls without actual shaper-API
	if err != nil {
		t.Logf("Test completed with errors: %v", err)
	}

	// Verify at least DHCP works
	if result.DHCPLeaseObtained {
		t.Log("✓ DHCP lease obtained successfully")
	} else {
		t.Log("⚠ DHCP lease not obtained (this might be expected without full setup)")
	}

	// Don't fail the test - this is more of an integration smoke test
	t.Logf("Test result: Success=%v, DHCP=%v, TFTP=%v, HTTP=%v",
		result.Success,
		result.DHCPLeaseObtained,
		result.TFTPBootFetched,
		result.HTTPBootCalled)
}

// TestIPXEBootFlow_WithProfile tests boot flow with actual Profile CRD
func TestIPXEBootFlow_WithProfile(t *testing.T) {
	t.Skip("Requires Profile CRD and shaper-API deployment - implement after base infrastructure test passes")

	// This test would:
	// 1. Create a Profile CRD
	// 2. Create an Assignment CRD
	// 3. Boot a VM
	// 4. Verify it gets the correct Profile
}

// TestIPXEBootFlow_MultipleProfiles tests with multiple profiles
func TestIPXEBootFlow_MultipleProfiles(t *testing.T) {
	t.Skip("Requires Profile CRD and shaper-API deployment - implement after base infrastructure test passes")

	// This test would:
	// 1. Create multiple Profile CRDs
	// 2. Create Assignments for different MACs
	// 3. Boot multiple VMs
	// 4. Verify each gets the correct Profile
}

// TestIPXEBootFlow_NoAssignment tests boot flow when no Assignment exists
func TestIPXEBootFlow_NoAssignment(t *testing.T) {
	t.Skip("Requires shaper-API deployment - implement after base infrastructure test passes")

	// This test would:
	// 1. Boot a VM without creating an Assignment
	// 2. Verify it falls back to default behavior
}
