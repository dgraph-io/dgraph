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
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/z"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestNodes(t *testing.T) {
	wrap := func(fn func(*testing.T, *dgo.Dgraph)) func(*testing.T) {
		return func(t *testing.T) {
			conn, err := grpc.Dial(z.SockAddr, grpc.WithInsecure())
			require.NoError(t, err)
			dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))
			fn(t, dg)
		}
	}

	tests := []struct {
		name string
		fn   func(*testing.T, *dgo.Dgraph)
	}{
		{name: "setup test data", fn: NodesSetup},
		{name: "move tablets from 3", fn: NodesMoveTablets3},
		{name: "test query 1", fn: NodesTestQuery},
		{name: "move tablets from 2", fn: NodesMoveTablets2},
		{name: "test query 2", fn: NodesTestQuery},
	}
	for _, tc := range tests {
		if !t.Run(tc.name, wrap(tc.fn)) {
			break
		}
	}
	t.Run("cleanup", wrap(NodesCleanup))
}

func NodesSetup(t *testing.T, c *dgo.Dgraph) {
	ctx := context.Background()

	require.NoError(t, c.Alter(ctx, &api.Operation{DropAll: true}))

	schema, err := ioutil.ReadFile(`../data/goldendata.schema`)
	require.NoError(t, err)
	require.NoError(t, c.Alter(ctx, &api.Operation{Schema: string(schema)}))

	fp, err := os.Open(`../data/goldendata_export.rdf.gz`)
	require.NoError(t, err)
	defer fp.Close()

	gz, err := gzip.NewReader(fp)
	require.NoError(t, err)
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
			require.NoError(t, err)
		}
		bb.Write(b)
		cnt++
		if cnt%100 == 0 {
			_, err = c.NewTxn().Mutate(ctx, &api.Mutation{
				CommitNow: true,
				SetNquads: bb.Bytes(),
			})
			require.NoError(t, err)
			bb.Reset()
		}
	}
}

type response struct {
	Groups map[string]struct {
		Members map[string]interface{} `json:"members"`
		Tablets map[string]struct {
			GroupID   int    `json:"groupId"`
			Predicate string `json:"predicate"`
		} `json:"tablets"`
	} `json:"groups"`
	Removed []struct {
		Addr    string `json:"addr"`
		GroupID int    `json:"groupId"`
		ID      string `json:"id"`
	} `json:"removed"`
}

func NodesCleanup(t *testing.T, c *dgo.Dgraph) {
	state, err := getState()
	require.NoError(t, err)

	// NOTE: in the rare occasion that we are in fact connected to node 2, skip this.
	for i := range state.Removed {
		if strings.HasSuffix(state.Removed[i].Addr, "7180") {
			t.Log("skipping cleanup, we are connected to a removed node.")
			return
		}
	}

	require.NoError(t, c.Alter(context.Background(), &api.Operation{DropAll: true}))
}

func getState() (*response, error) {
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

	var st response
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, err
	}
	return &st, nil
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

func NodesMoveTablets3(t *testing.T, c *dgo.Dgraph) {
	state1, err := getState()
	require.NoError(t, err)

	for pred := range state1.Groups["3"].Tablets {
		url := fmt.Sprintf("http://"+z.SockAddrZeroHttp+"/moveTablet?tablet=%s&group=2",
			url.QueryEscape(pred))
		resp, err := http.Get(url)
		require.NoError(t, err)
		require.NoError(t, getError(resp.Body))
		time.Sleep(time.Second)
	}

	state2, err := getState()
	require.NoError(t, err)

	if len(state2.Groups["3"].Tablets) > 0 {
		t.Errorf("moving tablets failed")
	}

	resp, err := http.Get("http://" + z.SockAddrZeroHttp + "/removeNode?group=3&id=3")
	require.NoError(t, err)
	require.NoError(t, getError(resp.Body))

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
		url := fmt.Sprintf("http://"+z.SockAddrZeroHttp+"/moveTablet?tablet=%s&group=1",
			url.QueryEscape(pred))
		resp, err := http.Get(url)
		require.NoError(t, err)
		require.NoError(t, getError(resp.Body))
		time.Sleep(time.Second)
	}

	state2, err := getState()
	require.NoError(t, err)

	if len(state2.Groups["2"].Tablets) > 0 {
		t.Errorf("moving tablets failed")
	}

	resp, err := http.Get("http://" + z.SockAddrZeroHttp + "/removeNode?group=2&id=2")
	require.NoError(t, err)
	require.NoError(t, getError(resp.Body))

	state2, err = getState()
	require.NoError(t, err)

	if _, ok := state2.Groups["2"]; ok {
		t.Errorf("node removal failed")
	}
}

func NodesTestQuery(t *testing.T, c *dgo.Dgraph) {
	resp, err := c.NewTxn().Query(context.Background(), `
  {
  q(func:anyofterms(name@en, "good bad"), first: -5) {
    name@en
  }
  }`)
	require.NoError(t, err)

	CompareJSON(t, `
  {
    "q": [
      {
        "name@en": "Good Grief"
      },
      {
        "name@en": "Half Good Killer"
      },
      {
        "name@en": "Bad Friend"
      },
      {
        "name@en": "Ace of Spades: Bad Destiny"
      },
      {
        "name@en": "Bad Girls 6"
      }
    ]
  }`, string(resp.GetJson()))
}
