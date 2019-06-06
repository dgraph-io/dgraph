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
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/z"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

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

func doTestQuery(t *testing.T, c *dgo.Dgraph) {
	resp, err := c.NewTxn().Query(context.Background(), `
  {
  q(func:anyofterms(name@en, "good bad"), first: -5) {
    name@en
  }
  }`)
	require.NoError(t, err)

	z.CompareJSON(t, `
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

func getClientToGroup(groupId string) (*dgo.Dgraph, error) {
	state, err := z.GetState()
	if err != nil {
		return nil, err
	}

	group, ok := state.Groups[groupId]
	if !ok {
		return nil, fmt.Errorf("group %s does not exist", groupId)
	}

	if len(group.Members) == 0 {
		return nil, fmt.Errorf("the group %s has no members", groupId)
	}

	member := group.Members["1"]
	parts := strings.Split(member.Addr, ":")
	if len(parts) != 2 {
		return nil, fmt.Errorf("the member has an invalid address: %v", member.Addr)
	}
	// internalPort is used for communication between alpha nodes
	internalPort, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("unable to parse the port number from %s", parts[1])
	}

	// externalPort is for handling connections from clients
	externalPort := internalPort + 2000

	conn, err := grpc.Dial(fmt.Sprintf("localhost:%d", externalPort), grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	return dgo.NewDgraphClient(api.NewDgraphClient(conn)), nil
}

func TestNodes(t *testing.T) {
	dg, err := getClientToGroup("1")
	require.NoError(t, err, "error while getting connection to group 1")

	NodesSetup(t, dg)

	state1, err := z.GetState()
	require.NoError(t, err)

	for pred := range state1.Groups["3"].Tablets {
		url := fmt.Sprintf("http://"+z.SockAddrZeroHttp+"/moveTablet?tablet=%s&group=2",
			url.QueryEscape(pred))
		resp, err := http.Get(url)
		require.NoError(t, err)
		require.NoError(t, getError(resp.Body))
		time.Sleep(time.Second)
	}

	state2, err := z.GetState()
	require.NoError(t, err)

	if len(state2.Groups["3"].Tablets) > 0 {
		t.Errorf("moving tablets failed")
	}

	resp, err := http.Get("http://" + z.SockAddrZeroHttp + "/removeNode?group=3&id=3")
	require.NoError(t, err)
	require.NoError(t, getError(resp.Body))

	state2, err = z.GetState()
	require.NoError(t, err)

	if _, ok := state2.Groups["3"]; ok {
		t.Errorf("node removal failed")
	}

	doTestQuery(t, dg)

	state1, err = z.GetState()
	require.NoError(t, err)

	for pred := range state1.Groups["2"].Tablets {
		url := fmt.Sprintf("http://"+z.SockAddrZeroHttp+"/moveTablet?tablet=%s&group=1",
			url.QueryEscape(pred))
		resp, err := http.Get(url)
		require.NoError(t, err)
		require.NoError(t, getError(resp.Body))
		time.Sleep(time.Second)
	}

	state2, err = z.GetState()
	require.NoError(t, err)

	if len(state2.Groups["2"].Tablets) > 0 {
		t.Errorf("moving tablets failed")
	}

	resp, err = http.Get("http://" + z.SockAddrZeroHttp + "/removeNode?group=2&id=2")
	require.NoError(t, err)
	require.NoError(t, getError(resp.Body))

	state2, err = z.GetState()
	require.NoError(t, err)

	if _, ok := state2.Groups["2"]; ok {
		t.Errorf("node removal failed")
	}

	doTestQuery(t, dg)
}
