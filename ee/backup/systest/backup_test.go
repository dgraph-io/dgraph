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
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"testing"

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
	t.Run("test local backup", wrap(BackupLocal))
}

// BackupSetup loads some data into the cluster so we can test backup.
func BackupSetup(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{DropAll: true}))

	schema, err := ioutil.ReadFile(`data/goldendata.schema`)
	require.NoError(t, c.Alter(ctx, &api.Operation{Schema: string(schema)}))

	fp, err := os.Open(`data/goldendata_export.rdf.gz`)
	x.Check(err)
	defer fp.Close()

	gz, err := gzip.NewReader(fp)
	x.Check(err)
	defer gz.Close()

	var (
		cnt int
		bb  bytes.Buffer
	)

	reader := bufio.NewReader(gz)
	for {
		b, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			x.Check(err)
		}
		bb.Write(b)
		cnt++
		if cnt%100 == 0 {
			_, err = c.NewTxn().Mutate(ctx, &api.Mutation{
				CommitNow: true,
				SetNquads: bb.Bytes(),
			})
			x.Check(err)
			cnt = 0
			bb.Reset()
		}
	}
}

func BackupLocal(t *testing.T, c *dgo.Dgraph) {
	resp, err := http.PostForm("http://localhost:8180/admin/backup", url.Values{
		"destination": []string{"/tmp"},
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	if !bytes.Contains(b, []byte("Success")) {
		t.Errorf("Backup request failed: %v", string(b))
	}
}
