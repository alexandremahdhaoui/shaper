package scenario

import "time"

// TestScenario represents a complete E2E test scenario loaded from YAML.
type TestScenario struct {
	// Name is the human-readable test scenario name
	Name string `yaml:"name"`

	// Description provides detailed information about what this test validates
	Description string `yaml:"description"`

	// Tags are labels for categorizing and filtering scenarios
	Tags []string `yaml:"tags,omitempty"`

	// Architecture specifies the CPU architecture for VMs (x86_64, aarch64)
	Architecture string `yaml:"architecture"`

	// Infrastructure contains infrastructure configuration
	Infrastructure InfrastructureSpec `yaml:"infrastructure,omitempty"`

	// VMs is the list of test VMs to provision
	VMs []VMSpec `yaml:"vms"`

	// Resources contains Kubernetes resources to create before test execution
	Resources []K8sResourceSpec `yaml:"resources,omitempty"`

	// Assertions contains test assertions to validate
	Assertions []AssertionSpec `yaml:"assertions"`

	// Timeouts contains timeout configurations for various operations
	Timeouts TimeoutSpec `yaml:"timeouts,omitempty"`

	// ExpectedOutcome documents the expected test outcome (for documentation)
	ExpectedOutcome *ExpectedOutcome `yaml:"expectedOutcome,omitempty"`
}

// InfrastructureSpec defines infrastructure configuration for the test environment.
type InfrastructureSpec struct {
	Network NetworkSpec `yaml:"network,omitempty"`
	Kind    KindSpec    `yaml:"kind,omitempty"`
	Shaper  ShaperSpec  `yaml:"shaper,omitempty"`
}

// NetworkSpec defines network configuration.
type NetworkSpec struct {
	// CIDR is the network CIDR for the test environment
	CIDR string `yaml:"cidr,omitempty"`

	// Bridge is the bridge name
	Bridge string `yaml:"bridge,omitempty"`

	// DHCPRange is the DHCP range for dnsmasq (format: "start_ip,end_ip")
	DHCPRange string `yaml:"dhcpRange,omitempty"`
}

// KindSpec defines kind cluster configuration.
type KindSpec struct {
	// ClusterName is the kind cluster name
	ClusterName string `yaml:"clusterName,omitempty"`

	// Version is the Kubernetes version
	Version string `yaml:"version,omitempty"`
}

// ShaperSpec defines shaper component configuration.
type ShaperSpec struct {
	// Namespace is the Kubernetes namespace for shaper components
	Namespace string `yaml:"namespace,omitempty"`

	// APIReplicas is the number of shaper-api replicas
	APIReplicas int `yaml:"apiReplicas,omitempty"`
}

// VMSpec defines a test VM configuration.
type VMSpec struct {
	// Name is the VM name (must be unique within scenario)
	Name string `yaml:"name"`

	// UUID is the VM UUID for iPXE boot identification (auto-generated if not specified)
	UUID string `yaml:"uuid,omitempty"`

	// MACAddress is the MAC address for VM network interface (auto-generated if not specified)
	MACAddress string `yaml:"macAddress,omitempty"`

	// Memory is the memory allocation in MB
	Memory string `yaml:"memory,omitempty"`

	// VCPUs is the number of virtual CPUs
	VCPUs int `yaml:"vcpus,omitempty"`

	// BootOrder is the boot device order
	BootOrder []string `yaml:"bootOrder,omitempty"`

	// Disk contains disk configuration (optional)
	Disk *DiskSpec `yaml:"disk,omitempty"`

	// Labels are additional labels for iPXE boot parameters
	Labels map[string]string `yaml:"labels,omitempty"`
}

// DiskSpec defines disk configuration for a VM.
type DiskSpec struct {
	// Image is the path to QCOW2 image
	Image string `yaml:"image"`

	// Size is the disk size (format: "10G", "20G", etc.)
	Size string `yaml:"size"`
}

// K8sResourceSpec defines a Kubernetes resource to create.
type K8sResourceSpec struct {
	// Kind is the Kubernetes resource kind
	Kind string `yaml:"kind"`

	// Name is the resource name
	Name string `yaml:"name"`

	// Namespace is the resource namespace
	Namespace string `yaml:"namespace,omitempty"`

	// YAML is the inline YAML resource definition
	YAML string `yaml:"yaml"`
}

// AssertionSpec defines a test assertion.
type AssertionSpec struct {
	// Type is the assertion type (dhcp_lease, tftp_boot, http_boot_called, etc.)
	Type string `yaml:"type"`

	// VM is the target VM name
	VM string `yaml:"vm"`

	// Expected is the expected value to match (for profile_match, assignment_match)
	Expected string `yaml:"expected,omitempty"`

	// Description is a human-readable assertion description
	Description string `yaml:"description,omitempty"`
}

// TimeoutSpec defines timeout configurations.
type TimeoutSpec struct {
	// DHCPLease is the max wait for DHCP lease acquisition
	DHCPLease DurationString `yaml:"dhcpLease,omitempty"`

	// TFTPBoot is the max wait for TFTP boot file fetch
	TFTPBoot DurationString `yaml:"tftpBoot,omitempty"`

	// HTTPBoot is the max wait for HTTP boot endpoint call
	HTTPBoot DurationString `yaml:"httpBoot,omitempty"`

	// VMProvision is the max wait for VM creation/boot
	VMProvision DurationString `yaml:"vmProvision,omitempty"`

	// ResourceReady is the max wait for K8s resource reconciliation
	ResourceReady DurationString `yaml:"resourceReady,omitempty"`

	// AssertionPoll is the poll interval for assertion checking
	AssertionPoll DurationString `yaml:"assertionPoll,omitempty"`
}

// DurationString is a wrapper for time.Duration that supports YAML unmarshaling.
type DurationString string

// Duration parses the DurationString into a time.Duration.
func (d DurationString) Duration() (time.Duration, error) {
	if d == "" {
		return 0, nil
	}
	return time.ParseDuration(string(d))
}

// ExpectedOutcome documents the expected test outcome.
type ExpectedOutcome struct {
	// Status is the expected status (passed, failed, etc.)
	Status string `yaml:"status,omitempty"`

	// Description describes the expected outcome
	Description string `yaml:"description,omitempty"`
}
