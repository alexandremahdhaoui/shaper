//go:build unit

package network

import (
	"context"
	"errors"
	"testing"
)

func TestNewLibvirtNetworkManager(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "with nil connection",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewLibvirtNetworkManager(nil)
			if mgr == nil {
				t.Fatal("expected non-nil manager")
			}
		})
	}
}

func TestLibvirtNetworkManager_Create_ValidationErrors(t *testing.T) {
	mgr := NewLibvirtNetworkManager(nil)
	ctx := context.Background()

	tests := []struct {
		name        string
		config      LibvirtNetworkConfig
		expectedErr error
	}{
		{
			name: "nil connection with valid name",
			config: LibvirtNetworkConfig{
				Name: "test",
			},
			expectedErr: ErrConnNil,
		},
		{
			name:        "empty network name (conn also nil, but checks conn first)",
			config:      LibvirtNetworkConfig{},
			expectedErr: ErrConnNil, // conn is checked before name
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

func TestLibvirtNetworkManager_Get_ValidationErrors(t *testing.T) {
	mgr := NewLibvirtNetworkManager(nil)
	ctx := context.Background()

	tests := []struct {
		name        string
		networkName string
		expectedErr error
	}{
		{
			name:        "empty network name",
			networkName: "",
			expectedErr: ErrNetworkNameRequired,
		},
		{
			name:        "nil connection",
			networkName: "test",
			expectedErr: ErrConnNil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := mgr.Get(ctx, tt.networkName)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, tt.expectedErr) {
				t.Errorf("expected error %v, got %v", tt.expectedErr, err)
			}
		})
	}
}

func TestLibvirtNetworkManager_Get_NotFound_ErrorCheck(t *testing.T) {
	// This test verifies that errors.Is works with ErrNetworkNotFound
	err := ErrNetworkNotFound
	if !errors.Is(err, ErrNetworkNotFound) {
		t.Fatal("errors.Is should work with ErrNetworkNotFound")
	}
}

func TestLibvirtNetworkManager_Delete_ValidationErrors(t *testing.T) {
	mgr := NewLibvirtNetworkManager(nil)
	ctx := context.Background()

	tests := []struct {
		name        string
		networkName string
		expectedErr error
	}{
		{
			name:        "empty network name",
			networkName: "",
			expectedErr: ErrNetworkNameRequired,
		},
		{
			name:        "nil connection",
			networkName: "test",
			expectedErr: ErrConnNil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mgr.Delete(ctx, tt.networkName)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, tt.expectedErr) {
				t.Errorf("expected error %v, got %v", tt.expectedErr, err)
			}
		})
	}
}
