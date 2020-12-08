package bulk

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/dgraph-io/dgraph/testutil"

	"github.com/dgraph-io/dgraph/systest/21million/common"

	"testing"
)

func TestQueries(t *testing.T) {
	t.Run("Run queries", common.TestQueriesFor21Million)
}

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
	if out, err := liveCmd.Output(); err != nil {
		fmt.Printf("error %v\n", err)
		fmt.Printf("output %v\n", out)
		os.Exit(1)
	}

	time.Sleep(10 * time.Minute)
	exitCode := m.Run()
	_ = os.RemoveAll("./t")
	os.Exit(exitCode)
}
