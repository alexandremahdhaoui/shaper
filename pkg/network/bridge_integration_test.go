//go:build integration

package network_test

import (
	"context"
	"errors"
	"testing"

	"github.com/alexandremahdhaoui/shaper/pkg/execcontext"
	"github.com/alexandremahdhaoui/shaper/pkg/network"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Integration tests for bridge management

func TestCreateBridge_Integration(t *testing.T) {
	// Create manager with sudo context
	execCtx := execcontext.New(nil, []string{"sudo"})
	mgr := network.NewBridgeManager(execCtx)
	ctx := context.Background()

	// Linux interface names limited to 15 chars
	bridgeName := "br" + uuid.NewString()[:6]
	config := network.BridgeConfig{
		Name: bridgeName,
		CIDR: "192.168.200.1/24",
	}

	// Create bridge
	err := mgr.Create(ctx, config)
	require.NoError(t, err)
	defer mgr.Delete(ctx, bridgeName)

	// Verify bridge exists using Get
	info, err := mgr.Get(ctx, bridgeName)
	require.NoError(t, err)
	require.NotNil(t, info)
	require.Equal(t, bridgeName, info.Name)
}

func TestCreateBridge_Idempotent_Integration(t *testing.T) {
	// Create manager with sudo context
	execCtx := execcontext.New(nil, []string{"sudo"})
	mgr := network.NewBridgeManager(execCtx)
	ctx := context.Background()

	// Linux interface names limited to 15 chars
	bridgeName := "br" + uuid.NewString()[:6]
	config := network.BridgeConfig{
		Name: bridgeName,
		CIDR: "192.168.201.1/24",
	}

	// Create bridge first time
	err := mgr.Create(ctx, config)
	require.NoError(t, err)
	defer mgr.Delete(ctx, bridgeName)

	// Create bridge second time - should not error (idempotent)
	err = mgr.Create(ctx, config)
	require.NoError(t, err)

	// Verify bridge still exists
	info, err := mgr.Get(ctx, bridgeName)
	require.NoError(t, err)
	require.NotNil(t, info)
}

func TestDeleteBridge_Integration(t *testing.T) {
	// Create manager with sudo context
	execCtx := execcontext.New(nil, []string{"sudo"})
	mgr := network.NewBridgeManager(execCtx)
	ctx := context.Background()

	// Linux interface names limited to 15 chars
	bridgeName := "br" + uuid.NewString()[:6]
	config := network.BridgeConfig{
		Name: bridgeName,
		CIDR: "192.168.202.1/24",
	}

	// Create bridge
	err := mgr.Create(ctx, config)
	require.NoError(t, err)

	// Verify it exists
	info, err := mgr.Get(ctx, bridgeName)
	require.NoError(t, err)
	require.NotNil(t, info)

	// Delete bridge
	err = mgr.Delete(ctx, bridgeName)
	require.NoError(t, err)

	// Verify it's gone using Get (should return ErrBridgeNotFound)
	_, err = mgr.Get(ctx, bridgeName)
	require.Error(t, err)
	require.True(t, errors.Is(err, network.ErrBridgeNotFound))
}

func TestBridgeGet_Integration(t *testing.T) {
	// Create manager with sudo context
	execCtx := execcontext.New(nil, []string{"sudo"})
	mgr := network.NewBridgeManager(execCtx)
	ctx := context.Background()

	// Linux interface names limited to 15 chars
	bridgeName := "br" + uuid.NewString()[:6]
	config := network.BridgeConfig{
		Name: bridgeName,
		CIDR: "192.168.203.1/24",
	}

	// Before creation - should not exist (ErrBridgeNotFound)
	_, err := mgr.Get(ctx, bridgeName)
	require.Error(t, err)
	require.True(t, errors.Is(err, network.ErrBridgeNotFound))

	// Create bridge
	err = mgr.Create(ctx, config)
	require.NoError(t, err)
	defer mgr.Delete(ctx, bridgeName)

	// After creation - should exist and return info
	info, err := mgr.Get(ctx, bridgeName)
	require.NoError(t, err)
	require.NotNil(t, info)
	require.Equal(t, bridgeName, info.Name)
	require.Equal(t, config.CIDR, info.CIDR)
}
