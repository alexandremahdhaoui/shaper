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

package types

import (
	"github.com/google/uuid"
	"k8s.io/client-go/util/jsonpath"
)

// ---------------------------------------------------- PROFILE ----------------------------------------------------- //

// Profile is a struct that holds the iPXE template and additional content.
type Profile struct {
	// Name is the name of the Profile resource.
	Name string
	// Namespace is the namespace of the Profile resource.
	Namespace string
	// IPXETemplate is the iPXE template.
	IPXETemplate string

	// AdditionalContent is a map of additional content.
	AdditionalContent map[string]Content
	// ContentIDToNameMap is a map of content IDs to content names.
	ContentIDToNameMap map[uuid.UUID]string
}

// ---------------------------------------------------- CONTENT ----------------------------------------------------- //

// Content is a struct that holds the content of a profile.
type Content struct {
	// Name is the name of the content.
	Name string
	// Exposed is whether the content is exposed.
	Exposed bool
	// ExposedUUID is the UUID of the exposed content.
	ExposedUUID uuid.UUID

	// PostTransformers is a list of post transformers.
	PostTransformers []TransformerConfig
	// ResolverKind is the kind of resolver to use.
	ResolverKind ResolverKind

	// Inline is the inline content.
	Inline string
	// ObjectRef is the object reference to the content.
	ObjectRef *ObjectRef
	// WebhookConfig is the webhook configuration for the content.
	WebhookConfig *WebhookConfig
}

// ObjectRef is a struct that holds a reference to an object.
type ObjectRef struct {
	// Group is the group of the object.
	Group string
	// Version is the version of the object.
	Version string
	// Resource is the resource of the object.
	Resource string
	// Namespace is the namespace of the object.
	Namespace string
	// Name is the name of the object.
	Name string

	// JSONPath is optional for types that extends this struct.
	JSONPath *jsonpath.JSONPath
}

// WebhookConfig is a struct that holds the configuration for a webhook.
type WebhookConfig struct {
	// URL is the URL of the webhook.
	URL string

	// MTLSObjectRef is the object reference to the mTLS configuration.
	MTLSObjectRef *MTLSObjectRef
	// BasicAuthObjectRef is the object reference to the basic auth configuration.
	BasicAuthObjectRef *BasicAuthObjectRef
}

// BasicAuthObjectRef is a struct that holds a reference to a basic auth secret.
type BasicAuthObjectRef struct {
	ObjectRef

	// UsernameJSONPath is the JSON path to the username.
	UsernameJSONPath *jsonpath.JSONPath
	// PasswordJSONPath is the JSON path to the password.
	PasswordJSONPath *jsonpath.JSONPath
}

// MTLSObjectRef is a struct that holds a reference to a mTLS secret.
type MTLSObjectRef struct {
	ObjectRef

	// ClientKeyJSONPath is the JSON path to the client key.
	ClientKeyJSONPath *jsonpath.JSONPath
	// ClientCertJSONPath is the JSON path to the client cert.
	ClientCertJSONPath *jsonpath.JSONPath
	// CaBundleJSONPath is the JSON path to the CA bundle.
	CaBundleJSONPath *jsonpath.JSONPath

	// TLSInsecureSkipVerify is whether to skip TLS verification.
	TLSInsecureSkipVerify bool
}

// --------------------------------------------------- RESOLVER ----------------------------------------------------- //

// ResolverKind is a type for resolver kinds.
type ResolverKind int

const (
	// InlineResolverKind is the inline resolver kind.
	InlineResolverKind ResolverKind = iota
	// ObjectRefResolverKind is the object ref resolver kind.
	ObjectRefResolverKind
	// WebhookResolverKind is the webhook resolver kind.
	WebhookResolverKind
)

// -------------------------------------------------- TRANSFORMER --------------------------------------------------- //

// TransformerKind is a type for transformer kinds.
type TransformerKind int

const (
	// ButaneTransformerKind is the butane transformer kind.
	ButaneTransformerKind TransformerKind = iota
	// WebhookTransformerKind is the webhook transformer kind.
	WebhookTransformerKind
)

// TransformerConfig is a struct that holds the configuration for a transformer.
type TransformerConfig struct {
	// Kind is the kind of transformer.
	Kind TransformerKind

	// Webhook is the webhook configuration.
	Webhook *WebhookConfig
}
