/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	dgoapi "github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/dql"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/x"
)

const (
	MutationQueryVar        = "x"
	MutationQueryVarUID     = "uid(x)"
	updateMutationCondition = `gt(len(x), 0)`
)

// Enum passed on to rewriteObject function.
type MutationType int

const (
	// Add Mutation
	Add MutationType = iota
	// Add Mutation with Upsert
	AddWithUpsert
	// Update Mutation used for to setting new nodes, edges.
	UpdateWithSet
	// Update Mutation used for removing edges.
	UpdateWithRemove
)

type Rewriter struct {
	// VarGen is the VariableGenerator used accross RewriteQueries and Rewrite functions
	// for Mutation. It generates unique variable names for DQL queries and mutations.
	VarGen *VariableGenerator
	// XidMetadata stores data like seenUIDs and variableObjMap to be used across Rewrite
	// and RewriteQueries functions for Mutations.
	XidMetadata *xidMetadata
}

type AddRewriter struct {
	frags []*mutationFragment
	Rewriter
}
type UpdateRewriter struct {
	setFrag *mutationFragment
	delFrag *mutationFragment
	Rewriter
}
type deleteRewriter struct {
	Rewriter
}

// A mutationFragment is a partially built Dgraph mutation.  Given a GraphQL
// mutation input, we traverse the input data and build a Dgraph mutation.  That
// mutation might require queries (e.g. to check types), conditions (to guard the
// upsert mutation to only run in the right conditions), post mutation checks (
// so we can investigate the mutation result and know what guarded mutations
// actually ran.
//
// In the case of XIDs a mutation might result in two fragments - one for the case
// of add a new object for the XID and another for link to an existing XID, depending
// on what condition evaluates to true in the upsert.
type mutationFragment struct {
	queries    []*dql.GraphQuery
	conditions []string
	fragment   interface{}
	deletes    []interface{}
	check      resultChecker
	newNodes   map[string]schema.Type
}

// xidMetadata is used to handle cases where we get multiple objects which have same xid value in a
// single mutation
type xidMetadata struct {
	// variableObjMap stores the mapping of xidVariable -> the input object which contains that xid
	variableObjMap map[string]map[string]interface{}
	// seenAtTopLevel tells whether the xidVariable has been previously seen at top level or not
	seenAtTopLevel map[string]bool
	// seenUIDs tells whether the UID is previously been seen during DFS traversal
	seenUIDs map[string]bool
}

// A mutationBuilder can build a json mutation []byte from a mutationFragment
type mutationBuilder func(frag *mutationFragment) ([]byte, error)

// A resultChecker checks an upsert (query) result and returns an error if the
// result indicates that the upsert didn't succeed.
type resultChecker func(map[string]interface{}) error

// A VariableGenerator generates unique variable names.
type VariableGenerator struct {
	counter       int
	xidVarNameMap map[string]string
}

func NewVariableGenerator() *VariableGenerator {
	return &VariableGenerator{
		counter:       0,
		xidVarNameMap: make(map[string]string),
	}
}

// Next gets the Next variable name for the given type and xid.
// So, if two objects of the same type have same value for xid field,
// then they will get same variable name.
func (v *VariableGenerator) Next(typ schema.Type, xidName, xidVal string, auth bool) string {
	// return previously allocated variable name for repeating xidVal
	var key string
	if xidName == "" || xidVal == "" {
		key = typ.Name()
	} else {
		// We add "." between values while generating key to removes duplicate xidError from below type of cases
		// mutation {
		//  addABC(input: [{ ab: "cd", abc: "d" }]) {
		//    aBC {
		//      ab
		//      abc
		//    }
		//   }
		// }
		// The two generated keys for this case will be
		// ABC.ab.cd and ABC.abc.d
		// It also ensures that xids from different types gets different variable names
		// here we are using the assertion that field name or type name can't have "." in them
		key = typ.FieldOriginatedFrom(xidName) + "." + xidName + "." + xidVal
	}

	if varName, ok := v.xidVarNameMap[key]; ok {
		return varName
	}

	// create new variable name
	v.counter++
	var varName string
	if auth {
		varName = fmt.Sprintf("%s_Auth%v", typ.Name(), v.counter)
	} else {
		varName = fmt.Sprintf("%s_%v", typ.Name(), v.counter)
	}

	// save it, if it was created for xidVal
	if xidName != "" && xidVal != "" {
		v.xidVarNameMap[key] = varName
	}

	return varName
}

// NewAddRewriter returns new MutationRewriter for add & update mutations.
func NewAddRewriter() MutationRewriter {
	return &AddRewriter{}
}

// NewUpdateRewriter returns new MutationRewriter for add & update mutations.
func NewUpdateRewriter() MutationRewriter {
	return &UpdateRewriter{}
}

// NewDeleteRewriter returns new MutationRewriter for delete mutations..
func NewDeleteRewriter() MutationRewriter {
	return &deleteRewriter{}
}

// NewXidMetadata returns a new empty *xidMetadata for storing the metadata.
func NewXidMetadata() *xidMetadata {
	return &xidMetadata{
		variableObjMap: make(map[string]map[string]interface{}),
		seenAtTopLevel: make(map[string]bool),
		seenUIDs:       make(map[string]bool),
	}
}

// isDuplicateXid returns true if:
//  1. we are at top level and this xid has already been seen at top level, OR
//  2. we are in a deep mutation and:
//     a. this newXidObj has a field which is inverse of srcField and that
//     invField is not of List type, OR
//     b. newXidObj has some values other than xid and isn't equal to existingXidObject
//
// It is used in places where we don't want to allow duplicates.
func (xidMetadata *xidMetadata) isDuplicateXid(atTopLevel bool, xidVar string,
	newXidObj map[string]interface{}, srcField schema.FieldDefinition) bool {
	if atTopLevel && xidMetadata.seenAtTopLevel[xidVar] {
		return true
	}

	if srcField != nil {
		invField := srcField.Inverse()
		if invField != nil && invField.Type().ListType() == nil {
			return true
		}
	}

	// We return an error if both occurrences of xid contain more than one values
	// and are not equal.
	// XID should be defined with all its values at one of the places and references with its
	// XID from other places.
	if len(newXidObj) > 1 && len(xidMetadata.variableObjMap[xidVar]) > 1 &&
		!reflect.DeepEqual(xidMetadata.variableObjMap[xidVar], newXidObj) {
		return true
	}

	return false
}

// RewriteQueries takes a GraphQL schema.Mutation add and creates queries to find out if
// referenced nodes by XID and UID exist or not.
// m must have a single argument called 'input' that carries the mutation data.
//
// For example, a GraphQL add mutation to add an object of type Author,
// with GraphQL input object (where country code is @id)
//
//	{
//	  name: "A.N. Author",
//	  country: { code: "ind", name: "India" },
//	  posts: [ { title: "A Post", text: "Some text" }]
//	  friends: [ { id: "0x123" } ]
//	}
//
// The following queries would be generated
//
//	query {
//	  Country2(func: eq(Country.code, "ind")) @filter(type: Country) {
//	    uid
//	  }
//	  Person3(func: uid(0x123)) @filter(type: Person) {
//	    uid
//	  }
//	}
//
// This query will be executed and depending on the result it would be decided whether
// to create a new country as part of this mutation or link it to an existing country.
// If it is found out that there is an existing country, no modifications are made to
// the country's attributes and its children. Mutations of the country's children are
// simply ignored.
// If it is found out that the Person with id 0x123 does not exist, the corresponding
// mutation will fail.
func (arw *AddRewriter) RewriteQueries(
	ctx context.Context,
	m schema.Mutation) ([]*dql.GraphQuery, []string, error) {

	arw.VarGen = NewVariableGenerator()
	arw.XidMetadata = NewXidMetadata()

	mutatedType := m.MutatedType()
	val, _ := m.ArgValue(schema.InputArgName).([]interface{})

	var ret []*dql.GraphQuery
	var retTypes []string
	var retErrors error

	for _, i := range val {
		obj := i.(map[string]interface{})
		queries, typs, errs := existenceQueries(ctx, mutatedType, nil, arw.VarGen, obj, arw.XidMetadata)
		if len(errs) > 0 {
			var gqlErrors x.GqlErrorList
			for _, err := range errs {
				gqlErrors = append(gqlErrors, schema.AsGQLErrors(err)...)
			}
			retErrors = schema.AppendGQLErrs(retErrors, schema.GQLWrapf(gqlErrors,
				"failed to rewrite mutation payload"))
		}
		ret = append(ret, queries...)
		retTypes = append(retTypes, typs...)
	}
	return ret, retTypes, retErrors
}

// RewriteQueries creates and rewrites set and remove update patches queries.
// The GraphQL updates look like:
//
//	input UpdateAuthorInput {
//		filter: AuthorFilter!
//		set: PatchAuthor
//		remove: PatchAuthor
//	}
//
// which gets rewritten in to a DQL queries to check if
// - referenced UIDs and XIDs in set and remove exist or not.
//
// Depending on the result of these executed queries, it is then decided whether to
// create new nodes or link to existing ones.
//
// Note that queries rewritten using RewriteQueries don't include UIDs or XIDs referenced
// as part of filter argument.
//
// See AddRewriter for how the rewritten queries look like.
func (urw *UpdateRewriter) RewriteQueries(
	ctx context.Context,
	m schema.Mutation) ([]*dql.GraphQuery, []string, error) {
	mutatedType := m.MutatedType()

	urw.VarGen = NewVariableGenerator()
	urw.XidMetadata = NewXidMetadata()

	inp := m.ArgValue(schema.InputArgName).(map[string]interface{})
	setArg := inp["set"]
	delArg := inp["remove"]

	var ret []*dql.GraphQuery
	var retTypes []string
	var retErrors error

	// Write existence queries for set
	if setArg != nil {
		obj := setArg.(map[string]interface{})
		if len(obj) != 0 {
			queries, typs, errs := existenceQueries(ctx, mutatedType, nil, urw.VarGen, obj, urw.XidMetadata)
			if len(errs) > 0 {
				var gqlErrors x.GqlErrorList
				for _, err := range errs {
					gqlErrors = append(gqlErrors, schema.AsGQLErrors(err)...)
				}
				retErrors = schema.AppendGQLErrs(retErrors, schema.GQLWrapf(gqlErrors,
					"failed to rewrite mutation payload"))
			}
			ret = append(ret, queries...)
			retTypes = append(retTypes, typs...)
		}
	}

	// Write existence queries for remove
	if delArg != nil {
		obj := delArg.(map[string]interface{})
		if len(obj) != 0 {
			queries, typs, errs := existenceQueries(ctx, mutatedType, nil, urw.VarGen, obj, urw.XidMetadata)
			if len(errs) > 0 {
				var gqlErrors x.GqlErrorList
				for _, err := range errs {
					gqlErrors = append(gqlErrors, schema.AsGQLErrors(err)...)
				}
				retErrors = schema.AppendGQLErrs(retErrors, schema.GQLWrapf(gqlErrors,
					"failed to rewrite mutation payload"))
			}
			ret = append(ret, queries...)
			retTypes = append(retTypes, typs...)
		}
	}
	return ret, retTypes, retErrors
}

// Rewrite takes a GraphQL schema.Mutation add and builds a Dgraph upsert mutation.
// m must have a single argument called 'input' that carries the mutation data.
// The arguments also consist of idExistence map which is a map from
// Variable Name --> UID . This map is used to know which referenced nodes exists and
// whether to link the newly created node to existing node or create a new one.
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
// with GraphQL input object (where country code is @id) :
//
//	{
//	  name: "A.N. Author",
//	  country: { code: "ind", name: "India" },
//	  posts: [ { title: "A Post", text: "Some text" }]
//	  friends: [ { id: "0x123" } ]
//	}
//
// and idExistence
//
//	{
//	  "Country2": "0x234",
//	  "Person3": "0x123"
//	}
//
// becomes an unconditional mutation.
//
//	{
//	  "uid":"_:Author1",
//	  "dgraph.type":["Author"],
//	  "Author.name":"A.N. Author",
//	  "Author.country": {
//	    "uid":"0x234"
//	  },
//	  "Author.posts": [ {
//	    "uid":"_:Post3"
//	    "dgraph.type":["Post"],
//	    "Post.text":"Some text",
//	    "Post.title":"A Post",
//	  } ],
//	  "Author.friends":[ {"uid":"0x123"} ],
//	}
func (arw *AddRewriter) Rewrite(
	ctx context.Context,
	m schema.Mutation,
	idExistence map[string]string) ([]*UpsertMutation, error) {

	mutationType := Add
	mutatedType := m.MutatedType()
	val, _ := m.ArgValue(schema.InputArgName).([]interface{})

	varGen := arw.VarGen
	xidMetadata := arw.XidMetadata
	// ret stores a slice of Upsert Mutations. These are used in executing upsert queries in graphql/resolve/mutation.go
	var ret []*UpsertMutation
	// queries contains queries which are performed along with mutations. These include
	// queries aiding upserts or additional deletes.
	// Example:
	// var(func: uid(0x123)) {
	//   Author_4 as Post.author
	// }
	// The above query is used to find old Author of the Post. The edge between the Post and
	// Author is then deleted using the accompanied mutation.
	var queries []*dql.GraphQuery
	// newNodes is map from variable name to node type.
	// This is used for applying auth on newly added nodes.
	// This is collated from newNodes of each fragment.
	// Example
	// newNodes["Project3"] = schema.Type(Project)
	newNodes := make(map[string]schema.Type)
	// mutationsAll stores mutations computed from fragment. These are returned as Mutation parameter
	// of UpsertMutation
	var mutationsAll []*dgoapi.Mutation
	// retErrors stores errors found out during rewriting mutations.
	// These are returned by this function.
	var retErrors error

	// Parse upsert parameter from addMutation input.
	// If upsert is set to True, this add mutation will be carried as an Upsert Mutation.
	upsert := false
	upsertVal := m.ArgValue(schema.UpsertArgName)
	if upsertVal != nil {
		upsert = upsertVal.(bool)
	}
	if upsert {
		mutationType = AddWithUpsert
	}

	for _, i := range val {
		obj := i.(map[string]interface{})
		fragment, upsertVar, errs := rewriteObject(
			ctx,
			mutatedType,
			nil,
			"",
			varGen,
			obj,
			xidMetadata,
			idExistence,
			mutationType,
		)
		if len(errs) > 0 {
			var gqlErrors x.GqlErrorList
			for _, err := range errs {
				gqlErrors = append(gqlErrors, schema.AsGQLErrors(err)...)
			}
			retErrors = schema.AppendGQLErrs(retErrors, schema.GQLWrapf(gqlErrors,
				"failed to rewrite mutation payload"))
		}
		// TODO: Do RBAC authorization along with RewriteQueries. This will save some time and queries need
		// not be executed in case RBAC is Negative.
		// upsertVar is non-empty in case this is an upsert Mutation and the XID at
		// top level exists. upsertVar in this case contains variable name of the node
		// which is going to be updated. Eg. State3 .
		if upsertVar != "" {
			// Add auth queries for upsert mutation.
			customClaims, err := m.GetAuthMeta().ExtractCustomClaims(ctx)
			if err != nil {
				return ret, err
			}

			authRw := &authRewriter{
				authVariables: customClaims.AuthVariables,
				varGen:        varGen,
				selector:      updateAuthSelector,
				parentVarName: m.MutatedType().Name() + "Root",
			}
			authRw.hasAuthRules = hasAuthRules(m.QueryField(), authRw)
			// Get upsert query of the form,
			// State1 as addState(func: uid(0x11)) @filter(type(State)) {
			// 		uid
			// }
			// These are formed while doing Upserts with Add Mutations. These also contain
			// any related auth queries.
			queries = append(queries, RewriteUpsertQueryFromMutation(
				m, authRw, upsertVar, upsertVar, idExistence[upsertVar])...)
			// Add upsert condition to ensure that the upsert takes place only when the node
			// exists and has proper auth permission.
			// Example condition:  cond: "@if(gt(len(State1), 0))"
			fragment.conditions = append(fragment.conditions, fmt.Sprintf("gt(len(%s), 0)", upsertVar))
		}
		if fragment != nil {
			arw.frags = append(arw.frags, fragment)
		}
	}

	for _, frag := range arw.frags {
		mutation, _ := mutationFromFragment(
			frag,
			func(frag *mutationFragment) ([]byte, error) {
				return json.Marshal(frag.fragment)
			},
			func(frag *mutationFragment) ([]byte, error) {
				if len(frag.deletes) > 0 {
					return json.Marshal(frag.deletes)
				}
				return nil, nil
			})

		if mutation != nil {
			mutationsAll = append(mutationsAll, mutation)
		}
		queries = append(queries, frag.queries...)
		copyTypeMap(frag.newNodes, newNodes)
	}

	if len(mutationsAll) > 0 {
		ret = append(ret, &UpsertMutation{
			Query:     queries,
			Mutations: mutationsAll,
			NewNodes:  newNodes,
		})
	}

	return ret, retErrors
}

// Rewrite rewrites set and remove update patches into dql upsert mutations.
// The GraphQL updates look like:
//
//	input UpdateAuthorInput {
//		filter: AuthorFilter!
//		set: PatchAuthor
//		remove: PatchAuthor
//	}
//
// which gets rewritten in to a Dgraph upsert mutation
// - filter becomes the query
// - set becomes the Dgraph set mutation
// - remove becomes the Dgraph delete mutation
//
// The semantics is the same as the Dgraph mutation semantics.
//   - Any values in set become the new values for those predicates (or add to the existing
//     values for lists)
//   - Any nulls in set are ignored.
//   - Explicit values in remove mean delete this if it is the actual value
//   - Nulls in remove become like delete * for the corresponding predicate.
//
// See AddRewriter for how the set and remove fragments get created.
func (urw *UpdateRewriter) Rewrite(
	ctx context.Context,
	m schema.Mutation,
	idExistence map[string]string) ([]*UpsertMutation, error) {
	mutatedType := m.MutatedType()

	varGen := urw.VarGen
	xidMetadata := urw.XidMetadata

	inp := m.ArgValue(schema.InputArgName).(map[string]interface{})
	setArg := inp["set"]
	delArg := inp["remove"]

	// ret stores a slice of Upsert Mutations. These are used in executing upsert queries in graphql/resolve/mutation.go
	var ret []*UpsertMutation
	// queries contains queries which are performed along with mutations. These include
	// queries aiding upserts or additional deletes.
	// Example:
	// var(func: uid(0x123)) {
	//   Author_4 as Post.author
	// }
	// The above query is used to find old Author of the Post. The edge between the Post and
	// Author is then deleted using the accompanied mutation.
	var queries []*dql.GraphQuery
	// newNodes is map from variable name to node type.
	// This is used for applying auth on newly added nodes.
	// This is collated from newNodes of each fragment.
	// Example
	// newNodes["Project3"] = schema.Type(Project)
	newNodes := make(map[string]schema.Type)
	// mutations stores mutations computed from fragment. These are returned as Mutation parameter
	// of UpsertMutation
	var mutations []*dgoapi.Mutation
	// retErrors stores errors found out during rewriting mutations.
	// These are returned by this function.
	var retErrors error

	customClaims, err := m.GetAuthMeta().ExtractCustomClaims(ctx)
	if err != nil {
		return ret, err
	}

	authRw := &authRewriter{
		authVariables: customClaims.AuthVariables,
		varGen:        varGen,
		selector:      updateAuthSelector,
		parentVarName: m.MutatedType().Name() + "Root",
	}
	authRw.hasAuthRules = hasAuthRules(m.QueryField(), authRw)

	queries = append(queries, RewriteUpsertQueryFromMutation(
		m, authRw, MutationQueryVar, m.Name(), "")...)
	srcUID := MutationQueryVarUID
	objDel, okDelArg := delArg.(map[string]interface{})
	objSet, okSetArg := setArg.(map[string]interface{})
	// if set and remove arguments in update patch are not present or they are empty
	// then we return from here
	if (setArg == nil || (len(objSet) == 0 && okSetArg)) && (delArg == nil || (len(objDel) == 0 && okDelArg)) {
		return ret, nil
	}

	if setArg != nil {
		if len(objSet) != 0 {
			fragment, _, errs := rewriteObject(
				ctx,
				mutatedType,
				nil,
				srcUID,
				varGen,
				objSet,
				xidMetadata,
				idExistence,
				UpdateWithSet,
			)
			if len(errs) > 0 {
				var gqlErrors x.GqlErrorList
				for _, err := range errs {
					gqlErrors = append(gqlErrors, schema.AsGQLErrors(err)...)
				}
				retErrors = schema.AppendGQLErrs(retErrors, schema.GQLWrapf(gqlErrors,
					"failed to rewrite mutation payload"))
			}
			if fragment != nil {
				urw.setFrag = fragment
			}
		}
	}

	if delArg != nil {
		if len(objDel) != 0 {
			// Set additional deletes to false
			fragment, _, errs := rewriteObject(
				ctx,
				mutatedType,
				nil,
				srcUID,
				varGen,
				objDel,
				xidMetadata,
				idExistence,
				UpdateWithRemove,
			)
			if len(errs) > 0 {
				var gqlErrors x.GqlErrorList
				for _, err := range errs {
					gqlErrors = append(gqlErrors, schema.AsGQLErrors(err)...)
				}
				retErrors = schema.AppendGQLErrs(retErrors, schema.GQLWrapf(gqlErrors,
					"failed to rewrite mutation payload"))
			}
			if fragment != nil {
				urw.delFrag = fragment
			}
		}
	}

	if urw.setFrag != nil {
		urw.setFrag.conditions = append(urw.setFrag.conditions, updateMutationCondition)
		mutSet, errSet := mutationFromFragment(
			urw.setFrag,
			func(frag *mutationFragment) ([]byte, error) {
				return json.Marshal(frag.fragment)
			},
			func(frag *mutationFragment) ([]byte, error) {
				if len(frag.deletes) > 0 {
					return json.Marshal(frag.deletes)
				}
				return nil, nil
			})

		if mutSet != nil {
			mutations = append(mutations, mutSet)
		}
		retErrors = schema.AppendGQLErrs(retErrors, errSet)
		queries = append(queries, urw.setFrag.queries...)
	}

	if urw.delFrag != nil {
		urw.delFrag.conditions = append(urw.delFrag.conditions, updateMutationCondition)
		mutDel, errDel := mutationFromFragment(
			urw.delFrag,
			func(frag *mutationFragment) ([]byte, error) {
				return nil, nil
			},
			func(frag *mutationFragment) ([]byte, error) {
				return json.Marshal(frag.fragment)
			})

		if mutDel != nil {
			mutations = append(mutations, mutDel)
		}
		retErrors = schema.AppendGQLErrs(retErrors, errDel)
		queries = append(queries, urw.delFrag.queries...)
	}

	if urw.setFrag != nil {
		copyTypeMap(urw.setFrag.newNodes, newNodes)
	}
	if urw.delFrag != nil {
		copyTypeMap(urw.delFrag.newNodes, newNodes)
	}

	if len(mutations) > 0 {
		ret = append(ret, &UpsertMutation{
			Query:     queries,
			Mutations: mutations,
			NewNodes:  newNodes,
		})
	}
	return ret, retErrors
}

// FromMutationResult rewrites the query part of a GraphQL add mutation into a Dgraph query.
func (arw *AddRewriter) FromMutationResult(
	ctx context.Context,
	mutation schema.Mutation,
	assigned map[string]string,
	result map[string]interface{}) ([]*dql.GraphQuery, error) {

	var errs error

	for _, frag := range arw.frags {
		err := checkResult(frag, result)
		errs = schema.AppendGQLErrs(errs, err)
	}

	// Find any newly added/updated rootUIDs.
	uids, err := convertIDsWithErr(arw.MutatedRootUIDs(mutation, assigned, result))
	errs = schema.AppendGQLErrs(errs, err)

	// Find out if its an upsert with Add mutation.
	// In this case, it may happen that no new node is created, but there may still
	// be some updated nodes. We don't throw an error in this case.
	upsert := false
	upsertVal := mutation.ArgValue(schema.UpsertArgName)
	if upsertVal != nil {
		upsert = upsertVal.(bool)
	}

	// This error is only relevant in case this is not an Upsert with Add Mutation.
	// During upsert with Add mutation, it may happen that no new nodes are created and
	// everything is perfectly alright.
	if len(uids) == 0 && errs == nil && !upsert {
		errs = schema.AsGQLErrors(errors.Errorf("no new node was created"))
	}

	customClaims, err := mutation.GetAuthMeta().ExtractCustomClaims(ctx)
	if err != nil {
		return nil, err
	}

	authRw := &authRewriter{
		authVariables: customClaims.AuthVariables,
		varGen:        NewVariableGenerator(),
		selector:      queryAuthSelector,
		parentVarName: mutation.MutatedType().Name() + "Root",
	}
	authRw.hasAuthRules = hasAuthRules(mutation.QueryField(), authRw)

	if errs != nil {
		return nil, errs
	}
	// No errors are thrown while rewriting queries by Ids.
	return rewriteAsQueryByIds(mutation.QueryField(), uids, authRw), nil
}

// FromMutationResult rewrites the query part of a GraphQL update mutation into a Dgraph query.
func (urw *UpdateRewriter) FromMutationResult(
	ctx context.Context,
	mutation schema.Mutation,
	assigned map[string]string,
	result map[string]interface{}) ([]*dql.GraphQuery, error) {

	err := checkResult(urw.setFrag, result)
	if err != nil {
		return nil, err
	}
	err = checkResult(urw.delFrag, result)
	if err != nil {
		return nil, err
	}

	uids, err := convertIDsWithErr(urw.MutatedRootUIDs(mutation, assigned, result))
	if err != nil {
		return nil, err
	}

	customClaims, err := mutation.GetAuthMeta().ExtractCustomClaims(ctx)
	if err != nil {
		return nil, err
	}

	authRw := &authRewriter{
		authVariables: customClaims.AuthVariables,
		varGen:        NewVariableGenerator(),
		selector:      queryAuthSelector,
		parentVarName: mutation.MutatedType().Name() + "Root",
	}
	authRw.hasAuthRules = hasAuthRules(mutation.QueryField(), authRw)
	return rewriteAsQueryByIds(mutation.QueryField(), uids, authRw), nil
}

func (arw *AddRewriter) MutatedRootUIDs(
	mutation schema.Mutation,
	assigned map[string]string,
	result map[string]interface{}) []string {

	var rootUIDs []string // This stores a list of added or updated rootUIDs.

	for _, frag := range arw.frags {
		fragUid := frag.fragment.(map[string]interface{})["uid"].(string)
		blankNodeName := strings.TrimPrefix(fragUid, "_:")
		uid, ok := assigned[blankNodeName]
		if ok {
			// any newly added uids will be present in assigned map
			rootUIDs = append(rootUIDs, uid)
		} else {
			// node was not part of assigned map. It is likely going to be part of Updated UIDs map.
			// Extract and add any updated uids. This is done for upsert With Add Mutation.
			// We extract out the variable name, eg. Project1 from uid(Project1)
			uidVar := strings.TrimSuffix(strings.TrimPrefix(fragUid, "uid("), ")")
			rootUIDs = append(rootUIDs, extractMutated(result, uidVar)...)
		}
	}

	return rootUIDs
}

func (urw *UpdateRewriter) MutatedRootUIDs(
	mutation schema.Mutation,
	assigned map[string]string,
	result map[string]interface{}) []string {

	return extractMutated(result, mutation.Name())
}

func extractMutated(result map[string]interface{}, mutatedField string) []string {
	var mutated []string

	if val, ok := result[mutatedField].([]interface{}); ok {
		for _, v := range val {
			if obj, vok := v.(map[string]interface{}); vok {
				if uid, uok := obj["uid"].(string); uok {
					mutated = append(mutated, uid)
				}
			}
		}
	}
	return mutated
}

// convertIDsWithErr is similar to convertIDs, except that it also returns the errors, if any.
func convertIDsWithErr(uidSlice []string) ([]uint64, error) {
	var errs error
	ret := make([]uint64, 0, len(uidSlice))
	for _, id := range uidSlice {
		uid, err := strconv.ParseUint(id, 0, 64)
		if err != nil {
			errs = schema.AppendGQLErrs(errs, schema.GQLWrapf(err,
				"received %s as a uid from Dgraph, but couldn't parse it as uint64", id))
			continue
		}
		ret = append(ret, uid)
	}
	return ret, errs
}

// checkResult checks if any mutationFragment in frags was successful in result.
// If any one of the frags (which correspond to conditional mutations) succeeded,
// then the mutation ran through ok.  Otherwise return an error showing why
// at least one of the mutations failed.
func checkResult(frag *mutationFragment, result map[string]interface{}) error {
	if frag == nil {
		return nil
	}

	if result == nil {
		return nil
	}

	err := frag.check(result)
	return err
}

func extractMutationFilter(m schema.Mutation) map[string]interface{} {
	var filter map[string]interface{}
	mutationType := m.MutationType()
	if mutationType == schema.UpdateMutation {
		input, ok := m.ArgValue("input").(map[string]interface{})
		if ok {
			filter, _ = input["filter"].(map[string]interface{})
		}
	} else if mutationType == schema.DeleteMutation {
		filter, _ = m.ArgValue("filter").(map[string]interface{})
	}
	return filter
}

func RewriteUpsertQueryFromMutation(
	m schema.Mutation,
	authRw *authRewriter,
	mutationQueryVar string,
	queryAttribute string,
	nodeID string) []*dql.GraphQuery {
	// The query needs to assign the results to a variable, so that the mutation can use them.
	dgQuery := []*dql.GraphQuery{{
		Var:  mutationQueryVar,
		Attr: queryAttribute,
	}}

	rbac := authRw.evaluateStaticRules(m.MutatedType())
	if rbac == schema.Negative {
		dgQuery[0].Attr = m.ResponseName() + "()"
		return dgQuery
	}

	// For interface, empty delete mutation should be returned if Auth rules are
	// not satisfied even for a single implementing type
	if m.MutatedType().IsInterface() {
		implementingTypesHasFailedRules := false
		implementingTypes := m.MutatedType().ImplementingTypes()
		for _, typ := range implementingTypes {
			if authRw.evaluateStaticRules(typ) != schema.Negative {
				implementingTypesHasFailedRules = true
			}
		}

		if !implementingTypesHasFailedRules {
			dgQuery[0].Attr = m.ResponseName() + "()"
			return dgQuery
		}
	}

	// Add uid child to the upsert query, so that we can get the list of nodes upserted.
	dgQuery[0].Children = append(dgQuery[0].Children, &dql.GraphQuery{
		Attr: "uid",
	})

	// TODO - Cache this instead of this being a loop to find the IDField.
	// nodeID is contains upsertVar in case this is an upsert with Add Mutation.
	// In all other cases nodeID is set to empty.
	// If it is set to empty, this is either a delete or update mutation.
	// In that case, we extract the IDs on which to apply this mutation using
	// extractMutationFilter.
	if nodeID == "" {
		filter := extractMutationFilter(m)
		if ids := idFilter(filter, m.MutatedType().IDField()); ids != nil {
			addUIDFunc(dgQuery[0], ids)
		} else {
			addTypeFunc(dgQuery[0], m.MutatedType().DgraphName())
		}

		_ = addFilter(dgQuery[0], m.MutatedType(), filter)
	} else {
		// It means this is called from upsert with Add mutation.
		// nodeID will be uid of the node to be upserted. We add UID func
		// and type filter to generate query like
		// State3 as addState(func: uid(0x13)) @filter(type(State)) {
		//  	uid
		// }
		uid, err := strconv.ParseUint(nodeID, 0, 64)
		if err != nil {
			dgQuery[0].Attr = m.ResponseName() + "()"
			return dgQuery
		}
		addUIDFunc(dgQuery[0], []uint64{uid})
		addTypeFilter(dgQuery[0], m.MutatedType())
	}
	dgQuery = authRw.addAuthQueries(m.MutatedType(), dgQuery, rbac)

	return dgQuery
}

// removeNodeReference removes any reference we know about (via @hasInverse) into a node.
func removeNodeReference(m schema.Mutation, authRw *authRewriter,
	qry *dql.GraphQuery) []interface{} {
	var deletes []interface{}
	for _, fld := range m.MutatedType().Fields() {
		invField := fld.Inverse()
		if invField == nil {
			// This field be a reverse edge, in that case we need to delete the incoming connections
			// to this node via its forward edges.
			invField = fld.ForwardEdge()
			if invField == nil {
				continue
			}
		}
		varName := authRw.varGen.Next(fld.Type(), "", "", false)

		qry.Children = append(qry.Children,
			&dql.GraphQuery{
				Var:  varName,
				Attr: invField.Type().DgraphPredicate(fld.Name()),
			})

		delFldName := fld.Type().DgraphPredicate(invField.Name())
		del := map[string]interface{}{"uid": MutationQueryVarUID}
		if invField.Type().ListType() == nil {
			deletes = append(deletes, map[string]interface{}{
				"uid":      fmt.Sprintf("uid(%s)", varName),
				delFldName: del})
		} else {
			deletes = append(deletes, map[string]interface{}{
				"uid":      fmt.Sprintf("uid(%s)", varName),
				delFldName: []interface{}{del}})
		}
	}
	return deletes
}

func (drw *deleteRewriter) Rewrite(
	ctx context.Context,
	m schema.Mutation,
	idExistence map[string]string) ([]*UpsertMutation, error) {

	if m.MutationType() != schema.DeleteMutation {
		return nil, errors.Errorf(
			"(internal error) call to build delete mutation for %s mutation type",
			m.MutationType())
	}

	customClaims, err := m.GetAuthMeta().ExtractCustomClaims(ctx)
	if err != nil {
		return nil, err
	}

	authRw := &authRewriter{
		authVariables: customClaims.AuthVariables,
		varGen:        drw.VarGen,
		selector:      deleteAuthSelector,
		parentVarName: m.MutatedType().Name() + "Root",
	}
	authRw.hasAuthRules = hasAuthRules(m.QueryField(), authRw)

	dgQry := RewriteUpsertQueryFromMutation(m, authRw, MutationQueryVar, m.Name(), "")
	qry := dgQry[0]

	deletes := []interface{}{map[string]interface{}{"uid": "uid(x)"}}
	// We need to remove node reference only if auth rule succeeds.
	if qry.Attr != m.ResponseName()+"()" {
		// We need to delete the node and then any reference we know about (via @hasInverse)
		// into this node.
		deletes = append(deletes, removeNodeReference(m, authRw, qry)...)
	}

	b, err := json.Marshal(deletes)
	if err != nil {
		return nil, err
	}

	upserts := []*UpsertMutation{{
		Query:     dgQry,
		Mutations: []*dgoapi.Mutation{{DeleteJson: b}},
	}}

	// If the mutation had the query field, then we also need to query the nodes which are going to
	// be deleted before they are deleted. Let's add a query to do that.
	if queryField := m.QueryField(); queryField != nil {
		queryAuthRw := &authRewriter{
			authVariables: customClaims.AuthVariables,
			varGen:        drw.VarGen,
			selector:      queryAuthSelector,
			filterByUid:   true,
			parentVarName: drw.VarGen.Next(queryField.Type(), "", "", false),
			varName:       MutationQueryVar,
			hasAuthRules:  hasAuthRules(queryField, authRw),
		}

		// these queries are responsible for querying the queryField
		queryFieldQry := rewriteAsQuery(queryField, queryAuthRw)

		// we don't want the `x` query to show up in GraphQL JSON response while querying the query
		// field. So, need to make it `var` query and remove any children from it as there can be
		// variables in them which won't be used in this query.
		// Need to make a copy because the query for the 1st upsert shouldn't be affected.
		qryCopy := &dql.GraphQuery{
			Var:      MutationQueryVar,
			Attr:     "var",
			Func:     qry.Func,
			Children: nil, // no need to copy children
			Filter:   qry.Filter,
		}
		// if there wasn't any root func because auth RBAC processing may have filtered out
		// everything, then need to append () to attr so that a valid DQL is formed.
		if qryCopy.Func == nil {
			qryCopy.Attr = qryCopy.Attr + "()"
		}
		// if the queryFieldQry didn't use the variable `x`, then need to make qryCopy not use that
		// variable name, so that a valid DQL is formed. This happens when RBAC processing returns
		// false.
		if queryFieldQry[0].Attr == queryField.DgraphAlias()+"()" {
			qryCopy.Var = ""
		}
		queryFieldQry = append(append([]*dql.GraphQuery{qryCopy}, dgQry[1:]...), queryFieldQry...)
		upserts = append(upserts, &UpsertMutation{Query: queryFieldQry})
	}

	return upserts, err
}

func (drw *deleteRewriter) FromMutationResult(
	ctx context.Context,
	mutation schema.Mutation,
	assigned map[string]string,
	result map[string]interface{}) ([]*dql.GraphQuery, error) {

	// There's no query that follows a delete
	return nil, nil
}

func (drw *deleteRewriter) MutatedRootUIDs(
	mutation schema.Mutation,
	assigned map[string]string,
	result map[string]interface{}) []string {

	return extractMutated(result, mutation.Name())
}

// RewriteQueries on deleteRewriter does not return any queries. queries to check
// existence of nodes are not needed as part of Delete Mutation.
// The function generates VarGen and XidMetadata which are used in Rewrite function.
func (drw *deleteRewriter) RewriteQueries(
	ctx context.Context,
	m schema.Mutation) ([]*dql.GraphQuery, []string, error) {

	drw.VarGen = NewVariableGenerator()

	return []*dql.GraphQuery{}, []string{}, nil
}

func asUID(val interface{}) (uint64, error) {
	if val == nil {
		return 0, errors.Errorf("ID value was null")
	}

	id, ok := val.(string)
	uid, err := strconv.ParseUint(id, 0, 64)

	if !ok || err != nil {
		return 0, errors.Errorf("ID argument (%s) was not able to be parsed", id)
	}

	return uid, nil
}

func addAuthSelector(t schema.Type) *schema.RuleNode {
	auth := t.AuthRules()
	if auth == nil || auth.Rules == nil {
		return nil
	}

	return auth.Rules.Add
}

func updateAuthSelector(t schema.Type) *schema.RuleNode {
	auth := t.AuthRules()
	if auth == nil || auth.Rules == nil {
		return nil
	}

	return auth.Rules.Update
}

func deleteAuthSelector(t schema.Type) *schema.RuleNode {
	auth := t.AuthRules()
	if auth == nil || auth.Rules == nil {
		return nil
	}

	return auth.Rules.Delete
}

func mutationFromFragment(
	frag *mutationFragment,
	setBuilder, delBuilder mutationBuilder) (*dgoapi.Mutation, error) {

	if frag == nil {
		return nil, nil
	}

	var conditions string
	if len(frag.conditions) > 0 {
		conditions = fmt.Sprintf("@if(%s)", strings.Join(frag.conditions, " AND "))
	}

	set, err := setBuilder(frag)
	if err != nil {
		return nil, schema.AsGQLErrors(err)
	}

	del, err := delBuilder(frag)
	if err != nil {
		return nil, schema.AsGQLErrors(err)
	}

	return &dgoapi.Mutation{
		SetJson:    set,
		DeleteJson: del,
		Cond:       conditions,
	}, nil

}

func checkXIDExistsQuery(
	xidVariable, xidString, xidPredicate string, typ schema.Type) *dql.GraphQuery {
	qry := &dql.GraphQuery{
		Attr: xidVariable,
		Func: &dql.Function{
			Name: "eq",
			Args: []dql.Arg{
				{Value: typ.DgraphPredicate(xidPredicate)},
				{Value: maybeQuoteArg("eq", xidString)},
			},
		},
		Children: []*dql.GraphQuery{{Attr: "uid"}, {Attr: "dgraph.type"}},
	}
	return qry
}

func checkUIDExistsQuery(val interface{}, variable string) (*dql.GraphQuery, error) {
	uid, err := asUID(val)
	if err != nil {
		return nil, err
	}

	query := &dql.GraphQuery{
		Attr:     variable,
		UID:      []uint64{uid},
		Children: []*dql.GraphQuery{{Attr: "uid"}, {Attr: "dgraph.type"}},
	}
	addUIDFunc(query, []uint64{uid})
	return query, nil
}

// asIDReference makes a mutation fragment that resolves a reference to the uid in val.  There's
// a bit of extra mutation to build if the original mutation contains a reference to
// another node: e.g it was say adding a Post with:
// { "title": "...", "author": { "id": "0x123" }, ... }
// and we'd gotten to here        ^^
// in rewriteObject with srcField = "author" srcUID = "XYZ"
// and the schema says that Post.author and Author.Posts are inverses of each other, then we need
// to make sure that inverse link is added/removed.  We have to make sure the Dgraph upsert
// mutation ends up like:
//
// mutation :
// { "uid": "XYZ", "title": "...", "author": { "id": "0x123", "posts": [ { "uid": "XYZ" } ] }, ... }
// asIDReference builds the fragment
// { "id": "0x123", "posts": [ { "uid": "XYZ" } ] }
func asIDReference(
	ctx context.Context,
	val interface{},
	srcField schema.FieldDefinition,
	srcUID string,
	varGen *VariableGenerator,
	isRemove bool) *mutationFragment {

	result := make(map[string]interface{}, 2)
	frag := newFragment(result)

	// No need to check if this is a valid UID. It is because this would have been checked
	// in checkUIDExistsQuery function called from corresponding getExistenceQueries function.

	result["uid"] = val // val will contain the UID string.

	addInverseLink(result, srcField, srcUID)

	// Delete any additional old edges from inverse nodes in case this is not a remove
	// as part of an Update Mutation.
	if !isRemove {
		addAdditionalDeletes(ctx, frag, varGen, srcField, srcUID, val.(string))
	}
	return frag

}

// rewriteObject rewrites obj to a list of mutation fragments.  See AddRewriter.Rewrite
// for a description of what those fragments look like.
//
// GraphQL validation has already ensured that the types of arguments (or variables)
// are correct and has ensured that non-nullables are not null.  But for deep mutations
// that's not quite enough, and we have add some extra checking on the reference
// types.
//
// Currently adds enforce the schema ! restrictions, but updates don't.
// e.g. a Post might have `title: String!“ in the schema, but,  a Post update could
// set that to to null. ATM we allow this and it'll just triggers GraphQL error propagation
// when that is in a query result.  This is the same case as deletes: e.g. deleting
// an author might make the `author: Author!` field of a bunch of Posts invalid.
// (That might actually be helpful if you want to run one mutation to remove something
// and then another to correct it.)
//
// rewriteObject builds a set of mutations. Using the argument idExistence, it is decided
// whether to create new nodes or link to existing nodes. Mutations are built recursively
// in a dfs like algorithm.
// In addition to returning the mutationFragment and any errors. It also returns upsertVar.
// In case this is an upsert Add mutation and top level node exists with XID, upsertVar stores
// the variable name of the top level node. Eg. State1
// In all other cases, upsertVar is "".
func rewriteObject(
	ctx context.Context,
	typ schema.Type,
	srcField schema.FieldDefinition,
	srcUID string,
	varGen *VariableGenerator,
	obj map[string]interface{},
	xidMetadata *xidMetadata,
	idExistence map[string]string,
	mutationType MutationType) (*mutationFragment, string, []error) {

	// There could be the following cases:
	// 1. We need to create a new node.
	// 2. We use an existing node and link it to the parent.
	//    We may have to add an inverse edge in this case. But generally, no other amendments
	//    to the node need to be done.
	// Note that as similar traversal of input tree was carried with getExistenceQueries, we
	// don't have to report the same errors.

	upsertVar := ""
	atTopLevel := srcField == nil
	var retErrors []error
	variable := ""

	id := typ.IDField()
	if id != nil {
		// Check if the ID field is referenced in the mutation
		if idVal, ok := obj[id.Name()]; ok {
			// This node is referenced and must definitely exist.
			// If it does not exist, we should be throwing an error.
			// No need to add query if the UID is already been seen.

			// Fetch corresponding variable name
			variable = varGen.Next(typ, id.Name(), idVal.(string), false)

			// Get whether UID exists or not from existenceQueriesResult
			if _, ok := idExistence[variable]; ok {
				// UID exists.
				// We return an error if this is at toplevel. Else, we return the ID reference
				if atTopLevel {
					// We need to conceal the error because we might be leaking information to the user if it
					// tries to add duplicate data to the field with @id.
					var err error
					if queryAuthSelector(typ) == nil {
						err = x.GqlErrorf("id %s already exists for type %s", idVal.(string), typ.Name())
					} else {
						// This error will only be reported in debug mode.
						err = x.GqlErrorf("GraphQL debug: id already exists for type %s", typ.Name())
					}
					retErrors = append(retErrors, err)
					return nil, upsertVar, retErrors
				} else {
					return asIDReference(ctx, idVal, srcField, srcUID, varGen, mutationType == UpdateWithRemove), upsertVar, nil
				}
			} else {
				// Reference UID does not exist. This is an error.
				err := errors.Errorf("ID \"%s\" isn't a %s", idVal.(string), srcField.Type().Name())
				retErrors = append(retErrors, err)
				return nil, upsertVar, retErrors
			}
		}
	}

	xids := typ.XIDFields()
	if len(xids) != 0 {
		// nonExistingXIDs stores number of uids for which there exist no nodes
		var nonExistingXIDs int
		// xidVariables stores the variable names for each XID.
		var xidVariables []string
		for _, xid := range xids {
			var xidString string
			if xidVal, ok := obj[xid.Name()]; ok && xidVal != nil {
				xidString, _ = extractVal(xidVal, xid.Name(), xid.Type().Name())
				variable = varGen.Next(typ, xid.Name(), xidString, false)

				// Three cases:
				// 1. If the queryResult UID exists. Add a reference.
				// 2. If the queryResult UID does not exist and this is the first time we are seeing
				//    this. Then, return error.
				// 3. The queryResult UID does not exist. But, this could be a reference to an XID
				//    node added during the mutation rewriting. This is handled by adding the new blank UID
				//    to existenceQueryResult.

				// Get whether node with XID exists or not from existenceQueriesResult
				if uid, ok := idExistence[variable]; ok {
					// node with XID exists. This is a reference.
					// We return an error if this is at toplevel. Else, we return the ID reference
					if atTopLevel {
						if mutationType == AddWithUpsert {
							// This means we are in Add Mutation with upsert: true.
							// In this case, we don't return an error and continue updating this node.
							// upsertVar is set to variable and srcUID is set to uid(variable) to continue
							// updating this node.
							upsertVar = variable
							srcUID = fmt.Sprintf("uid(%s)", variable)
						} else {
							// We return an error as we are at top level of non-upsert mutation and the XID exists.
							// We need to conceal the error because we might be leaking information to the user if it
							// tries to add duplicate data to the field with @id.
							var err error
							if queryAuthSelector(typ) == nil {
								err = x.GqlErrorf("id %s already exists "+
									"for field %s inside type %s", xidString, xid.Name(), typ.Name())
							} else {
								// This error will only be reported in debug mode.
								err = x.GqlErrorf("GraphQL debug: id %s already exists for "+
									"field %s inside type %s", xidString, xid.Name(), typ.Name())
							}
							retErrors = append(retErrors, err)
							return nil, upsertVar, retErrors
						}
					} else {
						// As we are not at top level, we return the XID reference. We don't update this node
						// further.
						return asIDReference(ctx, uid, srcField, srcUID, varGen, mutationType == UpdateWithRemove), upsertVar, nil
					}
				} else {

					// Node with XIDs does not exist. It means this is a new node.
					// This node will be created later.
					obj = xidMetadata.variableObjMap[variable]
					xidVariables = append(xidVariables, variable)
					// We add a new node only if
					// 1. All the xids are present and
					// 2. No node exist for any of the xid
					if nonExistingXIDs == len(xids)-1 {
						exclude := ""
						if srcField != nil {
							invField := srcField.Inverse()
							if invField != nil {
								exclude = invField.Name()
							}
						}
						// We replace obj with xidMetadata.variableObjMap[variable] in this case.
						// This is done to ensure that the first time we encounter an XID node, we use
						// its definition and later times, we just use its reference.

						if err := typ.EnsureNonNulls(obj, exclude); err != nil {
							// This object does not contain XID. This is an error.
							retErrors = append(retErrors, err)
							return nil, upsertVar, retErrors
						}
						// Set existenceQueryResult to _:variable. This is to make referencing to
						// this node later easier.
						// Set idExistence for all variables which are referencing this node to
						// the blank node _:variable.
						// Example: if We have multiple xids inside a type say person, then
						// we create a single blank node e.g. _:person1
						// and also two different query variables for xids say person1,person2 and assign
						// _:person1 to both of them in idExistence map
						// i.e. idExistence[person1]= _:person1
						// idExistence[person2]= _:person1
						for _, xidVariable := range xidVariables {
							idExistence[xidVariable] = fmt.Sprintf("_:%s", variable)
						}
					}
					nonExistingXIDs++

				}
			}
		}
		if upsertVar == "" {
			for _, xid := range xids {
				if xidVal, ok := obj[xid.Name()]; ok && xidVal != nil {
					// This is handled in the for loop above
					continue
				} else if mutationType == Add || mutationType == AddWithUpsert || !atTopLevel {
					// When we reach this stage we are absoulutely sure that this is not a reference and is
					// a new node and one of the XIDs is missing.
					// There are two possibilities here:
					// 1. This is an Add Mutation or we are at some deeper level inside Update Mutation:
					//    In this case this is an error as XID field if referenced anywhere inside Add Mutation
					//    or at deeper levels in Update Mutation has to be present. If multiple xids are not present
					//    then we return error for only one.
					// 2. This is an Update Mutation and we are at top level:
					//    In this case this is not an error as the UID at top level of Update Mutation is
					//    referenced as uid(x) in mutations. We don't throw an error in this case and continue
					//    with the function.
					err := errors.Errorf("field %s cannot be empty", xid.Name())
					retErrors = append(retErrors, err)
					return nil, upsertVar, retErrors
				}
			}
		} else {
			// In case this is known to be an Upsert. We delete all entries of XIDs
			// from obj. This is done to prevent any XID entries in the json which is returned
			// by rewriteObject and ensure that no XID value gets rewritten due to upsert.
			for _, xid := range xids {
				// To ensure that xid is not added to the output json in case of upsert
				delete(obj, xid.Name())
			}
		}
	}

	// This is not an XID reference. This is also not a UID reference.
	// This is definitely a new node.
	// Create new node
	if variable == "" {
		// This will happen in case when this is a new node and does not contain XID.
		variable = varGen.Next(typ, "", "", false)
	}

	// myUID is used for referencing this node. It is set to _:variable
	myUID := fmt.Sprintf("_:%s", variable)

	// Assign dgraph.types attribute.
	dgraphTypes := []string{typ.DgraphName()}
	dgraphTypes = append(dgraphTypes, typ.Interfaces()...)

	// Create newObj map. This map will be returned as part of mutationFragment.
	newObj := make(map[string]interface{}, len(obj))

	if (mutationType != Add && mutationType != AddWithUpsert && atTopLevel) || upsertVar != "" {
		// Two Cases
		// Case 1:
		// It's an update and we are at top level. So, the UID of node(s) for which
		// we are rewriting is/are referenced using "uid(x)" as part of mutations.
		// We don't need to create a new blank node in this case.
		// srcUID is equal to uid(x) in this case.
		// Case 2:
		// This is an upsert with Add Mutation and upsertVar is non-empty (which means
		// the XID at top level exists and this is an upsert).
		// We continue updating in this case and no new node is created. srcUID will be
		// equal to uid(variable) in this case. Eg. uid(State1)
		newObj["uid"] = srcUID
		myUID = srcUID
	} else if mutationType == UpdateWithRemove {
		// It's a remove. As remove can only be part of Update Mutation. It can
		// be inferred that this is an Update Mutation.
		// In case of remove of Update, deeper level nodes have to be referenced by ID
		// or XID. If we have reached this stage, we can be sure that no such reference
		// to ID or XID exists. In that case, we throw an error.
		err := errors.Errorf("id is not provided")
		retErrors = append(retErrors, err)
		return nil, upsertVar, retErrors
	} else {
		// We are in Add Mutation or at a deeper level in Update Mutation set.
		// If we have reached this stage, we can be sure that we need to create a new
		// node as part of the mutation. The new node is referenced as a blank node like
		// "_:Project2" . myUID will store the variable generated to reference this node.
		newObj["dgraph.type"] = dgraphTypes
		newObj["uid"] = myUID
	}

	// Add Inverse Link if necessary
	deleteInverseObject(obj, srcField)
	addInverseLink(newObj, srcField, srcUID)

	frag := newFragment(newObj)
	// TODO(Rajas)L Check if newNodes only needs to be set in case new nodes have been added.
	frag.newNodes[variable] = typ

	updateFromChildren := func(parentFragment, childFragment *mutationFragment) {
		copyTypeMap(childFragment.newNodes, parentFragment.newNodes)
		frag.queries = append(parentFragment.queries, childFragment.queries...)
		frag.deletes = append(parentFragment.deletes, childFragment.deletes...)
		frag.check = func(lcheck, rcheck resultChecker) resultChecker {
			return func(m map[string]interface{}) error {
				return schema.AppendGQLErrs(lcheck(m), rcheck(m))
			}
		}(parentFragment.check, childFragment.check)
	}

	// Iterate on fields and call the same function recursively.
	var fields []string
	for field := range obj {
		fields = append(fields, field)
	}
	// Fields are sorted to ensure that they are traversed in specific order each time. Golang maps
	// don't store keys in sorted order.
	sort.Strings(fields)
	for _, field := range fields {
		val := obj[field]

		fieldDef := typ.Field(field)
		fieldName := typ.DgraphPredicate(field)

		// This fixes mutation when dgraph predicate has special characters. PR #5526
		if strings.HasPrefix(fieldName, "<") && strings.HasSuffix(fieldName, ">") {
			fieldName = fieldName[1 : len(fieldName)-1]
		}

		// TODO: Write a function for aggregating data of fragment from child nodes.
		switch val := val.(type) {
		case map[string]interface{}:
			if fieldDef.Type().IsUnion() {
				fieldMutationFragment, _, err := rewriteUnionField(
					ctx,
					fieldDef,
					myUID,
					varGen,
					val,
					xidMetadata,
					idExistence,
					mutationType)
				if fieldMutationFragment != nil {
					newObj[fieldName] = fieldMutationFragment.fragment
					updateFromChildren(frag, fieldMutationFragment)
				}
				retErrors = append(retErrors, err...)
			} else if fieldDef.Type().IsGeo() {
				newObj[fieldName] =
					map[string]interface{}{
						"type":        fieldDef.Type().Name(),
						"coordinates": rewriteGeoObject(val, fieldDef.Type()),
					}
			} else {
				fieldMutationFragment, _, err := rewriteObject(
					ctx,
					fieldDef.Type(),
					fieldDef,
					myUID,
					varGen,
					val,
					xidMetadata,
					idExistence,
					mutationType)
				if fieldMutationFragment != nil {
					newObj[fieldName] = fieldMutationFragment.fragment
					updateFromChildren(frag, fieldMutationFragment)
				}
				retErrors = append(retErrors, err...)
			}
		case []interface{}:
			mutationFragments := make([]interface{}, 0)
			var fieldMutationFragment *mutationFragment
			var err []error
			for _, object := range val {
				switch object := object.(type) {
				case map[string]interface{}:
					if fieldDef.Type().IsUnion() {
						fieldMutationFragment, _, err = rewriteUnionField(
							ctx,
							fieldDef,
							myUID,
							varGen,
							object,
							xidMetadata,
							idExistence,
							mutationType)
					} else if fieldDef.Type().IsGeo() {
						fieldMutationFragment = newFragment(
							map[string]interface{}{
								"type":        fieldDef.Type().Name(),
								"coordinates": rewriteGeoObject(object, fieldDef.Type()),
							},
						)
					} else {
						fieldMutationFragment, _, err = rewriteObject(
							ctx,
							fieldDef.Type(),
							fieldDef,
							myUID,
							varGen,
							object,
							xidMetadata,
							idExistence,
							mutationType)
					}
					if fieldMutationFragment != nil {
						mutationFragments = append(mutationFragments, fieldMutationFragment.fragment)
						updateFromChildren(frag, fieldMutationFragment)
					}
					retErrors = append(retErrors, err...)
				default:
					// This is a scalar list.
					mutationFragments = append(mutationFragments, object)
				}

			}
			if newObj[fieldName] != nil {
				newObj[fieldName] = append(newObj[fieldName].([]interface{}), mutationFragments...)
			} else {
				newObj[fieldName] = mutationFragments
			}
		default:
			// This field is either a scalar value or a null.
			newObj[fieldName] = val
		}
	}

	return frag, upsertVar, retErrors
}

// existenceQueries takes a GraphQL JSON object as obj and creates queries to find
// out if referenced nodes by XID and UID exist or not.
// This is done in recursive fashion using a dfs.
// This function is called from RewriteQueries function on AddRewriter and UpdateRewriter
// objects.
// Look at description of RewriteQueries for an example of generated existence queries.
func existenceQueries(
	ctx context.Context,
	typ schema.Type,
	srcField schema.FieldDefinition,
	varGen *VariableGenerator,
	obj map[string]interface{},
	xidMetadata *xidMetadata) ([]*dql.GraphQuery, []string, []error) {

	atTopLevel := srcField == nil
	var ret []*dql.GraphQuery
	var retTypes []string
	var retErrors []error

	// Inverse Object field is deleted. This is to ensure that we don't refer any conflicting
	// inverse node as inverse of a field.
	// Example: For the given mutation,
	// addAuthor (input: [{name: ..., posts: [ {author: { id: "some id"}} ]} ] ),
	// the part, author: { id: "some id"} is removed. This ensures that the author
	// for the post is not set to something different but is set to the real author.
	deleteInverseObject(obj, srcField)

	id := typ.IDField()
	if id != nil {
		// Check if the ID field is referenced in the mutation
		if idVal, ok := obj[id.Name()]; ok {
			if idVal != nil {
				// No need to add query if the UID is already been seen.
				if xidMetadata.seenUIDs[idVal.(string)] {
					return ret, retTypes, retErrors
				}
				// Mark this UID as seen.
				xidMetadata.seenUIDs[idVal.(string)] = true
				variable := varGen.Next(typ, id.Name(), idVal.(string), false)

				query, err := checkUIDExistsQuery(idVal, variable)
				if err != nil {
					retErrors = append(retErrors, err)
				}
				ret = append(ret, query)
				retTypes = append(retTypes, srcField.Type().DgraphName())
				return ret, retTypes, retErrors
				// Add check UID query and return it.
				// There is no need to move forward. If reference ID field is given,
				// it has to exist.
			}
			// As the type has not been referenced by ID field, remove it so that it does
			// not interfere with further processing.
			delete(obj, id.Name())
		}
	}

	xids := typ.XIDFields()
	var xidString string
	var err error
	if len(xids) != 0 {
		for _, xid := range xids {
			if xidVal, ok := obj[xid.Name()]; ok && xidVal != nil {
				xidString, err = extractVal(xidVal, xid.Name(), xid.Type().Name())
				if err != nil {
					return nil, nil, append(retErrors, err)
				}
				variable := varGen.Next(typ, xid.Name(), xidString, false)
				// There are two cases:
				// Case 1: We are at top level:
				// 	       We return an error if the same node is referenced twice at top level.
				// Case 2: We are not at top level:
				//         We don't return an error if one of the occurrences of XID is a reference
				//         and other is definition.
				//         We return an error if both occurrences contain values other than XID and are
				//         not equal.
				if xidMetadata.variableObjMap[variable] != nil {
					// if we already encountered an object with same xid earlier, and this object is
					// considered a duplicate of the existing object, then return error.
					if xidMetadata.isDuplicateXid(atTopLevel, variable, obj, srcField) {
						err := errors.Errorf("duplicate XID found: %s", xidString)
						retErrors = append(retErrors, err)
						return nil, nil, retErrors
					}
					// In the other case it is not duplicate, we update variableObjMap in case the new
					// occurrence of XID is its description and the old occurrence was a reference.
					// Example:
					// obj = { "id": "1", "name": "name1"}
					// xidMetadata.variableObjMap[variable] = { "id": "1" }
					// In this case, as obj is the correct definition of the object, we update variableObjMap
					oldObj := xidMetadata.variableObjMap[variable]
					if len(oldObj) == 1 && len(obj) > 1 {
						// Continue execution to perform dfs in this case. There may be more nodes
						// in the subtree of this node.
						xidMetadata.variableObjMap[variable] = obj
					} else {
						// This is just a node reference. No need to proceed further.
						return ret, retTypes, retErrors
					}
				} else {

					// if not encountered till now, add it to the map,
					xidMetadata.variableObjMap[variable] = obj

					// save if this node was seen at top level.
					xidMetadata.seenAtTopLevel[variable] = atTopLevel

					// Add the corresponding existence query. As this is the first time we have
					// encountered this variable, the query is added only once per variable.
					query := checkXIDExistsQuery(variable, xidString, xid.Name(), typ)
					ret = append(ret, query)
					retTypes = append(retTypes, typ.DgraphName())
					// Don't return just over here as there maybe more nodes in the children tree.
				}
			}
		}
	}

	// Iterate on fields and call the same function recursively.
	var fields []string
	for field := range obj {
		fields = append(fields, field)
	}
	// Fields are sorted to ensure that they are traversed in specific order each time. Golang maps
	// don't store keys in sorted order.
	sort.Strings(fields)
	for _, field := range fields {
		val := obj[field]

		fieldDef := typ.Field(field)
		fieldName := typ.DgraphPredicate(field)

		// This fixes mutation when dgraph predicate has special characters. PR #5526
		if strings.HasPrefix(fieldName, "<") && strings.HasSuffix(fieldName, ">") {
			fieldName = fieldName[1 : len(fieldName)-1]
		}

		switch val := val.(type) {
		case map[string]interface{}:
			if fieldDef.Type().IsUnion() {
				fieldQueries, fieldTypes, err := existenceQueriesUnion(
					ctx, typ, fieldDef, varGen, val, xidMetadata, -1)
				retErrors = append(retErrors, err...)
				ret = append(ret, fieldQueries...)
				retTypes = append(retTypes, fieldTypes...)
			} else {
				fieldQueries, fieldTypes, err := existenceQueries(ctx,
					fieldDef.Type(), fieldDef, varGen, val, xidMetadata)
				retErrors = append(retErrors, err...)
				ret = append(ret, fieldQueries...)
				retTypes = append(retTypes, fieldTypes...)
			}
		case []interface{}:
			for i, object := range val {
				switch object := object.(type) {
				case map[string]interface{}:
					var fieldQueries []*dql.GraphQuery
					var fieldTypes []string
					var err []error
					if fieldDef.Type().IsUnion() {
						fieldQueries, fieldTypes, err = existenceQueriesUnion(
							ctx, typ, fieldDef, varGen, object, xidMetadata, i)
					} else {
						fieldQueries, fieldTypes, err = existenceQueries(
							ctx, fieldDef.Type(), fieldDef, varGen, object, xidMetadata)
					}
					retErrors = append(retErrors, err...)
					ret = append(ret, fieldQueries...)
					retTypes = append(retTypes, fieldTypes...)
				default:
					// This is a scalar list. So, it won't contain any XID.
					// Don't do anything.
				}

			}
		default:
			// This field is either a scalar value or a null.
			// Fields with ID directive cannot have empty values. Checking it here.
			if fieldDef.HasIDDirective() && val == "" {
				err := fmt.Errorf("encountered an empty value for @id field `%s`", fieldName)
				retErrors = append(retErrors, err)
				return nil, nil, retErrors
			}
		}
	}

	return ret, retTypes, retErrors
}

func existenceQueriesUnion(
	ctx context.Context,
	parentTyp schema.Type,
	srcField schema.FieldDefinition,
	varGen *VariableGenerator,
	obj map[string]interface{},
	xidMetadata *xidMetadata,
	listIndex int) ([]*dql.GraphQuery, []string, []error) {

	var retError []error
	if len(obj) != 1 {
		var err error
		// if this was called from rewriteList,
		// the listIndex will tell which particular item in the list has an error.
		if listIndex >= 0 {
			err = fmt.Errorf(
				"value for field `%s` in type `%s` index `%d` must have exactly one child, "+
					"found %d children", srcField.Name(), parentTyp.Name(), listIndex, len(obj))
		} else {
			err = fmt.Errorf(
				"value for field `%s` in type `%s` must have exactly one child, found %d children",
				srcField.Name(), parentTyp.Name(), len(obj))
		}
		retError = append(retError, err)
		return nil, nil, retError
	}

	var newtyp schema.Type
	for memberRef, memberRefVal := range obj {
		memberTypeName := strings.ToUpper(memberRef[:1]) + memberRef[1:len(
			memberRef)-3]
		srcField = srcField.WithMemberType(memberTypeName)
		newtyp = srcField.Type()
		obj = memberRefVal.(map[string]interface{})
	}
	return existenceQueries(ctx, newtyp, srcField, varGen, obj, xidMetadata)
}

// if this is a union field, then obj should have only one key which will be a ref
// to one of the member types. Eg:
// { "dogRef" : { ... } }
// So, just rewrite it as an object with correct underlying type.
func rewriteUnionField(
	ctx context.Context,
	srcField schema.FieldDefinition,
	srcUID string,
	varGen *VariableGenerator,
	obj map[string]interface{},
	xidMetadata *xidMetadata,
	existenceQueriesResult map[string]string,
	mutationType MutationType) (*mutationFragment, string, []error) {

	var newtyp schema.Type
	for memberRef, memberRefVal := range obj {
		memberTypeName := strings.ToUpper(memberRef[:1]) + memberRef[1:len(
			memberRef)-3]
		srcField = srcField.WithMemberType(memberTypeName)
		newtyp = srcField.Type()
		obj = memberRefVal.(map[string]interface{})
	}
	return rewriteObject(ctx, newtyp, srcField, srcUID, varGen, obj, xidMetadata, existenceQueriesResult, mutationType)
}

// rewriteGeoObject rewrites the given value correctly based on the underlying Geo type.
// Currently, it supports Point, Polygon and MultiPolygon.
func rewriteGeoObject(val map[string]interface{}, typ schema.Type) []interface{} {
	switch typ.Name() {
	case schema.Point:
		return rewritePoint(val)
	case schema.Polygon:
		return rewritePolygon(val)
	case schema.MultiPolygon:
		return rewriteMultiPolygon(val)
	}
	return nil
}

// rewritePoint constructs coordinates for Point type.
// For Point type, the mutation json is as follows:
// { "type": "Point", "coordinates": [11.11, 22.22] }
func rewritePoint(point map[string]interface{}) []interface{} {
	return []interface{}{point[schema.Longitude], point[schema.Latitude]}
}

// rewritePolygon constructs coordinates for Polygon type.
// For Polygon type, the mutation json is as follows:
//
//	{
//		"type": "Polygon",
//		"coordinates": [[[22.22,11.11],[16.16,15.15],[21.21,20.2]],[[22.28,11.18],[16.18,15.18],[21.28,20.28]]]
//	}
func rewritePolygon(val map[string]interface{}) []interface{} {
	// type casting this is safe, because of strict GraphQL schema
	coordinates := val[schema.Coordinates].([]interface{})
	resPoly := make([]interface{}, 0, len(coordinates))
	for _, pointList := range coordinates {
		// type casting this is safe, because of strict GraphQL schema
		points := pointList.(map[string]interface{})[schema.Points].([]interface{})
		resPointList := make([]interface{}, 0, len(points))
		for _, point := range points {
			resPointList = append(resPointList, rewritePoint(point.(map[string]interface{})))
		}
		resPoly = append(resPoly, resPointList)
	}
	return resPoly
}

// rewriteMultiPolygon constructs coordinates for MultiPolygon type.
// For MultiPolygon type, the mutation json is as follows:
//
//	{
//		"type": "MultiPolygon",
//		"coordinates": [[[[22.22,11.11],[16.16,15.15],[21.21,20.2]],[[22.28,11.18],[16.18,15.18],[21.28,20.28]]],
//	 	[[[92.22,91.11],[16.16,15.15],[21.21,20.2]],[[22.28,11.18],[16.18,15.18],[21.28,20.28]]]]
//	}
func rewriteMultiPolygon(val map[string]interface{}) []interface{} {
	// type casting this is safe, because of strict GraphQL schema
	polygons := val[schema.Polygons].([]interface{})
	res := make([]interface{}, 0, len(polygons))
	for _, polygon := range polygons {
		res = append(res, rewritePolygon(polygon.(map[string]interface{})))
	}
	return res
}

func checkQueryResult(qry string, yes, no error) resultChecker {
	return func(m map[string]interface{}) error {
		if val, exists := m[qry]; exists && val != nil {
			if data, ok := val.([]interface{}); ok && len(data) > 0 {
				return yes
			}
		}
		return no
	}
}

// addAdditionalDeletes creates any additional deletes that are needed when a reference changes.
// E.g. if we have
// type Post { ... author: Author @hasInverse(field: posts) ... }
// type Author { ... posts: [Post] ... }
// then if edge
// Post1 --- author --> Author1
// exists, there must also be edge
// Author1 --- posts --> Post1
// So if we did an update that changes the author of Post1 to Author2, we need to
// * add edge Post1 --- author --> Author2 (done by asIDReference/asXIDReference)
// * add edge Author2 --- posts --> Post1 (done by addInverseLink)
// * delete edge Author1 --- posts --> Post1 (done here by addAdditionalDeletes)
//
// This delete only needs to be done for singular edges - i.e. it doesn't need to be
// done when we add a new post to an author; that just adds new edges and doesn't
// leave an edge.
func addAdditionalDeletes(
	ctx context.Context,
	frag *mutationFragment,
	varGen *VariableGenerator,
	srcField schema.FieldDefinition,
	srcUID, variable string) {

	if srcField == nil {
		return
	}

	invField := srcField.Inverse()
	if invField == nil {
		return
	}

	addDelete(ctx, frag, varGen, variable, srcUID, invField, srcField)
	addDelete(ctx, frag, varGen, srcUID, variable, srcField, invField)
}

// addDelete adds a delete to the mutation if adding/updating an edge will cause another
// edge to disappear (see notes at addAdditionalDeletes)
//
// e.g. we have edges
// Post2 --- author --> Author3
// Author3 --- posts --> Post2
//
// we are about to attach
//
// Post2 --- author --> Author1
//
// # So Post2 should get removed from Author3's posts edge
//
// qryVar - is the variable storing Post2's uid
// excludeVar - is the uid we might have to exclude from the query
//
// e.g. if qryVar = Post2, we'll generate
//
//	query {
//	  ...
//		 var(func: uid(Post2)) {
//		  Author3 as Post.author
//		 }
//	 }
//
// and delete Json
//
// { "uid": "uid(Author3)", "Author.posts": [ { "uid": "uid(Post2)" } ] }
//
// removing the post from Author3
//
// but if there's a chance (e.g. during an update) that Author1 and Author3 are the same
// e.g. the update isn't really changing an existing edge, we have to definitely not
// do the delete. So we add a condition using the excludeVar
//
//	var(func: uid(Post2)) {
//	 Author3 as Post.author @filter(NOT(uid(Author1)))
//	}
//
// and the delete won't run.
func addDelete(
	ctx context.Context,
	frag *mutationFragment,
	varGen *VariableGenerator,
	qryVar, excludeVar string,
	qryFld, delFld schema.FieldDefinition) {

	// only add the delete for singular edges
	if qryFld.Type().ListType() != nil {
		return
	}

	if strings.HasPrefix(qryVar, "_:") {
		return
	}

	if strings.HasPrefix(qryVar, "uid(") {
		qryVar = qryVar[4 : len(qryVar)-1]
	}

	targetVar := varGen.Next(qryFld.Type(), "", "", false)
	delFldName := qryFld.Type().DgraphPredicate(delFld.Name())

	qry := &dql.GraphQuery{
		Attr: "var",
		Func: &dql.Function{
			Name: "uid",
			Args: []dql.Arg{{Value: qryVar}},
		},
		Children: []*dql.GraphQuery{{
			Var:  targetVar,
			Attr: delFld.Type().DgraphPredicate(qryFld.Name()),
		}},
	}

	exclude := excludeVar
	if strings.HasPrefix(excludeVar, "uid(") {
		exclude = excludeVar[4 : len(excludeVar)-1]
	}

	// We shouldn't do the delete if it ends up that the mutation is linking to the existing
	// value for this edge in Dgraph - otherwise (because there's a non-deterministic order
	// in executing set and delete) we might end up deleting the value in a set mutation.
	//
	// The only time that we always remove the edge and not check is a new node: e.g.
	// excludeVar is a blank node like _:Author1.   E.g. if
	// Post2 --- author --> Author3
	// Author3 --- posts --> Post2
	// is in the graph and we are creating a new node _:Author1 ... there's no way
	// Author3 and _:Author1 can be the same uid, so the check isn't required.
	if !strings.HasPrefix(excludeVar, "_:") {
		qry.Children[0].Filter = &dql.FilterTree{
			Op: "not",
			Child: []*dql.FilterTree{{
				Func: &dql.Function{
					Name: "uid",
					Args: []dql.Arg{{Value: exclude}}}}},
		}
	}

	frag.queries = append(frag.queries, qry)

	del := qryVar
	// Add uid around qryVar in case qryVar is not UID.
	if _, err := asUID(qryVar); err != nil {
		del = fmt.Sprintf("uid(%s)", qryVar)
	}

	if delFld.Type().ListType() == nil {
		frag.deletes = append(frag.deletes,
			map[string]interface{}{
				"uid":      fmt.Sprintf("uid(%s)", targetVar),
				delFldName: map[string]interface{}{"uid": del}})
	} else {
		frag.deletes = append(frag.deletes,
			map[string]interface{}{
				"uid":      fmt.Sprintf("uid(%s)", targetVar),
				delFldName: []interface{}{map[string]interface{}{"uid": del}}})
	}

	// If the type that we are adding the edge removal for has auth on it, we need to check
	// that we have permission to update it.  E.G. (see example at top)
	// if we end up needing to remove edge
	//  Author1 --- posts --> Post1
	// then we need update permission on Author1

	// grab the auth for Author1
	customClaims, err := qryFld.GetAuthMeta().ExtractCustomClaims(ctx)
	if err != nil {
		frag.check =
			checkQueryResult("auth.failed", nil, schema.GQLWrapf(err, "authorization failed"))
		return
	}

	newRw := &authRewriter{
		authVariables: customClaims.AuthVariables,
		varGen:        varGen,
		varName:       targetVar,
		selector:      updateAuthSelector,
		parentVarName: qryFld.Type().Name() + "Root",
	}
	if rn := newRw.selector(qryFld.Type()); rn != nil {
		newRw.hasAuthRules = true
	}

	authQueries, authFilter := newRw.rewriteAuthQueries(qryFld.Type())
	if len(authQueries) == 0 {
		// there's no auth to add for this type
		return
	}

	// There's already a query block like this added above
	// var(func: uid(Post3)) {
	//   Author4 as Post.author
	// }
	//
	// We'll bring out Author4 to a query so we can check it's length against the auth query.
	//
	// Author4(func: uid(Author4))
	// Author4.auth(func: uid(Auth4)) @filter(...auth filter...)
	// Author5, Author6, etc. ... auth queries...

	frag.queries = append(frag.queries,
		&dql.GraphQuery{
			Attr: targetVar,
			Func: &dql.Function{
				Name: "uid",
				Args: []dql.Arg{{Value: targetVar}}},
			Children: []*dql.GraphQuery{{Attr: "uid"}}},
		&dql.GraphQuery{
			Attr: targetVar + ".auth",
			Func: &dql.Function{
				Name: "uid",
				Args: []dql.Arg{{Value: targetVar}}},
			Filter:   authFilter,
			Children: []*dql.GraphQuery{{Attr: "uid"}}})

	frag.queries = append(frag.queries, authQueries...)

	frag.check = authCheck(frag.check, targetVar)
}

func authCheck(chk resultChecker, qry string) resultChecker {
	return func(m map[string]interface{}) error {

		if val, exists := m[qry]; exists && val != nil {
			if data, ok := val.([]interface{}); ok && len(data) > 0 {
				// There was an existing node ... did it pass auth?

				authVal, authExists := m[qry+".auth"]
				if !authExists || authVal == nil {
					return x.GqlErrorf("authorization failed")
				}

				if authData, ok := authVal.([]interface{}); ok && len(authData) != len(data) {
					return x.GqlErrorf("authorization failed")
				}

				// auth passed, but still need to check the existing conditions

				return chk(m)
			}
		}

		// There was no existing node, so auth wasn't needed, but still need to
		// apply the existing check function
		return chk(m)
	}
}

func attachChild(res map[string]interface{}, parent schema.Type, child schema.FieldDefinition, childUID string) {
	if parent == nil {
		return
	}
	if child.Type().ListType() != nil {
		res[parent.DgraphPredicate(child.Name())] =
			[]interface{}{map[string]interface{}{"uid": childUID}}
	} else {
		res[parent.DgraphPredicate(child.Name())] = map[string]interface{}{"uid": childUID}
	}
}

func deleteInverseObject(obj map[string]interface{}, srcField schema.FieldDefinition) {
	if srcField != nil {
		invField := srcField.Inverse()
		if invField != nil && invField.Type().ListType() == nil {
			delete(obj, invField.Name())
		}
	}
}

func addInverseLink(obj map[string]interface{}, srcField schema.FieldDefinition, srcUID string) {
	if srcField != nil {
		invField := srcField.Inverse()
		if invField != nil {
			attachChild(obj, srcField.Type(), invField, srcUID)
		}
	}
}

func newFragment(f interface{}) *mutationFragment {
	return &mutationFragment{
		fragment: f,
		check:    func(m map[string]interface{}) error { return nil },
		newNodes: make(map[string]schema.Type),
	}
}

func copyTypeMap(from, to map[string]schema.Type) {
	for name, typ := range from {
		to[name] = typ
	}
}

func extractVal(xidVal interface{}, xidName, typeName string) (string, error) {
	switch typeName {
	case "Int":
		switch xVal := xidVal.(type) {
		case json.Number:
			val, err := xVal.Int64()
			if err != nil {
				return "", err
			}
			return strconv.FormatInt(val, 10), nil
		case int64:
			return strconv.FormatInt(xVal, 10), nil
		default:
			return "", fmt.Errorf("encountered an XID %s with %s that isn't "+
				"a Int but data type in schema is Int", xidName, typeName)
		}
	case "Int64":
		switch xVal := xidVal.(type) {
		case json.Number:
			val, err := xVal.Int64()
			if err != nil {
				return "", err
			}
			return strconv.FormatInt(val, 10), nil
		case int64:
			return strconv.FormatInt(xVal, 10), nil
		// If the xid field is of type Int64, both String and Int forms are allowed.
		case string:
			return xVal, nil
		default:
			return "", fmt.Errorf("encountered an XID %s with %s that isn't "+
				"a Int64 but data type in schema is Int64", xidName, typeName)
		}
		// "ID" is given as input for the @extended type mutation.
	case "String", "ID":
		xidString, ok := xidVal.(string)
		if !ok {
			return "", fmt.Errorf("encountered an XID %s with %s that isn't "+
				"a String", xidName, typeName)
		}
		return xidString, nil
	default:
		return "", fmt.Errorf("encountered an XID %s with %s that isn't"+
			"allowed as Xid", xidName, typeName)
	}
}
