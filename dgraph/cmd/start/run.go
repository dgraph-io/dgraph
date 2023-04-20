package start

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/dgraph-io/dgraph/dgraph/cmd/zero"
	"github.com/dgraph-io/dgraph/ee"
	"github.com/dgraph-io/dgraph/ee/enc"
	"github.com/dgraph-io/dgraph/graphql/admin"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	"github.com/golang/glog"
	"github.com/spf13/cobra"
	otrace "go.opencensus.io/trace"
	"golang.org/x/net/trace"
	"google.golang.org/grpc"
)

var (
	bindall bool

	// used for computing uptime
	startTime = time.Now()

	// Start is the sub-command used to start both Zero and Alpha in one process.
	Start x.SubCommand

	// need this here to refer it in admin_backup.go
	adminServer admin.IServeGraphQL
	initDone    uint32

	telemetry *z.SuperFlag
	raft *z.SuperFlag
	auditConf *x.LoggerConf
	limit *z.SuperFlag
	limitConf *x.LimiterConf
	tlsClientConf *tls.Config
)

func init() {
	Start.Cmd = &cobra.Command{
		Use:   "start",
		Short: "Run Dgraph Alpha and Zero management server ",
		Long: `
A Dgraph Zero instance manages the Dgraph cluster. A Dgraph Alpha instance stores the data.
This command merges them into
`,
		Run: func(cmd *cobra.Command, args []string) {
			
			defer x.StartProfile(Start.Conf).Stop()
			run()
		},
		Annotations: map[string]string{"group": "core"},
	}
	// TO-DO: Start.EnvPrefix = "DGRAPH_ZERO"
	Start.Cmd.SetHelpTemplate(x.NonRootTemplate)

	flag := Start.Cmd.Flags()

	x.FillCommonFlags(flag) // zero + alpha
	// --tls SuperFlag
	x.RegisterServerTLSFlags(flag) // zero + alpha
	// --encryption and --vault Superflag
	ee.RegisterAclAndEncFlags(flag) // alpha

	// NEW
	flag.String("peer", "",
		"Comma separated list of peer Dgraph addresses of the form IP_ADDRESS:PORT.")

	// ZERO
	flag.String("ZERO_wal", "zw", "Directory storing WAL.")
	flag.Int("replicas", 1, "How many Dgraph Alpha replicas to run per data shard group."+
		" The count includes the original shard.")
	flag.Duration("rebalance_interval", 8*time.Minute, "Interval for trying a predicate move.")
	flag.String("enterprise_license", "", "Path to the enterprise license file.")
	flag.String("cid", "", "Cluster ID")

	flag.String("ZERO_limit", worker.ZeroLimitsDefaults, z.NewSuperFlagHelp(worker.ZeroLimitsDefaults).
		Head("Limit options").
		Flag("uid-lease",
			`The maximum number of UIDs that can be leased by namespace (except default namespace)
			in an interval specified by refill-interval. Set it to 0 to remove limiting.`).
		Flag("refill-interval",
			"The interval after which the tokens for UID lease are replenished.").
		Flag("disable-admin-http",
			"Turn on/off the administrative endpoints exposed over Zero's HTTP port.").
		String())

	flag.String("ZERO_raft", zero.RaftDefaults, z.NewSuperFlagHelp(zero.RaftDefaults).
		Head("Raft options").
		Flag("idx",
			"Provides an optional Raft ID that this Alpha would use to join Raft groups.").
		Flag("learner",
			`Make this Zero a "learner" node. In learner mode, this Zero will not participate `+
				"in Raft elections. This can be used to achieve a read-only replica.").
		String())


	// ALPHA
	flag.StringP("postings", "p", "p", "Directory to store posting lists.")
	flag.String("tmp", "t", "Directory to store temporary buffers.")

	flag.StringP("ALPHA_wal", "w", "w", "Directory to store raft write-ahead logs.")
	flag.String("export", "export", "Folder in which to store exports.")

	// Useful for running multiple servers on the same machine.
	flag.IntP("port_offset", "o", 0,
		"Value added to all listening port numbers. [Zero Grpc=5080, Zero HTTP=6080, Alpha Internal=7080, Alpha HTTP=8080, Alpha Grpc=9080]")

	// Custom plugins.
	flag.String("custom_tokenizers", "",
		"Comma separated list of tokenizer plugins for custom indices.")

	// By default Go GRPC traces all requests.
	grpc.EnableTracing = false

	flag.String("badger", worker.BadgerDefaults, z.NewSuperFlagHelp(worker.BadgerDefaults).
		Head("Badger options (Refer to badger documentation for all possible options)").
		Flag("compression",
			`[none, zstd:level, snappy] Specifies the compression algorithm and
			compression level (if applicable) for the postings directory."none" would disable
			compression, while "zstd:1" would set zstd compression at level 1.`).
		Flag("numgoroutines",
			"The number of goroutines to use in badger.Stream.").
		String())

	// Cache flags.
	flag.String("cache", worker.CacheDefaults, z.NewSuperFlagHelp(worker.CacheDefaults).
		Head("Cache options").
		Flag("size-mb",
			"Total size of cache (in MB) to be used in Dgraph.").
		Flag("percentage",
			"Cache percentages summing up to 100 for various caches (FORMAT: PostingListCache,"+
				"PstoreBlockCache,PstoreIndexCache)").
		String())

	flag.String("ALPHA_raft", worker.RaftDefaults, z.NewSuperFlagHelp(worker.RaftDefaults).
		Head("Raft options").
		Flag("idx",
			"Provides an optional Raft ID that this Alpha would use to join Raft groups.").
		Flag("group",
			"Provides an optional Raft Group ID that this Alpha would indicate to Zero to join.").
		Flag("learner",
			`Make this Alpha a "learner" node. In learner mode, this Alpha will not participate `+
				"in Raft elections. This can be used to achieve a read-only replica.").
		Flag("snapshot-after-entries",
			"Create a new Raft snapshot after N number of Raft entries. The lower this number, "+
				"the more frequent snapshot creation will be. Snapshots are created only if both "+
				"snapshot-after-duration and snapshot-after-entries threshold are crossed.").
		Flag("snapshot-after-duration",
			"Frequency at which we should create a new raft snapshots. Set "+
				"to 0 to disable duration based snapshot.").
		Flag("pending-proposals",
			"Number of pending mutation proposals. Useful for rate limiting.").
		String())

	flag.String("security", worker.SecurityDefaults, z.NewSuperFlagHelp(worker.SecurityDefaults).
		Head("Security options").
		Flag("token",
			"If set, all Admin requests to Dgraph will need to have this token. The token can be "+
				"passed as follows: for HTTP requests, in the X-Dgraph-AuthToken header. For Grpc, "+
				"in auth-token key in the context.").
		Flag("whitelist",
			"A comma separated list of IP addresses, IP ranges, CIDR blocks, or hostnames you wish "+
				"to whitelist for performing admin actions (i.e., --security "+
				`"whitelist=144.142.126.254,127.0.0.1:127.0.0.3,192.168.0.0/16,host.docker.`+
				`internal").`).
		String())

	flag.String("ALPHA_limit", worker.LimitDefaults, z.NewSuperFlagHelp(worker.LimitDefaults).
		Head("Limit options").
		Flag("query-edge",
			"The maximum number of edges that can be returned in a query. This applies to shortest "+
				"path and recursive queries.").
		Flag("normalize-node",
			"The maximum number of nodes that can be returned in a query that uses the normalize "+
				"directive.").
		Flag("mutations",
			"[allow, disallow, strict] The mutations mode to use.").
		Flag("mutations-nquad",
			"The maximum number of nquads that can be inserted in a mutation request.").
		Flag("disallow-drop",
			"Set disallow-drop to true to block drop-all and drop-data operation. It still"+
				" allows dropping attributes and types.").
		Flag("max-pending-queries",
			"Number of maximum pending queries before we reject them as too many requests.").
		Flag("query-timeout",
			"Maximum time after which a query execution will fail. If set to"+
				" 0, the timeout is infinite.").
		Flag("max-retries",
			"Commits to disk will give up after these number of retries to prevent locking the "+
				"worker in a failed state. Use -1 to retry infinitely.").
		Flag("txn-abort-after", "Abort any pending transactions older than this duration."+
			" The liveness of a transaction is determined by its last mutation.").
		Flag("shared-instance", "When set to true, it disables ACLs for non-galaxy users. "+
			"It expects the access JWT to be constructed outside dgraph for non-galaxy users as "+
			"login is denied to them. Additionally, this disables access to environment variables for minio, aws, etc.").
		String())

	flag.String("graphql", worker.GraphQLDefaults, z.NewSuperFlagHelp(worker.GraphQLDefaults).
		Head("GraphQL options").
		Flag("introspection",
			"Enables GraphQL schema introspection.").
		Flag("debug",
			"Enables debug mode in GraphQL. This returns auth errors to clients, and we do not "+
				"recommend turning it on for production.").
		Flag("extensions",
			"Enables extensions in GraphQL response body.").
		Flag("poll-interval",
			"The polling interval for GraphQL subscription.").
		Flag("lambda-url",
			"The URL of a lambda server that implements custom GraphQL Javascript resolvers.").
		String())

	flag.String("cdc", worker.CDCDefaults, z.NewSuperFlagHelp(worker.CDCDefaults).
		Head("Change Data Capture options").
		Flag("file",
			"The path where audit logs will be stored.").
		Flag("kafka",
			"A comma separated list of Kafka hosts.").
		Flag("sasl-user",
			"The SASL username for Kafka.").
		Flag("sasl-password",
			"The SASL password for Kafka.").
		Flag("sasl-mechanism",
			"The SASL mechanism for Kafka (PLAIN, SCRAM-SHA-256 or SCRAM-SHA-512)").
		Flag("ca-cert",
			"The path to CA cert file for TLS encryption.").
		Flag("client-cert",
			"The path to client cert file for TLS encryption.").
		Flag("client-key",
			"The path to client key file for TLS encryption.").
		String())

	flag.String("audit", worker.AuditDefaults, z.NewSuperFlagHelp(worker.AuditDefaults).
		Head("Audit options").
		Flag("output",
			`[stdout, /path/to/dir] This specifies where audit logs should be output to.
			"stdout" is for standard output. You can also specify the directory where audit logs
			will be saved. When stdout is specified as output other fields will be ignored.`).
		Flag("compress",
			"Enables the compression of old audit logs.").
		Flag("encrypt-file",
			"The path to the key file to be used for audit log encryption.").
		Flag("days",
			"The number of days audit logs will be preserved.").
		Flag("size",
			"The audit log max size in MB after which it will be rolled over.").
		String())
}

func runZero(zInitLock *sync.WaitGroup) {
	zero.SetOptions(zero.Options{
		Telemetry:         telemetry,
		Raft:              raft,
		Limit:             limit,
		Bindall:           Start.Conf.GetBool("bindall"),
		PortOffset:        Start.Conf.GetInt("port_offset"),
		NumReplicas:       Start.Conf.GetInt("replicas"),
		Peer:              Start.Conf.GetString("peer"),
		W:                 Start.Conf.GetString("ZERO_wal"),
		RebalanceInterval: Start.Conf.GetDuration("rebalance_interval"),
		TlsClientConfig:   tlsClientConf,
		Audit:             auditConf,
		LimiterConfig:     limitConf,
	})

	glog.Infof("Setting Config to: %+v", zero.Opts)
	x.WorkerConfig.Parse(Start.Conf)

	if !enc.EeBuild && Start.Conf.GetString("enterprise_license") != "" {
		log.Fatalf("ERROR: enterprise_license option cannot be applied to OSS builds. ")
	}

	if zero.Opts.NumReplicas < 0 || zero.Opts.NumReplicas%2 == 0 {
		log.Fatalf("ERROR: Number of replicas must be odd for consensus. Found: %d",
			zero.Opts.NumReplicas)
	}

	if Start.Conf.GetBool("expose_trace") {
		// TODO: Remove this once we get rid of event logs.
		trace.AuthRequest = func(req *http.Request) (any, sensitive bool) {
			return true, true
		}
	}

	if zero.Opts.Audit != nil {
		wd, err := filepath.Abs(zero.Opts.W)
		x.Check(err)
		ad, err := filepath.Abs(zero.Opts.Audit.Output)
		x.Check(err)
		x.AssertTruef(ad != wd,
			"WAL directory and Audit output cannot be the same ('%s').", zero.Opts.Audit.Output)
	}

	if zero.Opts.RebalanceInterval <= 0 {
		log.Fatalf("ERROR: Rebalance interval must be greater than zero. Found: %d",
			zero.Opts.RebalanceInterval)
	}

	grpc.EnableTracing = false
	otrace.ApplyConfig(otrace.Config{
		DefaultSampler: otrace.ProbabilitySampler(Start.Conf.GetFloat64("trace"))})

	addr := "localhost"
	if zero.Opts.Bindall {
		addr = "0.0.0.0"
	}
	if x.WorkerConfig.MyAddr == "" {
		x.WorkerConfig.MyAddr = fmt.Sprintf("localhost:%d", x.PortZeroGrpc+zero.Opts.PortOffset)
	}

	nodeId := zero.Opts.Raft.GetUint64("idx")
	if nodeId == 0 {
		log.Fatalf("ERROR: raft.idx flag cannot be 0. Please set idx to a unique positive integer.")
	}
	grpcListener, err := x.SetupListener(addr, x.PortZeroGrpc+zero.Opts.PortOffset, "grpc")
	x.Check(err)
	httpListener, err := x.SetupListener(addr, x.PortZeroHTTP+zero.Opts.PortOffset, "http")
	x.Check(err)

	// Create and initialize write-ahead log.
	x.Checkf(os.MkdirAll(zero.Opts.W, 0700), "Error while creating WAL dir.")
	store := raftwal.Init(zero.Opts.W)
	store.SetUint(raftwal.RaftId, nodeId)
	store.SetUint(raftwal.GroupId, 0) // All zeros have group zero.

	// Initialize the servers.
	var st zero.State
	st.InitServers(Start.Conf, grpcListener, httpListener, store)

	st.RegisterHttpHandlers(limit)

	// This must be here. It does not work if placed before Grpc init.
	st.InitAndStartNode()

	if zero.Opts.Telemetry.GetBool("reports") {
		go st.PeriodicallyPostTelemetry()
	}

	sdCh := make(chan os.Signal, 1)
	signal.Notify(sdCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// handle signals
	go st.HandleSignals(sdCh)

	st.HandleShutDown(sdCh, grpcListener, httpListener)

	st.MonitorMetrics()
	
	zInitLock.Done()
	st.Wait()

	st.Close(store)
}

// func runAlpha() {
// 	// ALPHA

// 	bindall = Start.Conf.GetBool("bindall")
// 	cache := z.NewSuperFlag(Start.Conf.GetString("cache")).MergeAndCheckDefault(
// 		worker.CacheDefaults)
// 	totalCache := cache.GetInt64("size-mb")
// 	x.AssertTruef(totalCache >= 0, "ERROR: Cache size must be non-negative")

// 	cachePercentage := cache.GetString("percentage")
// 	cachePercent, err := x.GetCachePercentages(cachePercentage, 3)
// 	x.Check(err)
// 	postingListCacheSize := (cachePercent[0] * (totalCache << 20)) / 100
// 	pstoreBlockCacheSize := (cachePercent[1] * (totalCache << 20)) / 100
// 	pstoreIndexCacheSize := (cachePercent[2] * (totalCache << 20)) / 100


// 	cacheOpts := fmt.Sprintf("blockcachesize=%d; indexcachesize=%d; ",
// 		pstoreBlockCacheSize, pstoreIndexCacheSize)
// 	bopts := badger.DefaultOptions("").FromSuperFlag(worker.BadgerDefaults + cacheOpts).
// 		FromSuperFlag(Start.Conf.GetString("badger"))
// 	security := z.NewSuperFlag(Start.Conf.GetString("security")).MergeAndCheckDefault(
// 		worker.SecurityDefaults)
// 	conf := audit.GetAuditConf(Start.Conf.GetString("audit"))
// 	opts := worker.Options{
// 		PostingDir:      Start.Conf.GetString("postings"),
// 		WALDir:          Start.Conf.GetString("wal"),
// 		CacheMb:         totalCache,
// 		CachePercentage: cachePercentage,

// 		MutationsMode:  worker.AllowMutations,
// 		AuthToken:      security.GetString("token"),
// 		Audit:          conf,
// 		ChangeDataConf: Start.Conf.GetString("cdc"),
// 	}

// 	keys, err := ee.GetKeys(Start.Conf)
// 	x.Check(err)

// 	if keys.AclKey != nil {
// 		opts.HmacSecret = keys.AclKey
// 		opts.AccessJwtTtl = keys.AclAccessTtl
// 		opts.RefreshJwtTtl = keys.AclRefreshTtl
// 		glog.Info("ACL secret key loaded successfully.")
// 	}

// 	x.Config.Limit = z.NewSuperFlag(Start.Conf.GetString("limit")).MergeAndCheckDefault(
// 		worker.LimitDefaults)
// 	abortDur := x.Config.Limit.GetDuration("txn-abort-after")
// 	switch strings.ToLower(x.Config.Limit.GetString("mutations")) {
// 	case "allow":
// 		opts.MutationsMode = worker.AllowMutations
// 	case "disallow":
// 		opts.MutationsMode = worker.DisallowMutations
// 	case "strict":
// 		opts.MutationsMode = worker.StrictMutations
// 	default:
// 		glog.Error(`--limit "mutations=<mode>;" must be one of allow, disallow, or strict`)
// 		os.Exit(1)
// 	}

// 	worker.SetConfiguration(&opts)

// 	ips, err := x.GetIPsFromString(security.GetString("whitelist"))
// 	x.Check(err)

// 	tlsClientConf, err := x.LoadClientTLSConfigForInternalPort(Start.Conf)
// 	x.Check(err)
// 	tlsServerConf, err := x.LoadServerTLSConfigForInternalPort(Start.Conf)
// 	x.Check(err)

// 	raft := z.NewSuperFlag(Start.Conf.GetString("raft")).MergeAndCheckDefault(worker.RaftDefaults)
// 	x.WorkerConfig = x.WorkerOptions{
// 		TmpDir:              Start.Conf.GetString("tmp"),
// 		ExportPath:          Start.Conf.GetString("export"),
// 		ZeroAddr:            strings.Split(Start.Conf.GetString("zero"), ","),
// 		Raft:                raft,
// 		WhiteListedIPRanges: ips,
// 		StrictMutations:     opts.MutationsMode == worker.StrictMutations,
// 		AclEnabled:          keys.AclKey != nil,
// 		AbortOlderThan:      abortDur,
// 		StartTime:           startTime,
// 		Security:            security,
// 		TLSClientConfig:     tlsClientConf,
// 		TLSServerConfig:     tlsServerConf,
// 		HmacSecret:          opts.HmacSecret,
// 		Audit:               opts.Audit != nil,
// 		Badger:              bopts,
// 	}
// 	x.WorkerConfig.Parse(Start.Conf)

// 	if telemetry.GetBool("reports") {
// 		go edgraph.PeriodicallyPostTelemetry()
// 	}

// 	// Set the directory for temporary buffers.
// 	z.SetTmpDir(x.WorkerConfig.TmpDir)

// 	x.WorkerConfig.EncryptionKey = keys.EncKey
// }

func run() {
	var err error

	telemetry = z.NewSuperFlag(Start.Conf.GetString("telemetry")).MergeAndCheckDefault(
		x.TelemetryDefaults)
	if telemetry.GetBool("sentry") {
		x.InitSentry(enc.EeBuild)
		defer x.FlushSentry()
		x.ConfigureSentryScope("alpha")
		x.WrapPanics()
		x.SentryOptOutNote()
	}

	x.PrintVersion()
	tlsClientConf, err = x.LoadClientTLSConfigForInternalPort(Start.Conf)
	x.Check(err)

	raft = z.NewSuperFlag(Start.Conf.GetString("ZERO_raft")).MergeAndCheckDefault(
		zero.RaftDefaults)
	// auditConf := audit.GetAuditConf(Start.Conf.GetString("audit"))
	limit = z.NewSuperFlag(Start.Conf.GetString("ZERO_limit")).MergeAndCheckDefault(
		worker.ZeroLimitsDefaults)
	limitConf = &x.LimiterConf{
		UidLeaseLimit: limit.GetUint64("uid-lease"),
		RefillAfter:   limit.GetDuration("refill-interval"),
	}

	var zInitLock sync.WaitGroup
	zInitLock.Add(1)
	go runZero(&zInitLock)
	zInitLock.Wait()

	zInitLock.Add(1)
	fmt.Println("HEREEEEEE")
	zInitLock.Wait()
	// runAlpha()
}
