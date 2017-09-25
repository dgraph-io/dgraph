package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/crc64"
	"io/ioutil"
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

func TestHelloWorld(t *testing.T) {
	s := setup(t, `
		name: string @index(term) .
	`, `
		_:pj <name> "Peter Jackson" .
		_:pp <name> "Peter Pan" .
	`)
	defer s.cleanup()
	t.Run("test case 1", s.strtest(`
	{
		q(func: anyofterms(name, "Peter")) {
			name
		}
	}
	`, `
	{
		"data": {
			"q": [
				{ "name": "Peter Pan" },
				{ "name": "Peter Jackson" }
			]
		}
	}
	`))
}

func init() {
	// TODO: Install the binaries.
}

type suite struct {
	t       *testing.T
	rootDir string
	kill    []*exec.Cmd

	bulkLoaderQueryPort string
	liveLoaderQueryPort string
	liveLoaderGRPCPort  string
}

func setup(t *testing.T, schema string, rdfs string) *suite {
	s := &suite{
		t:       t,
		rootDir: filepath.Join(os.TempDir(), "dgraph_systest"),
	}
	var (
		bulkloaderDir   = filepath.Join(s.rootDir, "bl_dir")
		bulkloaderDGZ   = filepath.Join(s.rootDir, "bl_dgz")
		bulkloaderDG    = filepath.Join(s.rootDir, "bl_dg")
		dgraphloaderDir = filepath.Join(s.rootDir, "dg_dir")
		dgraphloaderDGZ = filepath.Join(s.rootDir, "dg_dgz")
		dgraphloaderDG  = filepath.Join(s.rootDir, "dg_dg")
		dataDir         = filepath.Join(s.rootDir, "data")
	)
	s.checkFatal(
		os.RemoveAll(s.rootDir),
		os.MkdirAll(s.rootDir, 0755),
		os.MkdirAll(bulkloaderDir, 0755),
		os.MkdirAll(bulkloaderDGZ, 0755),
		os.MkdirAll(bulkloaderDG, 0755),
		os.MkdirAll(dgraphloaderDir, 0755),
		os.MkdirAll(dgraphloaderDGZ, 0755),
		os.MkdirAll(dgraphloaderDG, 0755),
		os.MkdirAll(dataDir, 0755),
	)

	rdfFile := filepath.Join(dataDir, "rdfs.rdf")
	schemaFile := filepath.Join(dataDir, "schema.txt")
	s.checkFatal(
		ioutil.WriteFile(rdfFile, []byte(rdfs), 0644),
		ioutil.WriteFile(schemaFile, []byte(schema), 0644),
	)

	blHTTPPort := freePort()
	blCmd := buildCmd("bulkloader", "-r", rdfFile, "-s", schemaFile, "-http", ":"+blHTTPPort)
	blCmd.Dir = bulkloaderDir
	if err := blCmd.Run(); err != nil {
		t.Fatalf("Bulkloader didn't run: %v\nOutput:\n%s", err, blCmd.Out.String())
	}
	s.checkFatal(os.Rename(
		filepath.Join(bulkloaderDir, "out", "0"),
		filepath.Join(bulkloaderDG, "p"),
	))

	s.bulkLoaderQueryPort, _ = s.startDgraph(bulkloaderDG, bulkloaderDGZ)

	s.liveLoaderQueryPort, s.liveLoaderGRPCPort = s.startDgraph(dgraphloaderDG, dgraphloaderDGZ)

	liveCmd := buildCmd("dgraphloader", "-r", rdfFile, "-s", schemaFile, "-d", ":"+s.liveLoaderGRPCPort)
	liveCmd.Dir = dgraphloaderDir
	if err := liveCmd.Run(); err != nil {
		t.Fatalf("Live Loader didn't run: %v\nOutput:\n%s", err, liveCmd.Out.String())
	}

	return s
}

func (s *suite) startDgraph(dgraphDir, dgraphZeroDir string) (queryPort string, grpcPort string) {
	port := freePort()
	zeroCmd := buildCmd("dgraphzero", "-id", "1", "-port", port)
	zeroCmd.Dir = dgraphZeroDir
	s.checkFatal(zeroCmd.Start())
	s.kill = append(s.kill, zeroCmd.Cmd)

	// Wait for dgraphzero to start listening.
	time.Sleep(time.Second * 1)

	queryPort = freePort()
	grpcPort = freePort()
	dgraphCmd := buildCmd("dgraph",
		"-memory_mb=1024",
		"-peer", ":"+port,
		"-port", queryPort,
		"-grpc_port", grpcPort,
		"-workerport", freePort(),
	)
	dgraphCmd.Dir = dgraphDir
	s.checkFatal(dgraphCmd.Start())
	s.kill = append(s.kill, dgraphCmd.Cmd)

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
	_ = os.RemoveAll(s.rootDir)
}

func (s *suite) strtest(query, wantResult string) func(*testing.T) {
	return func(t *testing.T) {
		for _, qPort := range []string{s.bulkLoaderQueryPort, s.liveLoaderQueryPort} {
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

type Cmd struct {
	*exec.Cmd
	Out *bytes.Buffer
}

func buildCmd(prog string, args ...string) Cmd {
	bin := filepath.Join(os.ExpandEnv("$GOPATH"), "bin", prog)
	cmd := exec.Command(bin, args...)
	out := new(bytes.Buffer)
	cmd.Stdout = out
	cmd.Stderr = out
	return Cmd{Cmd: cmd, Out: out}
}

func freePort() string {
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
