/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package hooks

import (
	"context"
	"sync/atomic"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
)

// ZeroHooks abstracts the Zero-coordinated operations the worker depends on:
// UID/namespace/timestamp assignment, transaction commit, and mutation application.
// The default implementation talks to a Zero cluster over the network; embedded
// deployments supply their own via Config.ZeroHooks.
//
// The worker entry points set pb.Num.Type before calling AssignUIDs and AssignNsIDs,
// so implementations receive a fully-typed request and need not set it themselves.
type ZeroHooks interface {
	AssignUIDs(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error)
	AssignTimestamps(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error)
	AssignNsIDs(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error)
	CommitOrAbort(ctx context.Context, tc *api.TxnContext) (*api.TxnContext, error)
	ApplyMutations(ctx context.Context, m *pb.Mutations) (*api.TxnContext, error)
}

// ZeroHooksFns adapts a set of plain functions into a ZeroHooks implementation,
// letting callers override individual operations without declaring a new type.
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
	// ZeroHooks overrides the Zero operations the worker performs. When nil, the
	// default network-backed hooks registered by the worker package are used.
	ZeroHooks ZeroHooks
}

// defaultHooksHolder wraps a ZeroHooks so it can live in an atomic.Pointer.
// An interface can't be stored in atomic.Pointer directly, and atomic.Value would
// panic if two implementations with different concrete types were ever registered.
type defaultHooksHolder struct {
	hooks ZeroHooks
}

var (
	// globalConfig holds the active embedded configuration. A nil value means
	// embedded mode is off, so embedded state is derived from this single pointer.
	globalConfig atomic.Pointer[Config]

	// defaultZeroHooks holds the fallback hooks registered by the worker package.
	defaultZeroHooks atomic.Pointer[defaultHooksHolder]
)

// Enable activates embedded mode with the given configuration.
// This must be called before any Dgraph operations.
func Enable(cfg *Config) {
	globalConfig.Store(cfg)
}

// Disable deactivates embedded mode.
func Disable() {
	globalConfig.Store(nil)
}

// IsEnabled returns true if embedded mode is currently active.
func IsEnabled() bool {
	return globalConfig.Load() != nil
}

// GetConfig returns the current embedded configuration, or nil if not enabled.
func GetConfig() *Config {
	return globalConfig.Load()
}

// SetDefaultZeroHooks registers the fallback ZeroHooks used when embedded mode is
// disabled or Config.ZeroHooks is nil. The worker package calls this from its init.
func SetDefaultZeroHooks(h ZeroHooks) {
	defaultZeroHooks.Store(&defaultHooksHolder{hooks: h})
}

// GetHooks returns the active Zero hooks.
// If embedded mode is not enabled, it returns the default hooks implementation.
func GetHooks() ZeroHooks {
	if cfg := globalConfig.Load(); cfg != nil && cfg.ZeroHooks != nil {
		return cfg.ZeroHooks
	}
	if h := defaultZeroHooks.Load(); h != nil {
		return h.hooks
	}
	panic("no ZeroHooks configured - ensure worker package is imported or hooks.Enable() is called")
}
