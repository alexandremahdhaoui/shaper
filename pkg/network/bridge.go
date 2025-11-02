package network

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

var (
	ErrBridgeNameRequired = errors.New("bridge name is required")
	ErrCIDRRequired       = errors.New("CIDR is required")
	ErrCreateBridge       = errors.New("failed to create bridge")
	ErrAddBridgeIP        = errors.New("failed to add IP address to bridge")
	ErrBringBridgeUp      = errors.New("failed to bring bridge up")
	ErrDeleteBridge       = errors.New("failed to delete bridge")
	ErrCheckBridgeExists  = errors.New("failed to check if bridge exists")
)

// BridgeConfig contains network bridge configuration
type BridgeConfig struct {
	Name string // e.g., "br-shaper"
	CIDR string // e.g., "192.168.100.1/24"
}

// CreateBridge creates a Linux network bridge
// Uses the 'ip' command to create a bridge device
func CreateBridge(config BridgeConfig) error {
	if config.Name == "" {
		return ErrBridgeNameRequired
	}
	if config.CIDR == "" {
		return ErrCIDRRequired
	}

	// Check if bridge already exists
	exists, err := BridgeExists(config.Name)
	if err != nil {
		return err
	}
	if exists {
		// Bridge already exists, just ensure it has the right IP
		return ensureBridgeIP(config.Name, config.CIDR)
	}

	// Create bridge: ip link add name <name> type bridge
	cmd := exec.Command("ip", "link", "add", "name", config.Name, "type", "bridge")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %v, output: %s", ErrCreateBridge, err, string(output))
	}

	// Add IP address: ip addr add <cidr> dev <name>
	cmd = exec.Command("ip", "addr", "add", config.CIDR, "dev", config.Name)
	if output, err := cmd.CombinedOutput(); err != nil {
		// Try to cleanup the bridge we just created
		_ = deleteBridge(config.Name)
		return fmt.Errorf("%w: %v, output: %s", ErrAddBridgeIP, err, string(output))
	}

	// Bring bridge up: ip link set <name> up
	cmd = exec.Command("ip", "link", "set", config.Name, "up")
	if output, err := cmd.CombinedOutput(); err != nil {
		// Try to cleanup
		_ = deleteBridge(config.Name)
		return fmt.Errorf("%w: %v, output: %s", ErrBringBridgeUp, err, string(output))
	}

	return nil
}

// DeleteBridge removes a network bridge
// Idempotent - returns nil if bridge doesn't exist
func DeleteBridge(name string) error {
	if name == "" {
		return ErrBridgeNameRequired
	}

	// Check if bridge exists
	exists, err := BridgeExists(name)
	if err != nil {
		return err
	}
	if !exists {
		// Bridge doesn't exist, nothing to do
		return nil
	}

	return deleteBridge(name)
}

// deleteBridge performs the actual deletion without existence check
func deleteBridge(name string) error {
	// Bring bridge down first: ip link set <name> down
	cmd := exec.Command("ip", "link", "set", name, "down")
	_ = cmd.Run() // Ignore errors, bridge might already be down

	// Delete bridge: ip link delete <name> type bridge
	cmd = exec.Command("ip", "link", "delete", name, "type", "bridge")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %v, output: %s", ErrDeleteBridge, err, string(output))
	}

	return nil
}

// BridgeExists checks if a bridge exists
func BridgeExists(name string) (bool, error) {
	if name == "" {
		return false, ErrBridgeNameRequired
	}

	// Use 'ip -d link show <name>' to check if bridge exists
	// -d flag shows detailed info including device type
	cmd := exec.Command("ip", "-d", "link", "show", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if error is because device doesn't exist
		if strings.Contains(string(output), "does not exist") ||
			strings.Contains(err.Error(), "does not exist") {
			return false, nil
		}
		// Some other error occurred
		return false, fmt.Errorf("%w: %v, output: %s", ErrCheckBridgeExists, err, string(output))
	}

	// Command succeeded, verify it's actually a bridge
	// Detailed output includes "bridge" on the second line
	if strings.Contains(string(output), "bridge") {
		return true, nil
	}

	// Device exists but is not a bridge
	return false, nil
}

// ensureBridgeIP ensures the bridge has the correct IP address
func ensureBridgeIP(name, cidr string) error {
	// Check if IP already assigned
	cmd := exec.Command("ip", "addr", "show", "dev", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to check bridge IP: %v", err)
	}

	// If CIDR already present, we're good
	if strings.Contains(string(output), cidr) {
		// Ensure bridge is up
		cmd = exec.Command("ip", "link", "set", name, "up")
		_ = cmd.Run()
		return nil
	}

	// Add the IP address
	cmd = exec.Command("ip", "addr", "add", cidr, "dev", name)
	if output, err := cmd.CombinedOutput(); err != nil {
		// If error is "File exists", the IP is already there with different CIDR
		if !strings.Contains(string(output), "File exists") {
			return fmt.Errorf("%w: %v, output: %s", ErrAddBridgeIP, err, string(output))
		}
	}

	// Bring bridge up
	cmd = exec.Command("ip", "link", "set", name, "up")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %v, output: %s", ErrBringBridgeUp, err, string(output))
	}

	return nil
}
