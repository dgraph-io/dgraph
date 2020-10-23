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

	"github.com/spf13/viper"
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
	// MutationsNQuadLimit is maximum number of nquads that can be present in a single
	// mutation request.
	MutationsNQuadLimit int
	// PollInterval is the polling interval for graphql subscription.
	PollInterval time.Duration
	// GraphqlExtension will be set to see extensions in graphql results
	GraphqlExtension bool
	// GraphqlDebug will enable debug mode in GraphQL
	GraphqlDebug bool
	// GraphqlLambdaUrl stores the URL of lambda functions for custom GraphQL resolvers
	GraphqlLambdaUrl string
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
	// ZeroAddr stores the list of address:port for the zero instances associated with this alpha.
	// Alpha would communicate via only one zero address from the list. All
	// the other addresses serve as fallback.
	ZeroAddr []string
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
	// ProposedGroupId will be used if there's a file in the p directory called group_id with the
	// proposed group ID for this server.
	ProposedGroupId uint32
	// StartTime is the start time of the alpha
	StartTime time.Time
	// LudicrousMode is super fast mode with fewer guarantees.
	LudicrousMode bool
	// Number of mutations that can be run together in ludicrous mode
	LudicrousConcurrency int
	// EncryptionKey is the key used for encryption at rest, backups, exports. Enterprise only feature.
	EncryptionKey SensitiveByteSlice
	// LogRequest indicates whether alpha should log all query/mutation requests coming to it.
	// Ideally LogRequest should be a bool value. But we are reading it using atomics across
	// queries hence it has been kept as int32. LogRequest value 1 enables logging of requests
	// coming to alphas and 0 disables it.
	LogRequest int32
	// If true, we should call msync or fsync after every write to survive hard reboots.
	HardSync bool
}

// WorkerConfig stores the global instance of the worker package's options.
var WorkerConfig WorkerOptions

func (w *WorkerOptions) Parse(conf *viper.Viper) {
	w.MyAddr = conf.GetString("my")
	w.Tracing = conf.GetFloat64("trace")

	if w.LudicrousMode {
		w.HardSync = false

	} else {
		survive := conf.GetString("survive")
		AssertTruef(survive == "process" || survive == "filesystem",
			"Invalid survival mode: %s", survive)
		w.HardSync = survive == "filesystem"
	}
}
