package orchestration

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e/infrastructure"
	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e/scenario"
)

var (
	// ErrDHCPLeaseNotFound indicates DHCP lease was not found in lease file
	ErrDHCPLeaseNotFound = errors.New("DHCP lease not found")
	// ErrDHCPLeaseFileNotFound indicates dnsmasq lease file doesn't exist
	ErrDHCPLeaseFileNotFound = errors.New("DHCP lease file not found")
)

// AssertionResult represents the result of an assertion validation
type AssertionResult struct {
	Type     string
	Expected string
	Actual   string
	Passed   bool
	Message  string
	Duration time.Duration
}

// AssertionValidator interface defines assertion validation
type AssertionValidator interface {
	Validate(ctx context.Context, assertion scenario.AssertionSpec, vm *VMInstance, infra *infrastructure.InfrastructureState) (*AssertionResult, error)
}

// DHCPLeaseValidator validates that a VM obtained a DHCP lease
type DHCPLeaseValidator struct {
	pollInterval time.Duration
}

// NewDHCPLeaseValidator creates a new DHCP lease validator
func NewDHCPLeaseValidator(pollInterval time.Duration) *DHCPLeaseValidator {
	if pollInterval == 0 {
		pollInterval = 2 * time.Second
	}
	return &DHCPLeaseValidator{
		pollInterval: pollInterval,
	}
}

// Validate checks if the VM obtained a DHCP lease from dnsmasq
// It polls the dnsmasq lease file until:
// - A lease is found matching the VM's MAC address or hostname
// - The context timeout is reached
//
// The dnsmasq lease file format is:
// <expiry-time> <mac-address> <ip-address> <hostname> <client-id>
// Example: 1699999999 52:54:00:12:34:56 192.168.100.100 test-vm *
func (v *DHCPLeaseValidator) Validate(
	ctx context.Context,
	assertion scenario.AssertionSpec,
	vm *VMInstance,
	infra *infrastructure.InfrastructureState,
) (*AssertionResult, error) {
	startTime := time.Now()

	result := &AssertionResult{
		Type:     assertion.Type,
		Expected: "DHCP lease obtained",
	}

	// Construct lease file path
	leaseFilePath := filepath.Join(filepath.Dir(infra.TFTPRoot), "dnsmasq.leases")

	// Poll for DHCP lease
	ticker := time.NewTicker(v.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			result.Duration = time.Since(startTime)
			result.Passed = false
			result.Actual = "DHCP lease not found"
			result.Message = fmt.Sprintf("Timeout waiting for DHCP lease for VM %s (MAC: %s)", vm.Spec.Name, vm.Spec.MACAddress)
			return result, nil

		case <-ticker.C:
			// Check if lease file exists
			if _, err := os.Stat(leaseFilePath); os.IsNotExist(err) {
				continue // File doesn't exist yet, keep polling
			}

			// Read and parse lease file
			lease, found, err := v.findLeaseInFile(leaseFilePath, vm)
			if err != nil {
				result.Duration = time.Since(startTime)
				result.Passed = false
				result.Actual = fmt.Sprintf("Error reading lease file: %v", err)
				result.Message = fmt.Sprintf("Failed to read DHCP lease file: %v", err)
				return result, err
			}

			if found {
				result.Duration = time.Since(startTime)
				result.Passed = true
				result.Actual = fmt.Sprintf("DHCP lease obtained: %s", lease)
				result.Message = fmt.Sprintf("VM %s obtained DHCP lease: %s", vm.Spec.Name, lease)
				return result, nil
			}
		}
	}
}

// findLeaseInFile searches for a DHCP lease matching the VM's MAC address or hostname
// Returns the lease line, whether it was found, and any error
func (v *DHCPLeaseValidator) findLeaseInFile(leaseFilePath string, vm *VMInstance) (string, bool, error) {
	file, err := os.Open(leaseFilePath)
	if err != nil {
		return "", false, fmt.Errorf("%w: %v", ErrDHCPLeaseFileNotFound, err)
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Dnsmasq lease format: <expiry-time> <mac-address> <ip-address> <hostname> <client-id>
		// Example: 1699999999 52:54:00:12:34:56 192.168.100.100 test-vm *
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		macAddr := fields[1]
		hostname := fields[3]

		// Match by MAC address (primary)
		if vm.Spec.MACAddress != "" && strings.EqualFold(macAddr, vm.Spec.MACAddress) {
			return line, true, nil
		}

		// Match by hostname (secondary, if MAC not set)
		if vm.Spec.Name != "" && hostname == vm.Spec.Name {
			return line, true, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", false, fmt.Errorf("error scanning lease file: %w", err)
	}

	return "", false, nil
}
