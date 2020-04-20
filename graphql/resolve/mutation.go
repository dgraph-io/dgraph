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

	dgoapi "github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/dgraph"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/x"
	otrace "go.opencensus.io/trace"
)

const touchedUidsKey = "_total"

// Mutations come in like this with variables:
//
// mutation themutation($post: PostInput!) {
//   addPost(input: $post) { ... some query ...}
// }
// - with variable payload
// { "post":
//   { "title": "My Post",
//     "author": { authorID: 0x123 },
//     ...
//   }
// }
//
//
// Or, like this with the payload in the mutation arguments
//
// mutation themutation {
//   addPost(input: { title: ... }) { ... some query ...}
// }
//
//
// Either way we build up a Dgraph json mutation to add the object
//
// For now, all mutations are only 1 level deep (cause of how we build the
// input objects) and only create a single node (again cause of inputs)

// A MutationResolver can resolve a single mutation.
type MutationResolver interface {
	Resolve(ctx context.Context, mutation schema.Mutation) (*Resolved, bool)
}

// A MutationRewriter can transform a GraphQL mutation into a Dgraph mutation and
// can build a Dgraph gql.GraphQuery to follow a GraphQL mutation.
//
// Mutations come in like:
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
// Where `addAuthor(input: $auth)` implies a mutation that must get run - written
// to a Dgraph mutation by Rewrite.  The GraphQL following `addAuthor(...)`implies
// a query to run and return the newly created author, so the
// mutation query rewriting is dependent on the context set up by the result of
// the mutation.
type MutationRewriter interface {
	// Rewrite rewrites GraphQL mutation m into a Dgraph mutation - that could
	// be as simple as a single DelNquads, or could be a Dgraph upsert mutation
	// with a query and multiple mutations guarded by conditions.
	Rewrite(ctx context.Context, m schema.Mutation) (*UpsertMutation, error)

	// FromMutationResult takes a GraphQL mutation and the results of a Dgraph
	// mutation and constructs a Dgraph query.  It's used to find the return
	// value from a GraphQL mutation - i.e. we've run the mutation indicated by m
	// now we need to query Dgraph to satisfy all the result fields in m.
	FromMutationResult(
		ctx context.Context,
		m schema.Mutation,
		assigned map[string]string,
		result map[string]interface{}) (*gql.GraphQuery, error)
}

// A DgraphExecutor can execute a mutation and returns the request response and any errors.
type DgraphExecutor interface {
	// Execute performs the actual mutation and returns a Dgraph response. If an error
	// occurs, that indicates that the execution failed in some way significant enough
	// way as to not continue processing this mutation or others in the same request.
	Execute(ctx context.Context, req *dgoapi.Request) (*dgoapi.Response, error)
}

// An UpsertMutation is the query and mutations needed for a Dgraph upsert.
// The node types is a blank node name -> Type mapping of nodes that could
// be created by the upsert.
type UpsertMutation struct {
	Query     *gql.GraphQuery
	Mutations []*dgoapi.Mutation
	NodeTypes map[string]schema.Type
}

// DgraphExecutorFunc is an adapter that allows us to compose dgraph execution and
// build a QueryExecuter from a function.  Based on the http.HandlerFunc pattern.
type DgraphExecutorFunc func(ctx context.Context, req *dgoapi.Request) (*dgoapi.Response, error)

// Execute calls qe(ctx, query)
func (ex DgraphExecutorFunc) Execute(
	ctx context.Context,
	req *dgoapi.Request) (*dgoapi.Response, error) {

	return ex(ctx, req)
}

// MutationResolverFunc is an adapter that allows to build a MutationResolver from
// a function.  Based on the http.HandlerFunc pattern.
type MutationResolverFunc func(ctx context.Context, m schema.Mutation) (*Resolved, bool)

// Resolve calls mr(ctx, mutation)
func (mr MutationResolverFunc) Resolve(ctx context.Context, m schema.Mutation) (*Resolved, bool) {
	return mr(ctx, m)
}

// NewDgraphResolver creates a new mutation resolver.  The resolver runs the pipeline:
// 1) rewrite the mutation using mr (return error if failed)
// 2) execute the mutation with me (return error if failed)
// 3) write a query for the mutation with mr (return error if failed)
// 4) execute the query with qe (return error if failed)
// 5) process the result with rc
func NewDgraphResolver(
	mr MutationRewriter,
	ex DgraphExecutor,
	rc ResultCompleter) MutationResolver {
	return &dgraphResolver{
		mutationRewriter: mr,
		executor:         ex,
		resultCompleter:  rc,
	}
}

// mutationResolver can resolve a single GraphQL mutation field
type dgraphResolver struct {
	mutationRewriter MutationRewriter
	executor         DgraphExecutor
	resultCompleter  ResultCompleter
}

func (mr *dgraphResolver) Resolve(ctx context.Context, m schema.Mutation) (*Resolved, bool) {
	span := otrace.FromContext(ctx)
	stop := x.SpanTimer(span, "resolveMutation")
	defer stop()
	if span != nil {
		span.Annotatef(nil, "mutation alias: [%s] type: [%s]", m.Alias(), m.MutationType())
	}

	resolved, success := mr.rewriteAndExecute(ctx, m)
	mr.resultCompleter.Complete(ctx, resolved)
	return resolved, success
}

func getNumUids(m schema.Mutation, a map[string]string, r map[string]interface{}) int {
	switch m.MutationType() {
	case schema.AddMutation:
		return len(a)
	default:
		mutated := extractMutated(r, m.ResponseName())
		return len(mutated)
	}
}

func (mr *dgraphResolver) rewriteAndExecute(
	ctx context.Context,
	mutation schema.Mutation) (*Resolved, bool) {

	emptyResult := func(err error) *Resolved {
		return &Resolved{
			Data:  map[string]interface{}{mutation.ResponseName(): nil},
			Field: mutation,
			Err:   err,
		}
	}

	upsert, err := mr.mutationRewriter.Rewrite(ctx, mutation)
	if err != nil {
		return emptyResult(schema.GQLWrapf(err, "couldn't rewrite mutation %s", mutation.Name())),
			resolverFailed
	}

	req := &dgoapi.Request{
		Query:     dgraph.AsString(upsert.Query),
		CommitNow: true,
		Mutations: upsert.Mutations,
	}

	mutResp, err := mr.executor.Execute(ctx, req)
	if err != nil {
		gqlErr := schema.GQLWrapLocationf(
			err, mutation.Location(), "mutation %s failed", mutation.Name())
		return emptyResult(gqlErr), resolverFailed

	}

	extM := &schema.Extensions{TouchedUids: mutResp.GetMetrics().GetNumUids()[touchedUidsKey]}
	result := make(map[string]interface{})
	if req.Query != "" && len(mutResp.GetJson()) != 0 {
		if err := json.Unmarshal(mutResp.GetJson(), &result); err != nil {
			return emptyResult(
					schema.GQLWrapf(err, "Couldn't unmarshal response from Dgraph mutation")),
				resolverFailed
		}
	}

	var errs error
	dgQuery, err := mr.mutationRewriter.FromMutationResult(ctx, mutation, mutResp.GetUids(), result)
	errs = schema.AppendGQLErrs(errs, schema.GQLWrapf(err,
		"couldn't rewrite query for mutation %s", mutation.Name()))
	if dgQuery == nil && err != nil {
		return emptyResult(errs), resolverFailed
	}

	qryResp, err := mr.executor.Execute(ctx,
		&dgoapi.Request{Query: dgraph.AsString(dgQuery), ReadOnly: true})
	errs = schema.AppendGQLErrs(errs, schema.GQLWrapf(err,
		"couldn't rewrite query for mutation %s", mutation.Name()))

	extQ := &schema.Extensions{TouchedUids: qryResp.GetMetrics().GetNumUids()[touchedUidsKey]}

	numUidsField := mutation.NumUidsField()
	numUidsFieldRespName := schema.NumUid
	numUids := 0
	if numUidsField != nil {
		numUidsFieldRespName = numUidsField.ResponseName()
		numUids = getNumUids(mutation, mutResp.Uids, result)
	}

	// merge the extensions we got from Mutate and Query into extM
	if extM == nil {
		extM = extQ
	} else {
		extM.Merge(extQ)
	}

	resolved := completeDgraphResult(ctx, mutation.QueryField(), qryResp.GetJson(), errs)
	if resolved.Data == nil && resolved.Err != nil {
		return &Resolved{
			Data: map[string]interface{}{
				mutation.ResponseName(): map[string]interface{}{
					numUidsFieldRespName:                 numUids,
					mutation.QueryField().ResponseName(): nil,
				}},
			Field:      mutation,
			Err:        err,
			Extensions: extM,
		}, resolverSucceeded
	}

	if resolved.Data == nil {
		resolved.Data = map[string]interface{}{}
	}

	dgRes := resolved.Data.(map[string]interface{})
	dgRes[numUidsFieldRespName] = numUids
	resolved.Data = map[string]interface{}{mutation.ResponseName(): dgRes}
	resolved.Field = mutation
	resolved.Extensions = extM

	return resolved, resolverSucceeded
}

// deleteCompletion returns `{ "msg": "Deleted" }`
func deleteCompletion() CompletionFunc {
	return CompletionFunc(func(ctx context.Context, resolved *Resolved) {
		if fld, ok := resolved.Data.(map[string]interface{}); ok {
			if rsp, ok := fld[resolved.Field.ResponseName()].(map[string]interface{}); ok {
				rsp["msg"] = "Deleted"
			}
		}
	})
}
