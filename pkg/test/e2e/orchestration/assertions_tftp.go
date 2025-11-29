package orchestration

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e/infrastructure"
	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e/scenario"
)

var (
	// ErrTFTPBootNotFound indicates TFTP boot request was not found in dnsmasq logs
	ErrTFTPBootNotFound = errors.New("TFTP boot request not found")
	// ErrDnsmasqLogNotFound indicates dnsmasq log file or journal not accessible
	ErrDnsmasqLogNotFound = errors.New("dnsmasq log not found")
)

// TFTPBootValidator validates that a VM fetched boot files via TFTP
type TFTPBootValidator struct {
	pollInterval time.Duration
}

// NewTFTPBootValidator creates a new TFTP boot validator
func NewTFTPBootValidator(pollInterval time.Duration) *TFTPBootValidator {
	if pollInterval == 0 {
		pollInterval = 2 * time.Second
	}
	return &TFTPBootValidator{
		pollInterval: pollInterval,
	}
}

// Validate checks if the VM fetched TFTP boot files from dnsmasq
// It polls dnsmasq logs until:
// - A TFTP request is found from the VM's IP address
// - The context timeout is reached
//
// Dnsmasq TFTP log format (when --log-queries --log-dhcp enabled):
// dnsmasq-tftp[PID]: sent /path/to/file.ipxe to 192.168.100.100
// dnsmasq-tftp[PID]: file /path/to/file.ipxe not found
func (v *TFTPBootValidator) Validate(
	ctx context.Context,
	assertion scenario.AssertionSpec,
	vm *VMInstance,
	infra *infrastructure.InfrastructureState,
) (*AssertionResult, error) {
	startTime := time.Now()

	result := &AssertionResult{
		Type:     assertion.Type,
		Expected: "TFTP boot file fetched",
	}

	// Get VM IP from metadata (populated after DHCP)
	vmIP := ""
	if vm.Metadata != nil && vm.Metadata.IP != "" {
		vmIP = vm.Metadata.IP
	}

	// If we don't have VM IP yet, we need to wait for DHCP first
	if vmIP == "" {
		result.Duration = time.Since(startTime)
		result.Passed = false
		result.Actual = "VM IP not available"
		result.Message = fmt.Sprintf("Cannot validate TFTP boot for VM %s: IP address not available (DHCP may not have completed)", vm.Spec.Name)
		return result, fmt.Errorf("VM IP not available for %s", vm.Spec.Name)
	}

	// Poll for TFTP boot request in logs
	ticker := time.NewTicker(v.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			result.Duration = time.Since(startTime)
			result.Passed = false
			result.Actual = "TFTP boot request not found"
			result.Message = fmt.Sprintf("Timeout waiting for TFTP boot request from VM %s (IP: %s)", vm.Spec.Name, vmIP)
			return result, nil

		case <-ticker.C:
			// Check dnsmasq logs for TFTP requests
			logEntry, found, err := v.findTFTPRequestInLogs(infra.DnsmasqID, vmIP)
			if err != nil {
				// Don't fail immediately on log read errors, keep retrying
				continue
			}

			if found {
				result.Duration = time.Since(startTime)
				result.Passed = true
				result.Actual = fmt.Sprintf("TFTP boot file fetched: %s", logEntry)
				result.Message = fmt.Sprintf("VM %s fetched TFTP boot file from %s", vm.Spec.Name, vmIP)
				return result, nil
			}
		}
	}
}

// findTFTPRequestInLogs searches dnsmasq logs for TFTP requests from the VM's IP
// Returns the log entry, whether it was found, and any error
func (v *TFTPBootValidator) findTFTPRequestInLogs(dnsmasqID, vmIP string) (string, bool, error) {
	// Try to read dnsmasq logs from journalctl (systemd)
	// Command: journalctl -u dnsmasq-<id> --no-pager
	cmd := exec.Command("journalctl", "-u", dnsmasqID, "--no-pager", "--since", "10 minutes ago")
	output, err := cmd.Output()
	if err != nil {
		// If journalctl fails, try reading from syslog
		return v.findTFTPRequestInSyslog(vmIP)
	}

	// Parse journalctl output
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()

		// Look for TFTP-related log entries
		// Format: "dnsmasq-tftp[PID]: sent /path/to/file to <IP>"
		if strings.Contains(line, "dnsmasq-tftp") &&
			strings.Contains(line, vmIP) &&
			(strings.Contains(line, "sent") || strings.Contains(line, "file")) {
			return line, true, nil
		}
	}

	return "", false, nil
}

// findTFTPRequestInSyslog searches /var/log/syslog for TFTP requests
func (v *TFTPBootValidator) findTFTPRequestInSyslog(vmIP string) (string, bool, error) {
	syslogPath := "/var/log/syslog"

	// Check if syslog exists
	if _, err := os.Stat(syslogPath); os.IsNotExist(err) {
		return "", false, ErrDnsmasqLogNotFound
	}

	file, err := os.Open(syslogPath)
	if err != nil {
		return "", false, fmt.Errorf("%w: %v", ErrDnsmasqLogNotFound, err)
	}
	defer func() { _ = file.Close() }()

	// Read last 1000 lines (avoid reading entire syslog)
	// This is a simple implementation - for production, use tail or similar
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		if len(lines) > 10000 {
			lines = lines[1:] // Keep only last 10000 lines
		}
	}

	if err := scanner.Err(); err != nil {
		return "", false, fmt.Errorf("error reading syslog: %w", err)
	}

	// Search for TFTP requests
	for _, line := range lines {
		if strings.Contains(line, "dnsmasq-tftp") &&
			strings.Contains(line, vmIP) &&
			(strings.Contains(line, "sent") || strings.Contains(line, "file")) {
			return line, true, nil
		}
	}

	return "", false, nil
}
