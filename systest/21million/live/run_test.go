package bulk

import (
	"github.com/dgraph-io/dgraph/testutil"
	"os"

	"github.com/dgraph-io/dgraph/systest/21million/common"

	"testing"
)

func TestQueries(t *testing.T) {
	t.Run("Run queries", common.TestQueriesFor21Million)
}

func TestMain(m *testing.M) {
	schemaFile := os.Getenv("TEST_DATA_DIRECTORY") + "/21million.schema"
	rdfFile := os.Getenv("TEST_DATA_DIRECTORY") + "/21million.rdf.gz"
	if err := testutil.LiveLoad(testutil.LiveOpts{
		Alpha:      testutil.ContainerAddr("alpha1", 9080),
		Zero:       testutil.SockAddrZero,
		RdfFile:    rdfFile,
		SchemaFile: schemaFile,
	}); err != nil {
		cleanupAndExit(1)
	}

	exitCode := m.Run()
	cleanupAndExit(exitCode)
}

func cleanupAndExit(exitCode int) {
	_ = os.RemoveAll("./t")
	os.Exit(exitCode)
}
