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

package server

import (
	"context"
	"errors"

	"github.com/alexandremahdhaoui/shaper/internal/controller"
	"github.com/alexandremahdhaoui/shaper/internal/types"
	"github.com/alexandremahdhaoui/shaper/pkg/generated/shaperserver"
)

var (
	ErrGetConfigByID      = errors.New("getting config by id")
	ErrGetIPXEBySelectors = errors.New("getting ipxe by labels")
)

// New returns a new server.
func New(ipxe controller.IPXE, config controller.Content) shaperserver.StrictServerInterface {
	return &server{
		ipxe:   ipxe,
		config: config,
	}
}

type server struct {
	ipxe   controller.IPXE
	config controller.Content
}

func (s *server) GetIPXEBootstrap(
	_ context.Context,
	_ shaperserver.GetIPXEBootstrapRequestObject,
) (shaperserver.GetIPXEBootstrapResponseObject, error) {
	// call controller
	b := s.ipxe.Boostrap()

	return shaperserver.GetIPXEBootstrap200TextResponse(b), nil
}

func (s *server) GetContentByID(
	ctx context.Context,
	request shaperserver.GetContentByIDRequestObject,
) (shaperserver.GetContentByIDResponseObject, error) {
	// TODO: instantiate child context with correlation ID.

	attributes := types.IPXESelectors{
		Buildarch: string(request.Params.Buildarch),
		UUID:      request.Params.Uuid,
	}

	// call controller
	b, err := s.config.GetByID(ctx, request.ContentID, attributes)
	if err != nil {
		return shaperserver.GetContentByID500JSONResponse{
			N500JSONResponse: shaperserver.N500JSONResponse{
				Code:    500,
				Message: errors.Join(err, ErrGetConfigByID).Error(),
			},
		}, nil
	}

	return shaperserver.GetContentByID200TextResponse(b), nil
}

func (s *server) GetIPXEBySelectors(
	ctx context.Context,
	request shaperserver.GetIPXEBySelectorsRequestObject,
) (shaperserver.GetIPXEBySelectorsResponseObject, error) {
	// TODO: create new context with correlation ID.

	// convert into type
	// TODO: use params instead of converting the echo context?
	selectors := types.IPXESelectors{
		Buildarch: string(request.Params.Buildarch),
		UUID:      request.Params.Uuid,
	}

	// call controller
	b, err := s.ipxe.FindProfileAndRender(ctx, selectors)
	if err != nil {
		return shaperserver.GetIPXEBySelectors500JSONResponse{
			N500JSONResponse: shaperserver.N500JSONResponse{
				Code:    0,
				Message: errors.Join(err, ErrGetIPXEBySelectors).Error(),
			},
		}, nil
	}

	return shaperserver.GetIPXEBySelectors200TextResponse(b), nil
}
