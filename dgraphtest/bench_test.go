package dgraphtest

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgo/v210/protos/api"
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

var TestClusterMap map[string]ClusterConfig

func BenchmarkLDBCAllQueries(b *testing.B) {
	clusterConf := NewClusterConfig().WithNumAlphas(1).WithNumZeros(1)
	RunPerfTest(b, clusterConf, func(cluster Cluster, b *testing.B) {

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

func setSchema(schema string, client *dgo.Dgraph) {
	var err error
	for retry := 0; retry < 60; retry++ {
		err = client.Alter(context.Background(), &api.Operation{Schema: schema})
		if err == nil {
			return
		}
		time.Sleep(time.Second)
	}
	panic(fmt.Sprintf("Could not alter schema. Got error %v", err.Error()))
}

func BenchmarkSimpleMutationQuery(b *testing.B) {
	clusterConf := NewClusterConfig().WithNumAlphas(1).WithNumZeros(1)
	RunPerfTest(b, clusterConf, func(cluster Cluster, b *testing.B) {

		// Write benchmark here.
		dg, err := cluster.Client()
		if err != nil {
			b.Fatalf("Error while getting a dgraph client: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)

		// require.NoError(b, alterSchema(`name: string .`))

		schema := `name: string @index(exact).`

		setSchema(schema, dg)

		m1 := `
		_:u1 <name> "Ab Bc" .
    	_:u2 <name> "Bc Cd" .
    	_:u3 <name> "Cd Da" .
		`

		q1 := `{
			q(func: eq(name, "Ab Bc")) {
				uid
				name
			}
		}`

		var resp *api.Response

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err = dg.NewTxn().Mutate(ctx, &api.Mutation{
				CommitNow: true,
				SetNquads: []byte(m1),
			})
			require.NoError(b, err)
			resp, err = dg.NewTxn().Query(ctx, q1)
			require.NoError(b, err)
		}
		b.StopTimer()
		fmt.Println(string(resp.Json))
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
	clusterConf := NewClusterConfig().WithNumAlphas(1).WithNumZeros(1)
	RunPerfTest(b, clusterConf, func(cluster Cluster, b *testing.B) {
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
	clusterConf := NewClusterConfig()
	RunPerfTest(b, clusterConf, func(cluster Cluster, b *testing.B) {
		var res int
		time.Sleep(5 * time.Second)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			res = FibonacciNonRecursive(20)
		}
		fmt.Println(res)
	})

}

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

func TestMain(m *testing.M) {

	TestClusterMap = make(map[string]ClusterConfig)
	TestClusterMap["BenchmarkLDBCAllQueries"] = NewClusterConfig().WithNumAlphas(1).WithNumZeros(1)
	m.Run()

}
