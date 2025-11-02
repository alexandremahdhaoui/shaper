package network

import (
	"errors"
	"fmt"

	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

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
)

// LibvirtNetworkConfig contains libvirt network configuration
type LibvirtNetworkConfig struct {
	Name       string
	BridgeName string // Linux bridge to attach to
	Mode       string // "bridge", "nat", "isolated"
}

// CreateLibvirtNetwork creates a libvirt network
// For bridge mode, it uses an existing Linux bridge
// For nat/isolated modes, libvirt manages the network
func CreateLibvirtNetwork(conn *libvirt.Connect, config LibvirtNetworkConfig) error {
	if conn == nil {
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
	exists, err := NetworkExists(conn, config.Name)
	if err != nil {
		return err
	}
	if exists {
		// Network already exists, ensure it's active
		return ensureNetworkActive(conn, config.Name)
	}

	// Generate network XML
	networkXML, err := GenerateNetworkXML(config)
	if err != nil {
		return err
	}

	// Define the network
	network, err := conn.NetworkDefineXML(networkXML)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDefineNetwork, err)
	}
	defer network.Free()

	// Start the network
	if err := network.Create(); err != nil {
		// Try to undefine on failure
		_ = network.Undefine()
		return fmt.Errorf("%w: %v", ErrStartNetwork, err)
	}

	// Set network to autostart
	if err := network.SetAutostart(true); err != nil {
		// Log but don't fail - autostart is not critical
		// We could add logging here if needed
	}

	return nil
}

// DeleteLibvirtNetwork removes a libvirt network
// Idempotent - returns nil if network doesn't exist
func DeleteLibvirtNetwork(conn *libvirt.Connect, name string) error {
	if name == "" {
		return ErrNetworkNameRequired
	}
	if conn == nil {
		return ErrConnNil
	}

	// Check if network exists
	exists, err := NetworkExists(conn, name)
	if err != nil {
		return err
	}
	if !exists {
		// Network doesn't exist, nothing to do
		return nil
	}

	// Look up the network
	network, err := conn.LookupNetworkByName(name)
	if err != nil {
		// Network might have been deleted between exists check and lookup
		return nil
	}
	defer network.Free()

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

// NetworkExists checks if a libvirt network exists
func NetworkExists(conn *libvirt.Connect, name string) (bool, error) {
	if name == "" {
		return false, ErrNetworkNameRequired
	}
	if conn == nil {
		return false, ErrConnNil
	}

	// Try to lookup the network
	network, err := conn.LookupNetworkByName(name)
	if err != nil {
		// Check if error is because network doesn't exist
		libvirtErr, ok := err.(libvirt.Error)
		if ok && libvirtErr.Code == libvirt.ERR_NO_NETWORK {
			return false, nil
		}
		// Some other error
		return false, fmt.Errorf("%w: %v", ErrCheckNetwork, err)
	}
	defer network.Free()

	return true, nil
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
		// Use a unique range to avoid conflicts with default network (192.168.122.0/24)
		network.IPs = []libvirtxml.NetworkIP{
			{
				Address: "192.168.150.1",
				Netmask: "255.255.255.0",
			},
		}

	case "isolated":
		// No forward element for isolated networks
		network.Bridge = &libvirtxml.NetworkBridge{
			Name: "",
			STP:  "on",
		}
		// Isolated networks also need an IP address
		network.IPs = []libvirtxml.NetworkIP{
			{
				Address: "192.168.151.1",
				Netmask: "255.255.255.0",
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

// ensureNetworkActive ensures a network is active (started)
func ensureNetworkActive(conn *libvirt.Connect, name string) error {
	network, err := conn.LookupNetworkByName(name)
	if err != nil {
		return fmt.Errorf("failed to lookup network: %v", err)
	}
	defer network.Free()

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
