/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package hooks

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/stretchr/testify/require"
)

// mockZeroHooks is a test implementation of ZeroHooks
type mockZeroHooks struct {
	assignUIDsCalled       bool
	assignTimestampsCalled bool
	assignNsIDsCalled      bool
	commitOrAbortCalled    bool
	applyMutationsCalled   bool
}

func (m *mockZeroHooks) AssignUIDs(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error) {
	m.assignUIDsCalled = true
	return &pb.AssignedIds{StartId: 1, EndId: 10}, nil
}

func (m *mockZeroHooks) AssignTimestamps(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error) {
	m.assignTimestampsCalled = true
	return &pb.AssignedIds{StartId: 100}, nil
}

func (m *mockZeroHooks) AssignNsIDs(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error) {
	m.assignNsIDsCalled = true
	return &pb.AssignedIds{StartId: 1}, nil
}

func (m *mockZeroHooks) CommitOrAbort(ctx context.Context, tc *api.TxnContext) (*api.TxnContext, error) {
	m.commitOrAbortCalled = true
	return &api.TxnContext{CommitTs: 200}, nil
}

func (m *mockZeroHooks) ApplyMutations(ctx context.Context, mut *pb.Mutations) (*api.TxnContext, error) {
	m.applyMutationsCalled = true
	return &api.TxnContext{}, nil
}

func resetGlobalState() {
	mu.Lock()
	defer mu.Unlock()
	enabled.Store(false)
	globalConfig.Store(nil)
	// Store a true nil by using a new atomic.Value (zero value has nil)
	defaultZeroHooks = atomic.Value{}
}

func TestEnableDisable(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	require.False(t, IsEnabled(), "should start disabled")
	require.Nil(t, GetConfig(), "config should be nil when disabled")

	cfg := &Config{
		DataDir:     "/tmp/test",
		CacheSizeMB: 128,
	}
	Enable(cfg)

	require.True(t, IsEnabled(), "should be enabled after Enable()")
	require.NotNil(t, GetConfig(), "config should not be nil after Enable()")
	require.Equal(t, "/tmp/test", GetConfig().DataDir)
	require.Equal(t, int64(128), GetConfig().CacheSizeMB)

	Disable()

	require.False(t, IsEnabled(), "should be disabled after Disable()")
	require.Nil(t, GetConfig(), "config should be nil after Disable()")
}

func TestGetHooksWithCustomConfig(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	mock := &mockZeroHooks{}
	cfg := &Config{
		ZeroHooks: mock,
	}
	Enable(cfg)

	hooks := GetHooks()
	require.NotNil(t, hooks)
	require.Equal(t, mock, hooks, "should return custom hooks from config")
}

func TestGetHooksWithDefaultHooks(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	mock := &mockZeroHooks{}
	SetDefaultZeroHooks(mock)

	hooks := GetHooks()
	require.NotNil(t, hooks)
	require.Equal(t, mock, hooks, "should return default hooks when no config")
}

func TestGetHooksPanicsWhenNoHooksConfigured(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	require.Panics(t, func() {
		GetHooks()
	}, "should panic when no hooks configured")
}

func TestGetHooksCustomOverridesDefault(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	defaultMock := &mockZeroHooks{}
	SetDefaultZeroHooks(defaultMock)

	customMock := &mockZeroHooks{}
	cfg := &Config{
		ZeroHooks: customMock,
	}
	Enable(cfg)

	hooks := GetHooks()
	require.Equal(t, customMock, hooks, "custom hooks should override default")
}

func TestZeroHooksFnsWrapper(t *testing.T) {
	var assignUIDsCalled, timestampsCalled, nsIDsCalled, commitCalled, mutationsCalled bool

	wrapper := ZeroHooksFns{
		AssignUIDsFn: func(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error) {
			assignUIDsCalled = true
			return &pb.AssignedIds{StartId: 1}, nil
		},
		AssignTimestampsFn: func(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error) {
			timestampsCalled = true
			return &pb.AssignedIds{StartId: 100}, nil
		},
		AssignNsIDsFn: func(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error) {
			nsIDsCalled = true
			return &pb.AssignedIds{StartId: 1}, nil
		},
		CommitOrAbortFn: func(ctx context.Context, tc *api.TxnContext) (*api.TxnContext, error) {
			commitCalled = true
			return &api.TxnContext{CommitTs: 200}, nil
		},
		ApplyMutationsFn: func(ctx context.Context, m *pb.Mutations) (*api.TxnContext, error) {
			mutationsCalled = true
			return &api.TxnContext{}, nil
		},
	}

	ctx := context.Background()

	_, _ = wrapper.AssignUIDs(ctx, &pb.Num{})
	require.True(t, assignUIDsCalled, "AssignUIDs should delegate to function")

	_, _ = wrapper.AssignTimestamps(ctx, &pb.Num{})
	require.True(t, timestampsCalled, "AssignTimestamps should delegate to function")

	_, _ = wrapper.AssignNsIDs(ctx, &pb.Num{})
	require.True(t, nsIDsCalled, "AssignNsIDs should delegate to function")

	_, _ = wrapper.CommitOrAbort(ctx, &api.TxnContext{})
	require.True(t, commitCalled, "CommitOrAbort should delegate to function")

	_, _ = wrapper.ApplyMutations(ctx, &pb.Mutations{})
	require.True(t, mutationsCalled, "ApplyMutations should delegate to function")
}

func TestConcurrentEnableDisable(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	var wg sync.WaitGroup
	iterations := 100

	// Concurrent enables
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cfg := &Config{
				DataDir:     "/tmp/test",
				CacheSizeMB: int64(i),
			}
			Enable(cfg)
		}(i)
	}

	// Concurrent disables
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			Disable()
		}()
	}

	// Concurrent reads
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = IsEnabled()
			_ = GetConfig()
		}()
	}

	wg.Wait()
	// Test passes if no race conditions detected
}

func TestSetDefaultZeroHooks(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	mock1 := &mockZeroHooks{}
	mock2 := &mockZeroHooks{}

	SetDefaultZeroHooks(mock1)
	require.Equal(t, mock1, GetHooks())

	SetDefaultZeroHooks(mock2)
	require.Equal(t, mock2, GetHooks())
}
