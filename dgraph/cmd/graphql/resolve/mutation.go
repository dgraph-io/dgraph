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

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/dgraph"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
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
	mutation         schema.Mutation
	schema           schema.Schema
	mutationRewriter dgraph.MutationRewriter
	queryRewriter    dgraph.QueryRewriter
	dgraph           dgraph.Client
}

const (
	mutationFailed    = false
	mutationSucceeded = true
)

// resolve a single mutation, returning the result of resolving the mutation and
// a bool where true indicates that the mutation itself succeeded and false indicates
// that some error prevented the actual mutation.
func (mr *mutationResolver) resolve(ctx context.Context) (*resolved, bool) {

	var res *resolved
	var mutationSucceeded bool
	switch mr.mutation.MutationType() {
	case schema.AddMutation:
		res, mutationSucceeded = mr.resolveMutation(ctx)
	case schema.DeleteMutation:
		res, mutationSucceeded = mr.resolveDeleteMutation(ctx)
	case schema.UpdateMutation:
		// TODO: this should typecheck the input before resolving (like delete does)
		res, mutationSucceeded = mr.resolveMutation(ctx)
	default:
		return &resolved{
			err: errors.Errorf(
				"Mutation %s was not executed because only add, delete and update "+
					"mutations are implemented.", mr.mutation.Name())}, mutationFailed
	}

	// Mutations should have an extra element to their result path.  Because the mutation
	// always looks like `addFoo(...) { foo { ... } }` and what's resolved above
	// is the `foo { ... }` part, so both the result and any error paths need a prefix
	// of `addFoo` added.

	// Add prefix to result
	var b bytes.Buffer
	b.WriteString("\"")
	b.WriteString(mr.mutation.ResponseName())
	b.WriteString(`": `)
	if len(res.data) > 0 {
		b.Write(res.data)
	} else {
		b.WriteString("null")
	}
	res.data = b.Bytes()

	// Add prefix to all error paths
	resErrs := schema.AsGQLErrors(res.err)
	for _, err := range resErrs {
		if len(err.Path) > 0 {
			err.Path = append([]interface{}{mr.mutation.ResponseName()}, err.Path...)
		}
	}
	res.err = resErrs

	return res, mutationSucceeded
}

func (mr *mutationResolver) resolveMutation(ctx context.Context) (*resolved, bool) {
	res := &resolved{}
	span := otrace.FromContext(ctx)
	stop := x.SpanTimer(span, "resolveMutation")
	defer stop()
	if span != nil {
		span.Annotatef(nil, "mutation alias: [%s] type: [%s]", mr.mutation.Alias(),
			mr.mutation.MutationType())
	}

	mut, err := mr.mutationRewriter.Rewrite(mr.mutation)
	if err != nil {
		res.err = schema.GQLWrapf(err, "couldn't rewrite mutation %s", mr.mutation.Name())
		return res, mutationFailed
	}

	assigned, err := mr.dgraph.Mutate(ctx, mut)
	if err != nil {
		res.err = schema.GQLWrapLocationf(
			err, mr.mutation.Location(), "mutation %s failed", mr.mutation.Name())
		return res, mutationFailed
	}

	dgQuery, err := mr.queryRewriter.FromMutationResult(mr.mutation, assigned)
	if err != nil {
		res.err = schema.GQLWrapf(err, "couldn't rewrite query for mutation %s",
			mr.mutation.Name())
		return res, mutationSucceeded
	}

	resp, err := mr.dgraph.Query(ctx, dgQuery)
	if err != nil {
		res.err = schema.GQLWrapf(err, "mutation %s succeeded but query failed",
			mr.mutation.Name())
		return res, mutationSucceeded
	}

	res.data, res.err = completeDgraphResult(ctx, mr.mutation.QueryField(), resp)
	return res, mutationSucceeded
}

func (mr *mutationResolver) resolveDeleteMutation(ctx context.Context) (*resolved, bool) {
	res := &resolved{}
	span := otrace.FromContext(ctx)
	stop := x.SpanTimer(span, "resolveDeleteMutation")
	defer stop()
	if span != nil {
		span.Annotatef(nil, "mutation alias: [%s] type: [%s]", mr.mutation.Alias(),
			mr.mutation.MutationType())
	}

	query, mut, err := mr.mutationRewriter.RewriteDelete(mr.mutation)
	if err != nil {
		res.err = schema.GQLWrapf(err, "couldn't rewrite mutation %s", mr.mutation.Name())
		return res, mutationFailed
	}

	if err = mr.dgraph.DeleteNodes(ctx, query, mut); err != nil {
		res.err = schema.GQLWrapf(err, "mutation %s failed", mr.mutation.Name())
		return res, mutationFailed
	}

	res.data = []byte(`{ "msg": "Deleted" }`)
	return res, mutationSucceeded
}
