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
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"

	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
)

var (
	copyBackupDir = "./data/backups_copy"
	restoreDir    = "./data/restore"
	testDirs      = []string{restoreDir}

	alphaBackupDir = "/data/backups"

	alphaContainers = []string{
		"alpha1",
		"alpha2",
		"alpha3",
	}
)

func getTlsConf(t *testing.T) *tls.Config {
	c := &x.TLSHelperConfig{
		CertRequired:     true,
		Cert:             "../../tls/live/client.liveclient.crt",
		Key:              "../../tls/live/client.liveclient.key",
		ServerName:       "alpha1",
		RootCACert:       "../../tls/live/ca.crt",
		UseSystemCACerts: true,
	}
	tlsConf, err := x.GenerateClientTLSConfig(c)
	require.NoError(t, err)
	return tlsConf
}

func getHttpClient(t *testing.T) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: getTlsConf(t),
		},
	}
}

func TestBackupFilesystem(t *testing.T) {
	conf := viper.GetViper()
	conf.Set("tls_cacert", "../../tls/live/ca.crt")
	conf.Set("tls_internal_port_enabled", true)
	conf.Set("tls_server_name", "alpha1")
	dg, err := testutil.DgraphClientWithCerts(testutil.SockAddr, conf)
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))

	// Add schema and types.
	require.NoError(t, dg.Alter(ctx, &api.Operation{Schema: `movie: string .
	 name: string @index(hash) .
     type Node {
         movie
     }`}))

	var buf bytes.Buffer
	for i := 0; i < 10000; i++ {
		buf.Write([]byte(fmt.Sprintf(`<_:x%d> <name> "ibrahim" .
		`, i)))
	}
	// Add initial data.
	_, err = dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: buf.Bytes(),
	})

	require.NoError(t, err)
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
	_, err = getHttpClient(t).Get("https://" + testutil.SockAddrZeroHttp + "/moveTablet?tablet=movie&group=1")
	require.NoError(t, err)

	// After the move, we need to pause a bit to give zero a chance to quorum.
	t.Log("Pausing to let zero move tablet...")
	moveOk := false
	for retry := 5; retry > 0; retry-- {
		time.Sleep(3 * time.Second)
		state, err := testutil.GetStateHttps(getTlsConf(t))
		require.NoError(t, err)
		if _, ok := state.Groups["1"].Tablets["movie"]; ok {
			moveOk = true
			break
		}
	}
	require.True(t, moveOk)

	// Setup test directories.
	dirSetup(t)

	// Send backup request.
	_ = runBackup(t, 3, 1)
	restored := runRestore(t, copyBackupDir, "", math.MaxUint64)

	// Check the predicates and types in the schema are as expected.
	// TODO: refactor tests so that minio and filesystem tests share most of their logic.
	preds := []string{"dgraph.graphql.schema", "dgraph.cors", "name", "dgraph.graphql.xid",
		"dgraph.type", "movie", "dgraph.graphql.schema_history", "dgraph.graphql.schema_created_at",
		"dgraph.graphql.p_query", "dgraph.graphql.p_sha256hash", "dgraph.drop.op"}
	types := []string{"Node", "dgraph.graphql", "dgraph.graphql.history", "dgraph.graphql.persisted_query"}
	testutil.CheckSchema(t, preds, types)

	verifyUids := func() {
		query := `
		{
			me(func: eq(name, "ibrahim")) {
				count(uid)
			}
		}`
		res, err := dg.NewTxn().Query(context.Background(), query)
		require.NoError(t, err)
		require.JSONEq(t, string(res.GetJson()), `{"me":[{"count":10000}]}`)
	}
	verifyUids()

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

	// Update schema and types to make sure updates to the schema are backed up.
	require.NoError(t, dg.Alter(ctx, &api.Operation{Schema: `
		movie: string .
		actor: string .
		name: string @index(hash) .
		type Node {
			movie
		}
		type NewNode {
			actor
		}`}))

	// Perform first incremental backup.
	_ = runBackup(t, 6, 2)
	restored = runRestore(t, copyBackupDir, "", incr1.Txn.CommitTs)

	// Check the predicates and types in the schema are as expected.
	preds = append(preds, "actor")
	types = append(types, "NewNode")
	testutil.CheckSchema(t, preds, types)

	// Perform some checks on the restored values.
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
	_ = runBackup(t, 9, 3)
	restored = runRestore(t, copyBackupDir, "", incr2.Txn.CommitTs)
	testutil.CheckSchema(t, preds, types)

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
	dirs := runBackupInternal(t, true, 12, 4)
	restored = runRestore(t, copyBackupDir, "", incr3.Txn.CommitTs)
	testutil.CheckSchema(t, preds, types)

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

	verifyUids()
	// Remove the full backup testDirs and verify restore catches the error.
	require.NoError(t, os.RemoveAll(dirs[0]))
	require.NoError(t, os.RemoveAll(dirs[3]))
	runFailingRestore(t, copyBackupDir, "", incr3.Txn.CommitTs)

	// Clean up test directories.
	dirCleanup(t)
}

func runBackup(t *testing.T, numExpectedFiles, numExpectedDirs int) []string {
	return runBackupInternal(t, false, numExpectedFiles, numExpectedDirs)
}

func runBackupInternal(t *testing.T, forceFull bool, numExpectedFiles,
	numExpectedDirs int) []string {
	backupRequest := `mutation backup($dst: String!, $ff: Boolean!) {
			backup(input: {destination: $dst, forceFull: $ff}) {
				response {
					code
					message
				}
			}
		}`

	adminUrl := "https://localhost:8180/admin"
	params := testutil.GraphQLParams{
		Query: backupRequest,
		Variables: map[string]interface{}{
			"dst": alphaBackupDir,
			"ff":  forceFull,
		},
	}
	b, err := json.Marshal(params)
	require.NoError(t, err)

	resp, err := getHttpClient(t).Post(adminUrl, "application/json", bytes.NewBuffer(b))
	require.NoError(t, err)
	defer resp.Body.Close()
	buf, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(buf), "Backup completed.")

	// Verify that the right amount of files and directories were created.
	copyToLocalFs(t)

	files := x.WalkPathFunc(copyBackupDir, func(path string, isdir bool) bool {
		return !isdir && strings.HasSuffix(path, ".backup")
	})
	require.Equal(t, numExpectedFiles, len(files))

	dirs := x.WalkPathFunc(copyBackupDir, func(path string, isdir bool) bool {
		return isdir && strings.HasPrefix(path, "data/backups_copy/dgraph.")
	})
	require.Equal(t, numExpectedDirs, len(dirs))

	manifests := x.WalkPathFunc(copyBackupDir, func(path string, isdir bool) bool {
		return !isdir && strings.Contains(path, "manifest.json")
	})
	require.Equal(t, numExpectedDirs, len(manifests))

	return dirs
}

func runRestore(t *testing.T, backupLocation, lastDir string, commitTs uint64) map[string]string {
	// Recreate the restore directory to make sure there's no previous data when
	// calling restore.
	require.NoError(t, os.RemoveAll(restoreDir))

	t.Logf("--- Restoring from: %q", backupLocation)
	result := worker.RunRestore("./data/restore", backupLocation, lastDir, x.SensitiveByteSlice(nil))
	require.NoError(t, result.Err)

	for i, pdir := range []string{"p1", "p2", "p3"} {
		pdir = filepath.Join("./data/restore", pdir)
		groupId, err := x.ReadGroupIdFile(pdir)
		require.NoError(t, err)
		require.Equal(t, uint32(i+1), groupId)
	}

	pdir := "./data/restore/p1"
	restored, err := testutil.GetPredicateValues(pdir, "movie", commitTs)
	require.NoError(t, err)
	t.Logf("--- Restored values: %+v\n", restored)
	return restored
}

// runFailingRestore is like runRestore but expects an error during restore.
func runFailingRestore(t *testing.T, backupLocation, lastDir string, commitTs uint64) {
	// Recreate the restore directory to make sure there's no previous data when
	// calling restore.
	require.NoError(t, os.RemoveAll(restoreDir))

	result := worker.RunRestore("./data/restore", backupLocation, lastDir, x.SensitiveByteSlice(nil))
	require.Error(t, result.Err)
	require.Contains(t, result.Err.Error(), "expected a BackupNum value of 1")
}

func dirSetup(t *testing.T) {
	// Clean up data from previous runs.
	dirCleanup(t)

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

func dirCleanup(t *testing.T) {
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

func copyToLocalFs(t *testing.T) {
	// The original backup files are not accessible because docker creates all files in
	// the shared volume as the root user. This restriction is circumvented by using
	// "docker cp" to create a copy that is not owned by the root user.
	if err := os.RemoveAll(copyBackupDir); err != nil {
		t.Fatalf("Error removing directory: %s", err.Error())
	}
	srcPath := "alpha1:/data/backups"
	if err := testutil.DockerCp(srcPath, copyBackupDir); err != nil {
		t.Fatalf("Error copying files from docker container: %s", err.Error())
	}
}
