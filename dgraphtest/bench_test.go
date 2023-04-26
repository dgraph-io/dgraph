package dgraphtest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/stretchr/testify/require"
)

type TestCases struct {
	Tag   string `yaml:"tag"`
	Query string `yaml:"query"`
	Resp  string `yaml:"resp"`
}

var TestClusterMap map[string]ClusterConfig

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

	clusterConf := PerfTest["BenchmarkSimpleMutationQuery"]
	RunPerfTest(b, clusterConf, func(c interface{}, b *testing.B) {

		cluster := c.(Cluster)
		// Write benchmark here.
		dg, cleanup, err := cluster.Client()
		defer cleanup()
		if err != nil {
			b.Fatalf("Error while getting a dgraph client: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)

		// require.NoError(b, alterSchema(`name: string .`))

		schema := `name: string @index(exact).`

		setSchema(schema, dg.Dgraph)

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

		_, err = dg.NewTxn().Mutate(ctx, &api.Mutation{
			CommitNow: true,
			SetNquads: []byte(m1),
		})
		require.NoError(b, err)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
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

// TODO(anurag): Remove this test once we have are sure that the testing framework is working.
// These are toy tests to test framework and have light weight reproducible performance tests.
func BenchmarkFibonacciRecursive(b *testing.B) {
	var ans int
	RunPerfTest(b, nil, func(c interface{}, b *testing.B) {
		var res int
		time.Sleep(5 * time.Second)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			res = FibonacciRecursive(20)
		}
		ans = res
	})
	fmt.Println("Res: ", ans)
}

// TODO(anurag): Remove this test once we have are sure that the testing framework is working.
// These are toy tests to test framework and have light weight reproducible performance tests.
func BenchmarkFibonacciNonRecursive(b *testing.B) {
	var ans int
	RunPerfTest(b, nil, func(cluster interface{}, b *testing.B) {
		var res int
		time.Sleep(5 * time.Second)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			res = FibonacciNonRecursive(20)
		}
		ans = res
	})
	fmt.Println("Res: ", ans)

}

func TestMain(m *testing.M) {
	m.Run()
}
