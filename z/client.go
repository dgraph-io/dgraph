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

package z

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/x"
)

func DgraphClient(serviceAddr string) *dgo.Dgraph {
	conn, err := grpc.Dial(serviceAddr, grpc.WithInsecure())
	x.Check(err)

	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))
	for {
		// keep retrying until we succeed or receive a non-retriable error
		err = dg.Alter(context.Background(), &api.Operation{DropAll: true})
		if err == nil || !strings.Contains(err.Error(), "Please retry") {
			break
		}
		time.Sleep(time.Second)
	}

	x.Check(err)

	return dg
}

func DropAll(t *testing.T, dg *dgo.Dgraph) {
	err := dg.Alter(context.Background(), &api.Operation{DropAll: true})
	x.Check(err)

	nodes := DbNodeCount(t, dg)
	require.Equal(t, 0, nodes)
}

func DbNodeCount(t *testing.T, dg *dgo.Dgraph) int {
	resp, err := dg.NewTxn().Query(context.Background(), `
		{
			q(func: has(_predicate_)) {
				count(uid)
			}
		}
	`)
	x.Check(err)

	type count struct {
		Count int
	}
	type root struct {
		Q []count
	}
	var response root
	err = json.Unmarshal(resp.GetJson(), &response)
	x.Check(err)

	return response.Q[0].Count
}
