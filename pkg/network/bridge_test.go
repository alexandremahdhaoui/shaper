//go:build unit

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

package network

import (
	"context"
	"errors"
	"testing"

	"github.com/alexandremahdhaoui/shaper/pkg/execcontext"
)

func TestNewBridgeManager(t *testing.T) {
	tests := []struct {
		name    string
		execCtx execcontext.Context
	}{
		{
			name:    "with nil context",
			execCtx: execcontext.New(nil, nil),
		},
		{
			name:    "with sudo context",
			execCtx: execcontext.New(nil, []string{"sudo"}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewBridgeManager(tt.execCtx)
			if mgr == nil {
				t.Fatal("expected non-nil manager")
			}
			if mgr.execCtx == nil {
				t.Fatal("expected non-nil execCtx")
			}
		})
	}
}

func TestBridgeManager_Create_ValidationErrors(t *testing.T) {
	mgr := NewBridgeManager(execcontext.New(nil, nil))
	ctx := context.Background()

	tests := []struct {
		name        string
		config      BridgeConfig
		expectedErr error
	}{
		{
			name: "empty bridge name",
			config: BridgeConfig{
				Name: "",
				CIDR: "192.168.100.1/24",
			},
			expectedErr: ErrBridgeNameRequired,
		},
		{
			name: "empty CIDR",
			config: BridgeConfig{
				Name: "br0",
				CIDR: "",
			},
			expectedErr: ErrCIDRRequired,
		},
		{
			name: "both empty",
			config: BridgeConfig{
				Name: "",
				CIDR: "",
			},
			expectedErr: ErrBridgeNameRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mgr.Create(ctx, tt.config)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, tt.expectedErr) {
				t.Errorf("expected error %v, got %v", tt.expectedErr, err)
			}
		})
	}
}

func TestBridgeManager_Get_ValidationErrors(t *testing.T) {
	mgr := NewBridgeManager(execcontext.New(nil, nil))
	ctx := context.Background()

	tests := []struct {
		name        string
		bridgeName  string
		expectedErr error
	}{
		{
			name:        "empty bridge name",
			bridgeName:  "",
			expectedErr: ErrBridgeNameRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := mgr.Get(ctx, tt.bridgeName)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, tt.expectedErr) {
				t.Errorf("expected error %v, got %v", tt.expectedErr, err)
			}
		})
	}
}

func TestBridgeManager_Get_NotFound_ErrorCheck(t *testing.T) {
	// This test verifies that errors.Is works with ErrBridgeNotFound
	// We can't actually test Get without mocking or running as root,
	// but we can verify the error type is usable with errors.Is
	err := ErrBridgeNotFound
	if !errors.Is(err, ErrBridgeNotFound) {
		t.Fatal("errors.Is should work with ErrBridgeNotFound")
	}
}

func TestBridgeManager_Delete_ValidationErrors(t *testing.T) {
	mgr := NewBridgeManager(execcontext.New(nil, nil))
	ctx := context.Background()

	tests := []struct {
		name        string
		bridgeName  string
		expectedErr error
	}{
		{
			name:        "empty bridge name",
			bridgeName:  "",
			expectedErr: ErrBridgeNameRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mgr.Delete(ctx, tt.bridgeName)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, tt.expectedErr) {
				t.Errorf("expected error %v, got %v", tt.expectedErr, err)
			}
		})
	}
}
