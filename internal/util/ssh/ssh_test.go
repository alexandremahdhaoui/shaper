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

//go:build unit

package ssh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alexandremahdhaoui/shaper/internal/util/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewClient_Success verifies NewClient() successfully reads a private key file and creates a client.
func TestNewClient_Success(t *testing.T) {
	// Create temporary directory
	tempDir := t.TempDir()

	// Use a test SSH private key (minimal valid key for testing)
	testPrivateKey := `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACA1OsJHLLbj6LWJ/f3V3Vql7M0q+UHQZ7yVqUb7YQxtcgAAAJj5pK1S+aSt
UgAAAAtzc2gtZWQyNTUxOQAAACA1OsJHLLbj6LWJ/f3V3Vql7M0q+UHQZ7yVqUb7YQxtcg
AAAED0mFPqGHb8AyNEf5T5FI7j9r8z0R2+3i5d1G5wK0v8pTU6wkcstuPotYn9/dXdWqXs
zSr5QdBnvJWpRvthDG1yAAAAE3Rlc3RAZXhhbXBsZS5sb2NhbAECAw==
-----END OPENSSH PRIVATE KEY-----`

	// Write key to temp file
	keyPath := filepath.Join(tempDir, "id_rsa")
	err := os.WriteFile(keyPath, []byte(testPrivateKey), 0600)
	require.NoError(t, err)

	// Create SSH client
	client, err := ssh.NewClient("test-host", "test-user", keyPath, "22")
	require.NoError(t, err, "NewClient should not return error")
	require.NotNil(t, client, "Client should not be nil")

	// Verify client fields
	assert.Equal(t, "test-host", client.Host)
	assert.Equal(t, "test-user", client.User)
	assert.Equal(t, "22", client.Port)
	assert.NotEmpty(t, client.PrivateKey, "PrivateKey should contain key bytes")
}

// TestNewClient_FileNotFound verifies NewClient() returns error when private key file doesn't exist.
func TestNewClient_FileNotFound(t *testing.T) {
	// Try to create client with nonexistent key file
	client, err := ssh.NewClient("test-host", "test-user", "/nonexistent/path/id_rsa", "22")

	// Verify error
	assert.Error(t, err, "Should return error for nonexistent file")
	assert.Nil(t, client, "Client should be nil on error")
	assert.Contains(t, err.Error(), "unable to read private key", "Error message should mention private key")
}

// TestClient_Run is not implemented as a unit test because:
// 1. Run() requires a real SSH server to connect to
// 2. Run() uses network operations (ssh.Dial) that need real endpoints
// 3. Run() parses SSH private keys which requires crypto operations with real keys
//
// This method should be tested via:
// - Integration tests with a test SSH server (mark with //go:build integration)
// - The existing VMM integration tests already exercise this (pkg/vmm/vm_test.go)
//
// For unit testing, consider:
// - Testing the Runner interface with a mock implementation
// - See internal/util/fakes/ for examples of test fakes

// TestClient_AwaitServer is not implemented as a unit test because:
// 1. AwaitServer() requires a real SSH server to connect to
// 2. Testing retry logic with timeouts requires either:
//    - A real SSH server that can be started/stopped
//    - Complex mocking of network layers (not practical)
//
// This method should be tested via:
// - Integration tests where you start an SSH server
// - Manual testing of connection retry behavior
//
// The method's logic is straightforward:
// - Retry connection every 5 seconds until timeout
// - Uses the same connection config as Run()
