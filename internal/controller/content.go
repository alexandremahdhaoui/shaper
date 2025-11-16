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

package controller

import (
	"context"
	"errors"
	"log/slog"

	"github.com/alexandremahdhaoui/shaper/internal/adapter"
	"github.com/alexandremahdhaoui/shaper/internal/types"
	"github.com/google/uuid"
)

var (
	ErrContentNotFound = errors.New("content cannot be found")
	ErrContentGetById  = errors.New("getting content by id")

	errUUIDCannotBeNil = errors.New("uuid cannot be nil")
)

// ---------------------------------------------------- INTERFACE --------------------------------------------------- //

// Content is an interface for getting content.
type Content interface {
	// GetByID gets content by ID.
	GetByID(
		ctx context.Context,
		contentID uuid.UUID,
		attributes types.IPXESelectors,
	) ([]byte, error)
}

// --------------------------------------------------- CONSTRUCTORS ------------------------------------------------- //

// NewContent returns a new Content.
func NewContent(profile adapter.Profile, mux ResolveTransformerMux) Content {
	return &content{
		profile: profile,
		mux:     mux,
	}
}

// ---------------------------------------------------- CONTENT ----------------------------------------------------- //

type content struct {
	profile adapter.Profile
	mux     ResolveTransformerMux
}

func (c *content) GetByID(
	ctx context.Context,
	contentID uuid.UUID,
	attributes types.IPXESelectors,
) ([]byte, error) {
	if contentID == uuid.Nil {
		return nil, errors.Join(errUUIDCannotBeNil, ErrContentGetById)
	}

	list, err := c.profile.ListByContentID(ctx, contentID)
	if errors.Is(err, adapter.ErrProfileNotFound) || len(list) == 0 {
		return nil, errors.Join(err, ErrContentNotFound, ErrContentGetById)
	}

	contentName := list[0].ContentIDToNameMap[contentID]
	cont := list[0].AdditionalContent[contentName]
	// NB: mux.ResolveAndTransform will always render the content. Please call ResolveAndTransformBatch
	// with the mux.ReturnExposedContentURL option to return a URL instead.
	out, err := c.mux.ResolveAndTransform(ctx, cont, types.IPXESelectors{
		UUID:      contentID, // the contentID takes precedence, thus should always overwrite the attribute uuid.
		Buildarch: attributes.Buildarch,
	})
	if err != nil {
		return nil, errors.Join(err, ErrContentGetById)
	}

	// Determine content type from resolver kind
	contentType := "unknown"
	switch cont.ResolverKind {
	case types.InlineResolverKind:
		contentType = "inline"
	case types.ObjectRefResolverKind:
		contentType = "objectref"
	case types.WebhookResolverKind:
		contentType = "webhook"
	}

	// Log config retrieval
	slog.InfoContext(ctx, "config_retrieved",
		"config_uuid", contentID.String(),
		"content_type", contentType,
		"size_bytes", len(out),
	)

	return out, nil
}
