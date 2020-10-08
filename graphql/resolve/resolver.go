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
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/dgraph/graphql/authorization"

	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/graphql/dgraph"
	"github.com/dgraph-io/dgraph/types"

	dgoapi "github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/graphql/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	otrace "go.opencensus.io/trace"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/graphql/schema"
)

type resolveCtxKey string

const (
	methodResolve = "RequestResolver.Resolve"

	resolveStartTime resolveCtxKey = "resolveStartTime"

	resolverFailed    = false
	resolverSucceeded = true

	errExpectedScalar = "A scalar type was returned, but GraphQL was expecting an object. " +
		"This indicates an internal error - " +
		"probably a mismatch between the GraphQL and Dgraph/remote schemas. " +
		"The value was resolved as null (which may trigger GraphQL error propagation) " +
		"and as much other data as possible returned."

	errExpectedObject = "A list was returned, but GraphQL was expecting just one item. " +
		"This indicates an internal error - " +
		"probably a mismatch between the GraphQL and Dgraph/remote schemas. " +
		"The value was resolved as null (which may trigger GraphQL error propagation) " +
		"and as much other data as possible returned."

	errExpectedList = "An object was returned, but GraphQL was expecting a list of objects. " +
		"This indicates an internal error - " +
		"probably a mismatch between the GraphQL and Dgraph/remote schemas. " +
		"The value was resolved as null (which may trigger GraphQL error propagation) " +
		"and as much other data as possible returned."

	errInternal = "Internal error"

	errExpectedNonNull = "Non-nullable field '%s' (type %s) was not present in result from Dgraph.  " +
		"GraphQL error propagation triggered."
)

// A ResolverFactory finds the right resolver for a query/mutation.
type ResolverFactory interface {
	queryResolverFor(query schema.Query) QueryResolver
	mutationResolverFor(mutation schema.Mutation) MutationResolver

	// WithQueryResolver adds a new query resolver.  Each time query name is resolved
	// resolver is called to create a new instance of a QueryResolver to resolve the
	// query.
	WithQueryResolver(name string, resolver func(schema.Query) QueryResolver) ResolverFactory

	// WithMutationResolver adds a new query resolver.  Each time mutation name is resolved
	// resolver is called to create a new instance of a MutationResolver to resolve the
	// mutation.
	WithMutationResolver(
		name string, resolver func(schema.Mutation) MutationResolver) ResolverFactory

	// WithConventionResolvers adds a set of our convention based resolvers to the
	// factory.  The registration happens only once.
	WithConventionResolvers(s schema.Schema, fns *ResolverFns) ResolverFactory

	// WithQueryMiddlewareConfig adds the configuration to use to apply middlewares before resolving
	// queries. The config should be a mapping of the name of query to its middlewares.
	WithQueryMiddlewareConfig(config map[string]QueryMiddlewares) ResolverFactory

	// WithMutationMiddlewareConfig adds the configuration to use to apply middlewares before
	// resolving mutations. The config should be a mapping of the name of mutation to its
	// middlewares.
	WithMutationMiddlewareConfig(config map[string]MutationMiddlewares) ResolverFactory

	// WithSchemaIntrospection adds schema introspection capabilities to the factory.
	// So __schema and __type queries can be resolved.
	WithSchemaIntrospection() ResolverFactory
}

// A ResultCompleter can take a []byte slice representing an intermediate result
// in resolving field and applies a completion step - for example, apply GraphQL
// error propagation or massaging error paths.
type ResultCompleter interface {
	Complete(ctx context.Context, resolved *Resolved)
}

// RequestResolver can process GraphQL requests and write GraphQL JSON responses.
// A schema.Request may contain any number of queries or mutations (never both).
// RequestResolver.Resolve() resolves all of them by finding the resolved answers
// of the component queries/mutations and joining into a single schema.Response.
type RequestResolver struct {
	schema    schema.Schema
	resolvers ResolverFactory
}

// A resolverFactory is the main implementation of ResolverFactory.  It stores a
// map of all the resolvers that have been registered and returns a resolver that
// just returns errors if it's asked for a resolver for a field that it doesn't
// know about.
type resolverFactory struct {
	queryResolvers    map[string]func(schema.Query) QueryResolver
	mutationResolvers map[string]func(schema.Mutation) MutationResolver

	queryMiddlewareConfig    map[string]QueryMiddlewares
	mutationMiddlewareConfig map[string]MutationMiddlewares

	// returned if the factory gets asked for resolver for a field that it doesn't
	// know about.
	queryError    QueryResolverFunc
	mutationError MutationResolverFunc
}

// ResolverFns is a convenience struct for passing blocks of rewriters and executors.
type ResolverFns struct {
	Qrw QueryRewriter
	Arw func() MutationRewriter
	Urw func() MutationRewriter
	Drw MutationRewriter
	Ex  DgraphExecutor
}

// dgraphExecutor is an implementation of both QueryExecutor and MutationExecutor
// that proxies query/mutation resolution through Query method in dgraph server.
type dgraphExecutor struct {
	dg *dgraph.DgraphEx
}

// adminExecutor is an implementation of both QueryExecutor and MutationExecutor
// that proxies query resolution through Query method in dgraph server, and
// it doesn't require authorization. Currently it's only used for querying
// gqlschema during init.
type adminExecutor struct {
	dg *dgraph.DgraphEx
}

// A Resolved is the result of resolving a single field - generally a query or mutation.
type Resolved struct {
	Data       interface{}
	Field      schema.Field
	Err        error
	Extensions *schema.Extensions
}

// restErr is Error returned from custom REST endpoint
type restErr struct {
	Errors x.GqlErrorList `json:"errors,omitempty"`
}

// CompletionFunc is an adapter that allows us to compose completions and build a
// ResultCompleter from a function.  Based on the http.HandlerFunc pattern.
type CompletionFunc func(ctx context.Context, resolved *Resolved)

// Complete calls cf(ctx, resolved)
func (cf CompletionFunc) Complete(ctx context.Context, resolved *Resolved) {
	cf(ctx, resolved)
}

// NewDgraphExecutor builds a DgraphExecutor for proxying requests through dgraph.
func NewDgraphExecutor() DgraphExecutor {
	return newDgraphExecutor(&dgraph.DgraphEx{})
}

func newDgraphExecutor(dg *dgraph.DgraphEx) DgraphExecutor {
	return &dgraphExecutor{dg: dg}
}

// NewAdminExecutor builds a DgraphExecutor for proxying requests through dgraph.
func NewAdminExecutor() DgraphExecutor {
	return &adminExecutor{dg: &dgraph.DgraphEx{}}
}

func (aex *adminExecutor) Execute(ctx context.Context, req *dgoapi.Request) (
	*dgoapi.Response, error) {
	ctx = context.WithValue(ctx, edgraph.Authorize, false)
	return aex.dg.Execute(ctx, req)
}

func (aex *adminExecutor) CommitOrAbort(ctx context.Context, tc *dgoapi.TxnContext) error {
	return aex.dg.CommitOrAbort(ctx, tc)
}

func (de *dgraphExecutor) Execute(ctx context.Context, req *dgoapi.Request) (
	*dgoapi.Response, error) {
	return de.dg.Execute(ctx, req)
}

func (de *dgraphExecutor) CommitOrAbort(ctx context.Context, tc *dgoapi.TxnContext) error {
	return de.dg.CommitOrAbort(ctx, tc)
}

func (rf *resolverFactory) WithQueryResolver(
	name string, resolver func(schema.Query) QueryResolver) ResolverFactory {
	rf.queryResolvers[name] = resolver
	return rf
}

func (rf *resolverFactory) WithMutationResolver(
	name string, resolver func(schema.Mutation) MutationResolver) ResolverFactory {
	rf.mutationResolvers[name] = resolver
	return rf
}

func (rf *resolverFactory) WithSchemaIntrospection() ResolverFactory {
	return rf.
		WithQueryResolver("__schema",
			func(q schema.Query) QueryResolver {
				return QueryResolverFunc(resolveIntrospection)
			}).
		WithQueryResolver("__type",
			func(q schema.Query) QueryResolver {
				return QueryResolverFunc(resolveIntrospection)
			}).
		WithQueryResolver("__typename",
			func(q schema.Query) QueryResolver {
				return QueryResolverFunc(resolveIntrospection)
			})
}

func (rf *resolverFactory) WithConventionResolvers(
	s schema.Schema, fns *ResolverFns) ResolverFactory {

	queries := append(s.Queries(schema.GetQuery), s.Queries(schema.FilterQuery)...)
	queries = append(queries, s.Queries(schema.PasswordQuery)...)
	for _, q := range queries {
		rf.WithQueryResolver(q, func(q schema.Query) QueryResolver {
			return NewQueryResolver(fns.Qrw, fns.Ex, StdQueryCompletion())
		})
	}

	for _, q := range s.Queries(schema.HTTPQuery) {
		rf.WithQueryResolver(q, func(q schema.Query) QueryResolver {
			return NewHTTPQueryResolver(&http.Client{
				// TODO - This can be part of a config later.
				Timeout: time.Minute,
			}, StdQueryCompletion())
		})
	}

	for _, q := range s.Queries(schema.DQLQuery) {
		rf.WithQueryResolver(q, func(q schema.Query) QueryResolver {
			// DQL queries don't need any QueryRewriter
			return NewQueryResolver(nil, fns.Ex, StdQueryCompletion())
		})
	}

	for _, m := range s.Mutations(schema.AddMutation) {
		rf.WithMutationResolver(m, func(m schema.Mutation) MutationResolver {
			return NewDgraphResolver(fns.Arw(), fns.Ex, StdMutationCompletion(m.Name()))
		})
	}

	for _, m := range s.Mutations(schema.UpdateMutation) {
		rf.WithMutationResolver(m, func(m schema.Mutation) MutationResolver {
			return NewDgraphResolver(fns.Urw(), fns.Ex, StdMutationCompletion(m.Name()))
		})
	}

	for _, m := range s.Mutations(schema.DeleteMutation) {
		rf.WithMutationResolver(m, func(m schema.Mutation) MutationResolver {
			return NewDgraphResolver(fns.Drw, fns.Ex, deleteCompletion())
		})
	}

	for _, m := range s.Mutations(schema.HTTPMutation) {
		rf.WithMutationResolver(m, func(m schema.Mutation) MutationResolver {
			return NewHTTPMutationResolver(&http.Client{
				// TODO - This can be part of a config later.
				Timeout: time.Minute,
			}, StdQueryCompletion())
		})
	}

	return rf
}

func (rf *resolverFactory) WithQueryMiddlewareConfig(
	config map[string]QueryMiddlewares) ResolverFactory {
	if len(config) != 0 {
		rf.queryMiddlewareConfig = config
	}
	return rf
}

func (rf *resolverFactory) WithMutationMiddlewareConfig(
	config map[string]MutationMiddlewares) ResolverFactory {
	if len(config) != 0 {
		rf.mutationMiddlewareConfig = config
	}
	return rf
}

// NewResolverFactory returns a ResolverFactory that resolves requests via
// query/mutation rewriting and execution through Dgraph.  If the factory gets asked
// to resolve a query/mutation it doesn't know how to rewrite, it uses
// the queryError/mutationError to build an error result.
func NewResolverFactory(
	queryError QueryResolverFunc, mutationError MutationResolverFunc) ResolverFactory {

	return &resolverFactory{
		queryResolvers:    make(map[string]func(schema.Query) QueryResolver),
		mutationResolvers: make(map[string]func(schema.Mutation) MutationResolver),

		queryMiddlewareConfig:    make(map[string]QueryMiddlewares),
		mutationMiddlewareConfig: make(map[string]MutationMiddlewares),

		queryError:    queryError,
		mutationError: mutationError,
	}
}

// StdQueryCompletion is the completion steps that get run for queries
func StdQueryCompletion() CompletionFunc {
	return noopCompletion
}

// StdMutationCompletion is the completion steps that get run for add and update mutations
func StdMutationCompletion(name string) CompletionFunc {
	return noopCompletion
}

// StdDeleteCompletion is the completion steps that get run for delete mutations
func StdDeleteCompletion(name string) CompletionFunc {
	return deleteCompletion()
}

func (rf *resolverFactory) queryResolverFor(query schema.Query) QueryResolver {
	mws := rf.queryMiddlewareConfig[query.Name()]
	if resolver, ok := rf.queryResolvers[query.Name()]; ok {
		return mws.Then(resolver(query))
	}

	return rf.queryError
}

func (rf *resolverFactory) mutationResolverFor(mutation schema.Mutation) MutationResolver {
	mws := rf.mutationMiddlewareConfig[mutation.Name()]
	if resolver, ok := rf.mutationResolvers[mutation.Name()]; ok {
		return mws.Then(resolver(mutation))
	}

	return rf.mutationError
}

// New creates a new RequestResolver.
func New(s schema.Schema, resolverFactory ResolverFactory) *RequestResolver {
	return &RequestResolver{
		schema:    s,
		resolvers: resolverFactory,
	}
}

// Resolve processes r.GqlReq and returns a GraphQL response.
// r.GqlReq should be set with a request before Resolve is called
// and a schema and backend Dgraph should have been added.
// Resolve records any errors in the response's error field.
func (r *RequestResolver) Resolve(ctx context.Context, gqlReq *schema.Request) *schema.Response {
	span := otrace.FromContext(ctx)
	stop := x.SpanTimer(span, methodResolve)
	defer stop()

	if r == nil {
		glog.Errorf("Call to Resolve with nil RequestResolver")
		return schema.ErrorResponse(errors.New(errInternal))
	}

	if r.schema == nil {
		glog.Errorf("Call to Resolve with no schema")
		return schema.ErrorResponse(errors.New(errInternal))
	}

	startTime := time.Now()
	resp := &schema.Response{
		Extensions: &schema.Extensions{
			Tracing: &schema.Trace{
				Version:   1,
				StartTime: startTime.Format(time.RFC3339Nano),
			},
		},
	}
	defer func() {
		endTime := time.Now()
		resp.Extensions.Tracing.EndTime = endTime.Format(time.RFC3339Nano)
		resp.Extensions.Tracing.Duration = endTime.Sub(startTime).Nanoseconds()
	}()
	ctx = context.WithValue(ctx, resolveStartTime, startTime)

	op, err := r.schema.Operation(gqlReq)
	if err != nil {
		return schema.ErrorResponse(err)
	}

	if glog.V(3) {
		// don't log the introspection queries they are sent too frequently
		// by GraphQL dev tools
		if !op.IsQuery() ||
			(op.IsQuery() && !strings.HasPrefix(op.Queries()[0].Name(), "__")) {
			b, err := json.Marshal(gqlReq.Variables)
			if err != nil {
				glog.Infof("Failed to marshal variables for logging : %s", err)
			}
			glog.Infof("Resolving GQL request: \n%s\nWith Variables: \n%s\n",
				gqlReq.Query, string(b))
		}
	}

	// resolveQueries will resolve user's queries.
	resolveQueries := func() {
		// Queries run in parallel and are independent of each other: e.g.
		// an error in one query, doesn't affect the others.

		var wg sync.WaitGroup
		allResolved := make([]*Resolved, len(op.Queries()))

		for i, q := range op.Queries() {
			wg.Add(1)

			go func(q schema.Query, storeAt int) {
				defer wg.Done()
				defer api.PanicHandler(
					func(err error) {
						allResolved[storeAt] = &Resolved{
							Data:  nil,
							Field: q,
							Err:   err,
						}
					})
				allResolved[storeAt] = r.resolvers.queryResolverFor(q).Resolve(ctx, q)
			}(q, i)
		}
		wg.Wait()

		// The GraphQL data response needs to be written in the same order as the
		// queries in the request.
		for _, res := range allResolved {
			// Errors and data in the same response is valid.  Both WithError and
			// AddData handle nil cases.
			addResult(resp, res)

		}
	}
	// A single request can contain either queries or mutations - not both.
	// GraphQL validation on the request would have caught that error case
	// before we get here.  At this point, we know it's valid, it's passed
	// GraphQL validation and any additional validation we've added.  So here,
	// we can just execute it.
	switch {
	case op.IsQuery():
		resolveQueries()
	case op.IsMutation():
		// A mutation operation can contain any number of mutation fields.  Those should be executed
		// serially.
		// (spec https://graphql.github.io/graphql-spec/June2018/#sec-Normal-and-Serial-Execution)
		//
		// The spec is ambiguous about what to do in the case of errors during that serial execution
		// - apparently deliberately so; see this comment from Lee Byron:
		// https://github.com/graphql/graphql-spec/issues/277#issuecomment-385588590
		// and clarification
		// https://github.com/graphql/graphql-spec/pull/438
		//
		// A reasonable interpretation of that is to stop a list of mutations after the first error -
		// which seems like the natural semantics and is what we enforce here.
		allSuccessful := true

		for _, m := range op.Mutations() {
			if !allSuccessful {
				resp.WithError(x.GqlErrorf(
					"Mutation %s was not executed because of a previous error.",
					m.ResponseName()).
					WithLocations(m.Location()))

				continue
			}

			var res *Resolved
			res, allSuccessful = r.resolvers.mutationResolverFor(m).Resolve(ctx, m)
			addResult(resp, res)
		}
	case op.IsSubscription():
		resolveQueries()
	}

	return resp
}

// ValidateSubscription will check the given subscription query is valid or not.
func (r *RequestResolver) ValidateSubscription(req *schema.Request) error {
	if r.schema == nil {
		glog.Errorf("Call to ValidateSubscription with no schema")
		return errors.New(errInternal)
	}

	op, err := r.schema.Operation(req)
	if err != nil {
		return err
	}

	for _, q := range op.Queries() {
		for _, field := range q.SelectionSet() {
			if err := validateCustomFieldsRecursively(field); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateCustomFieldsRecursively will return err if the given field is custom or any of its
// children is type of a custom field.
func validateCustomFieldsRecursively(field schema.Field) error {
	if has, _ := field.HasCustomDirective(); has {
		return x.GqlErrorf("Custom field `%s` is not supported in graphql subscription",
			field.Name()).WithLocations(field.Location())
	}
	for _, f := range field.SelectionSet() {
		err := validateCustomFieldsRecursively(f)
		if err != nil {
			return err
		}
	}
	return nil
}

func addResult(resp *schema.Response, res *Resolved) {
	// Errors should report the "path" into the result where the error was found.
	//
	// The definition of a path in a GraphQL error is here:
	// https://graphql.github.io/graphql-spec/June2018/#sec-Errors
	// For a query like (assuming field f is of a list type and g is a scalar type):
	// - q { f { g } }
	// a path to the 2nd item in the f list would look like:
	// - [ "q", "f", 2, "g" ]
	path := make([]interface{}, 0, maxPathLength(res.Field))
	var b []byte
	var gqlErr x.GqlErrorList

	if res.Data != nil {
		b, gqlErr = completeObject(path, []schema.Field{res.Field},
			res.Data.(map[string]interface{}))
	}

	resp.WithError(res.Err)
	resp.WithError(gqlErr)
	resp.AddData(b)
	resp.MergeExtensions(res.Extensions)
}

// noopCompletion just passes back it's result and err arguments
func noopCompletion(ctx context.Context, resolved *Resolved) {}

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
//    on, until we reach a nullable field, or the top level.
//
// The completeXYZ() functions below essentially covers the value completion alg from
// https://graphql.github.io/graphql-spec/June2018/#sec-Value-Completion.
// see also: error propagation
// https://graphql.github.io/graphql-spec/June2018/#sec-Errors-and-Non-Nullability
// and the spec requirements for response
// https://graphql.github.io/graphql-spec/June2018/#sec-Response.
//
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
//
// Returned errors are generally lists of errors resulting from the value completion
// algorithm that may emit multiple errors
func completeDgraphResult(
	ctx context.Context,
	field schema.Field,
	dgResult []byte,
	e error) *Resolved {

	span := trace.FromContext(ctx)
	stop := x.SpanTimer(span, "completeDgraphResult")
	defer stop()

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

	nullResponse := func(err error) *Resolved {
		return &Resolved{
			Data:  nil,
			Field: field,
			Err:   err,
		}
	}

	dgraphError := func() *Resolved {
		glog.Errorf("Could not process Dgraph result : \n%s", string(dgResult))
		return nullResponse(
			x.GqlErrorf("Couldn't process the result from Dgraph.  " +
				"This probably indicates a bug in Dgraph. Please let us know by filing an issue.").
				WithLocations(field.Location()))
	}

	if len(dgResult) == 0 {
		return nullResponse(e)
	}

	errs := schema.AsGQLErrors(e)

	// Dgraph should only return {} or a JSON object.  Also,
	// GQL type checking should ensure query results are only object types
	// https://graphql.github.io/graphql-spec/June2018/#sec-Query
	// So we are only building object results.
	var valToComplete map[string]interface{}
	d := json.NewDecoder(bytes.NewBuffer(dgResult))
	d.UseNumber()
	err := d.Decode(&valToComplete)
	if err != nil {
		glog.Errorf("%+v \n Dgraph result :\n%s\n",
			errors.Wrap(err, "failed to unmarshal Dgraph query result"),
			string(dgResult))
		return nullResponse(
			schema.GQLWrapLocationf(err, field.Location(), "couldn't unmarshal Dgraph result"))
	}

	switch val := valToComplete[field.DgraphAlias()].(type) {
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

				glog.Errorf("Got a list of length %v from Dgraph when expecting a "+
					"one-item list.\n", len(val))

				errs = append(errs,
					x.GqlErrorf(
						"Dgraph returned a list, but %s (type %s) was expecting just one item.  "+
							"The first item in the list was used to produce the result.",
						field.Name(), field.Type().String()).WithLocations(field.Location()))
			}

			valToComplete[field.DgraphAlias()] = internalVal
		}
	case interface{}:
		// no need to error in this case, this can be returned for custom HTTP query/mutation
	default:
		if val != nil {
			return dgraphError()
		}

		// valToComplete[field.Name()] is nil, so resolving for the
		// { } ---> "q": null
		// case
	}

	// TODO: correctly handle DgraphAlias for custom field resolution, at present it uses f.Name(),
	// it should be using f.DgraphAlias() to get values from valToComplete.
	// It works ATM because there hasn't been a scenario where there are two fields with same
	// name in implementing types of an interface with @custom on some field in those types.
	err = resolveCustomFields(ctx, field.SelectionSet(), valToComplete[field.DgraphAlias()])
	if err != nil {
		errs = append(errs, schema.AsGQLErrors(err)...)
	}

	return &Resolved{
		Data:  valToComplete,
		Field: field,
		Err:   errs,
	}
}

func copyTemplate(input interface{}) (interface{}, error) {
	b, err := json.Marshal(input)
	if err != nil {
		return nil, errors.Wrapf(err, "while marshaling map input: %+v", input)
	}

	var result interface{}
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, errors.Wrapf(err, "while unmarshalling into map: %s", b)
	}
	return result, nil
}

func keyNotFoundError(f schema.Field, key string) *x.GqlError {
	return x.GqlErrorf("Evaluation of custom field failed because key: %s "+
		"could not be found in the JSON response returned by external request "+
		"for field: %s within type: %s.", key, f.Name(),
		f.GetObjectName()).WithLocations(f.Location())
}

func jsonMarshalError(err error, f schema.Field, input interface{}) *x.GqlError {
	return x.GqlErrorf("Evaluation of custom field failed because json marshaling "+
		"(of: %+v) returned an error: %s for field: %s within type: %s.", input, err,
		f.Name(), f.GetObjectName()).WithLocations(f.Location())
}

func jsonUnmarshalError(err error, f schema.Field) *x.GqlError {
	return x.GqlErrorf("Evaluation of custom field failed because json unmarshaling"+
		" result of external request failed (with error: %s) for field: %s within "+
		"type: %s.", err, f.Name(), f.GetObjectName()).WithLocations(
		f.Location())
}

func externalRequestError(err error, f schema.Field) *x.GqlError {
	return x.GqlErrorf("Evaluation of custom field failed because external request"+
		" returned an error: %s for field: %s within type: %s.", err, f.Name(),
		f.GetObjectName()).WithLocations(f.Location())
}

func internalServerError(err error, f schema.Field) error {
	return schema.GQLWrapLocationf(err, f.Location(), "evaluation of custom field failed"+
		" for field: %s within type: %s.", f.Name(), f.GetObjectName())
}

type graphqlResp struct {
	Data   map[string]interface{} `json:"data,omitempty"`
	Errors x.GqlErrorList         `json:"errors,omitempty"`
}

func resolveCustomField(ctx context.Context, f schema.Field, vals []interface{}, mu *sync.RWMutex,
	errCh chan error) {
	defer api.PanicHandler(func(err error) {
		errCh <- internalServerError(err, f)
	})

	fconf, err := f.CustomHTTPConfig()
	if err != nil {
		errCh <- err
		return
	}

	// Here we build the input for resolving the fields which is sent as the body for the request.
	inputs := make([]interface{}, len(vals))

	graphql := fconf.RemoteGqlQueryName != ""
	// For GraphQL requests, we substitute arguments in the GraphQL query/mutation to make to
	// the remote endpoint using the values of other fields obtained from Dgraph.
	if graphql {
		requiredArgs := fconf.RequiredArgs
		for i := 0; i < len(inputs); i++ {
			vars := make(map[string]interface{})
			// vals[i] has all the values fetched for this type from Dgraph, lets copy over the
			// values required to process the remote GraphQL for the field into a new map.
			mu.RLock()
			m := vals[i].(map[string]interface{})
			for k, v := range m {
				if _, ok := requiredArgs[k]; ok {
					vars[k] = v
				}
			}
			mu.RUnlock()
			inputs[i] = vars
		}
	} else {
		for i := 0; i < len(inputs); i++ {
			if fconf.Template == nil {
				continue
			}
			temp, err := copyTemplate(*fconf.Template)
			if err != nil {
				errCh <- err
				return
			}

			mu.RLock()
			schema.SubstituteVarsInBody(&temp, vals[i].(map[string]interface{}))
			mu.RUnlock()
			inputs[i] = temp
		}
	}

	if fconf.Mode == schema.BATCH {
		var requestInput interface{}
		requestInput = inputs

		if graphql {
			body := make(map[string]interface{})
			body["query"] = fconf.RemoteGqlQuery
			body["variables"] = map[string]interface{}{fconf.GraphqlBatchModeArgument: requestInput}
			requestInput = body
		} else if f.HasLambdaDirective() {
			requestInput = getBodyForLambda(ctx, f, requestInput, nil)
		}

		b, err := json.Marshal(requestInput)
		if err != nil {
			errCh <- x.GqlErrorList{jsonMarshalError(err, f, inputs)}
			return
		}

		b, status, err := makeRequest(nil, fconf.Method, fconf.URL, string(b), fconf.ForwardHeaders)
		if err != nil {
			errCh <- x.GqlErrorList{externalRequestError(err, f)}
			return
		}

		// To collect errors from remote GraphQL endpoint and those encountered during execution.
		var errs error
		var result []interface{}
		var rerr restErr
		if graphql {
			resp := &graphqlResp{}
			err = json.Unmarshal(b, resp)
			if err != nil {
				errCh <- x.GqlErrorList{jsonUnmarshalError(err, f)}
				return
			}

			if len(resp.Errors) > 0 {
				errs = schema.AppendGQLErrs(errs, resp.Errors)
			}
			var ok bool
			result, ok = resp.Data[fconf.RemoteGqlQueryName].([]interface{})
			if !ok {
				errCh <- schema.AppendGQLErrs(errs, keyNotFoundError(f, fconf.RemoteGqlQueryName))
				return
			}
		} else {
			if status >= 200 && status < 300 {
				if err = json.Unmarshal(b, &result); err != nil {
					errCh <- x.GqlErrorList{jsonUnmarshalError(err, f)}
					return
				}
			} else {
				if err = json.Unmarshal(b, &rerr); err != nil {
					err = errors.Errorf("unexpected error with: %v", status)
					errCh <- x.GqlErrorList{externalRequestError(err, f)}
					return
				} else {
					errCh <- rerr.Errors
					return
				}
			}
		}
		if len(result) != len(vals) {
			gqlErr := x.GqlErrorf("Evaluation of custom field failed because expected result of "+
				"external request to be of size %v, got: %v for field: %s within type: %s.",
				len(vals), len(result), f.Name(), f.GetObjectName()).WithLocations(f.Location())
			errCh <- schema.AppendGQLErrs(errs, gqlErr)
			return
		}

		// Here we walk through all the objects in the array and substitute the value
		// that we got from the remote endpoint with the right key in the object.
		mu.Lock()
		for idx, val := range vals {
			val.(map[string]interface{})[f.Name()] = result[idx]
			vals[idx] = val
		}
		mu.Unlock()
		errCh <- errs
		return
	}

	// This is single mode, make calls concurrently for each input and fill in the results.
	errChan := make(chan error, len(inputs))
	for i := 0; i < len(inputs); i++ {
		go func(idx int, input interface{}) {
			defer api.PanicHandler(
				func(err error) {
					errChan <- internalServerError(err, f)
				})

			requestInput := input
			if graphql {
				body := make(map[string]interface{})
				body["query"] = fconf.RemoteGqlQuery
				body["variables"] = input
				requestInput = body
			}

			b, err := json.Marshal(requestInput)
			if err != nil {
				errChan <- x.GqlErrorList{jsonMarshalError(err, f, requestInput)}
				return
			}

			url := fconf.URL
			if !graphql {
				// For REST requests, we'll have to substitute the variables used in the URL.
				mu.RLock()
				url, err = schema.SubstituteVarsInURL(url,
					vals[idx].(map[string]interface{}))
				if err != nil {
					mu.RUnlock()
					gqlErr := x.GqlErrorf("Evaluation of custom field failed while substituting "+
						"variables into URL for remote endpoint with an error: %s for field: %s "+
						"within type: %s.", err, f.Name(),
						f.GetObjectName()).WithLocations(f.Location())
					errChan <- x.GqlErrorList{gqlErr}
					return
				}
				mu.RUnlock()
			}

			b, status, err := makeRequest(nil, fconf.Method, url, string(b), fconf.ForwardHeaders)
			if err != nil {
				errChan <- x.GqlErrorList{externalRequestError(err, f)}
				return
			}

			var result interface{}
			var rerr restErr
			var errs error
			if graphql {
				resp := &graphqlResp{}
				if err = json.Unmarshal(b, resp); err != nil {
					errChan <- x.GqlErrorList{jsonUnmarshalError(err, f)}
					return
				}

				if len(resp.Errors) > 0 {
					errs = schema.AppendGQLErrs(errs, resp.Errors)
				}
				var ok bool
				result, ok = resp.Data[fconf.RemoteGqlQueryName]
				if !ok {
					errChan <- schema.AppendGQLErrs(errs,
						keyNotFoundError(f, fconf.RemoteGqlQueryName))
					return
				}
			} else {
				if status >= 200 && status < 300 {
					if err = json.Unmarshal(b, &result); err != nil {
						errCh <- x.GqlErrorList{jsonUnmarshalError(err, f)}
						return
					}
				} else {
					if err = json.Unmarshal(b, &rerr); err != nil {
						err = errors.Errorf("unexpected error with: %v", status)
						errCh <- x.GqlErrorList{externalRequestError(err, f)}
						return
					} else {
						errCh <- rerr.Errors
						return
					}
				}
			}
			mu.Lock()
			val, ok := vals[idx].(map[string]interface{})
			if ok {
				val[f.Name()] = result
			}
			mu.Unlock()
			errChan <- errs
		}(i, inputs[i])
	}

	var errs error
	// Some of the errors can be null, so lets collect the non-null errors here.
	for i := 0; i < len(inputs); i++ {
		e := <-errChan
		if e != nil {
			errs = schema.AppendGQLErrs(errs, e)
		}
	}

	errCh <- errs
}

// resolveNestedFields resolves fields which themselves don't have the @custom directive but their
// children might
//
// queryUser {
//	 id
//	 classes {
//	   name @custom...
//   }
// }
// In the example above, resolveNestedFields would be called on classes field and vals would be the
// list of all users.
func resolveNestedFields(ctx context.Context, f schema.Field, vals []interface{}, mu *sync.RWMutex,
	errCh chan error) {
	defer api.PanicHandler(func(err error) {
		errCh <- internalServerError(err, f)
	})

	// If this field doesn't have custom directive and also doesn't have any children,
	// then there is nothing to do and we can just continue.
	if len(f.SelectionSet()) == 0 {
		errCh <- nil
		return
	}

	// Here below we do the de-duplication by walking through the result set. That is we would
	// go over vals and find the data for f to collect all unique values.
	var input []interface{}
	// node stores the pointer for a node. It is a map from id to the map for it.
	nodes := make(map[string]interface{})

	idField := f.Type().IDField()
	if idField == nil {
		idField = f.Type().XIDField()
		if idField == nil {
			// This should not happen as we only allow custom fields on types which either have
			// ID or a field with @id directive.
			errCh <- nil
			return
		}
	}

	idFieldName := idField.Name()
	castInterfaceToSlice := func(tmpVals interface{}) []interface{} {
		var fieldVals []interface{}
		switch tv := tmpVals.(type) {
		case []interface{}:
			fieldVals = tv
		case interface{}:
			fieldVals = []interface{}{tv}
		}
		return fieldVals
	}
	// Here we walk through the array and collect all unique values for this field. In the
	// example at the start of the function, we could be collecting all unique classes
	// across all users. This is where the batching happens so that we make one call per
	// field and not a separate call per user.
	mu.RLock()
	for _, v := range vals {
		val, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		tmpVals, ok := val[f.Name()]
		if !ok {
			continue
		}
		fieldVals := castInterfaceToSlice(tmpVals)
		for _, fieldVal := range fieldVals {
			fv, ok := fieldVal.(map[string]interface{})
			if !ok {
				continue
			}
			id, ok := fv[idFieldName].(string)
			if !ok {
				// If a type has a field of type ID! and it is not explicitly requested by the
				// user as part of the query, we would still have asked for it under the alias
				// dgraph.uid, so let's look for that here.
				id, ok = fv["dgraph.uid"].(string)
				if !ok {
					continue
				}
			}
			if _, ok := nodes[id]; !ok {
				input = append(input, fieldVal)
				nodes[id] = fieldVal
			}
		}
	}
	mu.RUnlock()

	if err := resolveCustomFields(ctx, f.SelectionSet(), input); err != nil {
		errCh <- err
		return
	}

	mu.Lock()
	for _, v := range vals {
		val, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		tmpVals, ok := val[f.Name()]
		if !ok {
			continue
		}
		fieldVals := castInterfaceToSlice(tmpVals)
		for idx, fieldVal := range fieldVals {
			fv, ok := fieldVal.(map[string]interface{})
			if !ok {
				continue
			}
			id, ok := fv[idFieldName].(string)
			if !ok {
				id, ok = fv["dgraph.uid"].(string)
				if !ok {
					continue
				}
			}
			// Get the pointer of the map corresponding to this id and put it at the
			// correct place.
			mval := nodes[id]
			fieldVals[idx] = mval
		}
	}
	mu.Unlock()
	errCh <- nil
}

// resolveCustomFields resolves fields with custom directive. Here is the rough algorithm that it
// follows.
// queryUser {
//	name @custom
//	age
//	school {
//		name
//		children
//		class { @custom
//			name
//			numChildren
//		}
//	}
//	cars { @custom
//		name
//	}
// }
// For fields with @custom directive
// 1. There would be one query sent to the remote endpoint.
// 2. In the above example, to fetch class all the school ids would be aggregated across different
// users deduplicated and then one query sent. The results would then be filled back appropriately.
//
// For fields without custom directive we recursively call resolveCustomFields and let it do the
// work.
// TODO - We can be smarter about this and know before processing the query if we should be making
// this recursive call upfront.
func resolveCustomFields(ctx context.Context, fields []schema.Field, data interface{}) error {
	if data == nil {
		return nil
	}

	var vals []interface{}
	switch v := data.(type) {
	case []interface{}:
		vals = v
	case interface{}:
		vals = []interface{}{v}
	}

	if len(vals) == 0 {
		return nil
	}

	// This mutex protects access to vals as it is concurrently read and written to by multiple
	// goroutines.
	mu := &sync.RWMutex{}
	errCh := make(chan error, len(fields))
	numRoutines := 0

	for _, f := range fields {
		if f.Skip() || !f.Include() {
			continue
		}

		numRoutines++
		hasCustomDirective, _ := f.HasCustomDirective()
		if !hasCustomDirective {
			go resolveNestedFields(ctx, f, vals, mu, errCh)
		} else {
			go resolveCustomField(ctx, f, vals, mu, errCh)
		}
	}

	var errs error
	for i := 0; i < numRoutines; i++ {
		if err := <-errCh; err != nil {
			errs = schema.AppendGQLErrs(errs, err)
		}
	}

	return errs
}

// completeGeoObject builds a json GraphQL result object for the geo type.
// It returns a bracketed json object like { "longitude" : 12.32 , "latitude" : 123.32 }.
func completeGeoObject(field schema.Field, val map[string]interface{}, path []interface{}) ([]byte, x.GqlErrorList) {
	var buf bytes.Buffer
	x.Check2(buf.WriteRune('{'))

	coordinate, _ := val["coordinates"].([]interface{})
	if coordinate == nil {
		gqlErr := x.GqlErrorf(errExpectedNonNull, field.Name(), field.Type()).WithLocations(field.Location())
		gqlErr.Path = copyPath(path)
		return nil, x.GqlErrorList{gqlErr}
	}

	fields := field.SelectionSet()
	comma := ""

	longitude := fmt.Sprintf("%s", coordinate[0])
	latitude := fmt.Sprintf("%s", coordinate[1])
	for _, field := range fields {
		x.Check2(buf.WriteString(comma))
		x.Check2(buf.WriteRune('"'))
		x.Check2(buf.WriteString(field.ResponseName()))
		x.Check2(buf.WriteString(`": `))

		switch field.ResponseName() {
		case "latitude":
			x.Check2(buf.WriteString(latitude))
		case "longitude":
			x.Check2(buf.WriteString(longitude))
		}

		comma = ","
	}

	x.Check2(buf.WriteRune('}'))
	return buf.Bytes(), nil
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
func completeObject(
	path []interface{},
	fields []schema.Field,
	res map[string]interface{}) ([]byte, x.GqlErrorList) {

	var errs x.GqlErrorList
	var buf bytes.Buffer
	comma := ""

	// Below map keeps track of fields which have been seen as part of
	// interface to avoid double entry in the resulting response
	seenField := make(map[string]bool)

	x.Check2(buf.WriteRune('{'))
	dgraphTypes, ok := res["dgraph.type"].([]interface{})
	for _, f := range fields {
		if f.Skip() || !f.Include() {
			continue
		}

		includeField := true
		// If typ is an interface, and dgraphTypes contains another type, then we ignore
		// fields which don't start with that type. This would happen when multiple
		// fragments (belonging to different types) are requested within a query for an interface.

		// If the dgraphPredicate doesn't start with the typ.Name(), then this field belongs to
		// a concrete type, lets check that it has inputType as the prefix, otherwise skip it.
		if len(dgraphTypes) > 0 {
			includeField = f.IncludeInterfaceField(dgraphTypes)
		}
		if _, ok := seenField[f.ResponseName()]; ok {
			includeField = false
		}
		if !includeField {
			continue
		}

		x.Check2(buf.WriteString(comma))
		x.Check2(buf.WriteRune('"'))
		x.Check2(buf.WriteString(f.ResponseName()))
		x.Check2(buf.WriteString(`": `))

		seenField[f.ResponseName()] = true

		val := res[f.DgraphAlias()]
		if f.Name() == schema.Typename {
			// From GraphQL spec:
			// https://graphql.github.io/graphql-spec/June2018/#sec-Type-Name-Introspection
			// "GraphQL supports type name introspection at any point within a query by the
			// meta‐field  __typename: String! when querying against any Object, Interface,
			// or Union. It returns the name of the object type currently being queried."

			// If we have dgraph.type information, we will use that to figure out the type
			// otherwise we will get it from the schema.
			if ok {
				val = f.TypeName(dgraphTypes)
			} else {
				val = f.GetObjectName()
			}
		}

		// Check that we should check that data should be of list type when we expect
		// f.Type().ListType() to be non-nil.
		if val != nil && f.Type().ListType() != nil {
			switch val.(type) {
			case []interface{}, []map[string]interface{}:
			default:
				// We were expecting a list but got a value which wasn't a list. Lets return an
				// error.
				return nil, x.GqlErrorList{&x.GqlError{
					Message:   errExpectedList,
					Locations: []x.Location{f.Location()},
					Path:      copyPath(path),
				}}
			}
		}
		completed, err := completeValue(append(path, f.ResponseName()), f, val)
		errs = append(errs, err...)
		if completed == nil {
			if !f.Type().Nullable() {
				return nil, errs
			}
			completed = []byte(`null`)
		}
		x.Check2(buf.Write(completed))
		comma = ", "
	}
	x.Check2(buf.WriteRune('}'))

	return buf.Bytes(), errs
}

// completeValue applies the value completion algorithm to a single value, which
// could turn out to be a list or object or scalar value.
func completeValue(
	path []interface{},
	field schema.Field,
	val interface{}) ([]byte, x.GqlErrorList) {

	switch val := val.(type) {
	case map[string]interface{}:
		switch field.Type().Name() {
		case "String", "ID", "Boolean", "Float", "Int", "Int64", "DateTime":
			return nil, x.GqlErrorList{&x.GqlError{
				Message:   errExpectedScalar,
				Locations: []x.Location{field.Location()},
				Path:      copyPath(path),
			}}
		}
		enumValues := field.EnumValues()
		if len(enumValues) > 0 {
			return nil, x.GqlErrorList{&x.GqlError{
				Message:   errExpectedScalar,
				Locations: []x.Location{field.Location()},
				Path:      copyPath(path),
			}}
		}
		if field.Type().IsPoint() {
			return completeGeoObject(field, val, path)
		}

		return completeObject(path, field.SelectionSet(), val)
	case []interface{}:
		return completeList(path, field, val)
	case []map[string]interface{}:
		// This case is different from the []interface{} case above and is true for admin queries
		// where we built the val ourselves.
		listVal := make([]interface{}, 0, len(val))
		for _, v := range val {
			listVal = append(listVal, v)
		}
		return completeList(path, field, listVal)
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

			gqlErr := x.GqlErrorf(errExpectedNonNull, field.Name(), field.Type()).WithLocations(field.Location())
			gqlErr.Path = copyPath(path)
			return nil, x.GqlErrorList{gqlErr}
		}

		// val is a scalar
		val, gqlErr := coerceScalar(val, field, path)
		if len(gqlErr) != 0 {
			return nil, gqlErr
		}

		// Can this ever error?  We can't have an unsupported type or value because
		// we just unmarshaled this val.
		b, err := json.Marshal(val)
		if err != nil {
			gqlErr := x.GqlErrorf(
				"Error marshalling value for field '%s' (type %s).  "+
					"Resolved as null (which may trigger GraphQL error propagation) ",
				field.Name(), field.Type()).
				WithLocations(field.Location())
			gqlErr.Path = copyPath(path)

			if field.Type().Nullable() {
				return []byte("null"), x.GqlErrorList{gqlErr}
			}

			return nil, x.GqlErrorList{gqlErr}
		}

		return b, nil
	}
}

// coerceScalar coerces a scalar value to field.Type() if possible according to the coercion rules
// defined in the GraphQL spec. If this is not possible, then it returns an error.
func coerceScalar(val interface{}, field schema.Field, path []interface{}) (interface{},
	x.GqlErrorList) {

	valueCoercionError := func(val interface{}) x.GqlErrorList {
		gqlErr := x.GqlErrorf(
			"Error coercing value '%+v' for field '%s' to type %s.",
			val, field.Name(), field.Type().Name()).
			WithLocations(field.Location())
		gqlErr.Path = copyPath(path)
		return x.GqlErrorList{gqlErr}
	}

	switch field.Type().Name() {
	case "String", "ID":
		switch v := val.(type) {
		case float64:
			val = strconv.FormatFloat(v, 'f', -1, 64)
		case int64:
			val = strconv.FormatInt(v, 10)
		case bool:
			val = strconv.FormatBool(v)
		case string:
		case json.Number:
			val = v.String()
		default:
			return nil, valueCoercionError(v)
		}
	case "Boolean":
		switch v := val.(type) {
		case string:
			val = len(v) > 0
		case bool:
		case json.Number:
			valFloat, _ := v.Float64()
			val = valFloat != 0
		default:
			return nil, valueCoercionError(v)
		}
	case "Int":
		switch v := val.(type) {
		case float64:
			// The spec says that we can coerce a Float value to Int, if we don't lose information.
			// See: https: //spec.graphql.org/June2018/#sec-Float
			// Lets try to see if this number could be converted to int32 without losing
			// information, otherwise return error.
			// See: https://github.com/golang/go/issues/19405 to understand why the comparison
			// should be done after double conversion.
			i32Val := int32(v)
			if v == float64(i32Val) {
				val = i32Val
			} else {
				return nil, valueCoercionError(v)
			}
		case bool:
			if v {
				val = 1
			} else {
				val = 0
			}
		case string:
			i, err := strconv.ParseFloat(v, 64)
			// An error can be encountered if we had a value that can't be fit into
			// a 64 bit floating point number..
			// Lets try to see if this number could be converted to int32 without losing
			// information, otherwise return error.
			if err != nil {
				return nil, valueCoercionError(v)
			}
			i32Val := int32(i)
			if i == float64(i32Val) {
				val = i32Val
			} else {
				return nil, valueCoercionError(v)
			}
		case int64:
			if v > math.MaxInt32 || v < math.MinInt32 {
				return nil, valueCoercionError(v)
			}
		case int:
			// numUids are added as int, so we need special handling for that.
			if v > math.MaxInt32 || v < math.MinInt32 {
				return nil, valueCoercionError(v)
			}
		case json.Number:
			// We have already checked range for int32 at input validation time.
			// So now just parse and check errors.
			i, err := strconv.ParseFloat(v.String(), 64)
			if err != nil {
				return nil, valueCoercionError(v)
			}
			i32Val := int32(i)
			if i == float64(i32Val) {
				val = i32Val
			} else {
				return nil, valueCoercionError(v)
			}
		default:
			return nil, valueCoercionError(v)
		}
	case "Int64":
		switch v := val.(type) {
		case bool:
			if v {
				val = 1
			} else {
				val = 0
			}
		case string:
			i, err := strconv.ParseInt(v, 10, 64)
			// An error can be encountered if we had a value that can't be fit into
			// a 64 bit int or because of other parsing issues.
			if err != nil {
				return nil, valueCoercionError(v)
			}
			val = i
		case json.Number:
			// To use whole 64-bit range for int64 without any coercing,
			// We pass int64 values as string to dgraph and parse it as integer here
			i, err := strconv.ParseInt(v.String(), 10, 64)
			if err != nil {
				return nil, valueCoercionError(v)
			}
			val = i
		default:
			return nil, valueCoercionError(v)
		}
	case "Float":
		switch v := val.(type) {
		case bool:
			if v {
				val = 1.0
			} else {
				val = 0.0
			}
		case string:
			i, err := strconv.ParseFloat(v, 64)
			// An error can be encountered if we had a value that can't be fit into
			// a 64 bit floating point number or because of other parsing issues.
			if err != nil {
				return nil, valueCoercionError(v)
			}
			val = i
		case json.Number:
			i, err := strconv.ParseFloat(v.String(), 64)
			if err != nil {
				return nil, valueCoercionError(v)
			}
			val = i
		case float64:
		default:
			return nil, valueCoercionError(v)
		}
	case "DateTime":
		switch v := val.(type) {
		case string:
			if _, err := types.ParseTime(v); err != nil {
				return nil, valueCoercionError(v)
			}
		case json.Number:
			valFloat, _ := v.Float64()
			truncated := math.Trunc(valFloat)
			if truncated == valFloat {
				// Lets interpret int values as unix timestamp.
				t := time.Unix(int64(truncated), 0).UTC()
				val = t.Format(time.RFC3339)
			} else {
				return nil, valueCoercionError(v)
			}
		default:
			return nil, valueCoercionError(v)
		}
	default:
		enumValues := field.EnumValues()
		// At this point we should only get fields which are of ENUM type, so we can return
		// an error if we don't get any enum values.
		if len(enumValues) == 0 {
			return nil, valueCoercionError(val)
		}
		switch v := val.(type) {
		case string:
			// Lets check that the enum value is valid.
			valid := false
			for _, ev := range enumValues {
				if ev == v {
					valid = true
					break
				}
			}
			if !valid {
				return nil, valueCoercionError(val)
			}
		default:
			return nil, valueCoercionError(v)
		}
	}
	return val, nil
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
// [ completeValue("..."), completeValue("..."), ... ]
// or
// [ completeObject({...}), completeObject({...}), ... ]
// depending on the type of list.
//
// If the list has non-nullable elements (a type like [T!]) and any of those
// elements resolve to null, then the whole list is crushed to null.
func completeList(
	path []interface{},
	field schema.Field,
	values []interface{}) ([]byte, x.GqlErrorList) {

	var buf bytes.Buffer
	var errs x.GqlErrorList
	comma := ""

	if field.Type().ListType() == nil {
		// This means a bug on our part - in rewriting, schema generation,
		// or Dgraph returned something unexpected.
		//
		// Let's crush it to null so we still get something from the rest of the
		// query and log the error.
		return mismatched(path, field, values)
	}

	x.Check2(buf.WriteRune('['))
	for i, b := range values {
		r, err := completeValue(append(path, i), field, b)
		errs = append(errs, err...)
		x.Check2(buf.WriteString(comma))
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
			x.Check2(buf.WriteString("null"))
		} else {
			x.Check2(buf.Write(r))
		}
		comma = ", "
	}
	x.Check2(buf.WriteRune(']'))

	return buf.Bytes(), errs
}

func mismatched(
	path []interface{},
	field schema.Field,
	values []interface{}) ([]byte, x.GqlErrorList) {

	glog.Errorf("completeList() called in resolving %s (Line: %v, Column: %v), "+
		"but its type is %s.\n"+
		"That could indicate the Dgraph schema doesn't match the GraphQL schema.",
		field.Name(), field.Location().Line, field.Location().Column, field.Type().Name())

	gqlErr := &x.GqlError{
		Message:   errExpectedObject,
		Locations: []x.Location{field.Location()},
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

// a httpResolver can resolve a single GraphQL field from an HTTP endpoint
type httpResolver struct {
	*http.Client
	resultCompleter ResultCompleter
}

type httpQueryResolver httpResolver
type httpMutationResolver httpResolver

// NewHTTPQueryResolver creates a resolver that can resolve GraphQL query from an HTTP endpoint
func NewHTTPQueryResolver(hc *http.Client, rc ResultCompleter) QueryResolver {
	return &httpQueryResolver{hc, rc}
}

// NewHTTPMutationResolver creates a resolver that resolves GraphQL mutation from an HTTP endpoint
func NewHTTPMutationResolver(hc *http.Client, rc ResultCompleter) MutationResolver {
	return &httpMutationResolver{hc, rc}
}

func (hr *httpResolver) Resolve(ctx context.Context, field schema.Field) *Resolved {
	span := otrace.FromContext(ctx)
	stop := x.SpanTimer(span, "resolveHTTP")
	defer stop()

	resolved := hr.rewriteAndExecute(ctx, field)
	hr.resultCompleter.Complete(ctx, resolved)
	return resolved
}

func makeRequest(client *http.Client, method, url, body string,
	header http.Header) ([]byte, int, error) {
	var reqBody io.Reader
	if body == "" || body == "null" {
		reqBody = http.NoBody
	} else {
		reqBody = bytes.NewBufferString(body)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header = header

	// TODO - Needs to be fixed, we shouldn't be initiating a new HTTP client everytime.
	if client == nil {
		client = &http.Client{
			Timeout: time.Minute,
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}

	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	return b, resp.StatusCode, err
}

func getBodyForLambda(ctx context.Context, field schema.Field, parents,
	args interface{}) map[string]interface{} {
	body := make(map[string]interface{})
	body["resolver"] = field.GetObjectName() + "." + field.Name()
	body["authHeader"] = map[string]interface{}{
		"key":   authorization.GetHeader(),
		"value": authorization.GetJwtToken(ctx),
	}
	if parents != nil {
		body["parents"] = parents
	}
	if args != nil {
		body["args"] = args
	}
	return body
}

func (hr *httpResolver) rewriteAndExecute(ctx context.Context, field schema.Field) *Resolved {
	emptyResult := func(err error) *Resolved {
		return &Resolved{
			Data:  map[string]interface{}{field.Name(): nil},
			Field: field,
			Err:   schema.AsGQLErrors(err),
		}
	}

	hrc, err := field.CustomHTTPConfig()
	if err != nil {
		return emptyResult(err)
	}

	var body string
	if hrc.Template != nil {
		jsonTemplate := *hrc.Template
		if field.HasLambdaDirective() {
			jsonTemplate = getBodyForLambda(ctx, field, nil, *hrc.Template)
		}
		b, err := json.Marshal(jsonTemplate)
		if err != nil {
			return emptyResult(jsonMarshalError(err, field, *hrc.Template))
		}
		body = string(b)
	}

	b, status, err := makeRequest(hr.Client, hrc.Method, hrc.URL, body, hrc.ForwardHeaders)
	if err != nil {
		return emptyResult(externalRequestError(err, field))
	}

	// this means it had body and not graphql, so just unmarshal it and return
	if hrc.RemoteGqlQueryName == "" {
		var result interface{}
		var rerr restErr
		if status >= 200 && status < 300 {
			if err := json.Unmarshal(b, &result); err != nil {
				return emptyResult(jsonUnmarshalError(err, field))
			}
		} else if err := json.Unmarshal(b, &rerr); err != nil {
			err = errors.Errorf("unexpected error with: %v", status)
			rerr.Errors = x.GqlErrorList{externalRequestError(err, field)}
		}

		return &Resolved{
			Data:  map[string]interface{}{field.Name(): result},
			Field: field,
			Err:   rerr.Errors,
		}
	}

	// we will reach here if it was a graphql request
	var resp struct {
		Data   map[string]interface{} `json:"data,omitempty"`
		Errors x.GqlErrorList         `json:"errors,omitempty"`
	}
	err = json.Unmarshal(b, &resp)
	if err != nil {
		gqlErr := jsonUnmarshalError(err, field)
		resp.Errors = append(resp.Errors, schema.AsGQLErrors(gqlErr)...)
		return emptyResult(resp.Errors)
	}
	data, ok := resp.Data[hrc.RemoteGqlQueryName]
	if !ok {
		return emptyResult(resp.Errors)
	}
	return &Resolved{
		Data:  map[string]interface{}{field.Name(): data},
		Field: field,
		Err:   resp.Errors,
	}
}

func (h *httpQueryResolver) Resolve(ctx context.Context, query schema.Query) *Resolved {
	return (*httpResolver)(h).Resolve(ctx, query)
}

func (h *httpMutationResolver) Resolve(ctx context.Context, mutation schema.Mutation) (*Resolved,
	bool) {
	resolved := (*httpResolver)(h).Resolve(ctx, mutation)
	return resolved, resolved.Err == nil || resolved.Err.Error() == ""
}

func EmptyResult(f schema.Field, err error) *Resolved {
	return &Resolved{
		Data:  map[string]interface{}{f.Name(): nil},
		Field: f,
		Err:   schema.GQLWrapLocationf(err, f.Location(), "resolving %s failed", f.Name()),
	}
}

func newtimer(ctx context.Context, Duration *schema.OffsetDuration) schema.OffsetTimer {
	resolveStartTime, _ := ctx.Value(resolveStartTime).(time.Time)
	tf := schema.NewOffsetTimerFactory(resolveStartTime)
	return tf.NewOffsetTimer(Duration)
}
