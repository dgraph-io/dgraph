package bulk

import (
	"fmt"
	"github.com/dgraph-io/dgraph/testutil"
	"os"
	"os/exec"

	"testing"
)
//
//func TestQueries(t *testing.T) {
//	t.Run("Run queries", common.TestQueriesFor21Million)
//}

func TestMain(m *testing.M) {
	schemaFile := os.Getenv("TEST_DATA_DIRECTORY") + "/21million.schema"
	rdfFile := os.Getenv("TEST_DATA_DIRECTORY") + "/21million.rdf.gz"

	liveCmd := exec.Command(testutil.DgraphBinaryPath(), "live",
		"--files", rdfFile,
		"--schema", schemaFile,
		"--alpha", testutil.SockAddr,
		"--zero", testutil.SockAddrZero,
		"--ludicrous_mode",
	)
	if out, err := liveCmd.CombinedOutput(); err != nil {
		fmt.Printf("error %v\n", err)
		fmt.Printf("output %v\n", out)
		cleanupAndExit(1)
	}

	// dont run queries
	//fmt.Print("waiting for the indexes to be completed \n")
	//time.Sleep(10 * time.Minute)
	//exitCode := m.Run()
	cleanupAndExit(0)
}

func cleanupAndExit(exitCode int) {
	_ = os.RemoveAll("./t")
	os.Exit(exitCode)
}
