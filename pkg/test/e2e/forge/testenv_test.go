//go:build e2e

package forge

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e/infrastructure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestJSONEnvironmentStore_SaveLoad tests save and load operations
func TestJSONEnvironmentStore_SaveLoad(t *testing.T) {
	// Create temp directory for store
	tempDir := t.TempDir()

	store, err := NewJSONEnvironmentStore(tempDir)
	require.NoError(t, err)

	// Create test state
	testState := &infrastructure.InfrastructureState{
		ID:             "test-123",
		BridgeName:     "br-test",
		LibvirtNetwork: "net-test",
		DnsmasqID:      "dnsmasq-test",
		KindCluster:    "kind-test",
		Kubeconfig:     "/tmp/kubeconfig",
		TFTPRoot:       "/tmp/tftp",
		ArtifactDir:    "/tmp/artifacts",
		CreatedAt:      time.Now(),
	}

	// Save state
	err = store.Save(testState)
	require.NoError(t, err)

	// Verify file exists
	filePath := filepath.Join(tempDir, "test-123.json")
	assert.FileExists(t, filePath)

	// Load state
	loadedState, err := store.Load("test-123")
	require.NoError(t, err)
	assert.Equal(t, testState.ID, loadedState.ID)
	assert.Equal(t, testState.BridgeName, loadedState.BridgeName)
	assert.Equal(t, testState.LibvirtNetwork, loadedState.LibvirtNetwork)
	assert.Equal(t, testState.DnsmasqID, loadedState.DnsmasqID)
	assert.Equal(t, testState.KindCluster, loadedState.KindCluster)
	assert.Equal(t, testState.Kubeconfig, loadedState.Kubeconfig)
	assert.Equal(t, testState.TFTPRoot, loadedState.TFTPRoot)
	assert.Equal(t, testState.ArtifactDir, loadedState.ArtifactDir)
}

// TestJSONEnvironmentStore_LoadNotFound tests loading non-existent environment
func TestJSONEnvironmentStore_LoadNotFound(t *testing.T) {
	tempDir := t.TempDir()

	store, err := NewJSONEnvironmentStore(tempDir)
	require.NoError(t, err)

	// Try to load non-existent environment
	_, err = store.Load("nonexistent")
	assert.True(t, errors.Is(err, ErrEnvironmentNotFound))
}

// TestJSONEnvironmentStore_List tests listing environments
func TestJSONEnvironmentStore_List(t *testing.T) {
	tempDir := t.TempDir()

	store, err := NewJSONEnvironmentStore(tempDir)
	require.NoError(t, err)

	// Save multiple environments
	states := []*infrastructure.InfrastructureState{
		{
			ID:          "test-1",
			BridgeName:  "br-test-1",
			KindCluster: "kind-test-1",
			CreatedAt:   time.Now(),
		},
		{
			ID:          "test-2",
			BridgeName:  "br-test-2",
			KindCluster: "kind-test-2",
			CreatedAt:   time.Now(),
		},
	}

	for _, state := range states {
		err = store.Save(state)
		require.NoError(t, err)
	}

	// List environments
	loaded, err := store.List()
	require.NoError(t, err)
	assert.Len(t, loaded, 2)

	// Verify IDs are present
	ids := make(map[string]bool)
	for _, env := range loaded {
		ids[env.ID] = true
	}
	assert.True(t, ids["test-1"])
	assert.True(t, ids["test-2"])
}

// TestJSONEnvironmentStore_Delete tests deleting environments
func TestJSONEnvironmentStore_Delete(t *testing.T) {
	tempDir := t.TempDir()

	store, err := NewJSONEnvironmentStore(tempDir)
	require.NoError(t, err)

	// Save environment
	testState := &infrastructure.InfrastructureState{
		ID:          "test-delete",
		BridgeName:  "br-test",
		KindCluster: "kind-test",
		CreatedAt:   time.Now(),
	}
	err = store.Save(testState)
	require.NoError(t, err)

	// Delete environment
	err = store.Delete("test-delete")
	require.NoError(t, err)

	// Verify file is gone
	filePath := filepath.Join(tempDir, "test-delete.json")
	assert.NoFileExists(t, filePath)

	// Try to load deleted environment
	_, err = store.Load("test-delete")
	assert.True(t, errors.Is(err, ErrEnvironmentNotFound))
}

// TestJSONEnvironmentStore_DeleteNotFound tests deleting non-existent environment
func TestJSONEnvironmentStore_DeleteNotFound(t *testing.T) {
	tempDir := t.TempDir()

	store, err := NewJSONEnvironmentStore(tempDir)
	require.NoError(t, err)

	// Try to delete non-existent environment
	err = store.Delete("nonexistent")
	assert.True(t, errors.Is(err, ErrEnvironmentNotFound))
}

// TestJSONEnvironmentStore_SaveNil tests saving nil state
func TestJSONEnvironmentStore_SaveNil(t *testing.T) {
	tempDir := t.TempDir()

	store, err := NewJSONEnvironmentStore(tempDir)
	require.NoError(t, err)

	// Try to save nil state
	err = store.Save(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

// TestJSONEnvironmentStore_SaveEmptyID tests saving state with empty ID
func TestJSONEnvironmentStore_SaveEmptyID(t *testing.T) {
	tempDir := t.TempDir()

	store, err := NewJSONEnvironmentStore(tempDir)
	require.NoError(t, err)

	// Try to save state with empty ID
	testState := &infrastructure.InfrastructureState{
		ID:         "",
		BridgeName: "br-test",
	}
	err = store.Save(testState)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

// TestJSONEnvironmentStore_LoadCorrupted tests loading corrupted JSON
func TestJSONEnvironmentStore_LoadCorrupted(t *testing.T) {
	tempDir := t.TempDir()

	store, err := NewJSONEnvironmentStore(tempDir)
	require.NoError(t, err)

	// Write corrupted JSON file
	filePath := filepath.Join(tempDir, "corrupted.json")
	err = os.WriteFile(filePath, []byte("not valid json"), 0o644)
	require.NoError(t, err)

	// Try to load corrupted file
	_, err = store.Load("corrupted")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrStoreCorrupted))
}

// TestJSONEnvironmentStore_ListSkipsNonJSON tests that List skips non-JSON files
func TestJSONEnvironmentStore_ListSkipsNonJSON(t *testing.T) {
	tempDir := t.TempDir()

	store, err := NewJSONEnvironmentStore(tempDir)
	require.NoError(t, err)

	// Save valid environment
	testState := &infrastructure.InfrastructureState{
		ID:          "test-valid",
		BridgeName:  "br-test",
		KindCluster: "kind-test",
		CreatedAt:   time.Now(),
	}
	err = store.Save(testState)
	require.NoError(t, err)

	// Create non-JSON file
	err = os.WriteFile(filepath.Join(tempDir, "README.txt"), []byte("readme"), 0o644)
	require.NoError(t, err)

	// Create corrupted JSON file
	err = os.WriteFile(filepath.Join(tempDir, "bad.json"), []byte("not json"), 0o644)
	require.NoError(t, err)

	// List should only return valid environment
	loaded, err := store.List()
	require.NoError(t, err)
	assert.Len(t, loaded, 1)
	assert.Equal(t, "test-valid", loaded[0].ID)
}

// TestTestenv_ParseConfig tests config parsing
func TestTestenv_ParseConfig(t *testing.T) {
	tempDir := t.TempDir()

	testenv, err := NewTestenv(tempDir, tempDir)
	require.NoError(t, err)

	tests := []struct {
		name      string
		config    map[string]interface{}
		expectErr bool
		checkSpec func(*testing.T, infrastructure.InfrastructureSpec)
	}{
		{
			name: "valid config",
			config: map[string]interface{}{
				"network": map[string]interface{}{
					"cidr":      "192.168.100.1/24",
					"bridge":    "br-test",
					"dhcpRange": "192.168.100.10,192.168.100.100",
				},
				"kind": map[string]interface{}{
					"clusterName": "kind-test",
					"version":     "v1.28.0",
				},
				"shaper": map[string]interface{}{
					"namespace":   "default",
					"apiReplicas": 2,
				},
			},
			expectErr: false,
			checkSpec: func(t *testing.T, spec infrastructure.InfrastructureSpec) {
				assert.Equal(t, "192.168.100.1/24", spec.Network.CIDR)
				assert.Equal(t, "br-test", spec.Network.Bridge)
				assert.Equal(t, "192.168.100.10,192.168.100.100", spec.Network.DHCPRange)
				assert.Equal(t, "kind-test", spec.Kind.ClusterName)
				assert.Equal(t, "v1.28.0", spec.Kind.Version)
				assert.Equal(t, "default", spec.Shaper.Namespace)
				assert.Equal(t, 2, spec.Shaper.APIReplicas)
			},
		},
		{
			name: "missing network",
			config: map[string]interface{}{
				"kind": map[string]interface{}{
					"clusterName": "kind-test",
				},
				"shaper": map[string]interface{}{
					"namespace": "default",
				},
			},
			expectErr: true,
		},
		{
			name: "missing kind",
			config: map[string]interface{}{
				"network": map[string]interface{}{
					"cidr":      "192.168.100.1/24",
					"bridge":    "br-test",
					"dhcpRange": "192.168.100.10,192.168.100.100",
				},
				"shaper": map[string]interface{}{
					"namespace": "default",
				},
			},
			expectErr: true,
		},
		{
			name: "missing shaper",
			config: map[string]interface{}{
				"network": map[string]interface{}{
					"cidr":      "192.168.100.1/24",
					"bridge":    "br-test",
					"dhcpRange": "192.168.100.10,192.168.100.100",
				},
				"kind": map[string]interface{}{
					"clusterName": "kind-test",
				},
			},
			expectErr: true,
		},
		{
			name: "default api replicas",
			config: map[string]interface{}{
				"network": map[string]interface{}{
					"cidr":      "192.168.100.1/24",
					"bridge":    "br-test",
					"dhcpRange": "192.168.100.10,192.168.100.100",
				},
				"kind": map[string]interface{}{
					"clusterName": "kind-test",
				},
				"shaper": map[string]interface{}{
					"namespace": "default",
				},
			},
			expectErr: false,
			checkSpec: func(t *testing.T, spec infrastructure.InfrastructureSpec) {
				assert.Equal(t, 1, spec.Shaper.APIReplicas)
			},
		},
		{
			name: "api replicas as float64",
			config: map[string]interface{}{
				"network": map[string]interface{}{
					"cidr":      "192.168.100.1/24",
					"bridge":    "br-test",
					"dhcpRange": "192.168.100.10,192.168.100.100",
				},
				"kind": map[string]interface{}{
					"clusterName": "kind-test",
				},
				"shaper": map[string]interface{}{
					"namespace":   "default",
					"apiReplicas": float64(3),
				},
			},
			expectErr: false,
			checkSpec: func(t *testing.T, spec infrastructure.InfrastructureSpec) {
				assert.Equal(t, 3, spec.Shaper.APIReplicas)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := testenv.parseConfig(tt.config)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.checkSpec != nil {
					tt.checkSpec(t, spec)
				}
			}
		})
	}
}

// TestTestenv_GetListDelete tests Get, List, and Delete operations
func TestTestenv_GetListDelete(t *testing.T) {
	tempDir := t.TempDir()

	testenv, err := NewTestenv(tempDir, tempDir)
	require.NoError(t, err)

	// Create test state manually
	testState := &infrastructure.InfrastructureState{
		ID:             "test-get-123",
		BridgeName:     "br-test",
		LibvirtNetwork: "net-test",
		DnsmasqID:      "dnsmasq-test",
		KindCluster:    "kind-test",
		Kubeconfig:     "/tmp/kubeconfig",
		TFTPRoot:       "/tmp/tftp",
		ArtifactDir:    "/tmp/artifacts",
		CreatedAt:      time.Now(),
	}
	err = testenv.store.Save(testState)
	require.NoError(t, err)

	ctx := context.Background()

	// Test Get
	result, err := testenv.Get(ctx, "test-get-123")
	require.NoError(t, err)
	assert.Equal(t, "test-get-123", result["id"])
	assert.Equal(t, "br-test", result["bridgeName"])
	assert.Equal(t, "net-test", result["libvirtNetwork"])
	assert.Equal(t, "dnsmasq-test", result["dnsmasqID"])
	assert.Equal(t, "kind-test", result["kindCluster"])
	assert.Equal(t, "/tmp/kubeconfig", result["kubeconfig"])
	assert.Equal(t, "/tmp/tftp", result["tftpRoot"])
	assert.Equal(t, "/tmp/artifacts", result["artifactDir"])

	// Test List
	list, err := testenv.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "test-get-123", list[0]["id"])
	assert.Equal(t, "kind-test", list[0]["kindCluster"])

	// Test Delete (will fail because infrastructure doesn't exist, but should remove from store)
	err = testenv.Delete(ctx, "test-get-123")
	// We expect an error because teardown will fail (no actual infrastructure)
	assert.Error(t, err)

	// But the store should still have removed it
	_, err = testenv.store.Load("test-get-123")
	assert.True(t, errors.Is(err, ErrEnvironmentNotFound))
}

// TestTestenv_GetNotFound tests Get with non-existent ID
func TestTestenv_GetNotFound(t *testing.T) {
	tempDir := t.TempDir()

	testenv, err := NewTestenv(tempDir, tempDir)
	require.NoError(t, err)

	ctx := context.Background()

	// Try to get non-existent environment
	_, err = testenv.Get(ctx, "nonexistent")
	assert.True(t, errors.Is(err, ErrEnvironmentNotFound))
}

// TestLoadScenario tests scenario loading helper
func TestLoadScenario(t *testing.T) {
	// Create temp scenario file
	tempDir := t.TempDir()
	scenarioPath := filepath.Join(tempDir, "test.yaml")

	scenarioYAML := `name: test-scenario
description: Test scenario
architecture: x86_64
vms:
  - name: test-vm
    uuid: "11111111-1111-1111-1111-111111111111"
    memory: 1024
    vcpus: 1
assertions:
  - vm: test-vm
    type: dhcp_lease
`

	err := os.WriteFile(scenarioPath, []byte(scenarioYAML), 0o644)
	require.NoError(t, err)

	// Load scenario
	scenario, err := LoadScenario(scenarioPath)
	require.NoError(t, err)
	assert.Equal(t, "test-scenario", scenario.Name)
	assert.Equal(t, "Test scenario", scenario.Description)
	assert.Len(t, scenario.VMs, 1)
	assert.Equal(t, "test-vm", scenario.VMs[0].Name)
}

// TestJSONEnvironmentStore_Concurrent tests concurrent operations
func TestJSONEnvironmentStore_Concurrent(t *testing.T) {
	tempDir := t.TempDir()

	store, err := NewJSONEnvironmentStore(tempDir)
	require.NoError(t, err)

	// Create multiple environments concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			state := &infrastructure.InfrastructureState{
				ID:          fmt.Sprintf("test-%d", id),
				BridgeName:  fmt.Sprintf("br-%d", id),
				KindCluster: fmt.Sprintf("kind-%d", id),
				CreatedAt:   time.Now(),
			}
			err := store.Save(state)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all saves
	for i := 0; i < 10; i++ {
		<-done
	}

	// List should have all 10
	loaded, err := store.List()
	require.NoError(t, err)
	assert.Len(t, loaded, 10)
}

// TestNewJSONEnvironmentStore_CreateDirectory tests directory creation
func TestNewJSONEnvironmentStore_CreateDirectory(t *testing.T) {
	tempDir := t.TempDir()
	storeDir := filepath.Join(tempDir, "nested", "store")

	// Store directory doesn't exist yet
	assert.NoDirExists(t, storeDir)

	// Creating store should create directory
	store, err := NewJSONEnvironmentStore(storeDir)
	require.NoError(t, err)
	assert.NotNil(t, store)
	assert.DirExists(t, storeDir)
}

// Helper function to create a minimal valid config
func minimalConfig() map[string]interface{} {
	return map[string]interface{}{
		"network": map[string]interface{}{
			"cidr":      "192.168.100.1/24",
			"bridge":    "br-test",
			"dhcpRange": "192.168.100.10,192.168.100.100",
		},
		"kind": map[string]interface{}{
			"clusterName": "kind-test",
		},
		"shaper": map[string]interface{}{
			"namespace": "default",
		},
	}
}

// BenchmarkSave benchmarks save operations
func BenchmarkSave(b *testing.B) {
	tempDir := b.TempDir()
	store, _ := NewJSONEnvironmentStore(tempDir)

	state := &infrastructure.InfrastructureState{
		ID:          "bench-test",
		BridgeName:  "br-test",
		KindCluster: "kind-test",
		CreatedAt:   time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state.ID = fmt.Sprintf("bench-%d", i)
		_ = store.Save(state)
	}
}

// BenchmarkLoad benchmarks load operations
func BenchmarkLoad(b *testing.B) {
	tempDir := b.TempDir()
	store, _ := NewJSONEnvironmentStore(tempDir)

	state := &infrastructure.InfrastructureState{
		ID:          "bench-test",
		BridgeName:  "br-test",
		KindCluster: "kind-test",
		CreatedAt:   time.Now(),
	}
	_ = store.Save(state)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.Load("bench-test")
	}
}
