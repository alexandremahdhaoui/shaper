//go:build e2e

package orchestration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e/infrastructure"
	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e/scenario"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockResourceApplierInterface is a mock for ResourceApplierInterface
type MockResourceApplierInterface struct {
	mock.Mock
}

func (m *MockResourceApplierInterface) ApplyResources(ctx context.Context, resources []K8sResource) ([]*AppliedResource, error) {
	args := m.Called(ctx, resources)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*AppliedResource), args.Error(1)
}

// MockVMOrchestratorInterface is a mock for VMOrchestratorInterface
type MockVMOrchestratorInterface struct {
	mock.Mock
}

func (m *MockVMOrchestratorInterface) ProvisionMultiple(ctx context.Context, specs []VMSpec) ([]*VMInstance, error) {
	args := m.Called(ctx, specs)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*VMInstance), args.Error(1)
}

func (m *MockVMOrchestratorInterface) GetEvents() []TestEvent {
	args := m.Called()
	if args.Get(0) == nil {
		return []TestEvent{}
	}
	return args.Get(0).([]TestEvent)
}

func TestNewTestExecutor(t *testing.T) {
	tests := []struct {
		name     string
		scenario *scenario.TestScenario
		infra    *infrastructure.InfrastructureState
	}{
		{
			name: "creates executor with all components",
			scenario: &scenario.TestScenario{
				Name: "test-scenario",
			},
			infra: &infrastructure.InfrastructureState{
				ID: "test-infra-123",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockVMOrch := &MockVMOrchestratorInterface{}
			mockResApplier := &MockResourceApplierInterface{}
			logCollector, err := NewLogCollector(t.TempDir())
			assert.NoError(t, err)

			executor := NewTestExecutor(tt.scenario, tt.infra, mockVMOrch, mockResApplier, logCollector)

			assert.NotNil(t, executor)
			assert.Equal(t, tt.scenario, executor.scenario)
			assert.Equal(t, tt.infra, executor.infra)
			assert.Equal(t, mockVMOrch, executor.vmOrch)
			assert.Equal(t, mockResApplier, executor.resApplier)
			assert.NotNil(t, executor.logCollector)
			assert.NotNil(t, executor.validators)
			// Validators should now be populated with 6 validators
			assert.Len(t, executor.validators, 6)
			assert.Contains(t, executor.validators, "dhcp_lease")
			assert.Contains(t, executor.validators, "tftp_boot")
			assert.Contains(t, executor.validators, "http_boot_called")
			assert.Contains(t, executor.validators, "profile_match")
			assert.Contains(t, executor.validators, "assignment_match")
			assert.Contains(t, executor.validators, "config_retrieved")
		})
	}
}

func TestExecutor_Execute_Success(t *testing.T) {
	tests := []struct {
		name            string
		scenario        *scenario.TestScenario
		expectedStatus  string
		expectedVMCount int
	}{
		{
			name: "successful test execution with single VM",
			scenario: &scenario.TestScenario{
				Name: "basic-test",
				VMs: []scenario.VMSpec{
					{
						Name:   "test-vm-1",
						Memory: "2048",
						VCPUs:  2,
					},
				},
				Resources: []scenario.K8sResourceSpec{
					{
						Kind: "Profile",
						Name: "test-profile",
						YAML: "apiVersion: v1\nkind: Profile",
					},
				},
				Assertions: []scenario.AssertionSpec{}, // No assertions for unit test
			},
			expectedStatus:  "passed",
			expectedVMCount: 1,
		},
		{
			name: "successful test execution with multiple VMs",
			scenario: &scenario.TestScenario{
				Name: "multi-vm-test",
				VMs: []scenario.VMSpec{
					{Name: "vm-1", Memory: "2048", VCPUs: 2},
					{Name: "vm-2", Memory: "2048", VCPUs: 2},
				},
				Assertions: []scenario.AssertionSpec{}, // No assertions for unit test
			},
			expectedStatus:  "passed",
			expectedVMCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockVMOrch := &MockVMOrchestratorInterface{}
			mockResApplier := &MockResourceApplierInterface{}
			logCollector, err := NewLogCollector(t.TempDir())
			assert.NoError(t, err)

			// Mock resource application
			if len(tt.scenario.Resources) > 0 {
				appliedResources := make([]*AppliedResource, len(tt.scenario.Resources))
				for i, res := range tt.scenario.Resources {
					appliedResources[i] = &AppliedResource{
						Kind:   res.Kind,
						Name:   res.Name,
						Status: "created",
					}
				}
				mockResApplier.On("ApplyResources", mock.Anything, mock.Anything).Return(appliedResources, nil)
			}

			// Mock VM provisioning
			vmInstances := make([]*VMInstance, len(tt.scenario.VMs))
			for i, vmSpec := range tt.scenario.VMs {
				vmInstances[i] = &VMInstance{
					Spec: VMSpec{
						Name:   vmSpec.Name,
						Memory: vmSpec.Memory,
						VCPUs:  vmSpec.VCPUs,
					},
					State: VMStateRunning,
				}
			}
			mockVMOrch.On("ProvisionMultiple", mock.Anything, mock.Anything).Return(vmInstances, nil)
			mockVMOrch.On("GetEvents").Return([]TestEvent{})

			infra := &infrastructure.InfrastructureState{
				ID:       "test-infra",
				TFTPRoot: t.TempDir(), // Required for validators
			}
			executor := NewTestExecutor(tt.scenario, infra, mockVMOrch, mockResApplier, logCollector)

			// Execute test
			ctx := context.Background()
			result, err := executor.Execute(ctx)

			// Assertions
			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, tt.expectedStatus, result.Status)
			assert.Equal(t, tt.scenario, result.Scenario)
			assert.Equal(t, infra, result.Infrastructure)
			assert.Len(t, result.VMResults, tt.expectedVMCount)
			assert.NotZero(t, result.Duration)
			assert.Empty(t, result.Errors)

			// Verify all VMs have results
			for i, vmResult := range result.VMResults {
				assert.Equal(t, tt.scenario.VMs[i].Name, vmResult.VMName)
				assert.Equal(t, "passed", vmResult.Status)
				assert.NotZero(t, vmResult.StartTime)
				assert.NotZero(t, vmResult.EndTime)
			}

			// Verify mocks were called
			mockResApplier.AssertExpectations(t)
			mockVMOrch.AssertExpectations(t)
		})
	}
}

func TestExecutor_Execute_ResourceApplyFailure(t *testing.T) {
	tests := []struct {
		name          string
		scenario      *scenario.TestScenario
		resourceError error
	}{
		{
			name: "resource application fails",
			scenario: &scenario.TestScenario{
				Name: "test-scenario",
				Resources: []scenario.K8sResourceSpec{
					{
						Kind: "Profile",
						Name: "test-profile",
						YAML: "invalid yaml",
					},
				},
				VMs: []scenario.VMSpec{
					{Name: "test-vm"},
				},
			},
			resourceError: errors.New("kubectl apply failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockVMOrch := &MockVMOrchestratorInterface{}
			mockResApplier := &MockResourceApplierInterface{}
			logCollector, err := NewLogCollector(t.TempDir())
			assert.NoError(t, err)

			// Mock resource application failure
			mockResApplier.On("ApplyResources", mock.Anything, mock.Anything).Return(nil, tt.resourceError)

			infra := &infrastructure.InfrastructureState{ID: "test-infra"}
			executor := NewTestExecutor(tt.scenario, infra, mockVMOrch, mockResApplier, logCollector)

			// Execute test
			ctx := context.Background()
			result, err := executor.Execute(ctx)

			// Assertions
			assert.Error(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, "error", result.Status)
			assert.Len(t, result.Errors, 1)
			assert.ErrorIs(t, result.Errors[0], ErrResourceApplyFailed)
			assert.Empty(t, result.VMResults) // No VMs provisioned on resource failure

			// Verify resource applier was called but VM orchestrator was not
			mockResApplier.AssertExpectations(t)
			mockVMOrch.AssertNotCalled(t, "ProvisionMultiple")
		})
	}
}

func TestExecutor_Execute_VMProvisionFailure(t *testing.T) {
	tests := []struct {
		name      string
		scenario  *scenario.TestScenario
		vmError   error
		expectErr error
	}{
		{
			name: "VM provisioning fails",
			scenario: &scenario.TestScenario{
				Name: "test-scenario",
				VMs: []scenario.VMSpec{
					{Name: "test-vm"},
				},
			},
			vmError:   errors.New("libvirt connection failed"),
			expectErr: ErrVMProvisionFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockVMOrch := &MockVMOrchestratorInterface{}
			mockResApplier := &MockResourceApplierInterface{}
			logCollector, err := NewLogCollector(t.TempDir())
			assert.NoError(t, err)

			// Mock VM provisioning failure
			mockVMOrch.On("ProvisionMultiple", mock.Anything, mock.Anything).Return(nil, tt.vmError)

			infra := &infrastructure.InfrastructureState{ID: "test-infra"}
			executor := NewTestExecutor(tt.scenario, infra, mockVMOrch, mockResApplier, logCollector)

			// Execute test
			ctx := context.Background()
			result, err := executor.Execute(ctx)

			// Assertions
			assert.Error(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, "error", result.Status)
			assert.Len(t, result.Errors, 1)
			assert.ErrorIs(t, result.Errors[0], tt.expectErr)
			assert.Empty(t, result.VMResults)

			mockVMOrch.AssertExpectations(t)
		})
	}
}

func TestExecutor_Execute_TimeoutHandling(t *testing.T) {
	tests := []struct {
		name        string
		scenario    *scenario.TestScenario
		ctxTimeout  time.Duration
		expectError bool
	}{
		{
			name: "context timeout during resource apply",
			scenario: &scenario.TestScenario{
				Name: "timeout-test",
				Resources: []scenario.K8sResourceSpec{
					{Kind: "Profile", Name: "test"},
				},
				VMs: []scenario.VMSpec{
					{Name: "test-vm"},
				},
			},
			ctxTimeout:  1 * time.Nanosecond, // Immediate timeout
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockVMOrch := &MockVMOrchestratorInterface{}
			mockResApplier := &MockResourceApplierInterface{}
			logCollector, err := NewLogCollector(t.TempDir())
			assert.NoError(t, err)

			// Mock slow resource application that respects context timeout
			mockResApplier.On("ApplyResources", mock.Anything, mock.Anything).
				Run(func(args mock.Arguments) {
					ctx := args.Get(0).(context.Context)
					<-ctx.Done() // Wait for context cancellation
				}).
				Return(nil, context.DeadlineExceeded)

			infra := &infrastructure.InfrastructureState{ID: "test-infra"}
			executor := NewTestExecutor(tt.scenario, infra, mockVMOrch, mockResApplier, logCollector)

			// Execute with timeout context
			ctx, cancel := context.WithTimeout(context.Background(), tt.ctxTimeout)
			defer cancel()

			result, err := executor.Execute(ctx)

			if tt.expectError {
				assert.Error(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, "error", result.Status)
			}

			mockResApplier.AssertExpectations(t)
		})
	}
}

func TestExecutor_getAssertionsForVM(t *testing.T) {
	tests := []struct {
		name          string
		scenario      *scenario.TestScenario
		vmName        string
		expectedCount int
		expectedTypes []string
	}{
		{
			name: "finds multiple assertions for VM",
			scenario: &scenario.TestScenario{
				Assertions: []scenario.AssertionSpec{
					{Type: "dhcp_lease", VM: "vm-1"},
					{Type: "http_boot", VM: "vm-1"},
					{Type: "dhcp_lease", VM: "vm-2"},
				},
			},
			vmName:        "vm-1",
			expectedCount: 2,
			expectedTypes: []string{"dhcp_lease", "http_boot"},
		},
		{
			name: "finds no assertions for VM",
			scenario: &scenario.TestScenario{
				Assertions: []scenario.AssertionSpec{
					{Type: "dhcp_lease", VM: "vm-1"},
				},
			},
			vmName:        "vm-2",
			expectedCount: 0,
		},
		{
			name: "handles empty assertions",
			scenario: &scenario.TestScenario{
				Assertions: []scenario.AssertionSpec{},
			},
			vmName:        "vm-1",
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &TestExecutor{
				scenario: tt.scenario,
			}

			assertions := executor.getAssertionsForVM(tt.vmName)

			assert.Len(t, assertions, tt.expectedCount)
			if tt.expectedTypes != nil {
				for i, assertion := range assertions {
					assert.Equal(t, tt.expectedTypes[i], assertion.Type)
				}
			}
		})
	}
}

func TestExecutor_determineOverallStatus(t *testing.T) {
	tests := []struct {
		name           string
		vmResults      []VMTestResult
		expectedStatus string
	}{
		{
			name: "all VMs passed",
			vmResults: []VMTestResult{
				{VMName: "vm-1", Status: "passed"},
				{VMName: "vm-2", Status: "passed"},
			},
			expectedStatus: "passed",
		},
		{
			name: "one VM failed",
			vmResults: []VMTestResult{
				{VMName: "vm-1", Status: "passed"},
				{VMName: "vm-2", Status: "failed"},
			},
			expectedStatus: "failed",
		},
		{
			name: "one VM error",
			vmResults: []VMTestResult{
				{VMName: "vm-1", Status: "passed"},
				{VMName: "vm-2", Status: "error"},
			},
			expectedStatus: "error",
		},
		{
			name:           "no VMs",
			vmResults:      []VMTestResult{},
			expectedStatus: "error",
		},
		{
			name: "error takes precedence over failed",
			vmResults: []VMTestResult{
				{VMName: "vm-1", Status: "failed"},
				{VMName: "vm-2", Status: "error"},
			},
			expectedStatus: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &TestExecutor{}
			status := executor.determineOverallStatus(tt.vmResults)
			assert.Equal(t, tt.expectedStatus, status)
		})
	}
}

func TestExecutor_executeVMTest_CreatesAssertionResults(t *testing.T) {
	tests := []struct {
		name              string
		scenario          *scenario.TestScenario
		vm                *VMInstance
		expectedAssertion int
	}{
		{
			name: "creates assertion results for VM",
			scenario: &scenario.TestScenario{
				Assertions: []scenario.AssertionSpec{
					{
						Type:     "dhcp_lease",
						VM:       "test-vm",
						Expected: "192.168.1.100",
					},
					{
						Type:     "http_boot_called",
						VM:       "test-vm",
						Expected: "/boot.ipxe",
					},
				},
			},
			vm: &VMInstance{
				Spec: VMSpec{
					Name: "test-vm",
				},
				State: VMStateRunning,
			},
			expectedAssertion: 2,
		},
		{
			name: "handles VM with no assertions",
			scenario: &scenario.TestScenario{
				Assertions: []scenario.AssertionSpec{},
			},
			vm: &VMInstance{
				Spec: VMSpec{
					Name: "test-vm",
				},
				State: VMStateRunning,
			},
			expectedAssertion: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockVMOrch := &MockVMOrchestratorInterface{}
			mockResApplier := &MockResourceApplierInterface{}
			logCollector, err := NewLogCollector(t.TempDir())
			assert.NoError(t, err)
			mockVMOrch.On("GetEvents").Return([]TestEvent{})

			infra := &infrastructure.InfrastructureState{
				ID:       "test-infra",
				TFTPRoot: t.TempDir(), // Add TFTPRoot for validators
			}
			executor := NewTestExecutor(tt.scenario, infra, mockVMOrch, mockResApplier, logCollector)

			// Use short timeout to prevent validators from polling indefinitely
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			result, err := executor.executeVMTest(ctx, tt.vm)

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, tt.vm.Spec.Name, result.VMName)
			assert.Len(t, result.Assertions, tt.expectedAssertion)

			// Assertions will now be validated (and likely fail in unit test due to missing infra)
			// Just verify they have been processed
			for _, assertion := range result.Assertions {
				assert.NotEmpty(t, assertion.Type)
				assert.NotZero(t, assertion.Duration)
				// In unit test, assertions will fail due to missing infrastructure
				assert.False(t, assertion.Passed)
			}
		})
	}
}

func TestExecutor_applyResources_NoResources(t *testing.T) {
	// Test that applyResources handles scenario with no resources
	scenario := &scenario.TestScenario{
		Name:      "no-resources",
		Resources: []scenario.K8sResourceSpec{},
	}

	mockVMOrch := &MockVMOrchestratorInterface{}
	mockResApplier := &MockResourceApplierInterface{}
	logCollector, err := NewLogCollector(t.TempDir())
	assert.NoError(t, err)

	infra := &infrastructure.InfrastructureState{ID: "test-infra"}
	executor := NewTestExecutor(scenario, infra, mockVMOrch, mockResApplier, logCollector)

	ctx := context.Background()
	err = executor.applyResources(ctx)

	assert.NoError(t, err)
	// ResourceApplier should not be called when there are no resources
	mockResApplier.AssertNotCalled(t, "ApplyResources")
}

func TestExecutor_provisionVMs_ConvertsDiskSpec(t *testing.T) {
	scenario := &scenario.TestScenario{
		VMs: []scenario.VMSpec{
			{
				Name:   "vm-with-disk",
				Memory: "2048",
				VCPUs:  2,
				Disk: &scenario.DiskSpec{
					Image: "/path/to/image.qcow2",
					Size:  "20G",
				},
			},
		},
	}

	mockVMOrch := &MockVMOrchestratorInterface{}
	mockResApplier := &MockResourceApplierInterface{}
	logCollector, err := NewLogCollector(t.TempDir())
	assert.NoError(t, err)

	// Capture the VMSpec passed to ProvisionMultiple
	var capturedSpecs []VMSpec
	mockVMOrch.On("ProvisionMultiple", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		capturedSpecs = args.Get(1).([]VMSpec)
	}).Return([]*VMInstance{
		{
			Spec:  VMSpec{Name: "vm-with-disk"},
			State: VMStateRunning,
		},
	}, nil)

	infra := &infrastructure.InfrastructureState{ID: "test-infra"}
	executor := NewTestExecutor(scenario, infra, mockVMOrch, mockResApplier, logCollector)

	ctx := context.Background()
	_, err = executor.provisionVMs(ctx)

	assert.NoError(t, err)
	assert.Len(t, capturedSpecs, 1)
	assert.NotNil(t, capturedSpecs[0].Disk)
	assert.Equal(t, "/path/to/image.qcow2", capturedSpecs[0].Disk.Image)
	assert.Equal(t, "20G", capturedSpecs[0].Disk.Size)
}
