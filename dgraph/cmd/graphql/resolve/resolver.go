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
	"github.com/pkg/errors"

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

// There's three basic types to consider here: GraphQL object types (equals json
// objects in the result), list types (equals lists of objects or scalars), and
// values (either scalar values, lists or objects).
//
// So the algorithm is a three way mutual recursion between those types.
//
// That works like this... if part of the json result from Dgraph
// looked like:
//
// {
//   "name": "A name"
//   "friends": [
//     { "name": "Friend 1"},
//     { "name": "Friend 2", "friends": [...] }
//   ]
// }
//
// Then, schematically, the recursion tree would look like:
//
// completeObject ( {
//   "name": completeValue("A name")
//   "friends": completeValue( completeList([
//     completeValue (completeObject ({ "name": completeValue ("Friend 1")} )),
//     completeValue (completeObject ({
//                           "name": completeValue("Friend 2"),
//                           "friends": completeValue ( completeList([ completeObject(..), ..]) } )
//

// completeDgraphResult starts the recursion with field as the top level GraphQL
// query and dgResult as the matching full Dgraph result.
func completeDgraphResult(field schema.Field, dgResult []byte) ([]byte, gqlerror.List) {
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

	// Dgraph should only return {} or a JSON object.  Also,
	// GQL type checking should ensure query results are only object types
	// https://graphql.github.io/graphql-spec/June2018/#sec-Query
	// So we are only building object results.
	var valToComplete map[string]interface{}
	err := json.Unmarshal(dgResult, &valToComplete)
	if err != nil {
		glog.Errorf("%v+", errors.Wrap(err, "Failed to unmarshal Dgraph query result"))
		return nil, schema.AsGQLErrors(
			schema.GQLWrapf(err, "internal error, couldn't unmarshal dgraph result"))
	}

	switch val := valToComplete[field.ResponseName()].(type) {
	case []interface{}:
		if field.Type().ListType() == nil {
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

			valToComplete[field.ResponseName()] = internalVal
		} else {
			valToComplete[field.ResponseName()] = val
		}
	default:
		if val != nil {
			return dgraphError(val)
		}

		valToComplete[field.ResponseName()] = nil
	}

	completed, gqlErrs := completeObject(field.Type(), []schema.Field{field}, valToComplete)
	if len(completed) > 2 {
		// chop leading '{' and trailing '}' from JSON object
		completed = completed[1 : len(completed)-1]
	}

	return completed, append(errs, gqlErrs...)
}

// completeObject builds a json GraphQL result object for the current query level.
//
// fields are all the fields from this bracketed level in the graphql query, e.g:
// {
//   name
//   dob
//   friends {...}
// }
// If it's the top level of a query then it'll be the top level query name.
//
// typ is the expected type matching those fields, e.g. above that'd be something
// like the `Person` type that has fields name, dob and friends.
//
// res is the results map from Dgraph for this level of the query.  This map needn't
// contain values for all the requested fields, e.g. if there's no corresponding
// values in the store or if the query contained a filter that excluded a value.
// So res might be the map : name->"A Name", friends -> []interface{}
//
// completeObject fills out this result putting in null for any missing values
// (dob above) and applying GraphQL error propagation for any null fields that the
// schema says can't be null.
//
// Example:
//
// if the map is name->"A Name", friends -> []interface{}
//
// and "dob" is nullable then the result should be json object
// {"name": "A Name", "dob": null, "friends": ABC}
// where ABC is the result of applying completeValue to res["friends"]
//
// if "dob" were non-nullable (maybe it's type is DateTime!), then the result is
// nil and the error propagates to the enclosing level.
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

// completeValue applies the value completion algorithm to a single value, which
// could turn out to be a list or object or scalar value.
func completeValue(field schema.Field, val interface{}) ([]byte, gqlerror.List) {

	switch val := val.(type) {
	case map[string]interface{}:
		return completeObject(field.Type(), field.SelectionSet(), val)
	case []interface{}:
		return completeList(field, val)
	default:
		if val == nil {
			if field.Type().ListType() != nil {
				// We could chose to set this to null.  This is our decision, not
				// anything required by the GraphQL spec.
				//
				// However, if we query, for example, for a persons's friends with
				// some restrictions, and there aren't any, is that really a case to
				// set this at null and error if the list is required?  What
				// about if an person has just been added and doesn't have any friends?
				// Doesn't seem right to add null and cause error propagation.
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

// completeList applies the completion algorithm to a list field and result.
//
// field is one field from the query - which should have a list type in the
// GraphQL schema.
//
// values is the list of values found by the query for this field.
//
// completeValue() is applied to every list element, but
// the type of field can only be a scalar list like [String], or an object
// list like [Person], so schematically the final result is either
// [ completValue("..."), completValue("..."), ... ]
// or
// [ completeObject({...}), completeObject({...}), ... ]
// depending on the type of list.
//
// If the list has non-nullable elements (a type like [T!]) and any of those
// elements resolve to null, then the whole list is crushed to null.
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
