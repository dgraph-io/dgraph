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
	"fmt"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/z"
	"github.com/stretchr/testify/require"
)

func TestSnapshot(t *testing.T) {
	snapshotTs := uint64(0)

	dg1 := z.DgraphClientWithGroot("localhost:9180")
	require.NoError(t, dg1.Alter(context.Background(), &api.Operation{
		DropOp: api.Operation_ALL,
	}))
	require.NoError(t, dg1.Alter(context.Background(), &api.Operation{
		Schema: "value: int .",
	}))

	for i := 1; i <= 10; i++ {
		err := z.RetryMutation(dg1, &api.Mutation{
			SetNquads: []byte(fmt.Sprintf(`_:node <value> "%d" .`, i)),
			CommitNow: true,
		})
		require.NoError(t, err)
	}
	verifySnapshot(t, dg1, 10)

	err := z.DockerStop("alpha2")
	require.NoError(t, err)

	for i := 11; i <= 600; i++ {
		err := z.RetryMutation(dg1, &api.Mutation{
			SetNquads: []byte(fmt.Sprintf(`_:node <value> "%d" .`, i)),
			CommitNow: true,
		})
		require.NoError(t, err)
	}
	snapshotTs = waitForSnapshot(t, snapshotTs)

	err = z.DockerStart("alpha2")
	require.NoError(t, err)

	dg2 := z.DgraphClientWithGroot("localhost:9182")
	verifySnapshot(t, dg2, 600)

	err = z.DockerStop("alpha2")
	require.NoError(t, err)

	for i := 601; i <= 1200; i++ {
		err := z.RetryMutation(dg1, &api.Mutation{
			SetNquads: []byte(fmt.Sprintf(`_:node <value> "%d" .`, i)),
			CommitNow: true,
		})
		require.NoError(t, err)
	}
	snapshotTs = waitForSnapshot(t, snapshotTs)

	err = z.DockerStart("alpha2")
	require.NoError(t, err)

	dg2 = z.DgraphClientWithGroot("localhost:9182")
	verifySnapshot(t, dg2, 1200)
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
	resp, err := z.RetryQuery(dg, q1)
	require.NoError(t, err)
	err = json.Unmarshal(resp.Json, &resMap)
	require.NoError(t, err)

	sum := 0
	for _, item := range resMap["values"] {
		sum += item["value"]
	}
	require.Equal(t, expectedSum, sum)
}

func waitForSnapshot(t *testing.T, prevSnapTs uint64) uint64 {
	snapPattern := `"snapshotTs":"([0-9]*)"`
	for {
		res, err := http.Get("http://localhost:6080/state")
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
