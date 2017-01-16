/*
 * Copyright 2015 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

var q0 = `
	{
		user(id:alice) {
			follows {
				status
			}
			status
		}
	}
`

func prepare() (dir1, dir2 string, ps *store.Store, rerr error) {
	var err error
	dir1, err = ioutil.TempDir("", "storetest_")
	if err != nil {
		return "", "", nil, err
	}
	ps, err = store.NewStore(dir1)
	if err != nil {
		return "", "", nil, err
	}

	dir2, err = ioutil.TempDir("", "wal_")
	if err != nil {
		return dir1, "", nil, err
	}

	posting.Init(ps)
	group.ParseGroupConfig("groups.conf")
	worker.StartRaftNodes(dir2)

	return dir1, dir2, ps, nil
}

func closeAll(dir1, dir2 string) {
	os.RemoveAll(dir2)
	os.RemoveAll(dir1)
}

func childAttrs(sg *query.SubGraph) []string {
	var out []string
	for _, c := range sg.Children {
		out = append(out, c.Attr)
	}
	return out
}

func TestQuery(t *testing.T) {
	dir1, dir2, _, err := prepare()
	require.NoError(t, err)
	defer closeAll(dir1, dir2)

	// Parse GQL into internal query representation.
	res, err := gql.Parse(q0)
	require.NoError(t, err)

	ctx := context.Background()
	g, err := query.ToSubGraph(ctx, res.Query[0])
	require.NoError(t, err)

	// Test internal query representation.
	require.EqualValues(t, []string{"follows", "status"}, childAttrs(g))
	require.EqualValues(t, []string{"status"}, childAttrs(g.Children[0]))

	ch := make(chan error)
	go query.ProcessGraph(ctx, g, nil, ch)
	err = <-ch
	require.NoError(t, err)

	var l query.Latency
	js, err := g.ToJSON(&l)
	require.NoError(t, err)
	j, err := json.Marshal(js)
	require.NoError(t, err)
	fmt.Println(string(j))
}

var qm = `
	mutation {
		set {
			<0x0a> <pred.rel> _:x .
			_:x <pred.val> "value" .
			_:x <pred.rel> _:y .
			_:y <pred.val> "value2" .
		}
	}
`

func TestAssignUid(t *testing.T) {
	dir1, dir2, _, err := prepare()
	require.NoError(t, err)
	defer closeAll(dir1, dir2)
	time.Sleep(5 * time.Second) // Wait for ME to become leader.

	// Parse GQL into internal query representation.
	res, err := gql.Parse(qm)
	require.NoError(t, err)

	ctx := context.Background()
	allocIds, err := mutationHandler(ctx, res.Mutation)
	require.NoError(t, err)

	require.EqualValues(t, len(allocIds), 2, "Expected two UIDs to be allocated")
	_, ok := allocIds["x"]
	require.True(t, ok)
	_, ok = allocIds["y"]
	require.True(t, ok)
}

func TestConvertToEdges(t *testing.T) {
	q1 := `<0x01> <type> <0x02> .
	       <0x01> <character> <0x03> .`
	nquads, err := convertToNQuad(context.Background(), q1)
	require.NoError(t, err)

	mr, err := convertToEdges(context.Background(), nquads)
	require.NoError(t, err)

	require.EqualValues(t, len(mr.edges), 2)
}

var q1 = `
{
	al(id: alice) {
		status
		follows {
			status
			follows {
				status
				follows {
					status
				}
			}
		}
		status
	}
}
`

func BenchmarkQuery(b *testing.B) {
	dir1, dir2, _, err := prepare()
	if err != nil {
		b.Error(err)
		return
	}
	defer closeAll(dir1, dir2)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res, err := gql.Parse(q1)
		if err != nil {
			b.Error(err)
			return
		}
		ctx := context.Background()
		g, err := query.ToSubGraph(ctx, res.Query[0])
		if err != nil {
			b.Error(err)
			return
		}

		ch := make(chan error)
		go query.ProcessGraph(ctx, g, nil, ch)
		err = <-ch
		require.NoError(b, err)
		var l query.Latency
		_, err = g.ToJSON(&l)
		require.NoError(b, err)
	}
}

func TestMain(m *testing.M) {
	x.Init()
	os.Exit(m.Run())
}
