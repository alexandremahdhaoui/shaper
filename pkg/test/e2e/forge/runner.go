package forge

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e/orchestration"
	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e/scenario"
	"github.com/alexandremahdhaoui/shaper/pkg/vmm"
)

var (
	// ErrScenarioLoadFailed indicates scenario loading failed
	ErrScenarioLoadFailed = errors.New("scenario load failed")
	// ErrTestExecutionFailed indicates test execution failed
	ErrTestExecutionFailed = errors.New("test execution failed")
)

// Runner executes E2E test scenarios
type Runner struct {
	scenarioDir string
	storeDir    string
	store       EnvironmentStore
}

// NewRunner creates a new test runner
func NewRunner(scenarioDir, storeDir string) (*Runner, error) {
	store, err := NewJSONEnvironmentStore(storeDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create store: %w", err)
	}

	return &Runner{
		scenarioDir: scenarioDir,
		storeDir:    storeDir,
		store:       store,
	}, nil
}

// Run executes a test scenario against a test environment
// Parameters:
//   - testID: ID of the test environment (from testenv create)
//   - scenarioName: name of the scenario file (without .yaml extension)
//
// Returns error if test execution fails
func (r *Runner) Run(ctx context.Context, testID string, scenarioName string) error {
	// Load infrastructure state
	state, err := r.store.Load(testID)
	if err != nil {
		return fmt.Errorf("failed to load environment %s: %w", testID, err)
	}

	// Load test scenario
	scenarioPath := fmt.Sprintf("%s/%s.yaml", r.scenarioDir, scenarioName)
	scenario, err := r.loadScenario(scenarioPath)
	if err != nil {
		return errors.Join(err, ErrScenarioLoadFailed)
	}

	// Create components for test execution
	// Connect to libvirt for VM orchestration
	vmmConn, err := newVMMConnection()
	if err != nil {
		return fmt.Errorf("failed to connect to libvirt: %w", err)
	}
	defer func() { _ = vmmConn.Close() }()

	vmOrch := orchestration.NewVMOrchestrator(vmmConn, state.LibvirtNetwork)
	resApplier := orchestration.NewResourceApplier(state.Kubeconfig, "default")
	logCollector, err := orchestration.NewLogCollector(state.ArtifactDir)
	if err != nil {
		return fmt.Errorf("failed to create log collector: %w", err)
	}

	// Create executor
	executor := orchestration.NewTestExecutor(
		scenario,
		state,
		vmOrch,
		resApplier,
		logCollector,
	)

	// Execute test
	result, err := executor.Execute(ctx)
	if err != nil {
		// Print report even on failure
		r.printReport(result)
		return errors.Join(err, ErrTestExecutionFailed)
	}

	// Print report
	r.printReport(result)

	// Return error if test failed
	if result.Status != "passed" {
		return fmt.Errorf("test failed: status=%s", result.Status)
	}

	return nil
}

// loadScenario loads and validates a test scenario
func (r *Runner) loadScenario(scenarioPath string) (*scenario.TestScenario, error) {
	loader := scenario.NewLoader(r.scenarioDir)
	testScenario, err := loader.Load(scenarioPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load scenario: %w", err)
	}

	// Validate scenario
	if err := scenario.Validate(testScenario); err != nil {
		return nil, fmt.Errorf("scenario validation failed: %w", err)
	}

	return testScenario, nil
}

// newVMMConnection creates a new VMM connection
// This is a helper to abstract libvirt connection for testing
func newVMMConnection() (orchestration.VMMInterface, error) {
	// Import vmm package
	vmmConn, err := vmm.NewVMM()
	if err != nil {
		return nil, err
	}
	return vmmConn, nil
}

// printReport prints test results to stdout
func (r *Runner) printReport(result *orchestration.TestResult) {
	if result == nil {
		return
	}

	_, _ = fmt.Fprintf(os.Stdout, "\n=== Test Report ===\n")
	_, _ = fmt.Fprintf(os.Stdout, "Scenario: %s\n", result.Scenario.Name)
	_, _ = fmt.Fprintf(os.Stdout, "Status: %s\n", result.Status)
	_, _ = fmt.Fprintf(os.Stdout, "Duration: %s\n", result.Duration)
	_, _ = fmt.Fprintf(os.Stdout, "\n")

	// Print VM results
	for _, vmResult := range result.VMResults {
		_, _ = fmt.Fprintf(os.Stdout, "VM: %s (Status: %s)\n", vmResult.VMName, vmResult.Status)

		// Print assertions
		for _, assertion := range vmResult.Assertions {
			status := "PASS"
			if !assertion.Passed {
				status = "FAIL"
			}
			_, _ = fmt.Fprintf(os.Stdout, "  [%s] %s: %s (duration: %s)\n",
				status,
				assertion.Type,
				assertion.Message,
				assertion.Duration,
			)
		}
		_, _ = fmt.Fprintf(os.Stdout, "\n")
	}

	// Print errors if any
	if len(result.Errors) > 0 {
		_, _ = fmt.Fprintf(os.Stdout, "Errors:\n")
		for _, err := range result.Errors {
			_, _ = fmt.Fprintf(os.Stdout, "  - %v\n", err)
		}
	}
}
