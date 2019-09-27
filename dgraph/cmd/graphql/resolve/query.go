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

package resolve

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/golang/glog"
	otrace "go.opencensus.io/trace"

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/api"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/dgraph"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/dgraph-io/dgraph/x"
)

// queryResolver can resolve a single GraphQL query field
type queryResolver struct {
	query         schema.Query
	schema        schema.Schema
	dgraph        dgraph.Client
	queryRewriter dgraph.QueryRewriter
	operation     schema.Operation
}

// resolve a query.
func (qr *queryResolver) resolve(ctx context.Context) *resolved {
	res := &resolved{}

	span := otrace.FromContext(ctx)
	stop := x.SpanTimer(span, "resolveQuery")
	defer stop()

	if qr.query.QueryType() == schema.SchemaQuery {
		resp, err := schema.Introspect(qr.operation, qr.query, qr.schema)
		if err != nil {
			res.err = err
			return res
		}
		// This is because Introspect returns an object.
		if len(resp) >= 2 {
			res.data = resp[1 : len(resp)-1]
		}
		return res
	}

	if qr.query.QueryType() == schema.ApolloServiceQuery {
		// Just print back the SDL
		// why does it work like this and not return a schema introspection query?

		type sdl struct {
			Sdl string `json:"sdl"`
		}

		js, err := json.Marshal(struct {
			Service *sdl `json:"_service"`
		}{
			Service: &sdl{Sdl: schema.Stringify(qr.schema.AsAstSchema(), nil, false)},
		})
		if err != nil {
			res.err = schema.GQLWrapf(err, "mashal response for Apollo _service query")
			return res
		}
		res.data = js[1 : len(js)-1]
		return res
	}

	if qr.query.QueryType() == schema.ApolloEntityQuery {

		// {
		//   "query": query ($_representations: [_Any!]!) {
		// 	  _entities(representations: $_representations) {
		// 		... on Product {
		// 			reviews {
		// 				 body
		// 			}
		// 		}
		// 	  }
		//   },
		//   "variables": {
		//     "_representations": [
		//       {
		//         "__typename": "Product",
		//         "upc": "B00005N5PF"
		//       },
		//       ...
		//     ]
		//   }
		// }

		// let's assume for now that all those representations are from the same type
		// so I unmarshal, compile an ID list, and then run an ids query against that
		// type name
		//
		// so what I really do is replace query with my ids query ... and
		// make the variables the empty map?
		// take '... on Product {' ---and-make---> 'queryProduct(filter: { ids: [ ... ] } { ... } )

		representations, ok := qr.query.ArgValue("representations").([]interface{})
		if !ok {
			// ... all the errorz
		}

		var ids []uint64
		var typ schema.Type

		for _, r := range representations {
			rep, ok := r.(map[string]interface{})
			if !ok {
				// ... all the errorz
				continue
			}

			typename, ok := rep["__typename"].(string)

			if !ok {
				continue
			}

			typ = qr.schema.Type(typename)
			idFld := typ.IDField()

			id, ok := rep[idFld.Name()].(string)
			if !ok {
				continue
			}

			uid, err := strconv.ParseUint(id, 0, 64)
			if err != nil {
				continue
			}

			ids = append(ids, uid)
		}

		selections := qr.query.SelectionSet() //[0].SelectionSet()

		// rewrite the query and fall through to the normal query processing
		qr.query = schema.IdQueryWithSelections(typ.Name(), "_entities", qr.operation, ids, selections)
	}

	dgQuery, err := qr.queryRewriter.Rewrite(qr.query)
	if err != nil {
		res.err = schema.GQLWrapf(err, "couldn't rewrite query")
		return res
	}

	resp, err := qr.dgraph.Query(ctx, dgQuery)
	if err != nil {
		glog.Infof("[%s] Dgraph query failed : %s", api.RequestID(ctx), err)
		res.err = schema.GQLWrapf(err, "[%s] failed to resolve query", api.RequestID(ctx))
		return res
	}

	completed, err := completeDgraphResult(ctx, qr.query, resp)
	res.err = err

	// chop leading '{' and trailing '}' from JSON object
	//
	// The final GraphQL result gets built like
	// { data:
	//    {
	//      q1: {...},
	//      q2: [ {...}, {...} ],
	//      ...
	//    }
	// }
	// Here we are building a single one of the q's, so the fully resolved
	// result should be q1: {...}, rather than {q1: {...}}.
	//
	// completeDgraphResult() always returns a valid JSON object.  At least
	// { q: null }
	// even in error cases, so this is safe.
	res.data = completed[1 : len(completed)-1]
	return res
}
