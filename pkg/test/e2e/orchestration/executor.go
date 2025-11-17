package orchestration

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e/infrastructure"
	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e/scenario"
)

// Timeout constants from schema
const (
	ResourceReadyTimeout = 60 * time.Second
	VMBootTimeout        = 180 * time.Second
	DHCPLeaseTimeout     = 30 * time.Second
	HTTPBootTimeout      = 120 * time.Second
)

var (
	// ErrResourceApplyFailed indicates resource application failed
	ErrResourceApplyFailed = errors.New("resource application failed")
	// ErrVMTestFailed indicates VM test execution failed
	ErrVMTestFailed = errors.New("VM test execution failed")
)

// VMTestResult represents test results for a single VM
type VMTestResult struct {
	VMName     string
	Status     string // passed, failed, error
	Assertions []AssertionResult
	Events     []TestEvent
	StartTime  time.Time
	EndTime    time.Time
}

// TestResult represents the complete test execution result
type TestResult struct {
	Scenario       *scenario.TestScenario
	Status         string // passed, failed, error
	StartTime      time.Time
	EndTime        time.Time
	Duration       time.Duration
	Infrastructure *infrastructure.InfrastructureState
	VMResults      []VMTestResult
	Errors         []error
}

// VMOrchestratorInterface defines the interface for VM orchestration operations
// This allows mocking for testing
type VMOrchestratorInterface interface {
	ProvisionMultiple(ctx context.Context, specs []VMSpec) ([]*VMInstance, error)
	GetEvents() []TestEvent
}

// ResourceApplierInterface defines the interface for K8s resource operations
// This allows mocking for testing
type ResourceApplierInterface interface {
	ApplyResources(ctx context.Context, resources []K8sResource) ([]*AppliedResource, error)
}

// TestExecutor orchestrates test execution
type TestExecutor struct {
	scenario     *scenario.TestScenario
	infra        *infrastructure.InfrastructureState
	vmOrch       VMOrchestratorInterface
	resApplier   ResourceApplierInterface
	logCollector *LogCollector                 // Added in Task 8c
	validators   map[string]AssertionValidator // Added in Task 8c
}

// NewTestExecutor creates a new test executor
func NewTestExecutor(
	scenario *scenario.TestScenario,
	infra *infrastructure.InfrastructureState,
	vmOrch VMOrchestratorInterface,
	resApplier ResourceApplierInterface,
	logCollector *LogCollector,
) *TestExecutor {
	e := &TestExecutor{
		scenario:     scenario,
		infra:        infra,
		vmOrch:       vmOrch,
		resApplier:   resApplier,
		logCollector: logCollector,
	}

	// Register validators (Task 8c)
	// Note: Using default poll intervals (2s) for all validators
	e.validators = map[string]AssertionValidator{
		"dhcp_lease":       NewDHCPLeaseValidator(0),
		"tftp_boot":        NewTFTPBootValidator(0),
		"http_boot_called": NewHTTPBootValidator(0),
		"profile_match":    NewProfileMatchValidator(0),
		"assignment_match": NewAssignmentMatchValidator(0),
		"config_retrieved": NewConfigRetrievedValidator(0),
	}

	return e
}

// Execute runs the complete test scenario
// Orchestrates: resource application -> VM provisioning -> test execution
func (e *TestExecutor) Execute(ctx context.Context) (*TestResult, error) {
	result := &TestResult{
		Scenario:       e.scenario,
		Status:         "running",
		StartTime:      time.Now(),
		Infrastructure: e.infra,
		VMResults:      make([]VMTestResult, 0),
		Errors:         make([]error, 0),
	}

	// Step 1: Apply Kubernetes resources
	if err := e.applyResources(ctx); err != nil {
		result.Status = "error"
		result.Errors = append(result.Errors, fmt.Errorf("%w: %v", ErrResourceApplyFailed, err))
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		return result, errors.Join(result.Errors...)
	}

	// Step 2: Provision VMs
	vms, err := e.provisionVMs(ctx)
	if err != nil {
		result.Status = "error"
		result.Errors = append(result.Errors, err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		return result, errors.Join(result.Errors...)
	}

	// Step 3: Execute VM tests
	vmResults, err := e.executeVMTests(ctx, vms)
	result.VMResults = vmResults
	if err != nil {
		result.Errors = append(result.Errors, err)
	}

	// Step 4: Determine overall test status
	result.Status = e.determineOverallStatus(vmResults)
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	if len(result.Errors) > 0 {
		return result, errors.Join(result.Errors...)
	}

	return result, nil
}

// applyResources applies Kubernetes resources from the scenario
func (e *TestExecutor) applyResources(ctx context.Context) error {
	if len(e.scenario.Resources) == 0 {
		// No resources to apply
		return nil
	}

	// Create context with timeout for resource operations
	resourceCtx, cancel := context.WithTimeout(ctx, ResourceReadyTimeout)
	defer cancel()

	// Convert scenario resources to K8sResource format
	resources := make([]K8sResource, len(e.scenario.Resources))
	for i, res := range e.scenario.Resources {
		resources[i] = K8sResource{
			Kind:      res.Kind,
			Name:      res.Name,
			Namespace: res.Namespace,
			YAML:      []byte(res.YAML),
		}
	}

	// Apply resources
	appliedResources, err := e.resApplier.ApplyResources(resourceCtx, resources)
	if err != nil {
		// Log which resources failed
		for _, applied := range appliedResources {
			if applied.Status == "failed" {
				return fmt.Errorf("resource %s/%s failed: %s", applied.Kind, applied.Name, applied.Error)
			}
		}
		return err
	}

	return nil
}

// provisionVMs provisions all VMs specified in the scenario
func (e *TestExecutor) provisionVMs(ctx context.Context) ([]*VMInstance, error) {
	// Create context with timeout for VM provisioning
	provisionCtx, cancel := context.WithTimeout(ctx, VMBootTimeout)
	defer cancel()

	// Convert scenario VM specs to orchestrator VMSpec format
	vmSpecs := make([]VMSpec, len(e.scenario.VMs))
	for i, vm := range e.scenario.VMs {
		vmSpec := VMSpec{
			Name:       vm.Name,
			UUID:       vm.UUID,
			MACAddress: vm.MACAddress,
			Memory:     vm.Memory,
			VCPUs:      vm.VCPUs,
			BootOrder:  vm.BootOrder,
			Labels:     vm.Labels,
		}

		// Convert disk spec if present
		if vm.Disk != nil {
			vmSpec.Disk = &DiskSpec{
				Image: vm.Disk.Image,
				Size:  vm.Disk.Size,
			}
		}

		vmSpecs[i] = vmSpec
	}

	// Provision VMs in parallel
	vms, err := e.vmOrch.ProvisionMultiple(provisionCtx, vmSpecs)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrVMProvisionFailed, err)
	}

	return vms, nil
}

// executeVMTests executes tests for all provisioned VMs
func (e *TestExecutor) executeVMTests(ctx context.Context, vms []*VMInstance) ([]VMTestResult, error) {
	results := make([]VMTestResult, 0, len(vms))
	var errs []error

	for _, vm := range vms {
		vmResult, err := e.executeVMTest(ctx, vm)
		if vmResult != nil {
			results = append(results, *vmResult)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("VM %s test failed: %w", vm.Spec.Name, err))
		}
	}

	if len(errs) > 0 {
		return results, errors.Join(append([]error{ErrVMTestFailed}, errs...)...)
	}

	return results, nil
}

// executeVMTest executes tests for a single VM
func (e *TestExecutor) executeVMTest(ctx context.Context, vm *VMInstance) (*VMTestResult, error) {
	result := &VMTestResult{
		VMName:     vm.Spec.Name,
		Status:     "running",
		Assertions: make([]AssertionResult, 0),
		Events:     e.vmOrch.GetEvents(), // Capture lifecycle events
		StartTime:  time.Now(),
	}

	// Wait for VM to boot - give it time to start the boot process
	time.Sleep(5 * time.Second)

	// Validate each assertion for this VM
	vmAssertions := e.getAssertionsForVM(vm.Spec.Name)
	for _, assertionSpec := range vmAssertions {
		assertionResult := e.validateAssertion(ctx, assertionSpec, vm)
		result.Assertions = append(result.Assertions, assertionResult)
	}

	// Set result status based on assertions
	result.EndTime = time.Now()
	result.Status = e.computeVMStatus(result.Assertions)

	return result, nil
}

// validateAssertion validates a single assertion using the appropriate validator
func (e *TestExecutor) validateAssertion(
	ctx context.Context,
	assertionSpec scenario.AssertionSpec,
	vm *VMInstance,
) AssertionResult {
	startTime := time.Now()

	// Get validator for this assertion type
	validator, ok := e.validators[assertionSpec.Type]
	if !ok {
		return AssertionResult{
			Type:     assertionSpec.Type,
			Expected: assertionSpec.Expected,
			Passed:   false,
			Message:  fmt.Sprintf("Unknown assertion type: %s", assertionSpec.Type),
			Duration: time.Since(startTime),
		}
	}

	// Call validator
	result, err := validator.Validate(ctx, assertionSpec, vm, e.infra)
	if err != nil {
		return AssertionResult{
			Type:     assertionSpec.Type,
			Expected: assertionSpec.Expected,
			Passed:   false,
			Message:  fmt.Sprintf("Validation error: %v", err),
			Duration: time.Since(startTime),
		}
	}

	result.Duration = time.Since(startTime)
	return *result
}

// getAssertionsForVM returns all assertions targeting a specific VM
func (e *TestExecutor) getAssertionsForVM(vmName string) []scenario.AssertionSpec {
	var assertions []scenario.AssertionSpec
	for _, assertion := range e.scenario.Assertions {
		if assertion.VM == vmName {
			assertions = append(assertions, assertion)
		}
	}
	return assertions
}

// computeVMStatus determines VM test status based on assertion results
// Returns "passed" if all assertions passed, "failed" otherwise
func (e *TestExecutor) computeVMStatus(assertions []AssertionResult) string {
	for _, assertion := range assertions {
		if !assertion.Passed {
			return "failed"
		}
	}
	return "passed"
}

// determineOverallStatus determines overall test status from VM results
// Error takes precedence over failed, failed takes precedence over passed
func (e *TestExecutor) determineOverallStatus(vmResults []VMTestResult) string {
	if len(vmResults) == 0 {
		return "error"
	}

	// First pass: check for any errors
	for _, vmResult := range vmResults {
		if vmResult.Status == "error" {
			return "error"
		}
	}

	// Second pass: check for any failures
	for _, vmResult := range vmResults {
		if vmResult.Status == "failed" {
			return "failed"
		}
	}

	// All VMs passed
	return "passed"
}
