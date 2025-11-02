package main

import (
	"fmt"
	"net/http"
)

// setupProbesServer creates an HTTP server for health probes (liveness and readiness).
func setupProbesServer(config *Config) *http.Server {
	mux := http.NewServeMux()

	// Register liveness probe handler
	mux.HandleFunc(config.ProbesServer.LivenessPath, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Register readiness probe handler
	mux.HandleFunc(config.ProbesServer.ReadinessPath, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	return &http.Server{ //nolint:exhaustruct
		Addr:    fmt.Sprintf(":%d", config.ProbesServer.Port),
		Handler: mux,
	}
}
