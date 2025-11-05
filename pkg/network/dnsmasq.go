package network

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"text/template"

	"github.com/alexandremahdhaoui/shaper/pkg/execcontext"
)

// Error variables for dnsmasq operations
var (
	ErrInterfaceRequired    = errors.New("interface is required")
	ErrDHCPRangeRequired    = errors.New("DHCP range is required")
	ErrTFTPRootRequired     = errors.New("TFTP root is required")
	ErrBootFilenameRequired = errors.New("boot filename is required")
	ErrConfigPathRequired   = errors.New("config path is required")
	ErrStartDnsmasq         = errors.New("failed to start dnsmasq")
	ErrStopDnsmasq          = errors.New("failed to stop dnsmasq")
	ErrReadPIDFile          = errors.New("failed to read PID file")
	ErrInvalidPID           = errors.New("invalid PID in PID file")
	ErrDnsmasqNotFound      = errors.New("dnsmasq process not found")
)

// DnsmasqConfig contains dnsmasq configuration
type DnsmasqConfig struct {
	Interface    string   // Network interface (e.g., "br-shaper")
	DHCPRange    string   // e.g., "192.168.100.10,192.168.100.250"
	TFTPRoot     string   // TFTP root directory
	BootFilename string   // iPXE boot file (e.g., "undionly.kpxe")
	PIDFile      string   // PID file path
	LeaseFile    string   // DHCP lease file
	LogQueries   bool     // Enable query logging
	LogDHCP      bool     // Enable DHCP logging
	DNSServers   []string // Optional DNS servers (for upstream resolution)
}

const dnsmasqConfTemplate = `# Dnsmasq configuration for shaper E2E testing
# Generated automatically - do not edit manually

# Bind to specific interface
interface={{.Interface}}
bind-interfaces

# DHCP configuration
dhcp-range={{.DHCPRange}},12h
{{if not .DNSServers}}dhcp-option=3{{end}}
{{if not .DNSServers}}dhcp-option=6{{end}}

{{if .DNSServers}}
# DNS servers
{{range .DNSServers}}server={{.}}
{{end}}
{{end}}

# TFTP configuration
enable-tftp
tftp-root={{.TFTPRoot}}
dhcp-boot={{.BootFilename}}

# Logging
{{if .LogQueries}}log-queries{{end}}
{{if .LogDHCP}}log-dhcp{{end}}

# Process management
{{if .PIDFile}}pid-file={{.PIDFile}}{{end}}
{{if .LeaseFile}}dhcp-leasefile={{.LeaseFile}}{{end}}

# Don't read /etc/resolv.conf or /etc/hosts
no-resolv
no-hosts

# Keep dnsmasq in foreground (for better process control)
keep-in-foreground
`

// GenerateConfig generates dnsmasq.conf content
func (c *DnsmasqConfig) GenerateConfig() ([]byte, error) {
	if c.Interface == "" {
		return nil, ErrInterfaceRequired
	}
	if c.DHCPRange == "" {
		return nil, ErrDHCPRangeRequired
	}
	if c.TFTPRoot == "" {
		return nil, ErrTFTPRootRequired
	}
	if c.BootFilename == "" {
		return nil, ErrBootFilenameRequired
	}

	tmpl, err := template.New("dnsmasq").Parse(dnsmasqConfTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, c); err != nil {
		return nil, fmt.Errorf("failed to execute template: %v", err)
	}

	return buf.Bytes(), nil
}

// WriteConfig writes the dnsmasq configuration to a file
func (c *DnsmasqConfig) WriteConfig(path string) error {
	if path == "" {
		return ErrConfigPathRequired
	}

	content, err := c.GenerateConfig()
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}

	// Write config file
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}

	return nil
}

// DnsmasqProcess manages dnsmasq process
type DnsmasqProcess struct {
	cmd        *exec.Cmd
	configPath string
	pidFile    string
}

// Stop stops dnsmasq process
func (d *DnsmasqProcess) Stop() error {
	if d.cmd == nil || d.cmd.Process == nil {
		return nil
	}

	// Try graceful shutdown first
	if err := d.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		// Process might already be dead
		return nil
	}

	// Wait for process to exit (with timeout)
	done := make(chan error, 1)
	go func() {
		done <- d.cmd.Wait()
	}()

	select {
	case <-done:
		// Process exited
		return nil
	case <-d.waitTimeout():
		// Timeout - force kill
		_ = d.cmd.Process.Kill()
		return fmt.Errorf("%w: process did not exit gracefully", ErrStopDnsmasq)
	}
}

// IsRunning checks if dnsmasq is running
func (d *DnsmasqProcess) IsRunning() bool {
	if d.cmd == nil || d.cmd.Process == nil {
		return false
	}

	// Try to signal the process with signal 0 (doesn't actually send signal, just checks if process exists)
	err := d.cmd.Process.Signal(syscall.Signal(0))
	return err == nil
}

// waitTimeout returns a channel that closes after 5 seconds
func (d *DnsmasqProcess) waitTimeout() <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		// Simple timeout implementation
		select {
		case <-ch:
			return
		default:
			// Wait 5 seconds
			cmd := exec.Command("sleep", "5")
			_ = cmd.Run()
			close(ch)
		}
	}()
	return ch
}

// dnsmasqEntry holds a dnsmasq process and its configuration
type dnsmasqEntry struct {
	process *DnsmasqProcess
	config  DnsmasqConfig
}

// DnsmasqManager manages dnsmasq processes
type DnsmasqManager struct {
	execCtx   execcontext.Context
	processes map[string]*dnsmasqEntry
	mu        sync.RWMutex // Protects processes map
}

// NewDnsmasqManager creates a new DnsmasqManager
func NewDnsmasqManager(execCtx execcontext.Context) *DnsmasqManager {
	return &DnsmasqManager{
		execCtx:   execCtx,
		processes: make(map[string]*dnsmasqEntry),
	}
}

// DnsmasqInfo contains information about a dnsmasq process
type DnsmasqInfo struct {
	ID        string
	Config    DnsmasqConfig
	IsRunning bool
	PID       int
}

// Create starts a new dnsmasq process with the given configuration
func (m *DnsmasqManager) Create(ctx context.Context, id string, config DnsmasqConfig) error {
	if id == "" {
		return errors.New("dnsmasq ID is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if process with same ID already exists
	if entry, exists := m.processes[id]; exists {
		if entry.process.IsRunning() {
			return fmt.Errorf("dnsmasq process with ID %s already exists and is running", id)
		}
		// Process exists but is not running, we can reuse the ID
		delete(m.processes, id)
	}

	// Set up config paths based on ID
	configPath := filepath.Join("/tmp", fmt.Sprintf("dnsmasq-%s.conf", id))
	if config.PIDFile == "" {
		config.PIDFile = filepath.Join("/tmp", fmt.Sprintf("dnsmasq-%s.pid", id))
	}
	if config.LeaseFile == "" {
		config.LeaseFile = filepath.Join("/tmp", fmt.Sprintf("dnsmasq-%s.leases", id))
	}

	// Write config file
	if err := config.WriteConfig(configPath); err != nil {
		return err
	}

	// Ensure TFTP root exists
	if err := os.MkdirAll(config.TFTPRoot, 0o755); err != nil {
		return fmt.Errorf("failed to create TFTP root: %v", err)
	}

	// Ensure lease file directory exists
	if config.LeaseFile != "" {
		leaseDir := filepath.Dir(config.LeaseFile)
		if err := os.MkdirAll(leaseDir, 0o755); err != nil {
			return fmt.Errorf("failed to create lease file directory: %v", err)
		}
	}

	// Start dnsmasq
	cmd := exec.Command("dnsmasq", "-C", configPath)
	execcontext.ApplyToCmd(m.execCtx, cmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%w: %v", ErrStartDnsmasq, err)
	}

	// Store process and config
	m.processes[id] = &dnsmasqEntry{
		process: &DnsmasqProcess{
			cmd:        cmd,
			configPath: configPath,
			pidFile:    config.PIDFile,
		},
		config: config,
	}

	return nil
}

// Get retrieves information about a dnsmasq process
// Returns ErrDnsmasqNotFound if the process doesn't exist
func (m *DnsmasqManager) Get(ctx context.Context, id string) (*DnsmasqInfo, error) {
	if id == "" {
		return nil, errors.New("dnsmasq ID is required")
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Look up process
	entry, exists := m.processes[id]
	if !exists {
		return nil, ErrDnsmasqNotFound
	}

	// Get PID and check if running
	pid := 0
	isRunning := false
	if entry.process.cmd != nil && entry.process.cmd.Process != nil {
		pid = entry.process.cmd.Process.Pid
		isRunning = entry.process.IsRunning()
	}

	return &DnsmasqInfo{
		ID:        id,
		Config:    entry.config,
		IsRunning: isRunning,
		PID:       pid,
	}, nil
}

// Delete stops and removes a dnsmasq process
// Idempotent - returns nil if process doesn't exist
func (m *DnsmasqManager) Delete(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("dnsmasq ID is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if process exists
	entry, exists := m.processes[id]
	if !exists {
		// Process doesn't exist, nothing to do
		return nil
	}

	// Stop the process
	if err := entry.process.Stop(); err != nil {
		// Log error but continue with cleanup
		// The process might already be stopped
	}

	// Clean up files
	if entry.process.pidFile != "" {
		_ = os.Remove(entry.process.pidFile)
	}
	if entry.process.configPath != "" {
		_ = os.Remove(entry.process.configPath)
	}

	// Remove from map
	delete(m.processes, id)

	return nil
}
