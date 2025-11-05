package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name           string
		configYAML     string
		expectedConfig *Config
		expectError    bool
	}{
		{
			name: "valid config with all fields",
			configYAML: `
assignmentNamespace: "default"
profileNamespace: "default"
kubeconfigPath: "in-cluster"
webhookServer:
  port: 9443
  certDir: "/tmp/k8s-webhook-server/serving-certs"
  certName: "tls.crt"
  keyName: "tls.key"
probesServer:
  port: 8081
  livenessPath: "/healthz"
  readinessPath: "/readyz"
metricsServer:
  port: 8080
  path: "/metrics"
`,
			expectedConfig: &Config{
				AssignmentNamespace: "default",
				ProfileNamespace:    "default",
				KubeconfigPath:      "in-cluster",
				WebhookServer: struct {
					Port     int    `json:"port"`
					CertDir  string `json:"certDir"`
					CertName string `json:"certName"`
					KeyName  string `json:"keyName"`
				}{
					Port:     9443,
					CertDir:  "/tmp/k8s-webhook-server/serving-certs",
					CertName: "tls.crt",
					KeyName:  "tls.key",
				},
				ProbesServer: struct {
					LivenessPath  string `json:"livenessPath"`
					ReadinessPath string `json:"readinessPath"`
					Port          int    `json:"port"`
				}{
					Port:          8081,
					LivenessPath:  "/healthz",
					ReadinessPath: "/readyz",
				},
				MetricsServer: struct {
					Path string `json:"path"`
					Port int    `json:"port"`
				}{
					Port: 8080,
					Path: "/metrics",
				},
			},
			expectError: false,
		},
		{
			name: "minimal config",
			configYAML: `
assignmentNamespace: "test-ns"
profileNamespace: "test-ns"
kubeconfigPath: "/path/to/kubeconfig"
webhookServer:
  port: 9443
probesServer:
  port: 8081
metricsServer:
  port: 8080
`,
			expectedConfig: &Config{
				AssignmentNamespace: "test-ns",
				ProfileNamespace:    "test-ns",
				KubeconfigPath:      "/path/to/kubeconfig",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary config file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")
			err := os.WriteFile(configPath, []byte(tt.configYAML), 0o644)
			require.NoError(t, err)

			// Set environment variable
			t.Setenv(ConfigPathEnvKey, configPath)

			// Load config
			config, err := loadConfig(nil)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, config)

			// Verify basic fields
			assert.Equal(t, tt.expectedConfig.AssignmentNamespace, config.AssignmentNamespace)
			assert.Equal(t, tt.expectedConfig.ProfileNamespace, config.ProfileNamespace)
			assert.Equal(t, tt.expectedConfig.KubeconfigPath, config.KubeconfigPath)

			// Verify webhook server config
			if tt.expectedConfig.WebhookServer.Port != 0 {
				assert.Equal(t, tt.expectedConfig.WebhookServer.Port, config.WebhookServer.Port, "webhook server port mismatch")
			}

			// Verify probes server config
			if tt.expectedConfig.ProbesServer.Port != 0 {
				assert.Equal(t, tt.expectedConfig.ProbesServer.Port, config.ProbesServer.Port, "probes server port mismatch")
			}

			// Verify metrics server config
			if tt.expectedConfig.MetricsServer.Port != 0 {
				assert.Equal(t, tt.expectedConfig.MetricsServer.Port, config.MetricsServer.Port, "metrics server port mismatch")
			}
		})
	}
}

func TestLoadConfig_MissingEnvVar(t *testing.T) {
	// Unset environment variable
	os.Unsetenv(ConfigPathEnvKey)

	config, err := loadConfig(nil)

	assert.Error(t, err)
	assert.Nil(t, config)
	assert.Contains(t, err.Error(), ConfigPathEnvKey)
}

func TestLoadConfig_NonExistentFile(t *testing.T) {
	t.Setenv(ConfigPathEnvKey, "/non/existent/path/config.yaml")

	config, err := loadConfig(nil)

	assert.Error(t, err)
	assert.Nil(t, config)
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte("invalid: yaml: content: ["), 0o644)
	require.NoError(t, err)

	t.Setenv(ConfigPathEnvKey, configPath)

	config, err := loadConfig(nil)

	assert.Error(t, err)
	assert.Nil(t, config)
}
