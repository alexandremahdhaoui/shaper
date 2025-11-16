//go:build integration

package main_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alexandremahdhaoui/shaper/internal/util/certutil"
	"github.com/alexandremahdhaoui/shaper/internal/util/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMTLSCertificateGeneration_Integration verifies end-to-end mTLS certificate generation and usage
// This integration test demonstrates:
// 1. Creating a test CA
// 2. Generating server and client certificates
// 3. Establishing an mTLS connection
// 4. Successful authentication with valid certificates
//
// Note: Comprehensive mTLS failure scenarios are tested in unit tests:
// - internal/adapter/resolver_test.go (TestWebhookResolver/mTLS_* tests)
// - internal/adapter/transformer_test.go (TestWebhookTransformer/mTLS_* tests)
// - internal/util/testutil/certs_test.go (certificate generation tests)
func TestMTLSCertificateGeneration_Integration(t *testing.T) {
	// Create test CA
	ca, err := testutil.NewTestCA()
	require.NoError(t, err, "should create test CA")

	serverAddr := "localhost:39443"

	// Generate server certificate
	serverKeyPEM, serverCertPEM, err := ca.GenerateServerCert(serverAddr)
	require.NoError(t, err, "should generate server certificate")
	require.NotEmpty(t, serverKeyPEM, "server key should not be empty")
	require.NotEmpty(t, serverCertPEM, "server cert should not be empty")

	// Generate client certificate
	clientKeyPEM, clientCertPEM, err := ca.GenerateClientCert("test-client")
	require.NoError(t, err, "should generate client certificate")
	require.NotEmpty(t, clientKeyPEM, "client key should not be empty")
	require.NotEmpty(t, clientCertPEM, "client cert should not be empty")

	// Create server TLS config
	serverTLSConfig, err := testutil.CreateServerTLSConfig(serverCertPEM, serverKeyPEM, ca.CACertPEM())
	require.NoError(t, err, "should create server TLS config")
	require.NotNil(t, serverTLSConfig, "server TLS config should not be nil")

	// Create client TLS config
	clientTLSConfig, err := testutil.CreateClientTLSConfig(clientCertPEM, clientKeyPEM, ca.CACertPEM())
	require.NoError(t, err, "should create client TLS config")
	require.NotNil(t, clientTLSConfig, "client TLS config should not be nil")

	// Start HTTPS server with mTLS
	mux := http.NewServeMux()
	mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		// Verify client certificate was presented
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			http.Error(w, "No client certificate", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("mTLS success"))
	})

	server := &http.Server{
		Addr:      serverAddr,
		Handler:   mux,
		TLSConfig: serverTLSConfig,
	}

	// Start server in background
	serverErrChan := make(chan error, 1)
	go func() {
		// Use empty strings because cert/key are in TLSConfig
		serverErrChan <- server.ListenAndServeTLS("", "")
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Ensure server cleanup
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	// Test successful mTLS connection
	t.Run("Successful mTLS connection", func(t *testing.T) {
		clientTLSConfig.ServerName = serverAddr // Set SNI

		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: clientTLSConfig,
			},
			Timeout: 5 * time.Second,
		}

		resp, err := client.Get(fmt.Sprintf("https://%s/test", serverAddr))
		require.NoError(t, err, "should successfully connect with valid mTLS certificates")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode, "should get 200 OK with valid mTLS")

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "should read response body")
		assert.Equal(t, "mTLS success", string(body), "should get success message")

		t.Logf("Successfully established mTLS connection to %s", serverAddr)
	})

	// Test connection without client certificate fails
	t.Run("Connection without client certificate fails", func(t *testing.T) {
		// Create client config without client certificate
		noClientCertConfig := &tls.Config{
			RootCAs:    ca.CertPool(),
			ServerName: serverAddr,
			MinVersion: tls.VersionTLS12,
		}

		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: noClientCertConfig,
			},
			Timeout: 5 * time.Second,
		}

		_, err := client.Get(fmt.Sprintf("https://%s/test", serverAddr))
		assert.Error(t, err, "should fail when no client certificate is provided")
		assert.Contains(t, err.Error(), "tls", "error should be TLS-related")

		t.Logf("Connection correctly rejected without client certificate: %v", err)
	})

	// Test connection with wrong CA fails
	t.Run("Connection with wrong CA fails", func(t *testing.T) {
		// Create a different CA
		wrongCA, err := certutil.NewCA()
		require.NoError(t, err)

		// Generate client cert from wrong CA
		wrongClientKey, wrongClientCert, err := wrongCA.NewCertifiedKeyPEM(serverAddr)
		require.NoError(t, err)

		// Create client config with cert from wrong CA
		wrongCAConfig := &tls.Config{
			Certificates: []tls.Certificate{
				func() tls.Certificate {
					cert, _ := tls.X509KeyPair(wrongClientCert, wrongClientKey)
					return cert
				}(),
			},
			RootCAs:    ca.CertPool(), // Use correct CA for server validation
			ServerName: serverAddr,
			MinVersion: tls.VersionTLS12,
		}

		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: wrongCAConfig,
			},
			Timeout: 5 * time.Second,
		}

		_, err = client.Get(fmt.Sprintf("https://%s/test", serverAddr))
		assert.Error(t, err, "should fail when client certificate is from wrong CA")
		assert.Contains(t, strings.ToLower(err.Error()), "tls", "error should be TLS-related")

		t.Logf("Connection correctly rejected with wrong CA certificate: %v", err)
	})
}
