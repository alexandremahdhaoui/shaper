package e2e

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// LogCollector collects logs from various sources during test execution
type LogCollector struct {
	artifactDir string
	testID      string
}

// NewLogCollector creates a new log collector
func NewLogCollector(artifactDir, testID string) *LogCollector {
	return &LogCollector{
		artifactDir: artifactDir,
		testID:      testID,
	}
}

// CollectLogs gathers all logs from the test environment
func (lc *LogCollector) CollectLogs(ctx context.Context, env *ShaperTestEnvironment) (*LogCollection, error) {
	collection := &LogCollection{
		VMConsoleLogs: make(map[string]string),
		VMSerialLogs:  make(map[string]string),
	}

	// Collect framework logs (if they exist)
	frameworkLogPath := filepath.Join(lc.artifactDir, lc.testID, "framework.log")
	if data, err := os.ReadFile(frameworkLogPath); err == nil {
		collection.FrameworkLog = string(data)
	}

	// Collect dnsmasq logs (if dnsmasq is running)
	if env.DnsmasqID != "" {
		dnsmasqLogPath := filepath.Join(env.TempDirRoot, "dnsmasq.log")
		if data, err := os.ReadFile(dnsmasqLogPath); err == nil {
			collection.DnsmasqLog = string(data)
		}
	}

	// Collect shaper-API logs from kubectl
	if env.Kubeconfig != "" {
		apiLogs, err := lc.collectShaperAPILogs(ctx, env.Kubeconfig, env.ShaperNamespace)
		if err == nil {
			collection.ShaperAPILog = apiLogs
		}
	}

	// Collect kubectl logs
	kubectlLogPath := filepath.Join(lc.artifactDir, lc.testID, "kubectl.log")
	if data, err := os.ReadFile(kubectlLogPath); err == nil {
		collection.KubectlLog = string(data)
	}

	// Collect VM console/serial logs
	for _, vm := range env.ClientVMs {
		// Console logs
		consoleLogPath := filepath.Join(lc.artifactDir, lc.testID, fmt.Sprintf("vm-%s-console.log", vm.Name))
		if data, err := os.ReadFile(consoleLogPath); err == nil {
			collection.VMConsoleLogs[vm.Name] = string(data)
		}

		// Serial logs
		serialLogPath := filepath.Join(lc.artifactDir, lc.testID, fmt.Sprintf("vm-%s-serial.log", vm.Name))
		if data, err := os.ReadFile(serialLogPath); err == nil {
			collection.VMSerialLogs[vm.Name] = string(data)
		}
	}

	return collection, nil
}

// collectShaperAPILogs collects logs from shaper-API pods
func (lc *LogCollector) collectShaperAPILogs(ctx context.Context, kubeconfig, namespace string) (string, error) {
	// Get shaper-api pod name
	getPodCmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", kubeconfig,
		"-n", namespace,
		"get", "pods",
		"-l", "app=shaper-api",
		"-o", "jsonpath={.items[0].metadata.name}",
	)

	podName, err := getPodCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get shaper-api pod name: %w", err)
	}

	if len(podName) == 0 {
		return "", fmt.Errorf("no shaper-api pod found")
	}

	// Get logs from pod
	getLogsCmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", kubeconfig,
		"-n", namespace,
		"logs", string(podName),
		"--tail", "1000",
	)

	logs, err := getLogsCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get shaper-api logs: %w", err)
	}

	return string(logs), nil
}

// WriteLogs writes collected logs to disk
func (lc *LogCollector) WriteLogs(collection *LogCollection) error {
	logDir := filepath.Join(lc.artifactDir, lc.testID)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Write framework log
	if collection.FrameworkLog != "" {
		if err := writeLogFile(filepath.Join(logDir, "framework.log"), collection.FrameworkLog); err != nil {
			return err
		}
	}

	// Write dnsmasq log
	if collection.DnsmasqLog != "" {
		if err := writeLogFile(filepath.Join(logDir, "dnsmasq.log"), collection.DnsmasqLog); err != nil {
			return err
		}
	}

	// Write shaper-API log
	if collection.ShaperAPILog != "" {
		if err := writeLogFile(filepath.Join(logDir, "shaper-api.log"), collection.ShaperAPILog); err != nil {
			return err
		}
	}

	// Write kubectl log
	if collection.KubectlLog != "" {
		if err := writeLogFile(filepath.Join(logDir, "kubectl.log"), collection.KubectlLog); err != nil {
			return err
		}
	}

	// Write VM console logs
	for vmName, consoleLog := range collection.VMConsoleLogs {
		if err := writeLogFile(filepath.Join(logDir, fmt.Sprintf("vm-%s-console.log", vmName)), consoleLog); err != nil {
			return err
		}
	}

	// Write VM serial logs
	for vmName, serialLog := range collection.VMSerialLogs {
		if err := writeLogFile(filepath.Join(logDir, fmt.Sprintf("vm-%s-serial.log", vmName)), serialLog); err != nil {
			return err
		}
	}

	return nil
}

// TailLog tails a log file and calls the callback for each line
func (lc *LogCollector) TailLog(ctx context.Context, logPath string, callback func(string)) error {
	file, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			callback(scanner.Text())
		}
	}

	return scanner.Err()
}

// writeLogFile writes a log string to a file
func writeLogFile(path, content string) error {
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write log file %s: %w", path, err)
	}
	return nil
}
