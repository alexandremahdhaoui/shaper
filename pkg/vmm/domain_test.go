/*
Copyright 2024 Alexandre Mahdhaoui

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package vmm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"libvirt.org/go/libvirtxml"
)

func TestGenerateDomainXML_NetworkBoot(t *testing.T) {
	config := &VMConfig{
		Name:        "test-vm",
		MemoryMB:    2048,
		VCPUs:       2,
		NetworkMode: "bridge",
		BridgeName:  "e2e-br0",
		BootOrder:   []string{"network"},
	}

	xmlStr, err := generateDomainXML(config)
	require.NoError(t, err)
	require.NotEmpty(t, xmlStr)

	// Parse XML to verify structure
	var domain libvirtxml.Domain
	err = domain.Unmarshal(xmlStr)
	require.NoError(t, err)

	// Verify name
	assert.Equal(t, "test-vm", domain.Name)

	// Verify memory
	assert.Equal(t, uint(2048), domain.Memory.Value)
	assert.Equal(t, "MiB", domain.Memory.Unit)

	// Verify vCPU
	assert.Equal(t, uint(2), domain.VCPU.Value)

	// Verify boot order
	require.NotNil(t, domain.OS)
	require.Len(t, domain.OS.BootDevices, 1)
	assert.Equal(t, "network", domain.OS.BootDevices[0].Dev)

	// Verify network interface
	require.NotNil(t, domain.Devices)
	require.Len(t, domain.Devices.Interfaces, 1)
	require.NotNil(t, domain.Devices.Interfaces[0].Source)
	require.NotNil(t, domain.Devices.Interfaces[0].Source.Bridge)
	assert.Equal(t, "e2e-br0", domain.Devices.Interfaces[0].Source.Bridge.Bridge)
}

func TestGenerateDomainXML_BridgeNetwork(t *testing.T) {
	config := &VMConfig{
		Name:        "test-vm",
		MemoryMB:    1024,
		VCPUs:       1,
		NetworkMode: "bridge",
		BridgeName:  "virbr0",
		MACAddress:  "52:54:00:12:34:56",
	}

	xmlStr, err := generateDomainXML(config)
	require.NoError(t, err)

	var domain libvirtxml.Domain
	err = domain.Unmarshal(xmlStr)
	require.NoError(t, err)

	// Verify bridge configuration
	require.Len(t, domain.Devices.Interfaces, 1)
	iface := domain.Devices.Interfaces[0]
	require.NotNil(t, iface.Source)
	require.NotNil(t, iface.Source.Bridge)
	assert.Equal(t, "virbr0", iface.Source.Bridge.Bridge)

	// Verify MAC address
	require.NotNil(t, iface.MAC)
	assert.Equal(t, "52:54:00:12:34:56", iface.MAC.Address)
}

func TestGenerateDomainXML_NATNetwork(t *testing.T) {
	config := &VMConfig{
		Name:        "test-vm-nat",
		MemoryMB:    512,
		VCPUs:       1,
		NetworkMode: "nat",
		BridgeName:  "custom-network",
	}

	xmlStr, err := generateDomainXML(config)
	require.NoError(t, err)

	var domain libvirtxml.Domain
	err = domain.Unmarshal(xmlStr)
	require.NoError(t, err)

	// Verify network configuration
	require.Len(t, domain.Devices.Interfaces, 1)
	iface := domain.Devices.Interfaces[0]
	require.NotNil(t, iface.Source)
	require.NotNil(t, iface.Source.Network)
	assert.Equal(t, "custom-network", iface.Source.Network.Network)
}

func TestGenerateDomainXML_UserNetwork(t *testing.T) {
	config := &VMConfig{
		Name:        "test-vm-user",
		MemoryMB:    512,
		VCPUs:       1,
		NetworkMode: "user",
	}

	xmlStr, err := generateDomainXML(config)
	require.NoError(t, err)

	var domain libvirtxml.Domain
	err = domain.Unmarshal(xmlStr)
	require.NoError(t, err)

	// Verify user network
	require.Len(t, domain.Devices.Interfaces, 1)
	require.NotNil(t, domain.Devices.Interfaces[0].Source)
	require.NotNil(t, domain.Devices.Interfaces[0].Source.User)
}

func TestGenerateDomainXML_GeneratedMAC(t *testing.T) {
	config := &VMConfig{
		Name:        "test-vm",
		MemoryMB:    1024,
		VCPUs:       1,
		NetworkMode: "user",
		// No MAC address specified
	}

	xmlStr, err := generateDomainXML(config)
	require.NoError(t, err)

	var domain libvirtxml.Domain
	err = domain.Unmarshal(xmlStr)
	require.NoError(t, err)

	// Verify MAC was generated
	require.Len(t, domain.Devices.Interfaces, 1)
	require.NotNil(t, domain.Devices.Interfaces[0].MAC)
	macAddr := domain.Devices.Interfaces[0].MAC.Address
	assert.NotEmpty(t, macAddr)

	// Verify it starts with libvirt prefix
	assert.True(t, strings.HasPrefix(macAddr, "52:54:00:"),
		"MAC address should start with libvirt prefix 52:54:00")
}

func TestGenerateDomainXML_MemoryAndCPU(t *testing.T) {
	tests := []struct {
		name     string
		memoryMB int
		vcpus    int
	}{
		{"512MB-1CPU", 512, 1},
		{"2GB-2CPU", 2048, 2},
		{"4GB-4CPU", 4096, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &VMConfig{
				Name:        tt.name,
				MemoryMB:    tt.memoryMB,
				VCPUs:       tt.vcpus,
				NetworkMode: "user",
			}

			xmlStr, err := generateDomainXML(config)
			require.NoError(t, err)

			var domain libvirtxml.Domain
			err = domain.Unmarshal(xmlStr)
			require.NoError(t, err)

			assert.Equal(t, uint(tt.memoryMB), domain.Memory.Value)
			assert.Equal(t, uint(tt.vcpus), domain.VCPU.Value)
		})
	}
}

func TestGenerateDomainXML_DefaultNetworkBoot(t *testing.T) {
	config := &VMConfig{
		Name:        "test-vm",
		MemoryMB:    1024,
		VCPUs:       1,
		NetworkMode: "user",
		// No BootOrder specified
	}

	xmlStr, err := generateDomainXML(config)
	require.NoError(t, err)

	var domain libvirtxml.Domain
	err = domain.Unmarshal(xmlStr)
	require.NoError(t, err)

	// Should default to network boot
	require.Len(t, domain.OS.BootDevices, 1)
	assert.Equal(t, "network", domain.OS.BootDevices[0].Dev)
}

func TestGenerateDomainXML_MultipleBootDevices(t *testing.T) {
	config := &VMConfig{
		Name:        "test-vm",
		MemoryMB:    1024,
		VCPUs:       1,
		NetworkMode: "user",
		BootOrder:   []string{"network", "hd"},
	}

	xmlStr, err := generateDomainXML(config)
	require.NoError(t, err)

	var domain libvirtxml.Domain
	err = domain.Unmarshal(xmlStr)
	require.NoError(t, err)

	require.Len(t, domain.OS.BootDevices, 2)
	assert.Equal(t, "network", domain.OS.BootDevices[0].Dev)
	assert.Equal(t, "hd", domain.OS.BootDevices[1].Dev)
}

func TestGenerateDomainXML_SerialConsole(t *testing.T) {
	config := &VMConfig{
		Name:        "test-vm",
		MemoryMB:    1024,
		VCPUs:       1,
		NetworkMode: "user",
	}

	xmlStr, err := generateDomainXML(config)
	require.NoError(t, err)

	var domain libvirtxml.Domain
	err = domain.Unmarshal(xmlStr)
	require.NoError(t, err)

	// Verify serial console
	require.Len(t, domain.Devices.Serials, 1)
	require.NotNil(t, domain.Devices.Serials[0].Source)
	require.NotNil(t, domain.Devices.Serials[0].Source.Pty)

	// Verify console
	require.Len(t, domain.Devices.Consoles, 1)
	require.NotNil(t, domain.Devices.Consoles[0].Source)
	require.NotNil(t, domain.Devices.Consoles[0].Source.Pty)
	assert.Equal(t, "serial", domain.Devices.Consoles[0].Target.Type)
}

func TestGenerateDomainXML_VNCGraphics(t *testing.T) {
	config := &VMConfig{
		Name:        "test-vm",
		MemoryMB:    1024,
		VCPUs:       1,
		NetworkMode: "user",
	}

	xmlStr, err := generateDomainXML(config)
	require.NoError(t, err)

	var domain libvirtxml.Domain
	err = domain.Unmarshal(xmlStr)
	require.NoError(t, err)

	// Verify VNC graphics
	require.Len(t, domain.Devices.Graphics, 1)
	require.NotNil(t, domain.Devices.Graphics[0].VNC)
	assert.Equal(t, -1, domain.Devices.Graphics[0].VNC.Port)
	assert.Equal(t, "yes", domain.Devices.Graphics[0].VNC.AutoPort)
}

func TestGenerateDomainXML_OSType(t *testing.T) {
	config := &VMConfig{
		Name:        "test-vm",
		MemoryMB:    1024,
		VCPUs:       1,
		NetworkMode: "user",
	}

	xmlStr, err := generateDomainXML(config)
	require.NoError(t, err)

	var domain libvirtxml.Domain
	err = domain.Unmarshal(xmlStr)
	require.NoError(t, err)

	// Verify OS type
	require.NotNil(t, domain.OS.Type)
	assert.Equal(t, "x86_64", domain.OS.Type.Arch)
	assert.Equal(t, "pc", domain.OS.Type.Machine)
	assert.Equal(t, "hvm", domain.OS.Type.Type)
}

func TestGenerateRandomMAC(t *testing.T) {
	// Generate multiple MACs to ensure uniqueness
	macs := make(map[string]bool)
	for i := 0; i < 100; i++ {
		mac, err := generateRandomMAC()
		require.NoError(t, err)

		// Verify format
		assert.Regexp(t, `^52:54:00:[0-9a-f]{2}:[0-9a-f]{2}:[0-9a-f]{2}$`, mac)

		// Verify uniqueness (should be very likely with 100 samples)
		assert.False(t, macs[mac], "Generated duplicate MAC address")
		macs[mac] = true
	}
}
