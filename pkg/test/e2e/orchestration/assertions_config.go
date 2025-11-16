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

// ErrConfigNotRetrieved indicates config was not retrieved from shaper-API
var ErrConfigNotRetrieved = errors.New("config not retrieved")

// ConfigRetrievedValidator validates that a VM retrieved config from shaper-API /config/{uuid}
type ConfigRetrievedValidator struct {
	pollInterval time.Duration
}

// NewConfigRetrievedValidator creates a new config retrieved validator
func NewConfigRetrievedValidator(pollInterval time.Duration) *ConfigRetrievedValidator {
	if pollInterval == 0 {
		pollInterval = 2 * time.Second
	}
	return &ConfigRetrievedValidator{
		pollInterval: pollInterval,
	}
}

// Validate checks if the VM retrieved config from shaper-API /config/{uuid} endpoint
// It polls shaper-API pod logs until:
// - A config_retrieved log entry is found with matching config_uuid
// - The context timeout is reached
//
// Expected log format (from internal/controller/content.go):
// level=INFO msg=config_retrieved config_uuid=<uuid> content_type=<type> size_bytes=<size>
func (v *ConfigRetrievedValidator) Validate(
	ctx context.Context,
	assertion scenario.AssertionSpec,
	vm *VMInstance,
	infra *infrastructure.InfrastructureState,
) (*AssertionResult, error) {
	startTime := time.Now()

	result := &AssertionResult{
		Type:     assertion.Type,
		Expected: "Config retrieved via /config/{uuid}",
	}

	// Extract expected config UUID from assertion spec if provided
	expectedUUID := assertion.Expected
	if expectedUUID == "" {
		// If no specific UUID expected, just verify any config was retrieved
		expectedUUID = "*"
	}

	// Poll for config retrieval in shaper-API logs
	ticker := time.NewTicker(v.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			result.Duration = time.Since(startTime)
			result.Passed = false
			result.Actual = "Config not retrieved"
			result.Message = fmt.Sprintf("Timeout waiting for config retrieval from VM %s", vm.Spec.Name)
			return result, nil

		case <-ticker.C:
			// Check shaper-API logs for config retrieval
			logEntry, found, err := v.findConfigRetrievalInLogs(ctx, infra, expectedUUID)
			if err != nil {
				// Don't fail immediately on log read errors, keep retrying
				continue
			}

			if found {
				result.Duration = time.Since(startTime)
				result.Passed = true
				result.Actual = fmt.Sprintf("Config retrieved: %s", logEntry)
				result.Message = fmt.Sprintf("VM %s retrieved config from /config/{uuid}", vm.Spec.Name)
				return result, nil
			}
		}
	}
}

// findConfigRetrievalInLogs searches shaper-API pod logs for config retrieval events
// Returns the log entry, whether it was found, and any error
func (v *ConfigRetrievedValidator) findConfigRetrievalInLogs(
	ctx context.Context,
	infra *infrastructure.InfrastructureState,
	expectedUUID string,
) (string, bool, error) {
	// Get shaper-API pod logs using kubectl
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

	// Parse logs looking for config_retrieved
	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		line := scanner.Text()

		// Look for structured log entry: msg=config_retrieved config_uuid=<uuid>
		if strings.Contains(line, "config_retrieved") {
			// Parse structured log for config_uuid
			if v.logEntryMatchesUUID(line, expectedUUID) {
				return line, true, nil
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", false, fmt.Errorf("error scanning logs: %w", err)
	}

	return "", false, nil
}

// logEntryMatchesUUID checks if a log entry matches the expected config UUID
func (v *ConfigRetrievedValidator) logEntryMatchesUUID(logLine string, expectedUUID string) bool {
	// If wildcard, accept any config_retrieved entry
	if expectedUUID == "*" {
		return true
	}

	// Try to parse as JSON (slog JSON format)
	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(logLine), &logEntry); err == nil {
		// JSON format
		if msg, ok := logEntry["msg"].(string); ok && msg == "config_retrieved" {
			if configUUID, ok := logEntry["config_uuid"].(string); ok {
				return configUUID == expectedUUID
			}
		}
		return false
	}

	// Try text format: msg=config_retrieved config_uuid=<uuid> content_type=<type>
	// This is the format from slog's default text handler
	if strings.Contains(logLine, "config_retrieved") {
		// Extract config_uuid field
		parts := strings.Fields(logLine)
		for _, part := range parts {
			if strings.HasPrefix(part, "config_uuid=") {
				uuid := strings.TrimPrefix(part, "config_uuid=")
				return uuid == expectedUUID
			}
		}
	}

	return false
}
