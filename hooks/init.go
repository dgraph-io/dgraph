/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package hooks

// This package provides configuration and hooks for running Dgraph in embedded mode.
// The actual initialization functions that would create import cycles are intentionally
// left to be called directly by the host application (e.g., modusGraph) using the
// individual packages (edgraph, worker, posting, schema, x).
//
// Usage:
//   1. Call hooks.Enable() with your ZeroHooks configuration
//   2. Initialize packages directly: edgraph.Init(), worker.State.InitStorage(), etc.
//   3. The hooks in this package will be called automatically by worker functions
//   4. Call hooks.Disable() when shutting down
