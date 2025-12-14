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

package adapter

import (
	"context"
	"errors"

	"k8s.io/utils/ptr"

	"github.com/alexandremahdhaoui/shaper/internal/types"
	"github.com/alexandremahdhaoui/shaper/pkg/v1alpha1"
	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/jsonpath"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrProfileNotFound = errors.New("profile not found")
	errProfileGet      = errors.New("error getting profile")

	errProfileListByContentID = errors.New("listing profile by content id")

	// Conversions

	errConvertingProfile       = errors.New("converting profile")
	errAddContExposedButNoUUID = errors.New(
		"additional content is exposed but doesn't have a UUID",
	)
	errConvertingTransformerConfig = errors.New("converting transformer config")
)

// --------------------------------------------------- INTERFACES --------------------------------------------------- //

// Profile is an interface for getting profiles.
type Profile interface {
	// Get gets a profile by name in the adapter's configured namespace.
	Get(ctx context.Context, name string) (types.Profile, error)
	// GetInNamespace gets a profile by name in a specific namespace.
	GetInNamespace(ctx context.Context, name, namespace string) (types.Profile, error)
	// ListByContentID lists profiles by content ID.
	ListByContentID(ctx context.Context, configID uuid.UUID) ([]types.Profile, error)
}

// --------------------------------------------------- CONSTRUCTORS ------------------------------------------------- //

// NewProfile returns a new Profile.
func NewProfile(c client.Client, namespace string) Profile {
	return &v1a1Profile{
		client:    c,
		namespace: namespace,
	}
}

// --------------------------------------------- CONCRETE IMPLEMENTATION -------------------------------------------- //

type v1a1Profile struct {
	client    client.Client
	namespace string
}

// --------------------------------------------- Get ----------------------------------------------------------- //

func (p *v1a1Profile) Get(ctx context.Context, name string) (types.Profile, error) {
	return p.GetInNamespace(ctx, name, p.namespace)
}

func (p *v1a1Profile) GetInNamespace(ctx context.Context, name, namespace string) (types.Profile, error) {
	obj := new(v1alpha1.Profile)

	if err := p.client.Get(ctx, k8stypes.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, obj); apierrors.IsNotFound(err) {
		return types.Profile{}, errors.Join(err, ErrProfileNotFound, errProfileGet)
	} else if err != nil {
		return types.Profile{}, errors.Join(err, errProfileGet)
	}

	out, err := fromV1alpha1.toProfile(obj)
	if err != nil {
		return types.Profile{}, errors.Join(err, errProfileGet)
	}

	return out, nil
}

// --------------------------------------------- ListByContentID ------------------------------------------------------ //

// ListByContentID retrieve at most one Profile by a config ID. The nature of UUIDs and the defaulting webhook driver
// ensures the list contains at most 1 ID.
func (p *v1a1Profile) ListByContentID(
	ctx context.Context,
	configID uuid.UUID,
) ([]types.Profile, error) {
	// list profiles
	obj := new(v1alpha1.ProfileList)
	if err := p.client.List(ctx, obj, uuidLabelSelector(configID)); apierrors.IsNotFound(err) ||
		len(obj.Items) == 0 {
		return nil, errors.Join(err, ErrProfileNotFound, errProfileListByContentID)
	} else if err != nil {
		return nil, errors.Join(err, errProfileListByContentID)
	}

	out := make([]types.Profile, 0, len(obj.Items))
	for i := range obj.Items {
		profile, err := fromV1alpha1.toProfile(&obj.Items[i])
		if err != nil {
			return nil, errors.Join(err, errProfileListByContentID)
		}

		out = append(out, profile)
	}

	return out, nil
}

// --------------------------------------------------- CONVERSION --------------------------------------------------- //

var fromV1alpha1 ipxev1a1

type ipxev1a1 struct{}

func (ipxev1a1) toProfile(input *v1alpha1.Profile) (types.Profile, error) {
	idNameMap, rev, err := v1alpha1.UUIDLabelSelectors(input.Labels)
	if err != nil {
		return types.Profile{}, errors.Join(err, errConvertingProfile)
	}

	out := types.Profile{
		Name:               input.Name,
		Namespace:          input.Namespace,
		IPXETemplate:       input.Spec.IPXETemplate,
		AdditionalContent:  make(map[string]types.Content),
		ContentIDToNameMap: idNameMap,
	}

	for _, c := range input.Spec.AdditionalContent {
		content := types.Content{}
		content.Name = c.Name

		// 1. Is content exposed?
		if c.Exposed {
			content.Exposed = true

			id, ok := rev[c.Name]
			if !ok {
				return types.Profile{}, errors.Join(
					errAddContExposedButNoUUID,
					errConvertingProfile,
				)
			}

			content.ExposedUUID = id
		}

		// 2. Post transformers.
		transformers, err := fromV1alpha1.toTransformerConfig(c.PostTransformations)
		if err != nil {
			return types.Profile{}, errors.Join(err, errConvertingProfile)
		}

		content.PostTransformers = transformers

		// 3. Content kind.
		switch {
		case c.Inline != nil:
			content.ResolverKind = types.InlineResolverKind
			content.Inline = *c.Inline
		case c.ObjectRef != nil:
			ref, err := fromV1alpha1.toObjectRef(c.ObjectRef)
			if err != nil {
				return types.Profile{}, errors.Join(err, errConvertingProfile)
			}

			content.ResolverKind = types.ObjectRefResolverKind
			content.ObjectRef = &ref
		case c.Webhook != nil:
			cfg, err := fromV1alpha1.toWebhookConfig(c.Webhook)
			if err != nil {
				return types.Profile{}, errors.Join(err, errConvertingProfile)
			}

			content.ResolverKind = types.WebhookResolverKind
			content.WebhookConfig = &cfg
		}

		// 4. Add content to the map.
		out.AdditionalContent[c.Name] = content
	}

	return out, nil
}

var errConvertingObjectRef = errors.New("converting object ref")

func (ipxev1a1) toObjectRef(objectRef *v1alpha1.ObjectRef) (types.ObjectRef, error) {
	jp, err := toJSONPath(objectRef.JSONPath)
	if err != nil {
		return types.ObjectRef{}, errors.Join(err, errConvertingObjectRef)
	}

	return types.ObjectRef{
		Group:     objectRef.Group,
		Version:   objectRef.Version,
		Resource:  objectRef.Resource,
		Namespace: objectRef.Namespace,
		Name:      objectRef.Name,
		JSONPath:  jp,
	}, nil
}

func (ipxev1a1) toTransformerConfig(
	input []v1alpha1.Transformer,
) ([]types.TransformerConfig, error) {
	out := make([]types.TransformerConfig, 0)

	for _, t := range input {
		var cfg types.TransformerConfig

		switch {
		case t.ButaneToIgnition:
			cfg.Kind = types.ButaneTransformerKind
		case t.Webhook != nil:
			typesCfg, err := fromV1alpha1.toWebhookConfig(t.Webhook)
			if err != nil {
				return nil, errors.Join(errConvertingTransformerConfig)
			}

			cfg.Kind = types.WebhookTransformerKind
			cfg.Webhook = ptr.To(typesCfg)
		}

		out = append(out, cfg)
	}

	return out, nil
}

var errConvertingWebhookConfig = errors.New("converting webhook config")

func (ipxev1a1) toWebhookConfig(input *v1alpha1.WebhookConfig) (types.WebhookConfig, error) {
	out := types.WebhookConfig{}
	out.URL = input.URL

	if input.MTLSObjectRef != nil {
		ref, err := fromV1alpha1.toMTLSObjectRef(input.MTLSObjectRef)
		if err != nil {
			return types.WebhookConfig{}, errors.Join(err, errConvertingWebhookConfig)
		}

		out.MTLSObjectRef = ref
	}

	if input.BasicAuthObjectRef != nil {
		ref, err := fromV1alpha1.toBasicAuthObjectRef(input.BasicAuthObjectRef)
		if err != nil {
			return types.WebhookConfig{}, errors.Join(err, errConvertingWebhookConfig)
		}

		out.BasicAuthObjectRef = ref
	}

	return out, nil
}

var errConvertingMTLSObjectRef = errors.New("converting mtls object ref")

func (ipxev1a1) toMTLSObjectRef(ref *v1alpha1.MTLSObjectRef) (*types.MTLSObjectRef, error) {
	ckjp, err := toJSONPath(ref.ClientKeyJSONPath)
	if err != nil {
		return nil, errors.Join(err, errConvertingMTLSObjectRef)
	}

	ccjp, err := toJSONPath(ref.ClientCertJSONPath)
	if err != nil {
		return nil, errors.Join(err, errConvertingMTLSObjectRef)
	}

	cbjp, err := toJSONPath(ref.CaBundleJSONPath)
	if err != nil {
		return nil, errors.Join(err, errConvertingMTLSObjectRef)
	}

	return &types.MTLSObjectRef{
		ObjectRef: types.ObjectRef{
			Group:     ref.Group,
			Version:   ref.Version,
			Resource:  ref.Resource,
			Namespace: ref.Namespace,
			Name:      ref.Name,
		},
		ClientKeyJSONPath:     ckjp,
		ClientCertJSONPath:    ccjp,
		CaBundleJSONPath:      cbjp,
		TLSInsecureSkipVerify: ref.TLSInsecureSkipVerify,
	}, nil
}

var errConvertingBasicAuthObjectRef = errors.New("converting basic auth object ref")

func (ipxev1a1) toBasicAuthObjectRef(
	ref *v1alpha1.BasicAuthObjectRef,
) (*types.BasicAuthObjectRef, error) {
	ujp, err := toJSONPath(ref.UsernameJSONPath)
	if err != nil {
		return nil, errors.Join(err, errConvertingBasicAuthObjectRef)
	}

	pjp, err := toJSONPath(ref.PasswordJSONPath)
	if err != nil {
		return nil, errors.Join(err, errConvertingBasicAuthObjectRef)
	}

	return &types.BasicAuthObjectRef{
		ObjectRef: types.ObjectRef{
			Group:     ref.Group,
			Version:   ref.Version,
			Resource:  ref.Resource,
			Namespace: ref.Namespace,
			Name:      ref.Name,
		},
		UsernameJSONPath: ujp,
		PasswordJSONPath: pjp,
	}, nil
}

var errConvertingStringToJSONPath = errors.New("converting string to JSONPath")

func toJSONPath(s string) (*jsonpath.JSONPath, error) {
	jp := jsonpath.New("")
	if err := jp.Parse(s); err != nil {
		return nil, errors.Join(err, errConvertingStringToJSONPath)
	}

	return jp, nil
}
