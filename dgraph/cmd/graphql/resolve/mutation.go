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

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
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
