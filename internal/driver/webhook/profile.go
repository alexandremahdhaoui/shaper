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

package webhook

import (
	"context"
	"errors"
	"regexp"

	"github.com/alexandremahdhaoui/shaper/pkg/v1alpha1"
	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/jsonpath"

	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	_ webhook.CustomDefaulter = &Profile{}
	_ webhook.CustomValidator = &Profile{}

	// Regexes

	contentNameRegex = regexp.MustCompile("")
)

// NewProfile returns a new Profile webhook.
func NewProfile() *Profile {
	return &Profile{}
}

type Profile struct{}

func (p *Profile) Default(ctx context.Context, obj runtime.Object) error {
	profile, ok := obj.(*v1alpha1.Profile)
	if !ok {
		return NewUnsupportedResource(obj) // TODO: wrap err
	}

	if err := p.validateProfileStatic(ctx, obj); err != nil {
		return err // TODO: wrap err
	}

	// 1. get config UUIDs
	reverseIDMap := make(map[string]string)
	for k, value := range profile.Labels {
		if v1alpha1.IsUUIDLabelSelector(k) {
			reverseIDMap[value] = k // "content name" -> "label holding uuid"
		}
	}

	// 2. Remove all "internal" labels.
	for k := range profile.Labels {
		if !v1alpha1.IsInternalLabel(k) {
			delete(profile.Labels, k)
		}
	}

	// 3. Set labels preserving old UUIDs. (this is a bit overengineered, but may prevent a few race conditions).
	for _, content := range profile.Spec.AdditionalContent {
		if content.Exposed {
			if id, ok := reverseIDMap[content.Name]; ok {
				profile.Labels[id] = content.Name
			} else {
				profile.Labels[v1alpha1.NewUUIDLabelSelector(uuid.New())] = content.Name
			}
		}
	}

	return nil
}

func (p *Profile) ValidateCreate(
	ctx context.Context,
	obj runtime.Object,
) (admission.Warnings, error) {
	if err := p.validateProfileStatic(ctx, obj); err != nil {
		return nil, err // TODO: wrap err
	}

	if err := p.validateProfileDynamic(ctx, obj); err != nil {
		return nil, err // TODO: wrap err
	}

	return nil, nil
}

func (p *Profile) ValidateUpdate(
	ctx context.Context,
	oldObj, newObj runtime.Object,
) (admission.Warnings, error) {
	if err := p.validateProfileStatic(ctx, newObj); err != nil {
		return nil, err // TODO: wrap err
	}

	if err := p.validateProfileDynamic(ctx, newObj); err != nil {
		return nil, err // TODO: wrap err
	}

	return nil, nil
}

func (p *Profile) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (p *Profile) validateProfileStatic(ctx context.Context, obj runtime.Object) error {
	for _, f := range []validatingFunc{
		validateIPXETemplate,
		validateAdditionalContent,
	} {
		if err := f(ctx, obj); err != nil {
			return err // TODO: wrap err
		}
	}

	return nil
}

func (p *Profile) validateProfileDynamic(ctx context.Context, obj runtime.Object) error {
	for _, f := range []validatingFunc{
		// TODO
	} {
		if err := f(ctx, obj); err != nil {
			return err // TODO: wrap err
		}
	}

	return nil
}

func validateIPXETemplate(_ context.Context, _ runtime.Object) error {
	return nil
}

func validateAdditionalContent(ctx context.Context, obj runtime.Object) error {
	profile := obj.(*v1alpha1.Profile)

	for _, content := range profile.Spec.AdditionalContent {
		if !contentNameRegex.MatchString(content.Name) { // TODO: create the regex
			return errors.New("invalid additionalContent name") // TODO: err + wrap err
		}

		for _, transformer := range content.PostTransformations {
			if err := validateTransformer(transformer); err != nil {
				return err // TODO: wrap err
			}
		}

		// Count non-nil content sources
		var i uint
		if content.Inline != nil {
			i++
		}
		if content.ObjectRef != nil {
			i++
		}
		if content.Webhook != nil {
			i++
		}

		switch {
		case i == 0 || i > 1:
			return errors.New(
				"additionalContent MUST contain exactly 1 content configuration",
			) // TODO: wrap err
		case content.Inline != nil:
			return nil
		case content.ObjectRef != nil:
			if err := validateObjectRef(content.ObjectRef); err != nil {
				return err // TODO: wrap err
			}
			return nil
		case content.Webhook != nil:
			if err := validateWebhookConfig(content.Webhook); err != nil {
				return err // TODO: wrap err
			}
			return nil
		}
	}

	return nil
}

func validateObjectRef(ref *v1alpha1.ObjectRef) error {
	if err := validateResourceRef(ref.ResourceRef); err != nil {
		return err // TODO: wrap err
	}

	if err := validateJSONPath(ref.JSONPath); err != nil {
		return err // TODO: wrap err
	}

	return nil
}

func validateWebhookConfig(cfg *v1alpha1.WebhookConfig) error {
	if cfg.BasicAuthObjectRef != nil {
		if err := validateBasicAuthObjectRef(cfg.BasicAuthObjectRef); err != nil {
			return err // TODO: wrap err
		}
	}

	if cfg.MTLSObjectRef != nil {
		if err := validateMTLSObjectRef(cfg.MTLSObjectRef); err != nil {
			return err // TODO: wrap err
		}
	}

	return nil
}

func validateTransformer(transformer v1alpha1.Transformer) error {
	cfgCount := 0
	if transformer.ButaneToIgnition {
		cfgCount += 1
	}

	if transformer.Webhook != nil {
		cfgCount += 1
	}

	switch {
	case cfgCount == 0 || cfgCount > 1:
		return errors.Join(
			errors.New("a tranformer must either enable butaneToIgnition or specify a webhook"),
			errors.New("a transformer MUST specify exactly one configuration"),
		)
	case transformer.Webhook != nil:
		if err := validateWebhookConfig(transformer.Webhook); err != nil {
			return err // TODO: wrap err
		}
	}

	return nil
}

func validateBasicAuthObjectRef(ref *v1alpha1.BasicAuthObjectRef) error {
	if err := validateResourceRef(ref.ResourceRef); err != nil {
		return err // TODO: wrap err
	}

	for _, s := range []string{
		ref.UsernameJSONPath,
		ref.PasswordJSONPath,
	} {
		if err := validateJSONPath(s); err != nil {
			return err // TODO: wrap err
		}
	}

	return nil
}

func validateMTLSObjectRef(ref *v1alpha1.MTLSObjectRef) error {
	if err := validateResourceRef(ref.ResourceRef); err != nil {
		return err // TODO: wrap err
	}

	for _, s := range []string{
		ref.CaBundleJSONPath,
		ref.ClientCertJSONPath,
		ref.ClientKeyJSONPath,
	} {
		if err := validateJSONPath(s); err != nil {
			return err // TODO: wrap err
		}
	}

	return nil
}

func validateResourceRef(ref v1alpha1.ResourceRef) error {
	if ref.Name == "" || len(ref.Name) > 63 {
		return errors.Join(errors.New("invalid name"), errors.New("invalid resource reference"))
	}

	return nil
}

func validateJSONPath(s string) error {
	if _, err := jsonpath.Parse("", s); err != nil {
		return err // TODO: wrap err
	}

	return nil
}
