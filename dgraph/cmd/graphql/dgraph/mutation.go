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

package dgraph

import (
	"fmt"
	"strconv"

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/vektah/gqlparser/gqlerror"
)

const (
	createdNode = "newnode"
)

// MutationRewriter can transform a GraphQL mutation into a Dgraph JSON mutation.
type MutationRewriter interface {
	Rewrite(m schema.Mutation) (interface{}, error)
}

type mutationRewriter struct{}

// NewMutationRewriter returns new mutation rewriter.
func NewMutationRewriter() MutationRewriter {
	return &mutationRewriter{}
}

// Rewrite takes a GraphQL schema.Mutation and rewrites it to a struct representing
// that mutation that's ready to be marshalled as a Dgraph JSON mutation.  As per
// the schemas generated by 'dgraph graphql init ...`, schema.Mutation m
// must have a single argument called 'input' that carries the mutation data.
//
// That argument could have been passed in the mutation like:
//
// addPost(input: { title: "...", ... })
//
// or be passed in a GraphQL variable like:
//
// addPost(input: $newPost)
//
// Either way, the data needs to have type information added and have some rewriting
// done - for example, rewriting field names from the GraphQL view to what's stored
// in Dgraph, and rewriting ID fields from their names to uid.
//
// For example, a GraphQL add mutation to add an object of type Author,
// with GraphQL input object:
//
// { name: "A.N. Author", country: { id: "0x123" }, posts: [] }
//
// becomes a Dgraph JSON mutation:
//
// { "uid":"_:newnode",
//   "dgraph.type":"Author",
//   "Author.name":"A.N. Author",
//   "Author.country": { "uid": "0x123" },
//   "Author.posts":[]
// }
func (mrw *mutationRewriter) Rewrite(m schema.Mutation) (interface{}, error) {

	if m.MutationType() != schema.AddMutation &&
		m.MutationType() != schema.UpdateMutation {
		return nil,
			gqlerror.Errorf(
				"internal error - call to build Dgraph mutation for %s mutation type",
				m.MutationType())
	}

	// ATM mutations aren't very deep.  At worst, a mutation can be one object with
	// reference to other existing objects. e.g. like:
	//
	// { "title": "...", author: { "id": "0x123" }, ... }
	//
	// or, if the schema says a post can have multiple authors
	//
	// { "title": "...", authors: [ { "id": "0x123" }, { "id": "0x321" }, ...] }
	//
	// Later, that'll turn into deeper mutations, so there will be much more to
	// dig through.

	mutatedType := m.MutatedType()

	// ArgValue resolves the argument regardless of if it was passed in the request
	// as an argument or in a GraphQL variable.  All Input arguments are non-nullable
	// so GraphQL validation will ensure that val isn't nil here.  It's also either
	// from a json list or object (not a single value).
	val := m.ArgValue(schema.InputArgName)

	switch val := val.(type) {
	case map[string]interface{}:
		var srcUID string
		if m.MutationType() == schema.AddMutation {
			srcUID = "_:" + createdNode
		} else {
			// must be schema.UpdateMutation, so the mutation payload came in like
			// { id: 0x123, patch { ... the actual changes ... } }
			// it's that "patch" object that needs to be built into the mutation.
			//
			// Patch can't be nil, schema gen builds updates with inputs like
			// input UpdateAuthorInput {
			// 	id: ID!
			// 	patch: PatchAuthor!
			// }
			// If patch were nil, validation would have failed.
			val = val["patch"].(map[string]interface{})

			uid, err := getUpdUID(m)
			if err != nil {
				return nil, err
			}
			srcUID = fmt.Sprintf("%#x", uid)
		}

		res, err := rewriteObject(mutatedType, nil, srcUID, val)
		if err != nil {
			return nil, err
		}

		res["uid"] = srcUID
		if m.MutationType() == schema.AddMutation {
			res["dgraph.type"] = mutatedType.Name()
		}

		return res, nil
	case []interface{}:
		// TODO: we don't yet have a bulk mutation that takes a list of mutations
		// like
		// addManyPost(input: [{...post1...}, {...post2...}, ...])
		// That'll need to rewrite all of those mutations and keep track of
		// adding blank nodes for every new object, so we can look them up in the
		// later query
		//
		// GraphQL validation means we shouldn't get here till those bulk mutations
		// are added to the schema.
		return nil,
			gqlerror.Errorf(
				"internal error - call to build a list of mutations, but that " +
					"isn't supported yet.")
	}

	return nil,
		gqlerror.Errorf(
			"internal error - call to build Dgraph mutation with input of unrecognized type; " +
				"this indicates a GraphQL validation error.")
}

func getUpdUID(m schema.Mutation) (uint64, error) {
	val := m.ArgValue(schema.InputArgName).(map[string]interface{})
	idArg := val[m.MutatedType().IDField().Name()]

	return asUID(idArg)
}

func asUID(val interface{}) (uint64, error) {
	id, ok := val.(string)
	uid, err := strconv.ParseUint(id, 0, 64)

	if !ok || err != nil {
		return 0, gqlerror.Errorf(
			"ID argument (%s) was not able to be parsed", id)
	}

	return uid, nil
}

// We are processing a mutation and got to an object obj like
//
// { "title": "...", author: { "id": "0x123" }, ... }
//
// here we process that object as being of type typ.  If we got here
// as a nested object like
//
// { "blaa" : { "title" ... }}
//
// then scrField is the field of that enclosing object and srcUID is the uid
// of that node (either a blank node or the actual uid).  If this is the top level
// of the mutation, then scrField will be nil and srcUID will be the top level
// uid or blank node of the mutation.
func rewriteObject(
	typ schema.Type,
	srcField schema.FieldDefinition,
	srcUID string,
	obj map[string]interface{}) (map[string]interface{}, error) {

	// GraphQL validation has already ensured that the types of arguments
	// (or variables) are correct and has ensured that non-nullables are
	// not null.  If this is an add mutation, that's all that's needed.
	// But an update mutation doesn't set the same restrictions:
	// e.g. a Post might have `title: String!`` in the schema, but, you
	// don't need to update the title everytime you make a post update,
	// so the input type for an update mutation might have `title: String`.
	//
	// That means a Post update could set a non-nullable field to null.
	// ATM we allow this and it'll just triggers GraphQL error propagation
	// when that is in a query result.  This is the same case as deletes:
	// e.g. deleting an author might make the `author: Author!` field of
	// a bunch of Posts invalid.
	// (That might actually be helpful if you want to run one
	// mutation to remove something and then another to correct it.)
	//
	// The long term plan is to allow the server to run in different modes.
	// Or even allow mutations to run in different modes.  One (like above)
	// will allow this breaking of referential integrity, other modes
	// might enforce it an reject a mutation if it breaks, or even might
	// have some sort of cascading delete or for scalars allow 'deleting'
	// to set to a default value.

	result := make(map[string]interface{}, len(obj))

	for field, val := range obj {
		var res interface{}
		var err error
		fieldName := fmt.Sprintf("%s.%s", typ.Name(), field)

		fieldDef := typ.Field(field)

		switch val := val.(type) {
		case map[string]interface{}:
			// TODO: Because mutations are just one level deep, we don't have to
			// reset the srcUID.  Once they are deeper, we first have to look up
			// the current UID and then set to that.
			res, err = rewriteObject(fieldDef.Type(), fieldDef, srcUID, val)
		case []interface{}:
			// This field is either a list of objects
			// { "title": "...", "authors": [ { "id": "0x123" }, { "id": "0x321" }, ...] }
			//          like here ^^
			// or it's a list of scalars - e.g. if schema said `scores: [Float]`
			// { "title": "...", "scores": [10.5, 9.3, ... ]
			//          like here ^^
			res, err = rewriteList(fieldDef.Type(), fieldDef, srcUID, val)
		default:
			// scalar value
			res = val

			if fieldDef.IsID() {
				fieldName = "uid"

				_, err := asUID(val)
				if err != nil {
					return nil, err
				}

				// If the mutation contains a reference to another node, e.g it was
				// say adding a post with:
				// { "title": "...", "author": { "id": "0x123" }, ... }
				// and we've gotten into here  ^^^^
				// and the schema says that Post.author and Author.Posts are inverses
				// of each other, then we need to make sure the inverse link is
				// added.  We have to make sure it ends up like
				//
				// { "title": "...", "author": { "id": "0x123", "posts": { "uid": ... } }, ... }
				//
				// We already know that "posts" isn't going to be in val because the
				// schema rules say that a reference to another type in an add/update
				// must either be just an object reference like
				//
				// "author": { "id": "0x123" }
				//
				// or must be a whole new node (<- TODO: that case isn't in schema yet)
				if srcField != nil {
					invType, invField := srcField.Inverse()
					if invType != nil && invField != nil {
						result[fmt.Sprintf("%s.%s", invType.Name(), invField.Name())] =
							map[string]interface{}{"uid": srcUID}
					}
				}
			}
		}
		if err != nil {
			return nil, err
		}
		result[fieldName] = res
	}

	return result, nil
}

func rewriteList(
	typ schema.Type,
	srcField schema.FieldDefinition,
	srcUID string,
	objects []interface{}) ([]interface{}, error) {

	result := make([]interface{}, len(objects))

	for i, obj := range objects {
		switch obj := obj.(type) {
		case map[string]interface{}:
			res, err := rewriteObject(typ, srcField, srcUID, obj)
			if err != nil {
				return nil, err
			}
			result[i] = res
		default:
			// scalar value - can't be a list because lists of lists aren't allowed
			result[i] = obj
		}
	}

	return result, nil
}
