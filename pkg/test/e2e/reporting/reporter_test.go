//go:build e2e

package reporting

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test fixtures

func createTestResultAllPass() *e2e.TestResult {
	startTime := time.Date(2025, 11, 16, 10, 30, 0, 0, time.UTC)
	endTime := startTime.Add(45*time.Second + 230*time.Millisecond)

	return &e2e.TestResult{
		Version: "1.0.0",
		TestID:  "basic-single-vm-boot-test-20251116-103000-abc123",
		Scenario: e2e.ScenarioInfo{
			Name:        "Basic Single VM Boot Test",
			Description: "Validates basic PXE boot flow with a single VM",
			File:        "/home/user/shaper/test/e2e/scenarios/basic-single-vm.yaml",
			Tags:        []string{"basic", "smoke", "boot"},
		},
		Execution: e2e.ExecutionInfo{
			StartTime:    startTime,
			EndTime:      endTime,
			Duration:     45.23,
			Architecture: "x86_64",
			Status:       "passed",
			ExitCode:     0,
		},
		Infra: e2e.Infrastructure{
			KindCluster: e2e.KindClusterInfo{
				Name:       "shaper-e2e",
				Version:    "v1.30.0",
				Kubeconfig: "/tmp/shaper-e2e-kubeconfig",
			},
			Network: e2e.NetworkInfo{
				Bridge:    "br-shaper-test",
				CIDR:      "192.168.100.0/24",
				DHCPRange: "192.168.100.100,192.168.100.200",
			},
			Shaper: e2e.ShaperInfo{
				Namespace:   "shaper-system",
				APIReplicas: 1,
				APIVersion:  "v0.1.0",
			},
		},
		Resources: []e2e.ResourceInfo{
			{
				Kind:      "Profile",
				Name:      "default-profile",
				Namespace: "shaper-system",
				Status:    "created",
				CreatedAt: startTime.Add(2 * time.Second),
			},
			{
				Kind:      "Assignment",
				Name:      "default-assignment",
				Namespace: "shaper-system",
				Status:    "created",
				CreatedAt: startTime.Add(3 * time.Second),
			},
		},
		VMs: []e2e.VMResult{
			{
				Name:       "test-vm-basic",
				UUID:       "550e8400-e29b-41d4-a716-446655440000",
				MACAddress: "52:54:00:12:34:56",
				IPAddress:  "192.168.100.100",
				Status:     "passed",
				Memory:     "1024",
				VCPUs:      1,
				Events: []e2e.VMEvent{
					{
						Timestamp: startTime.Add(5 * time.Second),
						Event:     "vm_created",
						Details:   map[string]interface{}{},
					},
					{
						Timestamp: startTime.Add(7 * time.Second),
						Event:     "dhcp_lease_obtained",
						Details: map[string]interface{}{
							"ip":        "192.168.100.100",
							"leaseTime": "3600",
						},
					},
					{
						Timestamp: startTime.Add(10 * time.Second),
						Event:     "tftp_boot",
						Details: map[string]interface{}{
							"file": "boot.ipxe",
						},
					},
					{
						Timestamp: startTime.Add(13 * time.Second),
						Event:     "http_boot_called",
						Details: map[string]interface{}{
							"endpoint":     "/ipxe?uuid=550e8400-e29b-41d4-a716-446655440000&buildarch=x86_64",
							"responseCode": 200,
						},
					},
				},
				Metrics: e2e.VMMetrics{
					ProvisionTime:     2.34,
					DHCPLeaseTime:     2.34,
					TFTPBootTime:      2.78,
					HTTPBootTime:      3.33,
					FirstResponseTime: 8.45,
				},
				Assertions: []e2e.AssertionInfo{
					{
						Type:        "dhcp_lease",
						Description: "VM should obtain DHCP lease from dnsmasq",
						Expected:    "DHCP lease obtained",
						Actual:      "192.168.100.100",
						Passed:      true,
						Duration:    2.34,
						Timestamp:   startTime.Add(7 * time.Second),
						Message:     "DHCP lease obtained successfully",
					},
					{
						Type:        "tftp_boot",
						Description: "VM should fetch boot file via TFTP",
						Expected:    "TFTP boot file fetched",
						Actual:      "boot.ipxe",
						Passed:      true,
						Duration:    2.78,
						Timestamp:   startTime.Add(10 * time.Second),
						Message:     "TFTP boot successful",
					},
					{
						Type:        "http_boot_called",
						Description: "VM should call shaper-API /boot.ipxe or /ipxe endpoint",
						Expected:    "HTTP 200 response",
						Actual:      "HTTP 200",
						Passed:      true,
						Duration:    3.33,
						Timestamp:   startTime.Add(13 * time.Second),
						Message:     "HTTP boot endpoint called successfully",
					},
					{
						Type:        "profile_match",
						Description: "shaper-API should return default-profile",
						Expected:    "default-profile",
						Actual:      "default-profile",
						Passed:      true,
						Duration:    0.12,
						Timestamp:   startTime.Add(13 * time.Second),
						Message:     "Profile matched successfully",
					},
				},
				Logs: e2e.VMLogPaths{
					Console: "/tmp/shaper-e2e/basic-test-123/vm-test-vm-basic-console.log",
					Serial:  "/tmp/shaper-e2e/basic-test-123/vm-test-vm-basic-serial.log",
				},
			},
		},
		Summary: e2e.AssertionStats{
			Total:    4,
			Passed:   4,
			Failed:   0,
			Skipped:  0,
			PassRate: 1.0,
		},
		Errors: []e2e.ErrorInfo{},
		Logs: e2e.LogPaths{
			Framework:   "/tmp/shaper-e2e/basic-test-123/framework.log",
			Dnsmasq:     "/tmp/shaper-e2e/basic-test-123/dnsmasq.log",
			ShaperAPI:   "/tmp/shaper-e2e/basic-test-123/shaper-api.log",
			Kubectl:     "/tmp/shaper-e2e/basic-test-123/kubectl.log",
			ArtifactDir: "/tmp/shaper-e2e/basic-test-123",
		},
		Metadata: e2e.TestMetadata{
			FrameworkVersion: "1.0.0",
			Hostname:         "test-runner-01",
			CIJobID:          "github-actions-789012",
			GitCommit:        "abc123def456",
			GitBranch:        "main",
		},
	}
}

func createTestResultWithFailures() *e2e.TestResult {
	result := createTestResultAllPass()
	result.TestID = "assignment-selector-test-20251116-113000-def456"
	result.Scenario.Name = "Assignment Selector Matching Test"
	result.Execution.Status = "failed"
	result.Execution.ExitCode = 1

	// Modify one VM to have failures
	vm := &result.VMs[0]
	vm.Name = "test-vm-uuid-match"
	vm.Status = "failed"

	// Mark last two assertions as failed
	vm.Assertions[2].Passed = false
	vm.Assertions[2].Type = "assignment_match"
	vm.Assertions[2].Expected = "uuid-specific-assignment"
	vm.Assertions[2].Actual = "default-fallback"
	vm.Assertions[2].Message = "Assignment mismatch: Expected uuid-specific-assignment but got default-fallback"

	vm.Assertions[3].Passed = false
	vm.Assertions[3].Expected = "custom-uuid-profile"
	vm.Assertions[3].Actual = "default-fallback-profile"
	vm.Assertions[3].Message = "Profile mismatch: Expected custom-uuid-profile but got default-fallback-profile"

	// Update summary
	result.Summary.Passed = 2
	result.Summary.Failed = 2
	result.Summary.PassRate = 0.5

	// Add errors
	result.Errors = []e2e.ErrorInfo{
		{
			Timestamp: result.Execution.StartTime.Add(18 * time.Second),
			Severity:  "error",
			Component: "assertion",
			Message:   "Assignment selector matching failed",
			Details:   "VM did not match uuid-specific-assignment",
		},
		{
			Timestamp: result.Execution.StartTime.Add(15 * time.Second),
			Severity:  "warning",
			Component: "vm",
			Message:   "TFTP boot was not detected",
			Details:   "No TFTP boot event was recorded",
		},
	}

	return result
}

func createTestLogCollection() *e2e.LogCollection {
	return &e2e.LogCollection{
		FrameworkLog: "Framework initialized\nTest started\nTest completed",
		DnsmasqLog:   "DHCP request received\nDHCP lease assigned",
		ShaperAPILog: "API started\nRequest received\nProfile returned",
		KubectlLog:   "Pod status: Running\nAll pods ready",
		VMConsoleLogs: map[string]string{
			"test-vm-basic": "VM booting\nPXE boot started\nBoot completed",
		},
		VMSerialLogs: map[string]string{
			"test-vm-basic": "Serial console output",
		},
	}
}

// Tests

func TestNewReporter(t *testing.T) {
	artifactDir := "/tmp/test-artifacts"
	reporter := NewReporter(artifactDir)

	assert.NotNil(t, reporter)
	assert.Equal(t, artifactDir, reporter.artifactDir)
}

func TestGenerateReport_JSON_AllPass(t *testing.T) {
	reporter := NewReporter("/tmp/test")
	result := createTestResultAllPass()
	logs := createTestLogCollection()

	jsonStr, err := reporter.GenerateReport(result, logs, FormatJSON)
	require.NoError(t, err)
	assert.NotEmpty(t, jsonStr)

	// Verify it's valid JSON
	var parsed map[string]interface{}
	err = json.Unmarshal([]byte(jsonStr), &parsed)
	require.NoError(t, err)

	// Verify key fields
	assert.Equal(t, "1.0.0", parsed["version"])
	assert.Equal(t, result.TestID, parsed["testID"])
	assert.Equal(t, "passed", parsed["execution"].(map[string]interface{})["status"])

	// Verify JSON is pretty-printed (has indentation)
	assert.Contains(t, jsonStr, "\n  ")
}

func TestGenerateReport_JSON_WithFailures(t *testing.T) {
	reporter := NewReporter("/tmp/test")
	result := createTestResultWithFailures()
	logs := createTestLogCollection()

	jsonStr, err := reporter.GenerateReport(result, logs, FormatJSON)
	require.NoError(t, err)
	assert.NotEmpty(t, jsonStr)

	// Verify it's valid JSON
	var parsed e2e.TestResult
	err = json.Unmarshal([]byte(jsonStr), &parsed)
	require.NoError(t, err)

	// Verify failure fields
	assert.Equal(t, "failed", parsed.Execution.Status)
	assert.Equal(t, 2, parsed.Summary.Failed)
	assert.Equal(t, 0.5, parsed.Summary.PassRate)
	assert.Len(t, parsed.Errors, 2)
}

func TestGenerateReport_Text_AllPass(t *testing.T) {
	reporter := NewReporter("/tmp/test")
	result := createTestResultAllPass()
	logs := createTestLogCollection()

	textStr, err := reporter.GenerateReport(result, logs, FormatText)
	require.NoError(t, err)
	assert.NotEmpty(t, textStr)

	// Verify key sections are present
	assert.Contains(t, textStr, "E2E TEST REPORT")
	assert.Contains(t, textStr, "SUMMARY")
	assert.Contains(t, textStr, "INFRASTRUCTURE")
	assert.Contains(t, textStr, "KUBERNETES RESOURCES")
	assert.Contains(t, textStr, "VM RESULTS")
	assert.Contains(t, textStr, "ASSERTION SUMMARY")
	assert.Contains(t, textStr, "LOGS")
	assert.Contains(t, textStr, "TEST RESULT")

	// Verify scenario name
	assert.Contains(t, textStr, "Basic Single VM Boot Test")

	// Verify status
	assert.Contains(t, textStr, "✓ PASSED")

	// Verify no FAILURES section (all passed)
	assert.NotContains(t, textStr, "FAILURES\n")

	// Verify VM details
	assert.Contains(t, textStr, "test-vm-basic")
	assert.Contains(t, textStr, "550e8400-e29b-41d4-a716-446655440000")
	assert.Contains(t, textStr, "192.168.100.100")

	// Verify assertions
	assert.Contains(t, textStr, "dhcp_lease")
	assert.Contains(t, textStr, "tftp_boot")
	assert.Contains(t, textStr, "http_boot_called")
	assert.Contains(t, textStr, "profile_match")

	// Verify metrics
	assert.Contains(t, textStr, "Performance Metrics")
	assert.Contains(t, textStr, "2.34s") // DHCP lease time
}

func TestGenerateReport_Text_WithFailures(t *testing.T) {
	reporter := NewReporter("/tmp/test")
	result := createTestResultWithFailures()
	logs := createTestLogCollection()

	textStr, err := reporter.GenerateReport(result, logs, FormatText)
	require.NoError(t, err)
	assert.NotEmpty(t, textStr)

	// Verify failure status
	assert.Contains(t, textStr, "✗ FAILED")

	// Verify FAILURES section exists
	assert.Contains(t, textStr, "FAILURES")

	// Verify failure details
	assert.Contains(t, textStr, "assignment_match")
	assert.Contains(t, textStr, "uuid-specific-assignment")
	assert.Contains(t, textStr, "default-fallback")

	// Verify failure guidance
	assert.Contains(t, textStr, "Possible Causes")
	assert.Contains(t, textStr, "Next Steps")

	// Verify ERRORS section
	assert.Contains(t, textStr, "ERRORS")
	assert.Contains(t, textStr, "Assignment selector matching failed")

	// Verify error severity formatting
	assert.Contains(t, textStr, "ERROR")
	assert.Contains(t, textStr, "WARNING")
}

func TestWriteReport_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	reporter := NewReporter(tmpDir)
	result := createTestResultAllPass()
	logs := createTestLogCollection()

	err := reporter.WriteReport(result, logs, FormatJSON)
	require.NoError(t, err)

	// Verify file exists
	reportPath := filepath.Join(tmpDir, result.TestID, "report.json")
	assert.FileExists(t, reportPath)

	// Verify file content is valid JSON
	data, err := os.ReadFile(reportPath)
	require.NoError(t, err)

	var parsed e2e.TestResult
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	// Verify content matches input
	assert.Equal(t, result.TestID, parsed.TestID)
	assert.Equal(t, result.Execution.Status, parsed.Execution.Status)
}

func TestWriteReport_Text(t *testing.T) {
	tmpDir := t.TempDir()
	reporter := NewReporter(tmpDir)
	result := createTestResultAllPass()
	logs := createTestLogCollection()

	err := reporter.WriteReport(result, logs, FormatText)
	require.NoError(t, err)

	// Verify file exists
	reportPath := filepath.Join(tmpDir, result.TestID, "report.txt")
	assert.FileExists(t, reportPath)

	// Verify file content
	data, err := os.ReadFile(reportPath)
	require.NoError(t, err)

	content := string(data)
	assert.Contains(t, content, "E2E TEST REPORT")
	assert.Contains(t, content, result.Scenario.Name)
}

func TestPrintSummary(t *testing.T) {
	// This test captures stdout, which is complex
	// For now, just verify it doesn't error
	reporter := NewReporter("/tmp/test")
	result := createTestResultAllPass()

	err := reporter.PrintSummary(result)
	assert.NoError(t, err)
}

func TestFormatSummary_AllPass(t *testing.T) {
	result := createTestResultAllPass()

	summary, err := formatSummary(result)
	require.NoError(t, err)
	assert.NotEmpty(t, summary)

	// Verify summary contains key information
	assert.Contains(t, summary, "TEST SUMMARY")
	assert.Contains(t, summary, result.Scenario.Name)
	assert.Contains(t, summary, "✓ PASSED")
	assert.Contains(t, summary, "45.23s")
	assert.Contains(t, summary, "1 total") // VMs
	assert.Contains(t, summary, "4 total") // Assertions
	assert.Contains(t, summary, "100.0% pass rate")

	// Should NOT contain failure summary
	assert.NotContains(t, summary, "Quick Failure Summary")
}

func TestFormatSummary_WithFailures(t *testing.T) {
	result := createTestResultWithFailures()

	summary, err := formatSummary(result)
	require.NoError(t, err)
	assert.NotEmpty(t, summary)

	// Verify failure information
	assert.Contains(t, summary, "✗ FAILED")
	assert.Contains(t, summary, "Quick Failure Summary")
	assert.Contains(t, summary, "assignment_match")
	assert.Contains(t, summary, "profile_match")
	assert.Contains(t, summary, "50.0% pass rate")
}

func TestGenerateReport_UnsupportedFormat(t *testing.T) {
	reporter := NewReporter("/tmp/test")
	result := createTestResultAllPass()
	logs := createTestLogCollection()

	_, err := reporter.GenerateReport(result, logs, "invalid")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
}

func TestWriteReport_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	reporter := NewReporter(tmpDir)
	result := createTestResultAllPass()
	logs := createTestLogCollection()

	err := reporter.WriteReport(result, logs, FormatJSON)
	require.NoError(t, err)

	// Verify directory was created
	reportDir := filepath.Join(tmpDir, result.TestID)
	info, err := os.Stat(reportDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestFormatEventDescription(t *testing.T) {
	tests := []struct {
		name     string
		event    e2e.VMEvent
		expected string
	}{
		{
			name: "vm_created",
			event: e2e.VMEvent{
				Event:   "vm_created",
				Details: map[string]interface{}{},
			},
			expected: "VM created",
		},
		{
			name: "dhcp_lease_obtained with IP",
			event: e2e.VMEvent{
				Event: "dhcp_lease_obtained",
				Details: map[string]interface{}{
					"ip": "192.168.100.100",
				},
			},
			expected: "DHCP lease obtained (192.168.100.100)",
		},
		{
			name: "tftp_boot with file",
			event: e2e.VMEvent{
				Event: "tftp_boot",
				Details: map[string]interface{}{
					"file": "boot.ipxe",
				},
			},
			expected: "TFTP boot file fetched (boot.ipxe)",
		},
		{
			name: "http_boot_called with endpoint",
			event: e2e.VMEvent{
				Event: "http_boot_called",
				Details: map[string]interface{}{
					"endpoint": "/ipxe?uuid=123",
				},
			},
			expected: "HTTP boot called (/ipxe?uuid=123)",
		},
		{
			name: "unknown event",
			event: e2e.VMEvent{
				Event:   "custom_event",
				Details: map[string]interface{}{},
			},
			expected: "custom_event",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatEventDescription(tt.event)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatStatus(t *testing.T) {
	tests := []struct {
		status   string
		contains string
	}{
		{"passed", "✓ PASSED"},
		{"failed", "✗ FAILED"},
		{"error", "⚠ ERROR"},
		{"skipped", "⚠ SKIPPED"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			result := formatStatus(tt.status)
			assert.Contains(t, result, tt.contains)
		})
	}
}

func TestWrapText(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		indent   int
		contains string
	}{
		{
			name:     "short text no wrap",
			text:     "Short text",
			indent:   4,
			contains: "Short text",
		},
		{
			name:     "long text wraps",
			text:     "This is a very long text that should wrap at word boundaries when it exceeds the maximum line length",
			indent:   4,
			contains: "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapText(tt.text, tt.indent)
			if tt.contains != "" {
				if tt.name == "long text wraps" {
					assert.Contains(t, result, tt.contains)
				} else {
					assert.Equal(t, tt.text, result)
				}
			}
		})
	}
}

func TestFormatFailureGuidance(t *testing.T) {
	tests := []struct {
		assertionType string
		contains      []string
	}{
		{
			assertionType: "dhcp_lease",
			contains: []string{
				"Possible Causes",
				"Dnsmasq",
				"Next Steps",
				"dnsmasq",
			},
		},
		{
			assertionType: "tftp_boot",
			contains: []string{
				"TFTP",
				"Boot file",
			},
		},
		{
			assertionType: "http_boot_called",
			contains: []string{
				"shaper-API",
				"running",
			},
		},
		{
			assertionType: "assignment_match",
			contains: []string{
				"Assignment",
				"selectors",
				"UUID",
			},
		},
		{
			assertionType: "profile_match",
			contains: []string{
				"Profile",
				"labels",
			},
		},
		{
			assertionType: "config_retrieved",
			contains: []string{
				"Config",
				"UUID",
			},
		},
		{
			assertionType: "unknown",
			contains: []string{
				"Check test logs",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.assertionType, func(t *testing.T) {
			result := formatFailureGuidance(tt.assertionType)
			for _, expected := range tt.contains {
				assert.Contains(t, result, expected)
			}
		})
	}
}

func TestJSONFormatMatchesSchema(t *testing.T) {
	// Verify the JSON output matches the schema from the spec
	reporter := NewReporter("/tmp/test")
	result := createTestResultAllPass()
	logs := createTestLogCollection()

	jsonStr, err := reporter.GenerateReport(result, logs, FormatJSON)
	require.NoError(t, err)

	var parsed map[string]interface{}
	err = json.Unmarshal([]byte(jsonStr), &parsed)
	require.NoError(t, err)

	// Verify top-level fields exist
	requiredFields := []string{
		"version", "testID", "scenario", "execution",
		"infrastructure", "vms", "assertions", "logs",
	}
	for _, field := range requiredFields {
		assert.Contains(t, parsed, field, "missing required field: %s", field)
	}

	// Verify scenario fields
	scenario := parsed["scenario"].(map[string]interface{})
	assert.Contains(t, scenario, "name")
	assert.Contains(t, scenario, "description")
	assert.Contains(t, scenario, "file")
	assert.Contains(t, scenario, "tags")

	// Verify execution fields
	execution := parsed["execution"].(map[string]interface{})
	assert.Contains(t, execution, "startTime")
	assert.Contains(t, execution, "endTime")
	assert.Contains(t, execution, "duration")
	assert.Contains(t, execution, "architecture")
	assert.Contains(t, execution, "status")
	assert.Contains(t, execution, "exitCode")

	// Verify infrastructure fields
	infra := parsed["infrastructure"].(map[string]interface{})
	assert.Contains(t, infra, "kindCluster")
	assert.Contains(t, infra, "network")
	assert.Contains(t, infra, "shaper")

	// Verify VMs array structure
	vms := parsed["vms"].([]interface{})
	assert.Len(t, vms, 1)
	vm := vms[0].(map[string]interface{})
	assert.Contains(t, vm, "name")
	assert.Contains(t, vm, "uuid")
	assert.Contains(t, vm, "macAddress")
	assert.Contains(t, vm, "status")
	assert.Contains(t, vm, "events")
	assert.Contains(t, vm, "metrics")
	assert.Contains(t, vm, "assertions")
	assert.Contains(t, vm, "logs")
}

func TestTextFormatContainsAllSections(t *testing.T) {
	reporter := NewReporter("/tmp/test")
	result := createTestResultWithFailures()
	logs := createTestLogCollection()

	textStr, err := reporter.GenerateReport(result, logs, FormatText)
	require.NoError(t, err)

	// Verify all required sections from spec
	sections := []string{
		"E2E TEST REPORT",
		"SUMMARY",
		"INFRASTRUCTURE",
		"Kind Cluster:",
		"Network:",
		"Shaper:",
		"KUBERNETES RESOURCES",
		"VM RESULTS",
		"Timeline:",
		"Performance Metrics:",
		"Assertions",
		"ASSERTION SUMMARY",
		"FAILURES",
		"ERRORS",
		"LOGS",
		"TEST RESULT:",
	}

	for _, section := range sections {
		assert.Contains(t, textStr, section, "missing section: %s", section)
	}
}

func TestReporterIntegration(t *testing.T) {
	// End-to-end integration test
	tmpDir := t.TempDir()
	reporter := NewReporter(tmpDir)
	result := createTestResultAllPass()
	logs := createTestLogCollection()

	// Write both JSON and text reports
	err := reporter.WriteReport(result, logs, FormatJSON)
	require.NoError(t, err)

	err = reporter.WriteReport(result, logs, FormatText)
	require.NoError(t, err)

	// Verify both files exist
	jsonPath := filepath.Join(tmpDir, result.TestID, "report.json")
	textPath := filepath.Join(tmpDir, result.TestID, "report.txt")
	assert.FileExists(t, jsonPath)
	assert.FileExists(t, textPath)

	// Verify JSON is valid
	jsonData, err := os.ReadFile(jsonPath)
	require.NoError(t, err)
	var parsedResult e2e.TestResult
	err = json.Unmarshal(jsonData, &parsedResult)
	require.NoError(t, err)
	assert.Equal(t, result.TestID, parsedResult.TestID)

	// Verify text contains key information
	textData, err := os.ReadFile(textPath)
	require.NoError(t, err)
	textContent := string(textData)
	assert.Contains(t, textContent, result.Scenario.Name)
	assert.Contains(t, textContent, "✓ PASSED")

	// Print summary (just verify no error)
	err = reporter.PrintSummary(result)
	assert.NoError(t, err)
}

func TestTextFormat_ColorCodes(t *testing.T) {
	// Verify color codes are used in text output
	reporter := NewReporter("/tmp/test")
	result := createTestResultWithFailures()
	logs := createTestLogCollection()

	textStr, err := reporter.GenerateReport(result, logs, FormatText)
	require.NoError(t, err)

	// Check for ANSI color codes
	assert.Contains(t, textStr, colorRed, "should contain red color for failures")
	assert.Contains(t, textStr, colorGreen, "should contain green color for passed items")
	assert.Contains(t, textStr, colorReset, "should contain color reset")
}

func TestJSONFormat_PrettyPrinting(t *testing.T) {
	reporter := NewReporter("/tmp/test")
	result := createTestResultAllPass()
	logs := createTestLogCollection()

	jsonStr, err := reporter.GenerateReport(result, logs, FormatJSON)
	require.NoError(t, err)

	// Verify pretty printing (indentation)
	lines := strings.Split(jsonStr, "\n")
	assert.Greater(t, len(lines), 10, "should have multiple lines")

	// Check for indentation
	hasIndentation := false
	for _, line := range lines {
		if strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "    ") {
			hasIndentation = true
			break
		}
	}
	assert.True(t, hasIndentation, "JSON should be indented")
}
