package main

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// setupMetricsServer creates an HTTP server for Prometheus metrics.
func setupMetricsServer(config *Config) *http.Server {
	mux := http.NewServeMux()

	// Use default path if not specified
	path := config.MetricsServer.Path
	if path == "" {
		path = "/metrics"
	}

	// Register metrics handler
	mux.Handle(path, promhttp.Handler())

	return &http.Server{ //nolint:exhaustruct
		Addr:    fmt.Sprintf(":%d", config.MetricsServer.Port),
		Handler: mux,
	}
}
