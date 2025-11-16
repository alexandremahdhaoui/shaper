//go:build e2e

package orchestration

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e/infrastructure"
	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e/scenario"
)

var (
	// ErrHTTPBootNotFound indicates HTTP boot request was not found in shaper-API logs
	ErrHTTPBootNotFound = errors.New("HTTP boot request not found")
	// ErrShaperAPILogNotFound indicates shaper-API logs not accessible
	ErrShaperAPILogNotFound = errors.New("shaper-API logs not found")
	// ErrKubectlFailed indicates kubectl command failed
	ErrKubectlFailed = errors.New("kubectl command failed")
)

// HTTPBootValidator validates that a VM called the shaper-API HTTP boot endpoint
type HTTPBootValidator struct {
	pollInterval time.Duration
}

// NewHTTPBootValidator creates a new HTTP boot validator
func NewHTTPBootValidator(pollInterval time.Duration) *HTTPBootValidator {
	if pollInterval == 0 {
		pollInterval = 2 * time.Second
	}
	return &HTTPBootValidator{
		pollInterval: pollInterval,
	}
}

// Validate checks if the VM called the shaper-API /boot.ipxe or /ipxe endpoint
// It polls shaper-API pod logs until:
// - An HTTP boot request is found from the VM with matching UUID/buildarch
// - The context timeout is reached
//
// Expected log format (from internal/driver/server/server.go):
// level=INFO msg=ipxe_boot_request uuid=<uuid> buildarch=<arch>
func (v *HTTPBootValidator) Validate(
	ctx context.Context,
	assertion scenario.AssertionSpec,
	vm *VMInstance,
	infra *infrastructure.InfrastructureState,
) (*AssertionResult, error) {
	startTime := time.Now()

	result := &AssertionResult{
		Type:     assertion.Type,
		Expected: "HTTP boot endpoint called",
	}

	// Poll for HTTP boot request in shaper-API logs
	ticker := time.NewTicker(v.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			result.Duration = time.Since(startTime)
			result.Passed = false
			result.Actual = "HTTP boot request not found"
			result.Message = fmt.Sprintf("Timeout waiting for HTTP boot request from VM %s (UUID: %s)", vm.Spec.Name, vm.Spec.UUID)
			return result, nil

		case <-ticker.C:
			// Check shaper-API logs for HTTP boot request
			logEntry, found, err := v.findHTTPBootRequestInLogs(ctx, infra, vm)
			if err != nil {
				// Don't fail immediately on log read errors, keep retrying
				continue
			}

			if found {
				result.Duration = time.Since(startTime)
				result.Passed = true
				result.Actual = fmt.Sprintf("HTTP boot request found: %s", logEntry)
				result.Message = fmt.Sprintf("VM %s called HTTP boot endpoint with UUID %s", vm.Spec.Name, vm.Spec.UUID)
				return result, nil
			}
		}
	}
}

// findHTTPBootRequestInLogs searches shaper-API pod logs for HTTP boot requests
// Returns the log entry, whether it was found, and any error
func (v *HTTPBootValidator) findHTTPBootRequestInLogs(
	ctx context.Context,
	infra *infrastructure.InfrastructureState,
	vm *VMInstance,
) (string, bool, error) {
	// Get shaper-API pod logs using kubectl
	// kubectl --kubeconfig <path> -n shaper-system logs -l app=shaper-api --tail=100
	cmd := exec.CommandContext(ctx,
		"kubectl",
		"--kubeconfig", infra.Kubeconfig,
		"-n", "shaper-system",
		"logs",
		"-l", "app=shaper-api",
		"--tail=500",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", false, fmt.Errorf("%w: %v (stderr: %s)", ErrKubectlFailed, err, stderr.String())
	}

	// Parse logs looking for ipxe_boot_request with matching UUID
	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		line := scanner.Text()

		// Look for structured log entry: msg=ipxe_boot_request uuid=<uuid>
		if strings.Contains(line, "ipxe_boot_request") {
			// Parse structured log for UUID
			if v.logEntryMatchesVM(line, vm) {
				return line, true, nil
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", false, fmt.Errorf("error scanning logs: %w", err)
	}

	return "", false, nil
}

// logEntryMatchesVM checks if a log entry matches the VM's UUID or other identifiers
func (v *HTTPBootValidator) logEntryMatchesVM(logLine string, vm *VMInstance) bool {
	// Try to parse as JSON (slog JSON format)
	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(logLine), &logEntry); err == nil {
		// JSON format
		if msg, ok := logEntry["msg"].(string); ok && msg == "ipxe_boot_request" {
			if uuid, ok := logEntry["uuid"].(string); ok {
				return uuid == vm.Spec.UUID
			}
		}
		return false
	}

	// Try text format: msg=ipxe_boot_request uuid=<uuid> buildarch=<arch>
	// This is the format from slog's default text handler
	if strings.Contains(logLine, "ipxe_boot_request") {
		// Extract uuid field
		parts := strings.Fields(logLine)
		for _, part := range parts {
			if strings.HasPrefix(part, "uuid=") {
				uuid := strings.TrimPrefix(part, "uuid=")
				return uuid == vm.Spec.UUID
			}
		}
	}

	return false
}
