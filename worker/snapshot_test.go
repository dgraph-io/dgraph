/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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

package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v2"
	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/stretchr/testify/require"
)

func TestSnapshot(t *testing.T) {
	snapshotTs := uint64(0)

	dg1, err := testutil.DgraphClient("localhost:9180")
	if err != nil {
		t.Fatalf("Error while getting a dgraph client: %v", err)
	}
	require.NoError(t, dg1.Alter(context.Background(), &api.Operation{
		DropOp: api.Operation_ALL,
	}))
	require.NoError(t, dg1.Alter(context.Background(), &api.Operation{
		Schema: "value: int .",
	}))

	err = testutil.DockerStop("alpha2")
	require.NoError(t, err)

	for i := 1; i <= 200; i++ {
		err := testutil.RetryMutation(dg1, &api.Mutation{
			SetNquads: []byte(fmt.Sprintf(`_:node <value> "%d" .`, i)),
			CommitNow: true,
		})
		require.NoError(t, err)
	}
	snapshotTs = waitForSnapshot(t, snapshotTs)

	err = testutil.DockerStart("alpha2")
	require.NoError(t, err)

	dg2, err := testutil.DgraphClient("localhost:9182")
	if err != nil {
		t.Fatalf("Error while getting a dgraph client: %v", err)
	}
	verifySnapshot(t, dg2, 200)

	err = testutil.DockerStop("alpha2")
	require.NoError(t, err)

	for i := 201; i <= 400; i++ {
		err := testutil.RetryMutation(dg1, &api.Mutation{
			SetNquads: []byte(fmt.Sprintf(`_:node <value> "%d" .`, i)),
			CommitNow: true,
		})
		require.NoError(t, err)
	}
	_ = waitForSnapshot(t, snapshotTs)

	err = testutil.DockerStart("alpha2")
	require.NoError(t, err)

	dg2, err = testutil.DgraphClient("localhost:9182")
	if err != nil {
		t.Fatalf("Error while getting a dgraph client: %v", err)
	}
	verifySnapshot(t, dg2, 400)
}

func verifySnapshot(t *testing.T, dg *dgo.Dgraph, num int) {
	expectedSum := (num * (num + 1)) / 2

	q1 := `
	{
		values(func: has(value)) {
			value
		}
	}`

	resMap := make(map[string][]map[string]int)
	resp, err := testutil.RetryQuery(dg, q1)
	require.NoError(t, err)
	err = json.Unmarshal(resp.Json, &resMap)
	require.NoError(t, err)

	sum := 0
	require.Equal(t, num, len(resMap["values"]))
	for _, item := range resMap["values"] {
		sum += item["value"]
	}
	require.Equal(t, expectedSum, sum)
}

func waitForSnapshot(t *testing.T, prevSnapTs uint64) uint64 {
	snapPattern := `"snapshotTs":"([0-9]*)"`
	for {
		res, err := http.Get("http://localhost:6180/state")
		require.NoError(t, err)
		body, err := ioutil.ReadAll(res.Body)
		res.Body.Close()
		require.NoError(t, err)

		regex, err := regexp.Compile(snapPattern)
		require.NoError(t, err)

		matches := regex.FindAllStringSubmatch(string(body), 1)
		if len(matches) == 0 {
			time.Sleep(time.Second)
			continue
		}

		snapshotTs, err := strconv.ParseUint(matches[0][1], 10, 64)
		require.NoError(t, err)
		if snapshotTs > prevSnapTs {
			return snapshotTs
		}

		time.Sleep(time.Second)
	}
}
