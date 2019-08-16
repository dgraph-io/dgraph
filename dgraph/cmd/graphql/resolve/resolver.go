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
	"fmt"
	"sync"

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/api"
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

// A resolved is the result of resolving a single query or mutation.
// A schema.Request may contain any number of queries or mutations (never both).
// RequestResolver.Resolve() resolves all of them by finding the resolved answers
// of the component queries/mutations and joining into a single schema.Response.
type resolved struct {
	data []byte
	err  error
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

// Resolve processes r.GqlReq and returns a GraphQL response.
// r.GqlReq should be set with a request before Resolve is called
// and a schema and backend Dgraph should have been added.
// Resolve records any errors in the response's error field.
func (r *RequestResolver) Resolve(ctx context.Context) *schema.Response {
	if r == nil {
		glog.Errorf("[%s] Call to Resolve with nil RequestResolver", api.RequestID(ctx))
		return schema.ErrorResponsef("[%s] Internal error", api.RequestID(ctx))
	}

	if r.Schema == nil {
		glog.Errorf("[%s] Call to Resolve with no schema", api.RequestID(ctx))
		return schema.ErrorResponsef("[%s] Internal error", api.RequestID(ctx))
	}

	if r.resp.Errors != nil {
		r.resp.Data.Reset()
		return r.resp
	}

	op, err := r.Schema.Operation(r.GqlReq)
	if err != nil {
		return schema.ErrorResponse(err)
	}

	if glog.V(3) {
		glog.Infof("[%s] Resolving GQL request: \n%s\n",
			api.RequestID(ctx), r.GqlReq.Query)
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

		var wg sync.WaitGroup
		allResolved := make([]*resolved, len(op.Queries()))

		for i, q := range op.Queries() {
			wg.Add(1)

			go func(q schema.Query, storeAt int) {
				defer wg.Done()

				allResolved[storeAt] = (&queryResolver{
					query:  q,
					schema: r.Schema,
					dgraph: r.dgraph,
				}).resolve(ctx)
			}(q, i)
		}
		wg.Wait()

		// The GraphQL data response needs to be written in the same order as the
		// queries in the request.
		for _, res := range allResolved {
			// Errors and data in the same response is valid.  Both WithError and
			// AddData handle nil cases.
			r.WithError(res.err)
			r.resp.AddData(res.data)
		}
	case op.IsMutation():
		// Mutations, unlike queries, are handled serially and the results are
		// not independent: e.g. if one mutation errors, we don't run the
		// remaining mutations.
		for _, m := range op.Mutations() {
			if r.resp.Errors != nil {
				r.WithError(
					gqlerror.Errorf("[%s] mutation %s not executed because of previous error",
						api.RequestID(ctx), m.ResponseName()))
				continue
			}

			mr := &mutationResolver{
				mutation: m,
				schema:   r.Schema,
				dgraph:   r.dgraph,
			}
			res := mr.resolve(ctx)
			r.WithError(res.err)
			r.resp.AddData(res.data)
		}
	case op.IsSubscription():
		schema.ErrorResponsef("[%s] Subscriptions not yet supported", api.RequestID(ctx))
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
// query and dgResult as the matching full Dgraph result.  Always returns a valid
// JSON []byte of the form
//   { "query-name": null }
// if there's no result, or
//   { "query-name": ... }
// if there is a result.
func completeDgraphResult(ctx context.Context, field schema.Field, dgResult []byte) (
	[]byte, gqlerror.List) {
	// We need an initial case in the alg because Dgraph always returns a list
	// result no matter what.
	//
	// If the query was for a non-list type, that needs to be corrected:
	//
	//   { "q":[{ ... }] }  --->  { "q":{ ... } }
	//
	// Also, if the query found nothing at all, that needs correcting too:
	//
	//    { }  --->  { "q": null }

	var errs gqlerror.List
	errLoc := field.Location()

	nullResponse := func() []byte {
		var buf bytes.Buffer
		buf.WriteString(`{ "`)
		buf.WriteString(field.ResponseName())
		buf.WriteString(`": null }`)
		return buf.Bytes()
	}

	dgraphError := func() ([]byte, gqlerror.List) {
		glog.Errorf("[%s] Could not process Dgraph result : \n%s",
			api.RequestID(ctx), string(dgResult))
		return nullResponse(), gqlerror.List{
			&gqlerror.Error{
				Message: "Couldn't process result from Dgraph.  " +
					"This probably indicates a bug in the Dgraph GraphQL layer.  " +
					"Please let us know : https://github.com/dgraph-io/dgraph/issues.",
				Locations: []gqlerror.Location{{Line: errLoc.Line, Column: errLoc.Column}},
			}}
	}

	// Dgraph should only return {} or a JSON object.  Also,
	// GQL type checking should ensure query results are only object types
	// https://graphql.github.io/graphql-spec/June2018/#sec-Query
	// So we are only building object results.
	var valToComplete map[string]interface{}
	err := json.Unmarshal(dgResult, &valToComplete)
	if err != nil {
		glog.Errorf("[%s] %+v \n Dgraph result :\n%s\n",
			api.RequestID(ctx),
			errors.Wrap(err, "failed to unmarshal Dgraph query result"),
			string(dgResult))
		return nullResponse(), schema.AsGQLErrors(
			schema.GQLWrapLocationf(err, errLoc.Line, errLoc.Column,
				"Internal error (couldn't unmarshal Dgraph result)"))
	}

	switch val := valToComplete[field.ResponseName()].(type) {
	case []interface{}:
		if field.Type().ListType() == nil {
			// Turn Dgraph list result to single object
			// "q":[{ ... }] ---> "q":{ ... }

			var internalVal interface{}

			if len(val) > 0 {
				var ok bool
				if internalVal, ok = val[0].(map[string]interface{}); !ok {
					// This really shouldn't happen. Dgraph only returns arrays
					// of json objects.
					return dgraphError()
				}
			}

			if len(val) > 1 {
				// If we get here, then we got a list result for a query that expected
				// a single item.  That probably indicates a schema error, or maybe
				// a bug in GraphQL processing or some data corruption.
				//
				// We'll continue and just try the first item to return some data.

				glog.Errorf("[%s] Got a list result from Dgraph when expecting a one-item list.\n"+
					"GraphQL query was : %s\nDgraph Result was : %s\n",
					api.RequestID(ctx), api.QueryString(ctx), string(dgResult))

				errs = append(errs,
					&gqlerror.Error{
						Message: fmt.Sprintf(
							"Dgraph returned a list, but %s (type %s) was expecting just one item. "+
								"The first item in the list was used to produce the result. "+
								"Logged as a potential bug; see the API log for more details.",
							field.Name(), field.Type().String()),
						Locations: []gqlerror.Location{{Line: errLoc.Line, Column: errLoc.Column}},
					})
			}

			valToComplete[field.ResponseName()] = internalVal
		}
	default:
		if val != nil {
			return dgraphError()
		}

		// valToComplete[field.ResponseName()] is nil, so resolving for the
		// { } ---> "q": null
		// case
	}

	// Errors should report the "path" into the result where the error was found.
	//
	// The definition of a path in a GraphQL error is here:
	// https://graphql.github.io/graphql-spec/June2018/#sec-Errors
	// For a query like (assuming field f is of a list type and g is a scalar type):
	// - q { f { g } }
	// a path to the 2nd item in the f list would look like:
	// - [ "q", "f", 2, "g" ]
	path := make([]interface{}, 0, maxPathLength(field))

	completed, gqlErrs := completeObject(
		path, field.Type(), []schema.Field{field}, valToComplete)

	if len(completed) < 2 {
		// This could only occur completeObject crushed the whole query, but
		// that should never happen because the result type shouldn't be '!'.
		// We should wrap enough testing around the schema generation that this
		// just can't happen.
		//
		// This isn't really an observable GraphQL error, so no need to add anything
		// to the payload of errors for the result.
		glog.Errorf("[%s] Top level completeObject didn't return a result.  "+
			"That's only possible if the query result it non-nullable.  "+
			"There's something wrong in the GraphQL schema.  \n"+
			"GraphQL query was : %s\nDgraph Result was : %s\n",
			api.RequestID(ctx), api.QueryString(ctx), string(dgResult))
		return nullResponse(), errs
	}

	return completed, append(errs, gqlErrs...)
}

// completeObject builds a json GraphQL result object for the current query level.
// It returns a bracketed json object like { f1:..., f2:..., ... }.
//
// fields are all the fields from this bracketed level in the GraphQL  query, e.g:
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
func completeObject(path []interface{}, typ schema.Type, fields []schema.Field, res map[string]interface{}) (
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

		completed, err := completeValue(append(path, f.ResponseName()), f, res[f.ResponseName()])
		errs = append(errs, err...)
		if completed == nil {
			if !f.Type().Nullable() {
				return nil, errs
			}
			completed = []byte(`null`)
		}
		buf.Write(completed)
		comma = ", "
	}
	buf.WriteRune('}')

	return buf.Bytes(), errs
}

// completeValue applies the value completion algorithm to a single value, which
// could turn out to be a list or object or scalar value.
func completeValue(path []interface{}, field schema.Field, val interface{}) ([]byte, gqlerror.List) {

	switch val := val.(type) {
	case map[string]interface{}:
		return completeObject(path, field.Type(), field.SelectionSet(), val)
	case []interface{}:
		return completeList(path, field, val)
	default:
		if val == nil {
			if field.Type().ListType() != nil {
				// We could choose to set this to null.  This is our decision, not
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

			errLoc := field.Location()
			gqlErr := &gqlerror.Error{
				Message: fmt.Sprintf(
					"Non-nullable field '%s' (type %s) was not present in result from Dgraph.  "+
						"GraphQL error propagation triggered.", field.Name(), field.Type()),
				Locations: []gqlerror.Location{{Line: errLoc.Line, Column: errLoc.Column}},
				Path:      copyPath(path),
			}
			return nil, gqlerror.List{gqlErr}
		}

		// val is a scalar

		// Can this ever error?  We can't have an unsupported type or value because
		// we just unmarshaled this val.
		json, err := json.Marshal(val)
		if err != nil {
			errLoc := field.Location()
			gqlErr := &gqlerror.Error{
				Message: fmt.Sprintf(
					"Error marshalling value for field '%s' (type %s).  "+
						"Resolved as null (which may trigger GraphQL error propagation) ",
					field.Name(), field.Type()),
				Locations: []gqlerror.Location{{Line: errLoc.Line, Column: errLoc.Column}},
				Path:      copyPath(path),
			}

			if field.Type().Nullable() {
				return []byte("null"), gqlerror.List{gqlErr}
			}

			return nil, gqlerror.List{gqlErr}
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
func completeList(path []interface{}, field schema.Field, values []interface{}) ([]byte, gqlerror.List) {
	var buf bytes.Buffer
	var errs gqlerror.List
	comma := ""

	if field.Type().ListType() == nil {
		// This means a bug on our part - in rewriting, schema generation,
		// or Dgraph returned somthing unexpected.
		//
		// Let's crush it to null so we still get something from the rest of the
		// query and log the error.
		return mismatched(path, field, values)
	}

	buf.WriteRune('[')
	for i, b := range values {
		r, err := completeValue(append(path, i), field, b)
		errs = append(errs, err...)
		buf.WriteString(comma)
		if r == nil {
			if !field.Type().ListType().Nullable() {
				// Unlike the choice in completeValue() above, where we turn missing
				// lists into [], the spec explicitly calls out:
				//  "If a List type wraps a Non-Null type, and one of the
				//  elements of that list resolves to null, then the entire list
				//  must resolve to null."
				//
				// The list gets reduced to nil, but an error recording that must
				// already be in errs.  See
				// https://graphql.github.io/graphql-spec/June2018/#sec-Errors-and-Non-Nullability
				// "If the field returns null because of an error which has already
				// been added to the "errors" list in the response, the "errors"
				// list must not be further affected."
				// The behavior is also in the examples in here:
				// https://graphql.github.io/graphql-spec/June2018/#sec-Errors
				return nil, errs
			}
			buf.WriteString("null")
		} else {
			buf.Write(r)
		}
		comma = ", "
	}
	buf.WriteRune(']')

	return buf.Bytes(), errs
}

func mismatched(path []interface{}, field schema.Field, values []interface{}) ([]byte, gqlerror.List) {
	errLoc := field.Location()

	glog.Error("completeList() called in resolving %s (Line: %v, Column: %v), but its type is %s.\n"+
		"That could indicate the Dgraph schema doesn't match the GraphQL schema."+
		field.Name(), errLoc.Line, errLoc.Column, field.Type().Name())

	gqlErr := &gqlerror.Error{
		Message: "Dgraph returned a list, but GraphQL was expecting just one item.  " +
			"This indicates an internal error - " +
			"probably a mismatch between GraphQL and Dgraph schemas.  " +
			"The value was resolved as null (which may trigger GraphQL error propagation) " +
			"and as much other data as possible returned.",
		Locations: []gqlerror.Location{{Line: errLoc.Line, Column: errLoc.Column}},
		Path:      copyPath(path),
	}

	val, errs := completeValue(path, field, nil)
	return val, append(errs, gqlErr)
}

func copyPath(path []interface{}) []interface{} {
	result := make([]interface{}, len(path))
	copy(result, path)
	return result
}

// maxPathLength finds the max length (including list indexes) of any path in the 'query' f.
// Used to pre-allocate a path buffer of the correct size before running completeObject on
// the top level query - means that we aren't reallocating slices multiple times
// during the complete* functions.
func maxPathLength(f schema.Field) int {
	childMax := 0
	for _, chld := range f.SelectionSet() {
		d := maxPathLength(chld)
		if d > childMax {
			childMax = d
		}
	}
	if f.Type().ListType() != nil {
		// It's f: [...], so add a space for field name and
		// a space for the index into the list
		return 2 + childMax
	}

	return 1 + childMax
}
