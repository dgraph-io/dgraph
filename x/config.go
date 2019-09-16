/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package x

import (
	"net"
	"time"
)

// Options stores the options for this package.
type Options struct {
	// PortOffset will be used to determine the ports to use (port = default port + offset).
	PortOffset int
	// QueryEdgeLimit is the maximum number of edges that will be traversed during
	// recurse and shortest-path queries.
	QueryEdgeLimit uint64
	// NormalizeNodeLimit is the maximum number of nodes allowed in a normalize query.
	NormalizeNodeLimit int
}

// Config stores the global instance of this package's options.
var Config Options

// IPRange represents an IP range.
type IPRange struct {
	Lower, Upper net.IP
}

// WorkerOptions stores the options for the worker package. It's declared here
// since it's used by multiple packages.
type WorkerOptions struct {
	// ExportPath indicates the folder to which exported data will be saved.
	ExportPath string
	// NumPendingProposals indicates the maximum number of pending mutation proposals.
	NumPendingProposals int
	// Tracing tells Dgraph to only sample a percentage of the traces equal to its value.
	// The value of this option must be between 0 and 1.
	// TODO: Get rid of this here.
	Tracing float64
	// MyAddr stores the address and port for this alpha.
	MyAddr string
	// ZeroAddr stores the address and port for the zero instance associated with this alpha.
	ZeroAddr string
	// RaftId represents the id of this alpha instance for participating in the RAFT
	// consensus protocol.
	RaftId uint64
	// WhiteListedIPRanges is a list of IP ranges from which requests will be allowed.
	WhiteListedIPRanges []IPRange
	// MaxRetries is the maximum number of times to retry a commit before giving up.
	MaxRetries int
	// StrictMutations will cause mutations to unknown predicates to fail if set to true.
	StrictMutations bool
	// AclEnabled indicates whether the enterprise ACL feature is turned on.
	AclEnabled bool
	// AbortOlderThan tells Dgraph to discard transactions that are older than this duration.
	AbortOlderThan time.Duration
	// SnapshotAfter indicates the number of entries in the RAFT logs that are needed
	// to allow a snapshot to be created.
	SnapshotAfter int
}

// WorkerConfig stores the global instance of the worker package's options.
var WorkerConfig WorkerOptions
