/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"crypto/tls"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/spf13/viper"

	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/ristretto/v2/z"
)

// Options stores the options for this package.
type Options struct {
	// PortOffset will be used to determine the ports to use (port = default port + offset).
	PortOffset int
	// Limit options:
	//
	// query-edge uint64 - maximum number of edges that can be returned in a query
	// normalize-node int - maximum number of nodes that can be returned in a query that uses the
	//                      normalize directive
	// mutations-nquad int - maximum number of nquads that can be inserted in a mutation request
	// BlockDropAll bool - if set to true, the drop all operation will be rejected by the server.
	// query-timeout duration - Maximum time after which a query execution will fail.
	// max-retries int64 - maximum number of retries made by dgraph to commit a transaction to disk.
	// shared-instance bool - if set to true, ACLs will be disabled for non-galaxy users.
	Limit                *z.SuperFlag
	LimitMutationsNquad  int
	LimitQueryEdge       uint64
	BlockClusterWideDrop bool
	LimitNormalizeNode   int
	QueryTimeout         time.Duration
	MaxRetries           int64
	SharedInstance       bool

	// GraphQL options:
	//
	// extensions bool - Will be set to see extensions in GraphQL results
	// debug bool - Will enable debug mode in GraphQL.
	// lambda-url string - Stores the URL of lambda functions for custom GraphQL resolvers
	// 			The configured lambda-url can have a parameter `$ns`,
	//			which should be replaced with the correct namespace value at runtime.
	// 	===========================================================================================
	// 	|                lambda-url                | $ns |           namespacedLambdaUrl          |
	// 	|==========================================|=====|========================================|
	// 	| http://localhost:8686/graphql-worker/$ns |  1  | http://localhost:8686/graphql-worker/1 |
	// 	| http://localhost:8686/graphql-worker     |  1  | http://localhost:8686/graphql-worker   |
	// 	|=========================================================================================|
	//
	// poll-interval duration - The polling interval for graphql subscription.
	GraphQL      *z.SuperFlag
	GraphQLDebug bool

	// feature flags
	NormalizeCompatibilityMode string
}

// Config stores the global instance of this package's options.
var Config Options

// IPRange represents an IP range.
type IPRange struct {
	Lower, Upper net.IP
}

// WorkerOptions stores the options for the worker package. It's declared here
// since it's used by multiple packages. Note the String override for this type,
// added to prevent sensitive information from being logged.
type WorkerOptions struct {
	// TmpDir is a directory to store temporary buffers.
	TmpDir string
	// ExportPath indicates the folder to which exported data will be saved.
	ExportPath string
	// Trace options:
	//
	// ratio float64 - the ratio of queries to trace (must be between 0 and 1)
	// jaeger string - URL of Jaeger to send OpenCensus traces
	// datadog string - URL of Datadog to send OpenCensus traces
	Trace *z.SuperFlag
	// MyAddr stores the address and port for this alpha.
	MyAddr string
	// ZeroAddr stores the list of address:port for the zero instances associated with this alpha.
	// Alpha would communicate via only one zero address from the list. All
	// the other addresses serve as fallback.
	ZeroAddr []string
	// TLS client config which will be used to connect with zero and alpha internally
	TLSClientConfig *tls.Config
	// TLS server config which will be used to initiate server internal port
	TLSServerConfig *tls.Config
	// Raft stores options related to Raft.
	Raft *z.SuperFlag
	// Badger stores the badger options.
	Badger badger.Options
	// WhiteListedIPRanges is a list of IP ranges from which requests will be allowed.
	WhiteListedIPRanges []IPRange
	// StrictMutations will cause mutations to unknown predicates to fail if set to true.
	StrictMutations bool
	// AclEnabled indicates whether the ACL feature is turned on.
	AclEnabled bool
	// AclJwtAlg stores the JWT signing algorithm.
	AclJwtAlg jwt.SigningMethod
	// AclPublicKey stores the public key used to verify JSON Web Tokens (JWT).
	// It could be a either a RSA or ECDSA PublicKey or HMAC symmetric key.
	// depending upon the JWT signing algorithm. Note that for symmetric algorithms,
	// this will contain the same key as the private key, needs to be used carefully.
	AclPublicKey interface{}
	// AbortOlderThan tells Dgraph to discard transactions that are older than this duration.
	AbortOlderThan time.Duration
	// ProposedGroupId will be used if there's a file in the p directory called group_id with the
	// proposed group ID for this server.
	ProposedGroupId uint32
	// StartTime is the start time of the alpha
	StartTime time.Time
	// Security options:
	//
	// whitelist string - comma separated IP addresses
	// token string - if set, all Admin requests to Dgraph will have this token.
	Security *z.SuperFlag
	// EncryptionKey is the key used for encryption at rest, backups, exports.
	EncryptionKey Sensitive
	// LogDQLRequest indicates whether alpha should log all query/mutation requests coming to it.
	// Ideally LogDQLRequest should be a bool value. But we are reading it using atomics across
	// queries hence it has been kept as int32. LogDQLRequest value 1 enables logging of requests
	// coming to alphas and 0 disables it.
	LogDQLRequest int32
	// SlowQueryLogThreshold is the duration after which a query is considered slow and logged
	// with structured fields including trace ID. Zero disables slow query logging.
	SlowQueryLogThreshold time.Duration
	// If true, we should call msync or fsync after every write to survive hard reboots.
	HardSync bool
	// Audit contains the audit flags that enables the audit.
	Audit bool
	// IntraMutationMinEdges gates the parallel mutation path in applyMutations: a
	// mutation takes it only when IntraMutationMinEdges > 0 and len(m.Edges) >=
	// it. 0 disables that path entirely and is the single kill switch for this
	// whole mechanism; 1 (the default) always takes it. A value above 1 keeps
	// small interactive mutations on the legacy path, which is worth doing only
	// if per-predicate goroutine spin-up measurably hurts them (crossover ~100
	// edges in benchmarks here).
	// Superflag key: "intra-mutation-min-edges".
	IntraMutationMinEdges int
	// IntraMutationParallelism sizes the pool of worker goroutines used inside a
	// single mutation. The pool is shared across the predicates that mutation
	// touches and apportioned by edge count, so a hot predicate is granted most
	// of it and tiny predicates get one worker each. Always further capped by
	// IntraMutationEdgesPerWorker.
	//
	// Note this parallelizes ONE mutation. Transactions still apply serially in
	// processApplyCh, so raising it does not relieve a many-concurrent-writers
	// bottleneck.
	// Superflag key: "intra-mutation-parallelism"; default "auto".
	IntraMutationParallelism IntraMutationParallelism
	// IntraMutationEdgesPerWorker is the minimum number of edges a worker should
	// have to justify existing: the pool is capped at totalEdges divided by this,
	// so small mutations do not over-spawn. Mirrors x.DivideAndRule's 256-edge
	// rule.
	//
	// This cap applies to every sizing mode, not only the per-CPU one. On a large
	// box it is frequently the *binding* constraint — at the default 256 a
	// 20k-edge mutation caps at 78 workers no matter how many cores exist — so it,
	// rather than the multiplier, is the knob that matters there.
	// Superflag key: "intra-mutation-edges-per-worker"; default 256.
	IntraMutationEdgesPerWorker int
}

// IntraMutationParallelism is the parsed form of the "intra-mutation-parallelism"
// feature flag. Exactly one field is meaningful: PerCPU when it is > 0, Workers
// otherwise. The zero value means off — every predicate runs on one goroutine.
//
// Two notations share one flag because they are the same axis — how many workers
// — expressed differently: an absolute count when the operator knows the
// workload, and a multiple of the CPUs Go may use when they want it to track the
// machine. Splitting them across two flags (as the earlier
// "mutations-pipeline-goroutines" / "-goroutines-fraction" pair did) meant one
// flag's value silently decided whether the other was read at all.
type IntraMutationParallelism struct {
	// Workers is an absolute worker count. Ignored when PerCPU > 0.
	Workers int
	// PerCPU, when > 0, sizes the pool as PerCPU * GOMAXPROCS instead.
	PerCPU float64
}

// Sizing returns the requested worker count, before IntraMutationEdgesPerWorker
// is applied. 0 means off. gomaxprocs is a parameter rather than a runtime read
// so the policy stays a pure, testable function.
func (p IntraMutationParallelism) Sizing(gomaxprocs int) int {
	if p.PerCPU > 0 {
		n := int(math.Round(float64(gomaxprocs) * p.PerCPU))
		if n < 1 {
			n = 1
		}
		return n
	}
	if p.Workers < 0 {
		return 0
	}
	return p.Workers
}

// String renders the value back into the flag notation that produced it. Note
// "auto" round-trips as "1x", which is what it means.
func (p IntraMutationParallelism) String() string {
	switch {
	case p.PerCPU > 0:
		return strconv.FormatFloat(p.PerCPU, 'g', -1, 64) + "x"
	case p.Workers > 0:
		return strconv.Itoa(p.Workers)
	default:
		return "off"
	}
}

// ParseIntraMutationParallelism parses the "intra-mutation-parallelism" flag:
// "off", "auto" (an alias for "1x"), an absolute worker count like "48", or a
// per-CPU multiplier like "1.5x".
func ParseIntraMutationParallelism(s string) (IntraMutationParallelism, error) {
	var zero IntraMutationParallelism
	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "", "off":
		return zero, nil
	case "auto":
		return IntraMutationParallelism{PerCPU: 1.0}, nil
	}
	if mult, ok := strings.CutSuffix(strings.ToLower(s), "x"); ok {
		f, err := strconv.ParseFloat(strings.TrimSpace(mult), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f <= 0 {
			return zero, fmt.Errorf("intra-mutation-parallelism: invalid per-CPU "+
				"multiplier %q; want a positive number followed by 'x', e.g. \"1.5x\"", s)
		}
		return IntraMutationParallelism{PerCPU: f}, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return zero, fmt.Errorf("intra-mutation-parallelism: invalid value %q; want "+
			"\"off\", \"auto\", a worker count like \"48\", or a per-CPU multiplier "+
			"like \"1.5x\"", s)
	}
	return IntraMutationParallelism{Workers: n}, nil
}

// WorkerConfig stores the global instance of the worker package's options.
var WorkerConfig WorkerOptions

func (w *WorkerOptions) Parse(conf *viper.Viper) {
	w.MyAddr = conf.GetString("my")
	w.Trace = z.NewSuperFlag(conf.GetString("trace")).MergeAndCheckDefault(TraceDefaults)

	survive := conf.GetString("survive")
	AssertTruef(survive == "process" || survive == "filesystem",
		"Invalid survival mode: %s", survive)
	w.HardSync = survive == "filesystem"
}

// String implements the Stringer interface to redact sensitive fields when logging.
func (w WorkerOptions) String() string {
	return fmt.Sprintf("{TmpDir:%s ExportPath:%s MyAddr:%s ZeroAddr:%v Raft:%v "+
		"WhiteListedIPRanges:%v StrictMutations:%v AclEnabled:%v AclJwtAlg:%v "+
		"AclPublicKey:**** AbortOlderThan:%v ProposedGroupId:%d StartTime:%v "+
		"Security:**** EncryptionKey:**** LogDQLRequest:%d SlowQueryThreshold:%v HardSync:%v Audit:%v "+
		"IntraMutationMinEdges:%d IntraMutationParallelism:%v IntraMutationEdgesPerWorker:%d}",
		w.TmpDir, w.ExportPath, w.MyAddr, w.ZeroAddr, w.Raft,
		w.WhiteListedIPRanges, w.StrictMutations, w.AclEnabled, w.AclJwtAlg,
		w.AbortOlderThan, w.ProposedGroupId, w.StartTime,
		w.LogDQLRequest, w.SlowQueryLogThreshold, w.HardSync, w.Audit,
		w.IntraMutationMinEdges, w.IntraMutationParallelism, w.IntraMutationEdgesPerWorker)
}
