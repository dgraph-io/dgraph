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

	errInternal = "Internal error"
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
	sync.RWMutex
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
	Data       []byte
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

func (aex *adminExecutor) Execute(ctx context.Context, req *dgoapi.Request, field schema.Field) (
	*dgoapi.Response, error) {
	ctx = context.WithValue(ctx, edgraph.Authorize, false)
	return aex.dg.Execute(ctx, req, field)
}

func (aex *adminExecutor) CommitOrAbort(ctx context.Context, tc *dgoapi.TxnContext) error {
	return aex.dg.CommitOrAbort(ctx, tc)
}

func (de *dgraphExecutor) Execute(ctx context.Context, req *dgoapi.Request, field schema.Field) (
	*dgoapi.Response, error) {
	return de.dg.Execute(ctx, req, field)
}

func (de *dgraphExecutor) CommitOrAbort(ctx context.Context, tc *dgoapi.TxnContext) error {
	return de.dg.CommitOrAbort(ctx, tc)
}

func (rf *resolverFactory) WithQueryResolver(
	name string, resolver func(schema.Query) QueryResolver) ResolverFactory {
	rf.Lock()
	defer rf.Unlock()
	rf.queryResolvers[name] = resolver
	return rf
}

func (rf *resolverFactory) WithMutationResolver(
	name string, resolver func(schema.Mutation) MutationResolver) ResolverFactory {
	rf.Lock()
	defer rf.Unlock()
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
			}).
		WithMutationResolver("__typename",
			func(m schema.Mutation) MutationResolver {
				return MutationResolverFunc(func(ctx context.Context, m schema.Mutation) (*Resolved, bool) {
					return DataResult(m, map[string]interface{}{"__typename": "Mutation"}, nil),
						resolverSucceeded
				})
			})
}

func (rf *resolverFactory) WithConventionResolvers(
	s schema.Schema, fns *ResolverFns) ResolverFactory {

	queries := append(s.Queries(schema.GetQuery), s.Queries(schema.FilterQuery)...)
	queries = append(queries, s.Queries(schema.PasswordQuery)...)
	queries = append(queries, s.Queries(schema.AggregateQuery)...)
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
			return NewDgraphResolver(fns.Drw, fns.Ex, StdDeleteCompletion(m.Name()))
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
	return noopCompletion
}

func (rf *resolverFactory) queryResolverFor(query schema.Query) QueryResolver {
	rf.RLock()
	defer rf.RUnlock()
	mws := rf.queryMiddlewareConfig[query.Name()]
	if resolver, ok := rf.queryResolvers[query.Name()]; ok {
		return mws.Then(resolver(query))
	}
	return rf.queryError
}

func (rf *resolverFactory) mutationResolverFor(mutation schema.Mutation) MutationResolver {
	rf.RLock()
	defer rf.RUnlock()
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
		if op.CacheControl() != "" {
			resp.Header = make(map[string][]string)
			resp.Header.Set(schema.CacheControlHeader, op.CacheControl())
			resp.Header.Set("Vary", "Accept-Encoding")
		}
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
					WithLocations(m.Location()).
					WithPath([]interface{}{m.ResponseName()}))

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
	// a path to the 3rd item in the f list would look like:
	// - [ "q", "f", 2, "g" ]
	if res.Data == nil && !res.Field.Type().Nullable() {
		// According to GraphQL spec, out of all the queries in the request, if any one query
		// returns null but expected return type is non-nullable then we set root data to null.
		resp.SetDataNull()
	} else {
		resp.AddData(res.Data)
	}

	resp.WithError(res.Err)
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

	alias := field.DgraphAlias()
	switch val := valToComplete[alias].(type) {
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
				// This case may occur during handling of aggregate Queries. So, we don't throw an error
				// and combine all items into one single item.

				if strings.HasSuffix(field.Type().String(), "AggregateResult") {
					for i := 1; i < len(val); i++ {
						var internalValMap interface{}
						var ok bool
						if internalValMap, ok = val[i].(map[string]interface{}); !ok {
							return dgraphError()
						}
						for key, val := range internalValMap.(map[string]interface{}) {
							internalVal.(map[string]interface{})[key] = val
						}
					}
				} else {

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
		// TODO (cleanup): remove this function altogether, not needed anymore
		Data:  nil, // valToComplete
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

func completeAlias(field schema.Field, buf *bytes.Buffer) {
	x.Check2(buf.WriteRune('"'))
	x.Check2(buf.WriteString(field.ResponseName()))
	x.Check2(buf.WriteString(`":`))
}

// completeObject builds a json GraphQL result object for the current query level.
// It returns a bracketed json object like { f1:..., f2:..., ... }.
// At present, it is only used for building custom results by:
//  * Admin Server
//  * @custom(http: {...}) query/mutation
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

	// seenField keeps track of fields which have been seen as part of
	// interface to avoid double entry in the resulting response
	seenField := make(map[string]bool)

	x.Check2(buf.WriteRune('{'))
	// @custom(http: {...}) query/mutation results may return __typename in response for abstract
	// fields, lets use that information if present.
	typename, ok := res[schema.Typename].(string)
	var dgraphTypes []string
	if ok && len(typename) > 0 {
		dgraphTypes = []string{typename}
	}

	for _, f := range fields {
		if f.SkipField(dgraphTypes, seenField) {
			continue
		}

		x.Check2(buf.WriteString(comma))
		completeAlias(f, &buf)

		val := res[f.Name()]
		if f.Name() == schema.Typename {
			// From GraphQL spec:
			// https://graphql.github.io/graphql-spec/June2018/#sec-Type-Name-Introspection
			// "GraphQL supports type name introspection at any point within a query by the
			// meta‐field  __typename: String! when querying against any Object, Interface,
			// or Union. It returns the name of the object type currently being queried."

			// If we have __typename information, we will use that to figure out the type
			// otherwise we will get it from the schema.
			val = f.TypeName(dgraphTypes)
		}

		// Check that data should be of list type when we expect f.Type().ListType() to be non-nil.
		if val != nil && f.Type().ListType() != nil {
			switch val.(type) {
			case []interface{}, []map[string]interface{}:
			default:
				// We were expecting a list but got a value which wasn't a list. Lets return an
				// error.
				return nil, x.GqlErrorList{f.GqlErrorf(path, schema.ErrExpectedList)}
			}
		}

		completed, err := completeValue(append(path, f.ResponseName()), f, val)
		errs = append(errs, err...)
		if completed == nil {
			if !f.Type().Nullable() {
				return nil, errs
			}
			completed = schema.JsonNull
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
			return nil, x.GqlErrorList{field.GqlErrorf(path, schema.ErrExpectedScalar)}
		}
		enumValues := field.EnumValues()
		if len(enumValues) > 0 {
			return nil, x.GqlErrorList{field.GqlErrorf(path, schema.ErrExpectedScalar)}
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
			if b := field.NullValue(); b != nil {
				return b, nil
			}

			return nil, x.GqlErrorList{field.GqlErrorf(path, schema.ErrExpectedNonNull,
				field.Name(), field.Type())}
		}

		// val is a scalar
		val, gqlErr := coerceScalar(val, field, path)
		if len(gqlErr) > 0 {
			return nil, gqlErr
		}

		// Can this ever error?  We can't have an unsupported type or value because
		// we just unmarshalled this val.
		b, err := json.Marshal(val)
		if err != nil {
			gqlErr := x.GqlErrorList{field.GqlErrorf(path,
				"Error marshalling value for field '%s' (type %s).  "+
					"Resolved as null (which may trigger GraphQL error propagation) ",
				field.Name(), field.Type())}

			if field.Type().Nullable() {
				return schema.JsonNull, gqlErr
			}

			return nil, gqlErr
		}

		return b, nil
	}
}

// coerceScalar coerces a scalar value to field.Type() if possible according to the coercion rules
// defined in the GraphQL spec. If this is not possible, then it returns an error. The crux of
// coercion rules defined in the spec is to not lose information during coercion.
// Note that, admin server specifically uses these:
//  * json.Number (Query.config.cacheMb)
//  * resolve.Unmarshal() everywhere else
// And, @custom(http: {...}) query/mutation would always use resolve.Unmarshal().
// Now, resolve.Unmarshal() can only give one of the following types for scalars:
//  * bool
//  * string
//  * json.Number (because it uses custom JSON decoder which preserves number precision)
// So, we need to consider only these cases at present.
func coerceScalar(val interface{}, field schema.Field, path []interface{}) (interface{},
	x.GqlErrorList) {

	valueCoercionError := func(val interface{}) x.GqlErrorList {
		return x.GqlErrorList{field.GqlErrorf(path,
			"Error coercing value '%+v' for field '%s' to type %s.",
			val, field.Name(), field.Type().Name())}
	}

	switch field.Type().Name() {
	case "String", "ID":
		switch v := val.(type) {
		case bool:
			val = strconv.FormatBool(v)
		case string:
			// do nothing, val is already string
		case json.Number:
			val = v.String()
		default:
			return nil, valueCoercionError(v)
		}
	case "Boolean":
		switch v := val.(type) {
		case bool:
			// do nothing, val is already bool
		case string:
			val = len(v) > 0
		case json.Number:
			valFloat, _ := v.Float64()
			val = valFloat != 0
		default:
			return nil, valueCoercionError(v)
		}
	case "Int":
		switch v := val.(type) {
		case bool:
			if v {
				val = 1
			} else {
				val = 0
			}
		case string:
			floatVal, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, valueCoercionError(v)
			}
			i32Val := int32(floatVal)
			if floatVal == float64(i32Val) {
				val = i32Val
			} else {
				return nil, valueCoercionError(v)
			}
		case json.Number:
			// float64 can always contain 32 bit integers without any information loss
			floatVal, err := v.Float64()
			if err != nil {
				return nil, valueCoercionError(v)
			}
			i32Val := int32(floatVal) // convert the float64 value to int32
			// now if converting the int32 back to float64 results in a mismatch means we lost
			// information during conversion, so return error.
			if floatVal != float64(i32Val) {
				return nil, valueCoercionError(v)
			}
			// otherwise, do nothing as val is already a valid number in int32 range
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
			if err != nil {
				return nil, valueCoercionError(v)
			}
			val = i
		case json.Number:
			if _, err := v.Int64(); err != nil {
				return nil, valueCoercionError(v)
			}
			// do nothing, as val is already a valid number in int64 range
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
			if err != nil {
				return nil, valueCoercionError(v)
			}
			val = i
		case json.Number:
			_, err := v.Float64()
			if err != nil {
				return nil, valueCoercionError(v)
			}
			// do nothing, as val is already a valid number in float
		default:
			return nil, valueCoercionError(v)
		}
	case "DateTime":
		switch v := val.(type) {
		case string:
			if t, err := types.ParseTime(v); err != nil {
				return nil, valueCoercionError(v)
			} else {
				// let's make sure that we always return a string in RFC3339 format as the original
				// string could have been in some other format.
				val = t.Format(time.RFC3339)
			}
		case json.Number:
			valFloat, err := v.Float64()
			if err != nil {
				return nil, valueCoercionError(v)
			}
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
			if !x.HasString(enumValues, v) {
				return nil, valueCoercionError(val)
			}
			// do nothing, as val already has a valid value
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

	if field.Type().ListType() == nil {
		// This means either a bug on our part - in admin server.
		// or @custom query/mutation returned something unexpected.
		//
		// Let's crush it to null so we still get something from the rest of the
		// query and log the error.
		return mismatched(path, field)
	}

	var buf bytes.Buffer
	var errs x.GqlErrorList
	comma := ""

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
			x.Check2(buf.Write(schema.JsonNull))
		} else {
			x.Check2(buf.Write(r))
		}
		comma = ", "
	}
	x.Check2(buf.WriteRune(']'))

	return buf.Bytes(), errs
}

func mismatched(path []interface{}, field schema.Field) ([]byte, x.GqlErrorList) {
	glog.Errorf("completeList() called in resolving %s (Line: %v, Column: %v), "+
		"but its type is %s.\n"+
		"That could indicate the Dgraph schema doesn't match the GraphQL schema.",
		field.Name(), field.Location().Line, field.Location().Column, field.Type().Name())

	val, errs := completeValue(path, field, nil)
	return val, append(errs, field.GqlErrorf(path, schema.ErrExpectedSingleItem))
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
	hrc, err := field.CustomHTTPConfig()
	if err != nil {
		return EmptyResult(field, err)
	}

	var body string
	if hrc.Template != nil {
		jsonTemplate := *hrc.Template
		if field.HasLambdaDirective() {
			jsonTemplate = getBodyForLambda(ctx, field, nil, *hrc.Template)
		}
		b, err := json.Marshal(jsonTemplate)
		if err != nil {
			return EmptyResult(field, jsonMarshalError(err, field, *hrc.Template))
		}
		body = string(b)
	}

	b, status, err := makeRequest(hr.Client, hrc.Method, hrc.URL, body, hrc.ForwardHeaders)
	if err != nil {
		return EmptyResult(field, externalRequestError(err, field))
	}

	// this means it had body and not graphql, so just unmarshal it and return
	if hrc.RemoteGqlQueryName == "" {
		var result interface{}
		var rerr restErr
		if status >= 200 && status < 300 {
			if err := Unmarshal(b, &result); err != nil {
				return EmptyResult(field, jsonUnmarshalError(err, field))
			}
		} else if err := Unmarshal(b, &rerr); err != nil {
			err = errors.Errorf("unexpected error with: %v", status)
			rerr.Errors = x.GqlErrorList{externalRequestError(err, field)}
		}

		return DataResult(field, map[string]interface{}{field.Name(): result}, rerr.Errors)
	}

	// we will reach here if it was a graphql request
	var resp struct {
		Data   map[string]interface{} `json:"data,omitempty"`
		Errors x.GqlErrorList         `json:"errors,omitempty"`
	}
	err = Unmarshal(b, &resp)
	if err != nil {
		gqlErr := jsonUnmarshalError(err, field)
		resp.Errors = append(resp.Errors, schema.AsGQLErrors(gqlErr)...)
		return EmptyResult(field, resp.Errors)
	}
	data, ok := resp.Data[hrc.RemoteGqlQueryName]
	if !ok {
		return EmptyResult(field, resp.Errors)
	}
	return DataResult(field, map[string]interface{}{field.Name(): data}, resp.Errors)
}

func (h *httpQueryResolver) Resolve(ctx context.Context, query schema.Query) *Resolved {
	return (*httpResolver)(h).Resolve(ctx, query)
}

func (h *httpMutationResolver) Resolve(ctx context.Context, mutation schema.Mutation) (*Resolved,
	bool) {
	resolved := (*httpResolver)(h).Resolve(ctx, mutation)
	return resolved, resolved.Err == nil || resolved.Err.Error() == ""
}

// Unmarshal is like json.Unmarshal() except it uses a custom decoder which preserves number
// precision by unmarshalling them into json.Number.
func Unmarshal(data []byte, v interface{}) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	return decoder.Decode(v)
}

func EmptyResult(f schema.Field, err error) *Resolved {
	return &Resolved{
		Data:  f.NullResponse(),
		Field: f,
		Err:   schema.GQLWrapLocationf(err, f.Location(), "resolving %s failed", f.Name()),
	}
}

func DataResult(f schema.Field, data map[string]interface{}, err error) *Resolved {
	b, errs := completeObject(make([]interface{}, 0, f.MaxPathLength()), []schema.Field{f}, data)

	return &Resolved{
		Data:  b,
		Field: f,
		Err:   schema.AppendGQLErrs(err, errs),
	}
}

func newtimer(ctx context.Context, Duration *schema.OffsetDuration) schema.OffsetTimer {
	resolveStartTime, _ := ctx.Value(resolveStartTime).(time.Time)
	tf := schema.NewOffsetTimerFactory(resolveStartTime)
	return tf.NewOffsetTimer(Duration)
}
