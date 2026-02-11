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

package e2e

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"
)

const (
	// ShaperAPINodePort is the NodePort used by shaper-api service.
	// This must match the nodePort configured in forge.yaml for shaper-api.
	ShaperAPINodePort = 30080

	// ShaperAPIServicePort is the port the shaper-api service listens on.
	// This is config.apiServer.port in values.yaml (default: 30443).
	ShaperAPIServicePort = 30443
)

var (
	// ErrPortForwardStart indicates the port-forward could not be started.
	ErrPortForwardStart = errors.New("failed to start port-forward")
	// ErrPortForwardNotReady indicates the port-forward is not ready.
	ErrPortForwardNotReady = errors.New("port-forward not ready")
	// ErrContentFetch indicates a failure to fetch content from the /content/{uuid} endpoint.
	ErrContentFetch = errors.New("failed to fetch content")
)

// PortForward represents an active kubectl port-forward process.
// When args and stopCh are set (via SetupGlobalPortForward), it auto-reconnects
// if the kubectl process dies (e.g. pod sandbox loss, Helm upgrades).
type PortForward struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	cancel context.CancelFunc
	Port   int
	URL    string

	// Auto-reconnect fields (only set for global port-forward)
	args   []string
	stopCh chan struct{}
	// stderr is captured at creation time to avoid data races with the
	// testing framework which may reassign os.Stderr during test execution.
	stderr io.Writer
}

// Stop terminates the port-forward process and its auto-reconnect goroutine.
func (pf *PortForward) Stop() {
	if pf.stopCh != nil {
		select {
		case <-pf.stopCh:
		default:
			close(pf.stopCh)
		}
	}

	pf.mu.Lock()
	defer pf.mu.Unlock()

	if pf.cancel != nil {
		pf.cancel()
	}
	if pf.cmd != nil && pf.cmd.Process != nil {
		_ = pf.cmd.Process.Kill()
		// Only call cmd.Wait() here if there's no auto-reconnect goroutine.
		// When auto-reconnect is active, its goroutine calls cmd.Wait() and
		// calling it concurrently here would race on exec.Cmd internal state.
		if pf.stopCh == nil {
			_ = pf.cmd.Wait()
		}
	}
}

// startAutoReconnect monitors the kubectl process and restarts it if it dies.
// This handles pod sandbox loss, pod restarts from Helm upgrades, etc.
func (pf *PortForward) startAutoReconnect() {
	go func() {
		for {
			pf.mu.Lock()
			cmd := pf.cmd
			pf.mu.Unlock()

			if cmd != nil && cmd.Process != nil {
				_ = cmd.Wait()
			}

			select {
			case <-pf.stopCh:
				return
			default:
			}

			select {
			case <-time.After(2 * time.Second):
			case <-pf.stopCh:
				return
			}

			pf.mu.Lock()
			if pf.cancel != nil {
				pf.cancel()
			}
			ctx, cancel := context.WithCancel(context.Background())
			newCmd := exec.CommandContext(ctx, "kubectl", pf.args...)
			newCmd.Stderr = pf.stderr
			if err := newCmd.Start(); err != nil {
				cancel()
				pf.mu.Unlock()
				_, _ = fmt.Fprintf(pf.stderr, "port-forward auto-reconnect: restart failed: %v\n", err)
				select {
				case <-time.After(3 * time.Second):
				case <-pf.stopCh:
					return
				}
				continue
			}
			pf.cancel = cancel
			pf.cmd = newCmd
			pf.mu.Unlock()

			_, _ = fmt.Fprintf(pf.stderr, "port-forward auto-reconnect: restarted (PID %d)\n", newCmd.Process.Pid)
		}
	}()
}

// SetupGlobalPortForward sets up kubectl port-forward to shaper-api service
// on a random available port for the test host to verify API endpoints.
// VMs reach shaper-api via kube-proxy NodePort (192.168.100.1:30080) directly,
// so this port-forward is only needed for localhost test verification.
//
// The port-forward must be started BEFORE any tests that call shaper-api.
func SetupGlobalPortForward(kubeconfig string) (*PortForward, error) {
	// Capture stderr now to avoid data races with the testing framework
	// which may reassign os.Stderr during test execution.
	stderr := os.Stderr

	// Find an available port to avoid conflicting with kube-proxy NodePort on 30080
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, errors.Join(ErrPortForwardStart, fmt.Errorf("failed to find available port: %w", err))
	}
	localPort := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	args := []string{
		"--kubeconfig", kubeconfig,
		"port-forward",
		"-n", ShaperSystemNamespace,
		"svc/shaper-api",
		fmt.Sprintf("%d:%d", localPort, ShaperAPIServicePort),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, errors.Join(ErrPortForwardStart, err)
	}

	pf := &PortForward{
		cmd:    cmd,
		cancel: cancel,
		Port:   localPort,
		URL:    fmt.Sprintf("http://localhost:%d", localPort),
		args:   args,
		stopCh: make(chan struct{}),
		stderr: stderr,
	}

	pf.startAutoReconnect()

	return pf, nil
}

// WaitForPortForwardReady waits for the port-forward to be ready by checking
// if the boot.ipxe endpoint responds successfully.
func WaitForPortForwardReady(pf *PortForward, timeout time.Duration) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Check the boot.ipxe endpoint to verify API server is ready
		resp, err := client.Get(pf.URL + "/boot.ipxe")
		if err == nil {
			defer func() { _ = resp.Body.Close() }()
			// Read and discard body to allow connection reuse
			_, _ = io.Copy(io.Discard, resp.Body)

			// boot.ipxe returns 200 OK when ready
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	return errors.Join(ErrPortForwardNotReady,
		fmt.Errorf("timeout after %v waiting for port-forward at %s", timeout, pf.URL))
}

// BridgeGatewayIP is the IP address of the libvirt NAT gateway.
// VMs on the TestNetwork see this as the host IP (192.168.100.1).
const BridgeGatewayIP = "192.168.100.1"

// VerifyBridgeAccess checks if VMs can reach shaper-api via the bridge gateway IP
// and NodePort. VMs chainload iPXE to http://192.168.100.1:30080/boot.ipxe via
// kube-proxy NodePort, not via the test port-forward.
func VerifyBridgeAccess(_ *PortForward) error {
	bridgeURL := fmt.Sprintf("http://%s:%d/boot.ipxe", BridgeGatewayIP, ShaperAPINodePort)
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(bridgeURL)
	if err != nil {
		return fmt.Errorf("bridge access failed at %s: %w (kube-proxy NodePort may not be ready)", bridgeURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bridge access returned %d at %s", resp.StatusCode, bridgeURL)
	}

	return nil
}

// WaitForIPXEEndpointReady polls the /ipxe endpoint until it returns a successful response.
// This is used to verify that shaper-api's cache has synced the Assignment before starting VMs.
// The function uses a test UUID that won't match any real Assignment, so it relies on
// the default assignment being present for the given buildarch.
func WaitForIPXEEndpointReady(ctx context.Context, baseURL, buildarch string) error {
	// Use a random UUID that won't match any specific assignment,
	// but should be served by the default assignment
	testUUID := "00000000-0000-0000-0000-000000000001"
	return WaitForIPXEEndpointReadyWithUUID(ctx, baseURL, testUUID, buildarch)
}

// WaitForIPXEEndpointReadyWithUUID polls the /ipxe endpoint with a specific UUID
// until it returns a successful response. This verifies that shaper-api's cache
// has synced and can serve the Assignment for the given UUID.
func WaitForIPXEEndpointReadyWithUUID(ctx context.Context, baseURL, vmUUID, buildarch string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	pollInterval := 1 * time.Second
	url := fmt.Sprintf("%s/ipxe?uuid=%s&buildarch=%s", baseURL, vmUUID, buildarch)

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for /ipxe endpoint to be ready at %s", url)
		default:
		}

		resp, err := client.Get(url)
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				// Check that response contains iPXE shebang (valid response)
				if len(body) > 0 && string(body[:6]) == "#!ipxe" {
					return nil
				}
			}
			// Continue polling - cache may not be synced yet (4xx/5xx expected before cache syncs)
		}

		time.Sleep(pollInterval)
	}
}

// SetupGlobalPortForwardWithWait sets up port-forward and waits for it to be ready.
// This is a convenience function that combines SetupGlobalPortForward and WaitForPortForwardReady.
func SetupGlobalPortForwardWithWait(kubeconfig string, timeout time.Duration) (*PortForward, error) {
	pf, err := SetupGlobalPortForward(kubeconfig)
	if err != nil {
		return nil, err
	}

	if err := WaitForPortForwardReady(pf, timeout); err != nil {
		pf.Stop()
		return nil, err
	}

	return pf, nil
}

// SetupMTLSPortForward sets up kubectl port-forward for mTLS testing.
// It forwards localPort on 0.0.0.0 to the shaper-api service on servicePort.
// This is necessary because VMs need to reach the mTLS endpoint via the bridge IP.
func SetupMTLSPortForward(kubeconfig string, localPort, servicePort int) (*PortForward, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Set up port-forward to API service for mTLS
	// --address 0.0.0.0 makes it listen on all interfaces including 192.168.100.1
	cmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", kubeconfig,
		"port-forward",
		"--address", "0.0.0.0",
		"-n", ShaperSystemNamespace,
		"svc/shaper-api",
		fmt.Sprintf("%d:%d", localPort, servicePort),
	)

	// Capture stderr for debugging
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, errors.Join(ErrPortForwardStart, err)
	}

	pf := &PortForward{
		cmd:    cmd,
		cancel: cancel,
		Port:   localPort,
		URL:    fmt.Sprintf("https://localhost:%d", localPort),
	}

	return pf, nil
}

// WaitForMTLSPortForwardReady waits for the mTLS port-forward to be ready by checking
// if the port is accepting TCP connections. Unlike WaitForPortForwardReady, this
// doesn't make HTTP requests since that would require valid TLS client certificates.
func WaitForMTLSPortForwardReady(pf *PortForward, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	pollInterval := 100 * time.Millisecond

	for time.Now().Before(deadline) {
		// Try to connect to the port
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", pf.Port), 1*time.Second)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(pollInterval)
	}

	return errors.Join(ErrPortForwardNotReady,
		fmt.Errorf("mTLS port-forward not accepting connections on port %d after %v", pf.Port, timeout))
}

// FetchContent fetches content from the /content/{uuid} endpoint.
// The buildarch parameter is required by the API.
// Returns the response body bytes on 200 OK, or an error with status code on failure.
func FetchContent(ctx context.Context, baseURL, uuid, buildarch string) ([]byte, error) {
	url := fmt.Sprintf("%s/content/%s?buildarch=%s", baseURL, uuid, buildarch)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Join(ErrContentFetch, err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Join(ErrContentFetch, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Join(ErrContentFetch, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Join(ErrContentFetch, fmt.Errorf("got status %d for %s", resp.StatusCode, url))
	}

	return body, nil
}
