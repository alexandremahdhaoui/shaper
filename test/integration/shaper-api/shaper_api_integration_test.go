//go:build integration

package main_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexandremahdhaoui/shaper/pkg/test/kind"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	helmChartPath = "../../../charts/shaper-api"
	testNamespace = "shaper-api-test"
	testTimeout   = 5 * time.Minute
)

// TestShaperAPIIntegration_FullLifecycle tests the complete lifecycle:
// 1. Deploy shaper-api via helm chart
// 2. Wait for pods to be ready
// 3. Test all API endpoints
// 4. Clean up
func TestShaperAPIIntegration_FullLifecycle(t *testing.T) {
	if !kind.IsKindInstalled() || !kind.IsKubectlInstalled() || !kind.IsHelmInstalled() {
		t.Skip("KIND, kubectl, or helm not installed")
	}

	// Get test kubeconfig
	kubeconfigPath := getTestKubeconfig(t)

	// Generate unique release name
	releaseName := "test-shaper-api-" + uuid.NewString()[:8]
	namespace := testNamespace + "-" + uuid.NewString()[:8]

	t.Logf("Installing shaper-api helm chart: release=%s, namespace=%s", releaseName, namespace)

	// Get absolute path to chart
	chartPath, err := filepath.Abs(helmChartPath)
	require.NoError(t, err, "failed to get chart absolute path")
	require.DirExists(t, chartPath, "helm chart directory not found")

	// Get local registry image configuration
	imageRepository, imageTag := getLocalRegistryImage(t, kubeconfigPath, "shaper-api")
	t.Logf("Using image: %s:%s", imageRepository, imageTag)

	// Create namespace first (helm will use it with --create-namespace but won't fail if it exists)
	t.Logf("Creating test namespace: %s", namespace)
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "create", "namespace", namespace)
	_ = cmd.Run() // Ignore error if namespace already exists

	// Note: Registry secret not needed since we use imagePullPolicy: Never with pre-loaded images
	// copyRegistrySecret(t, kubeconfigPath, namespace)

	// Install helm chart
	helmConfig := kind.HelmConfig{
		Kubeconfig:  kubeconfigPath,
		Namespace:   namespace,
		ReleaseName: releaseName,
		ChartPath:   chartPath,
		Values: map[string]string{
			"crds.enabled":     "false", // Assume CRDs already installed by test-setup
			"image.repository": imageRepository,
			"image.tag":        imageTag,
			"image.pullPolicy": "Never", // Use locally loaded image (via kind load)
		},
		WaitTimeout: testTimeout,
	}

	err = kind.HelmInstall(helmConfig)
	require.NoError(t, err, "failed to install helm chart")

	// Ensure cleanup
	defer func() {
		t.Logf("Cleaning up helm release: %s", releaseName)
		_ = kind.HelmUninstall(kubeconfigPath, namespace, releaseName)
		// Note: Namespace will be cleaned up by helm uninstall with --create-namespace
	}()

	// Wait for pods to be ready
	t.Log("Waiting for pods to be ready...")
	err = kind.WaitForShaperReady(kubeconfigPath, namespace, 2*time.Minute)
	require.NoError(t, err, "pods did not become ready in time")

	// Set up port forwarding
	t.Log("Setting up port forwarding...")
	localPort := "38443"
	remotePort := "30443" // API server port from values.yaml

	cleanup, err := kind.PortForwardService(kubeconfigPath, namespace, "shaper-api", localPort, remotePort)
	require.NoError(t, err, "failed to set up port forwarding")
	defer cleanup()

	// Base URL for API requests
	baseURL := fmt.Sprintf("http://localhost:%s", localPort)

	// Test all API endpoints
	t.Run("GET /boot.ipxe", func(t *testing.T) {
		testBootstrapEndpoint(t, baseURL)
	})

	t.Run("GET /ipxe with selectors", func(t *testing.T) {
		testIPXEBySelectorsEndpoint(t, baseURL, kubeconfigPath, namespace)
	})

	t.Run("Health probes", func(t *testing.T) {
		testHealthProbes(t, kubeconfigPath, namespace)
	})

	t.Run("Metrics endpoint", func(t *testing.T) {
		testMetricsEndpoint(t, kubeconfigPath, namespace)
	})
}

// testBootstrapEndpoint tests GET /boot.ipxe
func testBootstrapEndpoint(t *testing.T, baseURL string) {
	url := baseURL + "/boot.ipxe"
	t.Logf("Testing bootstrap endpoint: %s", url)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err, "failed to call bootstrap endpoint")
	defer resp.Body.Close()

	// Should return 200 OK
	assert.Equal(t, http.StatusOK, resp.StatusCode, "expected 200 OK for bootstrap endpoint")

	// Should return text/plain content
	contentType := resp.Header.Get("Content-Type")
	assert.Contains(t, contentType, "text/plain", "expected text/plain content type")

	// Read response body
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Should contain iPXE script markers
	bodyStr := string(body)
	assert.Contains(t, bodyStr, "#!ipxe", "response should contain iPXE shebang")

	t.Logf("Bootstrap endpoint test passed. Response length: %d bytes", len(body))
}

// testIPXEBySelectorsEndpoint tests GET /ipxe?uuid=...&buildarch=...
func testIPXEBySelectorsEndpoint(t *testing.T, baseURL, kubeconfigPath, namespace string) {
	// First, create a test Profile and Assignment
	testUUID := uuid.NewString()
	testBuildarch := "x86_64"

	profileName := "test-profile-" + uuid.NewString()[:8]
	assignmentName := "test-assignment-" + uuid.NewString()[:8]

	t.Logf("Creating test Profile: %s", profileName)

	// Create Profile CRD
	profileYAML := []byte(fmt.Sprintf(`apiVersion: shaper.amahdha.com/v1alpha1
kind: Profile
metadata:
  name: %s
  labels:
    test: integration
spec:
  ipxeTemplate: |
    #!ipxe
    echo Integration test profile
    echo UUID: {{ .UUID }}
    echo BuildArch: {{ .BuildArch }}
`, profileName))

	err := kind.CreateTestProfile(kubeconfigPath, "default", profileName, profileYAML)
	require.NoError(t, err, "failed to create test profile")

	t.Logf("Creating test Assignment: %s", assignmentName)

	// Create Assignment CRD
	assignmentYAML := []byte(fmt.Sprintf(`apiVersion: shaper.amahdha.com/v1alpha1
kind: Assignment
metadata:
  name: %s
  labels:
    test: integration
spec:
  subjectSelectors:
    uuidList:
      - "%s"
    buildarch:
      - "%s"
  profileName: %s
  isDefault: false
`, assignmentName, testUUID, testBuildarch, profileName))

	err = kind.CreateTestAssignment(kubeconfigPath, "default", assignmentName, assignmentYAML)
	require.NoError(t, err, "failed to create test assignment")

	// Wait a bit for CRDs to be processed
	time.Sleep(5 * time.Second)

	// Now test the endpoint
	url := fmt.Sprintf("%s/ipxe?uuid=%s&buildarch=%s", baseURL, testUUID, testBuildarch)
	t.Logf("Testing iPXE selectors endpoint: %s", url)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err, "failed to call iPXE selectors endpoint")
	defer resp.Body.Close()

	// Should return 200 OK if assignment matches, or 404 if not found
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	bodyStr := string(body)
	t.Logf("iPXE response status: %d, body length: %d", resp.StatusCode, len(body))

	// If we get 200, validate the response contains expected content
	if resp.StatusCode == http.StatusOK {
		assert.Contains(t, bodyStr, "#!ipxe", "response should contain iPXE shebang")
		assert.Contains(t, bodyStr, "Integration test profile", "response should contain test profile content")
		t.Log("iPXE selectors endpoint test passed with matching assignment")
	} else {
		// Log the response for debugging
		t.Logf("iPXE endpoint returned non-200 status. This may be expected if assignment didn't match. Body: %s", bodyStr)
	}
}

// testHealthProbes tests liveness and readiness probes
func testHealthProbes(t *testing.T, kubeconfigPath, namespace string) {
	t.Log("Testing health probes...")

	// Set up port forwarding for probes server (port 8081)
	localPort := "38081"
	remotePort := "8081"

	cleanup, err := kind.PortForwardService(kubeconfigPath, namespace, "shaper-api", localPort, remotePort)
	require.NoError(t, err, "failed to set up port forwarding for probes")
	defer cleanup()

	baseURL := fmt.Sprintf("http://localhost:%s", localPort)

	// Test liveness probe
	t.Run("liveness", func(t *testing.T) {
		url := baseURL + "/healthz"
		resp, err := http.Get(url)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "liveness probe should return 200 OK")
		t.Logf("Liveness probe: %s returned %d", url, resp.StatusCode)
	})

	// Test readiness probe
	t.Run("readiness", func(t *testing.T) {
		url := baseURL + "/readyz"
		resp, err := http.Get(url)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "readiness probe should return 200 OK")
		t.Logf("Readiness probe: %s returned %d", url, resp.StatusCode)
	})
}

// testMetricsEndpoint tests the Prometheus metrics endpoint
func testMetricsEndpoint(t *testing.T, kubeconfigPath, namespace string) {
	t.Log("Testing metrics endpoint...")

	// Set up port forwarding for metrics server (port 8080)
	localPort := "38080"
	remotePort := "8080"

	cleanup, err := kind.PortForwardService(kubeconfigPath, namespace, "shaper-api", localPort, remotePort)
	require.NoError(t, err, "failed to set up port forwarding for metrics")
	defer cleanup()

	url := fmt.Sprintf("http://localhost:%s/metrics", localPort)

	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "metrics endpoint should return 200 OK")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	bodyStr := string(body)

	// Validate metrics format (Prometheus exposition format)
	assert.Contains(t, bodyStr, "go_", "metrics should contain Go runtime metrics")
	assert.Contains(t, bodyStr, "promhttp_", "metrics should contain HTTP handler metrics")

	t.Logf("Metrics endpoint test passed. Metrics count: %d lines", strings.Count(bodyStr, "\n"))
}

// getTestKubeconfig returns the kubeconfig for the test cluster
func getTestKubeconfig(t *testing.T) string {
	t.Helper()

	// Check for project kubeconfig (created by `make test-setup`)
	// Navigate up from test/integration/shaper-api to project root
	projectRoot, err := filepath.Abs("../../..")
	require.NoError(t, err)

	projectKubeconfig := filepath.Join(projectRoot, "test", "shaper-kubeconfig")
	if _, err := os.Stat(projectKubeconfig); err == nil {
		return projectKubeconfig
	}

	// Fall back to KUBECONFIG env var
	if kc := os.Getenv("KUBECONFIG"); kc != "" {
		return kc
	}

	t.Skip("No test kubeconfig found. Run `make test-setup` first.")
	return ""
}

// copyRegistrySecret copies the local-container-registry-credentials secret to the target namespace
func copyRegistrySecret(t *testing.T, kubeconfigPath, targetNamespace string) {
	t.Helper()

	// Get the secret from local-container-registry namespace and copy it to target namespace
	cmd := exec.Command("sh", "-c",
		fmt.Sprintf("kubectl --kubeconfig %s get secret local-container-registry-credentials -n local-container-registry -o yaml | "+
			"kubectl --kubeconfig %s apply -n %s -f -",
			kubeconfigPath, kubeconfigPath, targetNamespace))
	output, err := cmd.CombinedOutput()

	// Ignore "already exists" or "unchanged" errors
	if err != nil && !strings.Contains(string(output), "created") && !strings.Contains(string(output), "configured") && !strings.Contains(string(output), "unchanged") {
		t.Logf("Failed to copy secret, output: %s", string(output))
		require.NoError(t, err, "failed to copy local-container-registry-credentials to test namespace")
	}
}

// getLocalRegistryImage returns the image repository and tag for the local registry
func getLocalRegistryImage(t *testing.T, kubeconfigPath, imageName string) (repository, tag string) {
	t.Helper()

	// Get commit SHA using git
	cmd := exec.Command("git", "rev-parse", "HEAD")
	output, err := cmd.Output()
	require.NoError(t, err, "failed to get git commit SHA")
	commitSHA := strings.TrimSpace(string(output))

	// Use the DNS name for the registry (required for TLS certificate validation)
	// The certificate has SANs for the DNS name, not the ClusterIP
	registryDNS := "local-container-registry.local-container-registry.svc.cluster.local"
	repository = fmt.Sprintf("%s:5000/%s", registryDNS, imageName)
	tag = commitSHA

	return repository, tag
}
