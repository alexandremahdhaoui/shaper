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

// ErrProfileMatchNotFound indicates expected profile match was not found in logs
var ErrProfileMatchNotFound = errors.New("profile match not found")

// ProfileMatchValidator validates that the correct Profile was returned for a VM
type ProfileMatchValidator struct {
	pollInterval time.Duration
}

// NewProfileMatchValidator creates a new profile match validator
func NewProfileMatchValidator(pollInterval time.Duration) *ProfileMatchValidator {
	if pollInterval == 0 {
		pollInterval = 2 * time.Second
	}
	return &ProfileMatchValidator{
		pollInterval: pollInterval,
	}
}

// Validate checks if the expected Profile was returned for the VM
// It polls shaper-API pod logs until:
// - A profile_matched log entry is found with the expected profile name
// - The context timeout is reached
//
// Expected log format (from internal/controller/ipxe.go):
// level=INFO msg=profile_matched profile_name=<name> profile_namespace=<ns> assignment=<assignment-name>
func (v *ProfileMatchValidator) Validate(
	ctx context.Context,
	assertion scenario.AssertionSpec,
	vm *VMInstance,
	infra *infrastructure.InfrastructureState,
) (*AssertionResult, error) {
	startTime := time.Now()

	result := &AssertionResult{
		Type:     assertion.Type,
		Expected: fmt.Sprintf("Profile: %s", assertion.Expected),
	}

	// Poll for profile match in shaper-API logs
	ticker := time.NewTicker(v.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			result.Duration = time.Since(startTime)
			result.Passed = false
			result.Actual = "Profile match not found"
			result.Message = fmt.Sprintf("Timeout waiting for Profile %s to be matched for VM %s", assertion.Expected, vm.Spec.Name)
			return result, nil

		case <-ticker.C:
			// Check shaper-API logs for profile_matched entry
			profileName, found, err := v.findProfileMatchInLogs(ctx, infra, vm)
			if err != nil {
				// Don't fail immediately on log read errors, keep retrying
				continue
			}

			if found {
				result.Actual = fmt.Sprintf("Profile: %s", profileName)
				result.Duration = time.Since(startTime)

				// Check if it matches expected profile
				if profileName == assertion.Expected {
					result.Passed = true
					result.Message = fmt.Sprintf("VM %s matched expected Profile %s", vm.Spec.Name, assertion.Expected)
				} else {
					result.Passed = false
					result.Message = fmt.Sprintf("VM %s matched Profile %s, expected %s", vm.Spec.Name, profileName, assertion.Expected)
				}
				return result, nil
			}
		}
	}
}

// findProfileMatchInLogs searches shaper-API pod logs for profile_matched entries
// Returns the matched profile name, whether it was found, and any error
func (v *ProfileMatchValidator) findProfileMatchInLogs(
	ctx context.Context,
	infra *infrastructure.InfrastructureState,
	vm *VMInstance,
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

	// Parse logs looking for profile_matched entries
	// We need to correlate with the VM's UUID from earlier ipxe_boot_request
	scanner := bufio.NewScanner(&stdout)

	// Strategy: Find the most recent profile_matched entry after an ipxe_boot_request for this VM
	// This is a simplified approach - in production, we'd use request IDs for proper correlation
	var lastIPXERequestLine int
	var lines []string
	lineNum := 0

	// First pass: collect all lines and find last ipxe_boot_request for this VM
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)

		if v.isIPXERequestForVM(line, vm) {
			lastIPXERequestLine = lineNum
		}
		lineNum++
	}

	if err := scanner.Err(); err != nil {
		return "", false, fmt.Errorf("error scanning logs: %w", err)
	}

	// Second pass: Find profile_matched entry after the last ipxe_boot_request
	for i := lastIPXERequestLine; i < len(lines); i++ {
		line := lines[i]
		if strings.Contains(line, "profile_matched") {
			// Extract profile_name from structured log
			if profileName := v.extractProfileName(line); profileName != "" {
				return profileName, true, nil
			}
		}
	}

	return "", false, nil
}

// isIPXERequestForVM checks if a log line is an ipxe_boot_request for the given VM
func (v *ProfileMatchValidator) isIPXERequestForVM(logLine string, vm *VMInstance) bool {
	if !strings.Contains(logLine, "ipxe_boot_request") {
		return false
	}

	// Try JSON format
	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(logLine), &logEntry); err == nil {
		if uuid, ok := logEntry["uuid"].(string); ok {
			return uuid == vm.Spec.UUID
		}
		return false
	}

	// Try text format
	parts := strings.Fields(logLine)
	for _, part := range parts {
		if strings.HasPrefix(part, "uuid=") {
			uuid := strings.TrimPrefix(part, "uuid=")
			return uuid == vm.Spec.UUID
		}
	}

	return false
}

// extractProfileName extracts the profile_name from a profile_matched log entry
func (v *ProfileMatchValidator) extractProfileName(logLine string) string {
	// Try JSON format
	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(logLine), &logEntry); err == nil {
		if profileName, ok := logEntry["profile_name"].(string); ok {
			return profileName
		}
		return ""
	}

	// Try text format: msg=profile_matched profile_name=<name> ...
	parts := strings.Fields(logLine)
	for _, part := range parts {
		if strings.HasPrefix(part, "profile_name=") {
			return strings.TrimPrefix(part, "profile_name=")
		}
	}

	return ""
}
