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
	"encoding/json"

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

			// There's one more step of the completion algorithm not done yet.  If the
			// result type of this query is T! and completeDgraphResult comes back nil.
			// Then we really should crush the whole query: e.g. if multiple queries
			// were asked in one operation, we should call off any other runnings
			// resolvers for the operation and crush the whole result to an error.
			//
			// ATM we just squash this one query, return the error and the results
			// of all other querie.
			// TODO: ^^  should we even have ! return types in queries?  and/or
			// should add that last step of error propagation with cancelation
			// when we make queries concurrent.
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

// Once a result has been returned from Dgraph, that result needs to be worked
// through for two main reasons:
//
// 1) (null insertion)
//    Where an edge was requested in a query, but isn't in the store, Dgraph just
//    won't return an edge for that in the results.  But GraphQL wants those as
//    "null" in the result.  And then we need to inspect those nulls via pt (2)
//
// 2) (error propagation)
//    The schema is a contract with consumers.  So if there's an `f: T!` in the
//    schema, that says: "this API never returns a null f".  If f turned out null
//    in the results, then returning null would break the contract.  GraphQL specifies
//    a set of rules about how to propagate and record those errors.
//
//    The basic intuition is that if we asked for something that's nullable and we
//    got back a null/error, then that's fine, just set it to null.  But if we asked
//    for something non-nullable and got a null/error, then the object we are building
//    is in an error state, and we should propagate that up to it's parent, and so
//    on, untill we reach a nullable field, or the top level.
//
// The completeXYZ() functions below essentially covers the value completion alg from
// https://graphql.github.io/graphql-spec/June2018/#sec-Value-Completion.
// see also: error propagation
// https://graphql.github.io/graphql-spec/June2018/#sec-Errors-and-Non-Nullability
// and the spec requirements for response
// https://graphql.github.io/graphql-spec/June2018/#sec-Response.

func completeDgraphResult(query schema.Query, val interface{}) ([]byte, gqlerror.List) {
	// We need an intial case in the alg because dgraph always returns a list
	// result no mater what.
	//
	// If the query was for a non-list type, that needs to be corrected:
	//
	//   "q":[{ ... }] ---> "q":{ ... }
	//
	// Also, if the query found nothing at all, that needs correcting too:
	//
	//    { } ---> "q": null

	var errs gqlerror.List

	dgraphError := func(v interface{}) ([]byte, gqlerror.List) {
		json, _ := json.Marshal(v)
		glog.Error("Dgraph return type was not an array of json objects : ",
			string(json))
		return nil, gqlerror.List{
			gqlerror.Errorf("Result of Dgraph query was not of a recognisable form.  " +
				"Please let us know : https://github.com/dgraph-io/dgraph/issues.")}
	}

	// GQL type checking should ensure query results are only object types
	// https://graphql.github.io/graphql-spec/June2018/#sec-Query
	// So we are only building object results.
	valToComplete := make(map[string]interface{})

	switch val := val.(type) {
	case []interface{}:
		if query.Type().ListType() == nil {
			var internalVal interface{}

			if len(val) > 0 {
				var ok bool
				if internalVal, ok = val[0].(map[string]interface{}); !ok {
					// This really shouldn't happen. Dgraph only returns arrays
					// of json objects.
					return dgraphError(val)
				}
			}

			if len(val) > 1 {
				// If we get here, then we got a list result for a query that expected
				// a single item.  That probably indicates a bug in GraphQL processing
				// or maybe some data corruption.
				//
				// We'll continue and just try the first item to return some data.
				json, _ := json.Marshal(val)
				glog.Error("Got a list result from Dgraph when expecting a one-item list : ",
					string(json))

				errs = append(errs,
					gqlerror.Errorf("Dgraph returned a list, but was expecting just one item."))

				// FIXME: Would be great to be able to log more context in a case like this.
				// If glog is set to 3+, we log they query, etc, but would be nice just when
				// there's an error to have more no mater what the log level.
			}

			valToComplete[query.ResponseName()] = internalVal
		} else {
			valToComplete[query.ResponseName()] = val
		}
	default:
		if val != nil {
			return dgraphError(val)
		}

		valToComplete[query.ResponseName()] = nil
	}

	completed, gqlErrs := completeObject(query.Type(), []schema.Field{query}, valToComplete)
	if len(completed) > 2 {
		// chop leading '{' and trailing '}' from JSON object
		completed = completed[1 : len(completed)-1]
	}

	return completed, append(errs, gqlErrs...)
}

func completeObject(typ schema.Type, fields []schema.Field, res map[string]interface{}) (
	[]byte, gqlerror.List) {

	var errs gqlerror.List
	var buf bytes.Buffer
	comma := ""

	buf.WriteRune('{')
	for _, f := range fields {
		buf.WriteString(comma)
		buf.WriteRune('"')
		buf.WriteString(f.ResponseName())
		buf.WriteString(`": `)

		completed, err := completeValue(f, res[f.ResponseName()])
		errs = append(errs, err...)
		if completed == nil {
			if f.Type().Nullable() {
				completed = []byte(`null`)
			} else {
				return nil, append(errs, gqlerror.Errorf("Crushed an object because of null error"))
			}
		}
		buf.Write(completed)
		comma = ", "
	}
	buf.WriteRune('}')

	return buf.Bytes(), errs
}

func completeValue(field schema.Field, val interface{}) ([]byte, gqlerror.List) {

	switch val := val.(type) {
	case map[string]interface{}:
		return completeObject(field.Type(), field.SelectionSet(), val)
	case []interface{}:
		return completeList(field, val)
	default:
		if val == nil {
			if field.Type().ListType() != nil {
				// We could chose to set this to null.  This is our decisions, not
				// anything required by the spec.
				//
				// However, if we query, for example, for an author's posts with
				// some restrictions, and there aren't any, is that really a case to
				// set this at null and error if the list is required?  What
				// about if an author has just been added and doesn't have any posts?
				// Doesn't seem right.
				//
				// Seems best if we pick [], rather than null, as the list value if
				// there's nothing in the Dgraph result.
				return []byte("[]"), nil
			}

			if field.Type().Nullable() {
				return []byte("null"), nil
			}

			return nil, gqlerror.List{gqlerror.Errorf("Crushed a null value")}
		}

		// val is a scalar
		json, err := json.Marshal(val)
		if err != nil {
			if field.Type().Nullable() {
				return []byte("null"), nil
			}

			return nil, gqlerror.List{gqlerror.Errorf("json marshalling failed")}
		}
		return json, nil
	}
}

func completeList(field schema.Field, values []interface{}) ([]byte, gqlerror.List) {
	var buf bytes.Buffer
	var errs gqlerror.List
	comma := ""

	if field.Type().ListType() == nil {
		// This means a bug on our part - in rewriting, schema generation,
		// or Dgraph returned somthing unexpected.
		//
		// For now, just crush it - this'll change in next PR that smooths out error meesages.
		return completeValue(field, nil)
	}

	buf.WriteRune('[')
	for _, b := range values {
		r, err := completeValue(field, b)
		errs = append(errs, err...)
		buf.WriteString(comma)
		if r == nil {
			if field.Type().ListType().Nullable() {
				buf.WriteString("null")
			} else {
				// Unlike the choice in completeValue() above, where we turn missing
				// lists into [], the spec explicitly calls out:
				//  "If a List type wraps a Non-Null type, and one of the
				//  elements of that list resolves to null, then the entire list
				//  must resolve to null."
				return nil, append(errs, gqlerror.Errorf("Crushed a list because of null error"))
			}
		} else {
			buf.Write(r)
		}
		comma = ", "
	}
	buf.WriteRune(']')

	return buf.Bytes(), errs
}
