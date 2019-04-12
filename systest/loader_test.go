/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TODO: Convert this to Docker based test.
func TestLoaderXidmap(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "")
	check(t, err)
	defer os.RemoveAll(tmpDir)

	cluster := NewDgraphCluster(tmpDir)
	check(t, cluster.Start())
	defer cluster.Close()

	data := os.ExpandEnv("$GOPATH/src/github.com/dgraph-io/dgraph/systest/data/first.rdf.gz")
	liveCmd := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"), "live",
		"--files", data,
		"--alpha", ":"+cluster.alphaPort,
		"--zero", ":"+cluster.zeroPort,
		"-x", "x",
	)
	liveCmd.Dir = tmpDir
	if err := liveCmd.Run(); err != nil {
		cluster.Close()
		t.Fatalf("Live Loader didn't run: %v\n", err)
	}

	// Load another file, live should reuse the xidmap.
	data = os.ExpandEnv("$GOPATH/src/github.com/dgraph-io/dgraph/systest/data/second.rdf.gz")
	liveCmd = exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"), "live",
		"--files", data,
		"--alpha", ":"+cluster.alphaPort,
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
	// cluster.dgraph.Process.Signal(syscall.SIGINT)
	// if _, err = cluster.dgraph.Process.Wait(); err != nil {
	// 	cluster.Close()
	// 	t.Fatalf("Error while waiting for Dgraph process to be killed: %v", err)
	// }

	// cluster.dgraph.Process = nil
	// if err := cluster.dgraph.Start(); err != nil {
	// 	cluster.Close()
	// 	t.Fatalf("Couldn't start Dgraph alpha again: %v\n", err)
	// }
	// time.Sleep(5 * time.Second)

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/admin/export", cluster.alphaPortOffset+8080))
	if err != nil {
		cluster.Close()
		t.Fatalf("Error while calling export: %v", err)
	}

	b, _ := ioutil.ReadAll(resp.Body)
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

	expected = `<0x1> <age> "13" .
<0x1> <friend> <0x2711> .
<0x1> <location> "Wonderland" .
<0x1> <name> "Alice" .
<0x2711> <name> "Bob" .
`

	if string(out) != expected {
		cluster.Close()
		t.Fatalf("Export is not as expected. Want:%v\nGot:%v\n", expected, string(out))
	}
}
