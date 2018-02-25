package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestLoaderXidmap(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "")
	check(t, err)
	defer os.RemoveAll(tmpDir)

	cluster := NewDgraphCluster(tmpDir)
	check(t, cluster.Start())
	defer cluster.Close()

	data := os.ExpandEnv("$GOPATH/src/github.com/dgraph-io/dgraph/systest/data/first.rdf.gz")
	liveCmd := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"), "live",
		"--rdfs", data,
		"--dgraph", ":"+cluster.dgraphPort,
		"--zero", ":"+cluster.zeroPort,
		"-x", "x",
	)
	liveCmd.Dir = tmpDir
	liveCmd.Stdout = os.Stdout
	liveCmd.Stderr = os.Stdout
	if err := liveCmd.Run(); err != nil {
		cluster.Close()
		t.Fatalf("Live Loader didn't run: %v\n", err)
	}

	// Load another file, live should reuse the xidmap.
	data = os.ExpandEnv("$GOPATH/src/github.com/dgraph-io/dgraph/systest/data/second.rdf.gz")
	liveCmd = exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"), "live",
		"--rdfs", data,
		"--dgraph", ":"+cluster.dgraphPort,
		"--zero", ":"+cluster.zeroPort,
		"-x", "x",
	)
	liveCmd.Dir = tmpDir
	liveCmd.Stdout = os.Stdout
	liveCmd.Stderr = os.Stdout
	if err := liveCmd.Run(); err != nil {
		cluster.Close()
		t.Fatalf("Live Loader didn't run: %v\n", err)
	}

	// Restart Dgraph before taking an export.
	cluster.dgraph.Process.Signal(syscall.SIGINT)
	if _, err = cluster.dgraph.Process.Wait(); err != nil {
		cluster.Close()
		t.Fatalf("Error while waiting for Dgraph process to be killed: %v", err)
	}

	cluster.dgraph.Process = nil
	if err := cluster.dgraph.Start(); err != nil {
		cluster.Close()
		t.Fatalf("Couldn't start Dgraph server again: %v\n", err)
	}
	time.Sleep(5 * time.Second)

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/admin/export", cluster.dgraphPortOffset+8080))
	if err != nil {
		cluster.Close()
		t.Fatalf("Error while calling export: %v", err)
	}

	b, err := ioutil.ReadAll(resp.Body)
	expected := `{"code": "Success", "message": "Export completed."}`
	if string(b) != expected {
		t.Fatalf("Unexpected message while exporting: %v", string(b))
	}

	dataFile, err := findFile(filepath.Join(tmpDir, "export"), ".rdf.gz")
	if err != nil {
		cluster.Close()
		t.Fatalf("While trying to find exported file: %v", err)
	}
	cmd := fmt.Sprintf("gunzip -c %s | sort", dataFile)
	out, err := exec.Command("sh", "-c", cmd).Output()
	if err != nil {
		cluster.Close()
		t.Fatalf("While trying to sort exported file: %v", err)
	}

	expected = `<_:uid1> <age> "13" .
<_:uid1> <friend> _:uid2711 .
<_:uid1> <location> "Wonderland" .
<_:uid1> <name> "Alice" .
<_:uid2711> <name> "Bob" .
`

	if string(out) != expected {
		cluster.Close()
		t.Fatalf("Export is not as expected.")
	}
}
