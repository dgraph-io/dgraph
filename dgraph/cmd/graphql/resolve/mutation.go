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
	"strconv"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgo/protos/api"
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

const (
	createdNode = "newnode"
)

func (r *RequestResolver) resolveMutation(m schema.Mutation) {
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

	if r.resp.Errors != nil {
		r.WithErrors(
			gqlerror.Errorf("mutation %s not executed because of previous error", m.Name()))
		return
	}

	// only addT(input: TInput) mutations are supported ATM

	val, err := m.ArgValue(inputArgName)
	if err != nil {
		r.WithErrors(
			gqlerror.Errorf("couldn't read input argument in mutation %s : %s", m.Name(), err))
		return
	}

	jsonMut, err := json.Marshal(buildMutationJSON(m, val))
	glog.V(2).Infof("Generated Dgraph mutation for %s: \n%s\n", m.Name(), jsonMut)
	if err != nil {
		r.WithErrors(gqlerror.Errorf("couldn't marshal mutation for %s : %s", m.Name(), err))
		return
	}
	mu := &api.Mutation{
		CommitNow: true,
		SetJson:   jsonMut,
	}

	ctx := context.Background()
	assigned, err := r.dgraphClient.NewTxn().Mutate(ctx, mu)
	if err != nil {
		r.WithErrors(gqlerror.Errorf("couldn't execute mutation for %s : %s", m.Name(), err))
		return
	}

	uid, err := strconv.ParseUint(assigned.Uids[createdNode], 0, 64)
	if err != nil {
		// FIXME:
		r.WithErrors(gqlerror.Errorf("couldn't execute mutation for %s : %s", m.Name(), err))
		return
	}

	// All our mutations currently have exactly 1 field
	f := m.SelectionSet()[0]
	qb := newQueryBuilder()
	qb.withAttr(f.ResponseName())
	qb.withUIDRoot(uid)
	qb.withSelectionSetFrom(f)

	gq, err := qb.query()
	if err != nil {
		r.WithErrors(
			gqlerror.Errorf("unable to query after mutation that created 0x%x in %s : %s",
				uid, m.Name(), err))
		return
	}

	res, err := executeQuery(gq, r.dgraphClient)
	if err != nil {
		r.WithErrors(gqlerror.Errorf("Failed to query dgraph with error : %s", err))
		glog.Infof("Dgraph query failed : %s", err) // maybe log more info if it could be a bug?
	}

	// what's the best way to append into the result []byte / json.RawMessage ??
	r.resp.Data.WriteRune('"')
	r.resp.Data.WriteString(m.ResponseName())
	r.resp.Data.WriteString(`": {`) // FIXME: what we write (list/object) depends on schema result type
	r.resp.Data.Write(res)
	r.resp.Data.WriteString("}\n")
}

func buildMutationJSON(f schema.Mutation, v interface{}) map[string]interface{} {
	mut := make(map[string]interface{})

	typeName := f.MutatedTypeName()
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
