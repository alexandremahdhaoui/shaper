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

package types

import (
	"encoding/hex"
	"net"
	"strings"

	"github.com/google/uuid"
)

// -------------------------------------------------- PARAMETERS ---------------------------------------------------- //

const (
	Mac        = "mac"         //	MAC address
	BusType    = "bustype"     // Bus type
	BusLoc     = "busloc"      // Bus location
	BusID      = "busid"       // Bus ExposedConfigID
	Chip       = "chip"        // Chip type
	Ssid       = "ssid"        // Wireless SSID
	ActiveScan = "active-scan" // Actively scan for wireless orks
	Key        = "key"         // Wireless encryption key

	// IPv4 settings

	Ip      = "ip"      // IP address
	Netmask = "netmask" // Subnet mask
	Gateway = "gateway" // Default gateway
	Dns     = "dns"     // DNS server
	Domain  = "domain"  // DNS domain

	// Boot settings

	Filename     = "filename"      // Boot filename
	NextServer   = "next-server"   // TFTP server
	RootPath     = "root-path"     // SAN root path
	SanFilename  = "scan-filename" // SAN filename
	InitiatorIqn = "initiator-iqn" // iSCSI initiator name
	KeepSan      = "keep-san"      // Preserve SAN connection
	SkipSanBoot  = "skip-san-boot" // Do not boot from SAN device

	// Host settings

	Hostname     = "hostname"     // Host name
	Uuid         = "uuid"         // UUID
	UserClass    = "user-class"   // DHCP user class
	Manufacturer = "manufacturer" // Manufacturer
	Product      = "product"      // Product name
	Serial       = "serial"       // Serial number
	Asset        = "asset"        // Asset tag

	// Authentication settings

	Username        = "username"         // User name
	Password        = "password"         // Password
	ReverseUsername = "reverse-username" // Reverse user name
	ReversePassword = "reverse-password" // Reverse password

	// Cryptography settings

	Crosscert = "crosscert" // Cross-signed certificate source
	Trust     = "trust"     // Trusted root certificate fingerprints
	Cert      = "cert"      // Client certificate
	Privkey   = "privkey"   // Client private key

	// Miscellaneous settings

	Buildarch  = "buildarch"   // Build architecture
	Cpumodel   = "cpumodel"    // CPU model
	Cpuvendor  = "cpuvendor"   // CPU vendor
	DhcpServer = "dhcp-server" // DHCP server
	Keymap     = "keymap"      // Keyboard layout
	Memsize    = "memsize"     // Memory size
	Platform   = "platform"    // Firmware platform
	Priority   = "priority"    // Settings priority
	Scriptlet  = "scriptlet"   // Boot scriptlet
	Syslog     = "syslog"      // Syslog server
	Syslogs    = "syslogs"     // Encrypted syslog server
	Sysmac     = "sysmac"      // System MAC address
	Unixtime   = "unixtime"    // Seconds since the Epoch
	UseCached  = "use-cached"  // Use cached settings
	Version    = "version"     // iPXE version
	Vram       = "vram"        // Video RAM contents
)

// --- PARAMS --- //

// IpxeParams is a struct that holds all the possible iPXE parameters.
type IpxeParams struct {
	// Mac is the MAC address of the network interface.
	Mac *hexa //	MAC address
	// BusType is the bus type of the network interface.
	BusType *string // Bus type
	// BusLoc is the bus location of the network interface.
	BusLoc *uint32 // Bus location
	// BusID is the bus ID of the network interface.
	BusID *hexa // Bus ExposedConfigID
	// Chip is the chip type of the network interface.
	Chip *string // Chip type
	// Ssid is the wireless SSID.
	Ssid *string // Wireless SSID
	// ActiveScan is whether to actively scan for wireless networks.
	ActiveScan *int8 // Actively scan for wireless orks
	// Key is the wireless encryption key.
	Key *string // Wireless encryption key

	// IPv4 settings

	// Ip is the IP address of the network interface.
	Ip *net.IP // IP address
	// Netmask is the netmask of the network interface.
	Netmask *net.IP // Subnet mask
	// Gateway is the gateway of the network interface.
	Gateway *net.IP // Default gateway
	// Dns is the DNS server of the network interface.
	Dns *net.IP // DNS server
	// Domain is the domain of the network interface.
	Domain *string // DNS domain

	// Boot settings

	// Filename is the boot filename.
	Filename *string // Boot filename
	// NextServer is the next server.
	NextServer *net.IP // TFTP server
	// RootPath is the root path.
	RootPath *string // SAN root path
	// SanFilename is the SAN filename.
	SanFilename *string // SAN filename
	// InitiatorIqn is the initiator IQN.
	InitiatorIqn *string // iSCSI initiator name
	// KeepSan is whether to keep the SAN.
	KeepSan *int8 // Preserve SAN connection
	// SkipSanBoot is whether to skip the SAN boot.
	SkipSanBoot *int8 // Do not boot from SAN device

	// Host settings

	// Hostname is the hostname of the machine.
	Hostname *string // Host name
	// UUID is the UUID of the machine.
	UUID *uuid.UUID // UUID
	// UserClass is the user class of the machine.
	UserClass *string // DHCP user class
	// Manufacturer is the manufacturer of the machine.
	Manufacturer *string // Manufacturer
	// Product is the product of the machine.
	Product *string // Product name
	// Serial is the serial number of the machine.
	Serial *string // Serial number
	// Asset is the asset tag of the machine.
	Asset *string // Asset tag

	// Authentication settings

	// Username is the username for authentication.
	Username *string // User name
	// Password is the password for authentication.
	Password *string // Password
	// ReverseUsername is the reverse username for authentication.
	ReverseUsername *string // Reverse user name
	// ReversePassword is the reverse password for authentication.
	ReversePassword *string // Reverse password

	// Cryptography settings

	// Crosscert is the cross-signed certificate.
	Crosscert *string // Cross-signed certificate source
	// Trust is the trusted certificate.
	Trust *hexa // Trusted root certificate fingerprints
	// Cert is the certificate.
	Cert *hexa // Client certificate
	// Privkey is the private key.
	Privkey *hexa // Client private key

	// Miscellaneous settings

	// Buildarch is the build architecture of the machine.
	Buildarch *string // Build architecture
	// Cpumodel is the CPU model of the machine.
	Cpumodel *string // CPU model
	// Cpuvendor is the CPU vendor of the machine.
	Cpuvendor *string // CPU vendor
	// DhcpServer is the DHCP server.
	DhcpServer *net.IP // DHCP server
	// Keymap is the keymap of the machine.
	Keymap *string // Keyboard layout
	// Memsize is the memory size of the machine.
	Memsize *int32 // Memory size
	// Platform is the platform of the machine.
	Platform *string // Firmware platform
	// Priority is the priority of the machine.
	Priority *int8 // Settings priority
	// Scriptlet is the scriptlet.
	Scriptlet *string // Boot scriptlet
	// Syslog is the syslog server.
	Syslog *net.IP // Syslog server
	// Syslogs is the syslogs server.
	Syslogs *string // Encrypted syslog server
	// Sysmac is the system MAC address.
	Sysmac *hexa // System MAC address
	// Unixtime is the unixtime.
	Unixtime *uint32 // Seconds since the Epoch
	// UseCached is whether to use cached settings.
	UseCached *uint8 // Use cached settings
	// Version is the version of iPXE.
	Version *string // iPXE version
	// Vram is the VRAM of the machine.
	Vram *[]byte // Video RAM contents
}

type hexa []byte

// UnmarshalText implements the [encoding.TextUnmarshaler] interface.
func (b *hexa) UnmarshalText(text []byte) error {
	*b = make(hexa, 0)

	for _, s := range strings.Split(string(text), ":") {
		decoded, err := hex.DecodeString(s)
		if err != nil {
			return err // TODO: write this err.
		}

		*b = append(*b, decoded...)
	}

	return nil
}

// ------------------------------------------------ LABEL SELECTORS ------------------------------------------------- //

// IPXESelectors is a struct that holds the selectors for an iPXE boot.
type IPXESelectors struct {
	// Buildarch is the build architecture of the machine.
	Buildarch string
	// UUID is the UUID of the machine.
	UUID uuid.UUID
}
