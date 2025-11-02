//go:build integration

package kind_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alexandremahdhaoui/shaper/pkg/test/kind"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Integration tests
//
// Note: These tests for cluster creation/deletion are designed to test the cluster
// management functions themselves. When running integration tests that use the shared
// cluster (created by `make test-setup`), these tests will be skipped since they
// require creating/destroying clusters.

func TestCreateCluster_Integration(t *testing.T) {
	if !kind.IsKindInstalled() {
		t.Skip("KIND not installed")
	}

	// Skip if using shared test cluster
	if _, err := os.Stat(getProjectKubeconfigPath(t)); err == nil {
		t.Skip(
			"Skipping cluster creation test when using shared test cluster. This test creates its own cluster.",
		)
	}

	clusterName := "test-" + uuid.NewString()[:8]
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")

	config := kind.ClusterConfig{
		Name:       clusterName,
		Kubeconfig: kubeconfigPath,
	}

	err := kind.CreateCluster(config)
	require.NoError(t, err)
	defer kind.DeleteCluster(clusterName)

	// Verify cluster exists
	exists, err := kind.ClusterExists(clusterName)
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
	if !kind.IsKindInstalled() {
		t.Skip("KIND not installed")
	}

	// Skip if using shared test cluster
	if _, err := os.Stat(getProjectKubeconfigPath(t)); err == nil {
		t.Skip(
			"Skipping cluster creation test when using shared test cluster. This test creates its own cluster.",
		)
	}

	clusterName := "test-" + uuid.NewString()[:8]
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")

	config := kind.ClusterConfig{
		Name:       clusterName,
		Kubeconfig: kubeconfigPath,
	}

	// Create first time
	err := kind.CreateCluster(config)
	require.NoError(t, err)
	defer kind.DeleteCluster(clusterName)

	// Create second time - should not error
	err = kind.CreateCluster(config)
	require.NoError(t, err)

	// Verify cluster still exists
	exists, err := kind.ClusterExists(clusterName)
	require.NoError(t, err)
	require.True(t, exists)
}

func TestDeleteCluster_Integration(t *testing.T) {
	if !kind.IsKindInstalled() {
		t.Skip("KIND not installed")
	}

	// Skip if using shared test cluster
	if _, err := os.Stat(getProjectKubeconfigPath(t)); err == nil {
		t.Skip(
			"Skipping cluster deletion test when using shared test cluster. This test creates and deletes its own cluster.",
		)
	}

	clusterName := "test-" + uuid.NewString()[:8]

	config := kind.ClusterConfig{
		Name: clusterName,
	}

	// Create cluster
	err := kind.CreateCluster(config)
	require.NoError(t, err)

	// Verify it exists
	exists, err := kind.ClusterExists(clusterName)
	require.NoError(t, err)
	require.True(t, exists)

	// Delete cluster
	err = kind.DeleteCluster(clusterName)
	require.NoError(t, err)

	// Verify it's gone
	exists, err = kind.ClusterExists(clusterName)
	require.NoError(t, err)
	require.False(t, exists)
}

func TestDeleteCluster_Idempotent_Integration(t *testing.T) {
	if !kind.IsKindInstalled() {
		t.Skip("KIND not installed")
	}

	// Skip if using shared test cluster
	if _, err := os.Stat(getProjectKubeconfigPath(t)); err == nil {
		t.Skip(
			"Skipping cluster deletion test when using shared test cluster. This test creates and deletes its own cluster.",
		)
	}

	clusterName := "test-" + uuid.NewString()[:8]

	config := kind.ClusterConfig{
		Name: clusterName,
	}

	// Create and delete cluster
	err := kind.CreateCluster(config)
	require.NoError(t, err)

	err = kind.DeleteCluster(clusterName)
	require.NoError(t, err)

	// Delete again - should not error
	err = kind.DeleteCluster(clusterName)
	require.NoError(t, err)
}

func TestClusterExists_NonExistent_Integration(t *testing.T) {
	if !kind.IsKindInstalled() {
		t.Skip("KIND not installed")
	}

	// This test doesn't create/delete clusters, so it can run with shared cluster
	// Check for cluster that doesn't exist
	exists, err := kind.ClusterExists("nonexistent-cluster-" + uuid.NewString())
	require.NoError(t, err)
	require.False(t, exists)
}

func TestGetKubeconfig_Integration(t *testing.T) {
	if !kind.IsKindInstalled() {
		t.Skip("KIND not installed")
	}

	// Skip if using shared test cluster - we can't get the cluster name easily from kubeconfig
	if _, err := os.Stat(getProjectKubeconfigPath(t)); err == nil {
		t.Skip(
			"Skipping kubeconfig retrieval test when using shared test cluster. This test creates its own cluster.",
		)
	}

	clusterName := "test-" + uuid.NewString()[:8]

	config := kind.ClusterConfig{
		Name: clusterName,
	}

	// Create cluster
	err := kind.CreateCluster(config)
	require.NoError(t, err)
	defer kind.DeleteCluster(clusterName)

	// Get kubeconfig
	kubeconfig, err := kind.GetKubeconfig(clusterName)
	require.NoError(t, err)
	require.NotEmpty(t, kubeconfig)
	require.Contains(t, kubeconfig, clusterName)
	require.Contains(t, kubeconfig, "apiVersion")
	require.Contains(t, kubeconfig, "clusters")
}
