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
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"text/template"

	"github.com/alexandremahdhaoui/shaper/internal/adapter"
	"github.com/alexandremahdhaoui/shaper/internal/types"
)

var (
	ErrIPXEFindProfileAndRender = errors.New("finding and rendering ipxe profile")

	errFallbackToDefaultAssignment = errors.New("fallback to default assignment")
	errSelectingAssignment         = errors.New("selecting assignment")
	errTemplatingIPXEProfile       = errors.New("templating ipxe profile")

	fmtCannotSelectAssignmentWithSelectors = "cannot select assignment with selectors: uuid=%q & buildarch=%q"
)

// ---------------------------------------------------- INTERFACES -------------------------------------------------- //

// IPXE is an interface for finding and rendering iPXE profiles.
type IPXE interface {
	// FindProfileAndRender finds a profile and renders it.
	FindProfileAndRender(ctx context.Context, selectors types.IPXESelectors) ([]byte, error)
	// Boostrap returns the iPXE bootstrap script.
	Boostrap() []byte
}

// --------------------------------------------------- CONSTRUCTORS ------------------------------------------------- //

// NewIPXE returns a new IPXE.
func NewIPXE(
	assignment adapter.Assignment,
	profile adapter.Profile,
	mux ResolveTransformerMux,
) IPXE {
	return &ipxe{
		assignment: assignment,
		profile:    profile,
		mux:        mux,
	}
}

// -------------------------------------------------------- IPXE ---------------------------------------------------- //

type ipxe struct {
	assignment adapter.Assignment
	profile    adapter.Profile
	mux        ResolveTransformerMux

	cachedBootstrap []byte
}

// -------------------------------------------------------- FindProfileAndRender ------------------------------------ //

func (i *ipxe) FindProfileAndRender(
	ctx context.Context,
	selectors types.IPXESelectors,
) ([]byte, error) {
	assignment, err := i.assignment.FindBySelectors(ctx, selectors)
	matchedBy := "uuid"
	if errors.Is(err, adapter.ErrAssignmentNotFound) {
		// fallback to default profile
		defaultAssignment, defaultErr := i.assignment.FindDefaultByBuildarch(
			ctx,
			selectors.Buildarch,
		)
		if defaultErr != nil {
			return nil, errors.Join(
				defaultErr,
				fmt.Errorf(
					fmtCannotSelectAssignmentWithSelectors,
					selectors.UUID,
					selectors.Buildarch,
				),
				errFallbackToDefaultAssignment,
				errSelectingAssignment,
				ErrIPXEFindProfileAndRender,
			)
		}

		assignment = defaultAssignment
		matchedBy = "default"
	} else if err != nil {
		return nil, errors.Join(err, errSelectingAssignment, ErrIPXEFindProfileAndRender)
	}

	// Log assignment selection
	slog.InfoContext(ctx, "assignment_selected",
		"assignment_name", assignment.Name,
		"assignment_namespace", assignment.Namespace,
		"subject_selectors", assignment.SubjectSelectors,
		"matched_by", matchedBy,
	)

	p, err := i.profile.Get(ctx, assignment.ProfileName)
	if err != nil {
		return nil, errors.Join(err, ErrIPXEFindProfileAndRender)
	}

	// Log profile match
	slog.InfoContext(ctx, "profile_matched",
		"profile_name", p.Name,
		"profile_namespace", p.Namespace,
		"assignment", assignment.Name,
	)

	data, err := i.mux.ResolveAndTransformBatch(
		ctx,
		p.AdditionalContent,
		selectors,
		ReturnExposedContentURL,
	)
	if err != nil {
		return nil, errors.Join(err, ErrIPXEFindProfileAndRender)
	}

	out, err := templateIPXEProfile(p.IPXETemplate, data)
	if err != nil {
		return nil, errors.Join(err, ErrIPXEFindProfileAndRender)
	}

	return out, nil
}

func templateIPXEProfile(ipxeTemplate string, data map[string][]byte) ([]byte, error) {
	tpl, err := template.New("").Parse(ipxeTemplate)
	if err != nil {
		return nil, errors.Join(err, errTemplatingIPXEProfile)
	}

	stringData := make(map[string]string)
	for k, v := range data {
		stringData[k] = string(v)
	}

	buf := bytes.NewBuffer(make([]byte, 0))
	if err := tpl.Execute(buf, stringData); err != nil {
		return nil, errors.Join(err, errTemplatingIPXEProfile)
	}

	return buf.Bytes(), nil
}

// -------------------------------------------------------- Bootstrap ----------------------------------------------- //

func (i *ipxe) Boostrap() []byte {
	if len(i.cachedBootstrap) > 0 {
		return bytes.Clone(i.cachedBootstrap)
	}

	// init boostrap
	params := ""
	for _, param := range orderedAllowedParamKeys {
		paramType := allowedParamsWithType[param]
		if params != "" {
			params = fmt.Sprintf("%s&", params)
		}

		if paramType == none {
			params = fmt.Sprintf("%s%s=${%s}", params, param, param)
			continue
		}

		params = fmt.Sprintf("%s%s=${%s:%s}", params, param, param, paramType)
	}

	i.cachedBootstrap = []byte(fmt.Sprintf(ipxeBootstrapFormat, params))

	return bytes.Clone(i.cachedBootstrap)
}

// TODO: mac should be `NETWORK_IFACE/mac`.

const (
	// #!ipxe
	// chain ipxe?uuid=${uuid}&mac=${mac:hexhyp}&domain=${domain}&hostname=${hostname}&serial=${serial}&arch=${buildarch:uristring}
	ipxeBootstrapFormat = `#!ipxe
chain ipxe?%s
`
	none      ipxeParamType = ""
	uriString ipxeParamType = "uristring"
)

type ipxeParamType string

var (
	orderedAllowedParamKeys = []string{
		types.Uuid,
		types.Buildarch,
	}

	allowedParamsWithType = map[string]ipxeParamType{
		// types.Mac,
		// types.BusType,
		// types.BusLoc,
		// types.BusID,
		// types.Chip,
		// types.Ssid,
		// types.ActiveScan,
		// types.Key,

		// IPv4 settings

		// types.Ip,
		// types.Netmask,
		// types.Gateway,
		// types.Dns,
		// types.Domain,

		// Boot settings

		// types.Filename,
		// types.NextServer,
		// types.RootPath,
		// types.SanFilename,
		// types.InitiatorIqn,
		// types.KeepSan,
		// types.SkipSanBoot,

		// Host settings

		// types.Hostname,
		types.Uuid: none,
		// types.UserClass,
		// types.Manufacturer,
		// types.Product,
		// types.Serial,
		// types.Asset,

		// Authentication settings

		// types.Username,
		// types.Password,
		// types.ReverseUsername,
		// types.ReversePassword,

		// Cryptography settings

		// types.Crosscert,
		// types.Trust,
		// types.Cert,
		// types.Privkey,

		// Miscellaneous settings

		types.Buildarch: uriString,
		// types.Cpumodel,
		// types.Cpuvendor,
		// types.DhcpServer,
		// types.Keymap,
		// types.Memsize,
		// types.Platform,
		// types.Priority,
		// types.Scriptlet,
		// types.Syslog,
		// types.Syslogs,
		// types.Sysmac,
		// types.Unixtime,
		// types.UseCached,
		// types.Version,
		// types.Vram,
	}
)
