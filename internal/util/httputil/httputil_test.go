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

package httputil_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/alexandremahdhaoui/shaper/internal/util/gracefulshutdown"
	"github.com/alexandremahdhaoui/shaper/internal/util/httputil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBasicAuth_ValidCredentials verifies BasicAuth middleware allows requests with valid credentials.
func TestBasicAuth_ValidCredentials(t *testing.T) {
	// Track if next handler was called
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	// Create validator that accepts specific credentials
	validator := func(username, password string, r *http.Request) (bool, error) {
		if username == "testuser" && password == "testpass" {
			return true, nil
		}
		return false, nil
	}

	// Wrap handler with BasicAuth middleware
	handler := httputil.BasicAuth(next, validator)

	// Create request with valid Basic Auth
	req := httptest.NewRequest("GET", "/test", nil)
	req.SetBasicAuth("testuser", "testpass")
	rr := httptest.NewRecorder()

	// Execute request
	handler.ServeHTTP(rr, req)

	// Verify response
	assert.Equal(t, http.StatusOK, rr.Code, "should return 200 OK")
	assert.Equal(t, "success", rr.Body.String(), "should return success message")
	assert.True(t, nextCalled, "next handler should have been called")
}

// TestBasicAuth_InvalidCredentials verifies BasicAuth middleware rejects requests with invalid credentials.
func TestBasicAuth_InvalidCredentials(t *testing.T) {
	tests := []struct {
		name           string
		setupAuth      func(*http.Request)
		expectedStatus int
		expectWWWAuth  bool
	}{
		{
			name: "wrong password",
			setupAuth: func(req *http.Request) {
				req.SetBasicAuth("testuser", "wrongpass")
			},
			expectedStatus: http.StatusUnauthorized,
			expectWWWAuth:  true,
		},
		{
			name: "no auth header",
			setupAuth: func(req *http.Request) {
				// Don't set any auth
			},
			expectedStatus: http.StatusUnauthorized,
			expectWWWAuth:  true,
		},
		{
			name: "wrong username",
			setupAuth: func(req *http.Request) {
				req.SetBasicAuth("wronguser", "testpass")
			},
			expectedStatus: http.StatusUnauthorized,
			expectWWWAuth:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Track if next handler was called
			nextCalled := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusOK)
			})

			// Create validator that only accepts specific credentials
			validator := func(username, password string, r *http.Request) (bool, error) {
				if username == "testuser" && password == "testpass" {
					return true, nil
				}
				return false, nil
			}

			// Wrap handler with BasicAuth middleware
			handler := httputil.BasicAuth(next, validator)

			// Create request
			req := httptest.NewRequest("GET", "/test", nil)
			tt.setupAuth(req)
			rr := httptest.NewRecorder()

			// Execute request
			handler.ServeHTTP(rr, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, rr.Code, "should return correct status code")
			assert.False(t, nextCalled, "next handler should NOT have been called")

			if tt.expectWWWAuth {
				wwwAuth := rr.Header().Get("WWW-Authenticate")
				assert.NotEmpty(t, wwwAuth, "should have WWW-Authenticate header")
				assert.Contains(t, wwwAuth, "Basic realm", "should contain Basic realm")
			}

			assert.Contains(t, rr.Body.String(), "Unauthorized", "response should indicate unauthorized")
		})
	}
}

// TestBasicAuth_ValidatorError verifies BasicAuth middleware handles validator errors correctly.
func TestBasicAuth_ValidatorError(t *testing.T) {
	// Track if next handler was called
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Create validator that returns an error
	validator := func(username, password string, r *http.Request) (bool, error) {
		return false, assert.AnError // Use a test error
	}

	// Wrap handler with BasicAuth middleware
	handler := httputil.BasicAuth(next, validator)

	// Create request with valid Basic Auth format
	req := httptest.NewRequest("GET", "/test", nil)
	req.SetBasicAuth("testuser", "testpass")
	rr := httptest.NewRecorder()

	// Execute request
	handler.ServeHTTP(rr, req)

	// Verify response
	assert.Equal(t, http.StatusInternalServerError, rr.Code, "should return 500 on validator error")
	assert.False(t, nextCalled, "next handler should NOT have been called")
	assert.NotEmpty(t, rr.Body.String(), "should have error message in response")
}

// TestServe verifies the Serve() function with mocked graceful shutdown.
func TestServe(t *testing.T) {
	t.Run("serve handles graceful shutdown", func(t *testing.T) {
		// Mock exit function with mutex protection
		var mu sync.Mutex
		exitCalled := false
		var exitCode int
		mockExit := func(code int) {
			mu.Lock()
			defer mu.Unlock()
			exitCode = code
			exitCalled = true
		}

		gs := gracefulshutdown.NewWithExit("test", mockExit)

		// Create test server with simple handler on dynamic port
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		server := &http.Server{
			Addr:    "127.0.0.1:0", // Use port 0 for dynamic port allocation
			Handler: handler,
		}

		servers := map[string]*http.Server{
			"test-server": server,
		}

		// Start Serve in goroutine (it blocks)
		go httputil.Serve(servers, gs)

		// Give servers time to start
		time.Sleep(100 * time.Millisecond)

		// Cancel context to trigger shutdown
		gs.CancelFunc()()

		// Give shutdown time to complete
		time.Sleep(200 * time.Millisecond)

		// Verify exit was called with code 0 (graceful shutdown)
		mu.Lock()
		defer mu.Unlock()
		assert.True(t, exitCalled, "exit should be called after shutdown")
		assert.Equal(t, 0, exitCode, "should exit with code 0 on graceful shutdown")
	})

	t.Run("serve handles server startup error", func(t *testing.T) {
		// Test that server errors trigger shutdown with exit code 1
		var mu sync.Mutex
		exitCalled := false
		var exitCode int
		mockExit := func(code int) {
			mu.Lock()
			defer mu.Unlock()
			if !exitCalled { // Only capture first exit call
				exitCode = code
				exitCalled = true
			}
		}

		gs := gracefulshutdown.NewWithExit("test", mockExit)

		// Create server that will fail to start (port already in use)
		// First bind a test server to a port
		blocker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer blocker.Close()

		// Try to create another server on the same address
		server := &http.Server{
			Addr:    blocker.Listener.Addr().String(),
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		}

		servers := map[string]*http.Server{
			"test-server": server,
		}

		// Start Serve (will fail immediately due to port conflict)
		go httputil.Serve(servers, gs)

		// Give time for error to occur and shutdown to be called
		time.Sleep(200 * time.Millisecond)

		// Should exit with code 1 on error
		mu.Lock()
		defer mu.Unlock()
		require.True(t, exitCalled, "exit should be called after error")
		assert.Equal(t, 1, exitCode, "should exit with code 1 on error")
	})
}
