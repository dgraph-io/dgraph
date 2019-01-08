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
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestNodes(t *testing.T) {
	wrap := func(fn func(*testing.T, *dgo.Dgraph)) func(*testing.T) {
		return func(t *testing.T) {
			conn, err := grpc.Dial("localhost:9180", grpc.WithInsecure())
			x.Check(err)
			dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))
			fn(t, dg)
		}
	}

	t.Run("setup test data", wrap(NodesSetup))
	t.Run("move tablets from 3", wrap(NodesMoveTablets3))
	t.Run("test query 1", wrap(NodesTestQuery))
	t.Run("move tablets from 2", wrap(NodesMoveTablets2))
	t.Run("test query 2", wrap(NodesTestQuery))
	t.Run("cleanup", wrap(NodesCleanup))
}

func NodesSetup(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{DropAll: true}))

	schema, err := ioutil.ReadFile(`data/goldendata.schema`)
	require.NoError(t, err)
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
			bb.Reset()
		}
	}
}

func NodesCleanup(t *testing.T, c *dgo.Dgraph) {
	require.NoError(t, c.Alter(context.Background(), &api.Operation{DropAll: true}))
}

type response struct {
	Groups map[string]struct {
		Members map[string]interface{} `json:"members"`
		Tablets map[string]struct {
			GroupID   int    `json:"groupId"`
			Predicate string `json:"predicate"`
		} `json:"tablets"`
	} `json:"groups"`
}

func getState() (*response, error) {
	resp, err := http.Get("http://localhost:6080/state")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var st response
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func NodesMoveTablets3(t *testing.T, c *dgo.Dgraph) {
	state1, err := getState()
	require.NoError(t, err)

	for pred := range state1.Groups["3"].Tablets {
		url := fmt.Sprintf("http://localhost:6080/moveTablet?tablet=%s&group=2", pred)
		resp, err := http.Get(url)
		require.NoError(t, err)
		resp.Body.Close()
		time.Sleep(time.Second)
	}

	state2, err := getState()
	require.NoError(t, err)

	if len(state2.Groups["3"].Tablets) > 0 {
		t.Errorf("moving tablets failed")
	}

	resp, err := http.Get("http://localhost:6080/removeNode?group=3&id=3")
	require.NoError(t, err)
	resp.Body.Close()

	state2, err = getState()
	require.NoError(t, err)

	if _, ok := state2.Groups["3"]; ok {
		t.Errorf("node removal failed")
	}
}

func NodesMoveTablets2(t *testing.T, c *dgo.Dgraph) {
	state1, err := getState()
	require.NoError(t, err)

	for pred := range state1.Groups["2"].Tablets {
		url := fmt.Sprintf("http://localhost:6080/moveTablet?tablet=%s&group=1", pred)
		resp, err := http.Get(url)
		require.NoError(t, err)
		resp.Body.Close()
		time.Sleep(time.Second)
	}

	state2, err := getState()
	require.NoError(t, err)

	if len(state2.Groups["2"].Tablets) > 0 {
		t.Errorf("moving tablets failed")
	}

	resp, err := http.Get("http://localhost:6080/removeNode?group=2&id=2")
	require.NoError(t, err)
	resp.Body.Close()

	state2, err = getState()
	require.NoError(t, err)

	if _, ok := state2.Groups["2"]; ok {
		t.Errorf("node removal failed")
	}
}

func NodesTestQuery(t *testing.T, c *dgo.Dgraph) {
	resp, err := c.NewTxn().Query(context.Background(), `
	{
	q(func:anyofterms(name@en, "good bad"), first: 5) {
		name@en
	}
	}`)
	require.NoError(t, err)

	CompareJSON(t, `
	{
		"q": [
			{
				"name@en": "Such Good People"
			},
			{
				"name@en": "You will come good"
			},
			{
				"name@en": "A Good Match"
			},
			{
				"name@en": "A Good Wife"
			},
			{
				"name@en": "Good"
			}
		]
	}`, string(resp.GetJson()))
}
