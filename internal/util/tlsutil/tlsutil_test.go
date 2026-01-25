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

package tlsutil_test

import (
	"crypto/tls"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alexandremahdhaoui/shaper/internal/util/certutil"
	"github.com/alexandremahdhaoui/shaper/internal/util/tlsutil"
)

// TestBuildTLSConfig_Disabled verifies that BuildTLSConfig returns nil when TLS is disabled.
func TestBuildTLSConfig_Disabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config *tlsutil.Config
	}{
		{
			name:   "nil config",
			config: nil,
		},
		{
			name: "enabled is false",
			config: &tlsutil.Config{
				Enabled: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tlsConfig, err := tlsutil.BuildTLSConfig(tt.config)
			assert.NoError(t, err)
			assert.Nil(t, tlsConfig)
		})
	}
}

// TestBuildTLSConfig_CertNotFound verifies error when certificate file is missing.
func TestBuildTLSConfig_CertNotFound(t *testing.T) {
	t.Parallel()

	config := &tlsutil.Config{
		Enabled:  true,
		CertPath: "/nonexistent/path/cert.pem",
		KeyPath:  "/nonexistent/path/key.pem",
	}

	tlsConfig, err := tlsutil.BuildTLSConfig(config)
	assert.Nil(t, tlsConfig)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, tlsutil.ErrCertNotFound))
	assert.Contains(t, err.Error(), "/nonexistent/path/cert.pem")
}

// TestBuildTLSConfig_KeyNotFound verifies error when key file is missing.
func TestBuildTLSConfig_KeyNotFound(t *testing.T) {
	t.Parallel()

	// Create temp dir with only cert file
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")

	// Create a dummy cert file
	err := os.WriteFile(certPath, []byte("dummy"), 0o600)
	require.NoError(t, err)

	config := &tlsutil.Config{
		Enabled:  true,
		CertPath: certPath,
		KeyPath:  "/nonexistent/path/key.pem",
	}

	tlsConfig, err := tlsutil.BuildTLSConfig(config)
	assert.Nil(t, tlsConfig)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, tlsutil.ErrKeyNotFound))
	assert.Contains(t, err.Error(), "/nonexistent/path/key.pem")
}

// TestBuildTLSConfig_CANotFound verifies error when CA file is missing and clientAuth != "none".
func TestBuildTLSConfig_CANotFound(t *testing.T) {
	t.Parallel()

	// Create temp dir with cert and key files
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	// Create dummy files (content doesn't matter for this test - we fail on CA check first)
	err := os.WriteFile(certPath, []byte("dummy"), 0o600)
	require.NoError(t, err)
	err = os.WriteFile(keyPath, []byte("dummy"), 0o600)
	require.NoError(t, err)

	config := &tlsutil.Config{
		Enabled:    true,
		CertPath:   certPath,
		KeyPath:    keyPath,
		CAPath:     "/nonexistent/path/ca.pem",
		ClientAuth: "require",
	}

	tlsConfig, err := tlsutil.BuildTLSConfig(config)
	assert.Nil(t, tlsConfig)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, tlsutil.ErrCANotFound))
	assert.Contains(t, err.Error(), "/nonexistent/path/ca.pem")
}

// TestBuildTLSConfig_InvalidClientAuth verifies error when clientAuth value is invalid.
func TestBuildTLSConfig_InvalidClientAuth(t *testing.T) {
	t.Parallel()

	// Create temp dir with cert and key files
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	// Create dummy files
	err := os.WriteFile(certPath, []byte("dummy"), 0o600)
	require.NoError(t, err)
	err = os.WriteFile(keyPath, []byte("dummy"), 0o600)
	require.NoError(t, err)

	config := &tlsutil.Config{
		Enabled:    true,
		CertPath:   certPath,
		KeyPath:    keyPath,
		ClientAuth: "invalid_value",
	}

	tlsConfig, err := tlsutil.BuildTLSConfig(config)
	assert.Nil(t, tlsConfig)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, tlsutil.ErrInvalidClientAuth))
	assert.Contains(t, err.Error(), "invalid_value")
}

// TestBuildTLSConfig_ClientAuthMapping verifies correct mapping of clientAuth strings.
func TestBuildTLSConfig_ClientAuthMapping(t *testing.T) {
	t.Parallel()

	// Generate real certificates for the test
	ca, err := certutil.NewCA()
	require.NoError(t, err)

	serverKey, serverCert, err := ca.NewCertifiedKeyPEM("localhost")
	require.NoError(t, err)

	tests := []struct {
		name               string
		clientAuth         string
		expectedClientAuth tls.ClientAuthType
		needsCA            bool
	}{
		{
			name:               "empty string defaults to none",
			clientAuth:         "",
			expectedClientAuth: tls.NoClientCert,
			needsCA:            false,
		},
		{
			name:               "none",
			clientAuth:         "none",
			expectedClientAuth: tls.NoClientCert,
			needsCA:            false,
		},
		{
			name:               "request",
			clientAuth:         "request",
			expectedClientAuth: tls.RequestClientCert,
			needsCA:            true,
		},
		{
			name:               "require",
			clientAuth:         "require",
			expectedClientAuth: tls.RequireAndVerifyClientCert,
			needsCA:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Create temp dir with cert and key files
			tmpDir := t.TempDir()
			certPath := filepath.Join(tmpDir, "cert.pem")
			keyPath := filepath.Join(tmpDir, "key.pem")
			caPath := filepath.Join(tmpDir, "ca.pem")

			err := os.WriteFile(certPath, serverCert, 0o600)
			require.NoError(t, err)
			err = os.WriteFile(keyPath, serverKey, 0o600)
			require.NoError(t, err)

			if tt.needsCA {
				err = os.WriteFile(caPath, ca.Cert(), 0o600)
				require.NoError(t, err)
			}

			config := &tlsutil.Config{
				Enabled:    true,
				CertPath:   certPath,
				KeyPath:    keyPath,
				ClientAuth: tt.clientAuth,
			}

			if tt.needsCA {
				config.CAPath = caPath
			}

			tlsConfig, err := tlsutil.BuildTLSConfig(config)
			require.NoError(t, err)
			require.NotNil(t, tlsConfig)
			assert.Equal(t, tt.expectedClientAuth, tlsConfig.ClientAuth)
		})
	}
}

// TestBuildTLSConfig_ValidConfig verifies a valid TLS configuration is built correctly.
func TestBuildTLSConfig_ValidConfig(t *testing.T) {
	t.Parallel()

	// Generate real certificates for the test
	ca, err := certutil.NewCA()
	require.NoError(t, err)

	serverKey, serverCert, err := ca.NewCertifiedKeyPEM("localhost")
	require.NoError(t, err)

	// Create temp dir with all files
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")
	caPath := filepath.Join(tmpDir, "ca.pem")

	err = os.WriteFile(certPath, serverCert, 0o600)
	require.NoError(t, err)
	err = os.WriteFile(keyPath, serverKey, 0o600)
	require.NoError(t, err)
	err = os.WriteFile(caPath, ca.Cert(), 0o600)
	require.NoError(t, err)

	config := &tlsutil.Config{
		Enabled:    true,
		CertPath:   certPath,
		KeyPath:    keyPath,
		CAPath:     caPath,
		ClientAuth: "require",
	}

	tlsConfig, err := tlsutil.BuildTLSConfig(config)
	require.NoError(t, err)
	require.NotNil(t, tlsConfig)

	// Verify TLS 1.2 minimum version
	assert.Equal(t, uint16(tls.VersionTLS12), tlsConfig.MinVersion)

	// Verify client auth is set
	assert.Equal(t, tls.RequireAndVerifyClientCert, tlsConfig.ClientAuth)

	// Verify certificates are loaded
	assert.Len(t, tlsConfig.Certificates, 1)

	// Verify CA pool is set
	assert.NotNil(t, tlsConfig.ClientCAs)
}

// TestBuildTLSConfig_NoClientAuthNoCA verifies that CA is not required when clientAuth is "none".
func TestBuildTLSConfig_NoClientAuthNoCA(t *testing.T) {
	t.Parallel()

	// Generate real certificates for the test
	ca, err := certutil.NewCA()
	require.NoError(t, err)

	serverKey, serverCert, err := ca.NewCertifiedKeyPEM("localhost")
	require.NoError(t, err)

	// Create temp dir with only cert and key (no CA)
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	err = os.WriteFile(certPath, serverCert, 0o600)
	require.NoError(t, err)
	err = os.WriteFile(keyPath, serverKey, 0o600)
	require.NoError(t, err)

	config := &tlsutil.Config{
		Enabled:    true,
		CertPath:   certPath,
		KeyPath:    keyPath,
		ClientAuth: "none",
		// CAPath intentionally not set
	}

	tlsConfig, err := tlsutil.BuildTLSConfig(config)
	require.NoError(t, err)
	require.NotNil(t, tlsConfig)

	// Verify TLS config is valid without CA
	assert.Equal(t, tls.NoClientCert, tlsConfig.ClientAuth)
	assert.Nil(t, tlsConfig.ClientCAs)
}

// TestBuildTLSConfig_LoadCertFailed verifies error when certificate loading fails.
func TestBuildTLSConfig_LoadCertFailed(t *testing.T) {
	t.Parallel()

	// Create temp dir with invalid cert and key files
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")

	// Create invalid cert/key content
	err := os.WriteFile(certPath, []byte("invalid cert content"), 0o600)
	require.NoError(t, err)
	err = os.WriteFile(keyPath, []byte("invalid key content"), 0o600)
	require.NoError(t, err)

	config := &tlsutil.Config{
		Enabled:    true,
		CertPath:   certPath,
		KeyPath:    keyPath,
		ClientAuth: "none",
	}

	tlsConfig, err := tlsutil.BuildTLSConfig(config)
	assert.Nil(t, tlsConfig)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, tlsutil.ErrLoadCertFailed))
}

// TestBuildTLSConfig_ParseCAFailed verifies error when CA parsing fails.
func TestBuildTLSConfig_ParseCAFailed(t *testing.T) {
	t.Parallel()

	// Generate real certificates for the test
	ca, err := certutil.NewCA()
	require.NoError(t, err)

	serverKey, serverCert, err := ca.NewCertifiedKeyPEM("localhost")
	require.NoError(t, err)

	// Create temp dir with valid cert/key but invalid CA
	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")
	caPath := filepath.Join(tmpDir, "ca.pem")

	err = os.WriteFile(certPath, serverCert, 0o600)
	require.NoError(t, err)
	err = os.WriteFile(keyPath, serverKey, 0o600)
	require.NoError(t, err)
	// Write invalid CA content
	err = os.WriteFile(caPath, []byte("invalid CA content"), 0o600)
	require.NoError(t, err)

	config := &tlsutil.Config{
		Enabled:    true,
		CertPath:   certPath,
		KeyPath:    keyPath,
		CAPath:     caPath,
		ClientAuth: "require",
	}

	tlsConfig, err := tlsutil.BuildTLSConfig(config)
	assert.Nil(t, tlsConfig)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, tlsutil.ErrParseCAFailed))
}
