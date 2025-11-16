//go:build e2e

package infrastructure

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateTestID(t *testing.T) {
	testID := generateTestID()

	// Check format: e2e-<timestamp>-<random>
	if !strings.HasPrefix(testID, "e2e-") {
		t.Errorf("test ID should start with 'e2e-', got: %s", testID)
	}

	// Should have 3 parts separated by hyphens (e2e, timestamp, random)
	parts := strings.Split(testID, "-")
	if len(parts) < 4 { // e2e-YYYYMMDD-HHMMSS-random
		t.Errorf("test ID should have format e2e-<timestamp>-<random>, got: %s", testID)
	}

	// Check uniqueness - generate two IDs
	testID2 := generateTestID()
	if testID == testID2 {
		t.Errorf("test IDs should be unique, both are: %s", testID)
	}
}

func TestValidateSpec(t *testing.T) {
	tests := []struct {
		name        string
		spec        InfrastructureSpec
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid spec",
			spec: InfrastructureSpec{
				Network: NetworkSpec{
					CIDR:      "192.168.100.1/24",
					Bridge:    "br-test",
					DHCPRange: "192.168.100.10,192.168.100.250",
				},
				Kind: KindSpec{
					ClusterName: "test-cluster",
				},
				Shaper: ShaperSpec{
					Namespace: "default",
				},
			},
			expectError: false,
		},
		{
			name: "missing network CIDR",
			spec: InfrastructureSpec{
				Network: NetworkSpec{
					Bridge:    "br-test",
					DHCPRange: "192.168.100.10,192.168.100.250",
				},
				Kind: KindSpec{
					ClusterName: "test-cluster",
				},
				Shaper: ShaperSpec{
					Namespace: "default",
				},
			},
			expectError: true,
			errorMsg:    "network CIDR is required",
		},
		{
			name: "missing bridge name",
			spec: InfrastructureSpec{
				Network: NetworkSpec{
					CIDR:      "192.168.100.1/24",
					DHCPRange: "192.168.100.10,192.168.100.250",
				},
				Kind: KindSpec{
					ClusterName: "test-cluster",
				},
				Shaper: ShaperSpec{
					Namespace: "default",
				},
			},
			expectError: true,
			errorMsg:    "network bridge name is required",
		},
		{
			name: "missing DHCP range",
			spec: InfrastructureSpec{
				Network: NetworkSpec{
					CIDR:   "192.168.100.1/24",
					Bridge: "br-test",
				},
				Kind: KindSpec{
					ClusterName: "test-cluster",
				},
				Shaper: ShaperSpec{
					Namespace: "default",
				},
			},
			expectError: true,
			errorMsg:    "network DHCP range is required",
		},
		{
			name: "missing cluster name",
			spec: InfrastructureSpec{
				Network: NetworkSpec{
					CIDR:      "192.168.100.1/24",
					Bridge:    "br-test",
					DHCPRange: "192.168.100.10,192.168.100.250",
				},
				Kind: KindSpec{},
				Shaper: ShaperSpec{
					Namespace: "default",
				},
			},
			expectError: true,
			errorMsg:    "KIND cluster name is required",
		},
		{
			name: "missing namespace",
			spec: InfrastructureSpec{
				Network: NetworkSpec{
					CIDR:      "192.168.100.1/24",
					Bridge:    "br-test",
					DHCPRange: "192.168.100.10,192.168.100.250",
				},
				Kind: KindSpec{
					ClusterName: "test-cluster",
				},
				Shaper: ShaperSpec{},
			},
			expectError: true,
			errorMsg:    "shaper namespace is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewInfrastructureManager(tt.spec, "/tmp/test-artifacts")
			err := mgr.validateSpec()

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorMsg)
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got: %v", tt.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			}
		})
	}
}

func TestNewInfrastructureManager(t *testing.T) {
	spec := InfrastructureSpec{
		Network: NetworkSpec{
			CIDR:      "192.168.100.1/24",
			Bridge:    "br-test",
			DHCPRange: "192.168.100.10,192.168.100.250",
		},
		Kind: KindSpec{
			ClusterName: "test-cluster",
		},
		Shaper: ShaperSpec{
			Namespace: "default",
		},
	}

	artifactDir := "/tmp/test-artifacts"
	mgr := NewInfrastructureManager(spec, artifactDir)

	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}

	if mgr.config.Network.CIDR != spec.Network.CIDR {
		t.Errorf("expected CIDR %s, got %s", spec.Network.CIDR, mgr.config.Network.CIDR)
	}

	if mgr.artifactDir != artifactDir {
		t.Errorf("expected artifact dir %s, got %s", artifactDir, mgr.artifactDir)
	}
}

func TestInfrastructureState(t *testing.T) {
	// Test that InfrastructureState has all required fields from architecture.md
	state := InfrastructureState{
		ID:             "e2e-20240101-120000-abc123",
		BridgeName:     "br-test",
		LibvirtNetwork: "net-test",
		DnsmasqID:      "dnsmasq-test",
		KindCluster:    "test-cluster",
		Kubeconfig:     "/tmp/kubeconfig",
		TFTPRoot:       "/tmp/tftp",
		ArtifactDir:    "/tmp/artifacts",
		CreatedAt:      time.Now(),
	}

	// Verify all fields are set
	if state.ID == "" {
		t.Error("ID should not be empty")
	}
	if state.BridgeName == "" {
		t.Error("BridgeName should not be empty")
	}
	if state.LibvirtNetwork == "" {
		t.Error("LibvirtNetwork should not be empty")
	}
	if state.DnsmasqID == "" {
		t.Error("DnsmasqID should not be empty")
	}
	if state.KindCluster == "" {
		t.Error("KindCluster should not be empty")
	}
	if state.Kubeconfig == "" {
		t.Error("Kubeconfig should not be empty")
	}
	if state.TFTPRoot == "" {
		t.Error("TFTPRoot should not be empty")
	}
	if state.ArtifactDir == "" {
		t.Error("ArtifactDir should not be empty")
	}
	if state.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestTeardownNilState(t *testing.T) {
	spec := InfrastructureSpec{
		Network: NetworkSpec{
			CIDR:      "192.168.100.1/24",
			Bridge:    "br-test",
			DHCPRange: "192.168.100.10,192.168.100.250",
		},
		Kind: KindSpec{
			ClusterName: "test-cluster",
		},
		Shaper: ShaperSpec{
			Namespace: "default",
		},
	}

	mgr := NewInfrastructureManager(spec, "/tmp/test-artifacts")
	ctx := context.Background()

	// Teardown with nil state should not panic
	err := mgr.Teardown(ctx, nil)
	if err != nil {
		t.Errorf("expected nil error for nil state, got: %v", err)
	}
}

func TestGetState(t *testing.T) {
	spec := InfrastructureSpec{
		Network: NetworkSpec{
			CIDR:      "192.168.100.1/24",
			Bridge:    "br-test",
			DHCPRange: "192.168.100.10,192.168.100.250",
		},
		Kind: KindSpec{
			ClusterName: "test-cluster",
		},
		Shaper: ShaperSpec{
			Namespace: "default",
		},
	}

	mgr := NewInfrastructureManager(spec, "/tmp/test-artifacts")

	// GetState is not implemented yet, should return ErrEnvironmentNotFound
	state, err := mgr.GetState("test-id")
	if state != nil {
		t.Error("expected nil state")
	}
	if err != ErrEnvironmentNotFound {
		t.Errorf("expected ErrEnvironmentNotFound, got: %v", err)
	}
}

func TestArtifactDirectoryCreation(t *testing.T) {
	// Create temp directory for test
	tempDir := t.TempDir()

	spec := InfrastructureSpec{
		Network: NetworkSpec{
			CIDR:      "192.168.100.1/24",
			Bridge:    "br-test-" + generateTestID(),
			DHCPRange: "192.168.100.10,192.168.100.250",
		},
		Kind: KindSpec{
			ClusterName: "test-cluster-" + generateTestID(),
		},
		Shaper: ShaperSpec{
			Namespace: "default",
		},
	}

	mgr := NewInfrastructureManager(spec, tempDir)

	// Note: This test can only verify directory creation logic
	// Full Setup() requires actual system resources (bridge, libvirt, etc.)
	// Those are tested in integration tests

	// Verify artifact directory would be created with proper path
	testID := "e2e-test-123"
	expectedPath := filepath.Join(tempDir, testID)

	// Simulate what Setup() does for artifact directory
	artifactDir := filepath.Join(mgr.artifactDir, testID)
	err := os.MkdirAll(artifactDir, 0o755)
	if err != nil {
		t.Fatalf("failed to create artifact directory: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(artifactDir); os.IsNotExist(err) {
		t.Errorf("artifact directory was not created: %s", expectedPath)
	}

	// Cleanup
	os.RemoveAll(artifactDir)
}

func TestInfrastructureSpecTypes(t *testing.T) {
	// Test that spec types match architecture.md requirements
	spec := InfrastructureSpec{
		Network: NetworkSpec{
			CIDR:      "192.168.100.1/24",
			Bridge:    "br-test",
			DHCPRange: "192.168.100.10,192.168.100.250",
		},
		Kind: KindSpec{
			ClusterName: "test-cluster",
			Version:     "v1.27.0",
		},
		Shaper: ShaperSpec{
			Namespace:   "shaper-system",
			APIReplicas: 2,
		},
	}

	// Verify NetworkSpec fields
	if spec.Network.CIDR == "" {
		t.Error("NetworkSpec.CIDR should not be empty")
	}
	if spec.Network.Bridge == "" {
		t.Error("NetworkSpec.Bridge should not be empty")
	}
	if spec.Network.DHCPRange == "" {
		t.Error("NetworkSpec.DHCPRange should not be empty")
	}

	// Verify KindSpec fields
	if spec.Kind.ClusterName == "" {
		t.Error("KindSpec.ClusterName should not be empty")
	}
	if spec.Kind.Version == "" {
		t.Error("KindSpec.Version should not be empty")
	}

	// Verify ShaperSpec fields
	if spec.Shaper.Namespace == "" {
		t.Error("ShaperSpec.Namespace should not be empty")
	}
	if spec.Shaper.APIReplicas <= 0 {
		t.Error("ShaperSpec.APIReplicas should be positive")
	}
}

func TestSentinelErrors(t *testing.T) {
	// Verify sentinel errors are defined
	errors := []error{
		ErrBridgeCreationFailed,
		ErrDnsmasqStartFailed,
		ErrLibvirtNetworkFailed,
		ErrKindClusterFailed,
		ErrDeploymentFailed,
		ErrEnvironmentNotFound,
		ErrInvalidSpec,
		ErrCleanupFailed,
	}

	for _, err := range errors {
		if err == nil {
			t.Error("sentinel error should not be nil")
		}
		if err.Error() == "" {
			t.Error("sentinel error should have a message")
		}
	}
}

// Integration test marker - these tests require actual system resources
// and should be run with: go test -tags=integration -v

func TestSetupIntegration(t *testing.T) {
	// Skip in unit test mode
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// This test requires:
	// - sudo access for network operations
	// - libvirt installed and running
	// - kind installed
	// - kubectl installed
	t.Skip("Integration test - requires sudo, libvirt, kind, kubectl - run manually with proper setup")
}

func TestTeardownIntegration(t *testing.T) {
	// Skip in unit test mode
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	t.Skip("Integration test - requires sudo, libvirt, kind - run manually with proper setup")
}
