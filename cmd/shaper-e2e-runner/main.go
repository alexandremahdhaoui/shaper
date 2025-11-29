package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e/forge"
	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e/scenario"
)

const (
	defaultScenarioDir = "test/e2e/scenarios"
	defaultStoreDir    = "/tmp/shaper-e2e-testenv"
)

// Exit codes
const (
	exitSuccess = 0 // Operation successful
	exitError   = 1 // Command execution error (including test failures)
)

func main() {
	// Parse command
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(exitError)
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "run":
		exitCode := cmdRun(args)
		os.Exit(exitCode)

	case "list-scenarios":
		if err := cmdListScenarios(args); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(exitError)
		}
		os.Exit(exitSuccess)

	case "-h", "--help", "help":
		printUsage()
		os.Exit(exitSuccess)

	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command '%s'\n", command)
		printUsage()
		os.Exit(exitError)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: shaper-e2e-runner [command] [options]

Commands:
  run <test-id> <scenario-path> [--format json|text] [--verbose]
      Run a test scenario against a test environment

  list-scenarios [--dir <scenarios-dir>] [--format json|text]
      List all available test scenarios

  help
      Show this help message

Options:
  --format string
      Output format: json or text (default: text)

  --verbose
      Enable verbose output for run command

  --dir string
      Scenario directory (default: %s)

Environment Variables:
  SHAPER_E2E_TESTENV_STORE  Override test environment store directory (default: %s)
  SHAPER_E2E_SCENARIOS      Override scenario directory (default: %s)

Examples:
  # Run a scenario against test environment
  shaper-e2e-runner run e2e-shaper-abc123 test/e2e/scenarios/basic-boot.yaml

  # Run with JSON output for parsing
  shaper-e2e-runner run e2e-shaper-abc123 basic-boot.yaml --format json

  # List all scenarios
  shaper-e2e-runner list-scenarios

  # List scenarios in custom directory
  shaper-e2e-runner list-scenarios --dir /path/to/scenarios --format json

Exit Codes:
  0  Success (test passed)
  1  Error (invalid arguments, environment not found, test failures, etc.)
`, defaultScenarioDir, defaultStoreDir, defaultScenarioDir)
}

// cmdRun executes a test scenario
// Returns exit code: 0=success, 1=error
func cmdRun(args []string) int {
	// Parse flags
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	format := fs.String("format", "text", "Output format: json or text")
	verbose := fs.Bool("verbose", false, "Enable verbose output")
	_ = fs.Parse(args) // Error is handled by flag.ExitOnError

	// Get positional arguments
	posArgs := fs.Args()
	if len(posArgs) < 2 {
		fmt.Fprintf(os.Stderr, "Error: 'run' requires <test-id> and <scenario-path>\n")
		fmt.Fprintf(os.Stderr, "Usage: shaper-e2e-runner run <test-id> <scenario-path> [--format json|text]\n")
		return exitError
	}

	testID := posArgs[0]
	scenarioPath := posArgs[1]

	// Get directories from environment or defaults
	storeDir := getEnvOrDefault("SHAPER_E2E_TESTENV_STORE", defaultStoreDir)
	scenarioDir := getEnvOrDefault("SHAPER_E2E_SCENARIOS", defaultScenarioDir)

	// Validate format
	if *format != "json" && *format != "text" {
		fmt.Fprintf(os.Stderr, "Error: invalid format '%s', must be 'json' or 'text'\n", *format)
		return exitError
	}

	if *verbose {
		fmt.Fprintf(os.Stderr, "Running test scenario...\n")
		fmt.Fprintf(os.Stderr, "  Test ID: %s\n", testID)
		fmt.Fprintf(os.Stderr, "  Scenario: %s\n", scenarioPath)
		fmt.Fprintf(os.Stderr, "  Store Dir: %s\n", storeDir)
		fmt.Fprintf(os.Stderr, "  Scenario Dir: %s\n\n", scenarioDir)
	}

	// Create runner
	runner, err := forge.NewRunner(scenarioDir, storeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create runner: %v\n", err)
		return exitError
	}

	// Execute test
	ctx := context.Background()
	err = runner.Run(ctx, testID, scenarioPath)

	// Handle result based on format
	if *format == "json" {
		// For JSON output, print structured result
		result := map[string]interface{}{
			"testID":   testID,
			"scenario": scenarioPath,
		}

		if err != nil {
			result["status"] = "failed"
			result["error"] = err.Error()

			// Encode and print
			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")
			_ = encoder.Encode(result) // Ignore encoding error as we're about to exit

			return exitError
		}

		result["status"] = "passed"

		// Encode and print
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(result) // Ignore encoding error as we're about to exit

		return exitSuccess
	}

	// Text format - runner already printed report
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nTest failed: %v\n", err)
		return exitError
	}

	fmt.Fprintf(os.Stderr, "\n✅ Test passed\n")
	return exitSuccess
}

// cmdListScenarios lists all available test scenarios
func cmdListScenarios(args []string) error {
	// Parse flags
	fs := flag.NewFlagSet("list-scenarios", flag.ExitOnError)
	dir := fs.String("dir", "", "Scenario directory")
	format := fs.String("format", "text", "Output format: json or text")
	_ = fs.Parse(args) // Error is handled by flag.ExitOnError

	// Get scenario directory
	scenarioDir := *dir
	if scenarioDir == "" {
		scenarioDir = getEnvOrDefault("SHAPER_E2E_SCENARIOS", defaultScenarioDir)
	}

	// Validate format
	if *format != "json" && *format != "text" {
		return fmt.Errorf("invalid format '%s', must be 'json' or 'text'", *format)
	}

	// Read directory entries
	entries, err := os.ReadDir(scenarioDir)
	if err != nil {
		return fmt.Errorf("failed to read scenario directory %s: %w", scenarioDir, err)
	}

	// Load scenarios
	loader := scenario.NewLoader(scenarioDir)
	var scenarios []scenarioInfo

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only process .yaml files
		if filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}

		// Load scenario - pass just the filename, loader will resolve it
		testScenario, err := loader.Load(entry.Name())
		if err != nil {
			// Skip invalid scenarios
			fmt.Fprintf(os.Stderr, "Warning: failed to load %s: %v\n", entry.Name(), err)
			continue
		}

		scenarios = append(scenarios, scenarioInfo{
			File:        entry.Name(),
			Name:        testScenario.Name,
			Description: testScenario.Description,
			Tags:        testScenario.Tags,
		})
	}

	// Output based on format
	if *format == "json" {
		// JSON output
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(scenarios)
	}

	// Text output - table format
	if len(scenarios) == 0 {
		fmt.Println("No scenarios found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "FILE\tNAME\tTAGS\tDESCRIPTION")
	_, _ = fmt.Fprintln(w, "----\t----\t----\t-----------")

	for _, s := range scenarios {
		tags := ""
		if len(s.Tags) > 0 {
			tags = fmt.Sprintf("%v", s.Tags)
		}

		// Truncate description for table
		desc := s.Description
		if len(desc) > 60 {
			desc = desc[:57] + "..."
		}
		// Remove newlines from description
		desc = truncateAtNewline(desc)

		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.File, s.Name, tags, desc)
	}

	return w.Flush()
}

// scenarioInfo holds scenario metadata for listing
type scenarioInfo struct {
	File        string   `json:"file"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
}

// getEnvOrDefault returns environment variable value or default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// truncateAtNewline truncates string at first newline
func truncateAtNewline(s string) string {
	for i, c := range s {
		if c == '\n' {
			return s[:i]
		}
	}
	return s
}
