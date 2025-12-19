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

package server

import (
	"context"
	"net"
	"net/http"
	"strings"
)

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const (
	// ClientIPContextKey is the context key for storing the client IP address.
	ClientIPContextKey contextKey = "client_ip"
)

// ClientIPMiddleware extracts the client IP from the request and adds it to the context.
// It checks X-Forwarded-For header first (for proxied requests), then falls back to RemoteAddr.
func ClientIPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := extractClientIP(r)
		ctx := context.WithValue(r.Context(), ClientIPContextKey, clientIP)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// extractClientIP extracts the client IP address from the request.
// It first checks the X-Forwarded-For header, then X-Real-IP, then RemoteAddr.
func extractClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (comma-separated list, first is original client)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			// Return the first IP (original client), trimmed
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fall back to RemoteAddr
	// RemoteAddr is in the format "IP:port" or "[IPv6]:port"
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// If SplitHostPort fails, return RemoteAddr as-is (might be just an IP)
		return r.RemoteAddr
	}

	return ip
}

// GetClientIP retrieves the client IP from the context.
// Returns empty string if not found.
func GetClientIP(ctx context.Context) string {
	if ip, ok := ctx.Value(ClientIPContextKey).(string); ok {
		return ip
	}
	return ""
}
