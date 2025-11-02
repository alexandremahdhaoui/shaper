//go:build integration

package kind

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Integration tests

func TestCreateCluster_Integration(t *testing.T) {
	if !IsKindInstalled() {
		t.Skip("KIND not installed")
	}

	clusterName := "test-" + uuid.NewString()[:8]
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")

	config := ClusterConfig{
		Name:       clusterName,
		Kubeconfig: kubeconfigPath,
	}

	err := CreateCluster(config)
	require.NoError(t, err)
	defer DeleteCluster(clusterName)

	// Verify cluster exists
	exists, err := ClusterExists(clusterName)
	require.NoError(t, err)
	require.True(t, exists)

	// Verify kubeconfig was created
	require.FileExists(t, kubeconfigPath)

	// Verify kubeconfig content
	kubeconfigContent, err := os.ReadFile(kubeconfigPath)
	require.NoError(t, err)
	require.Contains(t, string(kubeconfigContent), clusterName)
}

func TestCreateCluster_Idempotent_Integration(t *testing.T) {
	if !IsKindInstalled() {
		t.Skip("KIND not installed")
	}

	clusterName := "test-" + uuid.NewString()[:8]
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")

	config := ClusterConfig{
		Name:       clusterName,
		Kubeconfig: kubeconfigPath,
	}

	// Create first time
	err := CreateCluster(config)
	require.NoError(t, err)
	defer DeleteCluster(clusterName)

	// Create second time - should not error
	err = CreateCluster(config)
	require.NoError(t, err)

	// Verify cluster still exists
	exists, err := ClusterExists(clusterName)
	require.NoError(t, err)
	require.True(t, exists)
}

func TestDeleteCluster_Integration(t *testing.T) {
	if !IsKindInstalled() {
		t.Skip("KIND not installed")
	}

	clusterName := "test-" + uuid.NewString()[:8]

	config := ClusterConfig{
		Name: clusterName,
	}

	// Create cluster
	err := CreateCluster(config)
	require.NoError(t, err)

	// Verify it exists
	exists, err := ClusterExists(clusterName)
	require.NoError(t, err)
	require.True(t, exists)

	// Delete cluster
	err = DeleteCluster(clusterName)
	require.NoError(t, err)

	// Verify it's gone
	exists, err = ClusterExists(clusterName)
	require.NoError(t, err)
	require.False(t, exists)
}

func TestDeleteCluster_Idempotent_Integration(t *testing.T) {
	if !IsKindInstalled() {
		t.Skip("KIND not installed")
	}

	clusterName := "test-" + uuid.NewString()[:8]

	config := ClusterConfig{
		Name: clusterName,
	}

	// Create and delete cluster
	err := CreateCluster(config)
	require.NoError(t, err)

	err = DeleteCluster(clusterName)
	require.NoError(t, err)

	// Delete again - should not error
	err = DeleteCluster(clusterName)
	require.NoError(t, err)
}

func TestClusterExists_NonExistent_Integration(t *testing.T) {
	if !IsKindInstalled() {
		t.Skip("KIND not installed")
	}

	// Check for cluster that doesn't exist
	exists, err := ClusterExists("nonexistent-cluster-" + uuid.NewString())
	require.NoError(t, err)
	require.False(t, exists)
}

func TestGetKubeconfig_Integration(t *testing.T) {
	if !IsKindInstalled() {
		t.Skip("KIND not installed")
	}

	clusterName := "test-" + uuid.NewString()[:8]

	config := ClusterConfig{
		Name: clusterName,
	}

	// Create cluster
	err := CreateCluster(config)
	require.NoError(t, err)
	defer DeleteCluster(clusterName)

	// Get kubeconfig
	kubeconfig, err := GetKubeconfig(clusterName)
	require.NoError(t, err)
	require.NotEmpty(t, kubeconfig)
	require.Contains(t, kubeconfig, clusterName)
	require.Contains(t, kubeconfig, "apiVersion")
	require.Contains(t, kubeconfig, "clusters")
}

