/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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
	"context"
	"encoding/json"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v200"
	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/stretchr/testify/require"
)

// TestDeleteEdges inserts a large number of edges into a node,
// and attempts to delete all of them at once. If deletion is
// successful, the new number of edges will be 0.
func TestDeleteEdges(t *testing.T) {
	const numEdges = 1000000
	const numBatches = 50
	const uid = "0x1"

	// Wait for the cluster to come up.
	time.Sleep(5 * time.Second)

	dg, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)

	// Batched inserts, since single large inserts are slow.
	for i := 0; i < numBatches; i++ {
		err = doInsert(dg, uid, numEdges/numBatches)
		require.NoError(t, err)
	}

	count, err := doQuery(dg, uid)
	require.NoError(t, err)
	require.Equal(t, numEdges, count)

	err = doDelete(dg, uid)
	require.NoError(t, err)

	count, err = doQuery(dg, uid)
	require.NoError(t, err)
	require.Equal(t, count, 0)
}

func doInsert(dg *dgo.Dgraph, uid string, numEdges int) error {
	log.Printf("inserting %d follows for uid %s", numEdges, uid)

	var setNQuads []byte
	for i := 0; i < numEdges; i++ {
		nQuad := fmt.Sprintf("<%s> <follow> _:f%d .\n", uid, i)
		setNQuads = append(setNQuads, []byte(nQuad)...)
	}
	mutation := &api.Mutation{
		SetNquads: setNQuads,
	}

	request := &api.Request{
		Mutations: []*api.Mutation{mutation},
		CommitNow: true,
	}
	_, err := dg.NewTxn().Do(context.Background(), request)
	if err != nil {
		return err
	}

	return nil
}

func doQuery(dg *dgo.Dgraph, uid string) (int, error) {
	log.Printf("querying follows for uid %s", uid)

	query := fmt.Sprintf(`{
      q(func: uid(%s)) {
        count: count(follow)
	  }
	}`, uid)

	response, err := dg.NewReadOnlyTxn().Query(context.Background(), query)
	if err != nil {
		return 0, err
	}
	log.Printf("response: %s", string(response.Json))

	var responseJSON struct {
		Q []struct {
			Count int
		}
	}
	if err := json.Unmarshal(response.Json, &responseJSON); err != nil {
		return 0, err
	}

	switch len(responseJSON.Q) {
	case 0:
		return 0, nil
	case 1:
		return responseJSON.Q[0].Count, nil
	default:
		return 0, fmt.Errorf("unexpected number of results: %d", len(responseJSON.Q))
	}
}

func doDelete(dg *dgo.Dgraph, uid string) error {
	log.Printf("deleting follows for uid %s", uid)

	delNQuad := fmt.Sprintf("<%s> <follow>  * .", uid)
	mutation := &api.Mutation{
		DelNquads: []byte(delNQuad),
	}
	request := &api.Request{
		Mutations: []*api.Mutation{mutation},
		CommitNow: true,
	}
	_, err := dg.NewTxn().Do(context.Background(), request)
	if err != nil {
		return err
	}

	return nil
}
