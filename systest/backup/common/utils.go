/*
 * Copyright 2021 Dgraph Labs, Inc. and Contributors *
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

package common

import (
	"fmt"
	"os"
	"testing"

	"github.com/dgraph-io/badger/v3/options"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

var (
	copyBackupDir   = "./data/backups_copy"
	restoreDir      = "./data/restore"
	testDirs        = []string{restoreDir}
	alphaBackupDir  = "/data/backups"
	oldBackupDir    = "/data/to_restore"
	alphaContainers = []string{
		"alpha1",
		"alpha2",
		"alpha3",
	}
)

// RunFailingRestore is like runRestore but expects an error during restore.
func RunFailingRestore(t *testing.T, backupLocation, lastDir string, commitTs uint64) {
	// Recreate the restore directory to make sure there's no previous data when
	// calling restore.
	require.NoError(t, os.RemoveAll(restoreDir))

	result := worker.RunRestore("./data/restore", backupLocation, lastDir, x.SensitiveByteSlice(nil), options.Snappy, 0)
	require.Error(t, result.Err)
	require.Contains(t, result.Err.Error(), "expected a BackupNum value of 1")
}

func DirSetup(t *testing.T) {
	// Clean up data from previous runs.
	DirCleanup(t)

	for _, dir := range testDirs {
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			t.Fatalf("Error creating directory: %s", err.Error())
		}
	}

	for _, alpha := range alphaContainers {
		cmd := []string{"mkdir", "-p", alphaBackupDir}
		if err := testutil.DockerExec(alpha, cmd...); err != nil {
			t.Fatalf("Error executing command in docker container: %s", err.Error())
		}
	}
}

func DirCleanup(t *testing.T) {
	if err := os.RemoveAll(restoreDir); err != nil {
		t.Fatalf("Error removing directory: %s", err.Error())
	}

	if err := os.RemoveAll(copyBackupDir); err != nil {
		t.Fatalf("Error removing directory: %s", err.Error())
	}

	cmd := []string{"bash", "-c", "rm -rf /data/backups/dgraph.*"}
	if err := testutil.DockerExec(alphaContainers[0], cmd...); err != nil {
		t.Fatalf("Error executing command in docker container: %s", err.Error())
	}
}

func CopyOldBackupDir(t *testing.T) {
	for i := 1; i < 4; i++ {
		destPath := fmt.Sprintf("%s_alpha%d_1:/data", testutil.DockerPrefix, i)
		srchPath := "." + oldBackupDir
		if err := testutil.DockerCp(srchPath, destPath); err != nil {
			t.Fatalf("Error copying files from docker container: %s", err.Error())
		}
	}
}

func CopyToLocalFs(t *testing.T) {
	// The original backup files are not accessible because docker creates all files in
	// the shared volume as the root user. This restriction is circumvented by using
	// "docker cp" to create a copy that is not owned by the root user.
	if err := os.RemoveAll(copyBackupDir); err != nil {
		t.Fatalf("Error removing directory: %s", err.Error())
	}
	srcPath := testutil.DockerPrefix + "_alpha1_1:/data/backups"
	if err := testutil.DockerCp(srcPath, copyBackupDir); err != nil {
		t.Fatalf("Error copying files from docker container: %s", err.Error())
	}
}
