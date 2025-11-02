package kind

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var (
	errClusterNameRequired = errors.New("cluster name is required")
	errCreateCluster       = errors.New("failed to create KIND cluster")
	errDeleteCluster       = errors.New("failed to delete KIND cluster")
	errCheckCluster        = errors.New("failed to check if cluster exists")
	errGetKubeconfig       = errors.New("failed to get kubeconfig")
	errKindNotInstalled    = errors.New("kind command not found - please install KIND")
)

// ClusterConfig contains KIND cluster configuration
type ClusterConfig struct {
	Name       string // Cluster name
	Kubeconfig string // Path to save kubeconfig
	Image      string // Optional KIND node image (e.g., "kindest/node:v1.27.0")
	ConfigFile string // Optional KIND config file path
}

// CreateCluster creates a KIND cluster
func CreateCluster(config ClusterConfig) error {
	if config.Name == "" {
		return errClusterNameRequired
	}

	// Check if kind is installed
	if _, err := exec.LookPath("kind"); err != nil {
		return errKindNotInstalled
	}

	// Check if cluster already exists
	exists, err := ClusterExists(config.Name)
	if err != nil {
		return err
	}
	if exists {
		// Cluster already exists, just export kubeconfig if path provided
		if config.Kubeconfig != "" {
			return exportKubeconfig(config.Name, config.Kubeconfig)
		}
		return nil
	}

	// Build kind create command
	args := []string{"create", "cluster", "--name", config.Name}

	// Add optional image
	if config.Image != "" {
		args = append(args, "--image", config.Image)
	}

	// Add optional config file
	if config.ConfigFile != "" {
		args = append(args, "--config", config.ConfigFile)
	}

	// Add kubeconfig export
	if config.Kubeconfig != "" {
		args = append(args, "--kubeconfig", config.Kubeconfig)
	}

	// Create cluster
	cmd := exec.Command("kind", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %v", errCreateCluster, err)
	}

	// If kubeconfig path not specified in command, export it now
	if config.Kubeconfig != "" {
		return exportKubeconfig(config.Name, config.Kubeconfig)
	}

	return nil
}

// DeleteCluster deletes a KIND cluster
// Idempotent - returns nil if cluster doesn't exist
func DeleteCluster(name string) error {
	if name == "" {
		return errClusterNameRequired
	}

	// Check if kind is installed
	if _, err := exec.LookPath("kind"); err != nil {
		return errKindNotInstalled
	}

	// Check if cluster exists
	exists, err := ClusterExists(name)
	if err != nil {
		return err
	}
	if !exists {
		// Cluster doesn't exist, nothing to do
		return nil
	}

	// Delete cluster
	cmd := exec.Command("kind", "delete", "cluster", "--name", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %v, output: %s", errDeleteCluster, err, string(output))
	}

	return nil
}

// ClusterExists checks if a KIND cluster exists
func ClusterExists(name string) (bool, error) {
	if name == "" {
		return false, errClusterNameRequired
	}

	// Check if kind is installed
	if _, err := exec.LookPath("kind"); err != nil {
		return false, errKindNotInstalled
	}

	// Get list of clusters
	cmd := exec.Command("kind", "get", "clusters")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("%w: %v", errCheckCluster, err)
	}

	// Check if our cluster is in the list
	clusters := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, cluster := range clusters {
		if strings.TrimSpace(cluster) == name {
			return true, nil
		}
	}

	return false, nil
}

// GetKubeconfig gets the kubeconfig for a cluster
func GetKubeconfig(name string) (string, error) {
	if name == "" {
		return "", errClusterNameRequired
	}

	// Check if kind is installed
	if _, err := exec.LookPath("kind"); err != nil {
		return "", errKindNotInstalled
	}

	// Get kubeconfig
	cmd := exec.Command("kind", "get", "kubeconfig", "--name", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %v", errGetKubeconfig, err)
	}

	return string(output), nil
}

// exportKubeconfig exports kubeconfig to a file
func exportKubeconfig(name, path string) error {
	// Get kubeconfig content
	kubeconfig, err := GetKubeconfig(name)
	if err != nil {
		return err
	}

	// Write to file
	if err := os.WriteFile(path, []byte(kubeconfig), 0600); err != nil {
		return fmt.Errorf("failed to write kubeconfig: %v", err)
	}

	return nil
}

// IsKindInstalled checks if KIND is installed
func IsKindInstalled() bool {
	_, err := exec.LookPath("kind")
	return err == nil
}
