package network

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"text/template"
)

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
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}

	// Write config file
	if err := os.WriteFile(path, content, 0644); err != nil {
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

// StartDnsmasq starts dnsmasq with given config
func StartDnsmasq(config DnsmasqConfig, configPath string) (*DnsmasqProcess, error) {
	// Write config file
	if err := config.WriteConfig(configPath); err != nil {
		return nil, err
	}

	// Ensure TFTP root exists
	if err := os.MkdirAll(config.TFTPRoot, 0755); err != nil {
		return nil, fmt.Errorf("failed to create TFTP root: %v", err)
	}

	// Ensure lease file directory exists
	if config.LeaseFile != "" {
		leaseDir := filepath.Dir(config.LeaseFile)
		if err := os.MkdirAll(leaseDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create lease file directory: %v", err)
		}
	}

	// Start dnsmasq
	cmd := exec.Command("dnsmasq", "-C", configPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStartDnsmasq, err)
	}

	return &DnsmasqProcess{
		cmd:        cmd,
		configPath: configPath,
		pidFile:    config.PIDFile,
	}, nil
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

// StopByPIDFile stops dnsmasq using PID from PID file
func StopByPIDFile(pidFile string) error {
	if pidFile == "" {
		return fmt.Errorf("PID file path is required")
	}

	// Read PID from file
	pidBytes, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			// PID file doesn't exist, dnsmasq probably not running
			return nil
		}
		return fmt.Errorf("%w: %v", ErrReadPIDFile, err)
	}

	pidStr := strings.TrimSpace(string(pidBytes))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidPID, pidStr)
	}

	// Find process
	process, err := os.FindProcess(pid)
	if err != nil {
		// Process doesn't exist
		return nil
	}

	// Send SIGTERM
	if err := process.Signal(syscall.SIGTERM); err != nil {
		// Process might already be dead
		return nil
	}

	// Clean up PID file
	_ = os.Remove(pidFile)

	return nil
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
