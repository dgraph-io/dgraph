package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
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

var baseUrl = "https://github.com/dgraph-io/benchmarks/blob/master/ldbc/sf0.3/ldbc_rdf_0.3/"
var suffix = "?raw=true"
var schemaFile string
var rdfFile string
var rdfFileNames = [...]string{
	"Deltas.rdf",
	"comment_0.rdf",
	"containerOf_0.rdf",
	"forum_0.rdf",
	"hasCreator_0.rdf",
	"hasInterest_0.rdf",
	"hasMember_0.rdf",
	"hasModerator_0.rdf",
	"hasTag_0.rdf",
	"hasType_0.rdf",
	"isLocatedIn_0.rdf",
	"isPartOf_0.rdf",
	"isSubclassOf_0.rdf",
	"knows_0.rdf",
	"likes_0.rdf",
	"organisation_0.rdf",
	"person_0.rdf",
	"place_0.rdf",
	"post_0.rdf",
	"replyOf_0.rdf",
	"studyAt_0.rdf",
	"tag_0.rdf",
	"tagclass_0.rdf",
	"workAt_0.rdf"}

var ldbcDataFiles = map[string]string{
	"ldbcTypes.schema": "https://github.com/dgraph-io/benchmarks/blob/master/ldbc/sf0.3/ldbcTypes.schema?raw=true",
}

func TestQueries(t *testing.T) {
	dg, err := testutil.DgraphClient(testutil.ContainerAddr("alpha1", 9080))
	if err != nil {
		t.Fatalf("Error while getting a dgraph client: %v", err)
	}

	yfile, _ := os.ReadFile("test_cases.yaml")

	tc := make(map[string]TestCases)

	err = yaml.Unmarshal(yfile, &tc)

	if err != nil {
		t.Fatalf("Error while greading test cases yaml: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	for _, tt := range tc {
		desc := tt.Tag
		if cc := os.Getenv(COVERAGE_FLAG); cc == EXPECTED_COVERAGE_ENV {
			// LDBC test (IC05) times out for test-binaries (code coverage enabled)
			if desc == "IC05" {
				continue
			}
		}
		// TODO(anurag): IC06 and IC10 have non-deterministic results because of dataset.
		// Find a way to modify the queries to include them in the tests
		if desc == "IC06" || desc == "IC10" {
			continue
		}
		t.Run(desc, func(t *testing.T) {
			resp, err := dg.NewTxn().Query(ctx, tt.Query)
			require.NoError(t, err)
			testutil.CompareJSON(t, tt.Resp, string(resp.Json))
		})
		if ctx.Err() == context.DeadlineExceeded {
			t.Fatal("aborting test due to query timeout")
		}
	}
	cancel()
}

func downloadLDBCFiles(m *testing.M, dataDir string) {
	ok, err := exists(dataDir)
	if err != nil {
		fmt.Print("Skipping downloading as files already present\n")
		return
	}
	if ok {
		fmt.Print("Skipping downloading as files already present\n")
		return
	}

	x.Check(testutil.MakeDirEmpty([]string{dataDir}))

	for _, name := range rdfFileNames {
		filepath := baseUrl + name + suffix
		ldbcDataFiles[name] = filepath
	}

	start := time.Now()
	var wg sync.WaitGroup
	for fname, link := range ldbcDataFiles {
		wg.Add(1)
		go func(fname, link string, wg *sync.WaitGroup) {
			defer wg.Done()
			start := time.Now()
			cmd := exec.Command("wget", "-O", fname, link)
			cmd.Dir = dataDir
			if out, err := cmd.CombinedOutput(); err != nil {
				fmt.Printf("Error %v", err)
				fmt.Printf("Output %v", out)
			}
			fmt.Printf("Downloaded %s to %s in %s \n", fname, dataDir, time.Since(start))
		}(fname, link, &wg)
	}
	wg.Wait()
	fmt.Printf("Downloaded %d files in %s \n", len(ldbcDataFiles), time.Since(start))
}

// exists returns whether the given file or directory exists
func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func testFunction() {
	time.Sleep(20 * time.Millisecond)
}

var res []byte

func BenchmarkBulkload(b *testing.B) {
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
}

func BenchmarkTestFunction(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testFunction()
	}
}

func TestMain(m *testing.M) {
	// dataDir := os.TempDir() + "/ldbcData"
	path, err := os.Getwd()
	if err != nil {
		log.Println(err)
	}
	dataDir := path + "/ldbcData"
	fmt.Println("Datadir: ", dataDir)
	downloadLDBCFiles(m, dataDir)
	schemaFile = filepath.Join(dataDir, "ldbcTypes.schema")
	rdfFile = dataDir

	if err := testutil.MakeDirEmpty([]string{"out/0"}); err != nil {
		os.Exit(1)
	}

	if err := testutil.StartZeros("./docker-compose.yml"); err != nil {
		fmt.Printf("Error while bringin up zeros. Error: %v\n", err)
		cleanupAndExit(1)
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
	_ = os.RemoveAll("out")
	os.Exit(exitCode)
}
