/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors *
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
	"context"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/ee/backup"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/z"
)

var (
	backupDir      = "./data/backups"
	localBackupDir = "./data/backups_local"
	restoreDir     = "./data/restore"
	dirs           = []string{restoreDir}

	alphaBackupDir = "/data/backups"

	alphaContainers = []string{
		"alpha1",
		"alpha2",
		"alpha3",
	}
)

func TestBackupFilesystem(t *testing.T) {
	conn, err := grpc.Dial(z.SockAddr, grpc.WithInsecure())
	require.NoError(t, err)
	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))

	// Add initial data.
	ctx := context.Background()
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))
	require.NoError(t, dg.Alter(ctx, &api.Operation{Schema: `movie: string .`}))
	original, err := dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			<_:x1> <movie> "BIRDS MAN OR (THE UNEXPECTED VIRTUE OF IGNORANCE)" .
			<_:x2> <movie> "Spotlight" .
			<_:x3> <movie> "Moonlight" .
			<_:x4> <movie> "THE SHAPE OF WATERLOO" .
			<_:x5> <movie> "BLACK PUNTER" .
		`),
	})
	require.NoError(t, err)
	t.Logf("--- Original uid mapping: %+v\n", original.Uids)

	// Move tablet to group 1 to avoid messes later.
	_, err = http.Get("http://" + z.SockAddrZeroHttp + "/moveTablet?tablet=movie&group=1")
	require.NoError(t, err)

	// After the move, we need to pause a bit to give zero a chance to quorum.
	t.Log("Pausing to let zero move tablet...")
	moveOk := false
	for retry := 5; retry > 0; retry-- {
		time.Sleep(3 * time.Second)
		state, err := z.GetState()
		require.NoError(t, err)
		if _, ok := state.Groups["1"].Tablets["movie"]; ok {
			moveOk = true
			break
		}
	}
	require.True(t, moveOk)

	// Setup test directories.
	dirSetup()

	// Send backup request.
	dirs := runBackup(t, 3, 1)
	restored := runRestore(t, dirs[0], math.MaxUint64)

	checks := []struct {
		blank, expected string
	}{
		{blank: "x1", expected: "BIRDS MAN OR (THE UNEXPECTED VIRTUE OF IGNORANCE)"},
		{blank: "x2", expected: "Spotlight"},
		{blank: "x3", expected: "Moonlight"},
		{blank: "x4", expected: "THE SHAPE OF WATERLOO"},
		{blank: "x5", expected: "BLACK PUNTER"},
	}
	for _, check := range checks {
		require.EqualValues(t, check.expected, restored[original.Uids[check.blank]])
	}

	// Add more data for the incremental backup.
	incr1, err := dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(fmt.Sprintf(`
			<%s> <movie> "Birdman or (The Unexpected Virtue of Ignorance)" .
			<%s> <movie> "The Shape of Waterloo" .
		`, original.Uids["x1"], original.Uids["x4"])),
	})
	t.Logf("%+v", incr1)
	require.NoError(t, err)

	// Perform first incremental backup.
	dirs = runBackup(t, 6, 2)
	restored = runRestore(t, dirs[1], incr1.Context.CommitTs)

	checks = []struct {
		blank, expected string
	}{
		{blank: "x1", expected: "Birdman or (The Unexpected Virtue of Ignorance)"},
		{blank: "x4", expected: "The Shape of Waterloo"},
	}
	for _, check := range checks {
		require.EqualValues(t, check.expected, restored[original.Uids[check.blank]])
	}

	// Add more data for a second incremental backup.
	incr2, err := dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(fmt.Sprintf(`
				<%s> <movie> "The Shape of Water" .
				<%s> <movie> "The Black Panther" .
			`, original.Uids["x4"], original.Uids["x5"])),
	})
	require.NoError(t, err)

	// Perform second incremental backup.
	dirs = runBackup(t, 9, 3)
	restored = runRestore(t, dirs[2], incr2.Context.CommitTs)

	checks = []struct {
		blank, expected string
	}{
		{blank: "x4", expected: "The Shape of Water"},
		{blank: "x5", expected: "The Black Panther"},
	}

	for _, check := range checks {
		require.EqualValues(t, check.expected, restored[original.Uids[check.blank]])
	}

	// Add more data for a second full backup.
	incr3, err := dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(fmt.Sprintf(`
				<%s> <movie> "El laberinto del fauno" .
				<%s> <movie> "Black Panther 2" .
			`, original.Uids["x4"], original.Uids["x5"])),
	})
	require.NoError(t, err)

	// Perform second full backup.
	dirs = runBackupInternal(t, true, 12, 4)
	// Recreate the restore directory to make sure there's no previous data when
	// calling restore.
	require.NoError(t, os.RemoveAll(restoreDir))
	require.NoError(t, os.MkdirAll(restoreDir, os.ModePerm))
	restored = runRestore(t, dirs[3], incr3.Context.CommitTs)

	// Check all the values were restored to their most recent value.
	checks = []struct {
		blank, expected string
	}{
		{blank: "x1", expected: "Birdman or (The Unexpected Virtue of Ignorance)"},
		{blank: "x2", expected: "Spotlight"},
		{blank: "x3", expected: "Moonlight"},
		{blank: "x4", expected: "El laberinto del fauno"},
		{blank: "x5", expected: "Black Panther 2"},
	}

	for _, check := range checks {
		require.EqualValues(t, check.expected, restored[original.Uids[check.blank]])
	}

	// Clean up test directories.
	dirCleanup()
}

func runBackup(t *testing.T, numExpectedFiles, numExpectedDirs int) []string {
	return runBackupInternal(t, false, numExpectedFiles, numExpectedDirs)
}

func runBackupInternal(t *testing.T, forceFull bool, numExpectedFiles,
	numExpectedDirs int) []string {
	forceFullStr := "false"
	if forceFull {
		forceFullStr = "true"
	}

	resp, err := http.PostForm("http://localhost:8180/admin/backup", url.Values{
		"destination": []string{alphaBackupDir},
		"force_full":  []string{forceFullStr},
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.NoError(t, z.GetError(resp.Body))
	time.Sleep(time.Second)

	// Verify that the right amount of files and directories were created.
	copyToLocalFs()
	files := x.WalkPathFunc(localBackupDir, func(path string, isdir bool) bool {
		return !isdir && strings.HasSuffix(path, ".backup")
	})
	require.True(t, len(files) == numExpectedFiles)
	dirs := x.WalkPathFunc(localBackupDir, func(path string, isdir bool) bool {
		return isdir && strings.HasPrefix(path, "data/backups_local/dgraph.")
	})
	require.True(t, len(dirs) == numExpectedDirs)

	return dirs
}

func runRestore(t *testing.T, restoreDir string, commitTs uint64) map[string]string {
	t.Logf("--- Restoring from: %q", restoreDir)
	_, err := backup.RunRestore("./data/restore", restoreDir)
	require.NoError(t, err)

	restored, err := z.GetPValues("./data/restore/p1", "movie", commitTs)
	require.NoError(t, err)
	t.Logf("--- Restored values: %+v\n", restored)

	return restored
}

func dirSetup() {
	// Clean up data from previous runs.
	dirCleanup()

	for _, dir := range dirs {
		x.Check(os.MkdirAll(dir, os.ModePerm))
	}

	for _, alpha := range alphaContainers {
		cmd := []string{"mkdir", "-p", alphaBackupDir}
		x.Check(z.DockerExec(alpha, cmd...))
	}
}

func dirCleanup() {
	x.Check(os.RemoveAll(restoreDir))
	x.Check(os.RemoveAll(localBackupDir))

	cmd := []string{"bash", "-c", "rm -rf /data/backups/dgraph.*"}
	x.Check(z.DockerExec(alphaContainers[0], cmd...))
}

func copyToLocalFs() {
	// The original backup files are not accessible because docker creates all files in
	// the shared volume as the root user. This restriction is circumvented by using
	// "docker cp" to create a copy that is not owned by the root user.
	x.Check(os.RemoveAll(localBackupDir))
	srcPath := "alpha1:/data/backups"
	x.Check(z.DockerCp(srcPath, localBackupDir))
}
