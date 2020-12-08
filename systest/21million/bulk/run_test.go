package bulk

import (
	"github.com/dgraph-io/dgraph/systest/21million/common"
	"github.com/dgraph-io/dgraph/testutil"
	"log"
	"os"

	"testing"
)

func TestQueries(t *testing.T) {
	t.Run("Run queries", common.TestQueriesFor21Million)
}

func TestMain(m *testing.M) {
	schemaFile := os.Getenv("TEST_DATA_DIRECTORY") + "/21million.schema"
	rdfFile := os.Getenv("TEST_DATA_DIRECTORY") + "/21million.rdf.gz"
	if err := testutil.MakeDirEmpty([]string{"out/0", "out/1", "out/2"}); err != nil {
		os.Exit(1)
	}

	if err := testutil.BulkLoad(testutil.BulkOpts{
		Zero:       testutil.SockAddrZero,
		Shards:     3,
		RdfFile:    rdfFile,
		SchemaFile: schemaFile,
	}); err != nil {
		cleanupAndExit(1)
	}

	if err := testutil.BringAlphaUp("./alpha.yml"); err != nil {
		cleanupAndExit(1)
	}

	exitCode := m.Run()
	cleanupAndExit(exitCode)
}

func cleanupAndExit(exitCode int) {
	testutil.BringAlphaDown("./alpha.yml")
	log.Print(os.RemoveAll("out"))
	os.Exit(exitCode)
}