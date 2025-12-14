//go:build unit

// Copyright 2024 Alexandre Mahdhaoui
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDefaultConfig(t *testing.T) {
	config := NewDefaultConfig()

	assert.Equal(t, ":8080", config.MetricsBind)
	assert.Equal(t, ":8081", config.HealthBind)
	assert.Equal(t, "shaper-controller-leader", config.LeaderElectionID)
	assert.False(t, config.LeaderElection)
	assert.False(t, config.DevelopmentMode)
	assert.Equal(t, "", config.Namespace)
}

func TestLoadConfig_ValidJSON(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	configContent := `{
		"metricsBind": ":9090",
		"healthBind": ":9091",
		"leaderElectionID": "custom-leader",
		"leaderElection": true,
		"developmentMode": true,
		"namespace": "test-namespace"
	}`

	err := os.WriteFile(configPath, []byte(configContent), 0o600)
	require.NoError(t, err)

	// Load config
	config, err := LoadConfig(configPath)
	require.NoError(t, err)
	assert.NotNil(t, config)

	assert.Equal(t, ":9090", config.MetricsBind)
	assert.Equal(t, ":9091", config.HealthBind)
	assert.Equal(t, "custom-leader", config.LeaderElectionID)
	assert.True(t, config.LeaderElection)
	assert.True(t, config.DevelopmentMode)
	assert.Equal(t, "test-namespace", config.Namespace)
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	// Create temporary config file with invalid JSON
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	err := os.WriteFile(configPath, []byte(`{invalid json`), 0o600)
	require.NoError(t, err)

	// Load config should fail
	config, err := LoadConfig(configPath)
	assert.Error(t, err)
	assert.Nil(t, config)
	assert.Contains(t, err.Error(), "parsing config file")
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	// Try to load non-existent file
	config, err := LoadConfig("/nonexistent/path/config.json")
	assert.Error(t, err)
	assert.Nil(t, config)
	assert.Contains(t, err.Error(), "reading config file")
}

func TestLoadConfig_EmptyPath(t *testing.T) {
	// Clear environment variables
	os.Unsetenv("SHAPER_CONTROLLER_METRICS_ADDR")
	os.Unsetenv("SHAPER_CONTROLLER_HEALTH_ADDR")
	os.Unsetenv("SHAPER_CONTROLLER_LEADER_ELECTION_ID")

	// Load config with empty path should use defaults
	config, err := LoadConfig("")
	require.NoError(t, err)
	assert.NotNil(t, config)

	assert.Equal(t, ":8080", config.MetricsBind)
	assert.Equal(t, ":8081", config.HealthBind)
	assert.Equal(t, "shaper-controller-leader", config.LeaderElectionID)
}

func TestLoadConfig_EnvironmentOverrides(t *testing.T) {
	// Set environment variables
	os.Setenv("SHAPER_CONTROLLER_METRICS_ADDR", ":7070")
	os.Setenv("SHAPER_CONTROLLER_HEALTH_ADDR", ":7071")
	os.Setenv("SHAPER_CONTROLLER_LEADER_ELECTION_ID", "env-leader")
	os.Setenv("SHAPER_CONTROLLER_LEADER_ELECTION", "true")
	os.Setenv("SHAPER_CONTROLLER_DEV_MODE", "yes")
	os.Setenv("SHAPER_CONTROLLER_NAMESPACE", "env-namespace")
	defer func() {
		os.Unsetenv("SHAPER_CONTROLLER_METRICS_ADDR")
		os.Unsetenv("SHAPER_CONTROLLER_HEALTH_ADDR")
		os.Unsetenv("SHAPER_CONTROLLER_LEADER_ELECTION_ID")
		os.Unsetenv("SHAPER_CONTROLLER_LEADER_ELECTION")
		os.Unsetenv("SHAPER_CONTROLLER_DEV_MODE")
		os.Unsetenv("SHAPER_CONTROLLER_NAMESPACE")
	}()

	// Load config with empty path (env vars only)
	config, err := LoadConfig("")
	require.NoError(t, err)
	assert.NotNil(t, config)

	assert.Equal(t, ":7070", config.MetricsBind)
	assert.Equal(t, ":7071", config.HealthBind)
	assert.Equal(t, "env-leader", config.LeaderElectionID)
	assert.True(t, config.LeaderElection)
	assert.True(t, config.DevelopmentMode)
	assert.Equal(t, "env-namespace", config.Namespace)
}

func TestLoadConfig_EnvironmentOverridesJSON(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	configContent := `{
		"metricsBind": ":9090",
		"healthBind": ":9091"
	}`

	err := os.WriteFile(configPath, []byte(configContent), 0o600)
	require.NoError(t, err)

	// Set environment variables that should override JSON
	os.Setenv("SHAPER_CONTROLLER_METRICS_ADDR", ":7070")
	defer os.Unsetenv("SHAPER_CONTROLLER_METRICS_ADDR")

	// Load config - env should override JSON
	config, err := LoadConfig(configPath)
	require.NoError(t, err)
	assert.NotNil(t, config)

	assert.Equal(t, ":7070", config.MetricsBind) // From env
	assert.Equal(t, ":9091", config.HealthBind)  // From JSON
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		wantError bool
		errorMsg  string
	}{
		{
			name:      "valid config",
			config:    NewDefaultConfig(),
			wantError: false,
		},
		{
			name: "empty metricsBind",
			config: &Config{
				MetricsBind:      "",
				HealthBind:       ":8081",
				LeaderElectionID: "leader",
			},
			wantError: true,
			errorMsg:  "metricsBind cannot be empty",
		},
		{
			name: "empty healthBind",
			config: &Config{
				MetricsBind:      ":8080",
				HealthBind:       "",
				LeaderElectionID: "leader",
			},
			wantError: true,
			errorMsg:  "healthBind cannot be empty",
		},
		{
			name: "empty leaderElectionID",
			config: &Config{
				MetricsBind:      ":8080",
				HealthBind:       ":8081",
				LeaderElectionID: "",
			},
			wantError: true,
			errorMsg:  "leaderElectionID cannot be empty",
		},
		{
			name: "multiple validation errors",
			config: &Config{
				MetricsBind:      "",
				HealthBind:       "",
				LeaderElectionID: "",
			},
			wantError: true,
			errorMsg:  "metricsBind cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.wantError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
