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

/*
Package resolve does...
*/
package resolve

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/dgraph"

	dgoapi "github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/api"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	otrace "go.opencensus.io/trace"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
)

const (
	methodResolve = "RequestResolver.Resolve"

	resolverFailed    = false
	resolverSucceeded = true
)

// A Resolver can resolve a single query or mutation
type Resolver interface {
	Resolve(ctx context.Context) (*Resolved, bool)
}

// A ResolverFactory finds the right resolver for field given the context of a
// schema and operation
type ResolverFactory interface {

	// ResolverFor finds the right resolver to resolve field
	ResolverFor(schema schema.Schema, operation schema.Operation, field schema.Field) Resolver

	// WithResolver adds a new special case resolver to the resolver factory
	WithResolver(finder ResolverFinder) ResolverFactory

	// WithQueryRewriter adds a new query rewriter to the factory.  The rewriter
	// is used in the usual resolver pipeline.
	WithQueryRewriter(finder QueryRewriterFinder) ResolverFactory
}

// A QueryRewriter can build a Dgraph gql.GraphQuery from a GraphQL query, and
// can build a Dgraph gql.GraphQuery to follow a GraphQL mutation.
//
// GraphQL queries come in like:
//
// query {
// 	 getAuthor(id: "0x1") {
// 	  name
// 	 }
// }
//
// and get rewritten straight to Dgraph.  But mutations come in like:
//
// mutation addAuthor($auth: AuthorInput!) {
//   addAuthor(input: $auth) {
// 	   author {
// 	     id
// 	     name
// 	   }
//   }
// }
//
// Where `addAuthor(input: $auth)` implies a mutation that must get run, and the
// remainder implies a query to run and return the newly created author, so the
// mutation query rewriting is dependent on the context set up by the result of
// the mutation.
type QueryRewriter interface {
	Rewrite(q schema.Query) (*gql.GraphQuery, error)
	FromMutationResult(
		m schema.Mutation,
		assigned map[string]string,
		mutated map[string][]string) (*gql.GraphQuery, error)
}

// A MutationRewriter can transform a GraphQL mutation into a list of Dgraph mutations.
type MutationRewriter interface {
	Rewrite(m schema.Mutation) (*gql.GraphQuery, []*dgoapi.Mutation, error)
}

// A QueryExecutor can execute a gql.GraphQuery and return a result.  QueryExecutor's
// can be chained in a pipeline and don't need to return valid GraphQL results.
type QueryExecutor interface {
	Query(ctx context.Context, query *gql.GraphQuery) ([]byte, error)
}

// A MutationExecutor can execute a mutation and returns the assigned map, the
// mutated map and any errors.
type MutationExecutor interface {
	Mutate(ctx context.Context,
		query *gql.GraphQuery,
		mutations []*dgoapi.Mutation) (map[string]string, map[string][]string, error)
}

// A ResultCompleter can take a []byte slice representing an intermediate result
// in resolving field and applies a completion step - for example, apply GraphQL
// error propagation or massaging error paths.
type ResultCompleter interface {
	Complete(ctx context.Context, field schema.Field, result []byte) ([]byte, error)
}

// RequestResolver can process GraphQL requests and write GraphQL JSON responses.
type RequestResolver struct {
	GqlReq    *schema.Request
	Schema    schema.Schema
	resolvers ResolverFactory
}

// A resolverFactory is the main implementation of ResolverFactory.  It can generate
// a resolver for a query/mutation by building a resolver from rewriters and
// executors.
type resolverFactory struct {
	resolvers         []ResolverFinder
	queryRewriters    []QueryRewriterFinder
	mutationRewriters []mutationRewriterFinder
	queryExecutors    []queryExecutorFinder
	mutationExecutors []mutationExecutorFinder
	completers        []completerFinder
}

type ResolverFinder func(
	schema schema.Schema, operation schema.Operation, field schema.Field) Resolver

type QueryRewriterFinder func(
	schema schema.Schema, operation schema.Operation, field schema.Field) QueryRewriter

type mutationRewriterFinder func(
	schema schema.Schema, operation schema.Operation, field schema.Field) MutationRewriter

type queryExecutorFinder func(
	schema schema.Schema, operation schema.Operation, field schema.Field) QueryExecutor

type mutationExecutorFinder func(
	schema schema.Schema, operation schema.Operation, field schema.Field) MutationExecutor

type completerFinder func(
	schema schema.Schema, operation schema.Operation, field schema.Field) ResultCompleter

// A Resolved is the result of resolving a single query or mutation.
// A schema.Request may contain any number of queries or mutations (never both).
// RequestResolver.Resolve() resolves all of them by finding the resolved answers
// of the component queries/mutations and joining into a single schema.Response.
type Resolved struct {
	Data []byte
	Err  error
}

// An ErrorResolver is a Resolver that always results in error err.
type ErrorResolver struct {
	Err error
}

// a noopQueryExecutor does nothing and returns nil
type noopQueryExecutor struct{}

// a noopRewriter does nothing and returns nil
type noopRewriter struct{}

// CompletionFunc is an adapter that allows us to compose completions - based
// off the http.HandlerFunc pattern.
type CompletionFunc func(ctx context.Context, field schema.Field, result []byte) ([]byte, error)

// Complete calls cf(ctx, field, result)
func (cf CompletionFunc) Complete(ctx context.Context, field schema.Field, result []byte) ([]byte, error) {
	return cf(ctx, field, result)
}

// Resolve for an ErrorResolver just returns nil data and an error
func (er *ErrorResolver) Resolve(ctx context.Context) (*Resolved, bool) {
	return &Resolved{Err: er.Err}, resolverFailed
}

func (no *noopQueryExecutor) Query(ctx context.Context, query *gql.GraphQuery) ([]byte, error) {
	return nil, nil
}

func (no *noopRewriter) Rewrite(q schema.Query) (*gql.GraphQuery, error) {
	return nil, nil
}

func (no *noopRewriter) FromMutationResult(
	m schema.Mutation,
	assigned map[string]string,
	mutated map[string][]string) (*gql.GraphQuery, error) {
	return nil, nil
}

// NewResolverFactory returns a ResolverFactory that resolves requests via
// query/mutation rewriting and execution through Dgraph.
func NewResolverFactory(dg dgraph.Client,
	queryRewriter QueryRewriter,
	mutRewriter MutationRewriter) ResolverFactory {
	return (&resolverFactory{
		resolvers:         []ResolverFinder{noResolvers},
		queryRewriters:    []QueryRewriterFinder{alwaysQueryRewriter(queryRewriter)},
		mutationRewriters: []mutationRewriterFinder{alwaysMutationRewriter(mutRewriter)},
		queryExecutors:    []queryExecutorFinder{dgraphQueryExecutor(dg)},
		mutationExecutors: []mutationExecutorFinder{dgraphMutationExecutor(dg)},
		completers:        []completerFinder{stdQueryCompleter, stdMutationCompleter},
	}).withSchemaIntrospection()
}

func (rf *resolverFactory) WithResolver(finder ResolverFinder) ResolverFactory {
	rf.resolvers = append([]ResolverFinder{finder}, rf.resolvers...)
	return rf
}

func (rf *resolverFactory) WithQueryRewriter(finder QueryRewriterFinder) ResolverFactory {
	rf.queryRewriters = append([]QueryRewriterFinder{finder}, rf.queryRewriters...)
	return rf
}

func (rf *resolverFactory) WithQueryExecutor(finder queryExecutorFinder) ResolverFactory {
	rf.queryExecutors = append([]queryExecutorFinder{finder}, rf.queryExecutors...)
	return rf
}

func (rf *resolverFactory) WithCompleter(finder completerFinder) ResolverFactory {
	rf.completers = append([]completerFinder{finder}, rf.completers...)
	return rf
}

func (rf *resolverFactory) withSchemaIntrospection() ResolverFactory {

	rf.WithQueryRewriter(
		func(gqlSchema schema.Schema, op schema.Operation, field schema.Field) QueryRewriter {
			if qry, ok := field.(schema.Query); ok {
				if qry.QueryType() == schema.SchemaQuery {
					return &noopRewriter{}
				}
			}
			return nil
		})

	rf.WithQueryExecutor(
		func(gqlSchema schema.Schema, op schema.Operation, field schema.Field) QueryExecutor {
			if qry, ok := field.(schema.Query); ok {
				if qry.QueryType() == schema.SchemaQuery {
					return &schemaIntrospector{
						schema:    gqlSchema,
						operation: op,
						query:     qry,
					}
				}
			}
			return nil
		})

	rf.WithCompleter(
		func(gqlSchema schema.Schema, op schema.Operation, field schema.Field) ResultCompleter {
			if qry, ok := field.(schema.Query); ok {
				if qry.QueryType() == schema.SchemaQuery {
					return removeObjectCompletion(noopCompletion)
				}
			}
			return nil
		})

	return rf
}

func stdQueryCompleter(gqlSchema schema.Schema, op schema.Operation, field schema.Field) ResultCompleter {
	if _, ok := field.(schema.Query); ok {
		return removeObjectCompletion(completeDgraphResult)
	}
	return nil
}

func stdMutationCompleter(gqlSchema schema.Schema, op schema.Operation, field schema.Field) ResultCompleter {
	if mut, ok := field.(schema.Mutation); ok {
		switch mut.MutationType() {
		case schema.AddMutation, schema.UpdateMutation:
			return addPathCompletion(
				addObjectCompletion(completeDgraphResult, field.ResponseName()),
				field.ResponseName())
		case schema.DeleteMutation:
			return &deleteCompleter{}
		}
	}
	return nil
}

func noResolvers(gqlSchema schema.Schema, op schema.Operation, field schema.Field) Resolver {
	return nil
}

func alwaysQueryRewriter(qrw QueryRewriter) func(gqlSchema schema.Schema, op schema.Operation, field schema.Field) QueryRewriter {
	return func(gqlSchema schema.Schema, op schema.Operation, field schema.Field) QueryRewriter {
		return qrw
	}
}

func alwaysMutationRewriter(mrw MutationRewriter) func(gqlSchema schema.Schema, op schema.Operation, field schema.Field) MutationRewriter {
	return func(gqlSchema schema.Schema, op schema.Operation, field schema.Field) MutationRewriter {
		return mrw
	}
}

func dgraphQueryExecutor(dg dgraph.Client) func(gqlSchema schema.Schema, op schema.Operation, field schema.Field) QueryExecutor {
	return func(gqlSchema schema.Schema, op schema.Operation, field schema.Field) QueryExecutor {
		return dg
	}
}

func dgraphMutationExecutor(dg dgraph.Client) func(gqlSchema schema.Schema, op schema.Operation, field schema.Field) MutationExecutor {
	return func(gqlSchema schema.Schema, op schema.Operation, field schema.Field) MutationExecutor {
		return dg
	}
}

// want to keep massaging this a bit, so a ResolverFactory has some hooks like
func (rf *resolverFactory) ResolverFor(
	gqlSchema schema.Schema,
	op schema.Operation,
	field schema.Field) Resolver {

	errorResolver := &ErrorResolver{
		Err: errors.Errorf("%s was not executed because no suitable resolver could be found - "+
			"this indicates a resolver or validation bug "+
			"(Please let us know : https://github.com/dgraph-io/dgraph/issues)", field.Name())}

	specialCase := rf.specialCaseResolverFor(gqlSchema, op, field)
	if specialCase != nil {
		return specialCase
	}

	queryRewriter := rf.queryRewriterFor(gqlSchema, op, field)
	queryExecutor := rf.queryExecutorFor(gqlSchema, op, field)
	resultCompleter := rf.completerFor(gqlSchema, op, field)

	if queryRewriter == nil || queryExecutor == nil || resultCompleter == nil {
		return errorResolver
	}

	switch field := field.(type) {
	case schema.Query:
		return &queryResolver{
			query:           field,
			queryRewriter:   queryRewriter,
			queryExecutor:   queryExecutor,
			resultCompleter: resultCompleter,
		}

		/*
			// FIXME ... then this all goes
			if field.QueryType() == schema.SchemaQuery {
				return &queryResolver{
					query:         field,
					queryRewriter: &noopRewriter{},
					queryExecutor: &schemaIntrospector{
						schema:    gqlSchema,
						operation: op,
						query:     field,
					},
					resultCompleter: removeObjectCompletion(noopCompletion),
				}
			}
			return &queryResolver{
				query:           field,
				queryRewriter:   rf.queryRewriter,
				queryExecutor:   rf.dgraph,
				resultCompleter: removeObjectCompletion(completeDgraphResult),
			}
			// --- snip
		*/

	case schema.Mutation:
		mutationRewriter := rf.mutationRewriterFor(gqlSchema, op, field)
		mutationExecutor := rf.mutationExecutorFor(gqlSchema, op, field)

		if mutationRewriter == nil || mutationExecutor == nil {
			return errorResolver
		}

		return &mutationResolver{
			mutation:         field,
			queryRewriter:    queryRewriter,
			mutationRewriter: mutationRewriter,
			queryExecutor:    queryExecutor,
			mutationExecutor: mutationExecutor,
			resultCompleter:  resultCompleter,
		}

		/*
			resultCompleter: addPathCompletion(
							addObjectCompletion(completeDgraphResult, field.ResponseName()),
							field.ResponseName()),

						switch field.MutationType() {
						case schema.AddMutation, schema.UpdateMutation:
							return &mutationResolver{
								mutation:         field,
								queryRewriter:    rf.queryRewriter,
								mutationRewriter: rf.mutationRewriter,
								queryExecutor:    rf.dgraph,
								mutationExecutor: rf.dgraph,
								resultCompleter: addPathCompletion(
									addObjectCompletion(completeDgraphResult, field.ResponseName()),
									field.ResponseName()),
							}
						case schema.DeleteMutation:
							return &mutationResolver{
								mutation:         field,
								queryRewriter:    &noopRewriter{},
								mutationRewriter: rf.mutationRewriter,
								queryExecutor:    &noopQueryExecutor{},
								mutationExecutor: rf.dgraph,
								resultCompleter:  &deleteCompleter{},
							}
						}
		*/
	}

	return errorResolver
}

// Generics anyone :-)
func (rf *resolverFactory) specialCaseResolverFor(
	gqlSchema schema.Schema,
	op schema.Operation,
	field schema.Field) Resolver {

	for _, f := range rf.resolvers {
		r := f(gqlSchema, op, field)
		if r != nil {
			return r
		}
	}

	return nil
}

func (rf *resolverFactory) queryRewriterFor(
	gqlSchema schema.Schema,
	op schema.Operation,
	field schema.Field) QueryRewriter {

	for _, f := range rf.queryRewriters {
		r := f(gqlSchema, op, field)
		if r != nil {
			return r
		}
	}

	return nil
}

func (rf *resolverFactory) mutationRewriterFor(
	gqlSchema schema.Schema,
	op schema.Operation,
	field schema.Field) MutationRewriter {

	for _, f := range rf.mutationRewriters {
		r := f(gqlSchema, op, field)
		if r != nil {
			return r
		}
	}

	return nil
}

func (rf *resolverFactory) queryExecutorFor(
	gqlSchema schema.Schema,
	op schema.Operation,
	field schema.Field) QueryExecutor {

	for _, f := range rf.queryExecutors {
		r := f(gqlSchema, op, field)
		if r != nil {
			return r
		}
	}

	return nil
}

func (rf *resolverFactory) mutationExecutorFor(
	gqlSchema schema.Schema,
	op schema.Operation,
	field schema.Field) MutationExecutor {

	for _, f := range rf.mutationExecutors {
		r := f(gqlSchema, op, field)
		if r != nil {
			return r
		}
	}

	return nil
}

func (rf *resolverFactory) completerFor(
	gqlSchema schema.Schema,
	op schema.Operation,
	field schema.Field) ResultCompleter {

	for _, f := range rf.completers {
		r := f(gqlSchema, op, field)
		if r != nil {
			return r
		}
	}

	return nil
}

// removeObjectCompletion chops leading '{' and trailing '}' from a JSON object
//
// The final GraphQL result gets built like
// { data:
//    {
//      q1: {...},
//      q2: [ {...}, {...} ],
//      ...
//    }
// }
//
// When we are building a single one of the q's, the result is built initially as
// { q1: {...} }
// so the completed result should be
// q1: {...}
func removeObjectCompletion(cf CompletionFunc) CompletionFunc {
	return CompletionFunc(func(ctx context.Context, field schema.Field, result []byte) ([]byte, error) {
		res, err := cf(ctx, field, result)
		if len(res) >= 2 {
			res = res[1 : len(res)-1]
		}
		return res, err
	})
}

// addObjectCompletion adds an extra object name to the start of a result.
//
// A mutation always looks like
//   `addFoo(...) { foo { ... } }`
// What's resolved initially is
//   `foo { ... }`
// So `addFoo: ...` is added.
func addObjectCompletion(cf CompletionFunc, name string) CompletionFunc {
	return CompletionFunc(func(ctx context.Context, field schema.Field, result []byte) ([]byte, error) {

		res, err := cf(ctx, field, result)

		var b bytes.Buffer
		b.WriteString("\"")
		b.WriteString(name)
		b.WriteString(`": `)
		if len(res) > 0 {
			b.Write(res)
		} else {
			b.WriteString("null")
		}

		return b.Bytes(), err
	})
}

// addPathCompletion adds an extra object name to the start of every error path
// arrising from applying cf.
//
// A mutation always looks like
//   `addFoo(...) { foo { ... } }`
// But cf's error paths begin at `foo`, so `addFoo` needs to be added to all.
func addPathCompletion(cf CompletionFunc, name string) CompletionFunc {
	return CompletionFunc(func(ctx context.Context, field schema.Field, result []byte) ([]byte, error) {

		res, err := cf(ctx, field, result)

		resErrs := schema.AsGQLErrors(err)
		for _, err := range resErrs {
			if len(err.Path) > 0 {
				err.Path = append([]interface{}{name}, err.Path...)
			}
		}

		return res, resErrs
	})
}

// New creates a new RequestResolver
func New(s schema.Schema, resolverFactory ResolverFactory) *RequestResolver {
	return &RequestResolver{
		Schema:    s,
		resolvers: resolverFactory,
	}
}

// Resolve processes r.GqlReq and returns a GraphQL response.
// r.GqlReq should be set with a request before Resolve is called
// and a schema and backend Dgraph should have been added.
// Resolve records any errors in the response's error field.
func (r *RequestResolver) Resolve(ctx context.Context) *schema.Response {
	span := otrace.FromContext(ctx)
	stop := x.SpanTimer(span, methodResolve)
	defer stop()

	reqID := api.RequestID(ctx)

	if r == nil {
		glog.Errorf("[%s] Call to Resolve with nil RequestResolver", reqID)
		return schema.ErrorResponse(errors.New("Internal error"), reqID)
	}

	if r.Schema == nil {
		glog.Errorf("[%s] Call to Resolve with no schema", reqID)
		return schema.ErrorResponse(errors.New("Internal error"), reqID)
	}

	op, err := r.Schema.Operation(r.GqlReq)
	if err != nil {
		return schema.ErrorResponse(err, reqID)
	}

	resp := &schema.Response{
		Extensions: &schema.Extensions{
			RequestID: reqID,
		},
	}

	if glog.V(3) {
		b, err := json.Marshal(r.GqlReq.Variables)
		if err != nil {
			glog.Infof("Failed to marshal variables for logging : %s", err)
		}
		glog.Infof("[%s] Resolving GQL request: \n%s\nWith Variables: \n%s\n",
			reqID, r.GqlReq.Query, string(b))
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
		allResolved := make([]*Resolved, len(op.Queries()))

		for i, q := range op.Queries() {
			wg.Add(1)

			go func(q schema.Query, storeAt int) {
				defer wg.Done()
				defer api.PanicHandler(api.RequestID(ctx),
					func(err error) {
						allResolved[storeAt] = &Resolved{Err: err}
					})

				allResolved[storeAt], _ = r.resolvers.ResolverFor(r.Schema, op, q).Resolve(ctx)
			}(q, i)
		}
		wg.Wait()

		// The GraphQL data response needs to be written in the same order as the
		// queries in the request.
		for _, res := range allResolved {
			// Errors and data in the same response is valid.  Both WithError and
			// AddData handle nil cases.
			resp.WithError(res.Err)
			resp.AddData(res.Data)
		}
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
			res, allSuccessful = r.resolvers.ResolverFor(r.Schema, op, m).Resolve(ctx)
			resp.WithError(res.Err)
			resp.AddData(res.Data)
		}
	case op.IsSubscription():
		resp.WithError(errors.Errorf("Subscriptions not yet supported."))
	}

	return resp
}

// noopCompletion just passes back it's result argument
func noopCompletion(ctx context.Context, field schema.Field, result []byte) (
	[]byte, error) {
	return result, nil
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
func completeDgraphResult(ctx context.Context, field schema.Field, dgResult []byte) (
	[]byte, error) {
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

	var errs x.GqlErrorList

	nullResponse := func() []byte {
		var buf bytes.Buffer
		buf.WriteString(`{ "`)
		buf.WriteString(field.ResponseName())
		buf.WriteString(`": null }`)
		return buf.Bytes()
	}

	dgraphError := func() ([]byte, error) {
		glog.Errorf("[%s] Could not process Dgraph result : \n%s",
			api.RequestID(ctx), string(dgResult))
		return nullResponse(),
			x.GqlErrorf("Couldn't process the result from Dgraph.  " +
				"This probably indicates a bug in the Dgraph GraphQL layer.  " +
				"Please let us know : https://github.com/dgraph-io/dgraph/issues.").
				WithLocations(field.Location())
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
		return nullResponse(),
			schema.GQLWrapLocationf(err, field.Location(), "couldn't unmarshal Dgraph result")
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

				glog.Errorf("[%s] Got a list of length %v from Dgraph when expecting a "+
					"one-item list.\n"+
					"GraphQL query was : %s\n",
					api.RequestID(ctx), len(val), api.QueryString(ctx))

				errs = append(errs,
					x.GqlErrorf(
						"Dgraph returned a list, but %s (type %s) was expecting just one item.  "+
							"The first item in the list was used to produce the result. "+
							"Logged as a potential bug; see the API log for more details.",
						field.Name(), field.Type().String()).WithLocations(field.Location()))
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
			"That's only possible if the query result is non-nullable.  "+
			"There's something wrong in the GraphQL schema.  \n"+
			"GraphQL query was : %s\n",
			api.RequestID(ctx), api.QueryString(ctx))
		return nullResponse(), append(errs, gqlErrs...)
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
func completeObject(
	path []interface{},
	typ schema.Type,
	fields []schema.Field,
	res map[string]interface{}) ([]byte, x.GqlErrorList) {

	var errs x.GqlErrorList
	var buf bytes.Buffer
	comma := ""

	buf.WriteRune('{')

	dgraphTypes, _ := res["dgraph.type"].([]interface{})
	for _, f := range fields {
		if f.Skip() || !f.Include() {
			continue
		}

		inputType := f.ConcreteType(dgraphTypes)
		// If typ is an interface, and dgraphTypes contains another type, then we ignore
		// fields which don't start with that type. This would happen when multiple
		// fragments (belonging to different types) are requested within a query for an interface.

		// If the dgraphPredicate doesn't start with the typ.Name(), then this field belongs to
		// a concrete type, lets check that it has inputType as the prefix, otherwise skip it.
		if inputType != "" && !strings.HasPrefix(f.DgraphPredicate(), typ.Name()) &&
			!strings.HasPrefix(f.DgraphPredicate(), inputType) {
			continue
		}

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
func completeValue(
	path []interface{},
	field schema.Field,
	val interface{}) ([]byte, x.GqlErrorList) {

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

			gqlErr := x.GqlErrorf(
				"Non-nullable field '%s' (type %s) was not present in result from Dgraph.  "+
					"GraphQL error propagation triggered.", field.Name(), field.Type()).
				WithLocations(field.Location())
			gqlErr.Path = copyPath(path)

			return nil, x.GqlErrorList{gqlErr}
		}

		// val is a scalar

		// Can this ever error?  We can't have an unsupported type or value because
		// we just unmarshaled this val.
		json, err := json.Marshal(val)
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
func completeList(
	path []interface{},
	field schema.Field,
	values []interface{}) ([]byte, x.GqlErrorList) {

	var buf bytes.Buffer
	var errs x.GqlErrorList
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

func mismatched(
	path []interface{},
	field schema.Field,
	values []interface{}) ([]byte, x.GqlErrorList) {

	glog.Error("completeList() called in resolving %s (Line: %v, Column: %v), "+
		"but its type is %s.\n"+
		"That could indicate the Dgraph schema doesn't match the GraphQL schema.",
		field.Name(), field.Location().Line, field.Location().Column, field.Type().Name())

	gqlErr := &x.GqlError{
		Message: "Dgraph returned a list, but GraphQL was expecting just one item.  " +
			"This indicates an internal error - " +
			"probably a mismatch between GraphQL and Dgraph schemas.  " +
			"The value was resolved as null (which may trigger GraphQL error propagation) " +
			"and as much other data as possible returned.",
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
