//go:build e2e

package e2e

import "time"

// TestResult contains comprehensive test execution results
type TestResult struct {
	Version   string         `json:"version"`
	TestID    string         `json:"testID"`
	Scenario  ScenarioInfo   `json:"scenario"`
	Execution ExecutionInfo  `json:"execution"`
	Infra     Infrastructure `json:"infrastructure"`
	Resources []ResourceInfo `json:"resources,omitempty"`
	VMs       []VMResult     `json:"vms"`
	Summary   AssertionStats `json:"assertions"`
	Errors    []ErrorInfo    `json:"errors,omitempty"`
	Logs      LogPaths       `json:"logs"`
	Metadata  TestMetadata   `json:"metadata,omitempty"`
}

// ScenarioInfo contains scenario metadata
type ScenarioInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	File        string   `json:"file,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// ExecutionInfo contains test execution metadata
type ExecutionInfo struct {
	StartTime    time.Time `json:"startTime"`
	EndTime      time.Time `json:"endTime"`
	Duration     float64   `json:"duration"` // seconds
	Architecture string    `json:"architecture"`
	Status       string    `json:"status"` // passed, failed, error, skipped
	ExitCode     int       `json:"exitCode"`
}

// Infrastructure contains infrastructure configuration
type Infrastructure struct {
	KindCluster KindClusterInfo `json:"kindCluster"`
	Network     NetworkInfo     `json:"network"`
	Shaper      ShaperInfo      `json:"shaper"`
}

// KindClusterInfo contains Kind cluster details
type KindClusterInfo struct {
	Name       string `json:"name"`
	Version    string `json:"version,omitempty"`
	Kubeconfig string `json:"kubeconfig"`
}

// NetworkInfo contains network configuration
type NetworkInfo struct {
	Bridge    string `json:"bridge"`
	CIDR      string `json:"cidr"`
	DHCPRange string `json:"dhcpRange"`
}

// ShaperInfo contains Shaper deployment details
type ShaperInfo struct {
	Namespace   string `json:"namespace"`
	APIReplicas int    `json:"apiReplicas"`
	APIVersion  string `json:"apiVersion"`
}

// ResourceInfo contains Kubernetes resource creation info
type ResourceInfo struct {
	Kind      string    `json:"kind"`
	Name      string    `json:"name"`
	Namespace string    `json:"namespace"`
	Status    string    `json:"status"` // created, failed, skipped
	CreatedAt time.Time `json:"createdAt"`
	Error     string    `json:"error,omitempty"`
}

// VMResult contains per-VM test results
type VMResult struct {
	Name       string          `json:"name"`
	UUID       string          `json:"uuid"`
	MACAddress string          `json:"macAddress"`
	IPAddress  string          `json:"ipAddress,omitempty"`
	Status     string          `json:"status"` // passed, failed, error
	Memory     string          `json:"memory"`
	VCPUs      int             `json:"vcpus"`
	Events     []VMEvent       `json:"events"`
	Metrics    VMMetrics       `json:"metrics"`
	Assertions []AssertionInfo `json:"assertions"`
	Logs       VMLogPaths      `json:"logs"`
}

// VMEvent represents a VM lifecycle event
type VMEvent struct {
	Timestamp time.Time              `json:"timestamp"`
	Event     string                 `json:"event"` // vm_created, dhcp_lease_obtained, tftp_boot, http_boot_called, assertion_checked
	Details   map[string]interface{} `json:"details,omitempty"`
}

// VMMetrics contains performance metrics
type VMMetrics struct {
	ProvisionTime     float64 `json:"provisionTime"`     // seconds
	DHCPLeaseTime     float64 `json:"dhcpLeaseTime"`     // seconds
	TFTPBootTime      float64 `json:"tftpBootTime"`      // seconds
	HTTPBootTime      float64 `json:"httpBootTime"`      // seconds
	FirstResponseTime float64 `json:"firstResponseTime"` // seconds
}

// AssertionInfo contains assertion details
type AssertionInfo struct {
	Type        string    `json:"type"` // dhcp_lease, tftp_boot, http_boot_called, profile_match, assignment_match, config_retrieved
	Description string    `json:"description"`
	Expected    string    `json:"expected,omitempty"`
	Actual      string    `json:"actual,omitempty"`
	Passed      bool      `json:"passed"`
	Duration    float64   `json:"duration"` // seconds
	Timestamp   time.Time `json:"timestamp"`
	Message     string    `json:"message,omitempty"`
}

// AssertionStats contains aggregated assertion statistics
type AssertionStats struct {
	Total    int     `json:"total"`
	Passed   int     `json:"passed"`
	Failed   int     `json:"failed"`
	Skipped  int     `json:"skipped"`
	PassRate float64 `json:"passRate"`
}

// ErrorInfo contains error details
type ErrorInfo struct {
	Timestamp  time.Time `json:"timestamp"`
	Severity   string    `json:"severity"`  // error, warning, info
	Component  string    `json:"component"` // infrastructure, vm, assertion, resource
	Message    string    `json:"message"`
	Details    string    `json:"details,omitempty"`
	StackTrace string    `json:"stackTrace,omitempty"`
}

// LogPaths contains paths to all log files
type LogPaths struct {
	Framework   string `json:"framework"`
	Dnsmasq     string `json:"dnsmasq"`
	ShaperAPI   string `json:"shaperAPI"`
	Kubectl     string `json:"kubectl"`
	ArtifactDir string `json:"artifactDir"`
}

// VMLogPaths contains VM-specific log paths
type VMLogPaths struct {
	Console string `json:"console"`
	Serial  string `json:"serial"`
}

// TestMetadata contains additional metadata for test grid
type TestMetadata struct {
	FrameworkVersion string `json:"frameworkVersion,omitempty"`
	Hostname         string `json:"hostname,omitempty"`
	CIJobID          string `json:"ciJobID,omitempty"`
	GitCommit        string `json:"gitCommit,omitempty"`
	GitBranch        string `json:"gitBranch,omitempty"`
}

// LogCollection contains collected logs from test execution
type LogCollection struct {
	FrameworkLog  string
	DnsmasqLog    string
	ShaperAPILog  string
	KubectlLog    string
	VMConsoleLogs map[string]string // VM name -> console log
	VMSerialLogs  map[string]string // VM name -> serial log
}
