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
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	dgoapi "github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
)

const (
	mutationQueryVar        = "x"
	deleteUIDVarMutation    = `{ "uid": "uid(x)" }`
	updateMutationCondition = `gt(len(x), 0)`
)

type addRewriter struct {
	frags []*mutationFragment
}
type updateRewriter struct {
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
	deletes    []interface{} // TODO: functionality for next PR
	check      func(map[string]interface{}) error
	err        error
}

// A mutationBuilder can build a json mutation []byte from a mutationFragment
type mutationBuilder func(frag *mutationFragment) ([]byte, error)

type counter int

func (c *counter) next() int {
	*c++
	return int(*c)
}

// NewAddRewriter returns new MutationRewriter for add & update mutations.
func NewAddRewriter() MutationRewriter {
	return &addRewriter{}
}

// NewUpdateRewriter returns new MutationRewriter for add & update mutations.
func NewUpdateRewriter() MutationRewriter {
	return &updateRewriter{}
}

// NewDeleteRewriter returns new MutationRewriter for delete mutations..
func NewDeleteRewriter() MutationRewriter {
	return &deleteRewriter{}
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
func (mrw *addRewriter) Rewrite(
	m schema.Mutation) (*gql.GraphQuery, []*dgoapi.Mutation, error) {

	mutatedType := m.MutatedType()

	val := m.ArgValue(schema.InputArgName).(map[string]interface{})

	counter := counter(0)
	mrw.frags = rewriteObject(mutatedType, nil, "", &counter, val)
	mutations, err := mutationsFromFragments(
		mrw.frags,
		func(frag *mutationFragment) ([]byte, error) {
			return json.Marshal(frag.fragment)
		},
		func(frag *mutationFragment) ([]byte, error) {
			if len(frag.deletes) > 0 {
				return json.Marshal(frag.deletes)
			}
			return nil, nil
		})

	return queryFromFragments(mrw.frags),
		mutations,
		schema.GQLWrapf(err, "failed to rewrite mutation payload")
}

// FromMutationResult rewrites the query part of a GraphQL add mutation into a Dgraph query.
func (mrw *addRewriter) FromMutationResult(
	mutation schema.Mutation,
	assigned map[string]string,
	result map[string]interface{}) (*gql.GraphQuery, error) {

	var uid uint64

	err := checkResult(mrw.frags, result)
	if err != nil {
		return nil, err
	}

	if len(assigned) == 0 {
		return nil, x.GqlErrorf("no new node was created")
	}

	node := strings.TrimPrefix(mrw.frags[0].fragment.(map[string]interface{})["uid"].(string), "_:")
	uid, err = strconv.ParseUint(assigned[node], 0, 64)
	if err != nil {
		return nil, schema.GQLWrapf(err,
			"received %s as an assigned uid from Dgraph, but couldn't parse it as uint64",
			assigned[node])
	}

	return rewriteAsGet(mutation.QueryField(), uid, nil), nil
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
// See addRewriter for how the set and remove fragments get created.
func (urw *updateRewriter) Rewrite(
	m schema.Mutation) (*gql.GraphQuery, []*dgoapi.Mutation, error) {

	mutatedType := m.MutatedType()

	inp := m.ArgValue(schema.InputArgName).(map[string]interface{})
	setArg := inp["set"]
	delArg := inp["remove"]

	if setArg == nil && delArg == nil {
		return nil, nil, nil
	}

	upsertQuery := rewriteUpsertQueryFromMutation(m)
	srcUID := fmt.Sprintf("uid(%s)", mutationQueryVar)

	var errSet, errDel error
	var mutSet, mutDel []*dgoapi.Mutation
	counter := counter(0)

	if setArg != nil {
		urw.setFrags =
			rewriteObject(mutatedType, nil, srcUID, &counter, setArg.(map[string]interface{}))
		addUpdateCondition(urw.setFrags)
		mutSet, errSet = mutationsFromFragments(
			urw.setFrags,
			func(frag *mutationFragment) ([]byte, error) {
				return json.Marshal(frag.fragment)
			},
			func(frag *mutationFragment) ([]byte, error) {
				if len(frag.deletes) > 0 {
					return json.Marshal(frag.deletes)
				}
				return nil, nil
			})
	}

	if delArg != nil {
		urw.delFrags =
			rewriteObject(mutatedType, nil, srcUID, &counter, delArg.(map[string]interface{}))
		addUpdateCondition(urw.delFrags)
		mutDel, errDel = mutationsFromFragments(
			urw.delFrags,
			func(frag *mutationFragment) ([]byte, error) {
				return nil, nil
			},
			func(frag *mutationFragment) ([]byte, error) {
				return json.Marshal(frag.fragment)
			})
	}

	queries := []*gql.GraphQuery{upsertQuery}

	q1 := queryFromFragments(urw.setFrags)
	if q1 != nil {
		queries = append(queries, q1.Children...)
	}

	q2 := queryFromFragments(urw.delFrags)
	if q2 != nil {
		queries = append(queries, q2.Children...)
	}

	return &gql.GraphQuery{Children: queries},
		append(mutSet, mutDel...),
		schema.GQLWrapf(schema.AppendGQLErrs(errSet, errDel), "failed to rewrite mutation payload")
}

// FromMutationResult rewrites the query part of a GraphQL update mutation into a Dgraph query.
func (urw *updateRewriter) FromMutationResult(
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

	mutated := extractMutated(result, mutation.ResponseName())

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

	return rewriteAsQueryByIds(mutation.QueryField(), uids), nil
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

func extractFilter(m schema.Mutation) map[string]interface{} {
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

func rewriteUpsertQueryFromMutation(m schema.Mutation) *gql.GraphQuery {
	// The query needs to assign the results to a variable, so that the mutation can use them.
	dgQuery := &gql.GraphQuery{
		Var:  mutationQueryVar,
		Attr: m.ResponseName(),
	}
	// Add uid child to the upsert query, so that we can get the list of nodes upserted.
	dgQuery.Children = append(dgQuery.Children, &gql.GraphQuery{
		Attr: "uid",
	})

	if ids := idFilter(m); ids != nil {
		addUIDFunc(dgQuery, ids)
	} else {
		addTypeFunc(dgQuery, m.MutatedType().DgraphName())
	}

	filter := extractFilter(m)
	addFilter(dgQuery, m.MutatedType(), filter)
	return dgQuery
}

func (drw *deleteRewriter) Rewrite(m schema.Mutation) (
	*gql.GraphQuery, []*dgoapi.Mutation, error) {
	if m.MutationType() != schema.DeleteMutation {

		return nil, nil, errors.Errorf(
			"(internal error) call to build delete mutation for %s mutation type",
			m.MutationType())
	}

	return rewriteUpsertQueryFromMutation(m),
		[]*dgoapi.Mutation{{
			DeleteJson: []byte(deleteUIDVarMutation),
		}},
		nil
}

func (drw *deleteRewriter) FromMutationResult(
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

func mutationsFromFragments(
	frags []*mutationFragment,
	setBuilder mutationBuilder,
	delBuilder mutationBuilder) ([]*dgoapi.Mutation, error) {

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

// rewriteObject rewrites obj to a list of mutation fragments.  See addRewriter.Rewrite
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
func rewriteObject(
	typ schema.Type,
	srcField schema.FieldDefinition,
	srcUID string,
	counter *counter,
	obj map[string]interface{}) []*mutationFragment {

	atTopLevel := srcField == nil
	topLevelAdd := srcUID == ""

	variable := fmt.Sprintf("%s%v", typ.Name(), counter.next())

	id := typ.IDField()
	if id != nil {
		if idVal, ok := obj[id.Name()]; ok {
			if idVal != nil {
				return []*mutationFragment{asIDReference(idVal, srcField, srcUID, variable)}
			}
			delete(obj, id.Name())
		}
	}

	var xidFrag *mutationFragment
	var xidString string
	xid := typ.XIDField()
	if xid != nil {
		if xidVal, ok := obj[xid.Name()]; ok && xidVal != nil {
			xidString, ok = xidVal.(string)
			if !ok {
				errFrag := newFragment(nil)
				errFrag.err = errors.New("encountered an XID that isn't a string")
				return []*mutationFragment{errFrag}
			}
		}
	}

	if !atTopLevel { // top level is never a reference - it's adding/updating
		if xid != nil && xidString != "" {
			xidFrag =
				asXIDReference(srcField, srcUID, typ, xid.Name(), xidString, variable)
		}
	}

	if !atTopLevel { // top level mutations are fully checked by GraphQL validation
		if err := typ.CheckValidity(obj); err != nil {
			// This object is either an invalid deep mutation or it's an xid reference
			// and asXIDReference must to apply or it's an error.
			return invalidObjectFragment(err, xidFrag, variable, xidString)
		}
	}

	var newObj map[string]interface{}
	var myUID string
	if !atTopLevel || topLevelAdd {
		newObj = make(map[string]interface{}, len(obj)+3)
		dgraphTypes := []string{typ.DgraphName()}
		dgraphTypes = append(dgraphTypes, typ.Interfaces()...)
		newObj["dgraph.type"] = dgraphTypes
		myUID = fmt.Sprintf("_:%s", variable)

		addInverseLink(newObj, srcField, srcUID)

	} else { // it's the top level of an update add/remove
		newObj = make(map[string]interface{}, len(obj))
		myUID = srcUID
	}
	newObj["uid"] = myUID

	frag := newFragment(newObj)
	results := []*mutationFragment{frag}

	// if xidString != "", then we are adding with an xid.  In which case, we have to ensure
	// as part of the upsert that the xid doesn't already exist.
	if xidString != "" {
		if atTopLevel {
			// If not at top level, the query is already added by asXIDReference
			frag.queries = []*gql.GraphQuery{
				xidQuery(variable, xidString, xid.Name(), typ),
			}
		}
		frag.conditions = []string{fmt.Sprintf("eq(len(%s), 0)", variable)}
		frag.check = func(m map[string]interface{}) error {
			if val, exists := m[variable]; exists && val != nil {
				if data, ok := val.([]interface{}); ok && len(data) > 0 {
					return x.GqlErrorf("id %s already exists for type %s", xidString, typ.Name())
				}
			}
			return nil
		}
	}

	for field, val := range obj {
		var frags []*mutationFragment

		fieldDef := typ.Field(field)
		fieldName := typ.DgraphPredicate(field)

		switch val := val.(type) {
		case map[string]interface{}:
			// This field is another GraphQL object, which could either be linking to an
			// existing node by it's ID
			// { "title": "...", "author": { "id": "0x123" }
			//          like here ^^
			// or giving the data to create the object as part of a deep mutation
			// { "title": "...", "author": { "username": "new user", "dob": "...", ... }
			//          like here ^^
			frags = rewriteObject(fieldDef.Type(), fieldDef, myUID, counter, val)
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
			frags = rewriteList(fieldDef.Type(), fieldDef, myUID, counter, val)
		default:
			// This field is either:
			// 1) a scalar value: e.g.
			//   { "title": "My Post", ... }
			// 2) a JSON null: e.g.
			//   { "text": null, ... }
			//   e.g. to remove the text or
			//   { "friends": null, ... }
			//   to remove all friends
			frags = []*mutationFragment{newFragment(val)}
		}

		results = squashFragments(squashIntoObject(fieldName), results, frags)
	}

	if xidFrag != nil {
		results = append(results, xidFrag)
	}

	return results
}

func invalidObjectFragment(
	err error,
	xidFrag *mutationFragment,
	variable, xidString string) []*mutationFragment {

	if xidFrag != nil {
		xidFrag.check = func(m map[string]interface{}) error {
			if _, exists := m[variable]; exists {
				return nil
			}
			return schema.GQLWrapf(err, "xid %s doesn't exist and input object not well formed", xidString)
		}
		return []*mutationFragment{xidFrag}
	}
	return []*mutationFragment{{err: err}}
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
	val interface{},
	srcField schema.FieldDefinition,
	srcUID string,
	variable string) *mutationFragment {

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
	frag.check = func(m map[string]interface{}) error {
		if val, exists := m[variable]; exists && val != nil {
			if data, ok := val.([]interface{}); ok && len(data) > 0 {
				return nil
			}
		}
		return errors.Errorf("ID %#x isn't a %s", uid, srcField.Type().Name())
	}

	return frag

	// FIXME: if this is an update we also need to add a query that checks if
	// an author exists, and add a mutation to remove this post from that author
	// query(func: uid(XYZ)) { a as author }
	// +delete mutation
	// { uid: uid(a), posts: [ uid: "XYZ"] }
	// this can only occur at top level, not deep
	//
	// mutation was
	// { "title": "...", "author": { "id": "0x123" }, ... }
	//
	// we'll build
	// query XYZ = ...
	// query is 123 an author
	// query(func: uid(XYZ)) { a as author(and not 123) }
	// { "uid": "XYZ", "title": "...",
	//   "author": { "id": "0x123", "posts": [ { "uid": "XYZ" } ] }, ... }
	// also
	// delete { uid: uid(a), posts: [ uid: uid(XYZ)] }
	//
	// ** only if author is single  - other wise it's always adding to existing edges. **
	// ** only if update set mutation **
	// ** Also in an add that links to an existing node **
	// same sort of thing if it's xid, not id
	//
	// should go in some sort of deletes list
	//
	// Can tell by the type ???
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
// Author1 as Author1(func: eq(username, "A-user")) @filter(Author) { uid }
// condition :
// len(Author1) > 0
// mutation :
// { "uid": "XYZ", "title": "...", "author": { "id": "uid(Author1)", "posts": ...
// where asXIDReference builds the fragment
// { "id": "uid(Author1)", "posts": [ { "uid": "XYZ" } ] }
func asXIDReference(
	srcField schema.FieldDefinition,
	srcUID string,
	typ schema.Type,
	xidFieldName, xidString, xidVariable string) *mutationFragment {

	result := make(map[string]interface{}, 2)
	frag := newFragment(result)

	result["uid"] = fmt.Sprintf("uid(%s)", xidVariable)

	addInverseLink(result, srcField, srcUID)

	frag.queries = []*gql.GraphQuery{xidQuery(xidVariable, xidString, xidFieldName, typ)}
	frag.conditions = []string{fmt.Sprintf("eq(len(%s), 1)", xidVariable)}
	frag.check = func(m map[string]interface{}) error {
		if _, exists := m[xidVariable]; exists {
			return nil
		}
		return errors.Errorf("ID %s isn't a %s", xidString, srcField.Type().Name())
	}

	// FIXME: and remove any existing

	return frag
}

func addInverseLink(obj map[string]interface{}, srcField schema.FieldDefinition, srcUID string) {
	if srcField != nil {
		invType, invField := srcField.Inverse()
		if invType != nil && invField != nil {
			if invField.Type().ListType() != nil {
				obj[invType.DgraphPredicate(invField.Name())] =
					[]interface{}{map[string]interface{}{"uid": srcUID}}
			} else {
				obj[invType.DgraphPredicate(invField.Name())] =
					map[string]interface{}{"uid": srcUID}
			}
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
	typ schema.Type,
	srcField schema.FieldDefinition,
	srcUID string,
	counter *counter,
	objects []interface{}) []*mutationFragment {

	frags := []*mutationFragment{newFragment(make([]interface{}, 0))}

	for _, obj := range objects {
		switch obj := obj.(type) {
		case map[string]interface{}:
			frags = squashFragments(squashIntoList, frags,
				rewriteObject(typ, srcField, srcUID, counter, obj))
		default:
			// All objects in the list must be of the same type.  GraphQL validation makes sure
			// of that. So this must be a list of scalar values (lists of lists aren't allowed).
			return []*mutationFragment{
				newFragment(objects),
			}
		}
	}

	return frags
}

func newFragment(f interface{}) *mutationFragment {
	return &mutationFragment{
		fragment: f,
		check:    func(m map[string]interface{}) error { return nil },
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
		asObject[label] = v
		return asObject
	}
}

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
			var qs []*gql.GraphQuery
			var conds []string

			// FIXME: don't copy queries ... should just be one for whole thing
			if len(l.queries) > 0 {
				qs = make([]*gql.GraphQuery, len(l.queries), len(l.queries)+len(r.queries))
				copy(qs, l.queries)
			}

			if len(l.conditions) > 0 {
				conds = make([]string, len(l.conditions), len(l.conditions)+len(r.conditions))
				copy(conds, l.conditions)
			}

			result = append(result, &mutationFragment{
				queries:    append(qs, r.queries...),
				conditions: append(conds, r.conditions...),
				fragment:   combiner(l.fragment, r.fragment, len(right) > 1),
				check: func(m map[string]interface{}) error {
					return schema.AppendGQLErrs(l.check(m), r.check(m))
				},
				err: schema.AppendGQLErrs(l.err, r.err),
			})
		}
	}
	return result
}
