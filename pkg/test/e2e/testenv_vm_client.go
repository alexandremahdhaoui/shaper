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
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrVMClientCreate indicates a failure to create the VMClient.
	ErrVMClientCreate = errors.New("failed to create VMClient")
	// ErrVMCreate indicates a failure to create a VM.
	ErrVMCreate = errors.New("failed to create VM")
	// ErrVMDelete indicates a failure to delete a VM.
	ErrVMDelete = errors.New("failed to delete VM")
	// ErrVMGetIP indicates a failure to get the VM IP address.
	ErrVMGetIP = errors.New("failed to get VM IP address")
	// ErrVMStart indicates a failure to start a VM.
	ErrVMStart = errors.New("failed to start VM")
)

// VMSpec defines the specification for creating a VM.
type VMSpec struct {
	// Memory in MB
	Memory int
	// VCPUs is the number of virtual CPUs
	VCPUs int
	// Network is the name of the libvirt network to connect to
	Network string
	// BootOrder is the list of boot devices (e.g., ["network", "hd"])
	BootOrder []string
	// Firmware is the firmware type ("bios" or "uefi")
	Firmware string
	// AutoStart determines whether the VM should start automatically after creation
	AutoStart bool
}

// VMClient manages VMs using virsh commands.
type VMClient struct {
	stateDir string
}

// NewVMClient creates a new VMClient.
// The stateDir is used for storing temporary VM-related files.
func NewVMClient(stateDir string) (*VMClient, error) {
	if stateDir == "" {
		stateDir = "/tmp/shaper-testenv-vm"
	}
	return &VMClient{stateDir: stateDir}, nil
}

// CreateVM creates a new VM with the given name and specification.
// If AutoStart is false, the VM is defined but not started (useful for UUID discovery).
func (c *VMClient) CreateVM(ctx context.Context, name string, spec VMSpec) error {
	// Generate libvirt XML for the VM
	xml, err := c.generateVMXML(name, spec)
	if err != nil {
		return errors.Join(ErrVMCreate, err)
	}

	// Write XML to temporary file
	xmlPath := fmt.Sprintf("%s/%s.xml", c.stateDir, name)
	cmd := exec.CommandContext(ctx, "bash", "-c", fmt.Sprintf("mkdir -p %s && cat > %s", c.stateDir, xmlPath))
	cmd.Stdin = strings.NewReader(xml)
	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.Join(ErrVMCreate, errors.New(string(output)), err)
	}

	// Define the VM using virsh
	defineCmd := exec.CommandContext(ctx, "virsh", "define", xmlPath)
	if output, err := defineCmd.CombinedOutput(); err != nil {
		return errors.Join(ErrVMCreate, errors.New(string(output)), err)
	}

	// Start the VM if AutoStart is true
	if spec.AutoStart {
		if err := c.StartVM(ctx, name); err != nil {
			return err
		}
	}

	return nil
}

// StartVM starts a VM by name.
func (c *VMClient) StartVM(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "virsh", "start", name)
	if output, err := cmd.CombinedOutput(); err != nil {
		return errors.Join(ErrVMStart, errors.New(string(output)), err)
	}
	return nil
}

// DeleteVM deletes a VM by name.
// This will stop the VM if running and undefine it.
func (c *VMClient) DeleteVM(ctx context.Context, name string) error {
	// First try to stop the VM (ignore errors if already stopped)
	_ = StopVM(name)

	// Wait a bit for the VM to fully stop
	_ = WaitForVMState(name, "shut off", 10*time.Second)

	// Undefine the VM - try with --nvram first, then without (for BIOS VMs)
	cmd := exec.CommandContext(ctx, "virsh", "undefine", name, "--nvram")
	if _, err := cmd.CombinedOutput(); err != nil {
		// Try without --nvram for BIOS VMs
		cmd = exec.CommandContext(ctx, "virsh", "undefine", name)
		output, err := cmd.CombinedOutput()
		if err != nil {
			// Ignore error if VM doesn't exist
			if strings.Contains(string(output), "Domain not found") ||
				strings.Contains(string(output), "failed to get domain") {
				return nil
			}
			return errors.Join(ErrVMDelete, errors.New(string(output)), err)
		}
	}

	return nil
}

// GetVMIP returns the IP address of a VM.
// It uses virsh domifaddr to query the VM's network interface.
func (c *VMClient) GetVMIP(ctx context.Context, name string) (string, error) {
	// Wait for the VM to get an IP (max 60 seconds)
	deadline := time.Now().Add(60 * time.Second)
	pollInterval := 2 * time.Second

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		ip, err := c.tryGetVMIP(ctx, name)
		if err == nil && ip != "" {
			return ip, nil
		}

		time.Sleep(pollInterval)
	}

	return "", errors.Join(ErrVMGetIP, errors.New("timeout waiting for VM IP"))
}

// tryGetVMIP attempts to get the VM IP once.
// It first tries virsh domifaddr (requires guest agent), then falls back to
// checking dnsmasq leases (for PXE boot VMs without guest agent).
func (c *VMClient) tryGetVMIP(ctx context.Context, name string) (string, error) {
	// Method 1: Try virsh domifaddr (requires guest agent)
	cmd := exec.CommandContext(ctx, "virsh", "domifaddr", name)
	output, err := cmd.Output()
	if err == nil {
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
	}

	// Method 2: Get MAC address and check dnsmasq leases
	// This works for PXE boot VMs without guest agent
	mac, err := c.getVMMAC(ctx, name)
	if err != nil || mac == "" {
		return "", nil
	}

	// Check dnsmasq leases via SSH to DnsmasqServer
	ip, err := c.getIPFromDnsmasqLeases(ctx, mac)
	if err == nil && ip != "" {
		return ip, nil
	}

	return "", nil
}

// getVMMAC returns the MAC address of a VM's first network interface.
func (c *VMClient) getVMMAC(ctx context.Context, name string) (string, error) {
	cmd := exec.CommandContext(ctx, "virsh", "domiflist", name)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	// Parse output:
	// Interface  Type       Source     Model       MAC
	// -------------------------------------------------------
	// -          network    TestNetwork virtio      52:54:00:6e:68:03
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "52:54:") { // MAC addresses from QEMU start with 52:54:
			fields := strings.Fields(line)
			if len(fields) >= 5 {
				return fields[4], nil
			}
		}
	}

	return "", nil
}

// getIPFromDnsmasqLeases checks dnsmasq leases for a given MAC address.
// It SSHs to the DnsmasqServer (192.168.100.2) to read the leases file.
func (c *VMClient) getIPFromDnsmasqLeases(ctx context.Context, mac string) (string, error) {
	// SSH to DnsmasqServer to read leases
	sshKeyPath := fmt.Sprintf("%s/keys/VmSsh", c.stateDir)
	cmd := exec.CommandContext(ctx, "ssh",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		"-o", "ConnectTimeout=5",
		"-i", sshKeyPath,
		"ubuntu@192.168.100.2",
		"cat /var/lib/misc/dnsmasq.leases")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	// Parse leases file:
	// 1766068869 52:54:00:8e:2d:ed 192.168.100.132 * 01:52:54:00:8e:2d:ed
	macLower := strings.ToLower(mac)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			leaseMAC := strings.ToLower(fields[1])
			if leaseMAC == macLower {
				return fields[2], nil
			}
		}
	}

	return "", nil
}

// GetVMUUID returns the UUID of a VM.
func (c *VMClient) GetVMUUID(ctx context.Context, name string) (uuid.UUID, error) {
	return GetVMUUID(name)
}

// GetConsolePath returns the path to the VM's console log file.
func (c *VMClient) GetConsolePath(name string) string {
	return fmt.Sprintf("/tmp/%s-console.log", name)
}

// GetConsoleLog reads the VM's console log.
// This is useful for debugging PXE boot issues.
// Note: Uses sudo because libvirt creates console files as root.
func (c *VMClient) GetConsoleLog(ctx context.Context, name string) (string, error) {
	consolePath := c.GetConsolePath(name)
	// Use sudo cat because libvirt creates console files as root
	cmd := exec.CommandContext(ctx, "sudo", "cat", consolePath)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to read console log: %w", err)
	}
	return string(output), nil
}

// WaitForDnsmasqServerReady waits for the DnsmasqServer VM to be fully ready.
// This checks:
// 1. SSH is accessible
// 2. The iPXE binary exists at /var/lib/tftpboot/undionly.kpxe
// 3. dnsmasq service is running
// This is critical because the DnsmasqServer builds the custom iPXE binary during cloud-init,
// which can take several minutes. VMs that try to PXE boot before this completes will fail.
func (c *VMClient) WaitForDnsmasqServerReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	sshKeyPath := fmt.Sprintf("%s/keys/VmSsh", c.stateDir)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Check if iPXE binary exists
		// Use LogLevel=ERROR to suppress SSH warnings that would pollute the output
		cmd := exec.CommandContext(ctx, "ssh",
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile=/dev/null",
			"-o", "LogLevel=ERROR",
			"-o", "ConnectTimeout=5",
			"-i", sshKeyPath,
			"ubuntu@192.168.100.2",
			"test -f /var/lib/tftpboot/undionly.kpxe && systemctl is-active dnsmasq")
		output, err := cmd.CombinedOutput()
		if err == nil {
			outputStr := strings.TrimSpace(string(output))
			if outputStr == "active" {
				return nil
			}
		}

		time.Sleep(5 * time.Second)
	}

	return errors.New("timeout waiting for DnsmasqServer to be ready (iPXE binary and dnsmasq)")
}

// vmXMLTemplate is the libvirt domain XML template for PXE boot VMs.
// Note: Uses e1000 NIC for PXE boot because it has built-in iPXE ROM.
// virtio requires external ROM file which has permission issues with QEMU sandbox.
// The UUID is explicitly set in both <uuid> and <sysinfo> so iPXE sees the same UUID.
// Console output is captured to a file for debugging PXE boot issues.
const vmXMLTemplate = `<domain type='kvm'>
  <name>{{ .Name }}</name>
  <uuid>{{ .UUID }}</uuid>
  <memory unit='MiB'>{{ .Memory }}</memory>
  <vcpu>{{ .VCPUs }}</vcpu>
  <sysinfo type='smbios'>
    <system>
      <entry name='uuid'>{{ .UUID }}</entry>
    </system>
  </sysinfo>
  <os>
    {{- if eq .Firmware "uefi" }}
    <type arch='x86_64' machine='q35'>hvm</type>
    <loader readonly='yes' type='pflash'>/usr/share/OVMF/OVMF_CODE.fd</loader>
    {{- else }}
    <type arch='x86_64' machine='pc'>hvm</type>
    {{- end }}
    {{- range .BootOrder }}
    <boot dev='{{ . }}'/>
    {{- end }}
    <bios useserial='yes'/>
    <bootmenu enable='yes'/>
    <smbios mode='sysinfo'/>
  </os>
  <features>
    <acpi/>
    <apic/>
  </features>
  <cpu mode='host-passthrough'/>
  <clock offset='utc'/>
  <devices>
    <emulator>/usr/bin/qemu-system-x86_64</emulator>
    <interface type='network'>
      <source network='{{ .Network }}'/>
{{- if .HasNetworkBoot }}
      <model type='e1000'/>
{{- else }}
      <model type='virtio'/>
{{- end }}
    </interface>
    <serial type='file'>
      <source path='{{ .ConsolePath }}' append='off'/>
      <target port='0'/>
    </serial>
    <console type='file'>
      <source path='{{ .ConsolePath }}' append='off'/>
      <target type='serial' port='0'/>
    </console>
    <graphics type='vnc' port='-1' autoport='yes'/>
  </devices>
</domain>`

// generateVMXML generates the libvirt domain XML for the VM.
func (c *VMClient) generateVMXML(name string, spec VMSpec) (string, error) {
	tmpl, err := template.New("vm").Parse(vmXMLTemplate)
	if err != nil {
		return "", err
	}

	// Check if network boot is enabled (needed for PXE ROM)
	hasNetworkBoot := false
	for _, dev := range spec.BootOrder {
		if dev == "network" {
			hasNetworkBoot = true
			break
		}
	}

	// Generate a UUID for the VM. This UUID will be used both as the
	// libvirt domain UUID and (via smbios mode='uuid') as the SMBIOS UUID
	// that iPXE reports in ${uuid}.
	vmUUID := uuid.New()

	// Console log path for capturing PXE boot output
	consolePath := fmt.Sprintf("/tmp/%s-console.log", name)

	data := struct {
		Name           string
		UUID           string
		Memory         int
		VCPUs          int
		Network        string
		BootOrder      []string
		Firmware       string
		HasNetworkBoot bool
		ConsolePath    string
	}{
		Name:           name,
		UUID:           vmUUID.String(),
		Memory:         spec.Memory,
		VCPUs:          spec.VCPUs,
		Network:        spec.Network,
		BootOrder:      spec.BootOrder,
		Firmware:       spec.Firmware,
		HasNetworkBoot: hasNetworkBoot,
		ConsolePath:    consolePath,
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}
