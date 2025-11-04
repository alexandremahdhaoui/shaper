package main

import (
	"fmt"
	"net/http"
)

// setupProbesServer creates an HTTP server for health probes (liveness and readiness).
func setupProbesServer(config *Config) *http.Server {
	mux := http.NewServeMux()

	// Use default paths if not specified
	livenessPath := config.ProbesServer.LivenessPath
	if livenessPath == "" {
		livenessPath = "/healthz"
	}
	readinessPath := config.ProbesServer.ReadinessPath
	if readinessPath == "" {
		readinessPath = "/readyz"
	}

	// Register liveness probe handler
	mux.HandleFunc(livenessPath, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Register readiness probe handler
	mux.HandleFunc(readinessPath, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	return &http.Server{ //nolint:exhaustruct
		Addr:    fmt.Sprintf(":%d", config.ProbesServer.Port),
		Handler: mux,
	}
}
