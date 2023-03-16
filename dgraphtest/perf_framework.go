// benchmark_framework.go
package dgraphtest

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/dgraph-io/ristretto/z"
)

// aws, he, local, dgraphcloud
const (
	separate = iota // Each alpha/zero in their respective nodes
	allInOne        // All alpha and zero in one node
	allZallA        // All zero in one node and All alpha in another node
)
const (
	clientOnZ  = iota // Client will be set up on one the zero nodes
	clientOnA         // Client will be set up on one the alpha nodes
	clientDiff        // Client always a different node
)

type ResourceConfig struct {
	loc         string // local, aws, he, dcloud
	mem         string // RAM
	cpu         string // vCPU
	clusterType string // separate, allInOne, allZallA
	clientType  string // clientOnZ, clientOnA, clientDiff
}

type MetricConfig struct {
	gtc          bool // Go Tool chain metric
	dgraphProm   bool // Dgraph prometheus metrics
	promEndpoint string
	// What else configurations would we need here?
}

type DPerfFunc func(b *testing.B)

// Anyone writing a new benchmark only needs to provide information above this line

type MetricReport struct {
}

type DgraphPerf struct {
	name string
	cc   ClusterConfig
	mc   MetricConfig
	rc   ResourceConfig
	fnc  DPerfFunc
}

type AWSDetails struct {
}

type HEDetails struct {
}

type DcloudDetails struct {
}

type ResourceDetails struct {
	loc           string
	awsDetails    AWSDetails
	heDetails     HEDetails
	dcloudDetails DcloudDetails
}

func (dbench *DgraphPerf) isValidBenchConfig() error {

	return nil
}

func (dbench *DgraphPerf) provisionClientAndTarget() ResourceDetails {
	// Provisions relevant resources and returns details
	switch dbench.rc.loc {
	case "aws":
		return ResourceDetails{loc: "aws"}
	case "he":
		return ResourceDetails{loc: "he"}
	case "loc":
		return ResourceDetails{loc: "loc"}
	}

	return ResourceDetails{}
}

// func(b *testing.B) {
// 	// setup
// 	for i := 0; i < b.N; i++ {
// 	//reset/setup
//
// 	}
// }

func parseFlags() (bool, bool) {
	return false, true
}

func runBenchmarkFromClient(resources ResourceDetails, name string) {
	// ssh into the client, clone the repo and set env TEST_TO_RUN to name
	// go test -bench=BenchmarkBulkload -runType=GITHUB_CI
}

func collectMetricsFromClientAndTarget(resources ResourceDetails, name string) MetricReport {
	// ssh into the client, and target and collect metrics after the perftests is complete
	return MetricReport{}
}

var _threadId int32

type _threadIdKey struct{}

func runBenchmark(benchmarkCh chan DgraphPerf, metricReportCh chan MetricReport, closer *z.Closer) error {
	var err error
	threadId := atomic.AddInt32(&_threadId, 1)

	wg := new(sync.WaitGroup)
	defer func() {
		wg.Wait()
		closer.Done()
	}()

	ctx := closer.Ctx()
	ctx = context.WithValue(ctx, _threadIdKey{}, threadId)

	for task := range benchmarkCh {
		if ctx.Err() != nil {
			err = ctx.Err()
			return err
		}

		fmt.Println("Running: ", task)

		// provision resources for the benchmark
		resources := task.provisionClientAndTarget()

		runBenchmarkFromClient(resources, task.name)
		metricReportCh <- collectMetricsFromClientAndTarget(resources, task.name)
	}

	return nil
}

func getValidBenchmarks(benchmarksToRun map[string]DgraphPerf) map[string]DgraphPerf {

	valid := make(map[string]DgraphPerf)
	for k, v := range benchmarksToRun {
		if err := v.isValidBenchConfig(); err != nil {
			fmt.Printf("Invalid benchmark config. ", err)
			continue
		}
		valid[k] = v
	}
	return valid
}

var PerfTests map[string]DgraphPerf

func main() {

	// This piece orchestrates the different benchmarks

	N := len(PerfTests)
	closer := z.NewCloser(N)
	benchmarkCh := make(chan DgraphPerf)
	errCh := make(chan error, 1000)
	metricCh := make(chan MetricReport)

	for i := 0; i < N; i++ {
		go func() {
			if err := runBenchmark(benchmarkCh, metricCh, closer); err != nil {
				errCh <- err
				closer.Signal()
			}
		}()
	}

	go func() {
		defer close(benchmarkCh)
		valid := getValidBenchmarks(PerfTests)

		for k, task := range valid {
			select {
			case benchmarkCh <- task:
				fmt.Printf("Sent %s benchmark for processing.\n", k)
			case <-closer.HasBeenClosed():
				return
			}
		}
	}()

}

//####################################################################################################//

// type BenchmarkConfigDetails

// func setupCluster(r ResourceDetails) {

// }

// func setupClient(c ClientConfig) {

// }

// naming... think about this.
// metric... which all metric to put.

// // c1 := make(chan BenchmarkConfigDetails)
// for _, bench := range benchmarksToRun {
// 	wg.Add(1)
// 	go func(bench BenchmarkConfig) {
// 		resourceDetails := provisionResources(bench.config.ResourceConfig)
// 		setupCluster(resourceDetails)
// 		setupClient(bench.config.ClientConfig)
// 		// Provision client machine and clone repo and copy dgraph binary
// 	}(bench)
// }

// Modify this to use runTask

// resourcesToProvision := collectResources(benchmarksToRun)
// resourceDetails contains resource detail and corresponding benchmark name
// for _, r := range resourceDetails {
// 	setupCluster(r) // ssh into resource and setup dgraph
// 	setupClient(r)
// }

/*
	{
		1
		Target: AWS,
		RAM/CPU
	}
	{
		2
		Target: Dev,
		RAM/CPU
	}
*/

// configList := [1, 2]
// metricList := ["dgraph-prometheus", ]

// funcs := [bulkload, query]

// for()

// dgraphbench.bench("BenchMarkName", func(dgraph.b, mc BenchmarkConfig) {

// })

// func bulkLoadBenchmarkFunction() {
// 	if err := testutil.BulkLoad(testutil.BulkOpts{
// 		Zero:       testutil.SockAddrZero,
// 		Shards:     1,
// 		RdfFile:    rdfFile,
// 		SchemaFile: noschemaFile,
// 	}); err != nil {
// 		fmt.Println(err)
// 		cleanupAndExit(1)
// 	}
// }

// type Bench struct{
// 	func  =
// 	config =
// 	metric =

// }

// mybench := DgraphBench{
// 	clusterConfigList = clusterConfig
// 	metricConfig = metricConfig
// 	numRuns = 5
// 	Type = “long”
// 	funcToBench = bulkLoadBenchmarkFunction
// }

// dgraphtest.add("benchfuncntion", params, func(dgraphtest.B, cluster){b.setup(), b.run(), b.end()})

// mybench.setup()
// mybench.run()
// mybench.end() // would have results + cleanup.
//

/*

// func (b *DgraphPerf) setup() {
// 	// Sets up cluster using b.cc
// }

// func (b *DgraphPerf) run(f DgraphBenchmarkFunction) {

// }

// func (b *DgraphPerf) stop() {
// 	// Stops cluster using b.cc
// }

// func (b *DgraphPerf) metrics() error {
// 	// Logs metrics based on b.mc
// 	return nil
// }

// bulkload, query

// bulkload -- only zero
// query -- zero + alpha + dataset
// liveload --- zero + alpha

// Where to run client from?
// Where to run bulkloader?
// How to add alphas once the zero and bulkload happens
// Need a function on DgraphPerf object for running commands on remote machine
// May want to set-up multiple client machines
// Prometheus/Grafana setup

// Caveat for docker setup ---> --net=host to avoid docker networking

// function on DgraphPerf Object that incorporates locust/boomer functionality
// (given a mutation/query it automatically scales the requests)

// Think if we really need go-benchmark? If yes then can we incorporate it in this main.go
//

// Who will collect metrics? User or the framework?
// Do we need two functions - f and F, so that F can contain other parts
// f -- Smaller benchmark function

type BenchFunction func(b DgraphBenchmarkFunction) error

/*
1. Bulkload --- 2 different resource configs [HE, AWS]

2. Query --- 2 different cluster config [HA and Non HA]

3. Locust
*/

/*
Resource configs, CC configs, Metric configs -- list (given by User)
runBenchmark() // User
initialize() // User


for each rc * cc:
	provisionResources() // Framework
	setUpCluster() // User

	b.StartTimer()
	for count: // Framework
		b.StopTimer()
		initialize() // User
		b.StartTimer()
		runBenchmark() // User

	collectMetrics(mc) // Framework

*/
