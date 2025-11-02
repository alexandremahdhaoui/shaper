package main

import (
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/alexandremahdhaoui/shaper/pkg/v1alpha1"
	driverwebhook "github.com/alexandremahdhaoui/shaper/internal/driver/webhook"
)

// setupWebhookServer registers webhooks with the controller-runtime manager.
func setupWebhookServer(
	mgr ctrl.Manager,
	assignmentWebhook *driverwebhook.Assignment,
	profileWebhook *driverwebhook.Profile,
) error {
	server := mgr.GetWebhookServer()

	// Register Assignment validation webhook
	server.Register(
		"/validate-shaper-amahdha-com-v1alpha1-assignment",
		&webhook.Admission{
			Handler: admission.WithCustomValidator(
				mgr.GetScheme(),
				&v1alpha1.Assignment{},
				assignmentWebhook,
			),
		},
	)

	// Register Assignment mutation webhook
	server.Register(
		"/mutate-shaper-amahdha-com-v1alpha1-assignment",
		&webhook.Admission{
			Handler: admission.WithCustomDefaulter(
				mgr.GetScheme(),
				&v1alpha1.Assignment{},
				assignmentWebhook,
			),
		},
	)

	// Register Profile validation webhook
	server.Register(
		"/validate-shaper-amahdha-com-v1alpha1-profile",
		&webhook.Admission{
			Handler: admission.WithCustomValidator(
				mgr.GetScheme(),
				&v1alpha1.Profile{},
				profileWebhook,
			),
		},
	)

	// Register Profile mutation webhook
	server.Register(
		"/mutate-shaper-amahdha-com-v1alpha1-profile",
		&webhook.Admission{
			Handler: admission.WithCustomDefaulter(
				mgr.GetScheme(),
				&v1alpha1.Profile{},
				profileWebhook,
			),
		},
	)

	return nil
}
