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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/z"
)

func TestLoaderXidmap(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "loader_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	data := os.ExpandEnv("$GOPATH/src/github.com/dgraph-io/dgraph/systest/data/first.rdf.gz")
	liveCmd := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"), "live",
		"--files", data,
		"--alpha", z.SockAddr,
		"--zero", z.SockAddrZero,
		"-x", "x",
	)
	liveCmd.Dir = tmpDir
	require.NoError(t, liveCmd.Run())

	// Load another file, live should reuse the xidmap.
	data = os.ExpandEnv("$GOPATH/src/github.com/dgraph-io/dgraph/systest/data/second.rdf.gz")
	liveCmd = exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"), "live",
		"--files", data,
		"--alpha", z.SockAddr,
		"--zero", z.SockAddrZero,
		"-x", "x",
	)
	liveCmd.Dir = tmpDir
	liveCmd.Stdout = os.Stdout
	liveCmd.Stderr = os.Stdout
	require.NoError(t, liveCmd.Run())

	resp, err := http.Get(fmt.Sprintf("http://%s/admin/export", z.SockAddrHttp))
	require.NoError(t, err)

	b, _ := ioutil.ReadAll(resp.Body)
	expected := `{"code": "Success", "message": "Export completed."}`
	require.Equal(t, expected, string(b))

	require.NoError(t, copyExportFiles(tmpDir))

	dataFile, err := findFile(filepath.Join(tmpDir, "export"), ".rdf.gz")
	require.NoError(t, err)

	cmd := fmt.Sprintf("gunzip -c %s | sort", dataFile)
	out, err := exec.Command("sh", "-c", cmd).Output()
	require.NoError(t, err)

	expected = `<0x1> <age> "13" .
<0x1> <friend> <0x2711> .
<0x1> <location> "Wonderland" .
<0x1> <name> "Alice" .
<0x2711> <name> "Bob" .
`
	require.Equal(t, expected, string(out))
}

func copyExportFiles(tmpDir string) error {
	exportPath := filepath.Join(tmpDir, "export")
	if err := os.MkdirAll(exportPath, 0755); err != nil {
		return err
	}

	srcPath := "alpha1:/data/alpha1/export"
	dstPath := filepath.Join(tmpDir, "export")
	return z.DockerCp(srcPath, dstPath)
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
