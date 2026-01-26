/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package hooks

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
)

type ZeroHooks interface {
	AssignUIDs(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error)
	AssignTimestamps(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error)
	AssignNsIDs(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error)
	CommitOrAbort(ctx context.Context, tc *api.TxnContext) (*api.TxnContext, error)
	ApplyMutations(ctx context.Context, m *pb.Mutations) (*api.TxnContext, error)
}

type ZeroHooksFns struct {
	AssignUIDsFn       func(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error)
	AssignTimestampsFn func(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error)
	AssignNsIDsFn      func(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error)
	CommitOrAbortFn    func(ctx context.Context, tc *api.TxnContext) (*api.TxnContext, error)
	ApplyMutationsFn   func(ctx context.Context, m *pb.Mutations) (*api.TxnContext, error)
}

func (h ZeroHooksFns) AssignUIDs(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error) {
	return h.AssignUIDsFn(ctx, num)
}

func (h ZeroHooksFns) AssignTimestamps(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error) {
	return h.AssignTimestampsFn(ctx, num)
}

func (h ZeroHooksFns) AssignNsIDs(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error) {
	return h.AssignNsIDsFn(ctx, num)
}

func (h ZeroHooksFns) CommitOrAbort(ctx context.Context, tc *api.TxnContext) (*api.TxnContext, error) {
	return h.CommitOrAbortFn(ctx, tc)
}

func (h ZeroHooksFns) ApplyMutations(ctx context.Context, m *pb.Mutations) (*api.TxnContext, error) {
	return h.ApplyMutationsFn(ctx, m)
}

// Config holds the configuration for embedded mode operation.
type Config struct {
	// Hooks for bypassing Zero operations
	ZeroHooks ZeroHooks

	// DataDir is the directory where data files are stored
	DataDir string

	// CacheSizeMB is the size of the in-memory cache in megabytes
	CacheSizeMB int64
}

var (
	// globalConfig holds the current embedded configuration
	globalConfig atomic.Pointer[Config]

	defaultZeroHooks atomic.Value

	// enabled tracks whether embedded mode is active
	enabled atomic.Bool

	// mu protects initialization
	mu sync.Mutex
)

// Enable activates embedded mode with the given configuration.
// This must be called before any Dgraph operations.
func Enable(cfg *Config) {
	mu.Lock()
	defer mu.Unlock()

	globalConfig.Store(cfg)
	enabled.Store(true)
}

// Disable deactivates embedded mode.
func Disable() {
	mu.Lock()
	defer mu.Unlock()

	enabled.Store(false)
	globalConfig.Store(nil)
}

// IsEnabled returns true if embedded mode is currently active.
func IsEnabled() bool {
	return enabled.Load()
}

// GetConfig returns the current embedded configuration, or nil if not enabled.
func GetConfig() *Config {
	return globalConfig.Load()
}

func SetDefaultZeroHooks(h ZeroHooks) {
	defaultZeroHooks.Store(h)
}

// GetHooks returns the active Zero hooks.
// If embedded mode is not enabled, it returns the default hooks implementation.
func GetHooks() ZeroHooks {
	cfg := globalConfig.Load()
	if cfg != nil && cfg.ZeroHooks != nil {
		return cfg.ZeroHooks
	}
	if h := defaultZeroHooks.Load(); h != nil {
		return h.(ZeroHooks)
	}
	panic("no ZeroHooks configured - ensure worker package is imported or hooks.Enable() is called")
}
