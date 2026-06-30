/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

// Package hooks provides configuration and hooks for running Dgraph in embedded mode.
//
// The initialization functions that would create import cycles are intentionally left
// to the host application, which wires up the individual packages (edgraph, worker,
// posting, schema, x) directly.
//
// Usage:
//  1. Call hooks.Enable() with your ZeroHooks configuration.
//  2. Initialize packages directly: edgraph.Init(), worker.State.InitStorage(), etc.
//  3. The hooks in this package are then called automatically by worker functions.
//  4. Call hooks.Disable() when shutting down.
package hooks
