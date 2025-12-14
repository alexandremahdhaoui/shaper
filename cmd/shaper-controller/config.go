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
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

const (
	// ConfigPathEnvKey is the environment variable key for the config file path
	ConfigPathEnvKey = "SHAPER_CONTROLLER_CONFIG_PATH"
)

// Config holds the configuration for shaper-controller
type Config struct {
	// MetricsBind is the address for the metrics server (e.g., ":8080")
	MetricsBind string `json:"metricsBind"`

	// HealthBind is the address for the health probe server (e.g., ":8081")
	HealthBind string `json:"healthBind"`

	// LeaderElectionID is the name used for leader election
	LeaderElectionID string `json:"leaderElectionID"`

	// LeaderElection enables or disables leader election
	LeaderElection bool `json:"leaderElection"`

	// DevelopmentMode enables development logging
	DevelopmentMode bool `json:"developmentMode"`

	// Namespace is the namespace to watch (empty means all namespaces)
	Namespace string `json:"namespace,omitempty"`
}

// NewDefaultConfig returns a Config with sensible defaults
func NewDefaultConfig() *Config {
	return &Config{
		MetricsBind:      ":8080",
		HealthBind:       ":8081",
		LeaderElectionID: "shaper-controller-leader",
		LeaderElection:   false,
		DevelopmentMode:  false,
		Namespace:        "", // Watch all namespaces by default
	}
}

// LoadConfig loads configuration from a JSON file path or returns defaults with env var overrides
// If configPath is empty, it uses environment variables only
func LoadConfig(configPath string) (*Config, error) {
	config := NewDefaultConfig()

	// If config path provided, load from file
	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("reading config file %s: %w", configPath, err)
		}

		if err := json.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("parsing config file %s: %w", configPath, err)
		}
	}

	// Override with environment variables (if set)
	config.applyEnvironmentOverrides()

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

// applyEnvironmentOverrides applies environment variable overrides to the config
func (c *Config) applyEnvironmentOverrides() {
	if val := os.Getenv("SHAPER_CONTROLLER_METRICS_ADDR"); val != "" {
		c.MetricsBind = val
	}
	if val := os.Getenv("SHAPER_CONTROLLER_HEALTH_ADDR"); val != "" {
		c.HealthBind = val
	}
	if val := os.Getenv("SHAPER_CONTROLLER_LEADER_ELECTION_ID"); val != "" {
		c.LeaderElectionID = val
	}
	if val := os.Getenv("SHAPER_CONTROLLER_LEADER_ELECTION"); val != "" {
		c.LeaderElection = val == "true" || val == "1" || val == "yes"
	}
	if val := os.Getenv("SHAPER_CONTROLLER_DEV_MODE"); val != "" {
		c.DevelopmentMode = val == "true" || val == "1" || val == "yes"
	}
	if val := os.Getenv("SHAPER_CONTROLLER_NAMESPACE"); val != "" {
		c.Namespace = val
	}
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	var errs []error

	if c.MetricsBind == "" {
		errs = append(errs, errors.New("metricsBind cannot be empty"))
	}

	if c.HealthBind == "" {
		errs = append(errs, errors.New("healthBind cannot be empty"))
	}

	if c.LeaderElectionID == "" {
		errs = append(errs, errors.New("leaderElectionID cannot be empty"))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}
