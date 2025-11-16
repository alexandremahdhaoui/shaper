//go:build e2e

package reporting

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e"
)

// ReportFormat specifies the output format for reports
type ReportFormat string

const (
	// FormatJSON produces JSON-formatted reports
	FormatJSON ReportFormat = "json"
	// FormatText produces human-readable text reports
	FormatText ReportFormat = "text"
)

// Reporter generates test result reports in various formats
type Reporter struct {
	artifactDir string
}

// NewReporter creates a new reporter instance
func NewReporter(artifactDir string) *Reporter {
	return &Reporter{
		artifactDir: artifactDir,
	}
}

// GenerateReport generates a report in the specified format and returns it as a string
func (r *Reporter) GenerateReport(result *e2e.TestResult, logs *e2e.LogCollection, format ReportFormat) (string, error) {
	switch format {
	case FormatJSON:
		return formatJSON(result)
	case FormatText:
		return formatText(result, logs)
	default:
		return "", fmt.Errorf("unsupported format: %s", format)
	}
}

// WriteReport generates a report and writes it to disk
func (r *Reporter) WriteReport(result *e2e.TestResult, logs *e2e.LogCollection, format ReportFormat) error {
	// Generate report content
	content, err := r.GenerateReport(result, logs, format)
	if err != nil {
		return fmt.Errorf("failed to generate report: %w", err)
	}

	// Determine output path
	reportDir := filepath.Join(r.artifactDir, result.TestID)
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		return fmt.Errorf("failed to create report directory: %w", err)
	}

	var filename string
	switch format {
	case FormatJSON:
		filename = "report.json"
	case FormatText:
		filename = "report.txt"
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}

	reportPath := filepath.Join(reportDir, filename)
	if err := os.WriteFile(reportPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write report file: %w", err)
	}

	return nil
}

// PrintSummary prints a concise summary of test results to stdout
func (r *Reporter) PrintSummary(result *e2e.TestResult) error {
	summary, err := formatSummary(result)
	if err != nil {
		return fmt.Errorf("failed to format summary: %w", err)
	}

	fmt.Println(summary)
	return nil
}
