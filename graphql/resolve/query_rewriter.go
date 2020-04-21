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
	"fmt"
	"sort"
	"strconv"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/pkg/errors"
)

type queryRewriter struct{}

type authRewriter struct {
	authVariables map[string]interface{}
	isWritingAuth bool
	varGen        *VariableGenerator
	varName       string
}

// NewQueryRewriter returns a new QueryRewriter.
func NewQueryRewriter() QueryRewriter {
	return &queryRewriter{}
}

// Rewrite rewrites a GraphQL query into a Dgraph GraphQuery.
func (qr *queryRewriter) Rewrite(
	ctx context.Context,
	gqlQuery schema.Query) (*gql.GraphQuery, error) {

	authVariables, err := ExtractAuthVariables(ctx)
	if err != nil {
		return nil, err
	}

	auth := &authRewriter{
		authVariables: authVariables,
		varGen:        NewVariableGenerator(),
	}

	switch gqlQuery.QueryType() {
	case schema.GetQuery:

		// TODO: The only error that can occur in query rewriting is if an ID argument
		// can't be parsed as a uid: e.g. the query was something like:
		//
		// getT(id: "HI") { ... }
		//
		// But that's not a rewriting error!  It should be caught by validation
		// way up when the query first comes in.  All other possible problems with
		// the query are caught by validation.
		// ATM, I'm not sure how to hook into the GraphQL validator to get that to happen
		xid, uid, err := gqlQuery.IDArgValue()
		if err != nil {
			return nil, err
		}

		dgQuery := rewriteAsGet(gqlQuery, uid, xid)
		addTypeFilter(dgQuery, gqlQuery.Type())

		return dgQuery, nil

	case schema.FilterQuery:
		return rewriteAsQuery(gqlQuery, auth), nil
	case schema.PasswordQuery:
		return passwordQuery(gqlQuery)
	default:
		return nil, errors.Errorf("unimplemented query type %s", gqlQuery.QueryType())
	}
}

func passwordQuery(m schema.Query) (*gql.GraphQuery, error) {
	xid, uid, err := m.IDArgValue()
	if err != nil {
		return nil, err
	}

	dgQuery := rewriteAsGet(m, uid, xid)

	queriedType := m.Type()
	name := queriedType.PasswordField().Name()
	predicate := queriedType.DgraphPredicate(name)
	password := m.ArgValue(name).(string)

	op := &gql.GraphQuery{
		Attr:   "checkPwd",
		Func:   dgQuery.Func,
		Filter: dgQuery.Filter,
		Children: []*gql.GraphQuery{{
			Var: "pwd",
			Attr: fmt.Sprintf(`checkpwd(%s, "%s")`, predicate,
				password),
		}},
	}

	ft := &gql.FilterTree{
		Op: "and",
		Child: []*gql.FilterTree{{
			Func: &gql.Function{
				Name: "eq",
				Args: []gql.Arg{
					{
						Value: "val(pwd)",
					},
					{
						Value: "1",
					},
				},
			},
		}},
	}

	if dgQuery.Filter != nil {
		ft.Child = append(ft.Child, dgQuery.Filter)
	}

	dgQuery.Filter = ft

	qry := &gql.GraphQuery{
		Children: []*gql.GraphQuery{dgQuery, op},
	}

	return qry, nil
}

func intersection(a, b []uint64) []uint64 {
	m := make(map[uint64]bool)
	var c []uint64

	for _, item := range a {
		m[item] = true
	}

	for _, item := range b {
		if _, ok := m[item]; ok {
			c = append(c, item)
		}
	}

	return c
}

// addUID adds UID for every node that we query. Otherwise we can't tell the
// difference in a query result between a node that's missing and a node that's
// missing a single value.  E.g. if we are asking for an Author and only the
// 'text' of all their posts e.g. getAuthor(id: 0x123) { posts { text } }
// If the author has 10 posts but three of them have a title, but no text,
// then Dgraph would just return 7 posts.  And we'd have no way of knowing if
// there's only 7 posts, or if there's more that are missing 'text'.
// But, for GraphQL, we want to know about those missing values.
func addUID(dgQuery *gql.GraphQuery) {
	if len(dgQuery.Children) == 0 {
		return
	}
	for _, c := range dgQuery.Children {
		addUID(c)
	}

	uidChild := &gql.GraphQuery{
		Attr:  "uid",
		Alias: "dgraph.uid",
	}
	dgQuery.Children = append(dgQuery.Children, uidChild)
}

func rewriteAsQueryByIds(field schema.Field, uids []uint64) *gql.GraphQuery {
	dgQuery := &gql.GraphQuery{
		Attr: field.ResponseName(),
		Func: &gql.Function{
			Name: "uid",
			UID:  uids,
		},
	}

	if ids := idFilter(field, field.Type().IDField()); ids != nil {
		addUIDFunc(dgQuery, intersection(ids, uids))
	}

	addArgumentsToField(dgQuery, field)
	addSelectionSetFrom(dgQuery, field, nil) // FIXME: should also handle auth
	addUID(dgQuery)
	return dgQuery
}

// addArgumentsToField adds various different arguments to a field, such as
// filter, order, pagination and selection set.
func addArgumentsToField(dgQuery *gql.GraphQuery, field schema.Field) {
	filter, _ := field.ArgValue("filter").(map[string]interface{})
	addFilter(dgQuery, field.Type(), filter)
	addOrder(dgQuery, field)
	addPagination(dgQuery, field)
}

func rewriteAsGet(field schema.Field, uid uint64, xid *string) *gql.GraphQuery {
	if xid == nil {
		return rewriteAsQueryByIds(field, []uint64{uid})
	}

	xidArgName := field.XIDArg()
	eqXidFunc := &gql.Function{
		Name: "eq",
		Args: []gql.Arg{
			{Value: xidArgName},
			{Value: maybeQuoteArg("eq", *xid)},
		},
	}

	var dgQuery *gql.GraphQuery
	if uid > 0 {
		dgQuery = &gql.GraphQuery{
			Attr: field.ResponseName(),
			Func: &gql.Function{
				Name: "uid",
				UID:  []uint64{uid},
			},
		}
		dgQuery.Filter = &gql.FilterTree{
			Func: eqXidFunc,
		}

	} else {
		dgQuery = &gql.GraphQuery{
			Attr: field.ResponseName(),
			Func: eqXidFunc,
		}
	}
	addSelectionSetFrom(dgQuery, field, nil) // FIXME: should do auth
	addUID(dgQuery)
	return dgQuery
}

func rewriteAsQuery(field schema.Field, authRw *authRewriter) *gql.GraphQuery {
	dgQuery := &gql.GraphQuery{
		Attr: field.ResponseName(),
	}

	if authRw != nil && authRw.isWritingAuth && authRw.varName != "" {
		// When rewriting auth rules, they always start like
		//   Todo2 as var(func: uid(Todo1)) @cascade {
		// Where Todo1 is the variable generated from the filter of the field
		// we are adding auth to.
		//
		// TODO: Currently this only applies at the top level.  This means auth queries
		// from the top level query/get are as efficient as the original query (because
		// they start from the uid(Todo1) of the user query) ... however auth queries
		// on deeper fields will start like `func: type(Todo)`, that's ok for building
		// the feature and getting all the testing in place, but we should improve this so
		// that the internal auth queries start from exactly the possible nodes that the
		// internal field is considering.
		authRw.addVariableUIDFunc(dgQuery)
	} else if ids := idFilter(field, field.Type().IDField()); ids != nil {
		addUIDFunc(dgQuery, ids)
	} else {
		addTypeFunc(dgQuery, field.Type().DgraphName())
	}

	addArgumentsToField(dgQuery, field)
	selectionAuth := addSelectionSetFrom(dgQuery, field, authRw)
	addUID(dgQuery)

	dgQuery = authRw.addAuthQueries(field, dgQuery)

	if len(selectionAuth) > 0 {
		dgQuery = &gql.GraphQuery{Children: append([]*gql.GraphQuery{dgQuery}, selectionAuth...)}
	}

	return dgQuery
}

// addAuthQueries takes a field and the GraphQuery that has so far been constructed for
// the field and builds any auth queries that are need to restrict the result to only
// the nodes authorized to be queried, returning a new graphQuery that does the
// original query and the auth.
func (authRw *authRewriter) addAuthQueries(
	field schema.Field,
	dgQuery *gql.GraphQuery) *gql.GraphQuery {

	// There's no need to recursively inject auth queries into other auth queries, so if
	// we are already generating an auth query, there's nothing to add.
	if authRw == nil || authRw.isWritingAuth {
		return dgQuery
	}

	authRw.varName = authRw.varGen.Next(field.Type(), "", "")

	fldAuthQueries, filter := authRw.rewriteAuthQueries(field)
	if len(fldAuthQueries) == 0 {
		return dgQuery
	}

	// build a query like
	//   Todo1 as var(func: ... ) @filter(...)
	// that has the filter from the user query in it.  This is then used as
	// the starting point for both the user query and the auth query.
	varQry := rewriteAsQuery(field, &authRewriter{
		authVariables: authRw.authVariables,
		varGen:        authRw.varGen,
		isWritingAuth: true,
		varName:       "",
	})
	varQry.Var = authRw.varName
	varQry.Alias = ""
	varQry.Attr = "var"
	varQry.Children = nil

	// The user query starts from the var query generated above and is filtered
	// by the the filter generated from auth processing, so now we build
	//   queryTodo(func: uid(Todo1)) @filter(...auth-queries...) { ... }
	dgQuery.Func = &gql.Function{
		Name: "uid",
		Args: []gql.Arg{{Value: authRw.varName}},
	}
	dgQuery.Filter = filter

	// The final query that includes the user's filter and auth processsing is thus like
	//
	// queryTodo(func: uid(Todo1)) @filter(uid(Todo2) AND uid(Todo3)) { ... }
	// Todo1 as var(func: ... ) @filter(...)
	// Todo2 as var(func: uid(Todo1)) @cascade { ...auth query 1... }
	// Todo3 as var(func: uid(Todo1)) @cascade { ...auth query 2... }
	return &gql.GraphQuery{Children: append([]*gql.GraphQuery{dgQuery, varQry}, fldAuthQueries...)}
}

func (authRw *authRewriter) addVariableUIDFunc(q *gql.GraphQuery) {
	q.Func = &gql.Function{
		Name: "uid",
		Args: []gql.Arg{{Value: authRw.varName}},
	}
}

func (authRw *authRewriter) rewriteAuthQueries(f schema.Field) ([]*gql.GraphQuery, *gql.FilterTree) {
	if authRw == nil || authRw.isWritingAuth {
		return nil, nil
	}

	auth := f.Operation().Schema().AuthRules(f.Type())
	if auth == nil || auth.Rules == nil || auth.Rules.Query == nil {
		return nil, nil
	}

	return (&authRewriter{
		authVariables: authRw.authVariables,
		varGen:        authRw.varGen,
		isWritingAuth: true,
		varName:       authRw.varName,
	}).rewriteRuleNode(f, auth.Rules.Query)
}

func (authRw *authRewriter) rewriteRuleNode(
	field schema.Field,
	rn *schema.RuleNode) ([]*gql.GraphQuery, *gql.FilterTree) {

	nodeList := func(
		field schema.Field,
		rns []*schema.RuleNode) ([]*gql.GraphQuery, []*gql.FilterTree) {

		var qrys []*gql.GraphQuery
		var filts []*gql.FilterTree
		for _, orRn := range rns {
			q, f := authRw.rewriteRuleNode(field, orRn)
			qrys = append(qrys, q...)
			filts = append(filts, f)
		}
		return qrys, filts
	}

	switch {
	case len(rn.And) > 0:
		qrys, filts := nodeList(field, rn.And)
		return qrys, &gql.FilterTree{
			Op:    "and",
			Child: filts,
		}
	case len(rn.Or) > 0:
		qrys, filts := nodeList(field, rn.Or)
		return qrys, &gql.FilterTree{
			Op:    "or",
			Child: filts,
		}
	case rn.Not != nil:
		qrys, filter := authRw.rewriteRuleNode(field, rn.Not)
		return qrys, &gql.FilterTree{
			Op:    "not",
			Child: []*gql.FilterTree{filter},
		}
	case rn.Rule != nil:
		// create a copy of the auth query that's specialized for the values from the JWT
		qry := rn.Rule.AuthFor(field, authRw.authVariables)

		// build
		// Todo2 as var(func: uid(Todo1)) @cascade { ...auth query 1... }
		varName := authRw.varGen.Next(field.Type(), "", "")
		r1 := rewriteAsQuery(qry, authRw)
		r1.Var = varName
		r1.Attr = "var"
		r1.Cascade = true

		return []*gql.GraphQuery{r1}, &gql.FilterTree{
			Func: &gql.Function{
				Name: "uid",
				Args: []gql.Arg{{Value: varName}},
			},
		}
	}
	return nil, nil
}

func addTypeFilter(q *gql.GraphQuery, typ schema.Type) {
	thisFilter := &gql.FilterTree{
		Func: &gql.Function{
			Name: "type",
			Args: []gql.Arg{{Value: typ.DgraphName()}},
		},
	}

	if q.Filter == nil {
		q.Filter = thisFilter
	} else {
		q.Filter = &gql.FilterTree{
			Op:    "and",
			Child: []*gql.FilterTree{q.Filter, thisFilter},
		}
	}
}

func addUIDFunc(q *gql.GraphQuery, uids []uint64) {
	q.Func = &gql.Function{
		Name: "uid",
		UID:  uids,
	}
}

func addTypeFunc(q *gql.GraphQuery, typ string) {
	q.Func = &gql.Function{
		Name: "type",
		Args: []gql.Arg{{Value: typ}},
	}

}

// addSelectionSetFrom adds all the selections from field into q, and returns a list
// of extra queries needed to satisfy auth requirements
func addSelectionSetFrom(
	q *gql.GraphQuery,
	field schema.Field,
	auth *authRewriter) []*gql.GraphQuery {

	var authQueries []*gql.GraphQuery

	// Only add dgraph.type as a child if this field is an interface type and has some children.
	// dgraph.type would later be used in completeObject as different objects in the resulting
	// JSON would return different fields based on their concrete type.
	if field.InterfaceType() && len(field.SelectionSet()) > 0 {
		q.Children = append(q.Children, &gql.GraphQuery{
			Attr: "dgraph.type",
		})
	}
	for _, f := range field.SelectionSet() {
		// We skip typename because we can generate the information from schema or
		// dgraph.type depending upon if the type is interface or not. For interface type
		// we always query dgraph.type and can pick up the value from there.
		if f.Skip() || !f.Include() || f.Name() == schema.Typename {
			continue
		}

		child := &gql.GraphQuery{}

		if f.Alias() != "" {
			child.Alias = f.Alias()
		} else {
			child.Alias = f.Name()
		}

		if f.Type().Name() == schema.IDType {
			child.Attr = "uid"
		} else {
			child.Attr = f.DgraphPredicate()
		}

		filter, _ := f.ArgValue("filter").(map[string]interface{})
		addFilter(child, f.Type(), filter)
		addOrder(child, f)
		addPagination(child, f)

		selectionAuth := addSelectionSetFrom(child, f, auth)
		q.Children = append(q.Children, child)

		fieldAuth, authFilter := auth.rewriteAuthQueries(f)
		authQueries = append(authQueries, selectionAuth...)
		authQueries = append(authQueries, fieldAuth...)
		if len(fieldAuth) > 0 {
			if child.Filter == nil {
				child.Filter = authFilter
			} else {
				child.Filter = &gql.FilterTree{
					Op:    "and",
					Child: []*gql.FilterTree{child.Filter, authFilter},
				}
			}
		}
	}
	return authQueries
}

func addOrder(q *gql.GraphQuery, field schema.Field) {
	orderArg := field.ArgValue("order")
	order, ok := orderArg.(map[string]interface{})
	for ok {
		ascArg := order["asc"]
		descArg := order["desc"]
		thenArg := order["then"]

		if asc, ok := ascArg.(string); ok {
			q.Order = append(q.Order,
				&pb.Order{Attr: field.Type().DgraphPredicate(asc)})
		} else if desc, ok := descArg.(string); ok {
			q.Order = append(q.Order,
				&pb.Order{Attr: field.Type().DgraphPredicate(desc), Desc: true})
		}

		order, ok = thenArg.(map[string]interface{})
	}
}

func addPagination(q *gql.GraphQuery, field schema.Field) {
	q.Args = make(map[string]string)

	first := field.ArgValue("first")
	if first != nil {
		q.Args["first"] = fmt.Sprintf("%v", first)
	}

	offset := field.ArgValue("offset")
	if offset != nil {
		q.Args["offset"] = fmt.Sprintf("%v", offset)
	}
}

func convertIDs(idsSlice []interface{}) []uint64 {
	ids := make([]uint64, 0, len(idsSlice))
	for _, id := range idsSlice {
		uid, err := strconv.ParseUint(id.(string), 0, 64)
		if err != nil {
			// Skip sending the is part of the query to Dgraph.
			continue
		}
		ids = append(ids, uid)
	}
	return ids
}

func idFilter(field schema.Field, idField schema.FieldDefinition) []uint64 {
	filter, ok := field.ArgValue("filter").(map[string]interface{})
	if !ok || idField == nil {
		return nil
	}

	idsFilter := filter[idField.Name()]
	if idsFilter == nil {
		return nil
	}
	idsSlice := idsFilter.([]interface{})
	return convertIDs(idsSlice)
}

func addFilter(q *gql.GraphQuery, typ schema.Type, filter map[string]interface{}) {
	if len(filter) == 0 {
		return
	}

	// There are two cases here.
	// 1. It could be the case of a filter at root.  In this case we would have added a uid
	// function at root. Lets delete the ids key so that it isn't added in the filter.
	// Also, we need to add a dgraph.type filter.
	// 2. This could be a deep filter. In that case we don't need to do anything special.
	idField := typ.IDField()
	idName := ""
	if idField != nil {
		idName = idField.Name()
	}

	_, hasIDsFilter := filter[idName]
	filterAtRoot := hasIDsFilter && q.Func != nil && q.Func.Name == "uid"
	if filterAtRoot {
		// If id was present as a filter,
		delete(filter, idName)
	}
	q.Filter = buildFilter(typ, filter)
	if filterAtRoot {
		addTypeFilter(q, typ)
	}
}

// buildFilter builds a Dgraph gql.FilterTree from a GraphQL 'filter' arg.
//
// All the 'filter' args built by the GraphQL layer look like
// filter: { title: { anyofterms: "GraphQL" }, ... }
// or
// filter: { title: { anyofterms: "GraphQL" }, isPublished: true, ... }
// or
// filter: { title: { anyofterms: "GraphQL" }, and: { not: { ... } } }
// etc
//
// typ is the GraphQL type we are filtering on, and is needed to turn for example
// title (the GraphQL field) into Post.title (to Dgraph predicate).
//
// buildFilter turns any one filter object into a conjunction
// eg:
// filter: { title: { anyofterms: "GraphQL" }, isPublished: true }
// into:
// @filter(anyofterms(Post.title, "GraphQL") AND eq(Post.isPublished, true))
//
// Filters with `or:` and `not:` get translated to Dgraph OR and NOT.
//
// TODO: There's cases that don't make much sense like
// filter: { or: { title: { anyofterms: "GraphQL" } } }
// ATM those will probably generate junk that might cause a Dgraph error.  And
// bubble back to the user as a GraphQL error when the query fails. Really,
// they should fail query validation and never get here.
func buildFilter(typ schema.Type, filter map[string]interface{}) *gql.FilterTree {

	var ands []*gql.FilterTree
	var or *gql.FilterTree

	// Get a stable ordering so we generate the same thing each time.
	var keys []string
	for key := range filter {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Each key in filter is either "and", "or", "not" or the field name it
	// applies to such as "title" in: `title: { anyofterms: "GraphQL" }``
	for _, field := range keys {
		switch field {

		// In 'and', 'or' and 'not' cases, filter[field] must be a map[string]interface{}
		// or it would have failed GraphQL validation - e.g. 'filter: { and: 10 }'
		// would have failed validation.

		case "and":
			// title: { anyofterms: "GraphQL" }, and: { ... }
			//                       we are here ^^
			// ->
			// @filter(anyofterms(Post.title, "GraphQL") AND ... )
			ft := buildFilter(typ, filter[field].(map[string]interface{}))
			ands = append(ands, ft)
		case "or":
			// title: { anyofterms: "GraphQL" }, or: { ... }
			//                       we are here ^^
			// ->
			// @filter(anyofterms(Post.title, "GraphQL") OR ... )
			or = buildFilter(typ, filter[field].(map[string]interface{}))
		case "not":
			// title: { anyofterms: "GraphQL" }, not: { isPublished: true}
			//                       we are here ^^
			// ->
			// @filter(anyofterms(Post.title, "GraphQL") AND NOT eq(Post.isPublished, true))
			not := buildFilter(typ, filter[field].(map[string]interface{}))
			ands = append(ands,
				&gql.FilterTree{
					Op:    "not",
					Child: []*gql.FilterTree{not},
				})
		default:
			// It's a base case like:
			// title: { anyofterms: "GraphQL" } ->  anyofterms(Post.title: "GraphQL")

			switch dgFunc := filter[field].(type) {
			case map[string]interface{}:
				// title: { anyofterms: "GraphQL" } ->  anyofterms(Post.title, "GraphQL")
				// OR
				// numLikes: { le: 10 } -> le(Post.numLikes, 10)
				fn, val := first(dgFunc)
				ands = append(ands, &gql.FilterTree{
					Func: &gql.Function{
						Name: fn,
						Args: []gql.Arg{
							{Value: typ.DgraphPredicate(field)},
							{Value: maybeQuoteArg(fn, val)},
						},
					},
				})
			case []interface{}:
				// ids: [ 0x123, 0x124 ] -> uid(0x123, 0x124)
				ids := convertIDs(dgFunc)
				ands = append(ands, &gql.FilterTree{
					Func: &gql.Function{
						Name: "uid",
						UID:  ids,
					},
				})
			case interface{}:
				// isPublished: true -> eq(Post.isPublished, true)
				// OR an enum case
				// postType: Question -> eq(Post.postType, "Question")
				fn := "eq"
				ands = append(ands, &gql.FilterTree{
					Func: &gql.Function{
						Name: fn,
						Args: []gql.Arg{
							{Value: typ.DgraphPredicate(field)},
							{Value: fmt.Sprintf("%v", dgFunc)},
						},
					},
				})
			}
		}
	}

	var andFt *gql.FilterTree
	if len(ands) == 1 {
		andFt = ands[0]
	} else if len(ands) > 1 {
		andFt = &gql.FilterTree{
			Op:    "and",
			Child: ands,
		}
	}

	if or == nil {
		return andFt
	}

	return &gql.FilterTree{
		Op:    "or",
		Child: []*gql.FilterTree{andFt, or},
	}
}

func maybeQuoteArg(fn string, arg interface{}) string {
	switch arg := arg.(type) {
	case string: // dateTime also parsed as string
		if fn == "regexp" {
			return arg
		}
		return fmt.Sprintf("%q", arg)
	default:
		return fmt.Sprintf("%v", arg)
	}
}

// fst returns the first element it finds in a map - we bump into lots of one-element
// maps like { "anyofterms": "GraphQL" }.  fst helps extract that single mapping.
func first(aMap map[string]interface{}) (string, interface{}) {
	for key, val := range aMap {
		return key, val
	}
	return "", nil
}
