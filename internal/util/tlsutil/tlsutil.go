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

// Package tlsutil provides utilities for building TLS configurations.
package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
)

var (
	// ErrCertNotFound is returned when the certificate file does not exist.
	ErrCertNotFound = errors.New("certificate file not found")
	// ErrKeyNotFound is returned when the key file does not exist.
	ErrKeyNotFound = errors.New("key file not found")
	// ErrCANotFound is returned when the CA file does not exist.
	ErrCANotFound = errors.New("CA file not found")
	// ErrInvalidClientAuth is returned when the clientAuth value is not valid.
	ErrInvalidClientAuth = errors.New("invalid clientAuth value")
	// ErrLoadCertFailed is returned when loading the certificate fails.
	ErrLoadCertFailed = errors.New("failed to load certificate")
	// ErrLoadCAFailed is returned when loading the CA file fails.
	ErrLoadCAFailed = errors.New("failed to load CA file")
	// ErrParseCAFailed is returned when parsing the CA certificate fails.
	ErrParseCAFailed = errors.New("failed to parse CA certificate")
)

// Config holds the TLS configuration parameters.
type Config struct {
	// Enabled enables TLS for the server.
	Enabled bool
	// ClientAuth specifies the client authentication policy.
	// Valid values: "none", "request", "require".
	ClientAuth string
	// CertPath is the path to the server certificate file.
	CertPath string
	// KeyPath is the path to the server private key file.
	KeyPath string
	// CAPath is the path to the CA certificate file for client verification.
	CAPath string
}

// BuildTLSConfig builds a tls.Config from the provided configuration.
//
// Returns nil, nil when TLS is disabled.
// Returns an error if:
//   - CertPath does not exist
//   - KeyPath does not exist
//   - CAPath does not exist when ClientAuth != "none"
//   - ClientAuth value is not valid ("none", "request", "require")
//   - Loading the certificate or key fails
//   - Loading or parsing the CA certificate fails
func BuildTLSConfig(config *Config) (*tls.Config, error) {
	if config == nil || !config.Enabled {
		return nil, nil
	}

	// Validate certificate file exists
	if _, err := os.Stat(config.CertPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: %s", ErrCertNotFound, config.CertPath)
	}

	// Validate key file exists
	if _, err := os.Stat(config.KeyPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: %s", ErrKeyNotFound, config.KeyPath)
	}

	// Map clientAuth string to tls.ClientAuthType
	clientAuthType, err := parseClientAuth(config.ClientAuth)
	if err != nil {
		return nil, err
	}

	// Validate CA file exists when client auth is required
	if clientAuthType != tls.NoClientCert {
		if _, err := os.Stat(config.CAPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrCANotFound, config.CAPath)
		}
	}

	// Load server certificate and key
	cert, err := tls.LoadX509KeyPair(config.CertPath, config.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLoadCertFailed, err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		MaxVersion:   tls.VersionTLS12, // iPXE only supports TLS 1.2
		ClientAuth:   clientAuthType,
		// Include cipher suites compatible with iPXE (which only supports RSA key exchange)
		// iPXE does not support ECDHE cipher suites, so we need RSA-based ones
		CipherSuites: []uint16{
			// RSA cipher suites for iPXE compatibility (listed first for priority)
			tls.TLS_RSA_WITH_AES_128_CBC_SHA,
			tls.TLS_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			// ECDHE suites for modern clients
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		},
		// Force HTTP/1.1 for iPXE compatibility
		// iPXE does not support HTTP/2 and may not handle ALPN negotiation correctly
		// Setting NextProtos to http/1.1 disables HTTP/2 and ensures HTTP/1.1 is used
		NextProtos: []string{"http/1.1"},
		// Disable session tickets for iPXE compatibility
		// Go's TLS server sends NewSessionTicket after the handshake by default.
		// iPXE may not handle session tickets correctly, causing connection resets.
		SessionTicketsDisabled: true,
	}

	// Load CA certificate when client auth is enabled
	if clientAuthType != tls.NoClientCert {
		caBytes, err := os.ReadFile(config.CAPath)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrLoadCAFailed, err)
		}

		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caBytes) {
			return nil, ErrParseCAFailed
		}

		tlsConfig.ClientCAs = caPool
	}

	return tlsConfig, nil
}

// parseClientAuth maps a clientAuth string to tls.ClientAuthType.
func parseClientAuth(clientAuth string) (tls.ClientAuthType, error) {
	switch clientAuth {
	case "", "none":
		return tls.NoClientCert, nil
	case "request":
		return tls.RequestClientCert, nil
	case "require":
		return tls.RequireAndVerifyClientCert, nil
	default:
		return 0, fmt.Errorf("%w: %q (valid values: none, request, require)", ErrInvalidClientAuth, clientAuth)
	}
}
