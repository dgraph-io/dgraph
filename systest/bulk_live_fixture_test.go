package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/pkg/errors"
)

func init() {
	if testing.Short() {
		return
	}
	for _, name := range []string{
		"dgraph-bulk-loader",
		"dgraph-live-loader",
		"dgraph",
		"dgraphzero",
	} {
		cmd := exec.Command("go", "install", "github.com/dgraph-io/dgraph/cmd/"+name)
		cmd.Env = os.Environ()
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Fatalf("Could not run %q: %s", cmd.Args, string(out))
		}
	}
}

var rootDir = filepath.Join(os.TempDir(), "dgraph_systest")

type suite struct {
	t *testing.T

	liveCluster *DgraphCluster
	bulkCluster *DgraphCluster
}

func newSuite(t *testing.T, schema, rdfs string) *suite {
	if testing.Short() {
		t.Skip("Skipping system test with long runtime.")
	}
	s := &suite{t: t}
	s.checkFatal(makeDirEmpty(rootDir))
	rdfFile := filepath.Join(rootDir, "rdfs.rdf")
	s.checkFatal(ioutil.WriteFile(rdfFile, []byte(rdfs), 0644))
	schemaFile := filepath.Join(rootDir, "schema.txt")
	s.checkFatal(ioutil.WriteFile(schemaFile, []byte(schema), 0644))
	s.setup(schemaFile, rdfFile)
	return s
}

func newSuiteFromFile(t *testing.T, schemaFile, rdfFile string) *suite {
	if testing.Short() {
		t.Skip("Skipping system test with long runtime.")
	}
	s := &suite{t: t}
	s.setup(schemaFile, rdfFile)
	return s
}

func (s *suite) setup(schemaFile, rdfFile string) {
	var (
		bulkDir = filepath.Join(rootDir, "bulk")
		liveDir = filepath.Join(rootDir, "live")
	)
	s.checkFatal(
		makeDirEmpty(bulkDir),
		makeDirEmpty(liveDir),
	)

	s.bulkCluster = NewDgraphCluster(bulkDir)
	s.checkFatal(s.bulkCluster.StartZeroOnly())

	bulkCmd := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph-bulk-loader"),
		"-r", rdfFile,
		"-s", schemaFile,
		"-http", ":"+freePort(),
		"-z", ":"+s.bulkCluster.zeroPort,
		"-j=1", "-x=true",
	)
	bulkCmd.Stdout = os.Stdout
	bulkCmd.Stderr = os.Stdout
	bulkCmd.Dir = bulkDir
	if err := bulkCmd.Run(); err != nil {
		s.cleanup()
		s.t.Fatalf("Bulkloader didn't run: %v\n", err)
	}
	s.bulkCluster.zero.Process.Kill()
	s.bulkCluster.zero.Wait()
	s.checkFatal(os.Rename(
		filepath.Join(bulkDir, "out", "0", "p"),
		filepath.Join(bulkDir, "p"),
	))

	s.liveCluster = NewDgraphCluster(liveDir)
	s.checkFatal(s.liveCluster.Start())
	s.checkFatal(s.bulkCluster.Start())

	liveCmd := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph-live-loader"),
		"-r", rdfFile,
		"-s", schemaFile,
		"-d", ":"+s.liveCluster.dgraphPort,
		"-z", ":"+s.liveCluster.zeroPort,
		"-c=1", // use only 1 concurrent transaction to avoid txn conflicts
		"-m=10000",
	)
	liveCmd.Dir = liveDir
	liveCmd.Stdout = os.Stdout
	liveCmd.Stderr = os.Stdout
	if err := liveCmd.Run(); err != nil {
		s.cleanup()
		s.t.Fatalf("Live Loader didn't run: %v\n", err)
	}

}

func makeDirEmpty(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.MkdirAll(dir, 0755)
}

func (s *suite) cleanup() {
	// NOTE: Shouldn't raise any errors here or fail a test, since this is
	// called when we detect an error (don't want to mask the original problem).
	if s.liveCluster != nil {
		s.liveCluster.Close()
	}
	if s.bulkCluster != nil {
		s.bulkCluster.Close()
	}
	_ = os.RemoveAll(rootDir)
}

func (s *suite) testCase(query, wantResult string) func(*testing.T) {
	return func(t *testing.T) {
		for _, cluster := range []*DgraphCluster{s.bulkCluster, s.liveCluster} {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			txn := cluster.client.NewTxn()
			resp, err := txn.Query(ctx, query, nil)
			if err != nil {
				t.Fatalf("Could not query: %v", err)
			}
			CompareJSON(t, wantResult, string(resp.GetJson()))
		}
	}
}

func (s *suite) checkFatal(errs ...error) {
	for _, err := range errs {
		err = errors.Wrapf(err, "") // Add a stack.
		if err != nil {
			s.cleanup()
			s.t.Fatalf("%+v", err)
		}
	}
}

func check(t *testing.T, err error) {
	err = errors.Wrapf(err, "") // Add a stack.
	if err != nil {
		t.Fatalf("%+v", err)
	}
}

func init() {
	rand.Seed(int64(time.Now().Nanosecond()))
}

func freePort() string {
	// Linux reuses ports in FIFO order. So a port that we listen on and then
	// release will be free for a long time.
	for {
		p := 20000 + rand.Intn(40000)
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err == nil {
			listener.Close()
			return strconv.Itoa(p)
		}
	}
}
