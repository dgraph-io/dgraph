/*
 * Copyright 2017-2020 Dgraph Labs, Inc. and Contributors
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

package edgraph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v2/y"
	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/golang/glog"
)

// ResetCors make the dgraph to accept all the origins if no origins were given
// by the users.
func ResetCors(closer *y.Closer) {
	defer func() {
		glog.Infof("ResetCors closed")
		closer.Done()
	}()

	req := &api.Request{
		Query: `query{
			cors as var(func: has(dgraph.cors))
		}`,
		Mutations: []*api.Mutation{
			{
				Set: []*api.NQuad{
					{
						Subject:     "_:a",
						Predicate:   "dgraph.cors",
						ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: "*"}},
					},
				},
				Cond: `@if(eq(len(cors), 0))`,
			},
		},
		CommitNow: true,
	}

	for closer.Ctx().Err() == nil {
		ctx, cancel := context.WithTimeout(closer.Ctx(), time.Minute)
		defer cancel()
		if _, err := (&Server{}).doQuery(ctx, req, CorsMutationAllowed); err != nil {
			glog.Infof("Unable to upsert cors. Error: %v", err)
			time.Sleep(100 * time.Millisecond)
		}
		break
	}
}

func generateNquadsForCors(origins []string) []byte {
	out := &bytes.Buffer{}
	for _, origin := range origins {
		out.Write([]byte(fmt.Sprintf("uid(cors) <dgraph.cors> \"%s\" . \n", origin)))
	}
	return out.Bytes()
}

// AddCorsOrigins Adds the cors origins to the Dgraph.
func AddCorsOrigins(ctx context.Context, origins []string) error {
	req := &api.Request{
		Query: `query{
			cors as var(func: has(dgraph.cors))
		}`,
		Mutations: []*api.Mutation{
			{
				SetNquads: generateNquadsForCors(origins),
				Cond:      `@if(eq(len(cors), 1))`,
				DelNquads: []byte(`uid(cors) <dgraph.cors> * .`),
			},
		},
		CommitNow: true,
	}
	_, err := (&Server{}).doQuery(ctx, req, CorsMutationAllowed)
	return err
}

// GetCorsOrigins retrive all the cors origin from the database.
func GetCorsOrigins(ctx context.Context) ([]string, error) {
	req := &api.Request{
		Query: `query{
			me(func: has(dgraph.cors)){
				dgraph.cors
			}
		}`,
		ReadOnly: true,
	}
	res, err := (&Server{}).doQuery(ctx, req, NoAuthorize)
	if err != nil {
		return nil, err
	}

	type corsResponse struct {
		Me []struct {
			DgraphCors []string `json:"dgraph.cors"`
		} `json:"me"`
	}
	corsRes := &corsResponse{}
	if err = json.Unmarshal(res.Json, corsRes); err != nil {
		return nil, err
	}
	if len(corsRes.Me) != 1 {
		return []string{}, fmt.Errorf("GetCorsOrigins returned %d results", len(corsRes.Me))
	}
	return corsRes.Me[0].DgraphCors, nil
}

// UpdateSchemaHistory updates graphql schema history.
func UpdateSchemaHistory(ctx context.Context, schema string) error {
	glog.Info("Updating schema history")
	req := &api.Request{
		Mutations: []*api.Mutation{
			{
				Set: []*api.NQuad{
					{
						Subject:     "_:a",
						Predicate:   "dgraph.graphql.schema_history",
						ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: schema}},
					},
					{
						Subject:   "_:a",
						Predicate: "dgraph.type",
						ObjectValue: &api.Value{Val: &api.Value_StrVal{
							StrVal: "dgraph.graphql.history"}},
					},
				},
				SetNquads: []byte(fmt.Sprintf(`_:a <dgraph.graphql.schema_created_at> "%s" .`,
					time.Now().Format(time.RFC3339))),
			},
		},
		CommitNow: true,
	}
	_, err := (&Server{}).doQuery(ctx, req, CorsMutationAllowed)
	return err
}

// SchemaHistory contains the schema and created_at
type SchemaHistory struct {
	Schema    string `json:"dgraph.graphql.schema_history"`
	CreatedAt string `json:"dgraph.graphql.schema_created_at"`
}

// GetSchemaHistory retives graphql schema history from the database.
func GetSchemaHistory(ctx context.Context, limit, offset int64) ([]SchemaHistory, error) {
	req := &api.Request{
		Query: fmt.Sprintf(`{
			me(func: type(dgraph.graphql.history), orderdesc:dgraph.graphql.schema_created_at, first:%d, offset: %d) {
			  dgraph.graphql.schema_history
			  dgraph.graphql.schema_created_at
			}
		  }`, limit, offset),
		ReadOnly: true,
	}
	res, err := (&Server{}).doQuery(ctx, req, CorsMutationAllowed)
	if err != nil {
		return nil, err
	}

	type HistoryResponse struct {
		Me []SchemaHistory `json:"me"`
	}
	hr := HistoryResponse{}
	if err = json.Unmarshal(res.Json, &hr); err != nil {
		return nil, err
	}
	return hr.Me, nil
}
