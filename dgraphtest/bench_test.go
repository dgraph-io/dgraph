package dgraphtest

import (
	"os"
	"testing"
)

var perfTests map[string]DgraphPerf

func BenchmarkXYZ(b *testing.B) {
	for k, v := range PerfTests {
		b.Run(k, v.fnc)
		// dg client="url"
	}
}

func TestMain(m *testing.M) {

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
