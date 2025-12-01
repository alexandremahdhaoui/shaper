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

package main

import (
	"fmt"
	"os"

	"github.com/alexandremahdhaoui/shaper/internal/controller/reconciler"
	"github.com/alexandremahdhaoui/shaper/internal/util/logging"
	"github.com/alexandremahdhaoui/shaper/pkg/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
}

func main() {
	// Load configuration
	configPath := os.Getenv(ConfigPathEnvKey)
	config, err := LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Setup logger using shared logging utility
	logging.Setup(logging.Options{
		Development: config.DevelopmentMode,
	})

	setupLog.Info("Starting shaper-controller",
		"metricsAddr", config.MetricsBind,
		"probeAddr", config.HealthBind,
		"leaderElection", config.LeaderElection,
		"namespace", config.Namespace)

	// Create manager
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: config.MetricsBind,
		},
		HealthProbeBindAddress: config.HealthBind,
		LeaderElection:         config.LeaderElection,
		LeaderElectionID:       config.LeaderElectionID,
	})
	if err != nil {
		setupLog.Error(err, "unable to create manager")
		os.Exit(1)
	}

	// Setup controllers
	if err := setupControllers(mgr, ctrl.Log); err != nil {
		setupLog.Error(err, "unable to setup controllers")
		os.Exit(1)
	}

	// Setup health checks
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// setupControllers registers all reconcilers with the manager
func setupControllers(mgr ctrl.Manager, log logr.Logger) error {
	// Setup ProfileReconciler
	profileReconciler := &reconciler.ProfileReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Log:    log.WithName("controllers").WithName("Profile"),
	}
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Profile{}).
		Complete(profileReconciler); err != nil {
		return fmt.Errorf("failed to create Profile controller: %w", err)
	}
	log.Info("Profile controller registered")

	// Setup AssignmentReconciler
	assignmentReconciler := &reconciler.AssignmentReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Log:    log.WithName("controllers").WithName("Assignment"),
	}
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Assignment{}).
		Complete(assignmentReconciler); err != nil {
		return fmt.Errorf("failed to create Assignment controller: %w", err)
	}
	log.Info("Assignment controller registered")

	return nil
}
