package scenario

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidate_ValidScenario(t *testing.T) {
	scenario := &TestScenario{
		Name:         "Valid Scenario",
		Description:  "A valid test scenario",
		Architecture: "x86_64",
		VMs: []VMSpec{
			{Name: "test-vm"},
		},
		Assertions: []AssertionSpec{
			{Type: "dhcp_lease", VM: "test-vm"},
		},
	}

	err := Validate(scenario)
	assert.NoError(t, err)
}

func TestValidate_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name           string
		scenario       *TestScenario
		expectedErrors []string
	}{
		{
			name: "missing name",
			scenario: &TestScenario{
				Description:  "Test",
				Architecture: "x86_64",
				VMs:          []VMSpec{{Name: "vm"}},
				Assertions:   []AssertionSpec{{Type: "dhcp_lease", VM: "vm"}},
			},
			expectedErrors: []string{"name is required"},
		},
		{
			name: "missing description",
			scenario: &TestScenario{
				Name:         "Test",
				Architecture: "x86_64",
				VMs:          []VMSpec{{Name: "vm"}},
				Assertions:   []AssertionSpec{{Type: "dhcp_lease", VM: "vm"}},
			},
			expectedErrors: []string{"description is required"},
		},
		{
			name: "missing architecture",
			scenario: &TestScenario{
				Name:        "Test",
				Description: "Test",
				VMs:         []VMSpec{{Name: "vm"}},
				Assertions:  []AssertionSpec{{Type: "dhcp_lease", VM: "vm"}},
			},
			expectedErrors: []string{"architecture is required"},
		},
		{
			name: "no VMs",
			scenario: &TestScenario{
				Name:         "Test",
				Description:  "Test",
				Architecture: "x86_64",
				Assertions:   []AssertionSpec{{Type: "dhcp_lease", VM: "vm"}},
			},
			expectedErrors: []string{"at least one VM is required"},
		},
		{
			name: "no assertions",
			scenario: &TestScenario{
				Name:         "Test",
				Description:  "Test",
				Architecture: "x86_64",
				VMs:          []VMSpec{{Name: "vm"}},
			},
			expectedErrors: []string{"at least one assertion is required"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.scenario)
			assert.Error(t, err)
			for _, expectedErr := range tt.expectedErrors {
				assert.Contains(t, err.Error(), expectedErr)
			}
		})
	}
}

func TestValidate_InvalidArchitecture(t *testing.T) {
	scenario := &TestScenario{
		Name:         "Test",
		Description:  "Test",
		Architecture: "invalid_arch",
		VMs:          []VMSpec{{Name: "vm"}},
		Assertions:   []AssertionSpec{{Type: "dhcp_lease", VM: "vm"}},
	}

	err := Validate(scenario)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid architecture")
	assert.Contains(t, err.Error(), "must be one of: x86_64, aarch64")
}

func TestValidate_ValidArchitectures(t *testing.T) {
	architectures := []string{"x86_64", "aarch64"}

	for _, arch := range architectures {
		t.Run(arch, func(t *testing.T) {
			scenario := &TestScenario{
				Name:         "Test",
				Description:  "Test",
				Architecture: arch,
				VMs:          []VMSpec{{Name: "vm"}},
				Assertions:   []AssertionSpec{{Type: "dhcp_lease", VM: "vm"}},
			}

			err := Validate(scenario)
			assert.NoError(t, err)
		})
	}
}

func TestValidate_DuplicateVMNames(t *testing.T) {
	scenario := &TestScenario{
		Name:         "Test",
		Description:  "Test",
		Architecture: "x86_64",
		VMs: []VMSpec{
			{Name: "duplicate-vm"},
			{Name: "duplicate-vm"},
		},
		Assertions: []AssertionSpec{
			{Type: "dhcp_lease", VM: "duplicate-vm"},
		},
	}

	err := Validate(scenario)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate VM name")
}

func TestValidateVM(t *testing.T) {
	tests := []struct {
		name           string
		vm             VMSpec
		expectedErrors []string
	}{
		{
			name:           "valid VM",
			vm:             VMSpec{Name: "test-vm", Memory: "1024", VCPUs: 1},
			expectedErrors: nil,
		},
		{
			name:           "missing name",
			vm:             VMSpec{Memory: "1024"},
			expectedErrors: []string{"VM name is required"},
		},
		{
			name:           "invalid MAC address",
			vm:             VMSpec{Name: "vm", MACAddress: "invalid-mac"},
			expectedErrors: []string{"invalid MAC address format"},
		},
		{
			name:           "valid MAC address",
			vm:             VMSpec{Name: "vm", MACAddress: "52:54:00:12:34:56"},
			expectedErrors: nil,
		},
		{
			name:           "negative VCPUs",
			vm:             VMSpec{Name: "vm", VCPUs: -1},
			expectedErrors: []string{"vcpus must be >= 0"},
		},
		{
			name:           "invalid boot device",
			vm:             VMSpec{Name: "vm", BootOrder: []string{"network", "invalid"}},
			expectedErrors: []string{"invalid boot device"},
		},
		{
			name:           "valid boot devices",
			vm:             VMSpec{Name: "vm", BootOrder: []string{"network", "hd", "cdrom"}},
			expectedErrors: nil,
		},
		{
			name: "disk without image",
			vm: VMSpec{
				Name: "vm",
				Disk: &DiskSpec{Size: "10G"},
			},
			expectedErrors: []string{"disk image path is required"},
		},
		{
			name: "disk without size",
			vm: VMSpec{
				Name: "vm",
				Disk: &DiskSpec{Image: "/path/to/image.qcow2"},
			},
			expectedErrors: []string{"disk size is required"},
		},
		{
			name: "valid disk",
			vm: VMSpec{
				Name: "vm",
				Disk: &DiskSpec{Image: "/path/to/image.qcow2", Size: "10G"},
			},
			expectedErrors: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateVM(tt.vm, 0)
			if tt.expectedErrors == nil {
				assert.Empty(t, errs)
			} else {
				assert.NotEmpty(t, errs)
				errStr := ValidationErrors(errs).Error()
				for _, expectedErr := range tt.expectedErrors {
					assert.Contains(t, errStr, expectedErr)
				}
			}
		})
	}
}

func TestValidateAssertion(t *testing.T) {
	vms := []VMSpec{
		{Name: "vm1"},
		{Name: "vm2"},
	}

	tests := []struct {
		name           string
		assertion      AssertionSpec
		expectedErrors []string
	}{
		{
			name:           "valid dhcp_lease assertion",
			assertion:      AssertionSpec{Type: "dhcp_lease", VM: "vm1"},
			expectedErrors: nil,
		},
		{
			name:           "valid profile_match assertion",
			assertion:      AssertionSpec{Type: "profile_match", VM: "vm1", Expected: "test-profile"},
			expectedErrors: nil,
		},
		{
			name:           "missing type",
			assertion:      AssertionSpec{VM: "vm1"},
			expectedErrors: []string{"assertion type is required"},
		},
		{
			name:           "invalid type",
			assertion:      AssertionSpec{Type: "invalid_type", VM: "vm1"},
			expectedErrors: []string{"invalid assertion type"},
		},
		{
			name:           "missing VM",
			assertion:      AssertionSpec{Type: "dhcp_lease"},
			expectedErrors: []string{"assertion vm field is required"},
		},
		{
			name:           "VM not found",
			assertion:      AssertionSpec{Type: "dhcp_lease", VM: "nonexistent-vm"},
			expectedErrors: []string{"VM 'nonexistent-vm' not found"},
		},
		{
			name:           "profile_match without expected",
			assertion:      AssertionSpec{Type: "profile_match", VM: "vm1"},
			expectedErrors: []string{"expected value is required"},
		},
		{
			name:           "assignment_match without expected",
			assertion:      AssertionSpec{Type: "assignment_match", VM: "vm1"},
			expectedErrors: []string{"expected value is required"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateAssertion(tt.assertion, 0, vms)
			if tt.expectedErrors == nil {
				assert.Empty(t, errs)
			} else {
				assert.NotEmpty(t, errs)
				errStr := ValidationErrors(errs).Error()
				for _, expectedErr := range tt.expectedErrors {
					assert.Contains(t, errStr, expectedErr)
				}
			}
		})
	}
}

func TestValidateAssertion_AllValidTypes(t *testing.T) {
	vms := []VMSpec{{Name: "test-vm"}}
	validTypes := []string{
		"dhcp_lease",
		"tftp_boot",
		"http_boot_called",
		"profile_match",
		"assignment_match",
		"config_retrieved",
	}

	for _, assertionType := range validTypes {
		t.Run(assertionType, func(t *testing.T) {
			assertion := AssertionSpec{
				Type: assertionType,
				VM:   "test-vm",
			}

			// Add expected for types that require it
			if assertionType == "profile_match" || assertionType == "assignment_match" {
				assertion.Expected = "test-value"
			}

			errs := validateAssertion(assertion, 0, vms)
			assert.Empty(t, errs)
		})
	}
}

func TestValidateResource(t *testing.T) {
	tests := []struct {
		name           string
		resource       K8sResourceSpec
		expectedErrors []string
	}{
		{
			name: "valid Profile",
			resource: K8sResourceSpec{
				Kind: "Profile",
				Name: "test-profile",
				YAML: "apiVersion: v1\nkind: Profile",
			},
			expectedErrors: nil,
		},
		{
			name: "valid Assignment",
			resource: K8sResourceSpec{
				Kind: "Assignment",
				Name: "test-assignment",
				YAML: "apiVersion: v1\nkind: Assignment",
			},
			expectedErrors: nil,
		},
		{
			name:           "missing kind",
			resource:       K8sResourceSpec{Name: "test", YAML: "test"},
			expectedErrors: []string{"resource kind is required"},
		},
		{
			name:           "invalid kind",
			resource:       K8sResourceSpec{Kind: "InvalidKind", Name: "test", YAML: "test"},
			expectedErrors: []string{"invalid resource kind"},
		},
		{
			name:           "missing name",
			resource:       K8sResourceSpec{Kind: "Profile", YAML: "test"},
			expectedErrors: []string{"resource name is required"},
		},
		{
			name:           "missing yaml",
			resource:       K8sResourceSpec{Kind: "Profile", Name: "test"},
			expectedErrors: []string{"resource yaml is required"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateResource(tt.resource, 0)
			if tt.expectedErrors == nil {
				assert.Empty(t, errs)
			} else {
				assert.NotEmpty(t, errs)
				errStr := ValidationErrors(errs).Error()
				for _, expectedErr := range tt.expectedErrors {
					assert.Contains(t, errStr, expectedErr)
				}
			}
		})
	}
}

func TestValidateResource_AllValidKinds(t *testing.T) {
	validKinds := []string{"Profile", "Assignment", "ConfigMap", "Secret"}

	for _, kind := range validKinds {
		t.Run(kind, func(t *testing.T) {
			resource := K8sResourceSpec{
				Kind: kind,
				Name: "test-resource",
				YAML: "apiVersion: v1",
			}
			errs := validateResource(resource, 0)
			assert.Empty(t, errs)
		})
	}
}

func TestValidateTimeouts(t *testing.T) {
	tests := []struct {
		name           string
		timeouts       TimeoutSpec
		expectedErrors []string
	}{
		{
			name: "valid timeouts",
			timeouts: TimeoutSpec{
				DHCPLease:     "30s",
				TFTPBoot:      "60s",
				HTTPBoot:      "120s",
				VMProvision:   "180s",
				ResourceReady: "60s",
				AssertionPoll: "2s",
			},
			expectedErrors: nil,
		},
		{
			name: "invalid dhcpLease",
			timeouts: TimeoutSpec{
				DHCPLease: "invalid",
			},
			expectedErrors: []string{"timeouts.dhcpLease", "invalid duration format"},
		},
		{
			name: "invalid tftpBoot",
			timeouts: TimeoutSpec{
				TFTPBoot: "not-a-duration",
			},
			expectedErrors: []string{"timeouts.tftpBoot", "invalid duration format"},
		},
		{
			name:           "empty timeouts",
			timeouts:       TimeoutSpec{},
			expectedErrors: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateTimeouts(tt.timeouts)
			if tt.expectedErrors == nil {
				assert.Empty(t, errs)
			} else {
				assert.NotEmpty(t, errs)
				errStr := ValidationErrors(errs).Error()
				for _, expectedErr := range tt.expectedErrors {
					assert.Contains(t, errStr, expectedErr)
				}
			}
		})
	}
}

func TestValidateCIDR(t *testing.T) {
	tests := []struct {
		name        string
		cidr        string
		expectError bool
	}{
		{
			name:        "valid CIDR",
			cidr:        "192.168.100.0/24",
			expectError: false,
		},
		{
			name:        "valid CIDR /16",
			cidr:        "10.0.0.0/16",
			expectError: false,
		},
		{
			name:        "invalid CIDR - no mask",
			cidr:        "192.168.100.0",
			expectError: true,
		},
		{
			name:        "invalid CIDR - bad IP",
			cidr:        "999.999.999.999/24",
			expectError: true,
		},
		{
			name:        "invalid CIDR - bad mask",
			cidr:        "192.168.100.0/99",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCIDR(tt.cidr)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidate_InvalidCIDR(t *testing.T) {
	scenario := &TestScenario{
		Name:         "Test",
		Description:  "Test",
		Architecture: "x86_64",
		Infrastructure: InfrastructureSpec{
			Network: NetworkSpec{
				CIDR: "invalid-cidr",
			},
		},
		VMs:        []VMSpec{{Name: "vm"}},
		Assertions: []AssertionSpec{{Type: "dhcp_lease", VM: "vm"}},
	}

	err := Validate(scenario)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid CIDR notation")
}

func TestValidationError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      ValidationError
		expected string
	}{
		{
			name:     "with field",
			err:      ValidationError{Field: "name", Message: "is required"},
			expected: "validation error in field 'name': is required",
		},
		{
			name:     "without field",
			err:      ValidationError{Message: "general error"},
			expected: "validation error: general error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestValidationErrors_Error(t *testing.T) {
	tests := []struct {
		name     string
		errs     ValidationErrors
		expected string
	}{
		{
			name:     "empty errors",
			errs:     ValidationErrors{},
			expected: "no validation errors",
		},
		{
			name: "single error",
			errs: ValidationErrors{
				{Field: "name", Message: "is required"},
			},
			expected: "validation error in field 'name': is required",
		},
		{
			name: "multiple errors",
			errs: ValidationErrors{
				{Field: "name", Message: "is required"},
				{Field: "description", Message: "is required"},
			},
			expected: "validation error in field 'name': is required; validation error in field 'description': is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.errs.Error())
		})
	}
}
