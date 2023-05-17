package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/dgraph-io/dgraph/testutil"
)

type TestCases struct {
	Tag   string `yaml:"tag"`
	Query string `yaml:"query"`
	Resp  string `yaml:"resp"`
}

var (
	COVERAGE_FLAG         = "COVERAGE_OUTPUT"
	EXPECTED_COVERAGE_ENV = "--test.coverprofile=coverage.out"
)

var schemaFile string
var rdfFile string

func BenchmarkQueries(b *testing.B) {
	if err := testutil.StartZeros("./docker-compose.yml"); err != nil {
		fmt.Printf("Error while bringin up zeros. Error: %v\n", err)
		cleanupAndExit(1)
	}
	defer cleanUpZeroes()

	var args []string
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

	bulkCmd = exec.Command(testutil.DgraphBinaryPath(), args...)
	_, err := bulkCmd.CombinedOutput()
	if err != nil {
		b.Fatal(err)
	}
	fmt.Println("Successfully loaded data")

	if err := testutil.StartAlphas("./alpha.yml"); err != nil {
		fmt.Printf("Error while bringin up alphas. Error: %v\n", err)
		b.Fatal(err)
	}

	fmt.Println("add: ", testutil.SockAddr)
	dg, err := testutil.DgraphClient(testutil.SockAddr)
	if err != nil {
		b.Fatalf("Error while getting a dgraph client: %v", err)
	}

	yfile, _ := os.ReadFile("../ldbc/test_cases.yaml")

	tc := make(map[string]TestCases)

	err = yaml.Unmarshal(yfile, &tc)

	if err != nil {
		b.Fatalf("Error while greading test cases yaml: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)

	for _, bm := range tc {
		if bm.Tag != "IS01" && bm.Tag != "IC07" {
			continue
		}
		desc := fmt.Sprintf("LDBC Query-%s", bm.Tag)
		b.Run(desc, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				b.StartTimer()
				resp, err := dg.NewTxn().Query(ctx, bm.Query)
				b.StopTimer()
				require.NoError(b, err)
				testutil.CompareJSONBench(b, bm.Resp, string(resp.Json))
				if ctx.Err() == context.DeadlineExceeded {
					b.Fatal("aborting benchmark due to query timeout")
				}
			}
		})
	}
	cancel()
}

func testFunction() {
	time.Sleep(20 * time.Millisecond)
}

var res []byte

func BenchmarkBulkload(b *testing.B) {
	var args []string

	if err := testutil.StartZeros("./docker-compose.yml"); err != nil {
		fmt.Printf("Error while bringin up zeros. Error: %v\n", err)
		cleanupAndExit(1)
	}
	defer cleanUpZeroes()

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

func BenchmarkTestFunction(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testFunction()
	}
}

func cleanUpZeroes() {
	if err := testutil.StopZeros([]string{"zero1"}); err != nil {
		cleanupAndExit(1)
	}
	_ = os.RemoveAll("out")
}

func TestMain(m *testing.M) {
	// dataDir := os.TempDir() + "/ldbcData"
	path, err := os.Getwd()
	if err != nil {
		log.Println(err)
	}
	dataDir := path + "/ldbcData"
	fmt.Println("Datadir: ", dataDir)
	_, err = testutil.DownloadLDBCFiles(dataDir, true)
	if err != nil {
		os.Exit(1)
	}
	schemaFile = filepath.Join(dataDir, "ldbcTypes.schema")
	rdfFile = dataDir

	if err := testutil.MakeDirEmpty([]string{"out/0"}); err != nil {
		os.Exit(1)
	}

	// fmt.Printf("Took %s to bulkupload LDBC dataset\n", time.Since(start))

	// if err := testutil.StartAlphas("./alpha.yml"); err != nil {
	// 	fmt.Printf("Error while bringin up alphas. Error: %v\n", err)
	// 	cleanupAndExit(1)
	// }

	m.Run()
	// cleanupAndExit(exitCode)
}

func cleanupAndExit(exitCode int) {
	if cc := os.Getenv(COVERAGE_FLAG); cc == EXPECTED_COVERAGE_ENV {
		testutil.StopAlphasForCoverage("./alpha.yml")
		os.Exit(exitCode)
	}

	if testutil.StopAlphasAndDetectRace([]string{"alpha1"}) {
		// if there is race fail the test
		exitCode = 1
	}

	if err := testutil.StopZeros([]string{"zero1"}); err != nil {
		exitCode = 1
	}
	_ = os.RemoveAll("out")
	os.Exit(exitCode)
}
