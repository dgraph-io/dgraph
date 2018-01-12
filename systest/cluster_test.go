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

	// Approx time for snapshot to be transferred to the second instance.
	time.Sleep(time.Minute)

	cluster.dgraph.Process.Signal(syscall.SIGINT)
	if _, err = cluster.dgraph.Process.Wait(); err != nil {
		shutdownCluster()
		t.Fatalf("Error while waiting for Dgraph process to be killed: %v", err)
	}

	cluster.dgraph.Process = nil
	if err := cluster.dgraph.Start(); err != nil {
		shutdownCluster()
		t.Fatalf("Couldn't start Dgraph server again: %v\n", err)
	}

	// Wait for leader election to happen again.
	time.Sleep(15 * time.Second)

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
	fmt.Println(string(b), "port", o+8080)
	expected := `{"code": "Success", "message": "Export completed."}`
	if string(b) != expected {
		shutdownCluster()
		t.Fatalf("Unexpected message while exporting: %v", string(b))
	}

	dataFile, err := findFile(filepath.Join(dgraphDir, "export"), ".rdf.gz")
	cmd := fmt.Sprintf("zcat %s | wc -l", dataFile)
	out, err := exec.Command("sh", "-c", cmd).Output()
	quickCheck(err)
	if string(out) != "1120879\n" {
		shutdownCluster()
		t.Fatal("Export count mismatch. Got: %s", string(out))
	}

	schemaFile, err := findFile(filepath.Join(dgraphDir, "export"), ".schema.gz")
	cmd = fmt.Sprintf("zcat %s | wc -l", schemaFile)
	out, err = exec.Command("sh", "-c", cmd).Output()
	quickCheck(err)
	if string(out) != "10\n" {
		shutdownCluster()
		t.Fatal("Schema export count mismatch. Got: %s", string(out))
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
