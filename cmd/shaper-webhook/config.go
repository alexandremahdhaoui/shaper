package main

import (
	"context"
	"fmt"
	"os"

	"sigs.k8s.io/yaml"
)

const (
	// ConfigPathEnvKey is the environment variable key for the config file path.
	ConfigPathEnvKey = "SHAPER_WEBHOOK_CONFIG_PATH"
)

// loadConfig loads the configuration from the file specified in the
// SHAPER_WEBHOOK_CONFIG_PATH environment variable.
func loadConfig(ctx context.Context) (*Config, error) {
	// Get config path from environment variable
	configPath := os.Getenv(ConfigPathEnvKey)
	if configPath == "" {
		return nil, fmt.Errorf("environment variable %q must be set", ConfigPathEnvKey)
	}

	// Read file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	// Parse YAML (uses json tags)
	config := &Config{}
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	return config, nil
}

// Config is used to configure the webhook application.
//
// Some part of the configuration may be passed through environment variables.
type Config struct {
	// Adapters

	// AssignmentNamespace is the namespace where the Assignment resources are located.
	AssignmentNamespace string `json:"assignmentNamespace"`
	// ProfileNamespace is the namespace where the Profile resources are located.
	ProfileNamespace string `json:"profileNamespace"`

	// Kubeconfig

	// KubeconfigPath is the path to the kubeconfig file.
	//
	// It can be set to "in-cluster" to use the in-cluster config.
	KubeconfigPath string `json:"kubeconfigPath"`

	// WebhookServer is the configuration for the webhook server.
	WebhookServer struct {
		// Port is the port for the webhook server.
		Port int `json:"port"`
		// CertDir is the directory where TLS certificates are mounted.
		CertDir string `json:"certDir"`
		// CertName is the name of the TLS certificate file.
		CertName string `json:"certName"`
		// KeyName is the name of the TLS private key file.
		KeyName string `json:"keyName"`
	} `json:"webhookServer"`

	// ProbesServer is the configuration for the probes server.
	ProbesServer struct {
		// LivenessPath is the path for the liveness probe.
		LivenessPath string `json:"livenessPath"`
		// ReadinessPath is the path for the readiness probe.
		ReadinessPath string `json:"readinessPath"`
		// Port is the port for the probes server.
		Port int `json:"port"`
	} `json:"probesServer"`

	// MetricsServer is the configuration for the metrics server.
	MetricsServer struct {
		// Path is the path for the metrics server.
		Path string `json:"path"`
		// Port is the port for the metrics server.
		Port int `json:"port"`
	} `json:"metricsServer"`
}
