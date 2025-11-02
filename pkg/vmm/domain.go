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
	"crypto/rand"
	"fmt"

	"libvirt.org/go/libvirtxml"
)

// generateDomainXML creates libvirt domain XML from VMConfig
// Returns XML string ready for libvirt.DomainDefineXML()
func generateDomainXML(config *VMConfig) (string, error) {
	// Generate MAC address if not provided
	macAddress := config.MACAddress
	if macAddress == "" {
		var err error
		macAddress, err = generateRandomMAC()
		if err != nil {
			return "", fmt.Errorf("generate MAC address: %w", err)
		}
	}

	// Build boot devices list
	bootDevices := make([]libvirtxml.DomainBootDevice, 0)
	for _, dev := range config.BootOrder {
		bootDevices = append(bootDevices, libvirtxml.DomainBootDevice{Dev: dev})
	}
	// Default to network boot if not specified
	if len(bootDevices) == 0 {
		bootDevices = []libvirtxml.DomainBootDevice{{Dev: "network"}}
	}

	domain := &libvirtxml.Domain{
		Type: "kvm",
		Name: config.Name,
		Memory: &libvirtxml.DomainMemory{
			Value: uint(config.MemoryMB),
			Unit:  "MiB",
		},
		VCPU: &libvirtxml.DomainVCPU{
			Value: uint(config.VCPUs),
		},
		OS: &libvirtxml.DomainOS{
			Type: &libvirtxml.DomainOSType{
				Arch:    "x86_64",
				Machine: "pc",
				Type:    "hvm",
			},
			BootDevices: bootDevices,
		},
		Devices: &libvirtxml.DomainDeviceList{
			Interfaces: []libvirtxml.DomainInterface{
				buildNetworkInterface(config.NetworkMode, config.BridgeName, macAddress),
			},
			Serials: []libvirtxml.DomainSerial{
				{
					Source: &libvirtxml.DomainChardevSource{
						Pty: &libvirtxml.DomainChardevSourcePty{},
					},
					Target: &libvirtxml.DomainSerialTarget{
						Port: uintPtr(0),
					},
				},
			},
			Consoles: []libvirtxml.DomainConsole{
				{
					Source: &libvirtxml.DomainChardevSource{
						Pty: &libvirtxml.DomainChardevSourcePty{},
					},
					Target: &libvirtxml.DomainConsoleTarget{
						Type: "serial",
						Port: uintPtr(0),
					},
				},
			},
			Graphics: []libvirtxml.DomainGraphic{
				{
					VNC: &libvirtxml.DomainGraphicVNC{
						Port:     -1,
						AutoPort: "yes",
					},
				},
			},
		},
	}

	// Marshal to XML
	xmlBytes, err := domain.Marshal()
	if err != nil {
		return "", fmt.Errorf("marshal domain XML: %w", err)
	}

	return string(xmlBytes), nil
}

// buildNetworkInterface creates a network interface configuration
func buildNetworkInterface(mode, bridgeName, macAddress string) libvirtxml.DomainInterface {
	iface := libvirtxml.DomainInterface{
		Model: &libvirtxml.DomainInterfaceModel{
			Type: "virtio",
		},
		MAC: &libvirtxml.DomainInterfaceMAC{
			Address: macAddress,
		},
	}

	switch mode {
	case "bridge":
		iface.Source = &libvirtxml.DomainInterfaceSource{
			Bridge: &libvirtxml.DomainInterfaceSourceBridge{
				Bridge: bridgeName,
			},
		}
	case "nat", "network":
		networkName := "default"
		if bridgeName != "" {
			networkName = bridgeName
		}
		iface.Source = &libvirtxml.DomainInterfaceSource{
			Network: &libvirtxml.DomainInterfaceSourceNetwork{
				Network: networkName,
			},
		}
	case "user":
		iface.Source = &libvirtxml.DomainInterfaceSource{
			User: &libvirtxml.DomainInterfaceSourceUser{},
		}
	default:
		// Default to user mode
		iface.Source = &libvirtxml.DomainInterfaceSource{
			User: &libvirtxml.DomainInterfaceSourceUser{},
		}
	}

	return iface
}

// generateRandomMAC generates a random MAC address with libvirt's prefix (52:54:00)
func generateRandomMAC() (string, error) {
	buf := make([]byte, 3)
	_, err := rand.Read(buf)
	if err != nil {
		return "", err
	}

	// Use libvirt's prefix: 52:54:00
	return fmt.Sprintf("52:54:00:%02x:%02x:%02x", buf[0], buf[1], buf[2]), nil
}

// uintPtr is a helper to get pointer to uint
func uintPtr(v uint) *uint {
	return &v
}
