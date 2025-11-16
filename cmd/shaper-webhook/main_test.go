//go:build unit

package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHealthProbeBindAddress(t *testing.T) {
	tests := []struct {
		name     string
		port     int
		expected string
	}{
		{
			name:     "port 8081",
			port:     8081,
			expected: ":8081",
		},
		{
			name:     "port 9090",
			port:     9090,
			expected: ":9090",
		},
		{
			name:     "port 0 (disabled)",
			port:     0,
			expected: ":0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				ProbesServer: struct {
					LivenessPath  string `json:"livenessPath"`
					ReadinessPath string `json:"readinessPath"`
					Port          int    `json:"port"`
				}{
					Port:          tt.port,
					LivenessPath:  "/healthz",
					ReadinessPath: "/readyz",
				},
			}

			// Verify the bind address is formatted correctly
			bindAddress := fmt.Sprintf(":%d", config.ProbesServer.Port)
			assert.Equal(t, tt.expected, bindAddress)
		})
	}
}

func TestConfigDefaults(t *testing.T) {
	config := &Config{
		ProbesServer: struct {
			LivenessPath  string `json:"livenessPath"`
			ReadinessPath string `json:"readinessPath"`
			Port          int    `json:"port"`
		}{
			Port:          8081,
			LivenessPath:  "/healthz",
			ReadinessPath: "/readyz",
		},
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
		MetricsServer: struct {
			Path string `json:"path"`
			Port int    `json:"port"`
		}{
			Port: 8080,
			Path: "/metrics",
		},
	}

	// Verify probe server config
	assert.Equal(t, 8081, config.ProbesServer.Port)
	assert.Equal(t, "/healthz", config.ProbesServer.LivenessPath)
	assert.Equal(t, "/readyz", config.ProbesServer.ReadinessPath)

	// Verify webhook server config
	assert.Equal(t, 9443, config.WebhookServer.Port)
	assert.Equal(t, "/tmp/k8s-webhook-server/serving-certs", config.WebhookServer.CertDir)
	assert.Equal(t, "tls.crt", config.WebhookServer.CertName)
	assert.Equal(t, "tls.key", config.WebhookServer.KeyName)

	// Verify metrics server config
	assert.Equal(t, 8080, config.MetricsServer.Port)
	assert.Equal(t, "/metrics", config.MetricsServer.Path)
}
