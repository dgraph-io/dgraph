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

	dgoapi "github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/x"
	otrace "go.opencensus.io/trace"
)

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
	Rewrite(m schema.Mutation) (*gql.GraphQuery, []*dgoapi.Mutation, error)

	// FromMutationResult takes a GraphQL mutation and the results of a Dgraph
	// mutation and constructs a Dgraph query.  It's used to find the return
	// value from a GraphQL mutation - i.e. we've run the mutation indicated by m
	// now we need to query Dgraph to satisfy all the result fields in m.
	FromMutationResult(
		m schema.Mutation,
		assigned map[string]string,
		mutated map[string][]string) (*gql.GraphQuery, error)
}

// A MutationExecutor can execute a mutation and returns the assigned map, the
// mutated map and any errors.
type MutationExecutor interface {
	// Mutate performs the actual mutation and returns a map of newly assigned nodes,
	// a map of variable->[]uid from upsert mutations and any errors.  If an error
	// occurs, that indicates that the mutation failed in some way significant enough
	// way as to not continue procissing this mutation or others in the same request.
	Mutate(
		ctx context.Context,
		query *gql.GraphQuery,
		mutations []*dgoapi.Mutation) (map[string]string, map[string][]string, error)
}

// MutationResolverFunc is an adapter that allows to build a MutationResolver from
// a function.  Based on the http.HandlerFunc pattern.
type MutationResolverFunc func(ctx context.Context, mutation schema.Mutation) (*Resolved, bool)

// MutationExecutionFunc is an adapter that allows us to compose mutation execution and build a
// MutationExecuter from a function.  Based on the http.HandlerFunc pattern.
type MutationExecutionFunc func(
	ctx context.Context,
	query *gql.GraphQuery,
	mutations []*dgoapi.Mutation) (map[string]string, map[string][]string, error)

// Resolve calls mr(ctx, mutation)
func (mr MutationResolverFunc) Resolve(
	ctx context.Context,
	mutation schema.Mutation) (*Resolved, bool) {

	return mr(ctx, mutation)
}

// Mutate calls me(ctx, query, mutations)
func (me MutationExecutionFunc) Mutate(
	ctx context.Context,
	query *gql.GraphQuery,
	mutations []*dgoapi.Mutation) (map[string]string, map[string][]string, error) {
	return me(ctx, query, mutations)
}

// NewMutationResolver creates a new mutation resolver.  The resolver runs the pipeline:
// 1) rewrite the mutation using mr (return error if failed)
// 2) execute the mutation with me (return error if failed)
// 3) write a query for the mutation with mr (return error if failed)
// 4) execute the query with qe (return error if failed)
// 5) process the result with rc
func NewMutationResolver(
	mr MutationRewriter,
	qe QueryExecutor,
	me MutationExecutor,
	rc ResultCompleter) MutationResolver {
	return &mutationResolver{
		mutationRewriter: mr,
		queryExecutor:    qe,
		mutationExecutor: me,
		resultCompleter:  rc,
	}
}

// mutationResolver can resolve a single GraphQL mutation field
type mutationResolver struct {
	mutationRewriter MutationRewriter
	queryExecutor    QueryExecutor
	mutationExecutor MutationExecutor
	resultCompleter  ResultCompleter
}

func (mr *mutationResolver) Resolve(
	ctx context.Context, mutation schema.Mutation) (*Resolved, bool) {

	span := otrace.FromContext(ctx)
	stop := x.SpanTimer(span, "resolveMutation")
	defer stop()
	if span != nil {
		span.Annotatef(nil, "mutation alias: [%s] type: [%s]", mutation.Alias(),
			mutation.MutationType())
	}

	res, success, err := mr.rewriteAndExecute(ctx, mutation)

	completed, err := mr.resultCompleter.Complete(ctx, mutation.QueryField(), res, err)
	return &Resolved{
		Data: completed,
		Err:  err,
	}, success
}

func (mr *mutationResolver) rewriteAndExecute(
	ctx context.Context, mutation schema.Mutation) ([]byte, bool, error) {

	query, mutations, err := mr.mutationRewriter.Rewrite(mutation)
	if err != nil {
		return nil, resolverFailed,
			schema.GQLWrapf(err, "couldn't rewrite mutation %s", mutation.Name())
	}

	assigned, mutated, err := mr.mutationExecutor.Mutate(ctx, query, mutations)
	if err != nil {
		return nil, resolverFailed,
			schema.GQLWrapLocationf(err, mutation.Location(), "mutation %s failed", mutation.Name())
	}

	dgQuery, err := mr.mutationRewriter.FromMutationResult(mutation, assigned, mutated)
	if err != nil {
		return nil, resolverFailed,
			schema.GQLWrapf(err, "couldn't rewrite query for mutation %s", mutation.Name())
	}

	resp, err := mr.queryExecutor.Query(ctx, dgQuery)

	return resp, resolverSucceeded,
		schema.GQLWrapf(err, "mutation %s succeeded but query failed", mutation.Name())
}

// deleteCompletion returns `{ "msg": "Deleted" }`
// FIXME: after upsert mutations changes are done, it will return info about
// the result of a deletion.
func deleteCompletion() CompletionFunc {
	return CompletionFunc(func(
		ctx context.Context, field schema.Field, result []byte, err error) ([]byte, error) {

		return []byte(`{ "msg": "Deleted" }`), err
	})
}
