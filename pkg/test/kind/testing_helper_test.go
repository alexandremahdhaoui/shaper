//go:build integration

package kind_test

import (
	"os"
	"path/filepath"
	"testing"
)

const (
	// kubeconfigPath is the path to the kubeconfig file created by `make test-setup`
	// This is relative to the project root
	kubeconfigPath = ".ignore.kindenv.kubeconfig.yaml"
)

// getTestKubeconfig returns the path to the kubeconfig file for the test cluster.
// If the kubeconfig doesn't exist, it skips the test with a message to run `make test-setup` first.
func getTestKubeconfig(t *testing.T) string {
	t.Helper()

	kubeconfigFullPath := getProjectKubeconfigPath(t)

	// Check if kubeconfig exists
	if _, err := os.Stat(kubeconfigFullPath); os.IsNotExist(err) {
		t.Skipf("Kubeconfig not found at %s. Please run 'make test-setup' first to create the test cluster.", kubeconfigFullPath)
	} else if err != nil {
		t.Fatalf("Failed to check kubeconfig existence: %v", err)
	}

	return kubeconfigFullPath
}

// getProjectKubeconfigPath returns the path to the project kubeconfig without checking if it exists.
// This is useful for tests that want to check existence themselves.
func getProjectKubeconfigPath(t *testing.T) string {
	t.Helper()

	// Find project root by looking for go.mod
	projectRoot, err := findProjectRoot()
	if err != nil {
		t.Fatalf("Failed to find project root: %v", err)
	}

	return filepath.Join(projectRoot, kubeconfigPath)
}

// findProjectRoot finds the project root by looking for go.mod file
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root directory without finding go.mod
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
