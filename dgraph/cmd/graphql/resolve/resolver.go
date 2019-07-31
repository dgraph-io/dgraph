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
	"bytes"
	"context"

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/dgraph"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/vektah/gqlparser/gqlerror"
)

// GraphQL spec:
// https://graphql.github.io/graphql-spec/June2018
//
//
// GraphQL servers should serve both GET and POST
// https://graphql.org/learn/serving-over-http/
//
// GET should be like
// http://myapi/graphql?query={me{name}}
//
// POST should have a json content body like
// {
//   "query": "...",
//   "operationName": "...",
//   "variables": { "myVariable": "someValue", ... }
// }
//
// GraphQL servers should return 200 (even on errors),
// and result body should be json:
// {
//   "data": { "query_name" : { ... } },
//   "errors": [ { "message" : ..., ...} ... ]
// }
//
// Key points about the response
// (https://graphql.github.io/graphql-spec/June2018/#sec-Response)
//
// - If an error was encountered before execution begins,
//   the data entry should not be present in the result.
//
// - If an error was encountered during the execution that
//   prevented a valid response, the data entry in the response should be null.
//
// - If there's errors and data, both are returned
//
// - If no errors were encountered during the requested operation,
//   the errors entry should not be present in the result.
//
// - There's rules around how errors work when there's ! fields in the schema
//   https://graphql.github.io/graphql-spec/June2018/#sec-Errors-and-Non-Nullability
//
// - The "message" in an error is required, the rest is up to the implementation
//
// - The "data" works just like a Dgraph query
//

// RequestResolver can process GraphQL requests and write GraphQL JSON responses.
type RequestResolver struct {
	GqlReq *schema.Request
	Schema schema.Schema
	dgraph dgraph.Client
	resp   *schema.Response
}

// New creates a new RequestResolver
func New(s schema.Schema, dg dgraph.Client) *RequestResolver {
	return &RequestResolver{
		Schema: s,
		dgraph: dg,
		resp:   &schema.Response{},
	}
}

// WithError generates GraphQL errors from err and records those in r.
func (r *RequestResolver) WithError(err error) {
	r.resp.Errors = append(r.resp.Errors, schema.AsGQLErrors(err)...)
}

// Resolve processes h.GqlReq and returns a GraphQL response.
// h.GqlReq should be set with a request before Resolve is called
// and a schema and backend should have been added.
// Resolve records any errors in the response's error field.
func (r *RequestResolver) Resolve(ctx context.Context) *schema.Response {
	if r == nil {
		glog.Error("Call to Resolve with nil RequestResolver")
		return schema.ErrorResponsef("Internal error")
	}

	if r.Schema == nil {
		glog.Error("Call to Resolve with no schema")
		return schema.ErrorResponsef("Internal error")
	}

	if r.resp.Errors != nil {
		r.resp.Data.Reset()
		return r.resp
	}

	op, err := r.Schema.Operation(r.GqlReq)
	if err != nil {
		return schema.ErrorResponse(err)
	}

	// A single request can contain either queries or mutations - not both.
	// GraphQL validation on the request would have caught that error case
	// before we get here.  At this point, we know it's valid, it's passed
	// GraphQL validation and any additional validation we've added.  So here,
	// we can just execute it.
	switch {
	case op.IsQuery():
		// Queries run in parallel and are independent of each other: e.g.
		// an error in one query, doesn't affect the others.

		// TODO: this should handle all the queries in parallel.
		for _, q := range op.Queries() {
			qr := &QueryResolver{
				query:  q,
				schema: r.Schema,
				dgraph: r.dgraph,
			}
			resp, err := qr.Resolve(ctx)
			r.WithError(err)

			// Errors and data in the same response is valid.  Both WithError and
			// AddData handle nil cases.
			r.resp.AddData(resp)
		}
	case op.IsMutation():
		// Mutations, unlike queries, are handled serially and the results are
		// not independent: e.g. if one mutation errors, we don't run the
		// remaining mutations.
		for _, m := range op.Mutations() {
			if r.resp.Errors != nil {
				r.WithError(
					gqlerror.Errorf("mutation %s not executed because of previous error", m.Name()))
				continue
			}

			mr := &MutationResolver{
				mutation: m,
				schema:   r.Schema,
				dgraph:   r.dgraph,
			}
			resp, err := mr.Resolve(ctx)
			r.WithError(err)
			if len(resp) > 0 {
				var b bytes.Buffer
				b.WriteRune('"')
				b.WriteString(m.ResponseName())
				b.WriteString(`":`)
				b.Write(resp)

				r.resp.AddData(b.Bytes())
			}
		}
	case op.IsSubscription():
		schema.ErrorResponsef("Subscriptions not yet supported")
	}

	return r.resp
}
