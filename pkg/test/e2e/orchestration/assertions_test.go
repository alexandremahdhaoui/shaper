//go:build e2e

package orchestration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e/infrastructure"
	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e/scenario"
	"github.com/alexandremahdhaoui/shaper/pkg/vmm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDHCPLeaseValidator tests DHCP lease validation
func TestDHCPLeaseValidator(t *testing.T) {
	tests := []struct {
		name          string
		leaseFileData string
		vmMACAddress  string
		vmName        string
		timeout       time.Duration
		expectPass    bool
		expectError   bool
	}{
		{
			name: "lease found by MAC address",
			leaseFileData: `1699999999 52:54:00:12:34:56 192.168.100.100 test-vm-1 *
1699999999 52:54:00:12:34:57 192.168.100.101 test-vm-2 *`,
			vmMACAddress: "52:54:00:12:34:56",
			vmName:       "test-vm-1",
			timeout:      5 * time.Second,
			expectPass:   true,
			expectError:  false,
		},
		{
			name: "lease found by MAC address - case insensitive",
			leaseFileData: `1699999999 52:54:00:AB:CD:EF 192.168.100.100 test-vm *
`,
			vmMACAddress: "52:54:00:ab:cd:ef",
			vmName:       "test-vm",
			timeout:      5 * time.Second,
			expectPass:   true,
			expectError:  false,
		},
		{
			name: "lease found by hostname when MAC not set",
			leaseFileData: `1699999999 52:54:00:12:34:56 192.168.100.100 test-vm-hostname *
`,
			vmMACAddress: "", // MAC not set, should fall back to hostname
			vmName:       "test-vm-hostname",
			timeout:      5 * time.Second,
			expectPass:   true,
			expectError:  false,
		},
		{
			name:          "lease not found - timeout",
			leaseFileData: `1699999999 52:54:00:99:99:99 192.168.100.100 other-vm *`,
			vmMACAddress:  "52:54:00:12:34:56",
			vmName:        "test-vm",
			timeout:       1 * time.Second,
			expectPass:    false,
			expectError:   false, // No error, just timeout
		},
		{
			name:          "empty lease file",
			leaseFileData: "",
			vmMACAddress:  "52:54:00:12:34:56",
			vmName:        "test-vm",
			timeout:       1 * time.Second,
			expectPass:    false,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory and lease file
			tempDir := t.TempDir()
			tftpRoot := filepath.Join(tempDir, "tftp")
			require.NoError(t, os.MkdirAll(tftpRoot, 0o755))

			leaseFile := filepath.Join(tempDir, "dnsmasq.leases")
			if tt.leaseFileData != "" {
				require.NoError(t, os.WriteFile(leaseFile, []byte(tt.leaseFileData), 0o644))
			}

			// Create infrastructure state
			infra := &infrastructure.InfrastructureState{
				ID:         "test-id",
				TFTPRoot:   tftpRoot,
				DnsmasqID:  "test-dnsmasq",
				Kubeconfig: "/tmp/test-kubeconfig",
			}

			// Create VM instance
			vm := &VMInstance{
				Spec: VMSpec{
					Name:       tt.vmName,
					MACAddress: tt.vmMACAddress,
					UUID:       "test-uuid",
				},
				Metadata: &vmm.VMMetadata{
					Name: tt.vmName,
					IP:   "192.168.100.100",
				},
			}

			// Create assertion
			assertion := scenario.AssertionSpec{
				Type: "dhcp_lease",
				VM:   tt.vmName,
			}

			// Create validator with fast polling
			validator := NewDHCPLeaseValidator(100 * time.Millisecond)

			// Run validation with timeout
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			result, err := validator.Validate(ctx, assertion, vm, infra)

			// Verify results
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.NotNil(t, result)
			assert.Equal(t, tt.expectPass, result.Passed, "Result.Passed mismatch: %s", result.Message)
			assert.Equal(t, "dhcp_lease", result.Type)
			assert.NotEmpty(t, result.Message)
		})
	}
}

// TestDHCPLeaseValidator_LeaseFileNotExist tests behavior when lease file doesn't exist initially
func TestDHCPLeaseValidator_LeaseFileNotExist(t *testing.T) {
	tempDir := t.TempDir()
	tftpRoot := filepath.Join(tempDir, "tftp")
	require.NoError(t, os.MkdirAll(tftpRoot, 0o755))

	leaseFile := filepath.Join(tempDir, "dnsmasq.leases")

	infra := &infrastructure.InfrastructureState{
		ID:        "test-id",
		TFTPRoot:  tftpRoot,
		DnsmasqID: "test-dnsmasq",
	}

	vm := &VMInstance{
		Spec: VMSpec{
			Name:       "test-vm",
			MACAddress: "52:54:00:12:34:56",
			UUID:       "test-uuid",
		},
	}

	assertion := scenario.AssertionSpec{
		Type: "dhcp_lease",
		VM:   "test-vm",
	}

	validator := NewDHCPLeaseValidator(100 * time.Millisecond)

	// Start validation in background
	resultChan := make(chan *AssertionResult, 1)
	errChan := make(chan error, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go func() {
		result, err := validator.Validate(ctx, assertion, vm, infra)
		resultChan <- result
		errChan <- err
	}()

	// Wait a bit, then create lease file
	time.Sleep(500 * time.Millisecond)
	leaseData := "1699999999 52:54:00:12:34:56 192.168.100.100 test-vm *\n"
	require.NoError(t, os.WriteFile(leaseFile, []byte(leaseData), 0o644))

	// Wait for result
	result := <-resultChan
	err := <-errChan

	assert.NoError(t, err)
	assert.True(t, result.Passed, "Expected validation to pass after lease file was created")
}

// TestHTTPBootValidator_LogEntryMatches tests log entry matching logic
func TestHTTPBootValidator_LogEntryMatches(t *testing.T) {
	validator := NewHTTPBootValidator(1 * time.Second)

	tests := []struct {
		name        string
		logLine     string
		vmUUID      string
		expectMatch bool
	}{
		{
			name:        "JSON format - match",
			logLine:     `{"time":"2024-01-01T12:00:00Z","level":"INFO","msg":"ipxe_boot_request","uuid":"test-uuid-123","buildarch":"x86_64"}`,
			vmUUID:      "test-uuid-123",
			expectMatch: true,
		},
		{
			name:        "JSON format - no match",
			logLine:     `{"time":"2024-01-01T12:00:00Z","level":"INFO","msg":"ipxe_boot_request","uuid":"other-uuid","buildarch":"x86_64"}`,
			vmUUID:      "test-uuid-123",
			expectMatch: false,
		},
		{
			name:        "text format - match",
			logLine:     `time=2024-01-01T12:00:00Z level=INFO msg=ipxe_boot_request uuid=test-uuid-123 buildarch=x86_64`,
			vmUUID:      "test-uuid-123",
			expectMatch: true,
		},
		{
			name:        "text format - no match",
			logLine:     `time=2024-01-01T12:00:00Z level=INFO msg=ipxe_boot_request uuid=other-uuid buildarch=x86_64`,
			vmUUID:      "test-uuid-123",
			expectMatch: false,
		},
		{
			name:        "different message type",
			logLine:     `{"time":"2024-01-01T12:00:00Z","level":"INFO","msg":"other_message","uuid":"test-uuid-123"}`,
			vmUUID:      "test-uuid-123",
			expectMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := &VMInstance{
				Spec: VMSpec{
					UUID: tt.vmUUID,
				},
			}

			match := validator.logEntryMatchesVM(tt.logLine, vm)
			assert.Equal(t, tt.expectMatch, match)
		})
	}
}

// TestProfileMatchValidator_ExtractProfileName tests profile name extraction
func TestProfileMatchValidator_ExtractProfileName(t *testing.T) {
	validator := NewProfileMatchValidator(1 * time.Second)

	tests := []struct {
		name        string
		logLine     string
		expectName  string
		expectEmpty bool
	}{
		{
			name:        "JSON format",
			logLine:     `{"time":"2024-01-01T12:00:00Z","level":"INFO","msg":"profile_matched","profile_name":"default-profile","profile_namespace":"shaper-system","assignment":"default-assignment"}`,
			expectName:  "default-profile",
			expectEmpty: false,
		},
		{
			name:        "text format",
			logLine:     `time=2024-01-01T12:00:00Z level=INFO msg=profile_matched profile_name=custom-profile profile_namespace=shaper-system assignment=custom-assignment`,
			expectName:  "custom-profile",
			expectEmpty: false,
		},
		{
			name:        "text format with dashes in name",
			logLine:     `time=2024-01-01T12:00:00Z level=INFO msg=profile_matched profile_name=worker-profile-v2 profile_namespace=shaper-system`,
			expectName:  "worker-profile-v2",
			expectEmpty: false,
		},
		{
			name:        "missing profile_name field",
			logLine:     `{"time":"2024-01-01T12:00:00Z","level":"INFO","msg":"profile_matched","profile_namespace":"shaper-system"}`,
			expectName:  "",
			expectEmpty: true,
		},
		{
			name:        "wrong message type",
			logLine:     `{"time":"2024-01-01T12:00:00Z","level":"INFO","msg":"assignment_selected","assignment_name":"test"}`,
			expectName:  "",
			expectEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := validator.extractProfileName(tt.logLine)
			if tt.expectEmpty {
				assert.Empty(t, name)
			} else {
				assert.Equal(t, tt.expectName, name)
			}
		})
	}
}

// TestAssignmentMatchValidator_ExtractAssignmentName tests assignment name extraction
func TestAssignmentMatchValidator_ExtractAssignmentName(t *testing.T) {
	validator := NewAssignmentMatchValidator(1 * time.Second)

	tests := []struct {
		name        string
		logLine     string
		expectName  string
		expectEmpty bool
	}{
		{
			name:        "JSON format",
			logLine:     `{"time":"2024-01-01T12:00:00Z","level":"INFO","msg":"assignment_selected","assignment_name":"default-assignment","assignment_namespace":"shaper-system","matched_by":"default"}`,
			expectName:  "default-assignment",
			expectEmpty: false,
		},
		{
			name:        "text format",
			logLine:     `time=2024-01-01T12:00:00Z level=INFO msg=assignment_selected assignment_name=custom-assignment assignment_namespace=shaper-system matched_by=selectors`,
			expectName:  "custom-assignment",
			expectEmpty: false,
		},
		{
			name:        "text format with dashes in name",
			logLine:     `time=2024-01-01T12:00:00Z level=INFO msg=assignment_selected assignment_name=worker-node-assignment assignment_namespace=shaper-system`,
			expectName:  "worker-node-assignment",
			expectEmpty: false,
		},
		{
			name:        "missing assignment_name field",
			logLine:     `{"time":"2024-01-01T12:00:00Z","level":"INFO","msg":"assignment_selected","assignment_namespace":"shaper-system"}`,
			expectName:  "",
			expectEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := validator.extractAssignmentName(tt.logLine)
			if tt.expectEmpty {
				assert.Empty(t, name)
			} else {
				assert.Equal(t, tt.expectName, name)
			}
		})
	}
}

// TestAssertionResult_BasicFields tests AssertionResult basic functionality
func TestAssertionResult_BasicFields(t *testing.T) {
	result := &AssertionResult{
		Type:     "dhcp_lease",
		Expected: "DHCP lease obtained",
		Actual:   "DHCP lease obtained: 192.168.100.100",
		Passed:   true,
		Message:  "VM test-vm obtained DHCP lease",
		Duration: 2 * time.Second,
	}

	assert.Equal(t, "dhcp_lease", result.Type)
	assert.Equal(t, "DHCP lease obtained", result.Expected)
	assert.True(t, result.Passed)
	assert.NotEmpty(t, result.Message)
	assert.Greater(t, result.Duration, time.Duration(0))
}

// TestDHCPLeaseValidator_NewWithDefaultPollInterval tests constructor default
func TestDHCPLeaseValidator_NewWithDefaultPollInterval(t *testing.T) {
	validator := NewDHCPLeaseValidator(0)
	assert.NotNil(t, validator)
	assert.Equal(t, 2*time.Second, validator.pollInterval)
}

// TestHTTPBootValidator_NewWithDefaultPollInterval tests constructor default
func TestHTTPBootValidator_NewWithDefaultPollInterval(t *testing.T) {
	validator := NewHTTPBootValidator(0)
	assert.NotNil(t, validator)
	assert.Equal(t, 2*time.Second, validator.pollInterval)
}

// TestProfileMatchValidator_NewWithDefaultPollInterval tests constructor default
func TestProfileMatchValidator_NewWithDefaultPollInterval(t *testing.T) {
	validator := NewProfileMatchValidator(0)
	assert.NotNil(t, validator)
	assert.Equal(t, 2*time.Second, validator.pollInterval)
}

// TestAssignmentMatchValidator_NewWithDefaultPollInterval tests constructor default
func TestAssignmentMatchValidator_NewWithDefaultPollInterval(t *testing.T) {
	validator := NewAssignmentMatchValidator(0)
	assert.NotNil(t, validator)
	assert.Equal(t, 2*time.Second, validator.pollInterval)
}

// TestTFTPBootValidator_NewWithDefaultPollInterval tests constructor default
func TestTFTPBootValidator_NewWithDefaultPollInterval(t *testing.T) {
	validator := NewTFTPBootValidator(0)
	assert.NotNil(t, validator)
	assert.Equal(t, 2*time.Second, validator.pollInterval)
}

// TestProfileMatchValidator_IsIPXERequestForVM tests VM correlation logic
func TestProfileMatchValidator_IsIPXERequestForVM(t *testing.T) {
	validator := NewProfileMatchValidator(1 * time.Second)

	tests := []struct {
		name        string
		logLine     string
		vmUUID      string
		expectMatch bool
	}{
		{
			name:        "JSON format - match",
			logLine:     `{"msg":"ipxe_boot_request","uuid":"test-uuid"}`,
			vmUUID:      "test-uuid",
			expectMatch: true,
		},
		{
			name:        "text format - match",
			logLine:     `msg=ipxe_boot_request uuid=test-uuid buildarch=x86_64`,
			vmUUID:      "test-uuid",
			expectMatch: true,
		},
		{
			name:        "wrong message type",
			logLine:     `{"msg":"other_message","uuid":"test-uuid"}`,
			vmUUID:      "test-uuid",
			expectMatch: false,
		},
		{
			name:        "wrong UUID",
			logLine:     `{"msg":"ipxe_boot_request","uuid":"other-uuid"}`,
			vmUUID:      "test-uuid",
			expectMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := &VMInstance{
				Spec: VMSpec{
					UUID: tt.vmUUID,
				},
			}

			match := validator.isIPXERequestForVM(tt.logLine, vm)
			assert.Equal(t, tt.expectMatch, match)
		})
	}
}

// TestAssignmentMatchValidator_IsIPXERequestForVM tests VM correlation logic
func TestAssignmentMatchValidator_IsIPXERequestForVM(t *testing.T) {
	validator := NewAssignmentMatchValidator(1 * time.Second)

	tests := []struct {
		name        string
		logLine     string
		vmUUID      string
		expectMatch bool
	}{
		{
			name:        "JSON format - match",
			logLine:     `{"msg":"ipxe_boot_request","uuid":"vm-uuid-123"}`,
			vmUUID:      "vm-uuid-123",
			expectMatch: true,
		},
		{
			name:        "text format - match",
			logLine:     `msg=ipxe_boot_request uuid=vm-uuid-123 buildarch=x86_64`,
			vmUUID:      "vm-uuid-123",
			expectMatch: true,
		},
		{
			name:        "no match - different UUID",
			logLine:     `{"msg":"ipxe_boot_request","uuid":"different-uuid"}`,
			vmUUID:      "vm-uuid-123",
			expectMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := &VMInstance{
				Spec: VMSpec{
					UUID: tt.vmUUID,
				},
			}

			match := validator.isIPXERequestForVM(tt.logLine, vm)
			assert.Equal(t, tt.expectMatch, match)
		})
	}
}
