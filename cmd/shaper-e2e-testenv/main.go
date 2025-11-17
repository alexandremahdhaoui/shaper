package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/alexandremahdhaoui/shaper/pkg/test/e2e/forge"
)

// Exit codes
const (
	exitSuccess = 0 // Operation successful
	exitError   = 1 // Command execution error
)

func main() {
	// Create a new flag set for this tool
	fs := flag.NewFlagSet("shaper-e2e-testenv", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: shaper-e2e-testenv [command] [options]

Commands:
  create [--config <path>]  Create a new test environment
  get <test-id>             Get information about a test environment
  list                      List all test environments
  delete <test-id>          Delete a test environment

Options:
  --config <path>           Path to config JSON file (for create command)
  --format <json|text>      Output format (for get/list commands, default: text)

Environment Variables:
  SHAPER_E2E_STORE_DIR      Test environment store directory (default: ~/.shaper/e2e/testenv)
  SHAPER_E2E_ARTIFACTS_DIR  Artifact storage directory (default: ~/.shaper/e2e/artifacts)

Examples:
  # Create test environment
  shaper-e2e-testenv create

  # Create with custom config
  shaper-e2e-testenv create --config config.json

  # Get environment details
  shaper-e2e-testenv get e2e-123

  # List all environments
  shaper-e2e-testenv list

  # Delete environment
  shaper-e2e-testenv delete e2e-123
`)
	}

	if len(os.Args) < 2 {
		fs.Usage()
		os.Exit(exitError)
	}

	command := os.Args[1]

	switch command {
	case "create":
		cmdCreate(fs, os.Args[2:])
	case "get":
		cmdGet(fs, os.Args[2:])
	case "list":
		cmdList(fs, os.Args[2:])
	case "delete":
		cmdDelete(fs, os.Args[2:])
	case "-h", "--help", "help":
		fs.Usage()
		os.Exit(exitSuccess)
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command '%s'\n", command)
		fs.Usage()
		os.Exit(exitError)
	}
}

// getStoreDir returns the test environment store directory
func getStoreDir() string {
	if dir := os.Getenv("SHAPER_E2E_STORE_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(os.ExpandEnv("$HOME"), ".shaper", "e2e", "testenv")
}

// getArtifactDir returns the artifact storage directory
func getArtifactDir() string {
	if dir := os.Getenv("SHAPER_E2E_ARTIFACTS_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(os.ExpandEnv("$HOME"), ".shaper", "e2e", "artifacts")
}

// cmdCreate creates a new test environment
func cmdCreate(fs *flag.FlagSet, args []string) {
	var configPath string
	fs.StringVar(&configPath, "config", "", "Path to config JSON file")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(exitError)
	}

	// Load or create default config
	config := getDefaultConfig()
	if configPath != "" {
		loadedConfig, err := loadConfigFromFile(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to load config: %v\n", err)
			os.Exit(exitError)
		}
		config = loadedConfig
	}

	// Create testenv manager
	storeDir := getStoreDir()
	artifactDir := getArtifactDir()
	testenv, err := forge.NewTestenv(storeDir, artifactDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create testenv: %v\n", err)
		os.Exit(exitError)
	}

	// Create test environment
	fmt.Fprintf(os.Stderr, "Creating test environment...\n")
	ctx := context.Background()
	id, err := testenv.Create(ctx, config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create test environment: %v\n", err)
		os.Exit(exitError)
	}

	// Print ID to stdout (for forge integration)
	fmt.Println(id)

	// Print summary to stderr
	fmt.Fprintf(os.Stderr, "\n✅ Test environment created: %s\n", id)
	fmt.Fprintf(os.Stderr, "   Store: %s\n", storeDir)
	fmt.Fprintf(os.Stderr, "   Artifacts: %s\n", artifactDir)
	fmt.Fprintf(os.Stderr, "\nNext: Get details with:\n")
	fmt.Fprintf(os.Stderr, "   shaper-e2e-testenv get %s\n", id)
}

// cmdGet retrieves test environment details
func cmdGet(fs *flag.FlagSet, args []string) {
	var format string
	fs.StringVar(&format, "format", "text", "Output format (json|text)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(exitError)
	}

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Error: 'get' requires a test ID\n")
		fmt.Fprintf(os.Stderr, "Usage: shaper-e2e-testenv get <test-id> [--format json|text]\n")
		os.Exit(exitError)
	}

	testID := fs.Arg(0)

	// Create testenv manager
	storeDir := getStoreDir()
	artifactDir := getArtifactDir()
	testenv, err := forge.NewTestenv(storeDir, artifactDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create testenv: %v\n", err)
		os.Exit(exitError)
	}

	// Get environment details
	ctx := context.Background()
	details, err := testenv.Get(ctx, testID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to get test environment: %v\n", err)
		os.Exit(exitError)
	}

	// Output based on format
	if format == "json" {
		data, err := json.MarshalIndent(details, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to marshal JSON: %v\n", err)
			os.Exit(exitError)
		}
		fmt.Println(string(data))
	} else {
		// Human-readable text format
		fmt.Printf("\n=== Test Environment: %s ===\n\n", details["id"])
		fmt.Printf("ID:              %s\n", details["id"])
		fmt.Printf("Bridge:          %s\n", details["bridgeName"])
		fmt.Printf("Libvirt Network: %s\n", details["libvirtNetwork"])
		fmt.Printf("Dnsmasq ID:      %s\n", details["dnsmasqID"])
		fmt.Printf("KIND Cluster:    %s\n", details["kindCluster"])
		fmt.Printf("Kubeconfig:      %s\n", details["kubeconfig"])
		fmt.Printf("TFTP Root:       %s\n", details["tftpRoot"])
		fmt.Printf("Artifact Dir:    %s\n", details["artifactDir"])
		fmt.Printf("Created At:      %s\n", details["createdAt"])
		fmt.Printf("\n")
	}
}

// cmdList lists all test environments
func cmdList(fs *flag.FlagSet, args []string) {
	var format string
	fs.StringVar(&format, "format", "text", "Output format (json|text)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(exitError)
	}

	// Create testenv manager
	storeDir := getStoreDir()
	artifactDir := getArtifactDir()
	testenv, err := forge.NewTestenv(storeDir, artifactDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create testenv: %v\n", err)
		os.Exit(exitError)
	}

	// List environments
	ctx := context.Background()
	environments, err := testenv.List(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to list test environments: %v\n", err)
		os.Exit(exitError)
	}

	// Output based on format
	if format == "json" {
		data, err := json.MarshalIndent(environments, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to marshal JSON: %v\n", err)
			os.Exit(exitError)
		}
		fmt.Println(string(data))
	} else {
		// Human-readable table format
		if len(environments) == 0 {
			fmt.Println("No test environments found")
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tKIND CLUSTER\tCREATED AT")
		fmt.Fprintln(w, "--\t--\t--")

		for _, env := range environments {
			kindCluster := env["kindCluster"]
			if kindCluster == nil || kindCluster == "" {
				kindCluster = "(none)"
			}
			createdAt := env["createdAt"]
			if createdAt == nil || createdAt == "" {
				createdAt = "(unknown)"
			}

			fmt.Fprintf(w, "%s\t%s\t%s\n",
				env["id"],
				kindCluster,
				createdAt,
			)
		}

		w.Flush()
	}
}

// cmdDelete deletes a test environment
func cmdDelete(fs *flag.FlagSet, args []string) {
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(exitError)
	}

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Error: 'delete' requires a test ID\n")
		fmt.Fprintf(os.Stderr, "Usage: shaper-e2e-testenv delete <test-id>\n")
		os.Exit(exitError)
	}

	testID := fs.Arg(0)

	// Create testenv manager
	storeDir := getStoreDir()
	artifactDir := getArtifactDir()
	testenv, err := forge.NewTestenv(storeDir, artifactDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create testenv: %v\n", err)
		os.Exit(exitError)
	}

	// Delete environment
	fmt.Fprintf(os.Stderr, "Deleting test environment: %s\n", testID)
	ctx := context.Background()
	if err := testenv.Delete(ctx, testID); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to delete test environment: %v\n", err)
		fmt.Fprintf(os.Stderr, "Please review errors and retry if necessary\n")
		os.Exit(exitError)
	}

	fmt.Fprintf(os.Stderr, "\n✅ Test environment deleted: %s\n", testID)
}

// getDefaultConfig returns default configuration for test environment
func getDefaultConfig() map[string]interface{} {
	return map[string]interface{}{
		"network": map[string]interface{}{
			"cidr":      "192.168.100.1/24",
			"bridge":    "br-shaper-e2e",
			"dhcpRange": "192.168.100.10,192.168.100.250",
		},
		"kind": map[string]interface{}{
			"clusterName": "shaper-e2e",
			"version":     "", // Use KIND default
		},
		"shaper": map[string]interface{}{
			"namespace":   "default",
			"apiReplicas": 1,
		},
	}
}

// loadConfigFromFile loads configuration from a JSON file
func loadConfigFromFile(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config JSON: %w", err)
	}

	return config, nil
}
