package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/dgraph-io/dgraph/dgraphtest"
)

func setupClient(resources dgraphtest.ResourceDetails) {
	cmd := exec.Command("git", "clone", "-b", "anurag/perf-set-client", dgraphtest.DgraphRepoUrl)
	dgraphtest.RunCmdInResource(cmd.String(), resources)
}

func runTest(resources dgraphtest.ResourceDetails, task dgraphtest.DgraphPerf) {
	cmd := exec.Command("go", "test", "-benchmem", "-run=^$", "-bench", "^"+task.Name()+"$", "-count=1", "-v")
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	cmd.Dir = filepath.Join(filepath.Dir(wd), "dgraphtest")

	dgraphtest.RunCmdInResource(cmd.String(), resources)
}

func runTestLocally(task dgraphtest.DgraphPerf) {
	cmd := exec.Command("go", "test", "-benchmem", "-run=^$", "-bench", "^"+task.Name()+"$", "-count=1", "-v")
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	cmd.Dir = filepath.Join(filepath.Dir(wd), "dgraphtest")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		log.Panicf("Could not run cmd (%s): %v, %v", cmd, err, out)
	}
	fmt.Println(string(out))
}

var PerfTests map[string]dgraphtest.DgraphPerf

func runPerfTest(task dgraphtest.DgraphPerf) error {
	if os.Getenv("RunType") == "CI" {
		log.Println("Running on CI")
		resources := dgraphtest.ProvisionClientAndTarget(task)
		// setup the client
		setupClient(resources)
		runTest(resources, task)
	} else {
		runTestLocally(task)
	}

	return nil
}

func init() {

	/*
		Name: ldbc-all-query
		Cluster Configuration: numAlphas = 1; numZeros = 1; replicas = 0
		Resource Configuration: loc = aws (t2.xlarge); mem = 16; cpu = 4; clusterType = allInOne; clientType = clientOnA;
		Metric configuration: gtc = true
		fnc: BenchmarkLDBCAllQueries
	*/

}
func main() {

	// TODO (anurag): Make this concurrent
	PerfTests := make(map[string]dgraphtest.DgraphPerf)
	PerfTests["bench-simple"] = dgraphtest.NewDgraphPerf("BenchmarkSimpleMutationQuery")
	for key, val := range PerfTests {
		fmt.Println(key, val)
		err := runPerfTest(val)
		fmt.Println(err)
	}

}
