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
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	dgoapi "github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/authorization"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/x"

	"github.com/pkg/errors"
)

const (
	MutationQueryVar        = "x"
	MutationQueryVarUID     = "uid(x)"
	updateMutationCondition = `gt(len(x), 0)`
)

type AddRewriter struct {
	frags [][]*mutationFragment
}
type UpdateRewriter struct {
	setFrags []*mutationFragment
	delFrags []*mutationFragment
}
type deleteRewriter struct{}

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
	queries    []*gql.GraphQuery
	conditions []string
	fragment   interface{}
	deletes    []interface{}
	check      resultChecker
	newNodes   map[string]schema.Type
	err        error
}

// xidMetadata is used to handle cases where we get multiple objects which have same xid value in a
// single mutation
type xidMetadata struct {
	// variableObjMap stores the mapping of xidVariable -> the input object which contains that xid
	variableObjMap map[string]interface{}
	// seenAtTopLevel tells whether the xidVariable has been previously seen at top level or not
	seenAtTopLevel map[string]bool
	// queryExists tells whether the query part in upsert has already been created for xidVariable
	queryExists map[string]bool
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
		key = typ.FieldOriginatedFrom(xidName) + xidVal
	}

	if varName, ok := v.xidVarNameMap[key]; ok {
		return varName
	}

	// create new variable name
	v.counter++
	var varName string
	if auth {
		varName = fmt.Sprintf("%sAuth%v", typ.Name(), v.counter)
	} else {
		varName = fmt.Sprintf("%s%v", typ.Name(), v.counter)
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

// newXidMetadata returns a new empty *xidMetadata for storing the metadata.
func newXidMetadata() *xidMetadata {
	return &xidMetadata{
		variableObjMap: make(map[string]interface{}),
		seenAtTopLevel: make(map[string]bool),
		queryExists:    make(map[string]bool),
	}
}

// Rewrite takes a GraphQL schema.Mutation add and builds a Dgraph upsert mutation.
// m must have a single argument called 'input' that carries the mutation data.
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
// {
//   name: "A.N. Author",
//   country: { code: "ind", name: "India" },
//   posts: [ { title: "A Post", text: "Some text" }]
//   friends: [ { id: "0x123" } ]
// }
//
// becomes a guarded upsert with two possible paths - one if "ind" already exists
// and the other if we create "ind" as part of the mutation.
//
// Query:
// query {
//   Author4 as Author4(func: uid(0x123)) @filter(type(Author)) {
//     uid
//   }
//   Country2 as Country2(func: eq(Country.code, "ind")) @filter(type(Country)) {
//     uid
//   }
// }
//
// And two conditional mutations.  Both create a new post and check that the linked
// friend is an Author.  One links to India if it exists, the other creates it
//
// "@if(eq(len(Country2), 0) AND eq(len(Author4), 1))"
// {
//   "uid":"_:Author1"
//   "dgraph.type":["Author"],
//   "Author.name":"A.N. Author",
//   "Author.country":{
//     "uid":"_:Country2",
//     "dgraph.type":["Country"],
//     "Country.code":"ind",
//     "Country.name":"India"
//   },
//   "Author.posts": [ {
//     "uid":"_:Post3"
//     "dgraph.type":["Post"],
//     "Post.text":"Some text",
//     "Post.title":"A Post",
//   } ],
//   "Author.friends":[ {"uid":"0x123"} ],
// }
//
// and @if(eq(len(Country2), 1) AND eq(len(Author4), 1))
// {
//   "uid":"_:Author1",
//   "dgraph.type":["Author"],
//   "Author.name":"A.N. Author",
//   "Author.country": {
//     "uid":"uid(Country2)"
//   },
//   "Author.posts": [ {
//     "uid":"_:Post3"
//     "dgraph.type":["Post"],
//     "Post.text":"Some text",
//     "Post.title":"A Post",
//   } ],
//   "Author.friends":[ {"uid":"0x123"} ],
// }
func (mrw *AddRewriter) Rewrite(ctx context.Context, m schema.Mutation) ([]*UpsertMutation, error) {
	mutatedType := m.MutatedType()
	val, _ := m.ArgValue(schema.InputArgName).([]interface{})

	varGen := NewVariableGenerator()
	xidMd := newXidMetadata()
	var errs error

	mutationsAllSec := []*dgoapi.Mutation{}
	queriesSec := &gql.GraphQuery{}

	mutationsAll := []*dgoapi.Mutation{}
	queries := &gql.GraphQuery{}

	buildMutations := func(mutationsAll []*dgoapi.Mutation, queries *gql.GraphQuery,
		frag []*mutationFragment) []*dgoapi.Mutation {
		mutations, err := mutationsFromFragments(
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

		errs = schema.AppendGQLErrs(errs, schema.GQLWrapf(err,
			"failed to rewrite mutation payload"))

		mutationsAll = append(mutationsAll, mutations...)
		qry := queryFromFragments(frag)
		if qry != nil {
			queries.Children = append(queries.Children, qry.Children...)
		}

		return mutationsAll
	}

	for _, i := range val {
		obj := i.(map[string]interface{})
		frag := rewriteObject(ctx, nil, mutatedType, nil, "", varGen, true, obj, 0, xidMd)
		mrw.frags = append(mrw.frags, frag.secondPass)

		mutationsAll = buildMutations(mutationsAll, queries, frag.firstPass)
		mutationsAllSec = buildMutations(mutationsAllSec, queriesSec, frag.secondPass)
	}

	if len(queries.Children) == 0 {
		queries = nil
	}

	if len(queriesSec.Children) == 0 {
		queriesSec = nil
	}

	newNodes := make(map[string]schema.Type)
	for _, f := range mrw.frags {
		// squashFragments puts all the new nodes into the first fragment, so we only
		// need to collect from there.
		copyTypeMap(f[0].newNodes, newNodes)
	}

	result := []*UpsertMutation{}

	if len(mutationsAll) > 0 {
		result = append(result, &UpsertMutation{
			Query:     queries,
			Mutations: mutationsAll,
		})
	}

	if len(mutationsAllSec) > 0 {
		result = append(result, &UpsertMutation{
			Query:     queriesSec,
			Mutations: mutationsAllSec,
			NewNodes:  newNodes,
		})
	}

	return result, errs
}

// FromMutationResult rewrites the query part of a GraphQL add mutation into a Dgraph query.
func (mrw *AddRewriter) FromMutationResult(
	ctx context.Context,
	mutation schema.Mutation,
	assigned map[string]string,
	result map[string]interface{}) (*gql.GraphQuery, error) {

	var errs error

	uids := make([]uint64, 0)

	for _, frag := range mrw.frags {
		err := checkResult(frag, result)
		errs = schema.AppendGQLErrs(errs, err)
		if err != nil {
			continue
		}

		node := strings.TrimPrefix(frag[0].
			fragment.(map[string]interface{})["uid"].(string), "_:")
		val, ok := assigned[node]
		if !ok {
			continue
		}
		uid, err := strconv.ParseUint(val, 0, 64)
		if err != nil {
			errs = schema.AppendGQLErrs(errs, schema.GQLWrapf(err,
				"received %s as an assigned uid from Dgraph,"+
					" but couldn't parse it as uint64",
				assigned[node]))
		}

		uids = append(uids, uid)
	}

	if len(assigned) == 0 && errs == nil {
		errs = schema.AsGQLErrors(errors.Errorf("no new node was created"))
	}

	customClaims, err := authorization.ExtractCustomClaims(ctx)
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

	return rewriteAsQueryByIds(mutation.QueryField(), uids, authRw), errs
}

// Rewrite rewrites set and remove update patches into GraphQL+- upsert mutations.
// The GraphQL updates look like:
//
// input UpdateAuthorInput {
// 	filter: AuthorFilter!
// 	set: PatchAuthor
// 	remove: PatchAuthor
// }
//
// which gets rewritten in to a Dgraph upsert mutation
// - filter becomes the query
// - set becomes the Dgraph set mutation
// - remove becomes the Dgraph delete mutation
//
// The semantics is the same as the Dgraph mutation semantics.
// - Any values in set become the new values for those predicates (or add to the existing
//   values for lists)
// - Any nulls in set are ignored.
// - Explicit values in remove mean delete this if it is the actual value
// - Nulls in remove become like delete * for the corresponding predicate.
//
// See AddRewriter for how the set and remove fragments get created.
func (urw *UpdateRewriter) Rewrite(
	ctx context.Context,
	m schema.Mutation) ([]*UpsertMutation, error) {

	mutatedType := m.MutatedType()

	inp := m.ArgValue(schema.InputArgName).(map[string]interface{})
	setArg := inp["set"]
	delArg := inp["remove"]

	if setArg == nil && delArg == nil {
		return nil, nil
	}

	varGen := NewVariableGenerator()

	customClaims, err := authorization.ExtractCustomClaims(ctx)
	if err != nil {
		return nil, err
	}

	authRw := &authRewriter{
		authVariables: customClaims.AuthVariables,
		varGen:        varGen,
		selector:      updateAuthSelector,
		parentVarName: m.MutatedType().Name() + "Root",
	}
	authRw.hasAuthRules = hasAuthRules(m.QueryField(), authRw)

	upsertQuery := RewriteUpsertQueryFromMutation(m, authRw)
	srcUID := MutationQueryVarUID

	xidMd := newXidMetadata()
	var errs error

	buildMutation := func(setFrag, delFrag []*mutationFragment) *UpsertMutation {
		var mutSet, mutDel []*dgoapi.Mutation
		queries := []*gql.GraphQuery{upsertQuery}

		if setArg != nil {
			addUpdateCondition(setFrag)
			var errSet error
			mutSet, errSet = mutationsFromFragments(
				setFrag,
				func(frag *mutationFragment) ([]byte, error) {
					return json.Marshal(frag.fragment)
				},
				func(frag *mutationFragment) ([]byte, error) {
					if len(frag.deletes) > 0 {
						return json.Marshal(frag.deletes)
					}
					return nil, nil
				})

			urw.setFrags = append(urw.setFrags, setFrag...)
			errs = schema.AppendGQLErrs(errs, errSet)

			q1 := queryFromFragments(setFrag)
			if q1 != nil {
				queries = append(queries, q1.Children...)
			}
		}

		if delArg != nil {
			addUpdateCondition(delFrag)
			var errDel error
			mutDel, errDel = mutationsFromFragments(
				delFrag,
				func(frag *mutationFragment) ([]byte, error) {
					return nil, nil
				},
				func(frag *mutationFragment) ([]byte, error) {
					return json.Marshal(frag.fragment)
				})

			urw.delFrags = append(urw.delFrags, delFrag...)
			errs = schema.AppendGQLErrs(errs, errDel)

			q2 := queryFromFragments(delFrag)
			if q2 != nil {
				queries = append(queries, q2.Children...)
			}
		}

		newNodes := make(map[string]schema.Type)
		if urw.setFrags != nil {
			copyTypeMap(urw.setFrags[0].newNodes, newNodes)
		}
		if urw.delFrags != nil {
			copyTypeMap(urw.delFrags[0].newNodes, newNodes)
		}

		return &UpsertMutation{
			Query:     &gql.GraphQuery{Children: queries},
			Mutations: append(mutSet, mutDel...),
			NewNodes:  newNodes,
		}
	}

	var setFragF, setFragS, delFragF, delFragS []*mutationFragment

	if setArg != nil {
		setFrag := rewriteObject(ctx, nil, mutatedType, nil, srcUID, varGen, true,
			setArg.(map[string]interface{}), 0, xidMd)

		setFragF = setFrag.firstPass
		setFragS = setFrag.secondPass
	}

	if delArg != nil {
		delFrag := rewriteObject(ctx, nil, mutatedType, nil, srcUID, varGen, false,
			delArg.(map[string]interface{}), 0, xidMd)
		delFragF = delFrag.firstPass
		delFragS = delFrag.secondPass
	}

	result := []*UpsertMutation{}

	firstPass := buildMutation(setFragF, delFragF)
	if len(firstPass.Mutations) > 0 {
		result = append(result, firstPass)
	}

	secondPass := buildMutation(setFragS, delFragS)
	if len(secondPass.Mutations) > 0 {
		result = append(result, secondPass)
	}

	return result, schema.GQLWrapf(errs, "failed to rewrite mutation payload")
}

// FromMutationResult rewrites the query part of a GraphQL update mutation into a Dgraph query.
func (urw *UpdateRewriter) FromMutationResult(
	ctx context.Context,
	mutation schema.Mutation,
	assigned map[string]string,
	result map[string]interface{}) (*gql.GraphQuery, error) {

	err := checkResult(urw.setFrags, result)
	if err != nil {
		return nil, err
	}
	err = checkResult(urw.delFrags, result)
	if err != nil {
		return nil, err
	}

	mutated := extractMutated(result, mutation.Name())

	var uids []uint64
	if len(mutated) > 0 {
		// This is the case of a conditional upsert where we should get uids from mutated.
		for _, id := range mutated {
			uid, err := strconv.ParseUint(id, 0, 64)
			if err != nil {
				return nil, schema.GQLWrapf(err,
					"received %s as an updated uid from Dgraph, but couldn't parse it as "+
						"uint64", id)
			}
			uids = append(uids, uid)
		}
	}

	customClaims, err := authorization.ExtractCustomClaims(ctx)
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

func addUpdateCondition(frags []*mutationFragment) {
	for _, frag := range frags {
		frag.conditions = append(frag.conditions, updateMutationCondition)
	}
}

// checkResult checks if any mutationFragment in frags was successful in result.
// If any one of the frags (which correspond to conditional mutations) succeeded,
// then the mutation ran through ok.  Otherwise return an error showing why
// at least one of the mutations failed.
func checkResult(frags []*mutationFragment, result map[string]interface{}) error {
	if len(frags) == 0 {
		return nil
	}

	if result == nil {
		return nil
	}

	var err error
	for _, frag := range frags {
		err = frag.check(result)
		if err == nil {
			return nil
		}
	}

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

func RewriteUpsertQueryFromMutation(m schema.Mutation, authRw *authRewriter) *gql.GraphQuery {
	// The query needs to assign the results to a variable, so that the mutation can use them.
	dgQuery := &gql.GraphQuery{
		Var:  MutationQueryVar,
		Attr: m.Name(),
	}

	if m.MutatedType().InterfaceImplHasAuthRules() {
		dgQuery.Attr = m.ResponseName() + "()"
		return dgQuery
	}

	rbac := authRw.evaluateStaticRules(m.MutatedType())
	if rbac == schema.Negative {
		dgQuery.Attr = m.ResponseName() + "()"
		return dgQuery
	}
	// Add uid child to the upsert query, so that we can get the list of nodes upserted.
	dgQuery.Children = append(dgQuery.Children, &gql.GraphQuery{
		Attr: "uid",
	})

	// TODO - Cache this instead of this being a loop to find the IDField.
	filter := extractMutationFilter(m)
	if ids := idFilter(filter, m.MutatedType().IDField()); ids != nil {
		addUIDFunc(dgQuery, ids)
	} else {
		addTypeFunc(dgQuery, m.MutatedType().DgraphName())
	}

	addFilter(dgQuery, m.MutatedType(), filter)

	dgQuery = authRw.addAuthQueries(m.MutatedType(), dgQuery, rbac)

	return dgQuery
}

// removeNodeReference removes any reference we know about (via @hasInverse) into a node.
func removeNodeReference(m schema.Mutation, authRw *authRewriter,
	qry *gql.GraphQuery) []interface{} {
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
			&gql.GraphQuery{
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
	m schema.Mutation) ([]*UpsertMutation, error) {

	if m.MutationType() != schema.DeleteMutation {
		return nil, errors.Errorf(
			"(internal error) call to build delete mutation for %s mutation type",
			m.MutationType())
	}

	varGen := NewVariableGenerator()

	customClaims, err := authorization.ExtractCustomClaims(ctx)
	if err != nil {
		return nil, err
	}

	authRw := &authRewriter{
		authVariables: customClaims.AuthVariables,
		varGen:        varGen,
		selector:      deleteAuthSelector,
		parentVarName: m.MutatedType().Name() + "Root",
	}
	authRw.hasAuthRules = hasAuthRules(m.QueryField(), authRw)

	dgQry := RewriteUpsertQueryFromMutation(m, authRw)
	qry := dgQry
	if qry.Attr == "" {
		// Auth queries must have been added to the query, first query is the actual delete
		qry = dgQry.Children[0]
	}

	deletes := []interface{}{map[string]interface{}{"uid": "uid(x)"}}
	// We need to remove node reference only if auth rule succeeds.
	if qry.Attr != m.ResponseName()+"()" {
		// We need to delete the node and then any reference we know about (via @hasInverse)
		// into this node.
		deletes = append(deletes, removeNodeReference(m, authRw, qry)...)
	}

	b, err := json.Marshal(deletes)
	var finalQry *gql.GraphQuery
	// This rewrites the Upsert mutation so we can query the nodes before deletion. The query result
	// is later added to delete mutation result.
	if queryField := m.QueryField(); queryField.SelectionSet() != nil {
		queryAuthRw := &authRewriter{
			authVariables: customClaims.AuthVariables,
			varGen:        varGen,
			selector:      queryAuthSelector,
			filterByUid:   true,
		}
		queryAuthRw.parentVarName = queryAuthRw.varGen.Next(queryField.Type(), "", "",
			queryAuthRw.isWritingAuth)
		queryAuthRw.varName = MutationQueryVar
		queryAuthRw.hasAuthRules = hasAuthRules(queryField, authRw)

		queryDel := rewriteAsQuery(queryField, queryAuthRw)

		finalQry = &gql.GraphQuery{Children: append([]*gql.GraphQuery{dgQry}, queryDel)}
	} else {
		finalQry = dgQry
	}

	upsert := &UpsertMutation{
		Query:     finalQry,
		Mutations: []*dgoapi.Mutation{{DeleteJson: b}},
	}

	return []*UpsertMutation{upsert}, err
}

func (drw *deleteRewriter) FromMutationResult(
	ctx context.Context,
	mutation schema.Mutation,
	assigned map[string]string,
	result map[string]interface{}) (*gql.GraphQuery, error) {

	// There's no query that follows a delete
	return nil, nil
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

func mutationsFromFragments(
	frags []*mutationFragment,
	setBuilder, delBuilder mutationBuilder) ([]*dgoapi.Mutation, error) {

	mutations := make([]*dgoapi.Mutation, 0, len(frags))
	var errs x.GqlErrorList

	for _, frag := range frags {
		if frag.err != nil {
			errs = append(errs, schema.AsGQLErrors(frag.err)...)
			continue
		}

		var conditions string
		if len(frag.conditions) > 0 {
			conditions = fmt.Sprintf("@if(%s)", strings.Join(frag.conditions, " AND "))
		}

		set, err := setBuilder(frag)
		if err != nil {
			errs = append(errs, schema.AsGQLErrors(err)...)
			continue
		}

		del, err := delBuilder(frag)
		if err != nil {
			errs = append(errs, schema.AsGQLErrors(err)...)
			continue
		}

		mutations = append(mutations, &dgoapi.Mutation{
			SetJson:    set,
			DeleteJson: del,
			Cond:       conditions,
		})
	}

	var err error
	if len(errs) > 0 {
		err = errs
	}
	return mutations, err
}

func queryFromFragments(frags []*mutationFragment) *gql.GraphQuery {
	qry := &gql.GraphQuery{}
	for _, frag := range frags {
		qry.Children = append(qry.Children, frag.queries...)
	}

	if len(qry.Children) == 0 {
		return nil
	}

	return qry
}

type mutationRes struct {
	firstPass  []*mutationFragment
	secondPass []*mutationFragment
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
// e.g. a Post might have `title: String!`` in the schema, but,  a Post update could
// set that to to null. ATM we allow this and it'll just triggers GraphQL error propagation
// when that is in a query result.  This is the same case as deletes: e.g. deleting
// an author might make the `author: Author!` field of a bunch of Posts invalid.
// (That might actually be helpful if you want to run one mutation to remove something
// and then another to correct it.)
//
// rewriteObject returns two set of mutations, firstPass and secondPass. We start
// building mutations recursively in the secondPass. Whenever we encounter an XID object,
// we push it to firstPass. We need to make sure that the XID doesn't refer hasInverse links
// to secondPass, and then to make those links ourselves.
func rewriteObject(
	ctx context.Context,
	parentTyp schema.Type,
	typ schema.Type,
	srcField schema.FieldDefinition,
	srcUID string,
	varGen *VariableGenerator,
	withAdditionalDeletes bool,
	obj map[string]interface{},
	deepXID int,
	xidMetadata *xidMetadata) *mutationRes {

	atTopLevel := srcField == nil
	topLevelAdd := srcUID == ""

	variable := varGen.Next(typ, "", "", false)

	id := typ.IDField()
	if id != nil {
		if idVal, ok := obj[id.Name()]; ok {
			if idVal != nil {
				return &mutationRes{secondPass: []*mutationFragment{
					asIDReference(ctx, idVal, srcField, srcUID, variable,
						withAdditionalDeletes, varGen)}}
			}
			delete(obj, id.Name())
		}
	}

	var xidFrag *mutationFragment
	var xidString string
	xid := typ.XIDField()
	xidEncounteredFirstTime := false
	if xid != nil {
		if xidVal, ok := obj[xid.Name()]; ok && xidVal != nil {
			xidString, ok = xidVal.(string)
			if !ok {
				errFrag := newFragment(nil)
				errFrag.err = errors.New("encountered an XID that isn't a string")
				return &mutationRes{secondPass: []*mutationFragment{errFrag}}
			}
			// if the object has an xid, the variable name will be formed from the xidValue in order
			// to handle duplicate object addition/updation
			variable = varGen.Next(typ, xid.Name(), xidString, false)
			// check if an object with same xid has been encountered earlier
			if xidObj := xidMetadata.variableObjMap[variable]; xidObj != nil {
				// if we already encountered an object with same xid earlier, then we give error if:
				// 1. We are at top level and this object has already been seen at top level, as no
				//    duplicates are allowed for top level
				// 2. OR, we are in a deep mutation and:
				//		a. this obj is different from its first encounter
				//		b. OR, this object has a field which is inverse of srcField and that
				//		invField is not of List type
				var invField schema.FieldDefinition
				if srcField != nil {
					invField = srcField.Inverse()
				}
				if (atTopLevel && xidMetadata.seenAtTopLevel[variable]) || !reflect.DeepEqual(
					xidObj, obj) || (invField != nil && invField.Type().ListType() == nil) {
					errFrag := newFragment(nil)
					errFrag.err = errors.Errorf("duplicate XID found: %s", xidString)
					return &mutationRes{secondPass: []*mutationFragment{errFrag}}
				}
			} else {
				// if not encountered till now, add it to the map
				xidMetadata.variableObjMap[variable] = obj
				xidEncounteredFirstTime = true
			}
			// save if this variable was seen at top level
			if !xidMetadata.seenAtTopLevel[variable] {
				xidMetadata.seenAtTopLevel[variable] = atTopLevel
			}
		}

		deepXID += 1
	}

	var parentFrags []*mutationFragment

	if !atTopLevel { // top level is never a reference - it's a new addition.
		// this is the case of a lower level having xid which is a reference.
		if xid != nil && xidString != "" {
			xidFrag = asXIDReference(ctx, srcField, srcUID, typ, xid.Name(), xidString,
				variable, withAdditionalDeletes, varGen, xidMetadata)

			// Inverse Link is added as a Part of asXIDReference so we delete any provided
			// Link to the object.
			// Example: for this mutation
			// mutation addCountry($inp: AddCountryInput!) {
			// 	addCountry(input: [$inp]) {
			// 	  country {
			// 		id
			// 	  }
			// 	}
			//  }
			// with the input:
			//{
			// "inp": {
			// 	"name": "A Country",
			// 	"states": [
			// 	  { "code": "abc", "name": "Alphabet" },
			// 	  { "code": "def", "name": "Vowel", "country": { "name": "B country" } }
			// 	]
			//   }
			// }
			// we delete the link of Second state to "B Country"
			deleteInverseObject(obj, srcField)

			if deepXID > 2 {
				// Here we link the already existing node with an xid to the parent whose id is
				// passed in srcUID. We do this linking only if there is a hasInverse relationship
				// between the two.
				// So for example if we had the addAuthor mutation which is also adding nested
				// posts, then we link the authorUid(srcUID) - Author.posts - uid(Post) here.

				res := make(map[string]interface{}, 1)
				res["uid"] = srcUID
				attachChild(res, parentTyp, srcField, fmt.Sprintf("uid(%s)", variable))
				parentFrag := newFragment(res)
				parentFrag.conditions = append(parentFrag.conditions, xidFrag.conditions...)
				parentFrags = append(parentFrags, parentFrag)
			}
		} else if !withAdditionalDeletes {
			// In case of delete, id/xid is required
			var name string
			if xid != nil {
				name = xid.Name()
			} else {
				name = id.Name()
			}
			return &mutationRes{secondPass: invalidObjectFragment(fmt.Errorf("%s is not provided", name),
				xidFrag, variable, xidString)}
		}
	}

	if !atTopLevel && withAdditionalDeletes {
		// top level mutations are fully checked by GraphQL validation
		exclude := ""
		if srcField != nil {
			invField := srcField.Inverse()
			if invField != nil {
				exclude = invField.Name()
			}
		}
		if err := typ.EnsureNonNulls(obj, exclude); err != nil {
			// This object is either an invalid deep mutation or it's an xid reference
			// and asXIDReference must to apply or it's an error.
			return &mutationRes{secondPass: invalidObjectFragment(err, xidFrag, variable, xidString)}
		}
	}

	if !atTopLevel && !withAdditionalDeletes {
		// For remove op (!withAdditionalDeletes), we don't need to generate a new
		// blank node.
		if xidFrag != nil {
			return &mutationRes{secondPass: []*mutationFragment{xidFrag}}
		} else {
			return &mutationRes{}
		}
	}

	var myUID string
	newObj := make(map[string]interface{}, len(obj))

	if !atTopLevel || topLevelAdd {
		dgraphTypes := []string{typ.DgraphName()}
		dgraphTypes = append(dgraphTypes, typ.Interfaces()...)
		newObj["dgraph.type"] = dgraphTypes
		myUID = fmt.Sprintf("_:%s", variable)

		if xid == nil || deepXID > 2 {
			// If this object had an overwritten value for the inverse field, then we don't want to
			// use that value as we will add the link to the inverse field in the below
			// function call with the parent of this object
			// for example, for this mutation:
			// mutation addAuthor($auth: AddAuthorInput!) {
			// addAuthor(input: [$auth]) {
			// 	author {
			// 		id
			// 	}
			// 	}
			// }
			// with the following input
			//   {
			// 	"auth": {
			// 	  "name": "A.N. Author",
			// 	  "posts": [ { "postID": "0x456" }, {"title": "New Post", "author": {"name": "Abhimanyu"}} ]
			// 	}
			//   }
			// We delete the link of second input post with Author "name" : "Abhimanyu".
			deleteInverseObject(obj, srcField)

			// Lets link the new node that we are creating with the parent if a @hasInverse
			// exists between the two.
			// So for example if we had the addAuthor mutation which is also adding nested
			// posts, then we add the link _:Post Post.author AuthorUID(srcUID) here.
			addInverseLink(newObj, srcField, srcUID)

		}
	} else {
		myUID = srcUID
	}

	newObj["uid"] = myUID
	frag := newFragment(newObj)
	frag.newNodes[variable] = typ

	results := &mutationRes{secondPass: []*mutationFragment{frag}}
	if xid != nil && !atTopLevel && !xidEncounteredFirstTime && deepXID <= 2 {
		// If this is an xid that has been encountered before, e.g. think add mutations with
		// multiple objects as input. In that case we don't need to add the fragment to create this
		// object, so we clear it out. We do need other fragments for linking this node to its
		// parent which are added later.
		// If deepXID > 2 then even if the xid has been encountered before we still keep it and
		// build its mutation to cover all possible scenarios.
		results.secondPass = results.secondPass[:0]
	}

	// if xidString != "", then we are adding with an xid.  In which case, we have to ensure
	// as part of the upsert that the xid doesn't already exist.
	if xidString != "" {
		if atTopLevel && !xidMetadata.queryExists[variable] {
			// If not at top level, the query is already added by asXIDReference
			frag.queries = []*gql.GraphQuery{
				xidQuery(variable, xidString, xid.Name(), typ),
			}
			xidMetadata.queryExists[variable] = true
		}
		frag.conditions = []string{fmt.Sprintf("eq(len(%s), 0)", variable)}

		// We need to conceal the error because we might be leaking information to the user if it
		// tries to add duplicate data to the field with @id.
		var err error
		if queryAuthSelector(typ) == nil {
			err = x.GqlErrorf("id %s already exists for type %s", xidString, typ.Name())
		} else {
			// This error will only be reported in debug mode.
			err = x.GqlErrorf("GraphQL debug: id already exists for type %s", typ.Name())
		}
		frag.check = checkQueryResult(variable, err, nil)
	}

	if xid != nil && !atTopLevel {
		if deepXID <= 2 { // elements in firstPass or not
			// duplicate query in elements >= 2, as the pair firstPass element would already have
			// the same query.
			frag.queries = []*gql.GraphQuery{
				xidQuery(variable, xidString, xid.Name(), typ),
			}
		} else {
			// We need to link the parent to the element we are just creating
			res := make(map[string]interface{}, 1)
			res["uid"] = srcUID
			this := fmt.Sprintf("_:%s", variable)
			attachChild(res, parentTyp, srcField, this)

			parentFrag := newFragment(res)
			parentFrag.conditions = append(parentFrag.conditions, frag.conditions...)
			parentFrags = append(parentFrags, parentFrag)
		}
	}

	var childrenFirstPass []*mutationFragment
	// we build the mutation to add object here. If XID != nil, we would then move it to
	// firstPass from secondPass (frag).

	// if this object has an xid, then we don't need to
	// rewrite its children if we have encountered it earlier.

	// For deepXIDs even if the xid has been encountered before, we should build the mutation for
	// this object.
	if xidString == "" || xidEncounteredFirstTime || deepXID > 2 {
		var fields []string
		for field := range obj {
			fields = append(fields, field)
		}
		sort.Strings(fields)

		for _, field := range fields {
			val := obj[field]
			var frags *mutationRes

			fieldDef := typ.Field(field)
			fieldName := typ.DgraphPredicate(field)

			// This fixes mutation when dgraph predicate has special characters. PR #5526
			if strings.HasPrefix(fieldName, "<") && strings.HasSuffix(fieldName, ">") {
				fieldName = fieldName[1 : len(fieldName)-1]
			}

			switch val := val.(type) {
			case map[string]interface{}:
				// This field is another GraphQL object, which could either be linking to an
				// existing node by it's ID
				// { "title": "...", "author": { "id": "0x123" }
				//          like here ^^
				// or giving the data to create the object as part of a deep mutation
				// { "title": "...", "author": { "username": "new user", "dob": "...", ... }
				//          like here ^^
				if fieldDef.Type().IsPoint() {
					// For Point type, the mutation json in Dgraph is as follows:
					// { "type": "Point", "coordinates": [11.11, 22.22]}
					lat := val["latitude"]
					long := val["longitude"]
					frags = &mutationRes{
						secondPass: []*mutationFragment{
							newFragment(
								map[string]interface{}{
									"type":        "Point",
									"coordinates": []interface{}{long, lat},
								},
							),
						},
					}
				} else {
					frags =
						rewriteObject(ctx, typ, fieldDef.Type(), fieldDef, myUID, varGen,
							withAdditionalDeletes, val, deepXID, xidMetadata)
				}

			case []interface{}:
				// This field is either:
				// 1) A list of objects: e.g. if the schema said `categories: [Categories]`
				//   Which can be references to existing objects
				//   { "title": "...", "categories": [ { "id": "0x123" }, { "id": "0x321" }, ...] }
				//            like here ^^                ^^
				//   Or a deep mutation that creates new objects
				//   { "title": "...", "categories": [ { "name": "new category", ... }, ... ] }
				//            like here ^^                ^^
				// 2) Or a list of scalars - e.g. if schema said `scores: [Float]`
				//   { "title": "...", "scores": [10.5, 9.3, ... ]
				//            like here ^^
				frags =
					rewriteList(ctx, typ, fieldDef.Type(), fieldDef, myUID, varGen,
						withAdditionalDeletes, val, deepXID, xidMetadata)
			default:
				// This field is either:
				// 1) a scalar value: e.g.
				//   { "title": "My Post", ... }
				// 2) a JSON null: e.g.
				//   { "text": null, ... }
				//   e.g. to remove the text or
				//   { "friends": null, ... }
				//   to remove all friends

				// Fields with `id` directive cannot have empty values.
				if fieldDef.HasIDDirective() && val == "" {
					errFrag := newFragment(nil)
					errFrag.err = fmt.Errorf("encountered an empty value for @id field `%s`", fieldName)
					return &mutationRes{secondPass: []*mutationFragment{errFrag}}
				}
				frags = &mutationRes{secondPass: []*mutationFragment{newFragment(val)}}
			}
			childrenFirstPass = appendFragments(childrenFirstPass, frags.firstPass)

			results.secondPass = squashFragments(squashIntoObject(fieldName), results.secondPass,
				frags.secondPass)
		}
	}

	// In the case of an XID, move the secondPass (creation mutation) to firstPass
	if xid != nil && !atTopLevel {
		results.firstPass = appendFragments(results.firstPass, results.secondPass)
		results.secondPass = []*mutationFragment{}
	}

	// add current conditions to all the new fragments from children.
	// childrens should only be addded when this level is true
	conditions := []string{}
	for _, i := range results.firstPass {
		conditions = append(conditions, i.conditions...)
	}

	for _, i := range childrenFirstPass {
		i.conditions = append(i.conditions, conditions...)
	}
	results.firstPass = appendFragments(results.firstPass, childrenFirstPass)

	// parentFrags are reverse links to parents. only applicable for when deepXID > 2
	results.firstPass = appendFragments(results.firstPass, parentFrags)

	// xidFrag contains the mutation to update object if it is present.
	// add it to secondPass if deepXID <= 2, otherwise firstPass for relevant hasInverse links.
	if xidFrag != nil && deepXID > 2 {
		results.firstPass = appendFragments(results.firstPass, []*mutationFragment{xidFrag})
	} else if xidFrag != nil {
		results.secondPass = appendFragments(results.secondPass, []*mutationFragment{xidFrag})
	}

	return results
}

func invalidObjectFragment(
	err error,
	xidFrag *mutationFragment,
	variable, xidString string) []*mutationFragment {

	if xidFrag != nil {
		xidFrag.check =
			checkQueryResult(variable,
				nil,
				schema.GQLWrapf(err,
					"xid \"%s\" doesn't exist and input object not well formed", xidString))

		return []*mutationFragment{xidFrag}
	}
	return []*mutationFragment{{err: err}}
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
// query :
// Author1 as Author1(func: uid(0x123)) @filter(type(Author)) { uid }
// condition :
// len(Author1) > 0
// mutation :
// { "uid": "XYZ", "title": "...", "author": { "id": "0x123", "posts": [ { "uid": "XYZ" } ] }, ... }
// asIDReference builds the fragment
// { "id": "0x123", "posts": [ { "uid": "XYZ" } ] }
func asIDReference(
	ctx context.Context,
	val interface{},
	srcField schema.FieldDefinition,
	srcUID, variable string,
	withAdditionalDeletes bool,
	varGen *VariableGenerator) *mutationFragment {

	result := make(map[string]interface{}, 2)
	frag := newFragment(result)

	uid, err := asUID(val)
	if err != nil {
		frag.err = err
		return frag
	}

	result["uid"] = val

	addInverseLink(result, srcField, srcUID)

	qry := &gql.GraphQuery{
		Var:      variable,
		Attr:     variable,
		UID:      []uint64{uid},
		Children: []*gql.GraphQuery{{Attr: "uid"}},
	}
	addTypeFilter(qry, srcField.Type())
	addUIDFunc(qry, []uint64{uid})

	frag.queries = []*gql.GraphQuery{qry}
	frag.conditions = []string{fmt.Sprintf("eq(len(%s), 1)", variable)}
	frag.check =
		checkQueryResult(variable,
			nil,
			errors.Errorf("ID \"%#x\" isn't a %s", uid, srcField.Type().Name()))

	if withAdditionalDeletes {
		addAdditionalDeletes(ctx, frag, varGen, srcField, srcUID, variable)
	}

	return frag
}

// asXIDReference makes a mutation fragment that resolves a reference to an XID.  There's
// a bit of extra mutation to build since if the original mutation contains a reference to
// another node, e.g it was say adding a Post with:
// { "title": "...", "author": { "username": "A-user" }, ... }
// and we'd gotten to here        ^^
// in rewriteObject with srcField = "author" srcUID = "XYZ"
// and the schema says that Post.author and Author.Posts are inverses of each other, then we need
// to make sure that inverse link is added/removed.  We have to make sure the Dgraph upsert
// mutation ends up like:
//
// query :
// Author1 as Author1(func: eq(username, "A-user")) @filter(type(Author)) { uid }
// condition :
// len(Author1) > 0
// mutation :
// { "uid": "XYZ", "title": "...", "author": { "id": "uid(Author1)", "posts": ...
// where asXIDReference builds the fragment
// { "id": "uid(Author1)", "posts": [ { "uid": "XYZ" } ] }
func asXIDReference(
	ctx context.Context,
	srcField schema.FieldDefinition,
	srcUID string,
	typ schema.Type,
	xidFieldName, xidString, xidVariable string,
	withAdditionalDeletes bool,
	varGen *VariableGenerator,
	xidMetadata *xidMetadata) *mutationFragment {

	result := make(map[string]interface{}, 2)
	frag := newFragment(result)

	result["uid"] = fmt.Sprintf("uid(%s)", xidVariable)

	addInverseLink(result, srcField, srcUID)

	// add the query only if it has not been added already, otherwise we will be assigning same
	// variable name more than once in queries, resulting in dgraph error
	if !xidMetadata.queryExists[xidVariable] {
		frag.queries = []*gql.GraphQuery{xidQuery(xidVariable, xidString, xidFieldName, typ)}
		xidMetadata.queryExists[xidVariable] = true
	}
	frag.conditions = []string{fmt.Sprintf("eq(len(%s), 1)", xidVariable)}
	frag.check = checkQueryResult(xidVariable,
		nil,
		errors.Errorf("ID \"%s\" isn't a %s", xidString, srcField.Type().Name()))

	if withAdditionalDeletes {
		addAdditionalDeletes(ctx, frag, varGen, srcField, srcUID, xidVariable)
	}

	return frag
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
	srcField schema.FieldDefinition, srcUID, variable string) {

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
// So Post2 should get removed from Author3's posts edge
//
// qryVar - is the variable storing Post2's uid
// excludeVar - is the uid we might have to exclude from the query
//
// e.g. if qryVar = Post2, we'll generate
//
// query {
//   ...
// 	 var(func: uid(Post2)) {
// 	  Author3 as Post.author
// 	 }
//  }
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
// 	 var(func: uid(Post2)) {
// 	  Author3 as Post.author @filter(NOT(uid(Author1)))
// 	 }
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

	qry := &gql.GraphQuery{
		Attr: "var",
		Func: &gql.Function{
			Name: "uid",
			Args: []gql.Arg{{Value: qryVar}},
		},
		Children: []*gql.GraphQuery{{
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
		qry.Children[0].Filter = &gql.FilterTree{
			Op: "not",
			Child: []*gql.FilterTree{{
				Func: &gql.Function{
					Name: "uid",
					Args: []gql.Arg{{Value: exclude}}}}},
		}
	}

	frag.queries = append(frag.queries, qry)

	del := fmt.Sprintf("uid(%s)", qryVar)
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
	customClaims, err := authorization.ExtractCustomClaims(ctx)
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
		&gql.GraphQuery{
			Attr: targetVar,
			Func: &gql.Function{
				Name: "uid",
				Args: []gql.Arg{{Value: targetVar}}},
			Children: []*gql.GraphQuery{{Attr: "uid"}}},
		&gql.GraphQuery{
			Attr: targetVar + ".auth",
			Func: &gql.Function{
				Name: "uid",
				Args: []gql.Arg{{Value: targetVar}}},
			Filter:   authFilter,
			Children: []*gql.GraphQuery{{Attr: "uid"}}})

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

func xidQuery(xidVariable, xidString, xidPredicate string, typ schema.Type) *gql.GraphQuery {
	qry := &gql.GraphQuery{
		Var:  xidVariable,
		Attr: xidVariable,
		Func: &gql.Function{
			Name: "eq",
			Args: []gql.Arg{
				{Value: typ.DgraphPredicate(xidPredicate)},
				{Value: maybeQuoteArg("eq", xidString)},
			},
		},
		Children: []*gql.GraphQuery{{Attr: "uid"}},
	}
	addTypeFilter(qry, typ)
	return qry
}

func rewriteList(
	ctx context.Context,
	parentTyp schema.Type,
	typ schema.Type,
	srcField schema.FieldDefinition,
	srcUID string,
	varGen *VariableGenerator,
	withAdditionalDeletes bool,
	objects []interface{},
	deepXID int,
	xidMetadata *xidMetadata) *mutationRes {

	result := &mutationRes{}
	result.secondPass = []*mutationFragment{newFragment(make([]interface{}, 0))}
	foundSecondPass := false

	for _, obj := range objects {
		switch obj := obj.(type) {
		case map[string]interface{}:
			frag := rewriteObject(ctx, parentTyp, typ, srcField, srcUID, varGen,
				withAdditionalDeletes, obj, deepXID, xidMetadata)
			if len(frag.secondPass) != 0 {
				foundSecondPass = true
			}
			result.firstPass = appendFragments(result.firstPass, frag.firstPass)
			result.secondPass = squashFragments(squashIntoList, result.secondPass, frag.secondPass)
		default:
			// All objects in the list must be of the same type.  GraphQL validation makes sure
			// of that. So this must be a list of scalar values (lists of lists aren't allowed).
			return &mutationRes{secondPass: []*mutationFragment{
				newFragment(objects),
			}}
		}
	}

	if len(objects) != 0 && !foundSecondPass {
		result.secondPass = nil
	}

	return result
}

func newFragment(f interface{}) *mutationFragment {
	return &mutationFragment{
		fragment: f,
		check:    func(m map[string]interface{}) error { return nil },
		newNodes: make(map[string]schema.Type),
	}
}

func squashIntoList(list, v interface{}, makeCopy bool) interface{} {
	if list == nil {
		return []interface{}{v}
	}
	asList := list.([]interface{})
	if makeCopy {
		cpy := make([]interface{}, len(asList), len(asList)+1)
		copy(cpy, asList)
		asList = cpy
	}
	return append(asList, v)
}

func squashIntoObject(label string) func(interface{}, interface{}, bool) interface{} {
	return func(object, v interface{}, makeCopy bool) interface{} {
		asObject := object.(map[string]interface{})
		if makeCopy {
			cpy := make(map[string]interface{}, len(asObject)+1)
			for k, v := range asObject {
				cpy[k] = v
			}
			asObject = cpy
		}

		val := v

		// If there is an existing value for the label in the object, then we should append to it
		// instead of overwriting it if the existing value is a list. This can happen when there
		// is @hasInverse and we are doing nested adds.
		existing := asObject[label]
		switch ev := existing.(type) {
		case []interface{}:
			switch vv := v.(type) {
			case []interface{}:
				ev = append(ev, vv...)
				val = ev
			case interface{}:
				ev = append(ev, vv)
				val = ev
			default:
			}
		default:
		}
		asObject[label] = val
		return asObject
	}
}

func appendFragments(left, right []*mutationFragment) []*mutationFragment {
	if len(left) == 0 {
		return right
	}

	if len(right) == 0 {
		return left
	}

	result := make([]*mutationFragment, len(left)+len(right))
	i := 0

	var queries []*gql.GraphQuery
	for _, l := range left {
		queries = append(queries, l.queries...)
		result[i] = l
		result[i].queries = []*gql.GraphQuery{}
		result[i].newNodes = make(map[string]schema.Type)
		i++
	}

	for _, r := range right {
		queries = append(queries, r.queries...)
		result[i] = r
		result[i].queries = []*gql.GraphQuery{}
		result[i].newNodes = make(map[string]schema.Type)
		i++
	}

	newNodes := make(map[string]schema.Type)
	for _, l := range left {
		copyTypeMap(l.newNodes, newNodes)
	}
	for _, r := range right {
		copyTypeMap(r.newNodes, newNodes)
	}

	result[0].newNodes = newNodes
	result[0].queries = queries

	return result
}

// squashFragments takes two lists of mutationFragments and produces a single list
// that has all the right fragments squashed into the left.
//
// In most cases, this is len(left) == 1 and len(right) == 1 and the result is a
// single fragment.  For example, if left is what we have built so far for adding a
// new author and to original input contained:
// {
//   ...
//   country: { id: "0x123" }
// }
// rewriteObject is called on `{ id: "0x123" }` to create a fragment with
// Query: CountryXYZ as CountryXYZ(func: uid(0x123)) @filter(type(Country)) { uid }
// Condition: eq(len(CountryXYZ), 1)
// Fragment: { id: "0x123" }
// In this case, we just need to add `country: { id: "0x123" }`, the query and condition
// to the left fragment and the result is a single fragment.  If there are no XIDs
// in the schema, only 1 fragment can ever be generated.  We can always tell if the
// mutation means to link to an existing object (because the ID value is present),
// or if the intention is to create a new object (because the ID value isn't there,
// that means it's not known client side), so there's never any need for more than
// one conditional mutation.
//
// However, if there are XIDs, there can be multiple possible mutations.
// For example, if schema has `Type Country { code: String! @id, name: String! ... }`
// and the mutation input is
// {
//   ...
//   country: { code: "ind", name: "India" }
// }
// we can't tell from the mutation text if this mutation means to link to an existing
// country or if it's a deep add on the XID `code: "ind"`.  If the mutation was
// `country: { code: "ind" }`, we'd know it's a link because they didn't supply
// all the ! fields to correctly create a new country, but from
// `country: { code: "ind", name: "India" }` we have to go to the DB to check.
// So rewriteObject called on `{ code: "ind", name: "India" }` produces two fragments
//
// Query: CountryXYZ as CountryXYZ(func: eq(code, "ind")) @filter(type(Country)) { uid }
//
// Fragment1 (if "ind" already exists)
//  Cond: eq(len(CountryXYZ), 1)
//  Fragment: { uid: uid(CountryXYZ) }
//
// and
//
// Fragment2 (if "ind" doesn't exist)
//  Cond eq(len(CountryXYZ), 0)
//  Fragment: { uid: uid(CountryXYZ), code: "ind", name: "India" }
//
// Now we have to squash this into what we've already built for the author (left
// mutationFragment).  That'll end up as a result with two fragments (two possible
// mutations guarded by conditions on if the country exists), and to do
// that, we'll need to make some copies, e.g., because we'll end up with
// country: { uid: uid(CountryXYZ) }
// in one fragment, and
// country: { uid: uid(CountryXYZ), code: "ind", name: "India" }
// in the other we need to copy what we've already built for the author to represent
// the different mutation payloads.  Same goes for the conditions.
func squashFragments(
	combiner func(interface{}, interface{}, bool) interface{},
	left, right []*mutationFragment) []*mutationFragment {

	if len(left) == 0 {
		return right
	}

	if len(right) == 0 {
		return left
	}

	result := make([]*mutationFragment, 0, len(left)*len(right))
	for _, l := range left {
		for _, r := range right {
			var conds []string
			var deletes []interface{}

			if len(l.conditions) > 0 {
				conds = make([]string, len(l.conditions), len(l.conditions)+len(r.conditions))
				copy(conds, l.conditions)
			}

			if len(l.deletes) > 0 {
				deletes = make([]interface{}, len(l.deletes), len(l.deletes)+len(r.deletes))
				copy(deletes, l.deletes)
			}

			result = append(result, &mutationFragment{
				conditions: append(conds, r.conditions...),
				deletes:    append(deletes, r.deletes...),
				fragment:   combiner(l.fragment, r.fragment, len(right) > 1),
				check: func(lcheck, rcheck resultChecker) resultChecker {
					return func(m map[string]interface{}) error {
						return schema.AppendGQLErrs(lcheck(m), rcheck(m))
					}
				}(l.check, r.check),
				err: schema.AppendGQLErrs(l.err, r.err),
			})
		}
	}

	// queries and node types don't need copying, they just need to be all collected
	// at the end, so accumulate them all into one of the result fragments
	var queries []*gql.GraphQuery
	for _, l := range left {
		queries = append(queries, l.queries...)
	}
	for _, r := range right {
		queries = append(queries, r.queries...)
	}

	newNodes := make(map[string]schema.Type)
	for _, l := range left {
		copyTypeMap(l.newNodes, newNodes)
	}
	for _, r := range right {
		copyTypeMap(r.newNodes, newNodes)
	}
	result[0].newNodes = newNodes
	result[0].queries = queries

	return result
}

func copyTypeMap(from, to map[string]schema.Type) {
	for name, typ := range from {
		to[name] = typ
	}
}
