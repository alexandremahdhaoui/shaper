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

package adapter

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/alexandremahdhaoui/shaper/internal/types"
	"github.com/alexandremahdhaoui/shaper/pkg/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrAssignmentNotFound = errors.New("assignment not found")

	errAssignmentFindDefault     = errors.New("finding default assignment")
	errAssignmentFindBySelectors = errors.New("error finding assignment by selectors")
	errAssignmentList            = errors.New("listing assignment")
)

// --------------------------------------------------- INTERFACES --------------------------------------------------- //

// Assignment is an interface for finding assignments.
type Assignment interface {
	// FindDefaultByBuildarch finds the default assignment for a given build architecture.
	FindDefaultByBuildarch(ctx context.Context, buildarch string) (types.Assignment, error)
	// FindBySelectors finds an assignment by a given set of selectors.
	FindBySelectors(ctx context.Context, selectors types.IPXESelectors) (types.Assignment, error)
}

// --------------------------------------------------- CONSTRUCTORS ------------------------------------------------- //

// NewAssignment returns a new Assignment.
func NewAssignment(c client.Client, namespace string) Assignment {
	return &assignment{
		client:    c,
		namespace: namespace,
	}
}

// --------------------------------------------- CONCRETE IMPLEMENTATION -------------------------------------------- //

type assignment struct {
	client    client.Client
	namespace string
}

// --------------------------------------------- FindDefaultByBuildarch ------------------------------------------------- //

func (a *assignment) FindDefaultByBuildarch(ctx context.Context, buildarch string) (types.Assignment, error) {
	// list assignment
	list := new(v1alpha1.AssignmentList)

	// Build list options, filtering out nil values
	opts := make([]client.ListOption, 0, 2)
	if opt := buildarchLabelSelector(buildarch); opt != nil {
		opts = append(opts, opt)
	}
	opts = append(opts, defaultAssignmentLabelSelector())

	// Get the list of default matching the buildarch
	if err := a.client.List(ctx, list, opts...); err != nil {
		return types.Assignment{}, errors.Join(err, errAssignmentList, errAssignmentFindDefault)
	}

	if list == nil || len(list.Items) == 0 {
		return types.Assignment{}, errors.Join(ErrAssignmentNotFound, errAssignmentFindDefault)
	}

	var subjectSelectors map[string][]string
	if len(list.Items[0].Spec.SubjectSelectors.BuildarchList) > 0 || len(list.Items[0].Spec.SubjectSelectors.UUIDList) > 0 {
		subjectSelectors = make(map[string][]string)
		if len(list.Items[0].Spec.SubjectSelectors.BuildarchList) > 0 {
			buildarchStrings := make([]string, len(list.Items[0].Spec.SubjectSelectors.BuildarchList))
			for i, ba := range list.Items[0].Spec.SubjectSelectors.BuildarchList {
				buildarchStrings[i] = string(ba)
			}
			subjectSelectors["buildarch"] = buildarchStrings
		}
		if len(list.Items[0].Spec.SubjectSelectors.UUIDList) > 0 {
			subjectSelectors["uuid"] = list.Items[0].Spec.SubjectSelectors.UUIDList
		}
	}

	return types.Assignment{
		Name:             list.Items[0].Name,
		Namespace:        list.Items[0].Namespace,
		ProfileName:      list.Items[0].Spec.ProfileName,
		SubjectSelectors: subjectSelectors,
	}, nil
}

// --------------------------------------------- FindBySelectors --------------------------------------------- //

func (a *assignment) FindBySelectors(ctx context.Context, selectors types.IPXESelectors) (types.Assignment, error) {
	// list assignment
	list := new(v1alpha1.AssignmentList)

	// Build list options, filtering out nil values
	opts := make([]client.ListOption, 0, 2)
	if opt := buildarchLabelSelector(selectors.Buildarch); opt != nil {
		opts = append(opts, opt)
	}
	if opt := uuidLabelSelector(selectors.UUID); opt != nil {
		opts = append(opts, opt)
	}

	if err := a.client.List(ctx, list, opts...); err != nil {
		return types.Assignment{}, errors.Join(err, errAssignmentList, errAssignmentFindBySelectors)
	}

	if list == nil || len(list.Items) == 0 {
		return types.Assignment{}, errors.Join(ErrAssignmentNotFound, errAssignmentFindBySelectors)
	}

	var subjectSelectors map[string][]string
	if len(list.Items[0].Spec.SubjectSelectors.BuildarchList) > 0 || len(list.Items[0].Spec.SubjectSelectors.UUIDList) > 0 {
		subjectSelectors = make(map[string][]string)
		if len(list.Items[0].Spec.SubjectSelectors.BuildarchList) > 0 {
			buildarchStrings := make([]string, len(list.Items[0].Spec.SubjectSelectors.BuildarchList))
			for i, ba := range list.Items[0].Spec.SubjectSelectors.BuildarchList {
				buildarchStrings[i] = string(ba)
			}
			subjectSelectors["buildarch"] = buildarchStrings
		}
		if len(list.Items[0].Spec.SubjectSelectors.UUIDList) > 0 {
			subjectSelectors["uuid"] = list.Items[0].Spec.SubjectSelectors.UUIDList
		}
	}

	return types.Assignment{
		Name:             list.Items[0].Name,
		Namespace:        list.Items[0].Namespace,
		ProfileName:      list.Items[0].Spec.ProfileName,
		SubjectSelectors: subjectSelectors,
	}, nil
}

// --------------------------------------------- UTILS -------------------------------------------------------------- //

func buildarchLabelSelector(buildarch string) client.ListOption {
	switch v1alpha1.Buildarch(buildarch) {
	case v1alpha1.Arm32:
		return client.HasLabels{v1alpha1.Arm32BuildarchLabelSelector}
	case v1alpha1.Arm64:
		return client.HasLabels{v1alpha1.Arm64BuildarchLabelSelector}
	case v1alpha1.I386:
		return client.HasLabels{v1alpha1.I386BuildarchLabelSelector}
	case v1alpha1.X8664:
		return client.HasLabels{v1alpha1.X8664BuildarchLabelSelector}
	default:
		// not specifying anything implies any buildarch
		return nil
	}
}

func defaultAssignmentLabelSelector() client.ListOption {
	return client.HasLabels{v1alpha1.DefaultAssignmentLabel}
}

func uuidLabelSelector(id uuid.UUID) client.ListOption {
	return client.HasLabels{v1alpha1.NewUUIDLabelSelector(id)}
}
