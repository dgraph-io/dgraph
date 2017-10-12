package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/crc64"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
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
	t    *testing.T
	kill []*exec.Cmd

	bulkQueryPort string
	liveQueryPort string
	liveGRPCPort  string
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
		bulkDir = filepath.Join(rootDir, "bulk_dir")
		bulkDGZ = filepath.Join(rootDir, "bulk_dgz")
		bulkDG  = filepath.Join(rootDir, "bulk_dg")
		liveDir = filepath.Join(rootDir, "live_dir")
		liveDGZ = filepath.Join(rootDir, "live_dgz")
		liveDG  = filepath.Join(rootDir, "live_dg")
	)
	s.checkFatal(
		makeDirEmpty(bulkDir),
		makeDirEmpty(bulkDGZ),
		makeDirEmpty(bulkDG),
		makeDirEmpty(liveDir),
		makeDirEmpty(liveDGZ),
		makeDirEmpty(liveDG),
	)

	bulkCmd := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph-bulk-loader"), "-r", rdfFile,
		"-s", schemaFile, "-http", ":"+freePort(), "-j=1", "-x=true")
	bulkCmd.Dir = bulkDir
	if out, err := bulkCmd.CombinedOutput(); err != nil {
		s.cleanup()
		s.t.Fatalf("Bulkloader didn't run: %v\nOutput:\n%s", err, string(out))
	}
	s.checkFatal(os.Rename(
		filepath.Join(bulkDir, "out", "0", "p"),
		filepath.Join(bulkDG, "p"),
	))

	s.bulkQueryPort, _ = s.startDgraph(bulkDG, bulkDGZ)

	s.liveQueryPort, s.liveGRPCPort = s.startDgraph(liveDG, liveDGZ)

	liveCmd := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph-live-loader"), "-r", rdfFile,
		"-s", schemaFile, "-d", ":"+s.liveGRPCPort, "-x=true")
	liveCmd.Dir = liveDir
	if out, err := liveCmd.CombinedOutput(); err != nil {
		s.cleanup()
		s.t.Fatalf("Live Loader didn't run: %v\nOutput:\n%s", err, string(out))
	}
}

func makeDirEmpty(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.MkdirAll(dir, 0755)
}

func (s *suite) startDgraph(dgraphDir, dgraphZeroDir string) (queryPort string, grpcPort string) {
	port := freePort()
	zeroCmd := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraphzero"), "-idx", "1", "-port", port)
	zeroCmd.Dir = dgraphZeroDir
	zeroCmd.Stdout = os.Stdout
	zeroCmd.Stderr = os.Stderr
	s.checkFatal(zeroCmd.Start())
	s.kill = append(s.kill, zeroCmd)

	// Wait for dgraphzero to start listening.
	time.Sleep(time.Second * 1)

	queryPort = freePort()
	grpcPort = freePort()
	dgraphCmd := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"),
		"-memory_mb=1024",
		"-peer", ":"+port,
		"-port", queryPort,
		"-grpc_port", grpcPort,
		"-workerport", freePort(),
	)
	dgraphCmd.Dir = dgraphDir
	dgraphCmd.Stdout = os.Stdout
	dgraphCmd.Stderr = os.Stderr
	s.checkFatal(dgraphCmd.Start())
	s.kill = append(s.kill, dgraphCmd)

	// Wait for dgraph to start accepting requests. TODO: Could do this
	// programmatically by hitting the query port. This would be quicker than
	// just waiting 4 seconds (which seems to be the smallest amount of time to
	// reliably wait).
	time.Sleep(time.Second * 4)

	return queryPort, grpcPort
}

func (s *suite) cleanup() {
	// NOTE: Shouldn't raise any errors here or fail a test, since this is
	// called when we detect an error (don't want to mask the original problem).
	for _, k := range s.kill {
		_ = k.Process.Kill()
	}
	_ = os.RemoveAll(rootDir)
}

func (s *suite) singleQuery(query, wantResult string) func(*testing.T) {
	return s.multiQuery(
		fmt.Sprintf(`{ %s }`, query),
		fmt.Sprintf(`{ "data" : { %s } }`, wantResult),
	)
}

func (s *suite) multiQuery(query, wantResult string) func(*testing.T) {
	return func(t *testing.T) {
		for _, qPort := range []string{s.bulkQueryPort, s.liveQueryPort} {
			resp, err := http.Post("http://127.0.0.1:"+qPort+"/query",
				"", bytes.NewBufferString(query))
			if err != nil {
				t.Fatal("Could not post:", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatal("Bad response:", resp.Status)
			}

			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				t.Fatal("Could not read response:", err)
			}

			compareJSON(t, wantResult, string(body))
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

func compareJSON(t *testing.T, want, got string) {
	wantMap := map[string]interface{}{}
	err := json.Unmarshal([]byte(want), &wantMap)
	if err != nil {
		t.Fatalf("Could not unmarshal want JSON: %v", err)
	}
	gotMap := map[string]interface{}{}
	err = json.Unmarshal([]byte(got), &gotMap)
	if err != nil {
		t.Fatalf("Could not unmarshal got JSON: %v", err)
	}

	sortJSON(wantMap)
	sortJSON(gotMap)

	if !reflect.DeepEqual(wantMap, gotMap) {
		wantBuf, err := json.MarshalIndent(wantMap, "", "  ")
		if err != nil {
			t.Error("Could not marshal JSON:", err)
		}
		gotBuf, err := json.MarshalIndent(gotMap, "", "  ")
		if err != nil {
			t.Error("Could not marshal JSON:", err)
		}
		t.Errorf("Want JSON and Got JSON not equal\nWant:\n%v\nGot:\n%v",
			string(wantBuf), string(gotBuf))
	}
}

// sortJSON looks for any arrays in the unmarshalled JSON and sorts them in an
// arbitrary but deterministic order based on their content.
func sortJSON(i interface{}) uint64 {
	if i == nil {
		return 0
	}
	switch i := i.(type) {
	case map[string]interface{}:
		return sortJSONMap(i)
	case []interface{}:
		return sortJSONArray(i)
	default:
		h := crc64.New(crc64.MakeTable(crc64.ISO))
		fmt.Fprint(h, i)
		return h.Sum64()
	}
}

func sortJSONMap(m map[string]interface{}) uint64 {
	h := uint64(0)
	for _, k := range m {
		// Because xor is commutative, it doesn't matter that map iteration
		// is in random order.
		h ^= sortJSON(k)
	}
	return h
}

type arrayElement struct {
	elem   interface{}
	sortBy uint64
}

func sortJSONArray(a []interface{}) uint64 {
	h := uint64(0)
	elements := make([]arrayElement, len(a))
	for i, elem := range a {
		elements[i] = arrayElement{elem, sortJSON(elem)}
		h ^= elements[i].sortBy
	}
	sort.Slice(elements, func(i, j int) bool {
		return elements[i].sortBy < elements[j].sortBy
	})
	for i := range a {
		a[i] = elements[i].elem
	}
	return h
}
