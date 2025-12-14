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

package webhook

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/alexandremahdhaoui/shaper/internal/adapter"
	"github.com/alexandremahdhaoui/shaper/internal/types"
	"github.com/alexandremahdhaoui/shaper/pkg/v1alpha1"
	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	_ webhook.CustomValidator = &Assignment{}
	_ webhook.CustomDefaulter = &Assignment{}
)

// NewAssignment returns a new Assignment webhook.
func NewAssignment(assignment adapter.Assignment, profile adapter.Profile) *Assignment {
	return &Assignment{
		assignment: assignment,
		profile:    profile,
	}
}

type Assignment struct {
	assignment adapter.Assignment
	profile    adapter.Profile
}

func (a *Assignment) Default(ctx context.Context, obj runtime.Object) error {
	assignment, ok := obj.(*v1alpha1.Assignment)
	if !ok {
		return NewUnsupportedResource(obj) // TODO: wrap err
	}

	if err := a.validateAssignmentStatic(ctx, obj); err != nil {
		return err // TODO: wrap err
	}

	// Initialize Labels map if nil
	if assignment.Labels == nil {
		assignment.Labels = make(map[string]string)
	}

	// 1. Remove all "internal" labels. (remove ones created by users && clean up old ones)
	for k := range assignment.Labels {
		if !v1alpha1.IsInternalLabel(k) {
			delete(assignment.Labels, k)
		}
	}

	// 2. Add uuid subject selectors
	for _, subjectID := range assignment.Spec.SubjectSelectors.UUIDList {
		id, err := uuid.Parse(subjectID)
		if err != nil {
			return err // TODO: wrap err
		}

		v1alpha1.SetUUIDLabelSelector(assignment, id, "")
	}

	// 3. Add buildarch labels etc...
	buildarchList := assignment.Spec.SubjectSelectors.BuildarchList
	if len(buildarchList) == 0 {
		// unspecified implies any buildarch.
		buildarchList = slices.Clone(v1alpha1.AllowedBuildarchList)
	}

	for _, b := range buildarchList {
		assignment.SetBuildarch(b)
	}

	return nil
}

func (a *Assignment) ValidateCreate(
	ctx context.Context,
	obj runtime.Object,
) (admission.Warnings, error) {
	// simple validations should happen first.
	if err := a.validateAssignmentStatic(ctx, obj); err != nil {
		return nil, err // TODO: log + wrap err
	}

	// Validations making requesting to external services should happen after simple validations.
	if err := a.validateAssignmentDynamic(ctx, obj); err != nil {
		return nil, err // TODO: log + wrap err
	}

	return nil, nil
}

func (a *Assignment) ValidateUpdate(
	ctx context.Context,
	oldObj, newObj runtime.Object,
) (admission.Warnings, error) {
	// simple validations should happen first.
	if err := a.validateAssignmentStatic(ctx, newObj); err != nil {
		return nil, err // TODO: log + wrap err
	}

	// Validations making requesting to external services should happen after simple validations.
	if err := a.validateAssignmentDynamic(ctx, newObj); err != nil {
		return nil, err // TODO: log + wrap err
	}

	return nil, nil
}

func (a *Assignment) ValidateDelete(
	_ context.Context,
	_ runtime.Object,
) (admission.Warnings, error) {
	return nil, nil
}

func (a *Assignment) validateAssignmentStatic(ctx context.Context, obj runtime.Object) error {
	for _, f := range []validatingFunc{
		validateUUIDList,
		validateBuildarchList,
		validateIsDefault,
	} {
		if err := f(ctx, obj); err != nil {
			return err // TODO: wrap err
		}
	}

	return nil
}

func (a *Assignment) validateAssignmentDynamic(ctx context.Context, obj runtime.Object) error {
	// Defensive nil checks for adapters
	if a.profile == nil || a.assignment == nil {
		return errors.New("webhook adapters not properly initialized")
	}

	for _, f := range []validatingFunc{
		a.validateProfileName,
		a.validateDefaultAssignmentForBuildarchIsUnique,
		a.validateUUIDAssignmentIsUnique,
	} {
		if err := f(ctx, obj); err != nil {
			return err // TODO: wrap err
		}
	}

	return nil
}

func validateBuildarchList(_ context.Context, obj runtime.Object) error {
	assignment := obj.(*v1alpha1.Assignment)

	for _, b := range assignment.Spec.SubjectSelectors.BuildarchList {
		if _, ok := v1alpha1.AllowedBuildarch[b]; !ok {
			return errors.Join(
				errors.New("specified buildarch is not supported"),
				fmt.Errorf("expected one of 'arm32', 'arm64', 'i386', 'x86_64'; received %q", b.String()),
			) // TODO: err + wrap err
		}
	}

	return nil
}

func validateUUIDList(_ context.Context, obj runtime.Object) error {
	assignment := obj.(*v1alpha1.Assignment)

	for _, id := range assignment.Spec.SubjectSelectors.UUIDList {
		_, err := uuid.Parse(id)
		if err != nil {
			return err // TODO: wrap err
		}
	}

	return nil
}

func validateIsDefault(_ context.Context, obj runtime.Object) error {
	assignment := obj.(*v1alpha1.Assignment)

	if !assignment.Spec.IsDefault {
		return nil
	}

	if len(assignment.Spec.SubjectSelectors.UUIDList) > 0 {
		return errors.New("a default assignment must not specify subject selectors of type UUID") // TODO: err + wrap err
	}

	return nil
}

func (a *Assignment) validateProfileName(ctx context.Context, obj runtime.Object) error {
	assignment := obj.(*v1alpha1.Assignment)

	// Use the assignment's namespace to look up the profile
	_, err := a.profile.GetInNamespace(ctx, assignment.Spec.ProfileName, assignment.Namespace)
	if errors.Is(err, adapter.ErrProfileNotFound) {
		// Return an error if the referred profile does not exist.
		return errors.New("assignment must specify an existing profileName") // TODO: err + wrap err
	} else if err != nil {
		return err // TODO: wrap err
	}

	return nil
}

func (a *Assignment) validateDefaultAssignmentForBuildarchIsUnique(ctx context.Context, obj runtime.Object) error {
	//  A default assignment should be unique for a given list of buildarch.
	assignment := obj.(*v1alpha1.Assignment)
	if !assignment.Spec.IsDefault {
		return nil
	}

	for _, b := range assignment.GetBuildarchList() {
		assign, err := a.assignment.FindDefaultByBuildarch(ctx, b.String())
		if errors.Is(err, adapter.ErrAssignmentNotFound) {
			// this is the good scenario
			continue
		} else if err != nil {
			return err // TODO: wrap err
		}

		if assign.Name == assignment.Name {
			// update scenario should pass
			continue
		}

		return errors.New("TODO") // TODO: err + wrap err
	}

	return nil
}

func (a *Assignment) validateUUIDAssignmentIsUnique(ctx context.Context, obj runtime.Object) error {
	assignment := obj.(*v1alpha1.Assignment)

	if assignment.Spec.IsDefault {
		return nil
	}

	// 1. Assignment should be mutually exclusive.
	// 1.a. Build list of selectors
	selectorsList := make([]types.IPXESelectors, 0)
	// this is extremely inefficient
	for _, id := range assignment.Spec.SubjectSelectors.UUIDList {
		parsed, _ := uuid.Parse(id) // safely ignoring err because it has already been validated.

		selectorsList = append(selectorsList, types.IPXESelectors{
			UUID: parsed,
		})
	}

	// 1.b. check if we find any match.
	// TODO: looping over selectors is a very poor operation; use a better solution, such as listing all then filtering.
	for _, selectors := range selectorsList {
		// Verify no other Assignment exist for the specified selector.
		matchedAssignment, err := a.assignment.FindBySelectors(ctx, selectors)
		if errors.Is(err, adapter.ErrAssignmentNotFound) {
			// ignore if assignment is not found.
			continue
		} else if err != nil {
			return err // TODO: wrap error
		}

		if matchedAssignment.Name == assignment.GetName() {
			// UPDATE CASE: matchedAssignment matches the assignment being modified: we can safely ignore.
			continue
		}

		return errors.Join(
			errors.New("assignment cannot reference a subject selector referenced "),
		) // TODO: wrap err
	}

	return nil
}
