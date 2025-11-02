package kind

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

var (
	ErrKubeconfigRequired  = errors.New("kubeconfig path is required")
	ErrNamespaceRequired   = errors.New("namespace is required")
	ErrApplyCRD            = errors.New("failed to apply CRD")
	ErrApplyDeployment     = errors.New("failed to apply deployment")
	ErrWaitForReady        = errors.New("timeout waiting for pods to be ready")
	ErrKubectlNotInstalled = errors.New("kubectl command not found - please install kubectl")
	ErrCheckPodStatus      = errors.New("failed to check pod status")
)

// DeployConfig contains shaper deployment configuration
type DeployConfig struct {
	Kubeconfig     string        // Path to kubeconfig file
	Namespace      string        // Kubernetes namespace
	CRDPaths       []string      // Paths to CRD YAML files
	DeploymentPath string        // Path to deployment YAML
	WaitTimeout    time.Duration // Timeout for waiting for pods
}

// DeployShaperToKIND deploys shaper to KIND cluster
func DeployShaperToKIND(config DeployConfig) error {
	if config.Kubeconfig == "" {
		return ErrKubeconfigRequired
	}
	if config.Namespace == "" {
		return ErrNamespaceRequired
	}

	// Check if kubectl is installed
	if !IsKubectlInstalled() {
		return ErrKubectlNotInstalled
	}

	// Create namespace if it doesn't exist
	if err := CreateNamespace(config.Kubeconfig, config.Namespace); err != nil {
		return err
	}

	// Apply CRDs
	if len(config.CRDPaths) > 0 {
		if err := CreateCRDs(config.Kubeconfig, config.CRDPaths); err != nil {
			return err
		}
	}

	// Apply deployment if provided
	if config.DeploymentPath != "" {
		if err := applyManifest(config.Kubeconfig, config.Namespace, config.DeploymentPath); err != nil {
			return err
		}

		// Wait for pods to be ready
		timeout := config.WaitTimeout
		if timeout == 0 {
			timeout = 2 * time.Minute
		}
		if err := WaitForShaperReady(config.Kubeconfig, config.Namespace, timeout); err != nil {
			return err
		}
	}

	return nil
}

// CreateCRDs applies CRD definitions
func CreateCRDs(kubeconfig string, crdPaths []string) error {
	if kubeconfig == "" {
		return ErrKubeconfigRequired
	}

	for _, crdPath := range crdPaths {
		// Check if path exists
		if _, err := os.Stat(crdPath); err != nil {
			return fmt.Errorf("CRD path does not exist: %s: %v", crdPath, err)
		}

		// Apply CRD
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "apply", "-f", crdPath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%w: %s: %v, output: %s", ErrApplyCRD, crdPath, err, string(output))
		}
	}

	return nil
}

// WaitForShaperReady waits for shaper pods to be ready
func WaitForShaperReady(kubeconfig, namespace string, timeout time.Duration) error {
	if kubeconfig == "" {
		return ErrKubeconfigRequired
	}
	if namespace == "" {
		return ErrNamespaceRequired
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Use kubectl wait for pods to be ready
	// Wait for any pod with label app=shaper or similar
	// Since we don't know the exact label, we'll use a generic approach
	cmd := exec.CommandContext(ctx,
		"kubectl", "--kubeconfig", kubeconfig,
		"-n", namespace,
		"wait", "--for=condition=ready",
		"pod", "--all",
		"--timeout", timeout.String(),
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if it's a timeout
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("%w: %v", ErrWaitForReady, ctx.Err())
		}
		return fmt.Errorf("%w: %v, output: %s", ErrWaitForReady, err, string(output))
	}

	return nil
}

// ApplyManifest applies a Kubernetes manifest file
func ApplyManifest(kubeconfig, namespace, manifestPath string) error {
	return applyManifest(kubeconfig, namespace, manifestPath)
}

// applyManifest is the internal implementation
func applyManifest(kubeconfig, namespace, manifestPath string) error {
	if kubeconfig == "" {
		return ErrKubeconfigRequired
	}

	// Check if manifest exists
	if _, err := os.Stat(manifestPath); err != nil {
		return fmt.Errorf("manifest path does not exist: %s: %v", manifestPath, err)
	}

	// Apply manifest
	args := []string{"--kubeconfig", kubeconfig, "apply", "-f", manifestPath}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	cmd := exec.Command("kubectl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s: %v, output: %s", ErrApplyDeployment, manifestPath, err, string(output))
	}

	return nil
}

// CreateNamespace creates a Kubernetes namespace if it doesn't exist
func CreateNamespace(kubeconfig, namespace string) error {
	// Check if namespace exists
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "get", "namespace", namespace)
	if err := cmd.Run(); err == nil {
		// Namespace exists
		return nil
	}

	// Create namespace
	cmd = exec.Command("kubectl", "--kubeconfig", kubeconfig, "create", "namespace", namespace)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create namespace: %v, output: %s", err, string(output))
	}

	return nil
}

// IsKubectlInstalled checks if kubectl is installed
func IsKubectlInstalled() bool {
	_, err := exec.LookPath("kubectl")
	return err == nil
}

// GetPodStatus gets the status of pods in a namespace
func GetPodStatus(kubeconfig, namespace string) (string, error) {
	if kubeconfig == "" {
		return "", ErrKubeconfigRequired
	}
	if namespace == "" {
		return "", ErrNamespaceRequired
	}

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "-n", namespace, "get", "pods")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrCheckPodStatus, err)
	}

	return string(output), nil
}

// CreateTestProfile creates a test Profile CRD
func CreateTestProfile(kubeconfig, namespace, name string, profileYAML []byte) error {
	if kubeconfig == "" {
		return ErrKubeconfigRequired
	}

	// Write profile to temp file
	tempFile := filepath.Join(os.TempDir(), fmt.Sprintf("profile-%s.yaml", name))
	if err := os.WriteFile(tempFile, profileYAML, 0644); err != nil {
		return fmt.Errorf("failed to write profile YAML: %v", err)
	}
	defer os.Remove(tempFile)

	// Apply profile
	return applyManifest(kubeconfig, namespace, tempFile)
}

// CreateTestAssignment creates a test Assignment CRD
func CreateTestAssignment(kubeconfig, namespace, name string, assignmentYAML []byte) error {
	if kubeconfig == "" {
		return ErrKubeconfigRequired
	}

	// Write assignment to temp file
	tempFile := filepath.Join(os.TempDir(), fmt.Sprintf("assignment-%s.yaml", name))
	if err := os.WriteFile(tempFile, assignmentYAML, 0644); err != nil {
		return fmt.Errorf("failed to write assignment YAML: %v", err)
	}
	defer os.Remove(tempFile)

	// Apply assignment
	return applyManifest(kubeconfig, namespace, tempFile)
}

// DeleteManifest deletes resources from a manifest file
func DeleteManifest(kubeconfig, namespace, manifestPath string) error {
	if kubeconfig == "" {
		return ErrKubeconfigRequired
	}

	// Check if manifest exists
	if _, err := os.Stat(manifestPath); err != nil {
		// File doesn't exist, nothing to delete
		return nil
	}

	// Delete manifest
	args := []string{"--kubeconfig", kubeconfig, "delete", "-f", manifestPath, "--ignore-not-found"}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	cmd := exec.Command("kubectl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete manifest: %v, output: %s", err, string(output))
	}

	return nil
}
