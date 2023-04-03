package dgraphtest

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/testutil"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

var runType string

type TestCases struct {
	Tag   string `yaml:"tag"`
	Query string `yaml:"query"`
	Resp  string `yaml:"resp"`
}

func BenchmarkLDBCAllQueries(b *testing.B) {
	RunPerfTest(b, "name", func(cluster Cluster, b *testing.B) {

		// Write benchmark here.
		dg, err := cluster.Client()
		if err != nil {
			b.Fatalf("Error while getting a dgraph client: %v", err)
		}

		yfile, _ := os.ReadFile("../systest/ldbc/test_cases.yaml")

		tc := make(map[string]TestCases)

		err = yaml.Unmarshal(yfile, &tc)

		if err != nil {
			b.Fatalf("Error while greading test cases yaml: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)

		for i := 0; i < b.N; i++ {
			b.StartTimer()
			for _, tt := range tc {
				_, err := dg.NewTxn().Query(ctx, tt.Query)
				require.NoError(b, err)
				b.StopTimer()
				// testutil.CompareJSON(b, tt.Resp, string(resp.Json))
				if ctx.Err() == context.DeadlineExceeded {
					b.Fatal("aborting test due to query timeout")
				}
			}
		}
		cancel()
	})
}

func FibonacciRecursive(n int) int {
	if n <= 1 {
		return n
	}
	return FibonacciRecursive(n-1) + FibonacciRecursive(n-2)
}

func FibonacciNonRecursive(n int) int {
	if n <= 1 {
		return n
	}
	a, b := 0, 1
	for i := 2; i <= n; i++ {
		a, b = b, a+b
	}
	return b
}

func BenchmarkFibonacciRecursive(b *testing.B) {
	RunPerfTest(b, "recursive", func(cluster Cluster, b *testing.B) {
		var res int
		time.Sleep(5 * time.Second)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			res = FibonacciRecursive(20)
		}
		fmt.Println(res)
	})
}

func BenchmarkFibonacciNonRecursive(b *testing.B) {
	RunPerfTest(b, "nonrecursive", func(cluster Cluster, b *testing.B) {
		var res int
		time.Sleep(5 * time.Second)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			res = FibonacciNonRecursive(20)
		}
		fmt.Println(res)
	})

}

func BenchmarkFibWPerf(b *testing.B) {
	for i := 0; i < b.N; i++ {
		FibonacciNonRecursive(20)
	}
}

func BenchmarkFibRWPerf(b *testing.B) {
	time.Sleep(5 * time.Second)
	var res int
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res = FibonacciRecursive(20)
	}
	fmt.Println(res)
}

func BenchmarkBulkoad(b *testing.B) {
	if runType == "GH_CI" {
		// No need for local setup; cluster has been setup by perf_framework.go
		fmt.Println("Executing perf tests on CI")
	} else {
		// Setup cluster locally
		//
		fmt.Println("Executing perf tests locally")
	}

	// resource.json

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

// func BenchmarkLDBCAllQueries(b *testing.B) {

// }

func init() {
	flag.StringVar(&runType, "runType", "", "Run Type")
}

func TestMain(m *testing.M) {
	m.Run()

}

/*

query
	benchmark_query_test.go  <---
	benchmark_query.yml  <----

posting
	posting_query_test.go  <---
	posting_query.yml  <----


*/
