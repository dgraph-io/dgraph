package dgraphtest

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/dgraph-io/dgraph/testutil"
)

var runType string

func BenchmarkBulkoad(b *testing.B) {
	if runType == "GITHUB_CI" {
		// No need for local setup; cluster has been setup by perf_framework.go
		fmt.Println("Executing perf tests on CI")
	} else {
		// Setup cluster locally
		fmt.Println("Executing perf tests locally")
	}

	var args []string
	var schemaFile string
	var rdfFile string
	var res []byte
	args = append(args, "bulk",
		"-f", rdfFile,
		"-s", schemaFile,
		"--http", "localhost:8000",
		"--reduce_shards=1",
		"--map_shards=1",
		"--store_xids=true",
		"--zero", testutil.SockAddrZero,
		"--force-namespace", strconv.FormatUint(0, 10))

	bulkCmd := exec.Command(testutil.DgraphBinaryPath(), args...)
	fmt.Printf("Running %s\n", bulkCmd)

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		if err := testutil.MakeDirEmpty([]string{"out/0"}); err != nil {
			os.Exit(1)
		}
		bulkCmd = exec.Command(testutil.DgraphBinaryPath(), args...)
		b.StartTimer()
		out, err := bulkCmd.CombinedOutput()
		b.StopTimer()
		if err != nil {
			b.Fatal(err)
		}
		res = out
	}
	fmt.Printf("Finished bulk load. Output \n%s\n", res)

}

func BenchmarkSample(b *testing.B) {
	if runType == "GITHUB_CI" {
		// No need for local setup; cluster has been setup by perf_framework.go
		fmt.Println("Executing perf tests on CI")
	} else {
		// Setup cluster locally
		fmt.Println("Executing perf tests locally")
	}

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		b.StartTimer()
		// functionInTest()
		b.StopTimer()

	}
	fmt.Printf("Finished Test\n")
}

func init() {
	flag.StringVar(&runType, "runType", "", "Run Type")
}

func TestMain(m *testing.M) {

}

/*

query
	benchmark_query_test.go  <---
	benchmark_query.yml  <----

posting
	posting_query_test.go  <---
	posting_query.yml  <----


*/
