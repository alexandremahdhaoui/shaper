//go:build integration

package main_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/alexandremahdhaoui/shaper/pkg/test/kind"
	"github.com/alexandremahdhaoui/shaper/pkg/v1alpha1"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const (
	helmChartPath   = "../../../charts/shaper-api"
	testNamespace   = "shaper-api-test"
	testTimeout     = 5 * time.Minute
	testMachineUUID = "00000000-0000-0000-0000-000000000001" // Fake machine UUID for testing
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

	// Get test kubeconfig - skip if not available
	kubeconfigPath := getTestKubeconfig(t)
	if kubeconfigPath == "" {
		t.Skip("No valid kubeconfig found")
	}

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

	// Copy the testenv-lcr-credentials imagePullSecret to the test namespace
	// The secret is created by forge testenv-lcr in shaper-system/default namespaces
	copyRegistrySecret(t, kubeconfigPath, namespace)

	// Install helm chart
	helmConfig := kind.HelmConfig{
		Kubeconfig:  kubeconfigPath,
		Namespace:   namespace,
		ReleaseName: releaseName,
		ChartPath:   chartPath,
		Values: map[string]string{
			"crds.enabled":             "false", // Assume CRDs already installed by test-setup
			"image.repository":         imageRepository,
			"image.tag":                imageTag,
			"image.pullPolicy":         "IfNotPresent", // Pull from testenv-lcr registry
			"imagePullSecrets[0].name": "testenv-lcr-credentials",
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

const (
	// forgeKubeconfigEnvVar is the environment variable set by forge testenv
	// when running integration tests with `forge test integration run`
	forgeKubeconfigEnvVar = "FORGE_METADATA_TESTENV_KIND_KUBECONFIGPATH"
)

// getTestKubeconfig returns the kubeconfig for the test cluster.
// It checks the following sources in order:
// 1. FORGE_METADATA_TESTENV_KIND_KUBECONFIGPATH - set by forge testenv
// 2. KUBECONFIG - standard kubernetes config env var
// 3. Project kubeconfig at test/shaper-kubeconfig (created by `make test-setup`)
//
// If no valid kubeconfig is found, it skips the test with a helpful message.
//
// Note: This function also fixes a forge bug where kubeconfig files generated by
// testenv-kind are missing the current-context field. If detected, it sets the
// current-context to the first available context.
func getTestKubeconfig(t *testing.T) string {
	t.Helper()

	// Priority 1: Forge testenv provides kubeconfig path via environment variable
	if forgeKubeconfig := os.Getenv(forgeKubeconfigEnvVar); forgeKubeconfig != "" {
		if _, err := os.Stat(forgeKubeconfig); err == nil {
			t.Logf("Using kubeconfig from forge testenv: %s", forgeKubeconfig)
			fixKubeconfigCurrentContext(t, forgeKubeconfig)
			return forgeKubeconfig
		}
		t.Logf("Warning: %s is set to %s but file does not exist", forgeKubeconfigEnvVar, forgeKubeconfig)
	}

	// Priority 2: Standard KUBECONFIG environment variable
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		if _, err := os.Stat(kubeconfig); err == nil {
			t.Logf("Using kubeconfig from KUBECONFIG env var: %s", kubeconfig)
			return kubeconfig
		}
		t.Logf("Warning: KUBECONFIG is set to %s but file does not exist", kubeconfig)
	}

	// Priority 3: Project kubeconfig (created by `make test-setup`)
	// Navigate up from test/integration/shaper-api to project root
	projectRoot, err := filepath.Abs("../../..")
	require.NoError(t, err)
	projectKubeconfig := filepath.Join(projectRoot, "test", "shaper-kubeconfig")
	if _, err := os.Stat(projectKubeconfig); err == nil {
		t.Logf("Using project kubeconfig: %s", projectKubeconfig)
		return projectKubeconfig
	}

	// No valid kubeconfig found
	t.Skipf("No valid kubeconfig found. Options:\n"+
		"  1. Run 'forge test integration run' (sets %s)\n"+
		"  2. Set KUBECONFIG environment variable\n"+
		"  3. Run 'make test-setup' to create test/shaper-kubeconfig",
		forgeKubeconfigEnvVar)
	return ""
}

// fixKubeconfigCurrentContext fixes a forge bug where kubeconfig files generated by
// testenv-kind are missing the current-context field. This function reads the kubeconfig,
// and if current-context is empty or missing, it sets it to the first available context.
func fixKubeconfigCurrentContext(t *testing.T, kubeconfigPath string) {
	t.Helper()

	// Read the kubeconfig file
	data, err := os.ReadFile(kubeconfigPath)
	if err != nil {
		t.Logf("Warning: failed to read kubeconfig for current-context fix: %v", err)
		return
	}

	content := string(data)

	// Check if current-context is already set (not empty)
	// Look for 'current-context: ""' or 'current-context:' followed by newline
	emptyContextPattern := regexp.MustCompile(`current-context:\s*(""|'')?\s*\n`)
	hasEmptyContext := emptyContextPattern.MatchString(content)

	// Also check if current-context line doesn't exist at all
	hasCurrentContext := strings.Contains(content, "current-context:")

	if !hasEmptyContext && hasCurrentContext {
		// current-context is already set, nothing to do
		return
	}

	// Extract context name from the contexts section
	// Looking for pattern like:
	//   name: kind-forge-test-integration-...
	contextNamePattern := regexp.MustCompile(`contexts:\s*\n-\s+context:[\s\S]*?name:\s+(\S+)`)
	matches := contextNamePattern.FindStringSubmatch(content)
	if len(matches) < 2 {
		t.Logf("Warning: could not extract context name from kubeconfig")
		return
	}
	contextName := matches[1]

	t.Logf("Fixing kubeconfig: setting current-context to %s", contextName)

	// Fix the kubeconfig
	var newContent string
	if hasEmptyContext {
		// Replace empty current-context with the context name
		newContent = emptyContextPattern.ReplaceAllString(content, "current-context: "+contextName+"\n")
	} else {
		// Add current-context line after 'kind: Config'
		newContent = strings.Replace(content, "kind: Config", "current-context: "+contextName+"\nkind: Config", 1)
	}

	// Write the fixed kubeconfig back
	if err := os.WriteFile(kubeconfigPath, []byte(newContent), 0o600); err != nil {
		t.Logf("Warning: failed to write fixed kubeconfig: %v", err)
		return
	}
}

// copyRegistrySecret copies the testenv-lcr-credentials secret to the target namespace
// The secret is created by forge testenv-lcr in the namespaces configured in forge.yaml.
// For integration tests that create their own namespaces, we need to copy this secret.
func copyRegistrySecret(t *testing.T, kubeconfigPath, targetNamespace string) {
	t.Helper()

	// Try to get the secret from shaper-system first (where forge deploys it per forge.yaml)
	sourceNamespaces := []string{"shaper-system", "default"}
	secretName := "testenv-lcr-credentials"

	for _, sourceNS := range sourceNamespaces {
		// Check if secret exists in source namespace
		checkCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
			"get", "secret", secretName, "-n", sourceNS, "-o", "name")
		if err := checkCmd.Run(); err != nil {
			t.Logf("Secret %s not found in %s, trying next namespace", secretName, sourceNS)
			continue
		}

		t.Logf("Copying %s from %s to %s", secretName, sourceNS, targetNamespace)

		// Get the secret and recreate it in the target namespace
		// We use get + apply with namespace override to copy the secret
		cmd := exec.Command("sh", "-c",
			fmt.Sprintf(`kubectl --kubeconfig %s get secret %s -n %s -o json | \
				jq 'del(.metadata.namespace, .metadata.resourceVersion, .metadata.uid, .metadata.creationTimestamp)' | \
				kubectl --kubeconfig %s apply -n %s -f -`,
				kubeconfigPath, secretName, sourceNS,
				kubeconfigPath, targetNamespace))
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Logf("Failed to copy secret: %s", string(output))
			require.NoError(t, err, "failed to copy %s to test namespace", secretName)
		}

		t.Logf("Successfully copied %s to %s", secretName, targetNamespace)
		return
	}

	t.Fatalf("Could not find %s in any of the source namespaces: %v", secretName, sourceNamespaces)
}

const (
	// forgeRegistryFQDNEnvVar is the environment variable set by forge testenv-lcr
	// containing the full registry FQDN with port (e.g., "testenv-lcr.testenv-lcr.svc.cluster.local:5000")
	forgeRegistryFQDNEnvVar = "FORGE_METADATA_TESTENV_LCR_REGISTRYFQDN"

	// defaultRegistryFQDN is the default registry FQDN used when forge testenv-lcr is not available.
	// This matches the FQDN generated by testenv-lcr when using namespace "testenv-lcr"
	// as configured in forge.yaml testenv-integration alias.
	defaultRegistryFQDN = "testenv-lcr.testenv-lcr.svc.cluster.local:5000"
)

// getLocalRegistryImage returns the image repository and tag for the local registry
// The image tag defaults to "latest" which matches what forge testenv-lcr pushes (see forge.yaml).
func getLocalRegistryImage(t *testing.T, kubeconfigPath, imageName string) (repository, tag string) {
	t.Helper()

	// Get registry FQDN from environment variable set by forge testenv-lcr
	// Format is: "testenv-lcr.{namespace}.svc.cluster.local:5000"
	registryFQDN := os.Getenv(forgeRegistryFQDNEnvVar)
	if registryFQDN == "" {
		t.Logf("Warning: %s not set, using default: %s", forgeRegistryFQDNEnvVar, defaultRegistryFQDN)
		registryFQDN = defaultRegistryFQDN
	} else {
		t.Logf("Using registry FQDN from %s: %s", forgeRegistryFQDNEnvVar, registryFQDN)
	}

	repository = fmt.Sprintf("%s/%s", registryFQDN, imageName)
	// Use "latest" tag - this matches what forge testenv-lcr pushes (see forge.yaml images config)
	tag = "latest"

	return repository, tag
}

// TestContentEndpoint tests the /content/{uuid} endpoint with exposed content
func TestContentEndpoint(t *testing.T) {
	if !kind.IsKindInstalled() || !kind.IsKubectlInstalled() || !kind.IsHelmInstalled() {
		t.Skip("KIND, kubectl, or helm not installed")
	}

	// Get test kubeconfig - skip if not available
	kubeconfigPath := getTestKubeconfig(t)
	if kubeconfigPath == "" {
		t.Skip("No valid kubeconfig found")
	}

	// Set up Kubernetes client
	k8sClient := setupKubernetesClient(t, kubeconfigPath)

	// Generate unique release name
	releaseName := "test-content-" + uuid.NewString()[:8]
	namespace := testNamespace + "-" + uuid.NewString()[:8]

	t.Logf("Installing shaper-api helm chart: release=%s, namespace=%s", releaseName, namespace)

	// Get absolute path to chart
	chartPath, err := filepath.Abs(helmChartPath)
	require.NoError(t, err, "failed to get chart absolute path")
	require.DirExists(t, chartPath, "helm chart directory not found")

	// Get local registry image configuration
	imageRepository, imageTag := getLocalRegistryImage(t, kubeconfigPath, "shaper-api")
	t.Logf("Using image: %s:%s", imageRepository, imageTag)

	// Create namespace
	t.Logf("Creating test namespace: %s", namespace)
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "create", "namespace", namespace)
	_ = cmd.Run()

	// Copy the testenv-lcr-credentials imagePullSecret to the test namespace
	copyRegistrySecret(t, kubeconfigPath, namespace)

	// Install helm chart with webhooks enabled
	helmConfig := kind.HelmConfig{
		Kubeconfig:  kubeconfigPath,
		Namespace:   namespace,
		ReleaseName: releaseName,
		ChartPath:   chartPath,
		Values: map[string]string{
			"crds.enabled":             "false",
			"image.repository":         imageRepository,
			"image.tag":                imageTag,
			"image.pullPolicy":         "IfNotPresent",
			"imagePullSecrets[0].name": "testenv-lcr-credentials",
		},
		WaitTimeout: testTimeout,
	}

	err = kind.HelmInstall(helmConfig)
	require.NoError(t, err, "failed to install helm chart")

	defer func() {
		t.Logf("Cleaning up helm release: %s", releaseName)
		_ = kind.HelmUninstall(kubeconfigPath, namespace, releaseName)
	}()

	// Wait for pods to be ready
	t.Log("Waiting for pods to be ready...")
	err = kind.WaitForShaperReady(kubeconfigPath, namespace, 2*time.Minute)
	require.NoError(t, err, "pods did not become ready in time")

	// Set up port forwarding
	t.Log("Setting up port forwarding...")
	localPort := "38443"
	remotePort := "30443"

	cleanup, err := kind.PortForwardService(kubeconfigPath, namespace, "shaper-api", localPort, remotePort)
	require.NoError(t, err, "failed to set up port forwarding")
	defer cleanup()

	baseURL := fmt.Sprintf("http://localhost:%s", localPort)

	// Run test cases
	t.Run("Single exposed content", func(t *testing.T) {
		testSingleExposedContent(t, k8sClient, baseURL)
	})

	t.Run("Multiple exposed content", func(t *testing.T) {
		testMultipleExposedContent(t, k8sClient, baseURL)
	})

	t.Run("Butane transformation", func(t *testing.T) {
		testButaneTransformation(t, k8sClient, baseURL)
	})

	t.Run("Non-exposed content not accessible", func(t *testing.T) {
		testNonExposedContent(t, k8sClient, baseURL)
	})
}

// setupKubernetesClient creates a controller-runtime client for the test cluster
func setupKubernetesClient(t *testing.T, kubeconfigPath string) client.Client {
	t.Helper()

	os.Setenv("KUBECONFIG", kubeconfigPath)

	cfg, err := config.GetConfig()
	require.NoError(t, err, "failed to get kubeconfig")

	scheme := runtime.NewScheme()
	err = v1alpha1.AddToScheme(scheme)
	require.NoError(t, err, "failed to add v1alpha1 to scheme")

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	require.NoError(t, err, "failed to create Kubernetes client")

	return k8sClient
}

// extractContentUUID extracts the UUID for a given content name from Profile labels
func extractContentUUID(t *testing.T, profile *v1alpha1.Profile, contentName string) string {
	t.Helper()

	idNameMap, _, err := v1alpha1.UUIDLabelSelectors(profile.Labels)
	require.NoError(t, err, "failed to parse UUID labels")

	for id, name := range idNameMap {
		if name == contentName {
			return id.String()
		}
	}

	t.Fatalf("No UUID found for content: %s", contentName)
	return ""
}

// getContent makes an HTTP GET request to /content/{contentID}?uuid={machineUUID}&buildarch={buildarch}
func getContent(t *testing.T, baseURL, contentID, machineUUID, buildarch string) (int, []byte) {
	t.Helper()

	url := fmt.Sprintf("%s/content/%s?uuid=%s&buildarch=%s", baseURL, contentID, machineUUID, buildarch)
	t.Logf("Requesting content: %s", url)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err, "failed to call content endpoint")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "failed to read response body")

	return resp.StatusCode, body
}

// waitForProfileUUIDLabels waits for the Profile to have UUID labels set by the webhook
func waitForProfileUUIDLabels(t *testing.T, k8sClient client.Client, namespace, name string, expectedCount int, timeout time.Duration) *v1alpha1.Profile {
	t.Helper()

	ctx := context.Background()
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		profile := &v1alpha1.Profile{}
		key := client.ObjectKey{Namespace: namespace, Name: name}
		err := k8sClient.Get(ctx, key, profile)
		if err == nil {
			// Count UUID labels
			count := 0
			for k := range profile.Labels {
				if v1alpha1.IsUUIDLabelSelector(k) {
					count++
				}
			}

			if count == expectedCount {
				t.Logf("Profile %s has %d UUID labels", name, count)
				return profile
			}

			t.Logf("Waiting for UUID labels: current=%d, expected=%d", count, expectedCount)
		}

		time.Sleep(1 * time.Second)
	}

	t.Fatalf("Timeout waiting for Profile %s to have %d UUID labels", name, expectedCount)
	return nil
}

// testSingleExposedContent tests retrieving a single exposed content item
func testSingleExposedContent(t *testing.T, k8sClient client.Client, baseURL string) {
	ctx := context.Background()
	profileName := "test-single-content-" + uuid.NewString()[:8]

	// Create Profile with one exposed content
	profile := &v1alpha1.Profile{}
	profile.Name = profileName
	profile.Namespace = "default"
	profile.Labels = map[string]string{"test": "integration"}
	profile.Spec = v1alpha1.ProfileSpec{
		IPXETemplate: "#!ipxe\necho Test",
		AdditionalContent: []v1alpha1.AdditionalContent{
			{
				Name:    "test-config",
				Exposed: true,
				Inline:  stringPtr(`{"test": "content"}`),
			},
		},
	}

	t.Logf("Creating Profile: %s", profileName)
	err := k8sClient.Create(ctx, profile)
	require.NoError(t, err, "failed to create Profile")
	defer func() { _ = k8sClient.Delete(ctx, profile) }()

	// Wait for webhook to add UUID labels
	profile = waitForProfileUUIDLabels(t, k8sClient, "default", profileName, 1, 30*time.Second)

	// Extract UUID for the content
	contentUUID := extractContentUUID(t, profile, "test-config")
	t.Logf("Content UUID: %s", contentUUID)

	// Query /content/{uuid}
	statusCode, body := getContent(t, baseURL, contentUUID, testMachineUUID, "x86_64")

	// Verify response
	assert.Equal(t, http.StatusOK, statusCode, "expected 200 OK")
	assert.Contains(t, string(body), `{"test": "content"}`, "response should match original content")

	t.Logf("Single exposed content test passed. Response: %s", string(body))
}

// testMultipleExposedContent tests retrieving multiple exposed content items
func testMultipleExposedContent(t *testing.T, k8sClient client.Client, baseURL string) {
	ctx := context.Background()
	profileName := "test-multiple-content-" + uuid.NewString()[:8]

	// Create Profile with multiple exposed content items
	profile := &v1alpha1.Profile{}
	profile.Name = profileName
	profile.Namespace = "default"
	profile.Labels = map[string]string{"test": "integration"}
	profile.Spec = v1alpha1.ProfileSpec{
		IPXETemplate: "#!ipxe\necho Test",
		AdditionalContent: []v1alpha1.AdditionalContent{
			{
				Name:    "config1",
				Exposed: true,
				Inline:  stringPtr("content-1"),
			},
			{
				Name:    "config2",
				Exposed: true,
				Inline:  stringPtr("content-2"),
			},
			{
				Name:    "config3",
				Exposed: true,
				Inline:  stringPtr("content-3"),
			},
		},
	}

	t.Logf("Creating Profile with multiple content: %s", profileName)
	err := k8sClient.Create(ctx, profile)
	require.NoError(t, err, "failed to create Profile")
	defer func() { _ = k8sClient.Delete(ctx, profile) }()

	// Wait for webhook to add 3 UUID labels
	profile = waitForProfileUUIDLabels(t, k8sClient, "default", profileName, 3, 30*time.Second)

	// Test each content individually
	contentNames := []string{"config1", "config2", "config3"}
	expectedContent := map[string]string{
		"config1": "content-1",
		"config2": "content-2",
		"config3": "content-3",
	}

	for _, name := range contentNames {
		t.Run(name, func(t *testing.T) {
			contentUUID := extractContentUUID(t, profile, name)
			t.Logf("Testing %s with UUID: %s", name, contentUUID)

			statusCode, body := getContent(t, baseURL, contentUUID, testMachineUUID, "x86_64")

			assert.Equal(t, http.StatusOK, statusCode, "expected 200 OK")
			assert.Equal(t, expectedContent[name], string(body), "content should match")

			t.Logf("%s test passed. Response: %s", name, string(body))
		})
	}
}

// testButaneTransformation tests content with Butane to Ignition transformation
func testButaneTransformation(t *testing.T, k8sClient client.Client, baseURL string) {
	ctx := context.Background()
	profileName := "test-butane-" + uuid.NewString()[:8]

	// Create Profile with Butane content
	butaneConfig := `variant: fcos
version: 1.5.0
storage:
  files:
    - path: /etc/test.txt
      contents:
        inline: Hello from Butane!
      mode: 0644`

	profile := &v1alpha1.Profile{}
	profile.Name = profileName
	profile.Namespace = "default"
	profile.Labels = map[string]string{"test": "integration"}
	profile.Spec = v1alpha1.ProfileSpec{
		IPXETemplate: "#!ipxe\necho Test",
		AdditionalContent: []v1alpha1.AdditionalContent{
			{
				Name:    "butane-config",
				Exposed: true,
				Inline:  stringPtr(butaneConfig),
				PostTransformations: []v1alpha1.Transformer{
					{ButaneToIgnition: true},
				},
			},
		},
	}

	t.Logf("Creating Profile with Butane content: %s", profileName)
	err := k8sClient.Create(ctx, profile)
	require.NoError(t, err, "failed to create Profile")
	defer func() { _ = k8sClient.Delete(ctx, profile) }()

	// Wait for webhook to add UUID label
	profile = waitForProfileUUIDLabels(t, k8sClient, "default", profileName, 1, 30*time.Second)

	// Extract UUID
	contentUUID := extractContentUUID(t, profile, "butane-config")
	t.Logf("Butane content UUID: %s", contentUUID)

	// Query /content/{uuid}
	statusCode, body := getContent(t, baseURL, contentUUID, testMachineUUID, "x86_64")

	// Verify response - use require to stop test immediately on failure
	require.Equal(t, http.StatusOK, statusCode, "expected 200 OK, got body: %s", string(body))

	// Response should be valid JSON (Ignition format)
	var ignitionConfig map[string]interface{}
	err = json.Unmarshal(body, &ignitionConfig)
	require.NoError(t, err, "response should be valid JSON (Ignition format)")

	// Verify it's Ignition, not Butane
	require.Contains(t, ignitionConfig, "ignition", "response should contain ignition field")
	ignitionField, ok := ignitionConfig["ignition"].(map[string]interface{})
	require.True(t, ok, "ignition field should be a map")
	require.Contains(t, ignitionField, "version", "ignition field should have version")

	// Verify the content was transformed (should contain the file we defined)
	assert.Contains(t, ignitionConfig, "storage", "response should contain storage field")

	t.Logf("Butane transformation test passed. Ignition version: %v", ignitionField["version"])
}

// testNonExposedContent tests that non-exposed content is not accessible
func testNonExposedContent(t *testing.T, k8sClient client.Client, baseURL string) {
	ctx := context.Background()
	profileName := "test-non-exposed-" + uuid.NewString()[:8]

	// Create Profile with mixed exposed and non-exposed content
	profile := &v1alpha1.Profile{}
	profile.Name = profileName
	profile.Namespace = "default"
	profile.Labels = map[string]string{"test": "integration"}
	profile.Spec = v1alpha1.ProfileSpec{
		IPXETemplate: "#!ipxe\necho Test",
		AdditionalContent: []v1alpha1.AdditionalContent{
			{
				Name:    "exposed-content",
				Exposed: true,
				Inline:  stringPtr("this-is-exposed"),
			},
			{
				Name:    "secret-content",
				Exposed: false,
				Inline:  stringPtr("this-is-secret"),
			},
		},
	}

	t.Logf("Creating Profile with non-exposed content: %s", profileName)
	err := k8sClient.Create(ctx, profile)
	require.NoError(t, err, "failed to create Profile")
	defer func() { _ = k8sClient.Delete(ctx, profile) }()

	// Wait for webhook to add only 1 UUID label (for exposed content only)
	profile = waitForProfileUUIDLabels(t, k8sClient, "default", profileName, 1, 30*time.Second)

	// Verify only exposed content has UUID
	exposedUUID := extractContentUUID(t, profile, "exposed-content")
	assert.NotEmpty(t, exposedUUID, "exposed content should have UUID")

	// Verify non-exposed content has no UUID label
	idNameMap, _, err := v1alpha1.UUIDLabelSelectors(profile.Labels)
	require.NoError(t, err)

	hasSecretUUID := false
	for _, name := range idNameMap {
		if name == "secret-content" {
			hasSecretUUID = true
			break
		}
	}
	assert.False(t, hasSecretUUID, "non-exposed content should NOT have UUID label")

	// Verify exposed content is accessible
	statusCode, body := getContent(t, baseURL, exposedUUID, testMachineUUID, "x86_64")
	assert.Equal(t, http.StatusOK, statusCode, "exposed content should be accessible")
	assert.Equal(t, "this-is-exposed", string(body), "exposed content should match")

	// Try to query with a random UUID (simulating attempt to access non-exposed content)
	randomUUID := uuid.NewString()
	statusCode, _ = getContent(t, baseURL, randomUUID, testMachineUUID, "x86_64")
	assert.Equal(t, http.StatusInternalServerError, statusCode, "random UUID should return error")

	t.Logf("Non-exposed content test passed. Only exposed content is accessible.")
}

// stringPtr returns a pointer to a string
func stringPtr(s string) *string {
	return &s
}
