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
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dgraph-io/badger/v3/options"
	"github.com/dgraph-io/dgo/v200"
	"github.com/dgraph-io/dgo/v200/protos/api"
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

func sendRestoreRequest(t *testing.T, location string) {
	if location == "" {
		location = "/data/backup"
	}
	params := testutil.GraphQLParams{
		Query: `mutation restore($location: String!) {
			restore(input: {location: $location}) {
				code
				message
			}
		}`,
		Variables: map[string]interface{}{
			"location": location,
		},
	}
	resp := testutil.MakeGQLRequestWithTLS(t, &params, testutil.GetAlphaClientConfig(t))
	resp.RequireNoGraphQLErrors(t)

	var restoreResp struct {
		Restore struct {
			Code    string
			Message string
		}
	}

	require.NoError(t, json.Unmarshal(resp.Data, &restoreResp))
	require.Equal(t, restoreResp.Restore.Code, "Success")
	return
}

func TestBackupMultiTenancy(t *testing.T) {
	ctx := context.Background()

	dg := testutil.DgClientWithLogin(t, "groot", "password", x.GalaxyNamespace)
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))

	galaxyCreds := &testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: x.GalaxyNamespace}
	galaxyToken := testutil.Login(t, galaxyCreds)

	// Create a new namespace
	ns, err := testutil.CreateNamespace(t, galaxyToken)
	require.NoError(t, err)
	dg1 := testutil.DgClientWithLogin(t, "groot", "password", ns)

	addSchema := func(dg *dgo.Dgraph) {
		// Add schema and types.
		require.NoError(t, dg.Alter(ctx, &api.Operation{Schema: `movie: string .
	 name: string @index(hash) .
     type Node {
         movie
     }`}))
	}

	addSchema(dg)
	addSchema(dg1)

	addData := func(dg *dgo.Dgraph, name string) *api.Response {
		var buf bytes.Buffer
		for i := 0; i < 10000; i++ {
			buf.Write([]byte(fmt.Sprintf(`<_:x%d> <name> "%s" .
		`, i, name)))
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
		return original
	}

	original := make(map[uint64]*api.Response)
	original[x.GalaxyNamespace] = addData(dg, "galaxy")
	original[ns] = addData(dg1, "ns")

	// Setup test directories.
	dirSetup(t)

	// Send backup request.
	_ = runBackup(t, galaxyToken, 3, 1)
	restored := runRestore(t, copyBackupDir, "", math.MaxUint64, []uint64{x.GalaxyNamespace, ns})

	preds := []string{"dgraph.graphql.schema", "dgraph.cors", "name", "dgraph.graphql.xid",
		"dgraph.type", "movie", "dgraph.graphql.schema_history", "dgraph.graphql.schema_created_at",
		"dgraph.graphql.p_query", "dgraph.graphql.p_sha256hash", "dgraph.drop.op",
		"dgraph.xid", "dgraph.acl.rule", "dgraph.password", "dgraph.user.group", "dgraph.rule.predicate", "dgraph.rule.permission"} // ACL
	preds = append(preds, preds...)
	types := []string{"Node", "dgraph.graphql", "dgraph.graphql.history",
		"dgraph.graphql.persisted_query", "dgraph.type.cors",
		"dgraph.type.Rule", "dgraph.type.User", "dgraph.type.Group"} // ACL
	types = append(types, types...)
	testutil.CheckSchema(t, preds, types)

	verifyUids := func(dg *dgo.Dgraph, name string, count int) {
		query := fmt.Sprintf(`
		{
			me(func: eq(name, "%s")) {
				count(uid)
			}
		}`, name)
		res, err := dg.NewTxn().Query(context.Background(), query)
		require.NoError(t, err)
		require.JSONEq(t, string(res.GetJson()), fmt.Sprintf(`{"me":[{"count":%d}]}`, count))
	}
	verifyUids(dg, "galaxy", 10000)
	verifyUids(dg1, "ns", 10000)

	checks := []struct {
		blank, expected string
	}{
		{blank: "x1", expected: "BIRDS MAN OR (THE UNEXPECTED VIRTUE OF IGNORANCE)"},
		{blank: "x2", expected: "Spotlight"},
		{blank: "x3", expected: "Moonlight"},
		{blank: "x4", expected: "THE SHAPE OF WATERLOO"},
		{blank: "x5", expected: "BLACK PUNTER"},
	}
	for ns, orig := range original {
		for _, check := range checks {
			require.EqualValues(t, check.expected, restored[ns][orig.Uids[check.blank]])
		}
	}

	addMoreData := func(dg *dgo.Dgraph, ns uint64) *api.Response {
		// Add more data for the incremental backup.
		incr1, err := dg.NewTxn().Mutate(ctx, &api.Mutation{
			CommitNow: true,
			SetNquads: []byte(fmt.Sprintf(`
			<%s> <movie> "Birdman or (The Unexpected Virtue of Ignorance)" .
			<%s> <movie> "The Shape of Waterloo" .
		`, original[ns].Uids["x1"], original[ns].Uids["x4"])),
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
		return incr1
	}

	incr1 := make(map[uint64]*api.Response)
	incr1[x.GalaxyNamespace] = addMoreData(dg, x.GalaxyNamespace)
	incr1[ns] = addMoreData(dg1, ns)

	// Perform first incremental backup.
	_ = runBackup(t, galaxyToken, 6, 2)
	restored = runRestore(t, copyBackupDir, "",
		x.Max(incr1[x.GalaxyNamespace].Txn.CommitTs, incr1[ns].Txn.CommitTs), []uint64{x.GalaxyNamespace, ns})

	// Check the predicates and types in the schema are as expected.
	preds = append(preds, "actor", "actor")
	types = append(types, "NewNode", "NewNode")
	testutil.CheckSchema(t, preds, types)

	// Perform some checks on the restored values.
	checks = []struct {
		blank, expected string
	}{
		{blank: "x1", expected: "Birdman or (The Unexpected Virtue of Ignorance)"},
		{blank: "x4", expected: "The Shape of Waterloo"},
	}
	for ns, orig := range original {
		for _, check := range checks {
			require.EqualValues(t, check.expected, restored[ns][orig.Uids[check.blank]])
		}
	}

	addMoreData2 := func(dg *dgo.Dgraph, ns uint64) *api.Response {
		// Add more data for the incremental backup.
		incr2, err := dg.NewTxn().Mutate(ctx, &api.Mutation{
			CommitNow: true,
			SetNquads: []byte(fmt.Sprintf(`
				<%s> <movie> "The Shape of Water" .
				<%s> <movie> "The Black Panther" .
			`, original[ns].Uids["x4"], original[ns].Uids["x5"])),
		})
		require.NoError(t, err)
		return incr2
	}

	incr2 := make(map[uint64]*api.Response)
	incr2[x.GalaxyNamespace] = addMoreData2(dg, x.GalaxyNamespace)
	incr2[ns] = addMoreData2(dg1, ns)

	// Perform second incremental backup.
	_ = runBackup(t, galaxyToken, 9, 3)
	restored = runRestore(t, copyBackupDir, "",
		x.Max(incr2[x.GalaxyNamespace].Txn.CommitTs, incr2[ns].Txn.CommitTs), []uint64{x.GalaxyNamespace, ns})
	testutil.CheckSchema(t, preds, types)

	checks = []struct {
		blank, expected string
	}{
		{blank: "x4", expected: "The Shape of Water"},
		{blank: "x5", expected: "The Black Panther"},
	}
	for ns, orig := range original {
		for _, check := range checks {
			require.EqualValues(t, check.expected, restored[ns][orig.Uids[check.blank]])
		}
	}

	addMoreData3 := func(dg *dgo.Dgraph, ns uint64) *api.Response {
		// Add more data for the incremental backup.
		incr2, err := dg.NewTxn().Mutate(ctx, &api.Mutation{
			CommitNow: true,
			SetNquads: []byte(fmt.Sprintf(`
				<%s> <movie> "El laberinto del fauno" .
				<%s> <movie> "Black Panther 2" .
			`, original[ns].Uids["x4"], original[ns].Uids["x5"])),
		})
		require.NoError(t, err)
		return incr2
	}
	incr3 := make(map[uint64]*api.Response)
	incr3[x.GalaxyNamespace] = addMoreData3(dg, x.GalaxyNamespace)
	incr3[ns] = addMoreData3(dg1, ns)

	// Perform second full backup.
	_ = runBackupInternal(t, galaxyToken, true, 12, 4)
	restored = runRestore(t, copyBackupDir, "",
		x.Max(incr3[x.GalaxyNamespace].Txn.CommitTs, incr3[ns].Txn.CommitTs), []uint64{x.GalaxyNamespace, ns})
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
	for ns, orig := range original {
		for _, check := range checks {
			require.EqualValues(t, check.expected, restored[ns][orig.Uids[check.blank]])
		}
	}

	verifyUids(dg, "galaxy", 10000)
	verifyUids(dg1, "ns", 10000)

	// Do a DROP_DATA
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropOp: api.Operation_DATA}))
	verifyUids(dg, "galaxy", 0)
	verifyUids(dg1, "ns", 10000)

	// add some data in galaxy namespace.
	incr4, err := dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
				<_:x1> <movie> "El laberinto del fauno" .
				<_:x2> <movie> "Black Panther 2" .
			`),
	})
	require.NoError(t, err)
	original[x.GalaxyNamespace] = incr4

	// perform an incremental backup and then restore
	dirs := runBackup(t, galaxyToken, 15, 5)
	restored = runRestore(t, copyBackupDir, "", incr4.Txn.CommitTs, []uint64{x.GalaxyNamespace, ns})
	testutil.CheckSchema(t, preds, types)

	// Check that the newly added data is the only data for the movie predicate
	require.Len(t, restored[x.GalaxyNamespace], 2)
	checksUpdated := []struct {
		blank, expected string
	}{
		{blank: "x1", expected: "El laberinto del fauno"},
		{blank: "x2", expected: "Black Panther 2"},
	}
	for _, check := range checksUpdated {
		require.EqualValues(t, check.expected, restored[x.GalaxyNamespace][original[x.GalaxyNamespace].Uids[check.blank]])
	}
	require.Len(t, restored[ns], 5)
	for _, check := range checks {
		require.EqualValues(t, check.expected, restored[ns][original[ns].Uids[check.blank]])
	}

	// Verify that there is no data for predicate `name`
	verifyUids(dg, "galaxy", 0)
	verifyUids(dg1, "ns", 10000)

	// Do a DROP_DATA on namespace ns.
	require.NoError(t, dg1.Alter(ctx, &api.Operation{DropOp: api.Operation_DATA}))
	verifyUids(dg, "galaxy", 0)
	verifyUids(dg1, "ns", 0)

	// add some data in galaxy namespace.
	incr4, err = dg1.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
				<_:x1> <movie> "El laberinto del fauno" .
				<_:x2> <movie> "Black Panther 2" .
			`),
	})
	require.NoError(t, err)
	original[ns] = incr4

	// perform an incremental backup and then restore
	dirs = runBackup(t, galaxyToken, 18, 6)
	restored = runRestore(t, copyBackupDir, "", incr4.Txn.CommitTs, []uint64{x.GalaxyNamespace, ns})
	testutil.CheckSchema(t, preds, types)

	// Check that the newly added data is the only data for the movie predicate
	require.Len(t, restored[x.GalaxyNamespace], 2)
	require.Len(t, restored[ns], 2)
	for ns, orig := range original {
		for _, check := range checksUpdated {
			require.EqualValues(t, check.expected, restored[ns][orig.Uids[check.blank]])
		}
	}

	// Verify that there is no data for predicate `name`
	verifyUids(dg, "galaxy", 0)
	verifyUids(dg1, "ns", 0)

	// After deleting a namespace in incremental backup, we should not be able to get the data from
	// banned namespace.
	require.NoError(t, testutil.DeleteNamespace(t, galaxyToken, ns))
	dirs = runBackup(t, galaxyToken, 21, 7)
	restored = runRestore(t, copyBackupDir, "", math.MaxUint64, []uint64{x.GalaxyNamespace, ns})

	// Check that we do not restore the data from ns namespace.
	require.Len(t, restored[x.GalaxyNamespace], 2)
	require.Len(t, restored[ns], 0)

	// Remove the full backup testDirs and verify restore catches the error.
	require.NoError(t, os.RemoveAll(dirs[0]))
	require.NoError(t, os.RemoveAll(dirs[3]))
	runFailingRestore(t, copyBackupDir, "", incr4.Txn.CommitTs)

	// Clean up test directories.
	dirCleanup(t)
}

func runBackup(t *testing.T, token *testutil.HttpToken, numExpectedFiles, numExpectedDirs int) []string {
	return runBackupInternal(t, token, false, numExpectedFiles, numExpectedDirs)
}

func runBackupInternal(t *testing.T, token *testutil.HttpToken, forceFull bool, numExpectedFiles,
	numExpectedDirs int) []string {
	backupRequest := `mutation backup($dst: String!, $ff: Boolean!) {
			backup(input: {destination: $dst, forceFull: $ff}) {
				response {
					code
					message
				}
			}
		}`

	params := testutil.GraphQLParams{
		Query: backupRequest,
		Variables: map[string]interface{}{
			"dst": alphaBackupDir,
			"ff":  forceFull,
		},
	}
	resp := testutil.MakeRequest(t, token, params)
	var result struct {
		Backup struct {
			Response struct {
				Message, Code string
			}
		}
	}
	require.NoError(t, json.Unmarshal(resp.Data, &result))
	require.Contains(t, result.Backup.Response.Message, "Backup completed.")

	// Verify that the right amount of files and directories were created.
	copyToLocalFs(t)

	files := x.WalkPathFunc(copyBackupDir, func(path string, isdir bool) bool {
		return !isdir && strings.HasSuffix(path, ".backup") && strings.HasPrefix(path, "data/backups_copy/dgraph.")
	})
	require.Equal(t, numExpectedFiles, len(files))

	dirs := x.WalkPathFunc(copyBackupDir, func(path string, isdir bool) bool {
		return isdir && strings.HasPrefix(path, "data/backups_copy/dgraph.")
	})
	require.Equal(t, numExpectedDirs, len(dirs))

	manifests := x.WalkPathFunc(copyBackupDir, func(path string, isdir bool) bool {
		return !isdir && strings.Contains(path, "manifest.json") && strings.HasPrefix(path, "data/backups_copy/dgraph.")
	})
	require.Equal(t, numExpectedDirs, len(manifests))

	return dirs
}

func runRestore(t *testing.T, backupLocation, lastDir string, commitTs uint64,
	ns []uint64) map[uint64]map[string]string {
	// Recreate the restore directory to make sure there's no previous data when
	// calling restore.
	require.NoError(t, os.RemoveAll(restoreDir))

	t.Logf("--- Restoring from: %q", backupLocation)
	result := worker.RunRestore("./data/restore", backupLocation, lastDir, x.SensitiveByteSlice(nil), options.Snappy, 0)
	require.NoError(t, result.Err)

	for i, pdir := range []string{"p1", "p2", "p3"} {
		pdir = filepath.Join("./data/restore", pdir)
		groupId, err := x.ReadGroupIdFile(pdir)
		require.NoError(t, err)
		require.Equal(t, uint32(i+1), groupId)
	}

	restored := make(map[uint64]map[string]string)
	var err error
	pdir := "./data/restore/p1"
	for _, n := range ns {
		restored[n], err = testutil.GetPredicateValues(pdir, x.NamespaceAttr(n, "movie"), commitTs)
	}
	require.NoError(t, err)
	t.Logf("--- Restored values: %+v\n", restored)
	return restored
}

// runFailingRestore is like runRestore but expects an error during restore.
func runFailingRestore(t *testing.T, backupLocation, lastDir string, commitTs uint64) {
	// Recreate the restore directory to make sure there's no previous data when
	// calling restore.
	require.NoError(t, os.RemoveAll(restoreDir))

	result := worker.RunRestore("./data/restore", backupLocation, lastDir, x.SensitiveByteSlice(nil), options.Snappy, 0)
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
	srcPath := testutil.DockerPrefix + "_alpha1_1:/data/backups"
	if err := testutil.DockerCp(srcPath, copyBackupDir); err != nil {
		t.Fatalf("Error copying files from docker container: %s", err.Error())
	}
}
