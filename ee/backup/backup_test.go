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

package backup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestBackup(t *testing.T) {
	wrap := func(fn func(*testing.T, *dgo.Dgraph)) func(*testing.T) {
		return func(t *testing.T) {
			conn, err := grpc.Dial("localhost:9180", grpc.WithInsecure())
			x.Check(err)
			dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))
			fn(t, dg)
		}
	}

	t.Run("setup", wrap(BackupSetup))
	t.Run("full backup", wrap(BackupFull))
	// t.Run("full restore", wrap(RestoreFull))
	t.Run("incr backup", wrap(BackupIncr))
	// t.Run("incr restore", wrap(RestoreIncr))
	t.Run("cleanup", wrap(BackupCleanup))
}

var original *api.Assigned

// BackupSetup loads some data into the cluster so we can test backup.
func BackupSetup(t *testing.T, c *dgo.Dgraph) {
	var err error

	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{DropAll: true}))
	require.NoError(t, c.Alter(ctx, &api.Operation{Schema: `title: string .`}))
	original, err = c.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(`
			<_:x1> <title> "BIRDS MAN OR (THE UNEXPECTED VIRTUE OF IGNORANCE)" .
			<_:x2> <title> "Spotlight" .
			<_:x3> <title> "Moonlight" .
			<_:x4> <title> "THE SHAPE OF WATERLOO" .
			<_:x5> <title> "BLACK PUNTER" .
		`),
	})
	require.NoError(t, err)

	x.Check(os.Mkdir("./data/backups", 0777))
	x.Check(os.Mkdir("./data/restore", 0777))
}

func BackupCleanup(t *testing.T, c *dgo.Dgraph) {
	x.Check(os.RemoveAll("./data/backups"))
	x.Check(os.RemoveAll("./data/restore"))
}

func BackupFull(t *testing.T, c *dgo.Dgraph) {
	resp, err := http.PostForm("http://localhost:8180/admin/backup", url.Values{
		"destination": []string{"/data/backups"},
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.NoError(t, getError(resp.Body))
}

func RestoreFull(t *testing.T, c *dgo.Dgraph) {
	dirs := x.WalkPathFunc("./data/backups", func(path string, isdir bool) bool {
		return isdir && strings.HasPrefix(path, "data/backups/dgraph.")
	})
	require.True(t, len(dirs) == 1)
	sort.Strings(dirs)
	t.Logf("%+v", dirs)

	require.NoError(t, runRestore("./data/restore", dirs[0], 0))

	// verify
	bopt := badger.DefaultOptions
	bopt.Dir = "./data/restore/p1"
	bopt.ValueDir = bopt.Dir
	bopt.SyncWrites = false
	db, err := badger.Open(bopt)
	require.NoError(t, err)
	defer db.Close()

	// db.View(func(txn *badger.Txn) error {
	// 	opts := badger.DefaultIteratorOptions
	// 	opts.AllVersions = true
	// 	opts.PrefetchValues = false
	// 	it := txn.NewIterator(opts)
	// 	defer it.Close()
	// 	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
	// 		item := it.Item()
	// 	}
	// 	return nil
	// })
}

func BackupIncr(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	assigned, err := c.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(fmt.Sprintf(`
			<%s> <title> "Birdman or (The Unexpected Virtue of Ignorance)" .
			<%s> <title> "The Shape of Waterloo" .
		`, original.Uids["x1"], original.Uids["x4"])),
	})
	t.Logf("%+v", assigned)
	x.Check(err)

	resp, err := http.PostForm("http://localhost:8180/admin/backup", url.Values{
		"destination": []string{"/data/backups"},
		"at":          []string{fmt.Sprint(assigned.Context.StartTs)},
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.NoError(t, getError(resp.Body))

	assigned, err = c.NewTxn().Mutate(ctx, &api.Mutation{
		CommitNow: true,
		SetNquads: []byte(fmt.Sprintf(`
			<%s> <title> "The Shape of Water" .
			<%s> <title> "The Black Panther" .
		`, original.Uids["x4"], original.Uids["x5"])),
	})
	x.Check(err)

	resp, err = http.PostForm("http://localhost:8180/admin/backup", url.Values{
		"destination": []string{"/data/backups"},
		"at":          []string{fmt.Sprint(assigned.Context.StartTs)},
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.NoError(t, getError(resp.Body))
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
