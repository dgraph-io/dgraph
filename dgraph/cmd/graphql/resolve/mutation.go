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

// FIXME: triage this
//
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
	mutation         schema.Mutation
	queryRewriter    QueryRewriter
	mutationRewriter MutationRewriter
	queryExecutor    QueryExecutor
	mutationExecutor MutationExecutor
	resultCompleter  ResultCompleter
}

// a deleteCompleter returns `{ "msg": "Deleted" }`
// FIXME: after upsert mutations changes are done, it will return info about
// the result of a deletion.
type deleteCompleter struct{}

func (mr *mutationResolver) Resolve(ctx context.Context) (*Resolved, bool) {

	span := otrace.FromContext(ctx)
	stop := x.SpanTimer(span, "resolveMutation")
	defer stop()
	if span != nil {
		span.Annotatef(nil, "mutation alias: [%s] type: [%s]", mr.mutation.Alias(),
			mr.mutation.MutationType())
	}

	query, mutations, err := mr.mutationRewriter.Rewrite(mr.mutation)
	if err != nil {
		return &Resolved{
			Err: schema.GQLWrapf(err, "couldn't rewrite mutation %s", mr.mutation.Name()),
		}, resolverFailed
	}

	assigned, mutated, err := mr.mutationExecutor.Mutate(ctx, query, mutations)
	if err != nil {
		return &Resolved{
			Err: schema.GQLWrapLocationf(
				err, mr.mutation.Location(), "mutation %s failed", mr.mutation.Name()),
		}, resolverFailed
	}

	dgQuery, err := mr.queryRewriter.FromMutationResult(mr.mutation, assigned, mutated)
	if err != nil {
		return &Resolved{
			Err: schema.GQLWrapf(err, "couldn't rewrite query for mutation %s",
				mr.mutation.Name()),
		}, resolverFailed
	}

	resp, err := mr.queryExecutor.Query(ctx, dgQuery)
	if err != nil {
		return &Resolved{
			Err: schema.GQLWrapf(err, "mutation %s succeeded but query failed",
				mr.mutation.Name()),
		}, resolverSucceeded
	}

	completed, err := mr.resultCompleter.Complete(ctx, mr.mutation.QueryField(), resp)
	return &Resolved{
		Data: completed,
		Err:  err,
	}, resolverSucceeded
}

func (dc *deleteCompleter) Complete(ctx context.Context, field schema.Field, result []byte) ([]byte, error) {
	return []byte(`{ "msg": "Deleted" }`), nil
}
