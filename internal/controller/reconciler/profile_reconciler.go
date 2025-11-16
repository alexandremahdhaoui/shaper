package reconciler

import (
	"context"
	"errors"

	"github.com/alexandremahdhaoui/shaper/pkg/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ProfileReconciler reconciles Profile objects
type ProfileReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

// Verify ProfileReconciler implements reconcile.Reconciler
var _ reconcile.Reconciler = &ProfileReconciler{}

// Reconcile implements the reconciliation loop for Profile resources
// It generates UUIDs for exposed additional content and updates the Profile status
func (r *ProfileReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("profile", req.NamespacedName)

	// Fetch the Profile
	var profile v1alpha1.Profile
	if err := r.Get(ctx, req.NamespacedName, &profile); err != nil {
		if apierrors.IsNotFound(err) {
			// Profile was deleted, nothing to do
			log.V(1).Info("Profile not found, likely deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Join(err, errors.New("failed to get profile"))
	}

	// Check if status update is needed
	needsUpdate := false
	if profile.Status.ExposedAdditionalContent == nil {
		profile.Status.ExposedAdditionalContent = make(map[string]string)
		needsUpdate = true
	}

	// Generate UUIDs for exposed content that doesn't have one
	for _, content := range profile.Spec.AdditionalContent {
		if content.Exposed {
			if _, exists := profile.Status.ExposedAdditionalContent[content.Name]; !exists {
				// Generate new UUID for this exposed content
				contentUUID := uuid.New().String()
				profile.Status.ExposedAdditionalContent[content.Name] = contentUUID
				needsUpdate = true
				log.Info("Generated UUID for exposed content",
					"contentName", content.Name,
					"uuid", contentUUID)
			}
		}
	}

	// Update status if needed (idempotent)
	if needsUpdate {
		if err := r.Status().Update(ctx, &profile); err != nil {
			log.Error(err, "Failed to update Profile status")
			return ctrl.Result{}, errors.Join(err, errors.New("failed to update profile status"))
		}
		log.Info("Successfully updated Profile status")
	} else {
		log.V(1).Info("No status update needed")
	}

	return ctrl.Result{}, nil
}
