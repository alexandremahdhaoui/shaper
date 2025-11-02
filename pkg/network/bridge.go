package network

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/alexandremahdhaoui/shaper/pkg/execcontext"
)

// Error variables for bridge operations
var (
	ErrBridgeNameRequired = errors.New("bridge name is required")
	ErrCIDRRequired       = errors.New("CIDR is required")
	ErrCreateBridge       = errors.New("failed to create bridge")
	ErrAddBridgeIP        = errors.New("failed to add IP address to bridge")
	ErrBringBridgeUp      = errors.New("failed to bring bridge up")
	ErrDeleteBridge       = errors.New("failed to delete bridge")
	ErrCheckBridgeExists  = errors.New("failed to check if bridge exists")
	ErrBridgeNotFound     = errors.New("bridge not found")
)

// BridgeConfig contains network bridge configuration
type BridgeConfig struct {
	Name string // e.g., "br-shaper"
	CIDR string // e.g., "192.168.100.1/24"
}

// BridgeManager manages Linux network bridges
type BridgeManager struct {
	execCtx execcontext.Context
}

// NewBridgeManager creates a new BridgeManager
func NewBridgeManager(execCtx execcontext.Context) *BridgeManager {
	return &BridgeManager{
		execCtx: execCtx,
	}
}

// BridgeInfo contains information about a bridge
type BridgeInfo struct {
	Name string
	CIDR string
	IsUp bool
}

// Create creates a new bridge with the given configuration
// Idempotent - if bridge exists, ensures it has the correct IP
func (m *BridgeManager) Create(ctx context.Context, config BridgeConfig) error {
	if config.Name == "" {
		return ErrBridgeNameRequired
	}
	if config.CIDR == "" {
		return ErrCIDRRequired
	}

	// Check if bridge already exists
	info, err := m.Get(ctx, config.Name)
	if err != nil && !errors.Is(err, ErrBridgeNotFound) {
		// An error other than "not found" occurred
		return err
	}
	if info != nil {
		// Bridge already exists, just ensure it has the right IP
		return m.ensureBridgeIP(config.Name, config.CIDR)
	}

	// Create bridge: ip link add name <name> type bridge
	cmd := exec.Command("ip", "link", "add", "name", config.Name, "type", "bridge")
	execcontext.ApplyToCmd(m.execCtx, cmd)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %v, output: %s", ErrCreateBridge, err, string(output))
	}

	// Add IP address: ip addr add <cidr> dev <name>
	cmd = exec.Command("ip", "addr", "add", config.CIDR, "dev", config.Name)
	execcontext.ApplyToCmd(m.execCtx, cmd)
	if output, err := cmd.CombinedOutput(); err != nil {
		// Try to cleanup the bridge we just created
		_ = m.deleteBridge(config.Name)
		return fmt.Errorf("%w: %v, output: %s", ErrAddBridgeIP, err, string(output))
	}

	// Bring bridge up: ip link set <name> up
	cmd = exec.Command("ip", "link", "set", config.Name, "up")
	execcontext.ApplyToCmd(m.execCtx, cmd)
	if output, err := cmd.CombinedOutput(); err != nil {
		// Try to cleanup
		_ = m.deleteBridge(config.Name)
		return fmt.Errorf("%w: %v, output: %s", ErrBringBridgeUp, err, string(output))
	}

	return nil
}

// Get retrieves information about a bridge
// Returns ErrBridgeNotFound if the bridge doesn't exist
func (m *BridgeManager) Get(ctx context.Context, name string) (*BridgeInfo, error) {
	if name == "" {
		return nil, ErrBridgeNameRequired
	}

	// Check if bridge exists and get details: ip -d link show <name>
	cmd := exec.Command("ip", "-d", "link", "show", name)
	execcontext.ApplyToCmd(m.execCtx, cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if error is because device doesn't exist
		if strings.Contains(string(output), "does not exist") ||
			strings.Contains(err.Error(), "does not exist") {
			return nil, ErrBridgeNotFound
		}
		// Some other error occurred
		return nil, fmt.Errorf("%w: %v, output: %s", ErrCheckBridgeExists, err, string(output))
	}

	// Verify it's actually a bridge
	if !strings.Contains(string(output), "bridge") {
		return nil, ErrBridgeNotFound
	}

	// Determine if bridge is up
	isUp := strings.Contains(string(output), "state UP")

	// Get IP address information: ip addr show dev <name>
	cmd = exec.Command("ip", "addr", "show", "dev", name)
	execcontext.ApplyToCmd(m.execCtx, cmd)
	addrOutput, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get bridge IP: %v", err)
	}

	// Extract CIDR from "inet <ip>/<prefix>" line
	cidr := ""
	lines := strings.Split(string(addrOutput), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "inet ") {
			// Line format: "inet 192.168.100.1/24 brd ..."
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				cidr = parts[1]
				break
			}
		}
	}

	return &BridgeInfo{
		Name: name,
		CIDR: cidr,
		IsUp: isUp,
	}, nil
}

// Delete removes a bridge
// Idempotent - returns nil if bridge doesn't exist
func (m *BridgeManager) Delete(ctx context.Context, name string) error {
	if name == "" {
		return ErrBridgeNameRequired
	}

	// Check if bridge exists
	_, err := m.Get(ctx, name)
	if err != nil {
		if errors.Is(err, ErrBridgeNotFound) {
			// Bridge doesn't exist, nothing to do
			return nil
		}
		// Some other error occurred during Get
		return err
	}

	// Bridge exists, proceed with deletion
	return m.deleteBridge(name)
}

// deleteBridge performs the actual deletion without existence check
func (m *BridgeManager) deleteBridge(name string) error {
	// Bring bridge down first: ip link set <name> down
	cmd := exec.Command("ip", "link", "set", name, "down")
	execcontext.ApplyToCmd(m.execCtx, cmd)
	_ = cmd.Run() // Ignore errors, bridge might already be down

	// Delete bridge: ip link delete <name> type bridge
	cmd = exec.Command("ip", "link", "delete", name, "type", "bridge")
	execcontext.ApplyToCmd(m.execCtx, cmd)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %v, output: %s", ErrDeleteBridge, err, string(output))
	}

	return nil
}

// ensureBridgeIP ensures the bridge has the correct IP address
func (m *BridgeManager) ensureBridgeIP(name, cidr string) error {
	// Check if IP already assigned
	cmd := exec.Command("ip", "addr", "show", "dev", name)
	execcontext.ApplyToCmd(m.execCtx, cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to check bridge IP: %v", err)
	}

	// If CIDR already present, we're good
	if strings.Contains(string(output), cidr) {
		// Ensure bridge is up
		cmd = exec.Command("ip", "link", "set", name, "up")
		execcontext.ApplyToCmd(m.execCtx, cmd)
		_ = cmd.Run()
		return nil
	}

	// Add the IP address
	cmd = exec.Command("ip", "addr", "add", cidr, "dev", name)
	execcontext.ApplyToCmd(m.execCtx, cmd)
	if output, err := cmd.CombinedOutput(); err != nil {
		// If error is "File exists", the IP is already there with different CIDR
		if !strings.Contains(string(output), "File exists") {
			return fmt.Errorf("%w: %v, output: %s", ErrAddBridgeIP, err, string(output))
		}
	}

	// Bring bridge up
	cmd = exec.Command("ip", "link", "set", name, "up")
	execcontext.ApplyToCmd(m.execCtx, cmd)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %v, output: %s", ErrBringBridgeUp, err, string(output))
	}

	return nil
}
