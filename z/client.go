/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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
	"testing"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/x"
)

func DgraphClient(serviceAddr string) *dgo.Dgraph {
	conn, err := grpc.Dial(serviceAddr, grpc.WithInsecure())
	x.Check(err)

	dg := dgo.NewDgraphClient(api.NewDgraphClient(conn))
	err = dg.Alter(context.Background(), &api.Operation{DropAll: true})
	x.Check(err)

	return dg
}

func DropAll(t *testing.T, dg *dgo.Dgraph) {
	err := dg.Alter(context.Background(), &api.Operation{DropAll: true})
	x.Check(err)

	resp, err := dg.NewTxn().Query(context.Background(), `
		{
			q(func: has(name)) {
				nodes : count(uid)
			}
		}
	`)
	x.Check(err)
	CompareJSON(t, `{"q":[{"nodes":0}]}`, string(resp.GetJson()))
}
