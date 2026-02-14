//go:build e2e

// Copyright 2024 Alexandre Mahdhaoui
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package e2e

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrEnvVarNotSet indicates a required testenv environment variable is not set
var ErrEnvVarNotSet = errors.New("testenv environment variable not set")

// ErrStateFileNotFound indicates the testenv-vm state file was not found
var ErrStateFileNotFound = errors.New("testenv-vm state file not found")

// ErrVMIPNotFound indicates the VM IP could not be determined
var ErrVMIPNotFound = errors.New("VM IP address not found")

// TestenvConfig holds configuration from testenv-vm environment variables
type TestenvConfig struct {
	// VM information
	VMPXEClientIP  string
	VMPXEClientMAC string

	// SSH key information
	SSHKeyPath string

	// Network information
	BridgeIP        string
	BridgeInterface string

	// Kubernetes
	Kubeconfig string

	// ProjectRoot is the absolute path to the project root directory.
	// This is used to locate Helm charts and other project resources.
	ProjectRoot string

	// Isolation fields for parallel test execution
	// VMNamePrefix is a short hash prefix applied to all libvirt resource names.
	// Set by testenv-vm orchestrator via TESTENV_VM_NAME_PREFIX env var.
	VMNamePrefix string
	// DnsmasqServerIP is the IP address of the DnsmasqServer VM on the isolated subnet.
	// Derived from TESTENV_VM_DNSMASQSERVER_IP env var.
	DnsmasqServerIP string
}

// testenvVMState represents the structure of the testenv-vm state file
type testenvVMState struct {
	ID        string `json:"id"`
	Stage     string `json:"stage"`
	Status    string `json:"status"`
	Resources struct {
		Keys map[string]struct {
			State struct {
				PrivateKeyPath string `json:"privateKeyPath"`
			} `json:"state"`
		} `json:"keys"`
		Networks map[string]struct {
			State struct {
				IP            string `json:"ip"`
				InterfaceName string `json:"interfaceName"`
			} `json:"state"`
		} `json:"networks"`
		VMs map[string]struct {
			State struct {
				Name string `json:"name"`
				MAC  string `json:"mac"`
				UUID string `json:"uuid"`
			} `json:"state"`
		} `json:"vms"`
	} `json:"resources"`
}

// LoadTestenvConfig loads configuration from testenv-vm environment variables
// If environment variables are not set, it falls back to reading the testenv-vm state file
// Returns error if configuration cannot be loaded from either source
func LoadTestenvConfig() (*TestenvConfig, error) {
	// First try to load from environment variables
	cfg, envErr := loadFromEnv()
	if envErr == nil {
		return cfg, nil
	}

	// If env vars are not set, try to load from state file
	cfg, stateErr := loadFromStateFile()
	if stateErr == nil {
		return cfg, nil
	}

	// Return the env error as primary (more informative)
	return nil, errors.Join(envErr, stateErr)
}

// loadFromEnv loads configuration from environment variables
func loadFromEnv() (*TestenvConfig, error) {
	cfg := &TestenvConfig{}
	var missing []string

	// Load VM configuration (name: PxeClient in forge.yaml)
	// Note: VMPXEClientIP is optional for tests that create runtime VMs
	cfg.VMPXEClientIP = os.Getenv("TESTENV_VM_PXECLIENT_IP")
	// IP is optional - tests may create runtime VMs dynamically

	cfg.VMPXEClientMAC = os.Getenv("TESTENV_VM_PXECLIENT_MAC")
	// MAC is optional

	// Load SSH key (name: VmSsh in forge.yaml)
	// Note: SSH key is optional for PXE boot tests (VMs don't have SSH)
	cfg.SSHKeyPath = os.Getenv("TESTENV_KEY_VMSSH_PRIVATE_PATH")
	// SSH key is optional - PXE boot VMs don't use SSH

	// Load network configuration (name: TestNetwork in forge.yaml)
	cfg.BridgeIP = os.Getenv("TESTENV_NETWORK_TESTNETWORK_IP")
	// Bridge IP is optional - used for debugging only

	cfg.BridgeInterface = os.Getenv("TESTENV_NETWORK_TESTNETWORK_INTERFACE")
	// Interface name is optional

	// Load isolation config for parallel test execution
	cfg.VMNamePrefix = os.Getenv("TESTENV_VM_NAME_PREFIX")
	cfg.DnsmasqServerIP = os.Getenv("TESTENV_VM_DNSMASQSERVER_IP")

	// Load Kubernetes config - this is required
	cfg.Kubeconfig = os.Getenv("KUBECONFIG")
	if cfg.Kubeconfig == "" {
		missing = append(missing, "KUBECONFIG")
	}

	if len(missing) > 0 {
		return cfg, errors.Join(ErrEnvVarNotSet,
			errors.New("missing: "+strings.Join(missing, ", ")))
	}

	// Find project root
	cfg.ProjectRoot = findProjectRoot()

	return cfg, nil
}

// loadFromStateFile loads configuration from the testenv-vm state file
func loadFromStateFile() (*TestenvConfig, error) {
	stateDir := os.Getenv("TESTENV_VM_STATE_DIR")
	if stateDir == "" {
		stateDir = "/tmp/shaper-testenv-vm"
	}

	// Find the most recent state file for the e2e stage
	stateFilesDir := filepath.Join(stateDir, "state")
	entries, err := os.ReadDir(stateFilesDir)
	if err != nil {
		return nil, errors.Join(ErrStateFileNotFound, err)
	}

	// Find a state file for e2e stage (format: testenv-{stage}-{timestamp}-{id}.json)
	var stateFile string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "testenv-") && strings.HasSuffix(entry.Name(), ".json") {
			// Check if it's for e2e stage by reading and parsing
			candidatePath := filepath.Join(stateFilesDir, entry.Name())
			data, err := os.ReadFile(candidatePath)
			if err != nil {
				continue
			}
			var state testenvVMState
			if err := json.Unmarshal(data, &state); err != nil {
				continue
			}
			if state.Stage == "e2e" && state.Status == "ready" {
				stateFile = candidatePath
				break
			}
		}
	}

	if stateFile == "" {
		return nil, errors.Join(ErrStateFileNotFound, errors.New("no e2e state file found in "+stateFilesDir))
	}

	// Read and parse the state file
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return nil, errors.Join(ErrStateFileNotFound, err)
	}

	var state testenvVMState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, errors.Join(ErrStateFileNotFound, err)
	}

	cfg := &TestenvConfig{}

	// Extract key information
	if keyState, ok := state.Resources.Keys["VmSsh"]; ok {
		cfg.SSHKeyPath = keyState.State.PrivateKeyPath
	}

	// Extract network information
	if netState, ok := state.Resources.Networks["TestNetwork"]; ok {
		cfg.BridgeIP = netState.State.IP
		cfg.BridgeInterface = netState.State.InterfaceName
	}

	// Extract VM information
	if vmState, ok := state.Resources.VMs["PxeClient"]; ok {
		cfg.VMPXEClientMAC = vmState.State.MAC

		// Get VM IP using virsh domifaddr.
		// This is best-effort: when libvirt's built-in DHCP is disabled
		// (so DnsmasqServer is the sole DHCP provider), virsh domifaddr
		// won't have lease data. The IP is optional for PXE boot tests.
		vmIP, _ := getVMIPFromLibvirt(vmState.State.Name)
		cfg.VMPXEClientIP = vmIP
	}

	// Load isolation config for parallel test execution (from env, even in state-file path)
	cfg.VMNamePrefix = os.Getenv("TESTENV_VM_NAME_PREFIX")
	cfg.DnsmasqServerIP = os.Getenv("TESTENV_VM_DNSMASQSERVER_IP")

	// Load Kubernetes config from environment (this should still be set by forge)
	cfg.Kubeconfig = os.Getenv("KUBECONFIG")
	if cfg.Kubeconfig == "" {
		// Try common locations
		homeDir, _ := os.UserHomeDir()
		kubeconfigPaths := []string{
			filepath.Join(stateDir, "kubeconfig"),
			filepath.Join(homeDir, ".kube", "config"),
		}
		for _, path := range kubeconfigPaths {
			if _, err := os.Stat(path); err == nil {
				cfg.Kubeconfig = path
				break
			}
		}
	}

	// Validate required fields - only Kubeconfig is truly required
	// VMPXEClientIP, SSHKeyPath, BridgeIP are optional for PXE boot tests
	// that create runtime VMs dynamically
	var missing []string
	if cfg.Kubeconfig == "" {
		missing = append(missing, "Kubeconfig")
	}

	if len(missing) > 0 {
		return cfg, errors.New("missing fields from state file: " + strings.Join(missing, ", "))
	}

	// Find project root
	cfg.ProjectRoot = findProjectRoot()

	return cfg, nil
}

// findProjectRoot finds the project root directory by looking for go.mod file.
// It searches upward from the current working directory.
func findProjectRoot() string {
	// Try PROJECT_ROOT environment variable first
	if root := os.Getenv("PROJECT_ROOT"); root != "" {
		return root
	}

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	// Walk up the directory tree looking for go.mod
	dir := cwd
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root, not found
			break
		}
		dir = parent
	}

	// Fallback: try common locations relative to state dir
	stateDir := os.Getenv("TESTENV_VM_STATE_DIR")
	if stateDir != "" {
		// State dir is typically /path/to/project/.forge/tmp/...
		// Navigate up to find project root
		dir := stateDir
		for i := 0; i < 10; i++ {
			goModPath := filepath.Join(dir, "go.mod")
			if _, err := os.Stat(goModPath); err == nil {
				return dir
			}
			dir = filepath.Dir(dir)
		}
	}

	return ""
}

// getVMIPFromLibvirt queries libvirt to get the VM's IP address
func getVMIPFromLibvirt(vmName string) (string, error) {
	// Use virsh domifaddr to get the VM's IP address
	cmd := exec.Command("virsh", "domifaddr", vmName)
	output, err := cmd.Output()
	if err != nil {
		return "", errors.Join(errors.New("virsh domifaddr failed"), err)
	}

	// Parse the output to extract IP address
	// Format:
	//  Name       MAC address          Protocol     Address
	// -------------------------------------------------------------------------------
	//  vnet0      52:54:00:6e:68:03    ipv4         192.168.100.103/24
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 4 && fields[2] == "ipv4" {
			// Extract IP from "192.168.100.103/24" format
			ipWithMask := fields[3]
			ip := strings.Split(ipWithMask, "/")[0]
			return ip, nil
		}
	}

	return "", errors.New("no IPv4 address found in virsh output")
}

// MustLoadTestenvConfig loads configuration or panics
// Use in tests where missing config should fail the test
func MustLoadTestenvConfig() *TestenvConfig {
	cfg, err := LoadTestenvConfig()
	if err != nil {
		panic("failed to load testenv config: " + err.Error())
	}
	return cfg
}
