//go:build integration

/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
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
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/dgraph-io/badger/v4/options"
	"github.com/dgraph-io/dgo/v250"
	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/systest/backup/common"
	"github.com/dgraph-io/dgraph/v25/testutil"
	"github.com/dgraph-io/dgraph/v25/worker"
	"github.com/dgraph-io/dgraph/v25/x"
)

var (
	copyBackupDir  = "./data/backups_copy"
	restoreDir     = "./data/restore"
	alphaBackupDir = "/data/backups"
	oldBackupDir1  = "/data/to_restore/1"
	oldBackupDir2  = "/data/to_restore/2"
	oldBackupDir3  = "/data/to_restore/3"
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
}

// This test takes a backup and then restores an old backup in a cluster incrementally.
// Next, cleans up the cluster and tries restoring the backups above.
// Regression test for DGRAPH-2775
func TestBackupOfOldRestore(t *testing.T) {
	common.DirSetup(t)
	common.CopyOldBackupDir(t)

	conn, err := grpc.NewClient(testutil.GetSockAddr(), grpc.WithTransportCredentials(credentials.NewTLS(testutil.GetAlphaClientConfig(t))))
	require.NoError(t, err)
	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))
	require.NoError(t, err)

	testutil.DropAll(t, dg)
	time.Sleep(2 * time.Second)

	_ = runBackup(t, 3, 1)

	sendRestoreRequest(t, oldBackupDir1)
	testutil.WaitForRestore(t, dg, testutil.GetSockAddrHttp())

	q := `{ authors(func: has(Author.name)) { count(uid) } }`
	resp, err := dg.NewTxn().Query(context.Background(), q)
	require.NoError(t, err)
	require.JSONEq(t, "{\"authors\":[{\"count\":1}]}", string(resp.Json))

	_ = runBackup(t, 6, 2)

	// Clean the cluster and try restoring the backups created above.
	testutil.DropAll(t, dg)
	time.Sleep(2 * time.Second)
	sendRestoreRequest(t, alphaBackupDir)
	testutil.WaitForRestore(t, dg, testutil.GetSockAddrHttp())

	resp, err = dg.NewTxn().Query(context.Background(), q)
	require.NoError(t, err)
	require.JSONEq(t, "{\"authors\":[{\"count\":1}]}", string(resp.Json))
}

// This test restores the old backups.
// The backup dir contains:
// - Full backup with pred "p1", "p2", "p3". (insert k1, k2, k3).
// - Incremental backup after drop data was called and "p2", "p3", "p4" inserted. --> (insert k4,k5)
// - Incremental backup after "p3" was dropped.
func TestRestoreOfOldBackup(t *testing.T) {
	test := func(dir string) {
		common.DirSetup(t)
		common.CopyOldBackupDir(t)

		conn, err := grpc.NewClient(testutil.GetSockAddr(),
			grpc.WithTransportCredentials(credentials.NewTLS(testutil.GetAlphaClientConfig(t))))
		require.NoError(t, err)
		dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))
		require.NoError(t, err)

		testutil.DropAll(t, dg)
		time.Sleep(2 * time.Second)

		sendRestoreRequest(t, dir)
		testutil.WaitForRestore(t, dg, testutil.GetSockAddrHttp())

		queryAndCheck := func(pred string, cnt int) {
			q := fmt.Sprintf(`{ me(func: has(%s)) { count(uid) } }`, pred)
			r := fmt.Sprintf("{\"me\":[{\"count\":%d}]}", cnt)
			resp, err := dg.NewTxn().Query(context.Background(), q)
			require.NoError(t, err)
			require.JSONEq(t, r, string(resp.Json))
		}

		queryAndCheck("p1", 0)
		queryAndCheck("p2", 2)
		queryAndCheck("p3", 0)
		queryAndCheck("p4", 2)
	}
	t.Run("backup of 20.11", func(t *testing.T) { test(oldBackupDir2) })
	t.Run("backup of 21.03", func(t *testing.T) { test(oldBackupDir3) })
}

func TestBackupFilesystem(t *testing.T) {
	conn, err := grpc.NewClient(testutil.GetSockAddr(),
		grpc.WithTransportCredentials(credentials.NewTLS(testutil.GetAlphaClientConfig(t))))
	require.NoError(t, err)
	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))

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
	client := testutil.GetHttpsClient(t)
	_, err = client.Get("https://" + testutil.GetSockAddrZeroHttp() + "/moveTablet?tablet=movie&group=1")
	require.NoError(t, err)

	// After the move, we need to pause a bit to give zero a chance to quorum.
	t.Log("Pausing to let zero move tablet...")
	moveOk := false
	for retry := 5; retry > 0; retry-- {
		state, err := testutil.GetStateHttps(testutil.GetAlphaClientConfig(t))
		require.NoError(t, err)
		if _, ok := state.Groups["1"].Tablets[x.NamespaceAttr(x.RootNamespace, "movie")]; ok {
			moveOk = true
			break
		}
		time.Sleep(1 * time.Second)
	}

	require.True(t, moveOk)
	// Setup test directories.
	common.DirSetup(t)

	// Send backup request.
	_ = runBackup(t, 3, 1)
	restored := runRestore(t, copyBackupDir, "", math.MaxUint64)

	// Check the predicates and types in the schema are as expected.
	// TODO: refactor tests so that minio and filesystem tests share most of their logic.
	preds := []string{"dgraph.graphql.schema", "name", "dgraph.graphql.xid", "dgraph.type",
		"movie", "dgraph.graphql.p_query", "dgraph.drop.op", "dgraph.namespace.name", "dgraph.namespace.id"}
	types := []string{"Node", "dgraph.graphql", "dgraph.namespace", "dgraph.graphql.persisted_query"}
	testutil.CheckSchema(t, preds, types)

	verifyUids := func(count int) {
		query := `
		{
			me(func: eq(name, "ibrahim")) {
				count(uid)
			}
		}`
		res, err := dg.NewTxn().Query(context.Background(), query)
		require.NoError(t, err)
		require.JSONEq(t, string(res.GetJson()), fmt.Sprintf(`{"me":[{"count":%d}]}`, count))
	}
	verifyUids(10000)

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
	_ = runBackupInternal(t, true, 12, 4)
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

	verifyUids(10000)

	// Do a DROP_DATA
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropOp: api.Operation_DATA}))

	// add some data
	incr4, err := dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
				<_:x1> <movie> "El laberinto del fauno" .
				<_:x2> <movie> "Black Panther 2" .
			`),
	})
	require.NoError(t, err)

	// perform an incremental backup and then restore
	dirs := runBackup(t, 15, 5)
	restored = runRestore(t, copyBackupDir, "", incr4.Txn.CommitTs)
	testutil.CheckSchema(t, preds, types)

	// Check that the newly added data is the only data for the movie predicate
	require.Len(t, restored, 2)
	checks = []struct {
		blank, expected string
	}{
		{blank: "x1", expected: "El laberinto del fauno"},
		{blank: "x2", expected: "Black Panther 2"},
	}
	for _, check := range checks {
		require.EqualValues(t, check.expected, restored[incr4.Uids[check.blank]])
	}

	// Verify that there is no data for predicate `name`
	verifyUids(0)

	// Remove the full backup testDirs and verify restore catches the error.
	require.NoError(t, os.RemoveAll(dirs[0]))
	require.NoError(t, os.RemoveAll(dirs[3]))
	common.RunFailingRestore(t, copyBackupDir, "", incr4.Txn.CommitTs)

	// Clean up test directories.
	common.DirCleanup(t)
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
				}
				taskId
			}
		}`

	adminUrl := "https://" + testutil.GetSockAddrHttp() + "/admin"
	params := testutil.GraphQLParams{
		Query: backupRequest,
		Variables: map[string]interface{}{
			"dst": alphaBackupDir,
			"ff":  forceFull,
		},
	}
	b, err := json.Marshal(params)
	require.NoError(t, err)

	client := testutil.GetHttpsClient(t)
	resp, err := client.Post(adminUrl, "application/json", bytes.NewBuffer(b))
	require.NoError(t, err)
	defer resp.Body.Close()

	var data interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data))
	require.Equal(t, "Success",
		testutil.JsonGet(data, "data", "backup", "response", "code").(string))
	taskId := testutil.JsonGet(data, "data", "backup", "taskId").(string)
	testutil.WaitForTask(t, taskId, true, testutil.GetSockAddrHttp())

	// Verify that the right amount of files and directories were created.
	common.CopyToLocalFs(t)

	files := x.WalkPathFunc(copyBackupDir, func(path string, isdir bool) bool {
		return !isdir && strings.HasSuffix(path, ".backup") &&
			strings.HasPrefix(path, "data/backups_copy/dgraph.")
	})
	require.Equal(t, numExpectedFiles, len(files))

	dirs := x.WalkPathFunc(copyBackupDir, func(path string, isdir bool) bool {
		return isdir && strings.HasPrefix(path, "data/backups_copy/dgraph.")
	})
	require.Equal(t, numExpectedDirs, len(dirs))

	b, err = os.ReadFile(filepath.Join(copyBackupDir, "manifest.json"))
	require.NoError(t, err)
	var manifest worker.MasterManifest
	require.NoError(t, json.Unmarshal(b, &manifest))
	require.Equal(t, numExpectedDirs, len(manifest.Manifests))

	return dirs
}

// TestBackupSummaryManifest verifies that a backup writes manifest_summary.json
// alongside manifest.json, that the summary omits predicate groups, and that
// ListBackupManifests uses the summary when it is available.
func TestBackupSummaryManifest(t *testing.T) {
	common.DirSetup(t)
	defer common.DirCleanup(t)

	conn, err := grpc.NewClient(testutil.GetSockAddr(),
		grpc.WithTransportCredentials(credentials.NewTLS(testutil.GetAlphaClientConfig(t))))
	require.NoError(t, err)
	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))

	ctx := context.Background()
	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))
	require.NoError(t, dg.Alter(ctx, &api.Operation{
		Schema: `name: string @index(hash) .`,
	}))
	_, err = dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`<_:x1> <name> "summary-test" .`),
	})
	require.NoError(t, err)

	// Take a single full backup.
	_ = runBackup(t, 3, 1)

	// manifest_summary.json must be present alongside manifest.json.
	summaryFile := filepath.Join(copyBackupDir, "manifest_summary.json")
	require.FileExists(t, summaryFile, "manifest_summary.json was not created by backup")

	raw, err := os.ReadFile(summaryFile)
	require.NoError(t, err)

	var summary worker.MasterManifestSummary
	require.NoError(t, json.Unmarshal(raw, &summary), "manifest_summary.json is not valid JSON")
	require.Equal(t, 1, len(summary.Manifests))
	require.Equal(t, "full", summary.Manifests[0].Type)

	// Raw JSON must not contain the groups or drop_operations keys.
	rawStr := string(raw)
	require.NotContains(t, rawStr, `"groups"`)
	require.NotContains(t, rawStr, `"drop_operations"`)

	// ListBackupManifests should prefer the summary and return manifests without groups.
	manifests, err := worker.ListBackupManifests(copyBackupDir, nil, false)
	require.NoError(t, err)
	require.Equal(t, 1, len(manifests))
	require.Nil(t, manifests[0].Groups,
		"ListBackupManifests should return nil Groups when using summary manifest")

	// The full manifest.json must still be intact and readable (restore path unchanged).
	fullManifestFile := filepath.Join(copyBackupDir, "manifest.json")
	require.FileExists(t, fullManifestFile)
	rawFull, err := os.ReadFile(fullManifestFile)
	require.NoError(t, err)
	var fullMaster worker.MasterManifest
	require.NoError(t, json.Unmarshal(rawFull, &fullMaster))
	require.Equal(t, 1, len(fullMaster.Manifests))
	require.NotNil(t, fullMaster.Manifests[0].Groups, "full manifest.json must retain Groups for restore")
}

// listBackupsEntry mirrors the GraphQL Manifest type fields used in filter tests.
type listBackupsEntry struct {
	BackupId  string  `json:"backupId"`
	BackupNum float64 `json:"backupNum"`
	Type      string  `json:"type"`
	Path      string  `json:"path"`
	Encrypted bool    `json:"encrypted"`
	Since     float64 `json:"since"`
	Groups    []struct {
		GroupId    float64  `json:"groupId"`
		Predicates []string `json:"predicates"`
	} `json:"groups"`
}

// setupListBackupsFixture copies the pre-built testdata manifest files into the
// alpha container's backup directory. The test does not need to take any real backups.
func setupListBackupsFixture(t *testing.T) {
	t.Helper()
	common.DirSetup(t)
	t.Cleanup(func() { common.DirCleanup(t) })

	alpha := testutil.DockerPrefix + "_alpha1_1"
	require.NoError(t, testutil.DockerCp("testdata/manifest.json",
		alpha+":/data/backups/manifest.json"))
	require.NoError(t, testutil.DockerCp("testdata/manifest_summary.json",
		alpha+":/data/backups/manifest_summary.json"))
}

// makeListBackupsRunner returns a closure that posts a listBackups query with the
// given extra input fields and returns the decoded entries.
func makeListBackupsRunner(t *testing.T) func(extraInput string) []listBackupsEntry {
	t.Helper()
	adminUrl := "https://" + testutil.GetSockAddrHttp() + "/admin"
	client := testutil.GetHttpsClient(t)

	return func(extraInput string) []listBackupsEntry {
		t.Helper()
		query := fmt.Sprintf(`query {
			listBackups(input: {location: "%s"%s}) {
				backupId backupNum type path encrypted since
				groups { groupId predicates }
			}
		}`, alphaBackupDir, extraInput)
		params := testutil.GraphQLParams{Query: query}
		b, err := json.Marshal(params)
		require.NoError(t, err)
		resp, err := client.Post(adminUrl, "application/json", bytes.NewBuffer(b))
		require.NoError(t, err)
		defer resp.Body.Close()

		var result struct {
			Data struct {
				ListBackups []listBackupsEntry `json:"listBackups"`
			} `json:"data"`
			Errors []struct{ Message string } `json:"errors"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		require.Empty(t, result.Errors, "GraphQL errors: %v", result.Errors)
		return result.Data.ListBackups
	}
}

// makeListBackupsErrorRunner returns a closure that posts a listBackups query and
// returns the GraphQL error messages. Use this for inputs that should be rejected.
func makeListBackupsErrorRunner(t *testing.T) func(extraInput string) []string {
	t.Helper()
	adminUrl := "https://" + testutil.GetSockAddrHttp() + "/admin"
	client := testutil.GetHttpsClient(t)

	return func(extraInput string) []string {
		t.Helper()
		query := fmt.Sprintf(`query {
			listBackups(input: {location: "%s"%s}) {
				backupId
			}
		}`, alphaBackupDir, extraInput)
		params := testutil.GraphQLParams{Query: query}
		b, err := json.Marshal(params)
		require.NoError(t, err)
		resp, err := client.Post(adminUrl, "application/json", bytes.NewBuffer(b))
		require.NoError(t, err)
		defer resp.Body.Close()
		var result struct {
			Errors []struct{ Message string } `json:"errors"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		msgs := make([]string, len(result.Errors))
		for i, e := range result.Errors {
			msgs[i] = e.Message
		}
		return msgs
	}
}

// TestListBackupsInputErrors exercises GraphQL-level validation errors for listBackups.
func TestListBackupsInputErrors(t *testing.T) {
	setupListBackupsFixture(t)
	runExpectError := makeListBackupsErrorRunner(t)

	t.Run("lastNDays and sinceDate together return error", func(t *testing.T) {
		errs := runExpectError(`, lastNDays: 7, sinceDate: "2024-01-01"`)
		require.NotEmpty(t, errs, "expected a GraphQL error for lastNDays+sinceDate")
		require.Contains(t, errs[0], "sinceDate")
		require.Contains(t, errs[0], "lastNDays")
	})

	t.Run("negative lastNDays returns error", func(t *testing.T) {
		errs := runExpectError(`, lastNDays: -1`)
		require.NotEmpty(t, errs, "expected a GraphQL error for negative lastNDays")
		require.Contains(t, errs[0], "lastNDays")
	})

	t.Run("invalid sinceDate returns error", func(t *testing.T) {
		errs := runExpectError(`, sinceDate: "not-a-date"`)
		require.NotEmpty(t, errs, "expected a GraphQL error for invalid sinceDate")
		require.Contains(t, errs[0], "sinceDate")
	})
}

// TestListBackupsGraphQL is an E2E smoke test: take a real backup, then verify
// it appears in the listBackups query response.
func TestListBackupsGraphQL(t *testing.T) {
	common.DirSetup(t)
	defer common.DirCleanup(t)

	conn, err := grpc.NewClient(testutil.GetSockAddr(),
		grpc.WithTransportCredentials(credentials.NewTLS(testutil.GetAlphaClientConfig(t))))
	require.NoError(t, err)
	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))
	ctx := context.Background()

	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))
	require.NoError(t, dg.Alter(ctx, &api.Operation{Schema: `movie: string @index(hash) .`}))
	_, err = dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`<_:x1> <movie> "Inception" .`),
	})
	require.NoError(t, err)

	_ = runBackup(t, 3, 1)

	_, err = dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`<_:x2> <movie> "Interstellar" .`),
	})
	require.NoError(t, err)
	_ = runBackupInternal(t, false, 6, 2)

	runListBackups := makeListBackupsRunner(t)
	entries := runListBackups("")
	require.Equal(t, 2, len(entries), "expected one full and one incremental backup")

	types := make(map[string]bool)
	for _, e := range entries {
		types[e.Type] = true
		require.NotEmpty(t, e.BackupId)
		require.NotEmpty(t, e.Path)
	}
	require.True(t, types["full"])
	require.True(t, types["incremental"])
}

// TestListBackupsGroupsAutoDetect verifies that a listBackups query which
// selects the "groups" field returns populated predicate data even without
// setting fullManifest: true. This exercises the auto-detection in the resolver
// that reads manifest.json whenever "groups" appears in the selection set,
// preserving backward compatibility for existing queries.
func TestListBackupsGroupsAutoDetect(t *testing.T) {
	common.DirSetup(t)
	defer common.DirCleanup(t)

	conn, err := grpc.NewClient(testutil.GetSockAddr(),
		grpc.WithTransportCredentials(credentials.NewTLS(testutil.GetAlphaClientConfig(t))))
	require.NoError(t, err)
	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))
	ctx := context.Background()

	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))
	require.NoError(t, dg.Alter(ctx, &api.Operation{Schema: `movie: string @index(hash) .`}))
	_, err = dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`<_:x1> <movie> "Inception" .`),
	})
	require.NoError(t, err)
	_ = runBackup(t, 3, 1)

	// Query with groups in the selection but NO fullManifest: true.
	// The resolver must auto-detect the "groups" field and read the full manifest.
	adminUrl := "https://" + testutil.GetSockAddrHttp() + "/admin"
	client := testutil.GetHttpsClient(t)
	query := fmt.Sprintf(`query {
		listBackups(input: {location: "%s"}) {
			backupId type
			groups { groupId predicates }
		}
	}`, alphaBackupDir)
	params := testutil.GraphQLParams{Query: query}
	b, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := client.Post(adminUrl, "application/json", bytes.NewBuffer(b))
	require.NoError(t, err)
	defer resp.Body.Close()

	var result struct {
		Data struct {
			ListBackups []listBackupsEntry `json:"listBackups"`
		} `json:"data"`
		Errors []struct{ Message string } `json:"errors"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	require.Empty(t, result.Errors, "unexpected GraphQL errors: %v", result.Errors)

	entries := result.Data.ListBackups
	require.Equal(t, 1, len(entries), "expected exactly one backup")
	require.NotEmpty(t, entries[0].Groups,
		"groups must be populated when 'groups' is in the selection set, even without fullManifest: true")

	// Confirm that at least one group contains the 'movie' predicate.
	found := false
	for _, g := range entries[0].Groups {
		for _, p := range g.Predicates {
			if strings.Contains(p, "movie") {
				found = true
			}
		}
	}
	require.True(t, found, "expected 'movie' predicate in groups")
}

// TestListBackupsFilters loads pre-built fixture manifests (testdata/manifest*.json)
// into the alpha container and exercises the full matrix of date-filter inputs.
//
// Fixture layout — 7 entries across 3 series:
//
//	series-alpha (2023): dgraph.20230115, dgraph.20230201
//	series-beta  (2024): dgraph.20240601, dgraph.20240615, dgraph.20240701
//	series-gamma (2026): dgraph.20260101, dgraph.20260315
func TestListBackupsFilters(t *testing.T) {
	setupListBackupsFixture(t)
	runListBackups := makeListBackupsRunner(t)

	pathsOf := func(entries []listBackupsEntry) map[string]bool {
		m := make(map[string]bool, len(entries))
		for _, e := range entries {
			m[e.Path] = true
		}
		return m
	}

	tests := []struct {
		name       string
		input      string // extra fields appended after location in the GraphQL input
		wantCount  int
		mustHave   []string // paths that must appear
		mustAbsent []string // paths that must NOT appear
	}{
		{
			name:      "no filter returns all 7 entries",
			wantCount: 7,
			mustHave: []string{
				"dgraph.20230115.120000.000",
				"dgraph.20230201.080000.000",
				"dgraph.20240601.000000.000",
				"dgraph.20240615.000000.000",
				"dgraph.20240701.000000.000",
				"dgraph.20260101.000000.000",
				"dgraph.20260315.000000.000",
			},
		},
		{
			name:      "sinceDate=2024-01-01 excludes 2023 series",
			input:     `, sinceDate: "2024-01-01"`,
			wantCount: 5,
			mustHave: []string{
				"dgraph.20240601.000000.000",
				"dgraph.20240615.000000.000",
				"dgraph.20240701.000000.000",
				"dgraph.20260101.000000.000",
				"dgraph.20260315.000000.000",
			},
			mustAbsent: []string{
				"dgraph.20230115.120000.000",
				"dgraph.20230201.080000.000",
			},
		},
		{
			// dgraph.20240601.000000.000 is at midnight exactly; one second later excludes it.
			// Contrast with sinceDate="2024-06-01" (YYYY-MM-DD) which would include it.
			name:      "sinceDate RFC3339 one second past midnight excludes the midnight entry",
			input:     `, sinceDate: "2024-06-01T00:00:01Z"`,
			wantCount: 4,
			mustHave: []string{
				"dgraph.20240615.000000.000",
				"dgraph.20240701.000000.000",
				"dgraph.20260101.000000.000",
				"dgraph.20260315.000000.000",
			},
			mustAbsent: []string{
				"dgraph.20240601.000000.000",
				"dgraph.20230115.120000.000",
			},
		},
		{
			name:      "untilDate=2023-12-31 returns 2023 series only",
			input:     `, untilDate: "2023-12-31"`,
			wantCount: 2,
			mustHave: []string{
				"dgraph.20230115.120000.000",
				"dgraph.20230201.080000.000",
			},
			mustAbsent: []string{
				"dgraph.20240601.000000.000",
				"dgraph.20260101.000000.000",
			},
		},
		{
			name:      "sinceDate=2024-06-01 untilDate=2024-06-30 returns June-2024 entries only",
			input:     `, sinceDate: "2024-06-01", untilDate: "2024-06-30"`,
			wantCount: 2,
			mustHave: []string{
				"dgraph.20240601.000000.000",
				"dgraph.20240615.000000.000",
			},
			mustAbsent: []string{
				"dgraph.20240701.000000.000",
				"dgraph.20230115.120000.000",
				"dgraph.20260101.000000.000",
			},
		},
		{
			name:      "untilDate=2024-06-30 returns 2023 series and June-2024",
			input:     `, untilDate: "2024-06-30"`,
			wantCount: 4,
			mustHave: []string{
				"dgraph.20230115.120000.000",
				"dgraph.20230201.080000.000",
				"dgraph.20240601.000000.000",
				"dgraph.20240615.000000.000",
			},
			mustAbsent: []string{
				"dgraph.20240701.000000.000",
				"dgraph.20260101.000000.000",
			},
		},
		{
			name:      "sinceDate=2024-06-15 returns from Jun-15 onward",
			input:     `, sinceDate: "2024-06-15"`,
			wantCount: 4,
			mustHave: []string{
				"dgraph.20240615.000000.000",
				"dgraph.20240701.000000.000",
				"dgraph.20260101.000000.000",
				"dgraph.20260315.000000.000",
			},
			mustAbsent: []string{
				"dgraph.20230115.120000.000",
				"dgraph.20240601.000000.000",
			},
		},
		{
			name:      "sinceDate=2026-01-01 returns 2026 series only",
			input:     `, sinceDate: "2026-01-01"`,
			wantCount: 2,
			mustHave: []string{
				"dgraph.20260101.000000.000",
				"dgraph.20260315.000000.000",
			},
			mustAbsent: []string{
				"dgraph.20230115.120000.000",
				"dgraph.20240601.000000.000",
			},
		},
		{
			name:      "sinceDate=2026-01-01 untilDate=2026-01-31 returns only Jan-2026 full backup",
			input:     `, sinceDate: "2026-01-01", untilDate: "2026-01-31"`,
			wantCount: 1,
			mustHave:  []string{"dgraph.20260101.000000.000"},
			mustAbsent: []string{
				"dgraph.20260315.000000.000",
				"dgraph.20240601.000000.000",
			},
		},
		{
			name:      "2025 gap returns 0 (no backups between 2024 and 2026)",
			input:     `, sinceDate: "2025-01-01", untilDate: "2025-12-31"`,
			wantCount: 0,
		},
		{
			name:      "sinceDate far in future returns 0",
			input:     `, sinceDate: "2099-01-01"`,
			wantCount: 0,
		},
		{
			name:      "untilDate before all backups returns 0",
			input:     `, untilDate: "2022-12-31"`,
			wantCount: 0,
		},
		// untilDate RFC3339: the fixture entry dgraph.20230115.120000.000 is at noon UTC.
		// A cutoff of 11:59:59Z is before noon so it is excluded; 12:00:00Z is not after noon so it is included.
		{
			name:       "untilDate RFC3339 strict cutoff excludes entry at exact noon",
			input:      `, untilDate: "2023-01-15T11:59:59Z"`,
			wantCount:  0,
			mustAbsent: []string{"dgraph.20230115.120000.000"},
		},
		{
			name:      "untilDate RFC3339 at noon includes the noon entry",
			input:     `, untilDate: "2023-01-15T12:00:00Z"`,
			wantCount: 1,
			mustHave:  []string{"dgraph.20230115.120000.000"},
			mustAbsent: []string{
				"dgraph.20230201.080000.000",
				"dgraph.20240601.000000.000",
			},
		},
		// lastNDays cases — fixture newest entry is 2026-03-15, oldest is 2023-01-15.
		// lastNDays: 1    → since yesterday; no fixture entry is that recent → 0.
		// lastNDays: 36500 ≈ 100 years back; safely covers all fixture entries regardless of when CI runs → 7.
		{
			name:      "lastNDays=1 returns 0 (no fixture entry within last day)",
			input:     `, lastNDays: 1`,
			wantCount: 0,
		},
		{
			name:      "lastNDays=36500 returns all 7 entries",
			input:     `, lastNDays: 36500`,
			wantCount: 7,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			entries := runListBackups(tc.input)
			require.Equal(t, tc.wantCount, len(entries),
				"wrong entry count for input %q", tc.input)

			paths := pathsOf(entries)
			for _, p := range tc.mustHave {
				require.True(t, paths[p], "expected path %q in result", p)
			}
			for _, p := range tc.mustAbsent {
				require.False(t, paths[p], "unexpected path %q in result", p)
			}
		})
	}
}

// TestListBackupsPrecommittedManifest exercises date filters against the real
// master manifest that is already committed to the repo at
// data/to_restore/3/manifest.json. That file contains 3 entries, all dated
// 2021-05-17, from backup series "quirky_kapitsa4". No live backup is required.
//
// All queries use fullManifest:true so the alpha reads manifest.json directly
// (no manifest_summary.json is uploaded for this test).
func TestListBackupsPrecommittedManifest(t *testing.T) {
	common.DirSetup(t)
	defer common.DirCleanup(t)

	alpha := testutil.DockerPrefix + "_alpha1_1"
	require.NoError(t, testutil.DockerCp(
		"./data/to_restore/3/manifest.json",
		alpha+":/data/backups/manifest.json"))

	runListBackups := makeListBackupsRunner(t)
	// Wrap to always pass fullManifest:true — no summary file is present.
	run := func(extra string) []listBackupsEntry {
		return runListBackups(`, fullManifest: true` + extra)
	}

	tests := []struct {
		name         string
		extra        string
		wantCount    int
		wantBackupId string // if non-empty, all entries must have this backupId
	}{
		{
			name:         "no date filter returns all 3 entries",
			wantCount:    3,
			wantBackupId: "quirky_kapitsa4",
		},
		{
			// 2021-05-17 is exactly the date of all entries; sinceDate is inclusive.
			name:      "sinceDate=2021-05-17 (exact boundary) returns all 3",
			extra:     `, sinceDate: "2021-05-17"`,
			wantCount: 3,
		},
		{
			// The day after all entries — should exclude everything.
			name:      "sinceDate=2021-05-18 excludes all entries",
			extra:     `, sinceDate: "2021-05-18"`,
			wantCount: 0,
		},
		{
			// untilDate end-of-day covers all three entries from 2021-05-17.
			name:      "untilDate=2021-05-17 (end-of-day) returns all 3",
			extra:     `, untilDate: "2021-05-17"`,
			wantCount: 3,
		},
		{
			// The day before all entries — should exclude everything.
			name:      "untilDate=2021-05-16 excludes all entries",
			extra:     `, untilDate: "2021-05-16"`,
			wantCount: 0,
		},
		{
			name:      "sinceDate=2021-01-01 untilDate=2021-12-31 returns all 3 (within 2021)",
			extra:     `, sinceDate: "2021-01-01", untilDate: "2021-12-31"`,
			wantCount: 3,
		},
		{
			name:      "sinceDate=2022-01-01 returns 0 (all entries are from 2021)",
			extra:     `, sinceDate: "2022-01-01"`,
			wantCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			entries := run(tc.extra)
			require.Equal(t, tc.wantCount, len(entries))
			for _, e := range entries {
				if tc.wantBackupId != "" {
					require.Equal(t, tc.wantBackupId, e.BackupId)
				}
				// fullManifest:true — groups must be present on entries that have them.
				// The to_restore/3 manifest has groups populated on all 3 entries.
				require.NotEmpty(t, e.Groups, "fullManifest:true must return groups for path %s", e.Path)
			}
		})
	}

	// Sanity-check type distribution: 1 full + 2 incremental.
	t.Run("type distribution is 1 full and 2 incremental", func(t *testing.T) {
		entries := run("")
		require.Equal(t, 3, len(entries))
		types := make(map[string]int)
		for _, e := range entries {
			types[e.Type]++
		}
		require.Equal(t, 1, types["full"])
		require.Equal(t, 2, types["incremental"])
	})
}

// TestListBackupsFullManifest verifies the fullManifest input parameter.
// It takes a real full backup (which causes the alpha to write both manifest.json
// and manifest_summary.json), then checks:
//   - default / fullManifest:false  → uses manifest_summary.json, groups are empty
//   - fullManifest:true             → uses manifest.json, groups are populated
func TestListBackupsFullManifest(t *testing.T) {
	common.DirSetup(t)
	defer common.DirCleanup(t)

	conn, err := grpc.NewClient(testutil.GetSockAddr(),
		grpc.WithTransportCredentials(credentials.NewTLS(testutil.GetAlphaClientConfig(t))))
	require.NoError(t, err)
	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))
	ctx := context.Background()

	require.NoError(t, dg.Alter(ctx, &api.Operation{DropAll: true}))
	require.NoError(t, dg.Alter(ctx, &api.Operation{Schema: `name: string @index(hash) .`}))
	_, err = dg.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`<_:x1> <name> "test-fullmanifest" .`),
	})
	require.NoError(t, err)

	_ = runBackup(t, 3, 1)

	// runListBackups includes "groups" in the selection set, which triggers the
	// server-side auto-escalation to the full manifest.json.  Use it only when
	// we actually want group data (fullManifest: true subtest).
	runListBackups := makeListBackupsRunner(t)

	// runSummaryOnly omits "groups" from the selection so the server uses the
	// lightweight manifest_summary.json and returns manifests with empty groups.
	adminURL := "https://" + testutil.GetSockAddrHttp() + "/admin"
	httpClient := testutil.GetHttpsClient(t)
	runSummaryOnly := func(extraInput string) []listBackupsEntry {
		t.Helper()
		query := fmt.Sprintf(`query {
			listBackups(input: {location: "%s"%s}) {
				backupId backupNum type path encrypted since
			}
		}`, alphaBackupDir, extraInput)
		params := testutil.GraphQLParams{Query: query}
		b, err := json.Marshal(params)
		require.NoError(t, err)
		resp, err := httpClient.Post(adminURL, "application/json", bytes.NewBuffer(b))
		require.NoError(t, err)
		defer resp.Body.Close()
		var result struct {
			Data struct {
				ListBackups []listBackupsEntry `json:"listBackups"`
			} `json:"data"`
			Errors []struct{ Message string } `json:"errors"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		require.Empty(t, result.Errors, "GraphQL errors: %v", result.Errors)
		return result.Data.ListBackups
	}

	t.Run("default uses summary manifest and returns empty groups", func(t *testing.T) {
		entries := runSummaryOnly("")
		require.Equal(t, 1, len(entries))
		// manifest_summary.json omits groups; the response contains no group data.
		require.Empty(t, entries[0].Groups,
			"summary path must not populate groups")
	})

	t.Run("fullManifest=false explicit same as default", func(t *testing.T) {
		entries := runSummaryOnly(`, fullManifest: false`)
		require.Equal(t, 1, len(entries))
		require.Empty(t, entries[0].Groups)
	})

	t.Run("fullManifest=true reads manifest.json and returns populated groups", func(t *testing.T) {
		entries := runListBackups(`, fullManifest: true`)
		require.Equal(t, 1, len(entries))
		require.NotEmpty(t, entries[0].Groups,
			"full manifest path must populate groups")
		// At least one group must carry predicates (the schema we applied above).
		hasPredicates := false
		for _, g := range entries[0].Groups {
			if len(g.Predicates) > 0 {
				hasPredicates = true
				break
			}
		}
		require.True(t, hasPredicates,
			"at least one group must contain predicates from the applied schema")
	})
}

func runRestore(t *testing.T, backupLocation, lastDir string, commitTs uint64) map[string]string {
	// Recreate the restore directory to make sure there's no previous data when
	// calling restore.
	require.NoError(t, os.RemoveAll(restoreDir))

	t.Logf("--- Restoring from: %q", backupLocation)
	result := worker.RunOfflineRestore(restoreDir, backupLocation,
		lastDir, "", nil, options.Snappy, 0)
	require.NoError(t, result.Err)

	for i, pdir := range []string{"p1", "p2", "p3"} {
		pdir = filepath.Join("./data/restore", pdir)
		groupId, err := x.ReadGroupIdFile(pdir)
		require.NoError(t, err)
		require.Equal(t, uint32(i+1), groupId)
	}

	pdir := "./data/restore/p1"
	restored, err := testutil.GetPredicateValues(pdir, x.AttrInRootNamespace("movie"), commitTs)
	require.NoError(t, err)
	t.Logf("--- Restored values: %+v\n", restored)
	return restored
}
