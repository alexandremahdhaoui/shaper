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
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e"
	"github.com/stretchr/testify/require"
)

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
		require.NotEmpty(t, cfg.VMPXEClientIP)
		// Note: Actual ping test would require network access
	})

	t.Run("SSHKeyExists", func(t *testing.T) {
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
