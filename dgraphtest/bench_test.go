package dgraphtest

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/dgraph-io/dgraph/testutil"
)

var perfTests map[string]DgraphPerf

func BenchmarkBulkoad(b *testing.B) {
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

func BenchmarkXYZ(b *testing.B) {
	for k, v := range PerfTests {
		b.Run(k, v.fnc)
		// dg client="url"
	}
}

func TestMain(m *testing.M) {
	perfTests["bulkload"] = DgraphPerf{"bulkload", ClusterConfig{}, MetricConfig{}, ResourceConfig{}, BenchmarkBulkoad}

	// check env variable and run only that benchmark
	if bench := os.Getenv("TEST_TO_RUN"); bench == "" {
		perfTests = PerfTests
	} else {
		// filter perfTests based on bench
		perfTests[bench] = PerfTests[bench]
	}

}

/*

query
	benchmark_query_test.go  <---
	benchmark_query.yml  <----

posting
	posting_query_test.go  <---
	posting_query.yml  <----


*/
