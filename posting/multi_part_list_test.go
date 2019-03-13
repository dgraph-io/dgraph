/*
 * Copyright 2015-2019 Dgraph Labs, Inc. and Contributors
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

package posting

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"google.golang.org/grpc"
)

var client *dgo.Dgraph

func assignUids(num uint64) {
	_, err := http.Get(fmt.Sprintf("http://localhost:6080/assign?what=uids&num=%d", num))
	if err != nil {
		panic(fmt.Sprintf("Could not assign uids. Got error %v", err.Error()))
	}
}

func getNewClient() *dgo.Dgraph {
	conn, err := grpc.Dial("localhost:9180", grpc.WithInsecure())
	x.Check(err)
	return dgo.NewDgraphClient(api.NewDgraphClient(conn))
}

func setSchema(schema string) {
	err := client.Alter(context.Background(), &api.Operation{
		Schema: schema,
	})
	if err != nil {
		panic(fmt.Sprintf("Could not alter schema. Got error %v", err.Error()))
	}
}

func processQuery(t *testing.T, ctx context.Context, query string) (string, error) {
	txn := client.NewTxn()
	defer txn.Discard(ctx)

	res, err := txn.Query(ctx, query)
	if err != nil {
		return "", err
	}

	response := map[string]interface{}{}
	response["data"] = json.RawMessage(string(res.Json))

	jsonResponse, err := json.Marshal(response)
	require.NoError(t, err)
	return string(jsonResponse), err
}

func processQueryNoErr(t *testing.T, query string) string {
	res, err := processQuery(t, context.Background(), query)
	require.NoError(t, err)
	return res
}
