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
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	bpb "github.com/dgraph-io/badger/pb"
	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/ee/backup"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/z"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

var (
	backupDir  = "./data/backups"
	restoreDir = "./data/restore"
	dirs       = []string{backupDir, restoreDir}

	alphaBackupDir = "/data/backups"

	alphaContainers = []string{
		"alpha1",
		"alpha2",
		"alpha3",
	}
)

func TestBackupFilesystem(t *testing.T) {
	conn, err := grpc.Dial(z.SockAddr, grpc.WithInsecure())
	x.Check(err)
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
	time.Sleep(5 * time.Second)

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
	time.Sleep(5 * time.Second)

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
	time.Sleep(5 * time.Second)

	// Perform second full backup.
	dirs = runBackupInternal(t, true, 12, 4)
	// To make sure this backup contains all the data remove all the previous backups.
	require.NoError(t, os.RemoveAll(dirs[0]))
	require.NoError(t, os.RemoveAll(dirs[1]))
	require.NoError(t, os.RemoveAll(dirs[2]))
	// Also recreate the restore directory.
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

func getError(rc io.ReadCloser) error {
	defer rc.Close()
	b, err := ioutil.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("Read failed: %v", err)
	}
	if bytes.Contains(b, []byte("Error")) {
		return fmt.Errorf("%s", string(b))
	}
	return nil
}

func getState() (*z.StateResponse, error) {
	resp, err := http.Get("http://" + z.SockAddrZeroHttp + "/state")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if bytes.Contains(b, []byte("Error")) {
		return nil, fmt.Errorf("Failed to get state: %s", string(b))
	}

	var st z.StateResponse
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func getPValues(pdir, attr string, readTs uint64) (map[string]string, error) {
	opt := badger.DefaultOptions
	opt.Dir = pdir
	opt.ValueDir = pdir
	opt.TableLoadingMode = options.MemoryMap
	opt.ReadOnly = true
	db, err := badger.OpenManaged(opt)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	values := make(map[string]string)

	stream := db.NewStreamAt(math.MaxUint64)
	stream.ChooseKey = func(item *badger.Item) bool {
		pk := x.Parse(item.Key())
		switch {
		case pk.Attr != attr:
			return false
		case pk.IsSchema():
			return false
		}
		return pk.IsData()
	}
	stream.KeyToList = func(key []byte, it *badger.Iterator) (*bpb.KVList, error) {
		pk := x.Parse(key)
		pl, err := posting.ReadPostingList(key, it)
		if err != nil {
			return nil, err
		}
		var list bpb.KVList
		err = pl.Iterate(readTs, 0, func(p *pb.Posting) error {
			vID := types.TypeID(p.ValType)
			src := types.ValueForType(vID)
			src.Value = p.Value
			str, err := types.Convert(src, types.StringID)
			if err != nil {
				fmt.Println(err)
				return err
			}
			value := strings.TrimRight(str.Value.(string), "\x00")
			list.Kv = append(list.Kv, &bpb.KV{
				Key:   []byte(fmt.Sprintf("%#x", pk.Uid)),
				Value: []byte(value),
			})
			return nil
		})
		return &list, err
	}
	stream.Send = func(list *bpb.KVList) error {
		for _, kv := range list.Kv {
			values[string(kv.Key)] = string(kv.Value)
		}
		return nil
	}
	if err := stream.Orchestrate(context.Background()); err != nil {
		return nil, err
	}
	return values, err
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
	require.NoError(t, getError(resp.Body))
	time.Sleep(5 * time.Second)

	// Verify that the right amount of files and directories were created.
	copyToLocalFs()
	files := x.WalkPathFunc(backupDir, func(path string, isdir bool) bool {
		return !isdir && strings.HasSuffix(path, ".backup")
	})
	require.True(t, len(files) == numExpectedFiles)
	dirs := x.WalkPathFunc(backupDir, func(path string, isdir bool) bool {
		return isdir && strings.HasPrefix(path, "data/backups/dgraph.")
	})
	require.True(t, len(dirs) == numExpectedDirs)

	return dirs
}

func runRestore(t *testing.T, restoreDir string, commitTs uint64) map[string]string {
	t.Logf("--- Restoring from: %q", restoreDir)
	_, err := backup.RunRestore("./data/restore", restoreDir)
	require.NoError(t, err)

	restored, err := getPValues("./data/restore/p1", "movie", commitTs)
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
	x.Check(os.RemoveAll("./data"))
}

func copyToLocalFs() {
	cwd, err := os.Getwd()
	x.Check(err)

	for _, alpha := range alphaContainers {
		// Because docker cp does not support the * notation, the directory is copied
		// first to a temporary location in the local FS. Then all backup files are
		// combined under data/backups.
		tmpDir := "./data/backups/" + alpha
		srcPath := fmt.Sprintf("%s:/data/backups", alpha)
		x.Check(z.DockerCp(srcPath, tmpDir))

		paths, err := filepath.Glob(tmpDir + "/*")
		x.Check(err)

		opts := z.CmdOpts{
			Dir: cwd,
		}
		cpCmd := []string{"cp", "-r"}
		cpCmd = append(cpCmd, paths...)
		cpCmd = append(cpCmd, "./data/backups")

		x.Check(z.ExecWithOpts([]string{"ls", "./data/backups/"}, opts))
		x.Check(z.ExecWithOpts(cpCmd, opts))
		x.Check(z.ExecWithOpts([]string{"rm", "-r", tmpDir}, opts))
	}
}
