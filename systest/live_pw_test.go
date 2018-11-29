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
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func liveExportPassword(t *testing.T, topDir string) {
	tmpDir := filepath.Join(topDir, "test1")
	check(t, os.Mkdir(tmpDir, 0700))
	cluster := NewDgraphCluster(tmpDir)
	check(t, cluster.Start())
	defer cluster.Close()
	time.Sleep(2 * time.Second)

	schemaFile := os.ExpandEnv("$GOPATH/src/github.com/dgraph-io/dgraph/systest/data/secret.schema.gz")
	rdfFile := os.ExpandEnv("$GOPATH/src/github.com/dgraph-io/dgraph/systest/data/secret.rdf1.gz")
	liveCmd := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"), "live",
		"--rdfs", rdfFile,
		"--schema", schemaFile,
		"--dgraph", ":"+cluster.dgraphPort,
		"--zero", ":"+cluster.zeroPort,
	)
	liveCmd.Dir = tmpDir
	liveCmd.Stdout = os.Stdout
	liveCmd.Stderr = os.Stdout
	if err := liveCmd.Run(); err != nil {
		t.Errorf("Live Loader didn't run: %v\n", err)
	}

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/admin/export", cluster.dgraphPortOffset+8080))
	if err != nil {
		t.Errorf("Error while calling export: %v", err)
	}

	b, err := ioutil.ReadAll(resp.Body)
	expected := `{"code": "Success", "message": "Export completed."}`
	if string(b) != expected {
		t.Errorf("Unexpected message while exporting: %v", string(b))
	}
}

func liveImportPassword(t *testing.T, topDir string) {
	tmpDir := filepath.Join(topDir, "test2")
	check(t, os.Mkdir(tmpDir, 0700))
	cluster := NewDgraphCluster(tmpDir)
	check(t, cluster.Start())
	defer cluster.Close()
	time.Sleep(2 * time.Second)

	// fetch exported file from export test
	dataFile1, err := findFile(filepath.Join(topDir, "test1", "export"), ".rdf.gz")
	if err != nil {
		t.Errorf("While trying to find exported file: %v", err)
	}

	liveCmd := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"), "live",
		"--rdfs", dataFile1,
		"--dgraph", ":"+cluster.dgraphPort,
		"--zero", ":"+cluster.zeroPort,
	)
	liveCmd.Dir = tmpDir
	liveCmd.Stdout = os.Stdout
	liveCmd.Stderr = os.Stdout
	if err := liveCmd.Run(); err != nil {
		t.Errorf("Live Loader didn't run: %v\n", err)
	}

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/admin/export", cluster.dgraphPortOffset+8080))
	if err != nil {
		t.Errorf("Error while calling export: %v", err)
	}

	b, err := ioutil.ReadAll(resp.Body)
	expected := `{"code": "Success", "message": "Export completed."}`
	if string(b) != expected {
		t.Errorf("Unexpected message while exporting: %v", string(b))
	}

	out1, err := exec.Command(
		"sh", "-c", fmt.Sprint("gunzip -c ", dataFile1, " | sort")).Output()
	if err != nil {
		t.Errorf("While trying to sort exported file: %v", err)
	}

	dataFile2, err := findFile(filepath.Join(tmpDir, "export"), ".rdf.gz")
	if err != nil {
		t.Errorf("While trying to find exported file: %v", err)
	}
	out2, err := exec.Command(
		"sh", "-c", fmt.Sprint("gunzip -c ", dataFile2, " | sort")).Output()
	if err != nil {
		t.Errorf("While trying to sort exported file: %v", err)
	}

	if len(out1) == 0 || len(out2) == 0 || !bytes.Equal(out1, out2) {
		t.Errorf("Export is not as expected.\n--- Expected:\n%v\n--- Actual:\n%v",
			string(out1), string(out2))
	}
}

func TestLivePassword(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "")
	check(t, err)
	defer os.RemoveAll(tmpDir)

	t.Run("export", func(t *testing.T) {
		liveExportPassword(t, tmpDir)
	})

	t.Run("import", func(t *testing.T) {
		liveImportPassword(t, tmpDir)
	})
}
