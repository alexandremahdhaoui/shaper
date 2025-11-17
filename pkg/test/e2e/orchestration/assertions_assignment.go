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

// ErrAssignmentMatchNotFound indicates expected assignment match was not found in logs
var ErrAssignmentMatchNotFound = errors.New("assignment match not found")

// AssignmentMatchValidator validates that the correct Assignment was matched for a VM
type AssignmentMatchValidator struct {
	pollInterval time.Duration
}

// NewAssignmentMatchValidator creates a new assignment match validator
func NewAssignmentMatchValidator(pollInterval time.Duration) *AssignmentMatchValidator {
	if pollInterval == 0 {
		pollInterval = 2 * time.Second
	}
	return &AssignmentMatchValidator{
		pollInterval: pollInterval,
	}
}

// Validate checks if the expected Assignment was matched for the VM
// It polls shaper-API pod logs until:
// - An assignment_selected log entry is found with the expected assignment name
// - The context timeout is reached
//
// Expected log format (from internal/controller/ipxe.go):
// level=INFO msg=assignment_selected assignment_name=<name> assignment_namespace=<ns> subject_selectors=<selectors> matched_by=<default|selectors>
func (v *AssignmentMatchValidator) Validate(
	ctx context.Context,
	assertion scenario.AssertionSpec,
	vm *VMInstance,
	infra *infrastructure.InfrastructureState,
) (*AssertionResult, error) {
	startTime := time.Now()

	result := &AssertionResult{
		Type:     assertion.Type,
		Expected: fmt.Sprintf("Assignment: %s", assertion.Expected),
	}

	// Poll for assignment selection in shaper-API logs
	ticker := time.NewTicker(v.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			result.Duration = time.Since(startTime)
			result.Passed = false
			result.Actual = "Assignment match not found"
			result.Message = fmt.Sprintf("Timeout waiting for Assignment %s to be matched for VM %s", assertion.Expected, vm.Spec.Name)
			return result, nil

		case <-ticker.C:
			// Check shaper-API logs for assignment_selected entry
			assignmentName, found, err := v.findAssignmentMatchInLogs(ctx, infra, vm)
			if err != nil {
				// Don't fail immediately on log read errors, keep retrying
				continue
			}

			if found {
				result.Actual = fmt.Sprintf("Assignment: %s", assignmentName)
				result.Duration = time.Since(startTime)

				// Check if it matches expected assignment
				if assignmentName == assertion.Expected {
					result.Passed = true
					result.Message = fmt.Sprintf("VM %s matched expected Assignment %s", vm.Spec.Name, assertion.Expected)
				} else {
					result.Passed = false
					result.Message = fmt.Sprintf("VM %s matched Assignment %s, expected %s", vm.Spec.Name, assignmentName, assertion.Expected)
				}
				return result, nil
			}
		}
	}
}

// findAssignmentMatchInLogs searches shaper-API pod logs for assignment_selected entries
// Returns the matched assignment name, whether it was found, and any error
func (v *AssignmentMatchValidator) findAssignmentMatchInLogs(
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

	// Parse logs looking for assignment_selected entries
	// We need to correlate with the VM's UUID from earlier ipxe_boot_request
	scanner := bufio.NewScanner(&stdout)

	// Strategy: Find the most recent assignment_selected entry after an ipxe_boot_request for this VM
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

	// Second pass: Find assignment_selected entry after the last ipxe_boot_request
	for i := lastIPXERequestLine; i < len(lines); i++ {
		line := lines[i]
		if strings.Contains(line, "assignment_selected") {
			// Extract assignment_name from structured log
			if assignmentName := v.extractAssignmentName(line); assignmentName != "" {
				return assignmentName, true, nil
			}
		}
	}

	return "", false, nil
}

// isIPXERequestForVM checks if a log line is an ipxe_boot_request for the given VM
func (v *AssignmentMatchValidator) isIPXERequestForVM(logLine string, vm *VMInstance) bool {
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

// extractAssignmentName extracts the assignment_name from an assignment_selected log entry
func (v *AssignmentMatchValidator) extractAssignmentName(logLine string) string {
	// Try JSON format
	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(logLine), &logEntry); err == nil {
		if assignmentName, ok := logEntry["assignment_name"].(string); ok {
			return assignmentName
		}
		return ""
	}

	// Try text format: msg=assignment_selected assignment_name=<name> ...
	parts := strings.Fields(logLine)
	for _, part := range parts {
		if strings.HasPrefix(part, "assignment_name=") {
			return strings.TrimPrefix(part, "assignment_name=")
		}
	}

	return ""
}
