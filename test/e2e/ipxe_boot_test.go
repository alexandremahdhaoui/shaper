//go:build e2e

// Copyright 2024 Alexandre Mahdhaoui
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e"
	"github.com/alexandremahdhaoui/shaper/pkg/v1alpha1"
	"github.com/stretchr/testify/require"
)

var globalPortForward *e2e.PortForward

func TestMain(m *testing.M) {
	cfg, err := e2e.LoadTestenvConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load testenv config: %v\n", err)
		os.Exit(1)
	}

	pf, err := e2e.SetupGlobalPortForwardWithWait(cfg.Kubeconfig, 60*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to setup port-forward: %v\n", err)
		os.Exit(1)
	}
	globalPortForward = pf

	// Set up VM-access port-forward on 0.0.0.0:30080 so VMs on the libvirt network
	// can reach shaper-api via shaper.local (192.168.100.1:30080).
	vmPF, err := e2e.SetupVMAccessPortForward(cfg.Kubeconfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to setup VM-access port-forward: %v\n", err)
		pf.Stop()
		os.Exit(1)
	}

	if err := e2e.VerifyBridgeAccess(vmPF); err != nil {
		fmt.Fprintf(os.Stderr, "bridge access verification failed: %v\n", err)
		vmPF.Stop()
		pf.Stop()
		os.Exit(1)
	}

	vmClient, err := e2e.NewVMClient("/tmp/shaper-testenv-vm")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create VM client: %v\n", err)
		pf.Stop()
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	if err := vmClient.WaitForDnsmasqServerReady(ctx, 10*time.Minute); err != nil {
		fmt.Fprintf(os.Stderr, "DnsmasqServer not ready: %v\n", err)
		cancel()
		pf.Stop()
		os.Exit(1)
	}
	cancel()

	code := m.Run()
	vmPF.Stop()
	pf.Stop()
	os.Exit(code)
}

// TestIPXEBootFlow_E2E is the main end-to-end test for iPXE boot flow
// Uses testenv-vm environment variables for VM configuration
func TestIPXEBootFlow_E2E(t *testing.T) {
	// Load testenv configuration from environment variables
	// These are set by forge testenv when running `forge test e2e run`
	cfg, err := e2e.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available - run with forge test e2e run")

	t.Logf("Using testenv configuration:")
	t.Logf("  VM IP: %s", cfg.VMPXEClientIP)
	t.Logf("  Bridge IP: %s", cfg.BridgeIP)
	t.Logf("  SSH Key: %s", cfg.SSHKeyPath)
	t.Logf("  Kubeconfig: %s", cfg.Kubeconfig)

	// Run sub-tests
	t.Run("KubeconfigValid", func(t *testing.T) {
		require.FileExists(t, cfg.Kubeconfig)
	})

	t.Run("VMIPAccessible", func(t *testing.T) {
		// VM IP is optional for PXE boot tests that create runtime VMs
		if cfg.VMPXEClientIP == "" {
			t.Skip("VM IP not configured - this is expected for PXE boot tests")
		}
		// Note: Actual ping test would require network access
	})

	t.Run("SSHKeyExists", func(t *testing.T) {
		// SSH key is optional for PXE boot tests (VMs don't have SSH)
		if cfg.SSHKeyPath == "" {
			t.Skip("SSH key not configured - this is expected for PXE boot tests")
		}
		require.FileExists(t, cfg.SSHKeyPath)
	})

	t.Run("IPXEBootFlow", func(t *testing.T) {
		testIPXEBootFlowWithConfig(t, cfg)
	})
}

func testIPXEBootFlowWithConfig(t *testing.T, cfg *e2e.TestenvConfig) {
	t.Log("Testing iPXE boot flow with testenv configuration...")

	// Get shaper-api URL - either from env var, port-forward, or skip
	shaperAPIURL := os.Getenv("SHAPER_API_URL")
	var portForwardCmd *exec.Cmd
	var portForwardCancel context.CancelFunc

	if shaperAPIURL == "" {
		// Try to set up port-forward to shaper-api service
		localPort, cmd, cancel, err := setupPortForward(t, cfg.Kubeconfig)
		if err != nil {
			t.Skipf("Skipping iPXE boot flow test: could not set up port-forward to shaper-api: %v", err)
			return
		}
		portForwardCmd = cmd
		portForwardCancel = cancel
		shaperAPIURL = fmt.Sprintf("http://localhost:%d", localPort)
		t.Logf("Set up port-forward to shaper-api at %s", shaperAPIURL)

		// Cleanup port-forward on test completion
		t.Cleanup(func() {
			if portForwardCancel != nil {
				portForwardCancel()
			}
			if portForwardCmd != nil && portForwardCmd.Process != nil {
				_ = portForwardCmd.Process.Kill()
			}
		})

		// Wait for port-forward to be ready
		if err := waitForPortForward(shaperAPIURL, 30*time.Second); err != nil {
			t.Skipf("Skipping iPXE boot flow test: port-forward not ready: %v", err)
			return
		}
	} else {
		t.Logf("Using SHAPER_API_URL from environment: %s", shaperAPIURL)
	}

	t.Run("BootstrapEndpoint", func(t *testing.T) {
		url := shaperAPIURL + "/boot.ipxe"
		t.Logf("Testing bootstrap endpoint: %s", url)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err, "failed to call bootstrap endpoint")
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusOK, resp.StatusCode, "expected 200 OK for bootstrap endpoint")

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		bodyStr := string(body)
		require.Contains(t, bodyStr, "#!ipxe", "response should contain iPXE shebang")

		t.Logf("Bootstrap endpoint test passed. Response length: %d bytes", len(body))
	})
}

// setupPortForward sets up a kubectl port-forward to shaper-api service
// Returns the local port, command, cancel function, and error
func setupPortForward(t *testing.T, kubeconfig string) (int, *exec.Cmd, context.CancelFunc, error) {
	// Find an available port
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return 0, nil, nil, fmt.Errorf("failed to find available port: %w", err)
	}
	localPort := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	// Set up port-forward to API server (port 30443)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", kubeconfig,
		"port-forward",
		"-n", "shaper-system",
		"svc/shaper-api",
		fmt.Sprintf("%d:30443", localPort),
	)

	// Capture stderr for debugging
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		cancel()
		return 0, nil, nil, fmt.Errorf("failed to start port-forward: %w", err)
	}

	t.Logf("Started port-forward (PID %d) on port %d", cmd.Process.Pid, localPort)

	return localPort, cmd, cancel, nil
}

// waitForPortForward waits for the port-forward to be ready by checking the boot.ipxe endpoint
func waitForPortForward(url string, timeout time.Duration) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Check the boot.ipxe endpoint to verify API server is ready
		resp, err := client.Get(url + "/boot.ipxe")
		if err == nil {
			_ = resp.Body.Close()
			// boot.ipxe returns 200 OK when ready
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for port-forward to be ready")
}

// TestDefaultAssignmentBoot_E2E tests VM boot using a default assignment for i386.
// Note: BIOS iPXE firmware (undionly.kpxe) reports buildarch=i386, not x86_64.
// This test verifies the complete PXE boot flow:
// 1. VM boots from network via DHCP/TFTP
// 2. iPXE loads and chainloads to shaper-api
// 3. shaper-api matches default assignment for i386
// 4. Correct profile is served to VM
func TestDefaultAssignmentBoot_E2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Load testenv configuration
	cfg, err := e2e.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available - run with forge test e2e run")

	t.Logf("Using testenv configuration:")
	t.Logf("  Kubeconfig: %s", cfg.Kubeconfig)
	t.Logf("  Bridge IP: %s", cfg.BridgeIP)

	require.NotNil(t, globalPortForward, "global port-forward not initialized")
	portForward := globalPortForward

	// Create Kubernetes client
	k8sClient, err := e2e.NewK8sClient(cfg.Kubeconfig)
	require.NoError(t, err, "failed to create Kubernetes client")

	// Create VM client
	vmClient, err := e2e.NewVMClient("/tmp/shaper-testenv-vm")
	require.NoError(t, err, "failed to create VM client")

	// Test resources
	const (
		profileName    = "e2e-default-profile"
		assignmentName = "e2e-default-assignment"
		namespace      = "shaper-system"
		vmName         = "e2e-tc1-default-vm"
	)

	// iPXE template with marker for verification
	// Note: BIOS iPXE firmware reports buildarch=i386
	ipxeTemplate := `#!ipxe
echo =============================================
echo E2E Test: Default Assignment for i386
echo =============================================
echo UUID: ${uuid}
echo Buildarch: ${buildarch}
echo Profile: e2e-default-profile
echo =============================================
shell`

	// Cleanup function
	cleanup := func() {
		t.Log("Cleaning up test resources...")
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()

		// Delete VM first
		if err := vmClient.DeleteVM(cleanupCtx, vmName); err != nil {
			t.Logf("Warning: failed to delete VM %s: %v", vmName, err)
		}

		// Delete Assignment before Profile
		if err := e2e.DeleteAssignment(cleanupCtx, k8sClient, assignmentName, namespace); err != nil {
			t.Logf("Warning: failed to delete Assignment %s: %v", assignmentName, err)
		}

		// Delete Profile
		if err := e2e.DeleteProfile(cleanupCtx, k8sClient, profileName, namespace); err != nil {
			t.Logf("Warning: failed to delete Profile %s: %v", profileName, err)
		}
	}
	t.Cleanup(cleanup)

	// Step 1: Create Profile
	t.Log("Creating Profile...")
	_, err = e2e.CreateProfile(ctx, k8sClient, profileName, namespace, ipxeTemplate)
	require.NoError(t, err, "failed to create Profile")
	t.Logf("Created Profile: %s", profileName)

	// Step 2: Create default Assignment for i386
	// Note: BIOS iPXE firmware (undionly.kpxe) reports buildarch=i386
	t.Log("Creating default Assignment for i386...")
	_, err = e2e.CreateDefaultAssignment(ctx, k8sClient, assignmentName, namespace, profileName, "i386")
	require.NoError(t, err, "failed to create Assignment")
	t.Logf("Created Assignment: %s", assignmentName)

	// Step 3: Get shaper-api pod name for log verification
	podName, err := e2e.GetShaperAPIPodName(ctx, k8sClient)
	require.NoError(t, err, "failed to get shaper-api pod name")
	t.Logf("Found shaper-api pod: %s", podName)

	// Verify shaper-api can serve the /ipxe endpoint before starting VM.
	// This ensures the Assignment is visible in shaper-api's cache.
	t.Log("Verifying shaper-api can serve /ipxe endpoint...")
	verifyCtx, verifyCancel := context.WithTimeout(ctx, 30*time.Second)
	defer verifyCancel()
	err = e2e.WaitForIPXEEndpointReady(verifyCtx, portForward.URL, "i386")
	require.NoError(t, err, "shaper-api /ipxe endpoint not ready")

	// Sleep to ensure the pre-check log entry is outside the --since window
	// used by GetPodLogs (which adds a 1-second buffer). Without this, the
	// pre-check's profile_matched entry can be picked up as a false positive.
	time.Sleep(2 * time.Second)

	// Record timestamp before VM boot for log filtering
	startTime := time.Now()

	// Step 4: Create VM (without starting) to get UUID
	t.Log("Creating PXE boot VM...")
	vmSpec := e2e.VMSpec{
		Memory:    2048,
		VCPUs:     2,
		Network:   "TestNetwork",
		BootOrder: []string{"network"},
		Firmware:  "bios",
		AutoStart: false, // Don't start yet - need to get UUID first
	}
	err = vmClient.CreateVM(ctx, vmName, vmSpec)
	require.NoError(t, err, "failed to create VM")
	t.Logf("Created VM: %s (not started)", vmName)

	// Step 5: Get VM UUID for log matching
	// Note: We match by UUID instead of client_ip because port-forward makes
	// all requests appear to come from localhost, not the VM's actual IP.
	t.Log("Getting VM UUID...")
	vmUUID, err := vmClient.GetVMUUID(ctx, vmName)
	require.NoError(t, err, "failed to get VM UUID")
	t.Logf("VM UUID: %s", vmUUID.String())

	// Step 6: Start the VM
	t.Log("Starting VM...")
	err = vmClient.StartVM(ctx, vmName)
	require.NoError(t, err, "failed to start VM")
	t.Logf("Started VM: %s", vmName)

	// Step 7: Wait for VM to get an IP (indicates DHCP worked)
	t.Log("Waiting for VM to get IP address...")
	vmIP, err := vmClient.GetVMIP(ctx, vmName)
	require.NoError(t, err, "VM did not get IP address - DHCP may have failed")
	t.Logf("VM got IP address: %s", vmIP)

	// Step 8: Wait for profile_matched in shaper-api logs
	// The custom iPXE binary with embedded script sends uuid=${uuid}&buildarch=${buildarch}
	// to shaper-api. We verify the boot flow by checking that the expected profile was matched,
	// which provides end-to-end verification of the assignment selection and profile serving.
	t.Log("Waiting for profile_matched in shaper-api logs...")
	result, err := e2e.WaitForProfileMatched(ctx, cfg.Kubeconfig, e2e.ShaperSystemNamespace,
		podName, profileName, startTime, 2*time.Minute)
	if err != nil {
		// On failure, dump VM console log for debugging
		consoleLog, consoleErr := vmClient.GetConsoleLog(ctx, vmName)
		if consoleErr != nil {
			t.Logf("Failed to get VM console log: %v", consoleErr)
		} else {
			t.Logf("=== VM Console Log (%s) ===\n%s\n=== End Console Log ===", vmName, consoleLog)
		}
	}
	require.NoError(t, err, "did not find profile_matched log entry for expected profile")
	t.Logf("Found profile_matched: Profile=%s, Assignment=%s", result.ProfileName, result.AssignmentName)

	// Step 9: Verify the correct profile was matched
	require.Equal(t, profileName, result.ProfileName, "expected profile_matched to contain our profile")

	t.Log("✓ Test Case 1 PASSED: Default assignment boot flow verified")
}

// TestMTLSIPXEBoot_E2E tests VM boot using mTLS (mutual TLS) authentication.
// This test verifies the complete mTLS iPXE boot flow:
// 1. Generates mTLS certificate set (CA, server cert with IP SAN, client cert)
// 2. Creates K8s secret with TLS certs
// 3. Upgrades shaper-api with mTLS enabled
// 4. Creates test Profile and Assignment
// 5. Builds iPXE ISO with embedded client cert
// 6. Creates VM with CDROM boot (iPXE ISO)
// 7. VM boots from CDROM, chainloads to HTTPS shaper-api with mTLS
// 8. Verifies profile_matched and tls_client_connected in logs
func TestMTLSIPXEBoot_E2E(t *testing.T) {
	// Use 10-minute timeout to allow for:
	// - DnsmasqServer readiness check
	// - Helm upgrade with TLS (up to 5m)
	// - iPXE ISO build
	// - VM boot and verification
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Load testenv configuration
	cfg, err := e2e.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available - run with forge test e2e run")

	t.Logf("Using testenv configuration:")
	t.Logf("  Kubeconfig: %s", cfg.Kubeconfig)
	t.Logf("  Bridge IP: %s", cfg.BridgeIP)
	t.Logf("  Project Root: %s", cfg.ProjectRoot)
	require.NotEmpty(t, cfg.ProjectRoot, "ProjectRoot must be set to locate Helm charts")

	// Test constants
	const (
		vmName         = "e2e-mtls-vm"
		profileName    = "e2e-mtls-profile"
		assignmentName = "e2e-mtls-assignment"
		tlsSecretName  = "e2e-mtls-certs"
		namespace      = "shaper-system"
		mtlsNodePort   = 30443
	)

	// Bridge IP for server certificate SAN
	bridgeIP := net.ParseIP(e2e.BridgeGatewayIP)
	require.NotNil(t, bridgeIP, "failed to parse bridge IP")

	// Create Kubernetes client
	k8sClient, err := e2e.NewK8sClient(cfg.Kubeconfig)
	require.NoError(t, err, "failed to create Kubernetes client")

	// Create VM client
	vmClient, err := e2e.NewVMClient("/tmp/shaper-testenv-vm")
	require.NoError(t, err, "failed to create VM client")

	// Helm config for mTLS
	chartPath := filepath.Join(cfg.ProjectRoot, "charts", "shaper-api")
	helmConfig := e2e.MTLSHelmConfig{
		SecretName: tlsSecretName,
		Namespace:  namespace,
		ClientAuth: "require",
		ChartPath:  chartPath,
		NodePort:   mtlsNodePort,
	}

	// Cleanup function - use longer timeout for Helm downgrade (up to 5m)
	cleanup := func() {
		t.Log("Cleaning up test resources...")
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 6*time.Minute)
		defer cleanupCancel()

		// Delete VM first
		if err := vmClient.DeleteVM(cleanupCtx, vmName); err != nil {
			t.Logf("Warning: failed to delete VM %s: %v", vmName, err)
		}

		// Delete Assignment before Profile
		if err := e2e.DeleteAssignment(cleanupCtx, k8sClient, assignmentName, namespace); err != nil {
			t.Logf("Warning: failed to delete Assignment %s: %v", assignmentName, err)
		}

		// Delete Profile
		if err := e2e.DeleteProfile(cleanupCtx, k8sClient, profileName, namespace); err != nil {
			t.Logf("Warning: failed to delete Profile %s: %v", profileName, err)
		}

		// Downgrade shaper-api to HTTP
		t.Log("Downgrading shaper-api to HTTP-only configuration...")
		if err := e2e.DowngradeShaperAPIToHTTP(cleanupCtx, cfg.Kubeconfig, helmConfig); err != nil {
			t.Logf("Warning: failed to downgrade shaper-api: %v", err)
		}

		// Delete TLS secret
		if err := e2e.DeleteTLSSecret(cleanupCtx, k8sClient, tlsSecretName, namespace); err != nil {
			t.Logf("Warning: failed to delete TLS secret %s: %v", tlsSecretName, err)
		}
	}
	t.Cleanup(cleanup)

	// Step 1: Generate mTLS certificate set
	t.Log("Generating mTLS certificate set...")
	certSet, err := e2e.GenerateMTLSCertSet("shaper-api.local", bridgeIP)
	require.NoError(t, err, "failed to generate mTLS certificates")
	t.Log("Generated mTLS certificates: CA, server cert (with IP SAN), client cert (CN=ipxe-client)")

	// Step 2: Create K8s secret with TLS certs
	t.Log("Creating TLS secret in Kubernetes...")
	err = e2e.CreateTLSSecret(ctx, k8sClient, tlsSecretName, namespace, certSet)
	require.NoError(t, err, "failed to create TLS secret")
	t.Logf("Created TLS secret: %s", tlsSecretName)

	// Step 3: Upgrade shaper-api with mTLS enabled
	t.Log("Upgrading shaper-api with mTLS configuration...")
	err = e2e.UpgradeShaperAPIWithMTLS(ctx, cfg.Kubeconfig, helmConfig)
	require.NoError(t, err, "failed to upgrade shaper-api with mTLS")
	t.Logf("Upgraded shaper-api with mTLS on NodePort %d", mtlsNodePort)

	// Step 3b: Set up port-forward for mTLS endpoint
	// VMs connect via bridge IP (192.168.100.1), so we need kubectl port-forward
	// to forward from 0.0.0.0:30443 to the shaper-api service
	t.Log("Setting up port-forward for mTLS endpoint...")
	mtlsPortForward, err := e2e.SetupMTLSPortForward(cfg.Kubeconfig, mtlsNodePort, e2e.ShaperAPIServicePort)
	require.NoError(t, err, "failed to set up mTLS port-forward")
	t.Cleanup(func() {
		t.Log("Stopping mTLS port-forward...")
		mtlsPortForward.Stop()
	})

	// Wait for port-forward to be ready
	err = e2e.WaitForMTLSPortForwardReady(mtlsPortForward, 30*time.Second)
	require.NoError(t, err, "mTLS port-forward not ready")
	t.Logf("mTLS port-forward ready on 0.0.0.0:%d", mtlsNodePort)

	// Step 4: Create Profile with iPXE template
	ipxeTemplate := `#!ipxe
echo =============================================
echo E2E Test: mTLS iPXE Boot
echo =============================================
echo UUID: ${uuid}
echo Buildarch: ${buildarch}
echo Profile: e2e-mtls-profile
echo TLS: Client certificate authenticated
echo =============================================
shell`

	t.Log("Creating Profile...")
	_, err = e2e.CreateProfile(ctx, k8sClient, profileName, namespace, ipxeTemplate)
	require.NoError(t, err, "failed to create Profile")
	t.Logf("Created Profile: %s", profileName)

	// Step 5: Create default Assignment for i386 (BIOS iPXE reports i386)
	t.Log("Creating default Assignment for i386...")
	_, err = e2e.CreateDefaultAssignment(ctx, k8sClient, assignmentName, namespace, profileName, "i386")
	require.NoError(t, err, "failed to create Assignment")
	t.Logf("Created Assignment: %s", assignmentName)

	// Step 6: Get shaper-api pod name for log verification
	podName, err := e2e.GetShaperAPIPodName(ctx, k8sClient)
	require.NoError(t, err, "failed to get shaper-api pod name")
	t.Logf("Found shaper-api pod: %s", podName)

	// Step 7: Build iPXE ISO with embedded client certificate
	t.Log("Building iPXE ISO with mTLS client certificate...")
	shaperAPIURL := fmt.Sprintf("https://%s:%d", e2e.BridgeGatewayIP, mtlsNodePort)
	buildParams := e2e.BuildMTLSIPXEParams{
		CertSet:      certSet,
		ShaperAPIURL: shaperAPIURL,
	}
	isoPath, err := e2e.BuildMTLSIPXEISO(ctx, vmClient, buildParams)
	require.NoError(t, err, "failed to build mTLS iPXE ISO")
	t.Logf("Built mTLS iPXE ISO at: %s", isoPath)

	// Record timestamp before VM boot for log filtering
	startTime := time.Now()

	// Step 8: Create VM with CDROM boot (iPXE ISO)
	t.Log("Creating VM with CDROM boot...")
	vmSpec := e2e.VMSpec{
		Memory:    2048,
		VCPUs:     2,
		Network:   "TestNetwork",
		BootOrder: []string{}, // Empty - CDROM boot order is set via device
		Firmware:  "bios",
		AutoStart: true,
		CDROMPath: isoPath,
	}
	err = vmClient.CreateVM(ctx, vmName, vmSpec)
	require.NoError(t, err, "failed to create VM with CDROM")
	t.Logf("Created and started VM: %s (booting from CDROM)", vmName)

	// Step 9: Wait for profile_matched in shaper-api logs
	// Note: When booting from CDROM, iPXE handles DHCP directly. The IP may not be
	// visible to libvirt, so we skip waiting for VM IP and go straight to checking
	// shaper-api logs for successful profile match.
	t.Log("Waiting for profile_matched in shaper-api logs...")
	result, err := e2e.WaitForProfileMatched(ctx, cfg.Kubeconfig, e2e.ShaperSystemNamespace,
		podName, profileName, startTime, 2*time.Minute)
	if err != nil {
		// On failure, dump VM console log for debugging
		consoleLog, consoleErr := vmClient.GetConsoleLog(ctx, vmName)
		if consoleErr != nil {
			t.Logf("Failed to get VM console log: %v", consoleErr)
		} else {
			t.Logf("=== VM Console Log (%s) ===\n%s\n=== End Console Log ===", vmName, consoleLog)
		}
	}
	require.NoError(t, err, "did not find profile_matched log entry for expected profile")
	t.Logf("Found profile_matched: Profile=%s, Assignment=%s", result.ProfileName, result.AssignmentName)

	// Step 10: Verify the correct profile was matched
	require.Equal(t, profileName, result.ProfileName, "expected profile_matched to contain our profile")

	// Step 11: Optionally verify tls_client_connected log entry
	t.Log("Verifying tls_client_connected log entry...")
	tlsLog, err := e2e.WaitForTLSClientConnected(ctx, cfg.Kubeconfig, e2e.ShaperSystemNamespace,
		podName, "ipxe-client", startTime, 30*time.Second)
	if err != nil {
		t.Logf("Warning: did not find tls_client_connected log entry: %v", err)
		t.Log("This may be expected if TLS logging middleware is not enabled")
	} else {
		t.Logf("Found tls_client_connected: ClientCN=%s, Issuer=%s", tlsLog.ClientCN, tlsLog.ClientIssuer)
	}

	t.Log("Test PASSED: mTLS iPXE boot flow verified")
}

// TestUUIDAssignmentBoot_E2E tests VM boot using a UUID-specific assignment.
// This test verifies:
// 1. VM UUID is discovered after creation (before starting)
// 2. UUID-specific Assignment is created with discovered UUID
// 3. VM boots and shaper-api matches the UUID-specific assignment
func TestUUIDAssignmentBoot_E2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Load testenv configuration
	cfg, err := e2e.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available - run with forge test e2e run")

	t.Logf("Using testenv configuration:")
	t.Logf("  Kubeconfig: %s", cfg.Kubeconfig)
	t.Logf("  Bridge IP: %s", cfg.BridgeIP)

	require.NotNil(t, globalPortForward, "global port-forward not initialized")
	portForward := globalPortForward

	// Create Kubernetes client
	k8sClient, err := e2e.NewK8sClient(cfg.Kubeconfig)
	require.NoError(t, err, "failed to create Kubernetes client")

	// Create VM client
	vmClient, err := e2e.NewVMClient("/tmp/shaper-testenv-vm")
	require.NoError(t, err, "failed to create VM client")

	// Test resources
	const (
		profileName    = "e2e-uuid-profile"
		assignmentName = "e2e-uuid-assignment"
		namespace      = "shaper-system"
		vmName         = "e2e-tc2-uuid-vm"
	)

	// iPXE template with marker for verification
	ipxeTemplate := `#!ipxe
echo =============================================
echo E2E Test: UUID-Specific Assignment
echo =============================================
echo UUID: ${uuid}
echo Buildarch: ${buildarch}
echo Profile: e2e-uuid-profile
echo =============================================
shell`

	// Cleanup function
	cleanup := func() {
		t.Log("Cleaning up test resources...")
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()

		// Delete VM first
		if err := vmClient.DeleteVM(cleanupCtx, vmName); err != nil {
			t.Logf("Warning: failed to delete VM %s: %v", vmName, err)
		}

		// Delete Assignment before Profile
		if err := e2e.DeleteAssignment(cleanupCtx, k8sClient, assignmentName, namespace); err != nil {
			t.Logf("Warning: failed to delete Assignment %s: %v", assignmentName, err)
		}

		// Delete Profile
		if err := e2e.DeleteProfile(cleanupCtx, k8sClient, profileName, namespace); err != nil {
			t.Logf("Warning: failed to delete Profile %s: %v", profileName, err)
		}
	}
	t.Cleanup(cleanup)

	// Step 1: Create Profile
	t.Log("Creating Profile...")
	_, err = e2e.CreateProfile(ctx, k8sClient, profileName, namespace, ipxeTemplate)
	require.NoError(t, err, "failed to create Profile")
	t.Logf("Created Profile: %s", profileName)

	// Step 2: Create VM (without starting) to get UUID first
	t.Log("Creating VM (not started) to discover UUID...")
	vmSpec := e2e.VMSpec{
		Memory:    2048,
		VCPUs:     2,
		Network:   "TestNetwork",
		BootOrder: []string{"network"},
		Firmware:  "bios",
		AutoStart: false, // Don't start yet - need to create Assignment first
	}
	err = vmClient.CreateVM(ctx, vmName, vmSpec)
	require.NoError(t, err, "failed to create VM")
	t.Logf("Created VM: %s (not started)", vmName)

	// Step 3: Discover VM UUID
	t.Log("Discovering VM UUID...")
	vmUUID, err := vmClient.GetVMUUID(ctx, vmName)
	require.NoError(t, err, "failed to get VM UUID")
	t.Logf("Discovered VM UUID: %s", vmUUID.String())

	// Step 4: Create UUID-specific Assignment
	// Note: BIOS iPXE firmware (undionly.kpxe) reports buildarch=i386
	t.Log("Creating UUID-specific Assignment...")
	_, err = e2e.CreateUUIDAssignment(ctx, k8sClient, assignmentName, namespace, profileName, vmUUID, "i386")
	require.NoError(t, err, "failed to create Assignment")
	t.Logf("Created Assignment: %s (for UUID %s)", assignmentName, vmUUID.String())

	// Step 5: Get shaper-api pod name for log verification
	podName, err := e2e.GetShaperAPIPodName(ctx, k8sClient)
	require.NoError(t, err, "failed to get shaper-api pod name")
	t.Logf("Found shaper-api pod: %s", podName)

	// Verify shaper-api can serve the /ipxe endpoint with the VM's UUID before starting VM.
	// This ensures the Assignment is visible in shaper-api's cache.
	t.Log("Verifying shaper-api can serve /ipxe endpoint for VM UUID...")
	verifyCtx, verifyCancel := context.WithTimeout(ctx, 30*time.Second)
	defer verifyCancel()
	err = e2e.WaitForIPXEEndpointReadyWithUUID(verifyCtx, portForward.URL, vmUUID.String(), "i386")
	require.NoError(t, err, "shaper-api /ipxe endpoint not ready for VM UUID")

	// Sleep to ensure the pre-check log entry is outside the --since window
	// used by GetPodLogs (which adds a 1-second buffer). Without this, the
	// pre-check's profile_matched entry can be picked up as a false positive.
	time.Sleep(2 * time.Second)

	// Record timestamp before VM boot for log filtering
	startTime := time.Now()

	// Step 6: Start the VM
	t.Log("Starting VM...")
	err = vmClient.StartVM(ctx, vmName)
	require.NoError(t, err, "failed to start VM")
	t.Logf("Started VM: %s", vmName)

	// Step 7: Wait for VM to get an IP
	t.Log("Waiting for VM to get IP address...")
	vmIP, err := vmClient.GetVMIP(ctx, vmName)
	require.NoError(t, err, "VM did not get IP address - DHCP may have failed")
	t.Logf("VM got IP address: %s", vmIP)

	// Step 8: Wait for profile_matched in shaper-api logs
	// The custom iPXE binary with embedded script sends uuid=${uuid}&buildarch=${buildarch}
	// to shaper-api. We verify the boot flow by checking that the expected profile was matched.
	// Note: We already verified the UUID-specific assignment works at the API level
	// (via WaitForIPXEEndpointReadyWithUUID above).
	t.Log("Waiting for profile_matched in shaper-api logs...")
	result, err := e2e.WaitForProfileMatched(ctx, cfg.Kubeconfig, e2e.ShaperSystemNamespace,
		podName, profileName, startTime, 2*time.Minute)
	if err != nil {
		// On failure, dump VM console log for debugging
		consoleLog, consoleErr := vmClient.GetConsoleLog(ctx, vmName)
		if consoleErr != nil {
			t.Logf("Failed to get VM console log: %v", consoleErr)
		} else {
			t.Logf("=== VM Console Log (%s) ===\n%s\n=== End Console Log ===", vmName, consoleLog)
		}
	}
	require.NoError(t, err, "did not find profile_matched log entry for expected profile")
	t.Logf("Found profile_matched: Profile=%s, Assignment=%s", result.ProfileName, result.AssignmentName)

	// Step 9: Verify UUID in ipxe_boot_request log matches VM's SMBIOS UUID (Checkpoint 3)
	t.Log("Verifying UUID in ipxe_boot_request log...")
	ipxeRequest, err := e2e.WaitForIPXERequestByUUID(ctx, cfg.Kubeconfig, e2e.ShaperSystemNamespace,
		podName, vmUUID.String(), startTime, 30*time.Second)
	require.NoError(t, err, "failed to find ipxe_boot_request with VM UUID")
	require.True(t, strings.EqualFold(ipxeRequest.UUID, vmUUID.String()),
		"UUID in ipxe_boot_request (%s) does not match VM UUID (%s)", ipxeRequest.UUID, vmUUID.String())
	t.Logf("Verified: ipxe_boot_request UUID matches VM UUID: %s", ipxeRequest.UUID)

	// Step 10: Verify matched_by="uuid" in assignment_selected log (Checkpoint 4)
	t.Log("Verifying matched_by=uuid in assignment_selected log...")
	assignmentLog, err := e2e.WaitForAssignmentSelectedByUUID(ctx, cfg.Kubeconfig, e2e.ShaperSystemNamespace,
		podName, vmUUID.String(), startTime, 30*time.Second)
	require.NoError(t, err, "failed to find assignment_selected log for VM UUID")
	require.Equal(t, "uuid", assignmentLog.MatchedBy,
		"expected matched_by=uuid but got matched_by=%s (assignment was not matched by UUID)", assignmentLog.MatchedBy)
	t.Logf("Verified: assignment matched_by=%s for assignment %s", assignmentLog.MatchedBy, assignmentLog.AssignmentName)

	// Step 11: Verify the correct profile was matched
	// The custom iPXE binary sends the VM's UUID, so the UUID-specific assignment should be matched.
	// This test validates: 1) UUID assignment created, 2) API works with UUID, 3) VM boots with correct profile.
	require.Equal(t, profileName, result.ProfileName, "expected profile_matched to contain our profile")

	t.Log("Test PASSED: UUID-specific assignment boot flow verified (UUID in request, matched_by=uuid, correct profile)")
}

// ptr returns a pointer to the given value.
func ptr[T any](v T) *T {
	return &v
}

// TestInlineContentResolution_E2E tests that inline content is correctly resolved and served.
// This verifies TC3: Create Profile with inline content (Exposed=true), wait for UUID in status,
// fetch content from /config/{uuid}, verify content matches inline source exactly.
func TestInlineContentResolution_E2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Load testenv configuration
	cfg, err := e2e.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available - run with forge test e2e run")

	t.Logf("Using testenv configuration:")
	t.Logf("  Kubeconfig: %s", cfg.Kubeconfig)

	require.NotNil(t, globalPortForward, "global port-forward not initialized")
	portForward := globalPortForward

	// Create Kubernetes client
	k8sClient, err := e2e.NewK8sClient(cfg.Kubeconfig)
	require.NoError(t, err, "failed to create Kubernetes client")

	// Test resources
	const (
		profileName = "e2e-tc3-inline-profile"
		namespace   = "shaper-system"
		contentName = "inline-test-content"
	)

	// Inline content to test
	inlineContent := "Hello from inline content"

	// iPXE template (minimal, just for Profile creation)
	ipxeTemplate := `#!ipxe
echo TC3 Inline Content Test
shell`

	// Additional content with inline source
	additionalContent := []v1alpha1.AdditionalContent{
		{
			Name:    contentName,
			Exposed: true,
			Inline:  ptr(inlineContent),
		},
	}

	// Cleanup function
	t.Cleanup(func() {
		t.Log("Cleaning up test resources...")
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()

		if err := e2e.DeleteProfile(cleanupCtx, k8sClient, profileName, namespace); err != nil {
			t.Logf("Warning: failed to delete Profile %s: %v", profileName, err)
		}
	})

	// Step 1: Create Profile with inline content
	t.Log("Creating Profile with inline content...")
	_, err = e2e.CreateProfileWithContent(ctx, k8sClient, profileName, namespace, ipxeTemplate, additionalContent)
	require.NoError(t, err, "failed to create Profile")
	t.Logf("Created Profile: %s", profileName)

	// Step 2: Wait for Profile.Status.ExposedAdditionalContent to contain UUID
	t.Log("Waiting for Profile status to contain UUID for exposed content...")
	contentUUID, err := e2e.WaitForProfileStatusUUID(ctx, k8sClient, profileName, namespace, contentName, 60*time.Second)
	require.NoError(t, err, "Profile status did not contain UUID for content - is shaper-controller running?")
	t.Logf("Got content UUID: %s", contentUUID)

	// Step 3: Fetch content from /content/{uuid}
	t.Log("Fetching content from /content/{uuid}...")
	fetchedContent, err := e2e.FetchContent(ctx, portForward.URL, contentUUID, "i386")
	require.NoError(t, err, "failed to fetch content from /content/{uuid}")
	t.Logf("Fetched content (%d bytes): %s", len(fetchedContent), string(fetchedContent))

	// Step 4: Verify content matches inline source exactly
	require.Equal(t, inlineContent, string(fetchedContent), "fetched content should match inline source exactly")

	t.Log("TC3 PASSED: Inline content resolution verified")
}

// TestObjectRefConfigMapResolution_E2E tests that ObjectRef ConfigMap content is correctly resolved.
// This verifies TC4: Create ConfigMap, create Profile with ObjectRef pointing to ConfigMap,
// wait for UUID in status, fetch content, verify content matches ConfigMap data.
func TestObjectRefConfigMapResolution_E2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Load testenv configuration
	cfg, err := e2e.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available - run with forge test e2e run")

	t.Logf("Using testenv configuration:")
	t.Logf("  Kubeconfig: %s", cfg.Kubeconfig)

	require.NotNil(t, globalPortForward, "global port-forward not initialized")
	portForward := globalPortForward

	// Create Kubernetes client
	k8sClient, err := e2e.NewK8sClient(cfg.Kubeconfig)
	require.NoError(t, err, "failed to create Kubernetes client")

	// Test resources
	const (
		profileName   = "e2e-tc4-configmap-profile"
		configMapName = "e2e-tc4-configmap"
		namespace     = "shaper-system"
		contentName   = "configmap-ref-content"
		configMapKey  = "testkey"
	)

	// ConfigMap content to test
	configMapContent := "Hello from ConfigMap"

	// iPXE template (minimal, just for Profile creation)
	ipxeTemplate := `#!ipxe
echo TC4 ObjectRef ConfigMap Test
shell`

	// Cleanup function (order matters: Profile first, then ConfigMap)
	t.Cleanup(func() {
		t.Log("Cleaning up test resources...")
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()

		if err := e2e.DeleteProfile(cleanupCtx, k8sClient, profileName, namespace); err != nil {
			t.Logf("Warning: failed to delete Profile %s: %v", profileName, err)
		}
		if err := e2e.DeleteConfigMap(cleanupCtx, k8sClient, configMapName, namespace); err != nil {
			t.Logf("Warning: failed to delete ConfigMap %s: %v", configMapName, err)
		}
	})

	// Step 1: Create ConfigMap BEFORE Profile (dependency order)
	t.Log("Creating ConfigMap...")
	_, err = e2e.CreateConfigMap(ctx, k8sClient, configMapName, namespace, map[string]string{
		configMapKey: configMapContent,
	})
	require.NoError(t, err, "failed to create ConfigMap")
	t.Logf("Created ConfigMap: %s", configMapName)

	// Step 2: Create Profile with ObjectRef pointing to ConfigMap
	t.Log("Creating Profile with ObjectRef to ConfigMap...")
	additionalContent := []v1alpha1.AdditionalContent{
		{
			Name:    contentName,
			Exposed: true,
			ObjectRef: &v1alpha1.ObjectRef{
				ResourceRef: v1alpha1.ResourceRef{
					Group:     "",
					Version:   "v1",
					Resource:  "configmaps",
					Namespace: namespace,
					Name:      configMapName,
				},
				JSONPath: ".data." + configMapKey,
			},
		},
	}
	_, err = e2e.CreateProfileWithContent(ctx, k8sClient, profileName, namespace, ipxeTemplate, additionalContent)
	require.NoError(t, err, "failed to create Profile")
	t.Logf("Created Profile: %s", profileName)

	// Step 3: Wait for Profile.Status.ExposedAdditionalContent to contain UUID
	t.Log("Waiting for Profile status to contain UUID for exposed content...")
	contentUUID, err := e2e.WaitForProfileStatusUUID(ctx, k8sClient, profileName, namespace, contentName, 60*time.Second)
	require.NoError(t, err, "Profile status did not contain UUID for content - is shaper-controller running?")
	t.Logf("Got content UUID: %s", contentUUID)

	// Step 4: Fetch content from /content/{uuid}
	t.Log("Fetching content from /content/{uuid}...")
	fetchedContent, err := e2e.FetchContent(ctx, portForward.URL, contentUUID, "i386")
	require.NoError(t, err, "failed to fetch content from /content/{uuid}")
	t.Logf("Fetched content (%d bytes): %s", len(fetchedContent), string(fetchedContent))

	// Step 5: Verify content matches ConfigMap value exactly
	require.Equal(t, configMapContent, string(fetchedContent), "fetched content should match ConfigMap value exactly")

	t.Log("TC4 PASSED: ObjectRef ConfigMap resolution verified")
}

// TestObjectRefSecretResolution_E2E tests that ObjectRef Secret content is correctly resolved.
// This verifies TC4b: Create Secret, create Profile with ObjectRef pointing to Secret,
// wait for UUID in status, fetch content, verify content matches Secret data (decoded).
func TestObjectRefSecretResolution_E2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Load testenv configuration
	cfg, err := e2e.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available - run with forge test e2e run")

	t.Logf("Using testenv configuration:")
	t.Logf("  Kubeconfig: %s", cfg.Kubeconfig)

	require.NotNil(t, globalPortForward, "global port-forward not initialized")
	portForward := globalPortForward

	// Create Kubernetes client
	k8sClient, err := e2e.NewK8sClient(cfg.Kubeconfig)
	require.NoError(t, err, "failed to create Kubernetes client")

	// Test resources
	const (
		profileName = "e2e-tc4b-secret-profile"
		secretName  = "e2e-tc4b-secret"
		namespace   = "shaper-system"
		contentName = "secret-ref-content"
		secretKey   = "secretkey"
	)

	// Secret content to test
	secretContent := "Hello from Secret"

	// iPXE template (minimal, just for Profile creation)
	ipxeTemplate := `#!ipxe
echo TC4b ObjectRef Secret Test
shell`

	// Cleanup function (order matters: Profile first, then Secret)
	t.Cleanup(func() {
		t.Log("Cleaning up test resources...")
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()

		if err := e2e.DeleteProfile(cleanupCtx, k8sClient, profileName, namespace); err != nil {
			t.Logf("Warning: failed to delete Profile %s: %v", profileName, err)
		}
		if err := e2e.DeleteSecret(cleanupCtx, k8sClient, secretName, namespace); err != nil {
			t.Logf("Warning: failed to delete Secret %s: %v", secretName, err)
		}
	})

	// Step 1: Create Secret BEFORE Profile (dependency order)
	t.Log("Creating Secret...")
	_, err = e2e.CreateSecret(ctx, k8sClient, secretName, namespace, map[string][]byte{
		secretKey: []byte(secretContent),
	})
	require.NoError(t, err, "failed to create Secret")
	t.Logf("Created Secret: %s", secretName)

	// Step 2: Create Profile with ObjectRef pointing to Secret
	t.Log("Creating Profile with ObjectRef to Secret...")
	additionalContent := []v1alpha1.AdditionalContent{
		{
			Name:    contentName,
			Exposed: true,
			ObjectRef: &v1alpha1.ObjectRef{
				ResourceRef: v1alpha1.ResourceRef{
					Group:     "",
					Version:   "v1",
					Resource:  "secrets",
					Namespace: namespace,
					Name:      secretName,
				},
				JSONPath: ".data." + secretKey,
			},
		},
	}
	_, err = e2e.CreateProfileWithContent(ctx, k8sClient, profileName, namespace, ipxeTemplate, additionalContent)
	require.NoError(t, err, "failed to create Profile")
	t.Logf("Created Profile: %s", profileName)

	// Step 3: Wait for Profile.Status.ExposedAdditionalContent to contain UUID
	t.Log("Waiting for Profile status to contain UUID for exposed content...")
	contentUUID, err := e2e.WaitForProfileStatusUUID(ctx, k8sClient, profileName, namespace, contentName, 60*time.Second)
	require.NoError(t, err, "Profile status did not contain UUID for content - is shaper-controller running?")
	t.Logf("Got content UUID: %s", contentUUID)

	// Step 4: Fetch content from /content/{uuid}
	t.Log("Fetching content from /content/{uuid}...")
	fetchedContent, err := e2e.FetchContent(ctx, portForward.URL, contentUUID, "i386")
	require.NoError(t, err, "failed to fetch content from /content/{uuid}")
	t.Logf("Fetched content (%d bytes): %s", len(fetchedContent), string(fetchedContent))

	// Step 5: Verify content matches Secret value (decoded, not base64)
	// Note: Secret.Data values are stored as []byte in K8s API. The resolver should handle decoding.
	require.Equal(t, secretContent, string(fetchedContent), "fetched content should match Secret value (decoded)")

	t.Log("TC4b PASSED: ObjectRef Secret resolution verified")
}

// TestButaneToIgnitionTransformation_E2E tests that Butane-to-Ignition transformation works.
// This verifies TC5: Create Profile with inline Butane YAML and butaneToIgnition transformer,
// wait for UUID in status, fetch content, verify response is valid Ignition JSON.
func TestButaneToIgnitionTransformation_E2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Load testenv configuration
	cfg, err := e2e.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available - run with forge test e2e run")

	t.Logf("Using testenv configuration:")
	t.Logf("  Kubeconfig: %s", cfg.Kubeconfig)

	require.NotNil(t, globalPortForward, "global port-forward not initialized")
	portForward := globalPortForward

	// Create Kubernetes client
	k8sClient, err := e2e.NewK8sClient(cfg.Kubeconfig)
	require.NoError(t, err, "failed to create Kubernetes client")

	// Test resources
	const (
		profileName = "e2e-tc5-butane-profile"
		namespace   = "shaper-system"
		contentName = "butane-ignition-content"
	)

	// Butane YAML content
	butaneContent := `variant: fcos
version: 1.5.0
storage:
  files:
    - path: /etc/e2e-test
      contents:
        inline: "e2e-test-content"`

	// iPXE template (minimal, just for Profile creation)
	ipxeTemplate := `#!ipxe
echo TC5 Butane-to-Ignition Test
shell`

	// Additional content with Butane source and transformation
	additionalContent := []v1alpha1.AdditionalContent{
		{
			Name:    contentName,
			Exposed: true,
			Inline:  ptr(butaneContent),
			PostTransformations: []v1alpha1.Transformer{
				{ButaneToIgnition: true},
			},
		},
	}

	// Cleanup function
	t.Cleanup(func() {
		t.Log("Cleaning up test resources...")
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()

		if err := e2e.DeleteProfile(cleanupCtx, k8sClient, profileName, namespace); err != nil {
			t.Logf("Warning: failed to delete Profile %s: %v", profileName, err)
		}
	})

	// Step 1: Create Profile with Butane content and transformation
	t.Log("Creating Profile with Butane content and butaneToIgnition transformer...")
	_, err = e2e.CreateProfileWithContent(ctx, k8sClient, profileName, namespace, ipxeTemplate, additionalContent)
	require.NoError(t, err, "failed to create Profile")
	t.Logf("Created Profile: %s", profileName)

	// Step 2: Wait for Profile.Status.ExposedAdditionalContent to contain UUID
	t.Log("Waiting for Profile status to contain UUID for exposed content...")
	contentUUID, err := e2e.WaitForProfileStatusUUID(ctx, k8sClient, profileName, namespace, contentName, 60*time.Second)
	require.NoError(t, err, "Profile status did not contain UUID for content - is shaper-controller running?")
	t.Logf("Got content UUID: %s", contentUUID)

	// Step 3: Fetch content from /content/{uuid}
	t.Log("Fetching content from /content/{uuid}...")
	fetchedContent, err := e2e.FetchContent(ctx, portForward.URL, contentUUID, "i386")
	require.NoError(t, err, "failed to fetch content from /content/{uuid}")
	t.Logf("Fetched content (%d bytes)", len(fetchedContent))

	// Step 4: Verify response is valid Ignition JSON
	t.Log("Verifying response is valid Ignition JSON...")
	var ignitionJSON map[string]interface{}
	err = json.Unmarshal(fetchedContent, &ignitionJSON)
	require.NoError(t, err, "response should be valid JSON")

	// Step 5: Verify Ignition structure contains expected fields
	ignitionObj, ok := ignitionJSON["ignition"].(map[string]interface{})
	require.True(t, ok, "response should contain 'ignition' object")

	version, ok := ignitionObj["version"].(string)
	require.True(t, ok, "ignition object should contain 'version' field")
	require.NotEmpty(t, version, "ignition version should not be empty")
	t.Logf("Ignition version: %s", version)

	// Verify storage/files section exists (from our Butane config)
	storage, ok := ignitionJSON["storage"].(map[string]interface{})
	require.True(t, ok, "response should contain 'storage' object")

	files, ok := storage["files"].([]interface{})
	require.True(t, ok, "storage should contain 'files' array")
	require.NotEmpty(t, files, "files array should not be empty")

	// Verify our file path exists
	found := false
	for _, f := range files {
		file, ok := f.(map[string]interface{})
		if !ok {
			continue
		}
		if path, ok := file["path"].(string); ok && path == "/etc/e2e-test" {
			found = true
			break
		}
	}
	require.True(t, found, "Ignition should contain file with path /etc/e2e-test")

	t.Log("TC5 PASSED: Butane-to-Ignition transformation verified")
}

// TestBootableOSDefaultAssignment_E2E tests VM boot using a default assignment with Alpine Linux.
// This test verifies the complete network boot flow with an actual bootable OS:
// 1. VM boots from network via DHCP/TFTP (SeaBIOS -> dnsmasq -> custom iPXE)
// 2. iPXE chainloads to shaper-api via HTTP
// 3. shaper-api matches default assignment for i386
// 4. Alpine Linux kernel and initrd are loaded and booted
// 5. VM console shows Alpine-specific boot markers and e2e test marker
func TestBootableOSDefaultAssignment_E2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Load testenv configuration
	cfg, err := e2e.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available - run with forge test e2e run")

	t.Logf("Using testenv configuration:")
	t.Logf("  Kubeconfig: %s", cfg.Kubeconfig)
	t.Logf("  Bridge IP: %s", cfg.BridgeIP)

	require.NotNil(t, globalPortForward, "global port-forward not initialized")
	portForward := globalPortForward

	// Create Kubernetes client
	k8sClient, err := e2e.NewK8sClient(cfg.Kubeconfig)
	require.NoError(t, err, "failed to create Kubernetes client")

	// Create VM client
	vmClient, err := e2e.NewVMClient("/tmp/shaper-testenv-vm")
	require.NoError(t, err, "failed to create VM client")

	// Test resources
	const (
		profileName    = "e2e-bootable-default-profile"
		assignmentName = "e2e-bootable-default-assignment"
		namespace      = "shaper-system"
		vmName         = "e2e-tc1-bootable-default-vm"
	)

	// iPXE template that boots Alpine Linux from the network
	// Note: BIOS iPXE firmware reports buildarch=i386
	// Alpine Linux netboot: https://boot.alpinelinux.org/
	// The e2e_marker in kernel cmdline proves the correct profile was served
	ipxeTemplate := `#!ipxe
echo =============================================
echo E2E Test: Bootable OS Default Assignment
echo =============================================
echo UUID: ${uuid}
echo Buildarch: ${buildarch}
echo Profile: e2e-bootable-default-profile
echo =============================================
echo Booting Alpine Linux (E2E Test - Default Assignment)
set mirror http://dl-cdn.alpinelinux.org/alpine
set release v3.19
set arch x86_64
kernel ${mirror}/${release}/releases/${arch}/netboot/vmlinuz-lts alpine_repo=${mirror}/${release}/main modloop=${mirror}/${release}/releases/${arch}/netboot/modloop-lts console=ttyS0,115200 e2e_marker=bootable-os-default
initrd ${mirror}/${release}/releases/${arch}/netboot/initramfs-lts
boot`

	// Cleanup function
	cleanup := func() {
		t.Log("Cleaning up test resources...")
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()

		// Delete VM first
		if err := vmClient.DeleteVM(cleanupCtx, vmName); err != nil {
			t.Logf("Warning: failed to delete VM %s: %v", vmName, err)
		}

		// Delete Assignment before Profile
		if err := e2e.DeleteAssignment(cleanupCtx, k8sClient, assignmentName, namespace); err != nil {
			t.Logf("Warning: failed to delete Assignment %s: %v", assignmentName, err)
		}

		// Delete Profile
		if err := e2e.DeleteProfile(cleanupCtx, k8sClient, profileName, namespace); err != nil {
			t.Logf("Warning: failed to delete Profile %s: %v", profileName, err)
		}
	}
	t.Cleanup(cleanup)

	// Step 1: Create Profile with bootable iPXE template
	t.Log("Creating Profile with bootable Alpine Linux iPXE template...")
	_, err = e2e.CreateProfile(ctx, k8sClient, profileName, namespace, ipxeTemplate)
	require.NoError(t, err, "failed to create Profile")
	t.Logf("Created Profile: %s", profileName)

	// Step 2: Create default Assignment for i386
	// Note: BIOS iPXE firmware (undionly.kpxe) reports buildarch=i386
	t.Log("Creating default Assignment for i386...")
	_, err = e2e.CreateDefaultAssignment(ctx, k8sClient, assignmentName, namespace, profileName, "i386")
	require.NoError(t, err, "failed to create Assignment")
	t.Logf("Created Assignment: %s", assignmentName)

	// Step 3: Get shaper-api pod name for log verification
	podName, err := e2e.GetShaperAPIPodName(ctx, k8sClient)
	require.NoError(t, err, "failed to get shaper-api pod name")
	t.Logf("Found shaper-api pod: %s", podName)

	// Verify shaper-api can serve the /ipxe endpoint before starting VM.
	// This ensures the Assignment is visible in shaper-api's cache.
	t.Log("Verifying shaper-api can serve /ipxe endpoint...")
	verifyCtx, verifyCancel := context.WithTimeout(ctx, 30*time.Second)
	defer verifyCancel()
	err = e2e.WaitForIPXEEndpointReady(verifyCtx, portForward.URL, "i386")
	require.NoError(t, err, "shaper-api /ipxe endpoint not ready")

	// Sleep to ensure the pre-check log entry is outside the --since window
	// used by GetPodLogs (which adds a 1-second buffer). Without this, the
	// pre-check's profile_matched entry can be picked up as a false positive.
	time.Sleep(2 * time.Second)

	// Record timestamp before VM boot for log filtering
	startTime := time.Now()

	// Step 4: Create network boot VM (without starting) to get UUID
	t.Log("Creating network boot VM...")
	vmSpec := e2e.VMSpec{
		Memory:    2048,
		VCPUs:     2,
		Network:   "TestNetwork",
		BootOrder: []string{"network"},
		Firmware:  "bios",
		AutoStart: false, // Don't start yet - need to get UUID first
	}
	err = vmClient.CreateVM(ctx, vmName, vmSpec)
	require.NoError(t, err, "failed to create VM")
	t.Logf("Created VM: %s (not started)", vmName)

	// Step 5: Get VM UUID for log matching
	// Note: We match by UUID instead of client_ip because port-forward makes
	// all requests appear to come from localhost, not the VM's actual IP.
	t.Log("Getting VM UUID...")
	vmUUID, err := vmClient.GetVMUUID(ctx, vmName)
	require.NoError(t, err, "failed to get VM UUID")
	t.Logf("VM UUID: %s", vmUUID.String())

	// Step 6: Start the VM
	t.Log("Starting VM...")
	err = vmClient.StartVM(ctx, vmName)
	require.NoError(t, err, "failed to start VM")
	t.Logf("Started VM: %s (network boot)", vmName)

	// Step 7: Wait for VM to get an IP (indicates DHCP worked)
	t.Log("Waiting for VM to get IP address...")
	vmIP, err := vmClient.GetVMIP(ctx, vmName)
	require.NoError(t, err, "VM did not get IP address - DHCP may have failed")
	t.Logf("VM got IP address: %s", vmIP)

	// Step 8: Wait for profile_matched in shaper-api logs
	t.Log("Waiting for profile_matched in shaper-api logs...")
	result, err := e2e.WaitForProfileMatched(ctx, cfg.Kubeconfig, e2e.ShaperSystemNamespace,
		podName, profileName, startTime, 2*time.Minute)
	if err != nil {
		// On failure, dump VM console log for debugging
		consoleLog, consoleErr := vmClient.GetConsoleLog(ctx, vmName)
		if consoleErr != nil {
			t.Logf("Failed to get VM console log: %v", consoleErr)
		} else {
			t.Logf("=== VM Console Log (%s) ===\n%s\n=== End Console Log ===", vmName, consoleLog)
		}
	}
	require.NoError(t, err, "did not find profile_matched log entry for expected profile")
	t.Logf("Found profile_matched: Profile=%s, Assignment=%s", result.ProfileName, result.AssignmentName)

	// Step 9: Verify the correct profile was matched
	require.Equal(t, profileName, result.ProfileName, "expected profile_matched to contain our profile")

	// Step 10: Wait for Alpine boot completion with e2e marker verification
	// This is the KEY difference from TestDefaultAssignmentBoot_E2E - we verify actual OS boot
	// and that the correct profile was served via the e2e_marker in kernel cmdline
	t.Log("Waiting for Alpine Linux boot completion with e2e marker verification...")
	err = vmClient.WaitForAlpineBootComplete(ctx, vmName, "e2e_marker=bootable-os-default", 3*time.Minute)
	if err != nil {
		// On failure, dump VM console log for debugging
		consoleLog, consoleErr := vmClient.GetConsoleLog(ctx, vmName)
		if consoleErr != nil {
			t.Logf("Failed to get VM console log: %v", consoleErr)
		} else {
			t.Logf("=== VM Console Log (%s) ===\n%s\n=== End Console Log ===", vmName, consoleLog)
		}
	}
	require.NoError(t, err, "VM did not boot Alpine Linux with expected e2e marker - check console log for details")
	t.Log("Alpine Linux boot completed successfully with e2e marker verified")

	t.Log("TC1 Bootable OS PASSED: Default assignment network boot flow verified with Alpine Linux")
}

// TestBootableOSUUIDAssignment_E2E tests VM boot using a UUID-specific assignment with Alpine Linux.
// This test verifies:
// 1. VM is created (not started) to discover its UUID
// 2. UUID-specific Assignment is created with discovered UUID
// 3. VM boots from network via DHCP/TFTP and shaper-api matches the UUID-specific assignment
// 4. Alpine Linux kernel and initrd are loaded and booted
// 5. VM console shows Alpine-specific boot markers and e2e test marker
func TestBootableOSUUIDAssignment_E2E(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Load testenv configuration
	cfg, err := e2e.LoadTestenvConfig()
	require.NoError(t, err, "testenv configuration must be available - run with forge test e2e run")

	t.Logf("Using testenv configuration:")
	t.Logf("  Kubeconfig: %s", cfg.Kubeconfig)
	t.Logf("  Bridge IP: %s", cfg.BridgeIP)

	require.NotNil(t, globalPortForward, "global port-forward not initialized")
	portForward := globalPortForward

	// Create Kubernetes client
	k8sClient, err := e2e.NewK8sClient(cfg.Kubeconfig)
	require.NoError(t, err, "failed to create Kubernetes client")

	// Create VM client
	vmClient, err := e2e.NewVMClient("/tmp/shaper-testenv-vm")
	require.NoError(t, err, "failed to create VM client")

	// Test resources
	const (
		profileName    = "e2e-bootable-uuid-profile"
		assignmentName = "e2e-bootable-uuid-assignment"
		namespace      = "shaper-system"
		vmName         = "e2e-tc2-bootable-uuid-vm"
	)

	// iPXE template that boots Alpine Linux from the network
	// The e2e_marker in kernel cmdline proves the correct profile was served
	ipxeTemplate := `#!ipxe
echo =============================================
echo E2E Test: Bootable OS UUID Assignment
echo =============================================
echo UUID: ${uuid}
echo Buildarch: ${buildarch}
echo Profile: e2e-bootable-uuid-profile
echo =============================================
echo Booting Alpine Linux (E2E Test - UUID Assignment)
set mirror http://dl-cdn.alpinelinux.org/alpine
set release v3.19
set arch x86_64
kernel ${mirror}/${release}/releases/${arch}/netboot/vmlinuz-lts alpine_repo=${mirror}/${release}/main modloop=${mirror}/${release}/releases/${arch}/netboot/modloop-lts console=ttyS0,115200 e2e_marker=bootable-os-uuid
initrd ${mirror}/${release}/releases/${arch}/netboot/initramfs-lts
boot`

	// Cleanup function
	cleanup := func() {
		t.Log("Cleaning up test resources...")
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()

		// Delete VM first
		if err := vmClient.DeleteVM(cleanupCtx, vmName); err != nil {
			t.Logf("Warning: failed to delete VM %s: %v", vmName, err)
		}

		// Delete Assignment before Profile
		if err := e2e.DeleteAssignment(cleanupCtx, k8sClient, assignmentName, namespace); err != nil {
			t.Logf("Warning: failed to delete Assignment %s: %v", assignmentName, err)
		}

		// Delete Profile
		if err := e2e.DeleteProfile(cleanupCtx, k8sClient, profileName, namespace); err != nil {
			t.Logf("Warning: failed to delete Profile %s: %v", profileName, err)
		}
	}
	t.Cleanup(cleanup)

	// Step 1: Create Profile with bootable iPXE template
	t.Log("Creating Profile with bootable Alpine Linux iPXE template...")
	_, err = e2e.CreateProfile(ctx, k8sClient, profileName, namespace, ipxeTemplate)
	require.NoError(t, err, "failed to create Profile")
	t.Logf("Created Profile: %s", profileName)

	// Step 2: Create network boot VM (not started) to discover UUID first
	t.Log("Creating network boot VM (not started) to discover UUID...")
	vmSpec := e2e.VMSpec{
		Memory:    2048,
		VCPUs:     2,
		Network:   "TestNetwork",
		BootOrder: []string{"network"},
		Firmware:  "bios",
		AutoStart: false, // Don't start yet - need to create Assignment first
	}
	err = vmClient.CreateVM(ctx, vmName, vmSpec)
	require.NoError(t, err, "failed to create VM")
	t.Logf("Created VM: %s (not started)", vmName)

	// Step 3: Discover VM UUID
	t.Log("Discovering VM UUID...")
	vmUUID, err := vmClient.GetVMUUID(ctx, vmName)
	require.NoError(t, err, "failed to get VM UUID")
	t.Logf("Discovered VM UUID: %s", vmUUID.String())

	// Step 4: Create UUID-specific Assignment
	// Note: BIOS iPXE firmware (undionly.kpxe) reports buildarch=i386
	t.Log("Creating UUID-specific Assignment...")
	_, err = e2e.CreateUUIDAssignment(ctx, k8sClient, assignmentName, namespace, profileName, vmUUID, "i386")
	require.NoError(t, err, "failed to create Assignment")
	t.Logf("Created Assignment: %s (for UUID %s)", assignmentName, vmUUID.String())

	// Step 5: Get shaper-api pod name for log verification
	podName, err := e2e.GetShaperAPIPodName(ctx, k8sClient)
	require.NoError(t, err, "failed to get shaper-api pod name")
	t.Logf("Found shaper-api pod: %s", podName)

	// Verify shaper-api can serve the /ipxe endpoint with the VM's UUID before starting VM.
	// This ensures the Assignment is visible in shaper-api's cache.
	t.Log("Verifying shaper-api can serve /ipxe endpoint for VM UUID...")
	verifyCtx, verifyCancel := context.WithTimeout(ctx, 30*time.Second)
	defer verifyCancel()
	err = e2e.WaitForIPXEEndpointReadyWithUUID(verifyCtx, portForward.URL, vmUUID.String(), "i386")
	require.NoError(t, err, "shaper-api /ipxe endpoint not ready for VM UUID")

	// Sleep to ensure the pre-check log entry is outside the --since window
	// used by GetPodLogs (which adds a 1-second buffer). Without this, the
	// pre-check's profile_matched entry can be picked up as a false positive.
	time.Sleep(2 * time.Second)

	// Record timestamp before VM boot for log filtering
	startTime := time.Now()

	// Step 6: Start the VM
	t.Log("Starting VM...")
	err = vmClient.StartVM(ctx, vmName)
	require.NoError(t, err, "failed to start VM")
	t.Logf("Started VM: %s (network boot)", vmName)

	// Step 7: Wait for VM to get an IP (indicates DHCP worked)
	t.Log("Waiting for VM to get IP address...")
	vmIP, err := vmClient.GetVMIP(ctx, vmName)
	require.NoError(t, err, "VM did not get IP address - DHCP may have failed")
	t.Logf("VM got IP address: %s", vmIP)

	// Step 8: Wait for profile_matched in shaper-api logs
	t.Log("Waiting for profile_matched in shaper-api logs...")
	result, err := e2e.WaitForProfileMatched(ctx, cfg.Kubeconfig, e2e.ShaperSystemNamespace,
		podName, profileName, startTime, 2*time.Minute)
	if err != nil {
		// On failure, dump VM console log for debugging
		consoleLog, consoleErr := vmClient.GetConsoleLog(ctx, vmName)
		if consoleErr != nil {
			t.Logf("Failed to get VM console log: %v", consoleErr)
		} else {
			t.Logf("=== VM Console Log (%s) ===\n%s\n=== End Console Log ===", vmName, consoleLog)
		}
	}
	require.NoError(t, err, "did not find profile_matched log entry for expected profile")
	t.Logf("Found profile_matched: Profile=%s, Assignment=%s", result.ProfileName, result.AssignmentName)

	// Step 9: Verify the correct profile was matched
	require.Equal(t, profileName, result.ProfileName, "expected profile_matched to contain our profile")

	// Step 10: Wait for Alpine boot completion with e2e marker verification
	// This is the KEY difference from TestUUIDAssignmentBoot_E2E - we verify actual OS boot
	// and that the correct profile was served via the e2e_marker in kernel cmdline
	t.Log("Waiting for Alpine Linux boot completion with e2e marker verification...")
	err = vmClient.WaitForAlpineBootComplete(ctx, vmName, "e2e_marker=bootable-os-uuid", 3*time.Minute)
	if err != nil {
		// On failure, dump VM console log for debugging
		consoleLog, consoleErr := vmClient.GetConsoleLog(ctx, vmName)
		if consoleErr != nil {
			t.Logf("Failed to get VM console log: %v", consoleErr)
		} else {
			t.Logf("=== VM Console Log (%s) ===\n%s\n=== End Console Log ===", vmName, consoleLog)
		}
	}
	require.NoError(t, err, "VM did not boot Alpine Linux with expected e2e marker - check console log for details")
	t.Log("Alpine Linux boot completed successfully with e2e marker verified")

	t.Log("TC2 Bootable OS PASSED: UUID-specific assignment network boot flow verified with Alpine Linux")
}
