package main

import (
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
)

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
	data := os.ExpandEnv("$GOPATH/src/github.com/dgraph-io/dgraph/systest/data/goldendata.rdf.gz")

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

	// TODO - Make this more deterministic.
	dur := 90 * time.Second
	// TODO - Remove later when we move nightly to Teamcity
	if ok := os.Getenv("TRAVIS"); ok == "true" {
		dur = 2 * dur
	}
	// Approx time for snapshot to be transferred to the second instance.
	time.Sleep(dur)

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

	dur = 15 * time.Second
	if ok := os.Getenv("TRAVIS"); ok == "true" {
		dur = 2 * dur
	}
	// Wait for leader election to happen again.
	time.Sleep(dur)

	quickCheck := func(err error) {
		if err != nil {
			shutdownCluster()
			t.Fatalf("Got error: %v\n", err)
		}
	}

	// Now try and export data from second server.
	o, err := strconv.Atoi(n.offset)
	quickCheck(err)
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/admin/export", o+8080))
	quickCheck(err)
	b, err := ioutil.ReadAll(resp.Body)
	quickCheck(err)
	expected := `{"code": "Success", "message": "Export completed."}`
	if string(b) != expected {
		shutdownCluster()
		t.Fatalf("Unexpected message while exporting: %v", string(b))
	}

	dataFile, err := findFile(filepath.Join(dgraphDir, "export"), ".rdf.gz")
	quickCheck(err)
	cmd := fmt.Sprintf("gunzip -c %s | wc -l", dataFile)
	out, err := exec.Command("sh", "-c", cmd).Output()
	quickCheck(err)
	count := strings.TrimSpace(string(out))
	if count != "1120879" {
		shutdownCluster()
		t.Fatalf("Export count mismatch. Got: %s", count)
	}

	schemaFile, err := findFile(filepath.Join(dgraphDir, "export"), ".schema.gz")
	quickCheck(err)
	cmd = fmt.Sprintf("gunzip -c %s | wc -l", schemaFile)
	out, err = exec.Command("sh", "-c", cmd).Output()
	quickCheck(err)
	count = strings.TrimSpace(string(out))
	if count != "10" {
		shutdownCluster()
		t.Fatalf("Schema export count mismatch. Got: %s", count)
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
