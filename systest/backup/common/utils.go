/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors *
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

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/badger/v3/options"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/worker"
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

	result := worker.RunOfflineRestore(restoreDir, backupLocation, lastDir,
		"", nil, options.Snappy, 0)
	require.Error(t, result.Err)
	require.Contains(t, result.Err.Error(), "expected a BackupNum value of 1")
}

func DirSetup(t *testing.T) {
	// Clean up data from previous runs.
	DirCleanup(t)

	for _, dir := range testDirs {
		require.NoError(t, os.MkdirAll(dir, os.ModePerm))
	}

	for _, alpha := range alphaContainers {
		cmd := []string{"mkdir", "-p", alphaBackupDir}
		require.NoError(t, testutil.DockerExec(alpha, cmd...))
	}
}

func DirCleanup(t *testing.T) {
	require.NoError(t, os.RemoveAll(restoreDir))
	require.NoError(t, os.RemoveAll(copyBackupDir))

	cmd := []string{"bash", "-c", "rm -rf /data/backups/*"}
	require.NoError(t, testutil.DockerExec(alphaContainers[0], cmd...))
}

func CopyOldBackupDir(t *testing.T) {
	for i := 1; i < 4; i++ {
		destPath := fmt.Sprintf("%s_alpha%d_1:/data", testutil.DockerPrefix, i)
		srchPath := "." + oldBackupDir
		require.NoError(t, testutil.DockerCp(srchPath, destPath))
	}
}

func CopyToLocalFs(t *testing.T) {
	// The original backup files are not accessible because docker creates all files in
	// the shared volume as the root user. This restriction is circumvented by using
	// "docker cp" to create a copy that is not owned by the root user.
	require.NoError(t, os.RemoveAll(copyBackupDir))
	srcPath := testutil.DockerPrefix + "_alpha1_1:/data/backups"
	require.NoError(t, testutil.DockerCp(srcPath, copyBackupDir))
}

// to copy files fron nfs server
func CopyToLocalFsFromNFS(t *testing.T, backupDst string, copyBackupDirectory string) {
	// The original backup files are not accessible because docker creates all files in
	// the shared volume as the root user. This restriction is circumvented by using
	// "docker cp" to create a copy that is not owned by the root user.
	require.NoError(t, os.RemoveAll(copyBackupDirectory))
	srcPath := testutil.DockerPrefix + "_nfs_1:/data" + backupDst
	require.NoError(t, testutil.DockerCp(srcPath, copyBackupDirectory))
}
