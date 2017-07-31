/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package client_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"testing"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/dgraph"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	x.Init()
	x.Logger = log.New(ioutil.Discard, "", 0)
	os.Exit(m.Run())
}

func prepare() (res []string, options dgraph.Options) {
	clientDir, err := ioutil.TempDir("", "client_")
	x.Check(err)
	res = append(res, clientDir)

	options = dgraph.GetDefaultEmbeddedConfig()
	postingDir, err := ioutil.TempDir("", "p")
	x.Check(err)
	res = append(res, postingDir)

	options.PostingDir = postingDir
	walDir, err := ioutil.TempDir("", "w")
	x.Check(err)
	res = append(res, walDir)
	options.WALDir = walDir
	options.AllottedMemory = 2048

	return res, options
}

func removeDirs(dirs []string) {
	for _, dir := range dirs {
		os.Remove(dir)
	}
}

type Person struct {
	Name    string   `dgraph:"name"`
	FallsIn string   `dgraph:"falls.in"`
	Friends []Person `dgraph:"friend"`
}

type Res struct {
	Root []Person `dgraph:"me"`
}

func TestClientDelete(t *testing.T) {
	dirs, options := prepare()
	defer removeDirs(dirs)

	dgraphClient := dgraph.NewEmbeddedDgraphClient(options, client.DefaultOptions, dirs[0])
	defer dgraph.DisposeEmbeddedDgraph()
	req := client.Req{}
	alice, err := dgraphClient.NodeBlank("")
	require.NoError(t, err)
	bob, err := dgraphClient.NodeBlank("")
	require.NoError(t, err)

	e := alice.Edge("name")
	require.NoError(t, e.SetValueString("Alice"))
	require.NoError(t, req.Set(e))
	e = bob.Edge("name")
	require.NoError(t, e.SetValueString("Bob"))
	require.NoError(t, req.Set(e))
	e = alice.Edge("falls.in")
	require.NoError(t, e.SetValueString("Rabbit Hole"))
	require.NoError(t, req.Set(e))
	e = alice.ConnectTo("friend", bob)
	require.NoError(t, req.Set(e))
	aliceQuery := fmt.Sprintf(`{
		me(func: uid(%s)) {
			name
			falls.in
			friend {
				name
			}
		}
	}`, alice)
	req.SetQuery(aliceQuery)
	resp, err := dgraphClient.Run(context.Background(), &req)
	x.Check(err)

	var r Res
	err = client.Unmarshal(resp.N, &r)
	require.Equal(t, 1, len(r.Root))
	require.Equal(t, "Alice", r.Root[0].Name)
	require.Equal(t, "Rabbit Hole", r.Root[0].FallsIn)
	require.Equal(t, 1, len(r.Root[0].Friends))
	require.Equal(t, "Bob", r.Root[0].Friends[0].Name)

	// Lets test Edge delete
	req = client.Req{}
	r = Res{}
	e = alice.Edge("name")
	// S P * deletion for name.
	x.Check(e.Delete())
	x.Check(req.Delete(e))
	// S P * deletion for friend.
	e = alice.Edge("friend")
	x.Check(e.Delete())
	x.Check(req.Delete(e))
	req.SetQuery(aliceQuery)
	resp, err = dgraphClient.Run(context.Background(), &req)
	x.Check(err)
	err = client.Unmarshal(resp.N, &r)
	require.Equal(t, 1, len(r.Root))
	require.Equal(t, "", r.Root[0].Name)
	require.Equal(t, "Rabbit Hole", r.Root[0].FallsIn)
	require.Equal(t, 0, len(r.Root[0].Friends))

	// Lets test Node delete now.
	req = client.Req{}
	r = Res{}
	e = alice.Delete()
	x.Check(e.Delete())
	x.Check(req.Delete(e))
	req.SetQuery(aliceQuery)
	resp, err = dgraphClient.Run(context.Background(), &req)
	x.Check(err)
	err = client.Unmarshal(resp.N, &r)
	require.Equal(t, 0, len(r.Root))
}

func TestClientDeletePredicate(t *testing.T) {
	dirs, options := prepare()
	defer removeDirs(dirs)

	dgraphClient := dgraph.NewEmbeddedDgraphClient(options, client.DefaultOptions, dirs[0])
	defer dgraph.DisposeEmbeddedDgraph()
	req := client.Req{}
	alice, err := dgraphClient.NodeBlank("")
	require.NoError(t, err)
	bob, err := dgraphClient.NodeBlank("")
	require.NoError(t, err)
	charlie, err := dgraphClient.NodeBlank("")
	require.NoError(t, err)

	e := alice.Edge("name")
	require.NoError(t, e.SetValueString("Alice"))
	require.NoError(t, req.Set(e))

	e = bob.Edge("name")
	require.NoError(t, e.SetValueString("Bob"))
	require.NoError(t, req.Set(e))

	e = charlie.Edge("name")
	require.NoError(t, e.SetValueString("Charlie"))
	require.NoError(t, req.Set(e))

	e = alice.ConnectTo("friend", bob)
	require.NoError(t, req.Set(e))
	e = alice.ConnectTo("friend", charlie)
	require.NoError(t, req.Set(e))
	aliceQuery := fmt.Sprintf(`{
		me(func: uid(%s)) {
			name
			falls.in
			friend {
				name
			}
		}
	}`, alice)
	req.SetQuery(aliceQuery)
	resp, err := dgraphClient.Run(context.Background(), &req)
	x.Check(err)

	var r Res
	err = client.Unmarshal(resp.N, &r)
	require.Equal(t, 1, len(r.Root))
	require.Equal(t, 2, len(r.Root[0].Friends))

	req = client.Req{}
	req.SetQuery(aliceQuery)
	req.Delete(client.DeletePredicate("friend"))
	resp, err = dgraphClient.Run(context.Background(), &req)
	x.Check(err)
	r = Res{}
	err = client.Unmarshal(resp.N, &r)
	require.Equal(t, 1, len(r.Root))
	require.Equal(t, 0, len(r.Root[0].Friends))
}

func TestLangTag(t *testing.T) {
	dirs, options := prepare()
	defer removeDirs(dirs)

	dgraphClient := dgraph.NewEmbeddedDgraphClient(options, client.DefaultOptions, dirs[0])
	defer dgraph.DisposeEmbeddedDgraph()
	req := client.Req{}
	alice, err := dgraphClient.NodeBlank("")
	require.NoError(t, err)

	e := alice.Edge("name")
	require.NoError(t, e.SetValueString("Alice"))
	require.NoError(t, req.Set(e))
	aliceQuery := fmt.Sprintf(`{
		me(func: uid(%s)) {
			name
		}
	}`, alice)
	req.SetQuery(aliceQuery)
	resp, err := dgraphClient.Run(context.Background(), &req)
	x.Check(err)

	var r Res
	err = client.Unmarshal(resp.N, &r)
	require.Equal(t, 1, len(r.Root))
	require.Equal(t, "Alice", r.Root[0].Name)

	type Person struct {
		Name    string   `dgraph:"name@ru"`
		FallsIn string   `dgraph:"falls.in"`
		Friends []Person `dgraph:"friend"`
	}

	type Res struct {
		Root []Person `dgraph:"me"`
	}

	var r2 Res

	req = client.Req{}
	e = alice.Edge("name")
	require.NoError(t, e.SetValueStringWithLang("Алиса", "ru"))
	require.NoError(t, req.Set(e))
	aliceQuery = fmt.Sprintf(`{
		me(func: uid(%s)) {
			name@ru
		}
	}`, alice)
	req.SetQuery(aliceQuery)
	resp, err = dgraphClient.Run(context.Background(), &req)
	x.Check(err)
	err = client.Unmarshal(resp.N, &r2)
	require.Equal(t, 1, len(r2.Root))
	require.Equal(t, "Алиса", r2.Root[0].Name)
}
