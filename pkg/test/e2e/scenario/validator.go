package scenario

import (
	"fmt"
	"net"
	"strings"
)

// ValidationError represents a validation error with detailed context.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("validation error in field '%s': %s", e.Field, e.Message)
	}
	return fmt.Sprintf("validation error: %s", e.Message)
}

// ValidationErrors represents multiple validation errors.
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return "no validation errors"
	}
	var msgs []string
	for _, err := range e {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "; ")
}

// Validate validates a TestScenario and returns detailed validation errors.
func Validate(scenario *TestScenario) error {
	var errs ValidationErrors

	// Validate required top-level fields
	if scenario.Name == "" {
		errs = append(errs, ValidationError{Field: "name", Message: "name is required"})
	}
	if scenario.Description == "" {
		errs = append(errs, ValidationError{Field: "description", Message: "description is required"})
	}
	if scenario.Architecture == "" {
		errs = append(errs, ValidationError{Field: "architecture", Message: "architecture is required"})
	} else {
		// Validate architecture value
		validArchs := map[string]bool{"x86_64": true, "aarch64": true}
		if !validArchs[scenario.Architecture] {
			errs = append(errs, ValidationError{
				Field:   "architecture",
				Message: fmt.Sprintf("invalid architecture '%s', must be one of: x86_64, aarch64", scenario.Architecture),
			})
		}
	}

	// Validate VMs (at least one required)
	if len(scenario.VMs) == 0 {
		errs = append(errs, ValidationError{Field: "vms", Message: "at least one VM is required"})
	} else {
		// Validate each VM
		vmNames := make(map[string]bool)
		for i, vm := range scenario.VMs {
			vmErrs := validateVM(vm, i)
			errs = append(errs, vmErrs...)

			// Check for duplicate VM names
			if vm.Name != "" {
				if vmNames[vm.Name] {
					errs = append(errs, ValidationError{
						Field:   fmt.Sprintf("vms[%d].name", i),
						Message: fmt.Sprintf("duplicate VM name '%s'", vm.Name),
					})
				}
				vmNames[vm.Name] = true
			}
		}
	}

	// Validate assertions (at least one required)
	if len(scenario.Assertions) == 0 {
		errs = append(errs, ValidationError{Field: "assertions", Message: "at least one assertion is required"})
	} else {
		// Validate each assertion
		for i, assertion := range scenario.Assertions {
			assertionErrs := validateAssertion(assertion, i, scenario.VMs)
			errs = append(errs, assertionErrs...)
		}
	}

	// Validate infrastructure if specified
	if scenario.Infrastructure.Network.CIDR != "" {
		if err := validateCIDR(scenario.Infrastructure.Network.CIDR); err != nil {
			errs = append(errs, ValidationError{
				Field:   "infrastructure.network.cidr",
				Message: fmt.Sprintf("invalid CIDR notation: %v", err),
			})
		}
	}

	// Validate resources if specified
	for i, resource := range scenario.Resources {
		resourceErrs := validateResource(resource, i)
		errs = append(errs, resourceErrs...)
	}

	// Validate timeouts if specified
	timeoutErrs := validateTimeouts(scenario.Timeouts)
	errs = append(errs, timeoutErrs...)

	if len(errs) > 0 {
		return errs
	}
	return nil
}

// validateVM validates a single VM specification.
func validateVM(vm VMSpec, index int) ValidationErrors {
	var errs ValidationErrors
	prefix := fmt.Sprintf("vms[%d]", index)

	// Name is required
	if vm.Name == "" {
		errs = append(errs, ValidationError{
			Field:   prefix + ".name",
			Message: "VM name is required",
		})
	}

	// Validate MAC address format if specified
	if vm.MACAddress != "" {
		if _, err := net.ParseMAC(vm.MACAddress); err != nil {
			errs = append(errs, ValidationError{
				Field:   prefix + ".macAddress",
				Message: fmt.Sprintf("invalid MAC address format: %v", err),
			})
		}
	}

	// Validate VCPUs if specified
	if vm.VCPUs < 0 {
		errs = append(errs, ValidationError{
			Field:   prefix + ".vcpus",
			Message: "vcpus must be >= 0",
		})
	}

	// Validate boot order if specified
	if len(vm.BootOrder) > 0 {
		validBootDevices := map[string]bool{"network": true, "hd": true, "cdrom": true}
		for _, device := range vm.BootOrder {
			if !validBootDevices[device] {
				errs = append(errs, ValidationError{
					Field:   prefix + ".bootOrder",
					Message: fmt.Sprintf("invalid boot device '%s', must be one of: network, hd, cdrom", device),
				})
			}
		}
	}

	// Validate disk if specified
	if vm.Disk != nil {
		if vm.Disk.Image == "" {
			errs = append(errs, ValidationError{
				Field:   prefix + ".disk.image",
				Message: "disk image path is required when disk is specified",
			})
		}
		if vm.Disk.Size == "" {
			errs = append(errs, ValidationError{
				Field:   prefix + ".disk.size",
				Message: "disk size is required when disk is specified",
			})
		}
	}

	return errs
}

// validateAssertion validates a single assertion specification.
func validateAssertion(assertion AssertionSpec, index int, vms []VMSpec) ValidationErrors {
	var errs ValidationErrors
	prefix := fmt.Sprintf("assertions[%d]", index)

	// Type is required
	if assertion.Type == "" {
		errs = append(errs, ValidationError{
			Field:   prefix + ".type",
			Message: "assertion type is required",
		})
	} else {
		// Validate assertion type
		validTypes := map[string]bool{
			"dhcp_lease":       true,
			"tftp_boot":        true,
			"http_boot_called": true,
			"profile_match":    true,
			"assignment_match": true,
			"config_retrieved": true,
		}
		if !validTypes[assertion.Type] {
			errs = append(errs, ValidationError{
				Field:   prefix + ".type",
				Message: fmt.Sprintf("invalid assertion type '%s', must be one of: dhcp_lease, tftp_boot, http_boot_called, profile_match, assignment_match, config_retrieved", assertion.Type),
			})
		}

		// Validate expected field for types that require it
		requiresExpected := map[string]bool{
			"profile_match":    true,
			"assignment_match": true,
		}
		if requiresExpected[assertion.Type] && assertion.Expected == "" {
			errs = append(errs, ValidationError{
				Field:   prefix + ".expected",
				Message: fmt.Sprintf("expected value is required for assertion type '%s'", assertion.Type),
			})
		}
	}

	// VM is required
	if assertion.VM == "" {
		errs = append(errs, ValidationError{
			Field:   prefix + ".vm",
			Message: "assertion vm field is required",
		})
	} else {
		// Verify VM exists in scenario
		vmExists := false
		for _, vm := range vms {
			if vm.Name == assertion.VM {
				vmExists = true
				break
			}
		}
		if !vmExists {
			errs = append(errs, ValidationError{
				Field:   prefix + ".vm",
				Message: fmt.Sprintf("VM '%s' not found in scenario VMs", assertion.VM),
			})
		}
	}

	return errs
}

// validateResource validates a single resource specification.
func validateResource(resource K8sResourceSpec, index int) ValidationErrors {
	var errs ValidationErrors
	prefix := fmt.Sprintf("resources[%d]", index)

	// Kind is required
	if resource.Kind == "" {
		errs = append(errs, ValidationError{
			Field:   prefix + ".kind",
			Message: "resource kind is required",
		})
	} else {
		// Validate kind value
		validKinds := map[string]bool{
			"Profile":    true,
			"Assignment": true,
			"ConfigMap":  true,
			"Secret":     true,
		}
		if !validKinds[resource.Kind] {
			errs = append(errs, ValidationError{
				Field:   prefix + ".kind",
				Message: fmt.Sprintf("invalid resource kind '%s', must be one of: Profile, Assignment, ConfigMap, Secret", resource.Kind),
			})
		}
	}

	// Name is required
	if resource.Name == "" {
		errs = append(errs, ValidationError{
			Field:   prefix + ".name",
			Message: "resource name is required",
		})
	}

	// YAML is required
	if resource.YAML == "" {
		errs = append(errs, ValidationError{
			Field:   prefix + ".yaml",
			Message: "resource yaml is required",
		})
	}

	return errs
}

// validateTimeouts validates timeout specifications.
func validateTimeouts(timeouts TimeoutSpec) ValidationErrors {
	var errs ValidationErrors

	// Validate each timeout if specified
	validateTimeout := func(field string, duration DurationString) {
		if duration != "" {
			if _, err := duration.Duration(); err != nil {
				errs = append(errs, ValidationError{
					Field:   "timeouts." + field,
					Message: fmt.Sprintf("invalid duration format: %v", err),
				})
			}
		}
	}

	validateTimeout("dhcpLease", timeouts.DHCPLease)
	validateTimeout("tftpBoot", timeouts.TFTPBoot)
	validateTimeout("httpBoot", timeouts.HTTPBoot)
	validateTimeout("vmProvision", timeouts.VMProvision)
	validateTimeout("resourceReady", timeouts.ResourceReady)
	validateTimeout("assertionPoll", timeouts.AssertionPoll)

	return errs
}

// validateCIDR validates CIDR notation.
func validateCIDR(cidr string) error {
	_, _, err := net.ParseCIDR(cidr)
	return err
}
