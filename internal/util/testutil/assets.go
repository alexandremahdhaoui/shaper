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

package testutil

import (
	"fmt"

	"k8s.io/utils/ptr"

	"github.com/alexandremahdhaoui/shaper/internal/types"
	"github.com/alexandremahdhaoui/shaper/pkg/v1alpha1"
	"github.com/google/uuid"
	"k8s.io/client-go/util/jsonpath"
)

const (
	inlineName    = "test-inline"
	inlineContent = "test inline content"

	objectRefName = "test-object-ref"
	webhookName   = "test-webhook"

	ipxeTemplate = "abc123"

	WebhookServerFQDN    = "localhost"
	WebhookServerURLPath = "s3-test"
	WebhookServerPort    = 30443
)

func NewV1alpha1Profile() v1alpha1.Profile {
	return v1alpha1.Profile{
		Spec: v1alpha1.ProfileSpec{
			IPXETemplate: ipxeTemplate,
			AdditionalContent: []v1alpha1.AdditionalContent{
				NewV1alpha1AdditionalContentInline(),
				NewV1alpha1AdditionalContentObjectRef(),
				NewV1alpha1AdditionalContentWebhook(),
			},
		},
	}
}

func NewV1alpha1AdditionalContentInline() v1alpha1.AdditionalContent {
	return v1alpha1.AdditionalContent{
		Name:                inlineName,
		Exposed:             false,
		PostTransformations: nil,
		Inline:              ptr.To(inlineContent),
	}
}

func NewV1alpha1AdditionalContentObjectRef() v1alpha1.AdditionalContent {
	return v1alpha1.AdditionalContent{
		Name:                objectRefName,
		Exposed:             false,
		PostTransformations: nil,
		ObjectRef: &v1alpha1.ObjectRef{
			ResourceRef: v1alpha1.ResourceRef{
				Group:     "core",
				Version:   "v1",
				Resource:  "ConfigMap",
				Namespace: "test-namespace",
				Name:      "test-cm",
			},
			JSONPath: ".data.jsonPath",
		},
	}
}

func NewV1alpha1AdditionalContentWebhook() v1alpha1.AdditionalContent {
	return v1alpha1.AdditionalContent{
		Name:                webhookName,
		Exposed:             false,
		PostTransformations: nil,
		Webhook: &v1alpha1.WebhookConfig{
			URL: webhookURL(),
			MTLSObjectRef: &v1alpha1.MTLSObjectRef{
				ResourceRef: v1alpha1.ResourceRef{
					Group:     "core",
					Version:   "v1",
					Resource:  "Secret",
					Namespace: "test-namespace",
					Name:      "test-mtls",
				},
				ClientKeyJSONPath:  ".data.client\\.key",
				ClientCertJSONPath: ".data.client\\.crt",
				CaBundleJSONPath:   ".data.ca\\.crt",
			},
			BasicAuthObjectRef: &v1alpha1.BasicAuthObjectRef{
				ResourceRef: v1alpha1.ResourceRef{
					Group:     "yoursecret.amahdha.com",
					Version:   "v1beta2",
					Resource:  "YourSecret",
					Namespace: "test-namespace",
					Name:      "test-custom-secret",
				},
				UsernameJSONPath: ".data.username",
				PasswordJSONPath: ".data.password",
			},
		},
	}
}

func NewTypesProfile() types.Profile {
	ctInline := NewTypesContentInline()
	ctObjectRef := NewTypesContentObjectRef()
	ctWebhook := NewTypesContentWebhook()

	return types.Profile{
		IPXETemplate: ipxeTemplate,
		AdditionalContent: map[string]types.Content{
			ctInline.Name:    ctInline,
			ctObjectRef.Name: ctObjectRef,
			ctWebhook.Name:   ctWebhook,
		},
		ContentIDToNameMap: make(map[uuid.UUID]string),
	}
}

func NewTypesContentInline() types.Content {
	return types.Content{
		Name:             inlineName,
		PostTransformers: []types.TransformerConfig{},
		ResolverKind:     types.InlineResolverKind,
		Inline:           inlineContent,
	}
}

func NewTypesContentObjectRef() types.Content {
	return types.Content{
		Name:             objectRefName,
		PostTransformers: []types.TransformerConfig{},
		ResolverKind:     types.ObjectRefResolverKind,
		ObjectRef:        ptr.To(NewTypesObjectRef()),
	}
}

func NewTypesObjectRef() types.ObjectRef {
	return types.ObjectRef{
		Group:     "core",
		Version:   "v1",
		Resource:  "ConfigMap",
		Namespace: "test-namespace",
		Name:      "test-cm",
		JSONPath:  &jsonpath.JSONPath{}, // to annoying
	}
}

func NewTypesContentWebhook() types.Content {
	return types.Content{
		Name:             webhookName,
		PostTransformers: []types.TransformerConfig{},
		ResolverKind:     types.WebhookResolverKind,
		WebhookConfig:    ptr.To(NewTypesWebhookConfig()),
	}
}

func NewTypesTransformerConfigWebhook() types.TransformerConfig {
	return types.TransformerConfig{
		Kind:    types.WebhookTransformerKind,
		Webhook: ptr.To(NewTypesWebhookConfig()),
	}
}

func NewTypesWebhookConfig() types.WebhookConfig {
	return types.WebhookConfig{
		URL: webhookURL(),
		MTLSObjectRef: &types.MTLSObjectRef{
			ObjectRef: types.ObjectRef{
				Group:     "core",
				Version:   "v1",
				Resource:  "Secret",
				Namespace: "test-namespace",
				Name:      "test-mtls",
				JSONPath:  nil,
			},
			ClientKeyJSONPath:  &jsonpath.JSONPath{}, // to annoying
			ClientCertJSONPath: &jsonpath.JSONPath{}, // to annoying
			CaBundleJSONPath:   &jsonpath.JSONPath{}, // to annoying
		},
		BasicAuthObjectRef: &types.BasicAuthObjectRef{
			ObjectRef: types.ObjectRef{
				Group:     "yoursecret.amahdha.com",
				Version:   "v1beta2",
				Resource:  "YourSecret",
				Namespace: "test-namespace",
				Name:      "test-custom-secret",
				JSONPath:  nil,
			},
			UsernameJSONPath: &jsonpath.JSONPath{}, // to annoying
			PasswordJSONPath: &jsonpath.JSONPath{}, // to annoying
		},
	}
}

func MakeContentComparable(content types.Content) types.Content {
	if content.ObjectRef != nil {
		content.ObjectRef.JSONPath = &jsonpath.JSONPath{}
	}

	if content.WebhookConfig != nil {
		if content.WebhookConfig.BasicAuthObjectRef != nil {
			content.WebhookConfig.BasicAuthObjectRef.UsernameJSONPath = &jsonpath.JSONPath{}
			content.WebhookConfig.BasicAuthObjectRef.PasswordJSONPath = &jsonpath.JSONPath{}
		}

		if content.WebhookConfig.MTLSObjectRef != nil {
			content.WebhookConfig.MTLSObjectRef.CaBundleJSONPath = &jsonpath.JSONPath{}
			content.WebhookConfig.MTLSObjectRef.ClientCertJSONPath = &jsonpath.JSONPath{}
			content.WebhookConfig.MTLSObjectRef.ClientKeyJSONPath = &jsonpath.JSONPath{}
		}
	}

	return content
}

func MakeProfileComparable(profile types.Profile) types.Profile {
	for i := range profile.AdditionalContent {
		profile.AdditionalContent[i] = MakeContentComparable(profile.AdditionalContent[i])
	}

	return profile
}

func webhookURL() string {
	return fmt.Sprintf("%s:%d/%s", WebhookServerFQDN, WebhookServerPort, WebhookServerURLPath)
}
