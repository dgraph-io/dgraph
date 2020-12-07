package bulk

import (
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

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

	bulkCmd := exec.Command(testutil.DgraphBinaryPath(), "bulk",
		"-f", rdfFile,
		"-s", schemaFile,
		"--http", "",
		"-j=1",
		"--store-xids=true",
		"--zero", testutil.SockAddrZero,
	)
	bulkCmd.Dir = rootDir
	log.Print(bulkCmd.String())
	if out, err := bulkCmd.CombinedOutput(); err != nil {
		glog.Error("Error %v", err)
		glog.Error("Output %v", out)
		os.Exit(1)
	}

	cmd := exec.Command("docker-compose", "-f", "./alpha.yml", "-p", testutil.DockerPrefix,
		"up", "-d", "--force-recreate", "alpha1,alpha2,alpha3")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		glog.Infof("Error while bringing up alpha node. Prefix: %s. Error: %v\n", testutil.DockerPrefix, err)
		os.Exit(1)
	}
	for i := 0; i < 30; i++ {
		time.Sleep(time.Second)
		resp, err := http.Get(testutil.ContainerAddr("alpha1", 8080) + "/health")
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		if err == nil && resp.StatusCode == http.StatusOK {
			return
		}
	}

	exitCode := m.Run()
	_ = os.RemoveAll(rootDir)
	os.Exit(exitCode)
}
