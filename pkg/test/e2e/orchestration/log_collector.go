package orchestration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e"
)

var (
	// ErrLogCollectionFailed indicates log collection failed
	ErrLogCollectionFailed = errors.New("log collection failed")
	// ErrInvalidArtifactDir indicates artifact directory is invalid
	ErrInvalidArtifactDir = errors.New("invalid artifact directory")
)

// LogCollector manages collection of logs from various test components
type LogCollector struct {
	artifactDir string
}

// LogCollection contains paths to all collected logs
type LogCollection struct {
	DnsmasqLeases   string            // path to dnsmasq.leases file
	DnsmasqConfig   string            // path to dnsmasq.conf file
	ShaperAPILogs   string            // path to shaper-api logs
	KindClusterLogs string            // directory with kind logs
	VMLogs          map[string]string // VM name -> log file path
}

// NewLogCollector creates a new log collector
func NewLogCollector(artifactDir string) (*LogCollector, error) {
	if artifactDir == "" {
		return nil, fmt.Errorf("%w: artifact directory cannot be empty", ErrInvalidArtifactDir)
	}

	// Ensure artifact directory exists
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return nil, fmt.Errorf("%w: failed to create artifact directory: %v", ErrInvalidArtifactDir, err)
	}

	return &LogCollector{
		artifactDir: artifactDir,
	}, nil
}

// CollectAll collects logs from all sources
func (c *LogCollector) CollectAll(ctx context.Context, infra *e2e.ShaperTestEnvironment, vms []*VMInstance) (*LogCollection, error) {
	if infra == nil {
		return nil, fmt.Errorf("%w: infrastructure state is nil", ErrLogCollectionFailed)
	}

	// Create logs subdirectory
	logsDir := filepath.Join(c.artifactDir, "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return nil, fmt.Errorf("%w: failed to create logs directory: %v", ErrLogCollectionFailed, err)
	}

	collection := &LogCollection{
		VMLogs: make(map[string]string),
	}

	var errs []error

	// Collect dnsmasq logs
	if infra.DnsmasqID != "" {
		leasePath, err := c.CollectDnsmasqLogs(ctx, infra.DnsmasqID, infra.TempDirRoot)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to collect dnsmasq logs: %w", err))
		} else {
			collection.DnsmasqLeases = leasePath
			collection.DnsmasqConfig = filepath.Join(logsDir, "dnsmasq.conf")
		}
	}

	// Collect shaper-API logs
	if infra.Kubeconfig != "" {
		apiLogs, err := c.CollectShaperAPILogs(ctx, infra.Kubeconfig, infra.ShaperNamespace)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to collect shaper-api logs: %w", err))
		}
		// Always set the path if returned - error info is written to the file
		if apiLogs != "" {
			collection.ShaperAPILogs = apiLogs
		}
	}

	// Collect kind cluster logs
	if infra.KindCluster != "" {
		kindLogs, err := c.CollectKindLogs(ctx, infra.KindCluster)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to collect kind logs: %w", err))
		}
		// Always set the path if returned - error info is written to the directory
		if kindLogs != "" {
			collection.KindClusterLogs = kindLogs
		}
	}

	// Collect VM logs
	for _, vm := range vms {
		if vm == nil {
			continue
		}
		vmLog, err := c.CollectVMLogs(ctx, vm)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to collect logs for VM %s: %w", vm.Spec.Name, err))
		}
		// Always set the path if returned - error info is written to the file
		if vmLog != "" {
			collection.VMLogs[vm.Spec.Name] = vmLog
		}
	}

	if len(errs) > 0 {
		return collection, errors.Join(errs...)
	}

	return collection, nil
}

// CollectDnsmasqLogs collects dnsmasq lease and config files
func (c *LogCollector) CollectDnsmasqLogs(ctx context.Context, dnsmasqID string, tempDirRoot string) (string, error) {
	logsDir := filepath.Join(c.artifactDir, "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return "", fmt.Errorf("%w: failed to create logs directory: %v", ErrLogCollectionFailed, err)
	}

	// Copy dnsmasq.leases from temp directory
	leasesSrc := filepath.Join(tempDirRoot, "dnsmasq.leases")
	leasesDst := filepath.Join(logsDir, "dnsmasq.leases")

	if err := copyFile(leasesSrc, leasesDst); err != nil {
		// Lease file may not exist yet if no DHCP happened
		// Create empty file to indicate we tried
		if os.IsNotExist(err) {
			_ = os.WriteFile(leasesDst, []byte("# No DHCP leases found\n"), 0o644)
		} else {
			return "", fmt.Errorf("%w: failed to copy dnsmasq leases: %v", ErrLogCollectionFailed, err)
		}
	}

	// Copy dnsmasq config from /tmp/dnsmasq-<id>.conf
	configSrc := filepath.Join("/tmp", fmt.Sprintf("dnsmasq-%s.conf", dnsmasqID))
	configDst := filepath.Join(logsDir, "dnsmasq.conf")

	if err := copyFile(configSrc, configDst); err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("%w: failed to copy dnsmasq config: %v", ErrLogCollectionFailed, err)
		}
		// Config file missing - write a note
		_ = os.WriteFile(configDst, []byte(fmt.Sprintf("# Dnsmasq config not found at %s\n", configSrc)), 0o644)
	}

	return leasesDst, nil
}

// CollectShaperAPILogs collects shaper-api pod logs via kubectl
func (c *LogCollector) CollectShaperAPILogs(ctx context.Context, kubeconfig, namespace string) (string, error) {
	logsDir := filepath.Join(c.artifactDir, "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return "", fmt.Errorf("%w: failed to create logs directory: %v", ErrLogCollectionFailed, err)
	}
	logFile := filepath.Join(logsDir, "shaper-api.log")

	// Use kubectl to get logs from all pods with shaper-api label
	// kubectl --kubeconfig <path> -n <namespace> logs -l app.kubernetes.io/name=shaper-api
	cmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", kubeconfig,
		"-n", namespace,
		"logs",
		"-l", "app.kubernetes.io/name=shaper-api",
		"--tail=-1", // Get all logs
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// If no pods found or other error, save the error message
		errMsg := fmt.Sprintf("# Failed to collect shaper-api logs: %v\n# Output: %s\n", err, string(output))
		_ = os.WriteFile(logFile, []byte(errMsg), 0o644)
		return logFile, fmt.Errorf("%w: kubectl logs failed: %v", ErrLogCollectionFailed, err)
	}

	// Save logs to file
	if err := os.WriteFile(logFile, output, 0o644); err != nil {
		return "", fmt.Errorf("%w: failed to write logs: %v", ErrLogCollectionFailed, err)
	}

	return logFile, nil
}

// CollectVMLogs collects VM serial console logs from libvirt
func (c *LogCollector) CollectVMLogs(ctx context.Context, vm *VMInstance) (string, error) {
	if vm == nil {
		return "", fmt.Errorf("%w: VM instance is nil", ErrLogCollectionFailed)
	}

	logsDir := filepath.Join(c.artifactDir, "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return "", fmt.Errorf("%w: failed to create logs directory: %v", ErrLogCollectionFailed, err)
	}
	logFile := filepath.Join(logsDir, fmt.Sprintf("%s.log", vm.Spec.Name))

	// Try to get VM console log via virsh console --log
	// Note: This requires the VM to have serial console configured
	cmd := exec.CommandContext(ctx, "virsh", "console", vm.Spec.Name, "--force")

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Console may not be available - save a note
		errMsg := fmt.Sprintf("# VM console not available: %v\n# Output: %s\n", err, string(output))
		_ = os.WriteFile(logFile, []byte(errMsg), 0o644)
		return logFile, nil // Don't return error - console logs are optional
	}

	// Save console output
	if err := os.WriteFile(logFile, output, 0o644); err != nil {
		return "", fmt.Errorf("%w: failed to write VM logs: %v", ErrLogCollectionFailed, err)
	}

	return logFile, nil
}

// CollectKindLogs collects kind cluster logs via kind export logs
func (c *LogCollector) CollectKindLogs(ctx context.Context, clusterName string) (string, error) {
	logsDir := filepath.Join(c.artifactDir, "logs", "kind-cluster")

	// Create directory for kind logs
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return "", fmt.Errorf("%w: failed to create kind logs directory: %v", ErrLogCollectionFailed, err)
	}

	// Export kind logs: kind export logs --name <name> <dir>
	cmd := exec.CommandContext(ctx, "kind", "export", "logs",
		"--name", clusterName,
		logsDir,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Save error to a file in the logs directory
		errFile := filepath.Join(logsDir, "export-error.txt")
		errMsg := fmt.Sprintf("# Failed to export kind logs: %v\n# Output: %s\n", err, string(output))
		_ = os.WriteFile(errFile, []byte(errMsg), 0o644)
		return logsDir, fmt.Errorf("%w: kind export logs failed: %v", ErrLogCollectionFailed, err)
	}

	return logsDir, nil
}

// Helper functions

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	// Ensure destination directory exists
	dstDir := filepath.Dir(dst)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("failed to create destination directory: %v", err)
	}

	return os.WriteFile(dst, data, 0o644)
}
