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
	"sync"

	"github.com/dgraph-io/dgraph/graphql/dgraph"

	dgoapi "github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	otrace "go.opencensus.io/trace"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/graphql/schema"
)

const (
	methodResolve = "RequestResolver.Resolve"

	resolverFailed    = false
	resolverSucceeded = true
)

// A ResolverFactory finds the right resolver for a query/mutation.
type ResolverFactory interface {
	queryResolverFor(query schema.Query) QueryResolver
	mutationResolverFor(mutation schema.Mutation) MutationResolver

	// WithQueryResolver adds a new query resolver.  Each time query name is resolved
	// resolver is called to create a new instane of a QueryResolver to resolve the
	// query.
	WithQueryResolver(name string, resolver func(schema.Query) QueryResolver) ResolverFactory

	// WithMutationResolver adds a new query resolver.  Each time mutation name is resolved
	// resolver is called to create a new instane of a MutationResolver to resolve the
	// mutation.
	WithMutationResolver(
		name string, resolver func(schema.Mutation) MutationResolver) ResolverFactory

	// WithConventionResolvers adds a set of our convention based resolvers to the
	// factory.  The registration happens only once.
	WithConventionResolvers(s schema.Schema, fns *ResolverFns) ResolverFactory

	// WithSchemaIntrospection adds schema introspection capabilities to the factory.
	// So __schema and __type queries can be resolved.
	WithSchemaIntrospection() ResolverFactory
}

// A ResultCompleter can take a []byte slice representing an intermediate result
// in resolving field and applies a completion step - for example, apply GraphQL
// error propagation or massaging error paths.
type ResultCompleter interface {
	Complete(ctx context.Context, field schema.Field, result []byte, err error) ([]byte, error)
}

// RequestResolver can process GraphQL requests and write GraphQL JSON responses.
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

	// returned if the factory gets asked for resolver for a field that it doesn't
	// know about.
	queryError    QueryResolverFunc
	mutationError MutationResolverFunc
}

// ResolverFns is a convenience struct for passing blocks of rewriters and executors.
type ResolverFns struct {
	Qrw QueryRewriter
	Arw MutationRewriter
	Urw MutationRewriter
	Drw MutationRewriter
	Qe  QueryExecutor
	Me  MutationExecutor
}

// dgraphExecutor is an implementation of both QueryExecutor and MutationExecutor
// that proxies query/mutation resolution through Query method in dgraph server.
type dgraphExecutor struct {
}

// A Resolved is the result of resolving a single query or mutation.
// A schema.Request may contain any number of queries or mutations (never both).
// RequestResolver.Resolve() resolves all of them by finding the resolved answers
// of the component queries/mutations and joining into a single schema.Response.
type Resolved struct {
	Data []byte
	Err  error
}

// CompletionFunc is an adapter that allows us to compose completions and build a
// ResultCompleter from a function.  Based on the http.HandlerFunc pattern.
type CompletionFunc func(
	ctx context.Context, field schema.Field, result []byte, err error) ([]byte, error)

// Complete calls cf(ctx, field, result, err)
func (cf CompletionFunc) Complete(
	ctx context.Context,
	field schema.Field,
	result []byte,
	err error) ([]byte, error) {

	return cf(ctx, field, result, err)
}

// DgraphAsQueryExecutor builds a QueryExecutor for proxying requests through dgraph.
func DgraphAsQueryExecutor() QueryExecutor {
	return &dgraphExecutor{}
}

// DgraphAsMutationExecutor builds a MutationExecutor for dog.
func DgraphAsMutationExecutor() MutationExecutor {
	return &dgraphExecutor{}
}

func (de *dgraphExecutor) Query(ctx context.Context, query *gql.GraphQuery) ([]byte, error) {
	return dgraph.Query(ctx, query)
}

func (de *dgraphExecutor) Mutate(
	ctx context.Context,
	query *gql.GraphQuery,
	mutations []*dgoapi.Mutation) (map[string]string, []string, error) {
	return dgraph.Mutate(ctx, query, mutations)
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
	introspect := func(q schema.Query) QueryResolver {
		return &queryResolver{
			queryRewriter:   NoOpQueryRewrite(),
			queryExecutor:   introspectionExecution(q),
			resultCompleter: removeObjectCompletion(noopCompletion),
		}
	}

	rf.WithQueryResolver("__schema", introspect)
	rf.WithQueryResolver("__type", introspect)

	return rf
}

func (rf *resolverFactory) WithConventionResolvers(
	s schema.Schema, fns *ResolverFns) ResolverFactory {

	queries := append(s.Queries(schema.GetQuery), s.Queries(schema.FilterQuery)...)
	for _, q := range queries {
		rf.WithQueryResolver(q, func(q schema.Query) QueryResolver {
			return NewQueryResolver(fns.Qrw, fns.Qe, StdQueryCompletion())
		})
	}

	for _, m := range s.Mutations(schema.AddMutation) {
		rf.WithMutationResolver(m, func(m schema.Mutation) MutationResolver {
			return NewMutationResolver(
				fns.Arw, fns.Qe, fns.Me, StdMutationCompletion(m.ResponseName()))
		})
	}

	for _, m := range s.Mutations(schema.UpdateMutation) {
		rf.WithMutationResolver(m, func(m schema.Mutation) MutationResolver {
			return NewMutationResolver(
				fns.Urw, fns.Qe, fns.Me, StdMutationCompletion(m.ResponseName()))
		})
	}

	for _, m := range s.Mutations(schema.DeleteMutation) {
		rf.WithMutationResolver(m, func(m schema.Mutation) MutationResolver {
			return NewMutationResolver(
				fns.Drw, NoOpQueryExecution(), fns.Me, StdDeleteCompletion(m.ResponseName()))
		})
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

		queryError:    queryError,
		mutationError: mutationError,
	}
}

// StdQueryCompletion is the completion steps that get run for queries
func StdQueryCompletion() CompletionFunc {
	return removeObjectCompletion(completeDgraphResult)
}

// StdMutationCompletion is the completion steps that get run for add and update mutations
func StdMutationCompletion(name string) CompletionFunc {
	return addPathCompletion(name, addRootFieldCompletion(name, completeDgraphResult))
}

// StdDeleteCompletion is the completion steps that get run for add and update mutations
func StdDeleteCompletion(name string) CompletionFunc {
	return addPathCompletion(name, addRootFieldCompletion(name, deleteCompletion()))
}

func (rf *resolverFactory) queryResolverFor(query schema.Query) QueryResolver {
	if resolver, ok := rf.queryResolvers[query.Name()]; ok {
		return resolver(query)
	}

	return rf.queryError
}

func (rf *resolverFactory) mutationResolverFor(mutation schema.Mutation) MutationResolver {
	if resolver, ok := rf.mutationResolvers[mutation.Name()]; ok {
		return resolver(mutation)
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

	reqID := api.RequestID(ctx)

	if r == nil {
		glog.Errorf("[%s] Call to Resolve with nil RequestResolver", reqID)
		return schema.ErrorResponse(errors.New("Internal error"), reqID)
	}

	if r.schema == nil {
		glog.Errorf("[%s] Call to Resolve with no schema", reqID)
		return schema.ErrorResponse(errors.New("Internal error"), reqID)
	}

	op, err := r.schema.Operation(gqlReq)
	if err != nil {
		return schema.ErrorResponse(err, reqID)
	}

	resp := &schema.Response{
		Extensions: &schema.Extensions{
			RequestID: reqID,
		},
	}

	if glog.V(3) {
		b, err := json.Marshal(gqlReq.Variables)
		if err != nil {
			glog.Infof("Failed to marshal variables for logging : %s", err)
		}
		glog.Infof("[%s] Resolving GQL request: \n%s\nWith Variables: \n%s\n",
			reqID, gqlReq.Query, string(b))
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

				allResolved[storeAt] = r.resolvers.queryResolverFor(q).Resolve(ctx, q)
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
			res, allSuccessful = r.resolvers.mutationResolverFor(m).Resolve(ctx, m)
			resp.WithError(res.Err)
			resp.AddData(res.Data)
		}
	case op.IsSubscription():
		resp.WithError(errors.Errorf("Subscriptions not yet supported."))
	}

	return resp
}

// noopCompletion just passes back it's result and err arguments
func noopCompletion(
	ctx context.Context, field schema.Field, result []byte, err error) ([]byte, error) {
	return result, err
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
	return CompletionFunc(
		func(ctx context.Context, field schema.Field, result []byte, err error) ([]byte, error) {
			res, err := cf(ctx, field, result, err)
			if len(res) >= 2 {
				res = res[1 : len(res)-1]
			}
			return res, err
		})
}

// addRootFieldCompletion adds an extra object name to the start of a result.
//
// A mutation always looks like
//   `addFoo(...) { foo { ... } }`
// What's resolved initially is
//   `foo { ... }`
// So `addFoo: ...` is added.
func addRootFieldCompletion(name string, cf CompletionFunc) CompletionFunc {
	return CompletionFunc(func(
		ctx context.Context, field schema.Field, result []byte, err error) ([]byte, error) {

		res, err := cf(ctx, field, result, err)

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
func addPathCompletion(name string, cf CompletionFunc) CompletionFunc {
	return CompletionFunc(func(
		ctx context.Context, field schema.Field, result []byte, err error) ([]byte, error) {

		res, err := cf(ctx, field, result, err)

		resErrs := schema.AsGQLErrors(err)
		for _, err := range resErrs {
			if len(err.Path) > 0 {
				err.Path = append([]interface{}{name}, err.Path...)
			}
		}

		return res, resErrs
	})
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
func completeDgraphResult(ctx context.Context, field schema.Field, dgResult []byte, e error) (
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

	errs := schema.AsGQLErrors(e)
	if len(dgResult) == 0 {
		return nil, errs
	}

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
		if !includeField {
			continue
		}

		buf.WriteString(comma)
		buf.WriteRune('"')
		buf.WriteString(f.ResponseName())
		buf.WriteString(`": `)

		val := res[f.ResponseName()]
		if f.Name() == schema.TypenameDirective {
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

		completed, err := completeValue(append(path, f.ResponseName()), f, val)
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
		// or Dgraph returned something unexpected.
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

	glog.Errorf("completeList() called in resolving %s (Line: %v, Column: %v), "+
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
