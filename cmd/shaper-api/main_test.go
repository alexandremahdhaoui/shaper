//go:build unit

/*
Copyright 2024 Alexandre Mahdhaoui

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package main_test provides unit tests for shaper-api configuration and constants.
//
// This test file verifies:
// - Exported constants (Name, ConfigPathEnvKey)
// - This demonstrates proper black-box testing using package main_test
//
// Note: Kubernetes client creation (NewKubeRestConfig, NewKubeClient) is tested
// in internal/k8s package tests, not here, following the DRY principle.
package main_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	main "github.com/alexandremahdhaoui/shaper/cmd/shaper-api"
)

// TestConstants verifies the exported constant values
func TestConstants(t *testing.T) {
	assert.Equal(t, "shaper-api", main.Name)
	assert.Equal(t, "IPXER_CONFIG_PATH", main.ConfigPathEnvKey)
}
