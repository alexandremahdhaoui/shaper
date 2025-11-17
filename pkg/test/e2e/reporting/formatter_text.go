package reporting

import (
	"fmt"
	"strings"
	"time"

	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e"
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorGray   = "\033[90m"
)

// formatText generates a human-readable text report
func formatText(result *e2e.TestResult, logs *e2e.LogCollection) (string, error) {
	var sb strings.Builder

	// Header
	sb.WriteString(strings.Repeat("=", 80) + "\n")
	sb.WriteString("E2E TEST REPORT\n")
	sb.WriteString(strings.Repeat("=", 80) + "\n\n")

	// Summary section
	sb.WriteString("SUMMARY\n")
	sb.WriteString(strings.Repeat("-", 8) + "\n")
	sb.WriteString(fmt.Sprintf("Scenario:     %s\n", result.Scenario.Name))
	sb.WriteString(fmt.Sprintf("Status:       %s\n", formatStatus(result.Execution.Status)))
	sb.WriteString(fmt.Sprintf("Duration:     %.2fs\n", result.Execution.Duration))
	sb.WriteString(fmt.Sprintf("Started:      %s\n", result.Execution.StartTime.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("Completed:    %s\n", result.Execution.EndTime.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("Architecture: %s\n", result.Execution.Architecture))
	sb.WriteString(fmt.Sprintf("Test ID:      %s\n\n", result.TestID))

	// Infrastructure section
	sb.WriteString("INFRASTRUCTURE\n")
	sb.WriteString(strings.Repeat("-", 14) + "\n")
	sb.WriteString("Kind Cluster:\n")
	sb.WriteString(fmt.Sprintf("  Name:       %s\n", result.Infra.KindCluster.Name))
	if result.Infra.KindCluster.Version != "" {
		sb.WriteString(fmt.Sprintf("  Version:    %s\n", result.Infra.KindCluster.Version))
	}
	sb.WriteString(fmt.Sprintf("  Kubeconfig: %s\n\n", result.Infra.KindCluster.Kubeconfig))

	sb.WriteString("Network:\n")
	sb.WriteString(fmt.Sprintf("  Bridge:     %s\n", result.Infra.Network.Bridge))
	sb.WriteString(fmt.Sprintf("  CIDR:       %s\n", result.Infra.Network.CIDR))
	sb.WriteString(fmt.Sprintf("  DHCP Range: %s\n\n", result.Infra.Network.DHCPRange))

	sb.WriteString("Shaper:\n")
	sb.WriteString(fmt.Sprintf("  Namespace:  %s\n", result.Infra.Shaper.Namespace))
	sb.WriteString(fmt.Sprintf("  API Version: %s\n", result.Infra.Shaper.APIVersion))
	sb.WriteString(fmt.Sprintf("  Replicas:   %d\n\n", result.Infra.Shaper.APIReplicas))

	// Kubernetes resources section
	if len(result.Resources) > 0 {
		sb.WriteString("KUBERNETES RESOURCES\n")
		sb.WriteString(strings.Repeat("-", 20) + "\n")
		for _, res := range result.Resources {
			statusSymbol := "✓"
			statusColor := colorGreen
			if res.Status == "failed" {
				statusSymbol = "✗"
				statusColor = colorRed
			}
			sb.WriteString(fmt.Sprintf("%s%s%s %s/%s (created at %s)\n",
				statusColor, statusSymbol, colorReset,
				res.Kind, res.Name,
				res.CreatedAt.Format(time.RFC3339)))
			if res.Error != "" {
				sb.WriteString(fmt.Sprintf("  Error: %s\n", res.Error))
			}
		}
		sb.WriteString("\n")
	}

	// VM results section
	sb.WriteString("VM RESULTS\n")
	sb.WriteString(strings.Repeat("-", 10) + "\n")
	for i, vm := range result.VMs {
		sb.WriteString(fmt.Sprintf("[%d/%d] %s\n", i+1, len(result.VMs), vm.Name))
		sb.WriteString(fmt.Sprintf("  UUID:       %s\n", vm.UUID))
		sb.WriteString(fmt.Sprintf("  MAC:        %s\n", vm.MACAddress))
		if vm.IPAddress != "" {
			sb.WriteString(fmt.Sprintf("  IP:         %s\n", vm.IPAddress))
		}
		sb.WriteString(fmt.Sprintf("  Status:     %s\n\n", formatStatus(vm.Status)))

		// Timeline
		if len(vm.Events) > 0 {
			sb.WriteString("  Timeline:\n")
			startTime := result.Execution.StartTime
			for _, event := range vm.Events {
				elapsed := event.Timestamp.Sub(startTime).Seconds()
				eventDesc := formatEventDescription(event)
				sb.WriteString(fmt.Sprintf("    %06.2fs  %s\n", elapsed, eventDesc))
			}
			sb.WriteString("\n")
		}

		// Performance metrics
		sb.WriteString("  Performance Metrics:\n")
		sb.WriteString(fmt.Sprintf("    Provision Time:       %.2fs\n", vm.Metrics.ProvisionTime))
		sb.WriteString(fmt.Sprintf("    DHCP Lease Time:      %.2fs\n", vm.Metrics.DHCPLeaseTime))
		sb.WriteString(fmt.Sprintf("    TFTP Boot Time:       %.2fs", vm.Metrics.TFTPBootTime))
		if vm.Metrics.TFTPBootTime == 0 {
			sb.WriteString(" (not detected)")
		}
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("    HTTP Boot Time:       %.2fs\n", vm.Metrics.HTTPBootTime))
		sb.WriteString(fmt.Sprintf("    First Response Time:  %.2fs\n\n", vm.Metrics.FirstResponseTime))

		// Assertions
		passedCount := 0
		for _, a := range vm.Assertions {
			if a.Passed {
				passedCount++
			}
		}
		sb.WriteString(fmt.Sprintf("  Assertions (%d/%d passed):\n", passedCount, len(vm.Assertions)))
		for _, assertion := range vm.Assertions {
			symbol := "✓"
			statusColor := colorGreen
			if !assertion.Passed {
				symbol = "✗"
				statusColor = colorRed
			}
			sb.WriteString(fmt.Sprintf("    %s%s%s %s: %s (%.2fs)\n",
				statusColor, symbol, colorReset,
				assertion.Type, assertion.Description, assertion.Duration))

			if assertion.Expected != "" || assertion.Actual != "" {
				sb.WriteString(fmt.Sprintf("      Expected: %s\n", assertion.Expected))
				sb.WriteString(fmt.Sprintf("      Actual:   %s\n", assertion.Actual))
			}

			if !assertion.Passed && assertion.Message != "" {
				sb.WriteString(fmt.Sprintf("      Message:  %s\n", wrapText(assertion.Message, 16)))
			}
		}
		sb.WriteString("\n")
	}

	// Assertion summary
	sb.WriteString("ASSERTION SUMMARY\n")
	sb.WriteString(strings.Repeat("-", 17) + "\n")
	sb.WriteString(fmt.Sprintf("Total:   %d\n", result.Summary.Total))
	sb.WriteString(fmt.Sprintf("Passed:  %d (%.1f%%)\n", result.Summary.Passed, result.Summary.PassRate*100))
	sb.WriteString(fmt.Sprintf("Failed:  %d (%.1f%%)\n", result.Summary.Failed, (1-result.Summary.PassRate)*100))
	sb.WriteString(fmt.Sprintf("Skipped: %d (%.1f%%)\n\n", result.Summary.Skipped, 0.0))

	// Failures section (if any)
	if result.Summary.Failed > 0 {
		sb.WriteString("FAILURES\n")
		sb.WriteString(strings.Repeat("-", 8) + "\n")
		failureNum := 1
		for _, vm := range result.VMs {
			for _, assertion := range vm.Assertions {
				if !assertion.Passed {
					sb.WriteString(fmt.Sprintf("[%d] VM: %s - Assertion: %s\n", failureNum, vm.Name, assertion.Type))
					sb.WriteString(fmt.Sprintf("    Type:     %s\n", assertion.Type))
					if assertion.Expected != "" {
						sb.WriteString(fmt.Sprintf("    Expected: %s\n", assertion.Expected))
					}
					if assertion.Actual != "" {
						sb.WriteString(fmt.Sprintf("    Actual:   %s\n", assertion.Actual))
					}
					if assertion.Message != "" {
						sb.WriteString(fmt.Sprintf("    Message:  %s\n", wrapText(assertion.Message, 14)))
					}
					sb.WriteString("\n")
					sb.WriteString(formatFailureGuidance(assertion.Type))
					sb.WriteString("\n")
					failureNum++
				}
			}
		}
	}

	// Errors section (if any)
	if len(result.Errors) > 0 {
		sb.WriteString("ERRORS\n")
		sb.WriteString(strings.Repeat("-", 6) + "\n")
		for i, err := range result.Errors {
			severityColor := colorRed
			if err.Severity == "warning" {
				severityColor = colorYellow
			} else if err.Severity == "info" {
				severityColor = colorBlue
			}

			sb.WriteString(fmt.Sprintf("[%d] %s [%s%s%s] %s\n",
				i+1,
				err.Timestamp.Format(time.RFC3339),
				severityColor, strings.ToUpper(err.Severity), colorReset,
				err.Component))
			sb.WriteString(fmt.Sprintf("    Message: %s\n", err.Message))
			if err.Details != "" {
				sb.WriteString(fmt.Sprintf("    Details: %s\n", wrapText(err.Details, 13)))
			}
			sb.WriteString("\n")
		}
	}

	// Logs section
	sb.WriteString("LOGS\n")
	sb.WriteString(strings.Repeat("-", 4) + "\n")
	sb.WriteString(fmt.Sprintf("Framework:  %s\n", result.Logs.Framework))
	sb.WriteString(fmt.Sprintf("Dnsmasq:    %s\n", result.Logs.Dnsmasq))
	sb.WriteString(fmt.Sprintf("Shaper API: %s\n", result.Logs.ShaperAPI))
	sb.WriteString(fmt.Sprintf("Kubectl:    %s\n", result.Logs.Kubectl))
	if len(result.VMs) > 0 {
		sb.WriteString("VM Logs:\n")
		for _, vm := range result.VMs {
			sb.WriteString(fmt.Sprintf("  %s (console): %s\n", vm.Name, vm.Logs.Console))
			sb.WriteString(fmt.Sprintf("  %s (serial):  %s\n", vm.Name, vm.Logs.Serial))
		}
	}
	sb.WriteString(fmt.Sprintf("\nArtifacts:  %s/\n\n", result.Logs.ArtifactDir))

	// Footer
	sb.WriteString(strings.Repeat("=", 80) + "\n")
	finalStatus := formatStatus(result.Execution.Status)
	sb.WriteString(fmt.Sprintf("TEST RESULT: %s\n", finalStatus))
	sb.WriteString(strings.Repeat("=", 80) + "\n")

	return sb.String(), nil
}

// formatStatus formats status with color
func formatStatus(status string) string {
	switch strings.ToLower(status) {
	case "passed":
		return fmt.Sprintf("%s✓ PASSED%s", colorGreen, colorReset)
	case "failed":
		return fmt.Sprintf("%s✗ FAILED%s", colorRed, colorReset)
	case "error":
		return fmt.Sprintf("%s⚠ ERROR%s", colorYellow, colorReset)
	case "skipped":
		return fmt.Sprintf("%s⚠ SKIPPED%s", colorYellow, colorReset)
	default:
		return status
	}
}

// formatEventDescription formats a VM event description
func formatEventDescription(event e2e.VMEvent) string {
	switch event.Event {
	case "vm_created":
		return "VM created"
	case "dhcp_lease_obtained":
		if ip, ok := event.Details["ip"].(string); ok {
			return fmt.Sprintf("DHCP lease obtained (%s)", ip)
		}
		return "DHCP lease obtained"
	case "tftp_boot":
		if file, ok := event.Details["file"].(string); ok {
			return fmt.Sprintf("TFTP boot file fetched (%s)", file)
		}
		return "TFTP boot file fetched"
	case "http_boot_called":
		if endpoint, ok := event.Details["endpoint"].(string); ok {
			return fmt.Sprintf("HTTP boot called (%s)", endpoint)
		}
		return "HTTP boot called"
	case "assertion_checked":
		return "Assertion checked"
	default:
		return event.Event
	}
}

// formatFailureGuidance provides troubleshooting guidance for failed assertions
func formatFailureGuidance(assertionType string) string {
	var guidance strings.Builder

	guidance.WriteString("    Possible Causes:\n")

	switch assertionType {
	case "dhcp_lease":
		guidance.WriteString("    - Dnsmasq is not running or misconfigured\n")
		guidance.WriteString("    - VM network interface is not connected to the bridge\n")
		guidance.WriteString("    - DHCP range is exhausted\n")
		guidance.WriteString("    - Firewall blocking DHCP traffic\n\n")
		guidance.WriteString("    Next Steps:\n")
		guidance.WriteString("    1. Check dnsmasq is running: ps aux | grep dnsmasq\n")
		guidance.WriteString("    2. Verify bridge exists: ip link show\n")
		guidance.WriteString("    3. Check dnsmasq logs for DHCP requests\n")
		guidance.WriteString("    4. Verify VM is attached to correct network\n")

	case "tftp_boot":
		guidance.WriteString("    - TFTP is not enabled in dnsmasq\n")
		guidance.WriteString("    - Boot file is missing from TFTP root\n")
		guidance.WriteString("    - TFTP port (69) is blocked\n\n")
		guidance.WriteString("    Next Steps:\n")
		guidance.WriteString("    1. Check TFTP root directory and files\n")
		guidance.WriteString("    2. Verify dnsmasq TFTP configuration\n")
		guidance.WriteString("    3. Check dnsmasq logs for TFTP requests\n")

	case "http_boot_called":
		guidance.WriteString("    - shaper-API is not running or not accessible\n")
		guidance.WriteString("    - iPXE boot script has incorrect URL\n")
		guidance.WriteString("    - Network routing issue\n\n")
		guidance.WriteString("    Next Steps:\n")
		guidance.WriteString("    1. Verify shaper-API pod is running\n")
		guidance.WriteString("    2. Check shaper-API service and endpoints\n")
		guidance.WriteString("    3. Review iPXE boot script for correct API URL\n")
		guidance.WriteString("    4. Check VM can reach shaper-API (network connectivity)\n")

	case "assignment_match":
		guidance.WriteString("    - Assignment subject selectors are not correctly configured\n")
		guidance.WriteString("    - VM UUID is not being passed to shaper-API correctly\n")
		guidance.WriteString("    - shaper-API is not evaluating UUID selectors properly\n")
		guidance.WriteString("    - Assignment controller has not reconciled the assignment\n\n")
		guidance.WriteString("    Next Steps:\n")
		guidance.WriteString("    1. Verify Assignment exists and has correct selectors\n")
		guidance.WriteString("    2. Check Assignment status and labels\n")
		guidance.WriteString("    3. Review shaper-API logs for assignment selection logic\n")
		guidance.WriteString("    4. Verify VM UUID is being sent in HTTP request\n")
		guidance.WriteString("    5. Check if Assignment controller has added selector labels\n")

	case "profile_match":
		guidance.WriteString("    - Profile does not exist or has wrong labels\n")
		guidance.WriteString("    - Assignment selector did not match VM\n")
		guidance.WriteString("    - Profile selector in Assignment is incorrect\n\n")
		guidance.WriteString("    Next Steps:\n")
		guidance.WriteString("    1. Verify Profile exists with correct name\n")
		guidance.WriteString("    2. Check Profile labels match Assignment selectors\n")
		guidance.WriteString("    3. Review Assignment profileSelectors\n")
		guidance.WriteString("    4. Check shaper-API logs for profile resolution\n")

	case "config_retrieved":
		guidance.WriteString("    - Config UUID is invalid or not found\n")
		guidance.WriteString("    - shaper-API cannot resolve config content\n")
		guidance.WriteString("    - Transformer failed to process config\n\n")
		guidance.WriteString("    Next Steps:\n")
		guidance.WriteString("    1. Verify config UUID exists in Profile status\n")
		guidance.WriteString("    2. Check shaper-API logs for config retrieval errors\n")
		guidance.WriteString("    3. Review Profile additionalContent configuration\n")

	default:
		guidance.WriteString("    - Check test logs for detailed error information\n\n")
		guidance.WriteString("    Next Steps:\n")
		guidance.WriteString("    1. Review all log files in artifact directory\n")
		guidance.WriteString("    2. Check Kubernetes events and pod status\n")
	}

	return guidance.String()
}

// wrapText wraps text at word boundaries with indentation
func wrapText(text string, indent int) string {
	if len(text) <= 64 {
		return text
	}

	var result strings.Builder
	words := strings.Fields(text)
	lineLen := 0
	indentStr := strings.Repeat(" ", indent)

	for i, word := range words {
		if i > 0 && lineLen+len(word)+1 > 64 {
			result.WriteString("\n" + indentStr)
			lineLen = 0
		} else if i > 0 {
			result.WriteString(" ")
			lineLen++
		}
		result.WriteString(word)
		lineLen += len(word)
	}

	return result.String()
}

// formatSummary formats a concise summary for stdout
func formatSummary(result *e2e.TestResult) (string, error) {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("=", 60) + "\n")
	sb.WriteString("TEST SUMMARY\n")
	sb.WriteString(strings.Repeat("=", 60) + "\n")
	sb.WriteString(fmt.Sprintf("Scenario: %s\n", result.Scenario.Name))
	sb.WriteString(fmt.Sprintf("Status:   %s\n", formatStatus(result.Execution.Status)))
	sb.WriteString(fmt.Sprintf("Duration: %.2fs\n", result.Execution.Duration))
	sb.WriteString(fmt.Sprintf("\n"))
	sb.WriteString(fmt.Sprintf("VMs:        %d total\n", len(result.VMs)))

	passedVMs := 0
	for _, vm := range result.VMs {
		if vm.Status == "passed" {
			passedVMs++
		}
	}
	sb.WriteString(fmt.Sprintf("  Passed:   %d\n", passedVMs))
	sb.WriteString(fmt.Sprintf("  Failed:   %d\n", len(result.VMs)-passedVMs))

	sb.WriteString(fmt.Sprintf("\n"))
	sb.WriteString(fmt.Sprintf("Assertions: %d total, %d passed, %d failed (%.1f%% pass rate)\n",
		result.Summary.Total,
		result.Summary.Passed,
		result.Summary.Failed,
		result.Summary.PassRate*100))

	if result.Summary.Failed > 0 {
		sb.WriteString(fmt.Sprintf("\n"))
		sb.WriteString(fmt.Sprintf("%sQuick Failure Summary:%s\n", colorRed, colorReset))
		failureNum := 1
		for _, vm := range result.VMs {
			for _, assertion := range vm.Assertions {
				if !assertion.Passed {
					sb.WriteString(fmt.Sprintf("  %d. VM %s: %s - %s\n",
						failureNum, vm.Name, assertion.Type, assertion.Description))
					failureNum++
				}
			}
		}
	}

	sb.WriteString(fmt.Sprintf("\n"))
	sb.WriteString(fmt.Sprintf("Full report: %s/%s/report.txt\n", result.Logs.ArtifactDir, result.TestID))
	sb.WriteString(strings.Repeat("=", 60) + "\n")

	return sb.String(), nil
}
