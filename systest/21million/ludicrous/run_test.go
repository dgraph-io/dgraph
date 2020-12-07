package bulk

import (
	"os"
	"os/exec"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/testutil"

	"github.com/dgraph-io/dgraph/systest/21million/common"

	"testing"
)

func TestQueries(t *testing.T) {
	t.Run("Run queries", common.TestQueriesFor21Million)
}

var rootDir = os.TempDir()

func TestMain(m *testing.M) {
	schemaFile := os.Getenv("GOPATH") + "/src/github.com/dgraph-io/benchmarks/data/21million.schema"
	rdfFile := os.Getenv("GOPATH") + "/src/github.com/dgraph-io/benchmarks/data/21million.rdf.gz"

	liveCmd := exec.Command(testutil.DgraphBinaryPath(), "live",
		"--files", rdfFile,
		"--schema", schemaFile,
		"--alpha", testutil.SockAddr,
		"--zero", testutil.SockAddrZero,
		"--ludicrous_mode",
	)
	liveCmd.Dir = rootDir
	if out, err := liveCmd.Output(); err != nil {
		glog.Error("Error %v", err)
		glog.Error("Output %v", out)
		os.Exit(1)
	}

	exitCode := m.Run()
	_ = os.RemoveAll(rootDir)
	os.Exit(exitCode)
}
