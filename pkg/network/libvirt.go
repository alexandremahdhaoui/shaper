package network

import (
	"context"
	"errors"
	"fmt"

	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

// Error variables for libvirt network operations
var (
	ErrNetworkNameRequired = errors.New("network name is required")
	ErrConnNil             = errors.New("libvirt connection is nil")
	ErrCreateNetwork       = errors.New("failed to create libvirt network")
	ErrDefineNetwork       = errors.New("failed to define libvirt network")
	ErrStartNetwork        = errors.New("failed to start libvirt network")
	ErrDestroyNetwork      = errors.New("failed to destroy libvirt network")
	ErrUndefineNetwork     = errors.New("failed to undefine libvirt network")
	ErrCheckNetwork        = errors.New("failed to check if network exists")
	ErrMarshalNetworkXML   = errors.New("failed to marshal network XML")
	ErrNetworkNotFound     = errors.New("libvirt network not found")
)

// LibvirtNetworkConfig contains libvirt network configuration
type LibvirtNetworkConfig struct {
	Name       string
	BridgeName string // Linux bridge to attach to
	Mode       string // "bridge", "nat", "isolated"
	IPAddress  string // IP address for NAT/isolated mode (e.g., "192.168.150.1")
	Netmask    string // Netmask for NAT/isolated mode (e.g., "255.255.255.0")
}

// LibvirtNetworkManager manages libvirt virtual networks
type LibvirtNetworkManager struct {
	conn *libvirt.Connect
}

// NewLibvirtNetworkManager creates a new LibvirtNetworkManager
func NewLibvirtNetworkManager(conn *libvirt.Connect) *LibvirtNetworkManager {
	return &LibvirtNetworkManager{
		conn: conn,
	}
}

// LibvirtNetworkInfo contains information about a libvirt network
type LibvirtNetworkInfo struct {
	Name       string
	BridgeName string
	Mode       string
	IsActive   bool
	Autostart  bool
}

// Create creates a new libvirt network with the given configuration
// Idempotent - if network exists, ensures it's active
func (m *LibvirtNetworkManager) Create(ctx context.Context, config LibvirtNetworkConfig) error {
	if m.conn == nil {
		return ErrConnNil
	}
	if config.Name == "" {
		return ErrNetworkNameRequired
	}

	// Set default mode if not specified
	if config.Mode == "" {
		config.Mode = "bridge"
	}

	// Check if network already exists
	info, err := m.Get(ctx, config.Name)
	if err != nil && !errors.Is(err, ErrNetworkNotFound) {
		// An error other than "not found" occurred
		return err
	}
	if info != nil {
		// Network already exists, ensure it's active
		return m.ensureNetworkActive(config.Name)
	}

	// Generate network XML
	networkXML, err := GenerateNetworkXML(config)
	if err != nil {
		return err
	}

	// Define the network
	network, err := m.conn.NetworkDefineXML(networkXML)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDefineNetwork, err)
	}
	defer func() { _ = network.Free() }()

	// Start the network
	if err := network.Create(); err != nil {
		// Try to undefine on failure
		_ = network.Undefine()
		return fmt.Errorf("%w: %v", ErrStartNetwork, err)
	}

	// Set network to autostart (don't fail if this errors - autostart is not critical)
	_ = network.SetAutostart(true)

	return nil
}

// ensureNetworkActive ensures a network is active (started)
func (m *LibvirtNetworkManager) ensureNetworkActive(name string) error {
	network, err := m.conn.LookupNetworkByName(name)
	if err != nil {
		return fmt.Errorf("failed to lookup network: %v", err)
	}
	defer func() { _ = network.Free() }()

	active, err := network.IsActive()
	if err != nil {
		return fmt.Errorf("failed to check network state: %v", err)
	}

	if !active {
		if err := network.Create(); err != nil {
			return fmt.Errorf("%w: %v", ErrStartNetwork, err)
		}
	}

	return nil
}

// Get retrieves information about a libvirt network
// Returns ErrNetworkNotFound if the network doesn't exist
func (m *LibvirtNetworkManager) Get(ctx context.Context, name string) (*LibvirtNetworkInfo, error) {
	if name == "" {
		return nil, ErrNetworkNameRequired
	}
	if m.conn == nil {
		return nil, ErrConnNil
	}

	// Try to lookup the network
	network, err := m.conn.LookupNetworkByName(name)
	if err != nil {
		// Check if error is because network doesn't exist
		libvirtErr, ok := err.(libvirt.Error)
		if ok && libvirtErr.Code == libvirt.ERR_NO_NETWORK {
			return nil, ErrNetworkNotFound
		}
		// Some other error
		return nil, fmt.Errorf("%w: %v", ErrCheckNetwork, err)
	}
	defer func() { _ = network.Free() }()

	// Get active status
	isActive, err := network.IsActive()
	if err != nil {
		return nil, fmt.Errorf("failed to check network state: %v", err)
	}

	// Get autostart status
	autostart, err := network.GetAutostart()
	if err != nil {
		return nil, fmt.Errorf("failed to check autostart: %v", err)
	}

	// Get network XML to extract bridge name and mode
	xmlDesc, err := network.GetXMLDesc(0)
	if err != nil {
		return nil, fmt.Errorf("failed to get network XML: %v", err)
	}

	// Parse XML
	var networkXML libvirtxml.Network
	if err := networkXML.Unmarshal(xmlDesc); err != nil {
		return nil, fmt.Errorf("failed to parse network XML: %v", err)
	}

	// Extract bridge name and mode
	bridgeName := ""
	if networkXML.Bridge != nil {
		bridgeName = networkXML.Bridge.Name
	}

	mode := "isolated" // default
	if networkXML.Forward != nil {
		mode = networkXML.Forward.Mode
	}

	return &LibvirtNetworkInfo{
		Name:       name,
		BridgeName: bridgeName,
		Mode:       mode,
		IsActive:   isActive,
		Autostart:  autostart,
	}, nil
}

// Delete removes a libvirt network
// Idempotent - returns nil if network doesn't exist
func (m *LibvirtNetworkManager) Delete(ctx context.Context, name string) error {
	if name == "" {
		return ErrNetworkNameRequired
	}
	if m.conn == nil {
		return ErrConnNil
	}

	// Check if network exists
	_, err := m.Get(ctx, name)
	if err != nil {
		if errors.Is(err, ErrNetworkNotFound) {
			// Network doesn't exist, nothing to do
			return nil
		}
		// Some other error occurred during Get
		return err
	}

	// Look up the network
	network, err := m.conn.LookupNetworkByName(name)
	if err != nil {
		// Network might have been deleted between exists check and lookup
		libvirtErr, ok := err.(libvirt.Error)
		if ok && libvirtErr.Code == libvirt.ERR_NO_NETWORK {
			return nil
		}
		return fmt.Errorf("failed to lookup network: %v", err)
	}
	defer func() { _ = network.Free() }()

	// Check if network is active
	active, err := network.IsActive()
	if err != nil {
		return fmt.Errorf("failed to check network state: %v", err)
	}

	// Destroy (stop) network if it's active
	if active {
		if err := network.Destroy(); err != nil {
			return fmt.Errorf("%w: %v", ErrDestroyNetwork, err)
		}
	}

	// Undefine (remove) the network
	if err := network.Undefine(); err != nil {
		return fmt.Errorf("%w: %v", ErrUndefineNetwork, err)
	}

	return nil
}

// GenerateNetworkXML creates libvirt network XML from config
func GenerateNetworkXML(config LibvirtNetworkConfig) (string, error) {
	network := &libvirtxml.Network{
		Name: config.Name,
	}

	switch config.Mode {
	case "bridge":
		if config.BridgeName == "" {
			return "", errors.New("bridge name required for bridge mode")
		}
		network.Forward = &libvirtxml.NetworkForward{
			Mode: "bridge",
		}
		network.Bridge = &libvirtxml.NetworkBridge{
			Name: config.BridgeName,
		}

	case "nat":
		network.Forward = &libvirtxml.NetworkForward{
			Mode: "nat",
		}
		// For NAT mode, let libvirt create its own bridge
		network.Bridge = &libvirtxml.NetworkBridge{
			Name: "",
			STP:  "on",
		}
		// NAT networks need an IP address configuration
		ipAddr := config.IPAddress
		if ipAddr == "" {
			// Default to avoid conflicts with default network (192.168.122.0/24)
			ipAddr = "192.168.150.1"
		}
		netmask := config.Netmask
		if netmask == "" {
			netmask = "255.255.255.0"
		}
		network.IPs = []libvirtxml.NetworkIP{
			{
				Address: ipAddr,
				Netmask: netmask,
			},
		}

	case "isolated":
		// No forward element for isolated networks
		network.Bridge = &libvirtxml.NetworkBridge{
			Name: "",
			STP:  "on",
		}
		// Isolated networks also need an IP address
		ipAddr := config.IPAddress
		if ipAddr == "" {
			ipAddr = "192.168.151.1"
		}
		netmask := config.Netmask
		if netmask == "" {
			netmask = "255.255.255.0"
		}
		network.IPs = []libvirtxml.NetworkIP{
			{
				Address: ipAddr,
				Netmask: netmask,
			},
		}

	default:
		return "", fmt.Errorf("unsupported network mode: %s", config.Mode)
	}

	// Marshal to XML
	xml, err := network.Marshal()
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrMarshalNetworkXML, err)
	}

	return xml, nil
}
