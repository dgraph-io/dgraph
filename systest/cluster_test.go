package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
)

type Member struct {
	Id      string `json:"id"`
	GroupId int    `json:"groupId"`
	Leader  bool   `json:"leader"`
}

type GroupState struct {
	Members map[string]Member `json:"members"`
}

type State struct {
	Groups map[string]GroupState `json:"groups"`
}

func waitForConvergence(t *testing.T, c *DgraphCluster) {
	for i := 0; i < 60; i++ {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/state", c.zeroPortOffset+6080))
		require.NoError(t, err)
		b, err := ioutil.ReadAll(resp.Body)
		require.NoError(t, err)

		var s State
		require.NoError(t, json.Unmarshal(b, &s))
		members := s.Groups["1"].Members
		if len(members) == 2 && (members["1"].Leader || members["2"].Leader) {
			break
		}

		x.Println("Couldn't find leader, waiting...")
		time.Sleep(time.Second)
	}
}

type matchExport struct {
	expectedRDF    int
	expectedSchema int
	dir            string
	port           int
}

func matchExportCount(opts matchExport) error {
	// Now try and export data from second server.
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/admin/export", opts.port))
	if err != nil {
		return err
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	expected := `{"code": "Success", "message": "Export completed."}`
	if string(b) != expected {
		return x.Errorf("Unexpected message while exporting: %v", string(b))
	}

	dataFile, err := findFile(filepath.Join(opts.dir, "export"), ".rdf.gz")
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf("gunzip -c %s | wc -l", dataFile)
	out, err := exec.Command("sh", "-c", cmd).Output()
	if err != nil {
		return err
	}
	count := strings.TrimSpace(string(out))
	if count != strconv.Itoa(opts.expectedRDF) {
		return x.Errorf("Export count mismatch. Got: %s", count)
	}

	schemaFile, err := findFile(filepath.Join(opts.dir, "export"), ".schema.gz")
	if err != nil {
		return err
	}
	cmd = fmt.Sprintf("gunzip -c %s | wc -l", schemaFile)
	out, err = exec.Command("sh", "-c", cmd).Output()
	if err != nil {
		return err
	}
	count = strings.TrimSpace(string(out))
	if count != strconv.Itoa(opts.expectedSchema) {
		return x.Errorf("Schema export count mismatch. Got: %s", count)
	}
	return nil
}

func waitForNodeToBeHealthy(t *testing.T, port int) {
	for i := 0; i < 90; i++ {
		// Ignore error, server might be unhealthy temporarily.
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", port))
		if err != nil {
			x.Printf("Server running on: [%v] is not up yet, waiting...\n", port)
			time.Sleep(2 * time.Second)
			continue
		}
		b, err := ioutil.ReadAll(resp.Body)
		require.NoError(t, err)

		if string(b) == "OK" {
			break
		}

		x.Printf("Server running on: [%v] not healthy, retrying...\n", port)
		time.Sleep(2 * time.Second)
	}
}

func TestClusterSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}
	tmpDir, err := ioutil.TempDir("", "")
	check(t, err)
	defer os.RemoveAll(tmpDir)

	cluster := NewDgraphCluster(tmpDir)
	check(t, cluster.Start())
	defer cluster.Close()

	schema := os.ExpandEnv("$GOPATH/src/github.com/dgraph-io/dgraph/systest/data/goldendata.schema")
	data := os.ExpandEnv("$GOPATH/src/github.com/dgraph-io/dgraph/systest/data/goldendata_first_200k.rdf.gz")

	liveCmd := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"), "live",
		"--rdfs", data,
		"--schema", schema,
		"--dgraph", ":"+cluster.dgraphPort,
		"--zero", ":"+cluster.zeroPort,
	)
	liveCmd.Dir = tmpDir
	liveCmd.Stdout = os.Stdout
	liveCmd.Stderr = os.Stdout
	if err := liveCmd.Run(); err != nil {
		cluster.Close()
		t.Fatalf("Live Loader didn't run: %v\n", err)
	}

	// Start another Dgraph node.
	var dgraphDir = filepath.Join(tmpDir, "dgraph_2")
	n, err := cluster.AddNode(dgraphDir)

	shutdownCluster := func() {
		cluster.Close()
		n.process.Process.Kill()
	}
	defer shutdownCluster()

	if err != nil {
		shutdownCluster()
		t.Fatalf("Couldn't add server: %v\n", err)
	}

	quickCheck := func(err error) {
		if err != nil {
			shutdownCluster()
			t.Fatalf("Got error: %v\n", err)
		}
	}

	o, err := strconv.Atoi(n.offset)
	quickCheck(err)

	waitForNodeToBeHealthy(t, o+x.PortHTTP)

	cluster.dgraph.Process.Signal(syscall.SIGINT)
	if _, err = cluster.dgraph.Process.Wait(); err != nil {
		shutdownCluster()
		t.Fatalf("Error while waiting for Dgraph process to be killed: %v", err)
	}

	cluster.dgraph.Process = nil
	fmt.Println("Trying to restart Dgraph Server")
	if err := cluster.dgraph.Start(); err != nil {
		shutdownCluster()
		t.Fatalf("Couldn't start Dgraph server again: %v\n", err)
	}

	waitForNodeToBeHealthy(t, cluster.dgraphPortOffset+x.PortHTTP)
	waitForNodeToBeHealthy(t, o+x.PortHTTP)
	waitForConvergence(t, cluster)

	err = matchExportCount(matchExport{
		expectedRDF:    2e5,
		expectedSchema: 10,
		dir:            dgraphDir,
		port:           o + x.PortHTTP,
	})
	if err != nil {
		shutdownCluster()
		t.Fatal(err)
	}
}

func findFile(dir string, ext string) (string, error) {
	var fp string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(path, ext) {
			fp = path
			return nil
		}
		return nil
	})
	return fp, err
}
