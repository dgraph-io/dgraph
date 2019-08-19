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
	"fmt"
	"strconv"

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/api"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/dgraph"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/vektah/gqlparser/gqlerror"
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
	mutation schema.Mutation
	schema   schema.Schema
	dgraph   dgraph.Client
}

const (
	createdNode = "newnode"
)

// resolve a single mutation.
func (mr *mutationResolver) resolve(ctx context.Context) *resolved {
	// A mutation operation can contain any number of mutation fields.  Those should be executed
	// serially.
	// (spec https://graphql.github.io/graphql-spec/June2018/#sec-Normal-and-Serial-Execution)
	//
	// The spec is ambigous about what to do in the case of errors during that serial execution
	// - apparently deliberatly so; see this comment from Lee Byron:
	// https://github.com/graphql/graphql-spec/issues/277#issuecomment-385588590
	// and clarification
	// https://github.com/graphql/graphql-spec/pull/438
	//
	// A reasonable interpretation of that is to stop a list of mutations after the first error -
	// which seems like the natural semantics and is what we enforce here.
	//
	// What we aren't following the exact semantics for is the error propagation.
	// According to the spec
	// https://graphql.github.io/graphql-spec/June2018/#sec-Executing-Selection-Sets,
	// https://graphql.github.io/graphql-spec/June2018/#sec-Errors-and-Non-Nullability
	// and the commentry here:
	// https://github.com/graphql/graphql-spec/issues/277
	//
	// If we had a schema with:
	//
	// type Mutation {
	// 	 push(val: Int!): Int!
	// }
	//
	// and then ran operation:
	//
	//  mutation {
	// 	  one: push(val: 1)
	// 	  thirteen: push(val: 13)
	// 	  two: push(val: 2)
	//  }
	//
	// if `push(val: 13)` fails with an error, then only errors should be returned from the whole
	// mutation` - because the result value is ! and one of them failed, the error should propagate
	// to the entire operation. That is, even though `push(val: 1)` succeeded and we already
	// calculated its result value, we should squash that and return null data and an error.
	// (nothing in GraphQL says where any transaction or persistence boundries lie)
	//
	// We aren't doing that below - we aren't even inspecting if the result type is !.  For now,
	// we'll return any data we've already calculated and following errors.  However:
	// TODO: we should be picking through all results and propagating errors according to spec
	// TODO: and, we should have all mutation return types not have ! so we avoid the above

	var res *resolved
	switch mr.mutation.MutationType() {
	case schema.AddMutation:
		res = mr.resolveAddMutation(ctx)
	case schema.DeleteMutation:
		res = mr.resolveDeleteMutation(ctx)
	case schema.UpdateMutation:
		res = mr.resolveUpdateMutation(ctx)
	default:
		return &resolved{
			err: gqlerror.Errorf(
				"[%s] Only add, delete and update mutations are implemented", api.RequestID(ctx))}
	}

	if len(res.data) > 0 {
		var b bytes.Buffer
		b.WriteRune('"')
		b.WriteString(mr.mutation.ResponseName())
		b.WriteString(`": `)
		b.Write(res.data)

		res.data = b.Bytes()
	}

	return res
}

func (mr *mutationResolver) resolveAddMutation(ctx context.Context) *resolved {
	res := &resolved{}
	val := mr.mutation.ArgValue(schema.InputArgName)

	assigned, err := mr.dgraph.Mutate(ctx, buildMutationJSON(mr.mutation, val))
	if err != nil {
		res.err = schema.GQLWrapf(err,
			"[%s] mutation %s failed", api.RequestID(ctx), mr.mutation.Name())
		return res
	}

	uid, err := strconv.ParseUint(assigned[createdNode], 0, 64)
	if err != nil {
		res.err = schema.GQLWrapf(err,
			"[%s] recieved assigned from Dgraph, but couldn't parse as uint64",
			api.RequestID(ctx))
		return res
	}

	// All our mutations currently have exactly 1 field
	f := mr.mutation.SelectionSet()[0]
	qb := dgraph.NewQueryBuilder().
		WithAttr(f.ResponseName()).
		WithUIDRoot(uid).
		WithSelectionSetFrom(f)

	resp, err := mr.dgraph.Query(ctx, qb)
	if err != nil {
		res.err = schema.GQLWrapf(err, "[%s] mutation %s created node 0x%x but query failed",
			api.RequestID(ctx), mr.mutation.Name(), uid)
		return res
	}

	completed, err := completeDgraphResult(ctx, f, resp)
	res.data = completed
	res.err = err
	return res
}

func (mr *mutationResolver) resolveDeleteMutation(ctx context.Context) *resolved {
	res := &resolved{}
	uid, err := mr.mutation.IDArgValue()
	if err != nil {
		res.err = schema.GQLWrapf(err, "[%s] couldn't read ID argument in mutation %s",
			api.RequestID(ctx), mr.mutation.Name())
		return res
	}

	err = mr.dgraph.AssertType(ctx, uid, mr.mutation.MutatedTypeName())
	if err != nil {
		return &resolved{
			err: schema.GQLWrapf(err, "[%s] couldn't complete %s",
				api.RequestID(ctx), mr.mutation.Name())}
	}

	err = mr.dgraph.DeleteNode(ctx, uid)
	if err != nil {
		res.err = schema.GQLWrapf(err, "[%s] couldn't complete %s",
			api.RequestID(ctx), mr.mutation.Name())
		// FIXME: ^^ also add the GraphQL path etc to link properly to the operation
		return res
	}

	res.data = []byte(`{ "msg": "Deleted" }`)
	return res
}

func (mr *mutationResolver) resolveUpdateMutation(ctx context.Context) *resolved {
	res := &resolved{}
	val := mr.mutation.ArgValue(schema.InputArgName)

	uid, err := mr.mutation.IDArgValue()
	if err != nil {
		res.err = schema.GQLWrapf(err,
			"[%s] couldn't read id argument in mutation", api.RequestID(ctx))
		return res
	}

	// We'll need to do better than this once there's deepper mutations
	mut := buildMutationJSON(mr.mutation, val)
	mut["uid"] = fmt.Sprintf("0x%x", uid)
	_, err = mr.dgraph.Mutate(ctx, mut)
	if err != nil {
		res.err = schema.GQLWrapf(err,
			"[%s] couldn't run mutation mutation", api.RequestID(ctx))
		return res
	}

	// All our mutations currently have exactly 1 field
	f := mr.mutation.SelectionSet()[0]
	qb := dgraph.NewQueryBuilder().
		WithAttr(f.ResponseName()).
		WithUIDRoot(uid).
		WithSelectionSetFrom(f)

	resp, err := mr.dgraph.Query(ctx, qb)
	if err != nil {
		res.err = schema.GQLWrapf(err,
			"[%s] mutation %s updated node 0x%x but query failed",
			api.RequestID(ctx), mr.mutation.Name(), uid)
		return res
	}

	completed, err := completeDgraphResult(ctx, f, resp)
	res.data = completed
	res.err = err
	return res
}

func buildMutationJSON(m schema.Mutation, v interface{}) map[string]interface{} {
	mut := make(map[string]interface{})

	typeName := m.MutatedTypeName()
	mut["uid"] = "_:" + createdNode
	mut["dgraph.type"] = typeName

	switch vv := v.(type) {
	case map[string]interface{}:
		for key, val := range vv {
			mut[typeName+"."+key] = val
		}
	default:
		// when we do bulk mutations, there will be [] in here
		//
		// also not sure about mutations that contain raw values yet
		// ... I think no
	}

	// When adding references to existing nodes with TRef inputs,
	// this'll also need to transform those to be 'uid'

	return mut
}
