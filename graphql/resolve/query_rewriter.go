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
	"bytes"
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"github.com/dgraph-io/dgraph/dql"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
)

type queryRewriter struct{}

type authRewriter struct {
	authVariables map[string]interface{}
	isWritingAuth bool
	// `filterByUid` is used to when we have to rewrite top level query with uid function. The
	// variable name is passed in `varName`. If true it will rewrite as following:
	// queryType(uid(varName)) {
	// Once such case is when we perform query in delete mutation.
	filterByUid bool
	selector    func(t schema.Type) *schema.RuleNode
	varGen      *VariableGenerator
	varName     string
	// `parentVarName` is used to link a query with it's previous level.
	parentVarName string
	// `hasAuthRules` indicates if any of fields in the complete query hierarchy has auth rules.
	hasAuthRules bool
	// `hasCascade` indicates if any of fields in the complete query hierarchy has cascade directive.
	hasCascade bool
}

// The struct is used as a return type for buildCommonAuthQueries function.
type commonAuthQueryVars struct {
	// Stores queries of the form
	// var(func: uid(Ticket)) {
	//		User as Ticket.assignedTo
	// }
	parentQry *dql.GraphQuery
	// Stores queries which aggregate filters and auth rules. Eg.
	// // User6 as var(func: uid(User2), orderasc: ...) @filter((eq(User.username, "User1") AND (...Auth Filter))))
	selectionQry *dql.GraphQuery
}

// NewQueryRewriter returns a new QueryRewriter.
func NewQueryRewriter() QueryRewriter {
	return &queryRewriter{}
}

func hasAuthRules(field schema.Field, authRw *authRewriter) bool {
	if field == nil {
		return false
	}

	rn := authRw.selector(field.ConstructedFor())
	if rn != nil {
		return true
	}

	for _, childField := range field.SelectionSet() {
		if authRules := hasAuthRules(childField, authRw); authRules {
			return true
		}
	}
	return false
}

func hasCascadeDirective(field schema.Field) bool {
	if c := field.Cascade(); c != nil {
		return true
	}

	for _, childField := range field.SelectionSet() {
		if res := hasCascadeDirective(childField); res {
			return true
		}
	}
	return false
}

// Returns the auth selector to be used depending on the query type.
func getAuthSelector(queryType schema.QueryType) func(t schema.Type) *schema.RuleNode {
	if queryType == schema.PasswordQuery {
		return passwordAuthSelector
	}
	return queryAuthSelector
}

// Rewrite rewrites a GraphQL query into a Dgraph GraphQuery.
func (qr *queryRewriter) Rewrite(
	ctx context.Context,
	gqlQuery schema.Query) ([]*dql.GraphQuery, error) {

	customClaims, err := gqlQuery.GetAuthMeta().ExtractCustomClaims(ctx)
	if err != nil {
		return nil, err
	}

	authRw := &authRewriter{
		authVariables: customClaims.AuthVariables,
		varGen:        NewVariableGenerator(),
		selector:      getAuthSelector(gqlQuery.QueryType()),
		parentVarName: gqlQuery.ConstructedFor().Name() + "Root",
	}
	authRw.hasAuthRules = hasAuthRules(gqlQuery, authRw)
	authRw.hasCascade = hasCascadeDirective(gqlQuery)

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

		dgQuery := rewriteAsGet(gqlQuery, uid, xid, authRw)
		return dgQuery, nil

	case schema.FilterQuery:
		return rewriteAsQuery(gqlQuery, authRw), nil
	case schema.PasswordQuery:
		return passwordQuery(gqlQuery, authRw)
	case schema.AggregateQuery:
		return aggregateQuery(gqlQuery, authRw), nil
	case schema.EntitiesQuery:
		return entitiesQuery(gqlQuery, authRw)
	default:
		return nil, errors.Errorf("unimplemented query type %s", gqlQuery.QueryType())
	}
}

// entitiesQuery rewrites the Apollo `_entities` Query which is sent from the Apollo gateway to a DQL query.
// This query is sent to the Dgraph service to resolve types `extended` and defined by this service.
func entitiesQuery(field schema.Query, authRw *authRewriter) ([]*dql.GraphQuery, error) {

	// Input Argument to the Query is a List of "__typename" and "keyField" pair.
	// For this type Extension:-
	// 	extend type Product @key(fields: "upc") {
	// 		upc: String @external
	// 		reviews: [Review]
	// 	}
	// Input to the Query will be
	// "_representations": [
	// 		{
	// 		  "__typename": "Product",
	// 	 	 "upc": "B00005N5PF"
	// 		},
	// 		...
	//   ]

	parsedRepr, err := field.RepresentationsArg()
	if err != nil {
		return nil, err
	}

	typeDefn := parsedRepr.TypeDefn
	rbac := authRw.evaluateStaticRules(typeDefn)

	dgQuery := &dql.GraphQuery{
		Attr: field.Name(),
	}

	if rbac == schema.Negative {
		dgQuery.Attr = dgQuery.Attr + "()"
		return []*dql.GraphQuery{dgQuery}, nil
	}

	// Construct Filter at Root Func.
	// if keyFieldsIsID = true and keyFieldValueList = {"0x1", "0x2"}
	// then query will be formed as:-
	// 	_entities(func: uid("0x1", "0x2") {
	//		...
	//	}
	// if keyFieldsIsID = false then query will be like:-
	// 	_entities(func: eq(keyFieldName,"0x1", "0x2") {
	//		...
	//	}

	// If the key field is of ID type and is not an external field
	// then we query it using the `uid` otherwise we treat it as string
	// and query using `eq` function.
	// We also don't need to add Order to the query as the results are
	// automatically returned in the ascending order of the uids.
	if parsedRepr.KeyField.IsID() && !parsedRepr.KeyField.IsExternal() {
		addUIDFunc(dgQuery, convertIDs(parsedRepr.KeyVals))
	} else {
		addEqFunc(dgQuery, typeDefn.DgraphPredicate(parsedRepr.KeyField.Name()), parsedRepr.KeyVals)
		// Add the  ascending Order of the keyField in the query.
		// The result will be converted into the exact in the resultCompletion step.
		dgQuery.Order = append(dgQuery.Order,
			&pb.Order{Attr: typeDefn.DgraphPredicate(parsedRepr.KeyField.Name())})
	}
	// AddTypeFilter in as the Filter to the Root the Query.
	// Query will be like :-
	// 	_entities(func: ...) @filter(type(typeName)) {
	//		...
	// 	}
	addTypeFilter(dgQuery, typeDefn)

	selectionAuth := addSelectionSetFrom(dgQuery, field, authRw)
	addUID(dgQuery)

	dgQueries := authRw.addAuthQueries(typeDefn, []*dql.GraphQuery{dgQuery}, rbac)
	return append(dgQueries, selectionAuth...), nil

}

func aggregateQuery(query schema.Query, authRw *authRewriter) []*dql.GraphQuery {

	// Get the type which the count query is written for
	mainType := query.ConstructedFor()

	dgQuery, rbac := addCommonRules(query, mainType, authRw)
	if rbac == schema.Negative {
		return dgQuery
	}

	// Add filter
	filter, _ := query.ArgValue("filter").(map[string]interface{})
	_ = addFilter(dgQuery[0], mainType, filter)

	dgQuery = authRw.addAuthQueries(mainType, dgQuery, rbac)

	// mainQuery is the query with Attr: query.Name()
	// It is the first query in dgQuery list.
	mainQuery := dgQuery[0]

	// Changing mainQuery Attr name to var. This is used in the final aggregate<Type> query.
	mainQuery.Attr = "var"

	finalMainQuery := &dql.GraphQuery{
		Attr: query.DgraphAlias() + "()",
	}
	// Add selection set to mainQuery and finalMainQuery.
	isAggregateVarAdded := make(map[string]bool)
	isCountVarAdded := false

	for _, f := range query.SelectionSet() {
		// fldName stores Name of the field f.
		fldName := f.Name()
		if fldName == "count" {
			if !isCountVarAdded {
				child := &dql.GraphQuery{
					Var:  "countVar",
					Attr: "count(uid)",
				}
				mainQuery.Children = append(mainQuery.Children, child)
				isCountVarAdded = true
			}
			finalQueryChild := &dql.GraphQuery{
				Alias: f.DgraphAlias(),
				Attr:  "max(val(countVar))",
			}
			finalMainQuery.Children = append(finalMainQuery.Children, finalQueryChild)
			continue
		}

		// Handle other aggregate functions than count
		aggregateFunctions := []string{"Max", "Min", "Sum", "Avg"}

		for _, function := range aggregateFunctions {
			// A field can have at maximum one of the aggregation functions as suffix
			if strings.HasSuffix(fldName, function) {
				// constructedForDgraphPredicate stores the Dgraph predicate for which aggregate function has been queried.
				constructedForDgraphPredicate := f.DgraphPredicateForAggregateField()
				// constructedForField contains the field for which aggregate function has been queried.
				// As all aggregate functions have length 3, removing last 3 characters from fldName.
				constructedForField := fldName[:len(fldName)-3]
				// isAggregateVarAdded ensures that a field is added to Var query at maximum once.
				// If a field has already been added to the var query, don't add it again.
				// Eg. Even if scoreMax and scoreMin are queried, the query will contain only one expression
				// of the from, "scoreVar as Tweets.score"
				if !isAggregateVarAdded[constructedForField] {
					child := &dql.GraphQuery{
						Var:  constructedForField + "Var",
						Attr: constructedForDgraphPredicate,
					}
					// The var field is added to mainQuery. This adds the following DQL query.
					// var(func: type(Tweets)) {
					//        scoreVar as Tweets.score
					// }

					mainQuery.Children = append(mainQuery.Children, child)
					isAggregateVarAdded[constructedForField] = true
				}
				finalQueryChild := &dql.GraphQuery{
					Alias: f.DgraphAlias(),
					Attr:  strings.ToLower(function) + "(val(" + constructedForField + "Var))",
				}
				// This adds the following DQL query
				// aggregateTweets() {
				//        TweetsAggregateResult.scoreMin : min(val(scoreVar))
				// }
				finalMainQuery.Children = append(finalMainQuery.Children, finalQueryChild)
				break
			}
		}
	}

	return append([]*dql.GraphQuery{finalMainQuery}, dgQuery...)
}

func passwordQuery(m schema.Query, authRw *authRewriter) ([]*dql.GraphQuery, error) {
	xid, uid, err := m.IDArgValue()
	if err != nil {
		return nil, err
	}

	dgQuery := rewriteAsGet(m, uid, xid, authRw)

	// Handle empty dgQuery
	if strings.HasSuffix(dgQuery[0].Attr, "()") {
		return dgQuery, nil
	}

	// mainQuery is the query with check<Type>Password as Attr.
	// It is the first in the list of dgQuery.
	mainQuery := dgQuery[0]

	queriedType := m.Type()
	name := queriedType.PasswordField().Name()
	predicate := queriedType.DgraphPredicate(name)
	password := m.ArgValue(name).(string)

	// This adds the checkPwd function
	op := &dql.GraphQuery{
		Attr:   "checkPwd",
		Func:   mainQuery.Func,
		Filter: mainQuery.Filter,
		Children: []*dql.GraphQuery{{
			Var: "pwd",
			Attr: fmt.Sprintf(`checkpwd(%s, "%s")`, predicate,
				password),
		}},
	}

	ft := &dql.FilterTree{
		Op: "and",
		Child: []*dql.FilterTree{{
			Func: &dql.Function{
				Name: "eq",
				Args: []dql.Arg{
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

	if mainQuery.Filter != nil {
		ft.Child = append(ft.Child, mainQuery.Filter)
	}

	mainQuery.Filter = ft

	return append(dgQuery, op), nil
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
func addUID(dgQuery *dql.GraphQuery) {
	if len(dgQuery.Children) == 0 {
		return
	}
	hasUid := false
	for _, c := range dgQuery.Children {
		if c.Attr == "uid" {
			hasUid = true
		}
		addUID(c)
	}

	// If uid was already requested by the user then we don't need to add it again.
	if hasUid {
		return
	}
	uidChild := &dql.GraphQuery{
		Attr:  "uid",
		Alias: "dgraph.uid",
	}
	dgQuery.Children = append(dgQuery.Children, uidChild)
}

func rewriteAsQueryByIds(
	field schema.Field,
	uids []uint64,
	authRw *authRewriter) []*dql.GraphQuery {
	if field == nil {
		return nil
	}

	rbac := authRw.evaluateStaticRules(field.Type())
	dgQuery := []*dql.GraphQuery{{
		Attr: field.DgraphAlias(),
	}}

	if rbac == schema.Negative {
		dgQuery[0].Attr = dgQuery[0].Attr + "()"
		return dgQuery
	}

	dgQuery[0].Func = &dql.Function{
		Name: "uid",
		UID:  uids,
	}

	if ids := idFilter(extractQueryFilter(field), field.Type().IDField()); ids != nil {
		addUIDFunc(dgQuery[0], intersection(ids, uids))
	}

	addArgumentsToField(dgQuery[0], field)

	// The function getQueryByIds is called for passwordQuery or fetching query result types
	// after making a mutation. In both cases, we want the selectionSet to use the `query` auth
	// rule. queryAuthSelector function is used as selector before calling addSelectionSetFrom function.
	// The original selector function of authRw is stored in oldAuthSelector and used after returning
	// from addSelectionSetFrom function.
	oldAuthSelector := authRw.selector
	authRw.selector = queryAuthSelector
	selectionAuth := addSelectionSetFrom(dgQuery[0], field, authRw)
	authRw.selector = oldAuthSelector

	addUID(dgQuery[0])
	addCascadeDirective(dgQuery[0], field)

	dgQuery = authRw.addAuthQueries(field.Type(), dgQuery, rbac)

	if len(selectionAuth) > 0 {
		dgQuery = append(dgQuery, selectionAuth...)
	}

	return dgQuery
}

// addArgumentsToField adds various different arguments to a field, such as
// filter, order and pagination.
func addArgumentsToField(dgQuery *dql.GraphQuery, field schema.Field) {
	filter, _ := field.ArgValue("filter").(map[string]interface{})
	_ = addFilter(dgQuery, field.Type(), filter)
	addOrder(dgQuery, field)
	addPagination(dgQuery, field)
}

func addTopLevelTypeFilter(query *dql.GraphQuery, field schema.Field) {
	addTypeFilter(query, field.Type())
}

func rewriteAsGet(
	query schema.Query,
	uid uint64,
	xidArgToVal map[string]string,
	auth *authRewriter) []*dql.GraphQuery {

	var dgQuery []*dql.GraphQuery
	rbac := auth.evaluateStaticRules(query.Type())

	// If Get query is for Type and none of the authrules are satisfied, then it is
	// caught here but in case of interface, we need to check validity on each
	// implementing type as Rules for the interface are made empty.
	if rbac == schema.Negative {
		return []*dql.GraphQuery{{Attr: query.DgraphAlias() + "()"}}
	}

	// For interface, empty query should be returned if Auth rules are
	// not satisfied even for a single implementing type
	if query.Type().IsInterface() {
		implementingTypesHasFailedRules := false
		implementingTypes := query.Type().ImplementingTypes()
		for _, typ := range implementingTypes {
			if auth.evaluateStaticRules(typ) != schema.Negative {
				implementingTypesHasFailedRules = true
			}
		}

		if !implementingTypesHasFailedRules {
			return []*dql.GraphQuery{{Attr: query.Name() + "()"}}
		}
	}

	if len(xidArgToVal) == 0 {
		dgQuery = rewriteAsQueryByIds(query, []uint64{uid}, auth)

		// Add the type filter to the top level get query. When the auth has been written into the
		// query the top level get query may be present in query's children.
		addTopLevelTypeFilter(dgQuery[0], query)

		return dgQuery
	}
	// iterate over map in sorted order to ensure consistency
	xids := make([]string, len(xidArgToVal))
	i := 0
	for k := range xidArgToVal {
		xids[i] = k
		i++
	}
	sort.Strings(xids)
	xidArgNameToDgPredMap := query.XIDArgs()
	var flt []*dql.FilterTree
	for _, xid := range xids {
		eqXidFuncTemp := &dql.Function{
			Name: "eq",
			Args: []dql.Arg{
				{Value: xidArgNameToDgPredMap[xid]},
				{Value: maybeQuoteArg("eq", xidArgToVal[xid])},
			},
		}
		flt = append(flt, &dql.FilterTree{
			Func: eqXidFuncTemp,
		})
	}
	if uid > 0 {
		dgQuery = []*dql.GraphQuery{{
			Attr: query.DgraphAlias(),
			Func: &dql.Function{
				Name: "uid",
				UID:  []uint64{uid},
			},
		}}
		dgQuery[0].Filter = &dql.FilterTree{
			Op:    "and",
			Child: flt,
		}

	} else {
		dgQuery = []*dql.GraphQuery{{
			Attr: query.DgraphAlias(),
			Func: flt[0].Func,
		}}
		if len(flt) > 1 {
			dgQuery[0].Filter = &dql.FilterTree{
				Op:    "and",
				Child: flt[1:],
			}
		}
	}

	// Apply query auth rules even for password query
	oldAuthSelector := auth.selector
	auth.selector = queryAuthSelector
	selectionAuth := addSelectionSetFrom(dgQuery[0], query, auth)
	auth.selector = oldAuthSelector

	addUID(dgQuery[0])
	addTypeFilter(dgQuery[0], query.Type())
	addCascadeDirective(dgQuery[0], query)

	dgQuery = auth.addAuthQueries(query.Type(), dgQuery, rbac)

	if len(selectionAuth) > 0 {
		dgQuery = append(dgQuery, selectionAuth...)
	}

	return dgQuery
}

// Adds common RBAC and UID, Type rules to DQL query.
// This function is used by rewriteAsQuery and aggregateQuery functions
func addCommonRules(
	field schema.Field,
	fieldType schema.Type,
	authRw *authRewriter) ([]*dql.GraphQuery, schema.RuleResult) {
	rbac := authRw.evaluateStaticRules(fieldType)
	dgQuery := &dql.GraphQuery{
		Attr: field.DgraphAlias(),
	}

	if rbac == schema.Negative {
		dgQuery.Attr = dgQuery.Attr + "()"
		return []*dql.GraphQuery{dgQuery}, rbac
	}

	// When rewriting auth rules, they always start like
	// Todo2 as var(func: uid(Todo1)) @cascade {
	// Where Todo1 is the variable generated from the filter of the field
	// we are adding auth to.
	// Except for the case in which filter in auth rules is on field of
	// ID type. In this situation we write it as:
	// Todo2 as var(func: uid(0x5....)) @cascade {
	// We first check ids in the query filter and rewrite accordingly.
	ids := idFilter(extractQueryFilter(field), fieldType.IDField())

	// Todo: Add more comments to this block.
	if authRw != nil && (authRw.isWritingAuth || authRw.filterByUid) &&
		(authRw.varName != "" || authRw.parentVarName != "") && ids == nil {
		authRw.addVariableUIDFunc(dgQuery)
		// This is executed when querying while performing delete mutation request since
		// in case of delete mutation we already have variable `MutationQueryVar` at root level.
		if authRw.filterByUid {
			// Since the variable is only added at the top level we reset the `authRW` variables.
			authRw.varName = ""
			authRw.filterByUid = false
		}
	} else if ids != nil {
		addUIDFunc(dgQuery, ids)
	} else {
		addTypeFunc(dgQuery, fieldType.DgraphName())
	}
	return []*dql.GraphQuery{dgQuery}, rbac
}

func rewriteAsQuery(field schema.Field, authRw *authRewriter) []*dql.GraphQuery {
	dgQuery, rbac := addCommonRules(field, field.Type(), authRw)
	if rbac == schema.Negative {
		return dgQuery
	}

	addArgumentsToField(dgQuery[0], field)
	selectionAuth := addSelectionSetFrom(dgQuery[0], field, authRw)
	// we don't need to query uid for auth queries, as they always have at least one field in their
	// selection set.
	if !authRw.writingAuth() {
		addUID(dgQuery[0])
	}
	addCascadeDirective(dgQuery[0], field)

	dgQuery = authRw.addAuthQueries(field.Type(), dgQuery, rbac)

	if len(selectionAuth) > 0 {
		return append(dgQuery, selectionAuth...)
	}

	dgQuery = rootQueryOptimization(dgQuery)
	return dgQuery
}

func rootQueryOptimization(dgQuery []*dql.GraphQuery) []*dql.GraphQuery {
	if dgQuery[0].Filter != nil && dgQuery[0].Filter.Func != nil &&
		dgQuery[0].Filter.Func.Name == "eq" && dgQuery[0].Func.Name == "type" {
		rootFunc := dgQuery[0].Func
		dgQuery[0].Func = dgQuery[0].Filter.Func
		dgQuery[0].Filter.Func = rootFunc
	}
	return dgQuery
}

func (authRw *authRewriter) writingAuth() bool {
	return authRw != nil && authRw.isWritingAuth

}

// addAuthQueries takes a field and the GraphQuery that has so far been constructed for
// the field and builds any auth queries that are need to restrict the result to only
// the nodes authorized to be queried, returning a new graphQuery that does the
// original query and the auth.
func (authRw *authRewriter) addAuthQueries(
	typ schema.Type,
	dgQuery []*dql.GraphQuery,
	rbacEval schema.RuleResult) []*dql.GraphQuery {

	// There's no need to recursively inject auth queries into other auth queries, so if
	// we are already generating an auth query, there's nothing to add.
	if authRw == nil || authRw.isWritingAuth {
		return dgQuery
	}

	authRw.varName = authRw.varGen.Next(typ, "", "", authRw.isWritingAuth)

	fldAuthQueries, filter := authRw.rewriteAuthQueries(typ)

	// If We are adding AuthRules on an Interfaces's operation,
	// we need to construct auth filters by verifying Auth rules on the
	// implementing types.

	if typ.IsInterface() {
		// First we fetch the list of Implementing types here
		implementingTypes := make([]schema.Type, 0)
		implementingTypes = append(implementingTypes, typ.ImplementingTypes()...)

		var qrys []*dql.GraphQuery
		var filts []*dql.FilterTree
		implementingTypesHasAuthRules := false
		for _, object := range implementingTypes {

			// It could be the case that None of implementing Types have Auth Rules, which clearly
			// indicates that neither the interface, nor any of the implementing type has its own
			// Auth rules.
			// ImplementingTypeHasAuthRules is set to true even if one of the implemented type have
			// Auth rules or Interface has its own auth rule, in the latter case, all the
			// implemented types must have inherited those auth rules.
			if object.AuthRules().Rules != nil {
				implementingTypesHasAuthRules = true
			}

			// First Check if the Auth Rules of the given type are satisfied or not.
			// It might be possible that auth rule inherited from some other interface
			// is not being satisfied. In that case we have to Drop this type
			rbac := authRw.evaluateStaticRules(object)
			if rbac == schema.Negative {
				continue
			}

			// Form Query Like Todo_1 as var(func: type(Todo))
			queryVar := authRw.varGen.Next(object, "", "", authRw.isWritingAuth)
			varQry := &dql.GraphQuery{
				Attr: "var",
				Var:  queryVar,
				Func: &dql.Function{
					Name: "type",
					Args: []dql.Arg{{Value: object.Name()}},
				},
			}
			qrys = append(qrys, varQry)

			// Form Auth Queries for the given object
			objAuthQueries, objfilter := (&authRewriter{
				authVariables: authRw.authVariables,
				varGen:        authRw.varGen,
				varName:       queryVar,
				selector:      authRw.selector,
				parentVarName: authRw.parentVarName,
				hasAuthRules:  authRw.hasAuthRules,
			}).rewriteAuthQueries(object)

			// 1. If there is no Auth Query for the Given type then it means that
			// neither the inherited interface, nor this type has any Auth rules.
			// In this case the query must return all the nodes of this type.
			// then simply we need to Put uid(Todo1) with OR in the main query filter.
			// 2. If rbac evaluates to `Positive` which means RBAC rule is satisfied.
			// Either it is the only auth rule, or it is present with `OR`, which means
			// query must return all the nodes of this type.
			if len(objAuthQueries) == 0 || rbac == schema.Positive {
				objfilter = &dql.FilterTree{
					Func: &dql.Function{
						Name: "uid",
						Args: []dql.Arg{{Value: queryVar, IsValueVar: false, IsGraphQLVar: false}},
					},
				}
				filts = append(filts, objfilter)
			} else {
				qrys = append(qrys, objAuthQueries...)
				filts = append(filts, objfilter)
			}
		}

		// For an interface having Auth rules in some of the implementing types, len(qrys) = 0
		// indicates that None of the type satisfied the Auth rules, We must return Empty Query here.
		if implementingTypesHasAuthRules && len(qrys) == 0 {
			return []*dql.GraphQuery{{
				Attr: dgQuery[0].Attr + "()",
			}}
		}

		// Join all the queries in qrys using OR filter and
		// append these queries into fldAuthQueries
		fldAuthQueries = append(fldAuthQueries, qrys...)
		objOrfilter := &dql.FilterTree{
			Op:    "or",
			Child: filts,
		}

		// if filts is non empty, which means it was a query on interface
		// having Either any of the types satisfying auth rules or having
		// some type with no Auth rules, In this case, the query will be different
		// and will look somewhat like this:
		// PostRoot as var(func: uid(Post1)) @filter((uid(QuestionAuth2) OR uid(AnswerAuth4)))
		if len(filts) > 0 {
			filter = objOrfilter
		}

		// Adding the case of Query on interface in which None of the implementing type have
		// Auth Query Rules, in that case, we also return simple query.
		if typ.IsInterface() && !implementingTypesHasAuthRules {
			return dgQuery
		}

	}

	if len(fldAuthQueries) == 0 && !authRw.hasAuthRules {
		return dgQuery
	}

	if rbacEval != schema.Uncertain {
		fldAuthQueries = nil
		filter = nil
	}

	// build a query like
	//   Todo1 as var(func: ... ) @filter(...)
	// that has the filter from the user query in it.  This is then used as
	// the starting point for other auth queries.
	//
	// We already have the query, so just copy it and modify the original
	varQry := &dql.GraphQuery{
		Var:    authRw.varName,
		Attr:   "var",
		Func:   dgQuery[0].Func,
		Filter: dgQuery[0].Filter,
	}

	// build the root auth query like
	//   TodoRoot as var(func: uid(Todo1), orderasc: ..., first: ..., offset: ...) @filter(... type auth queries ...)
	// that has the order and pagination params from user query in it and filter set to auth
	// queries built for this type. This is then used as the starting point for user query and
	// auth queries for children.
	// if @cascade directive is present in the user query then pagination and order are applied only
	// on the user query and not on root query.
	rootQry := &dql.GraphQuery{
		Var:  authRw.parentVarName,
		Attr: "var",
		Func: &dql.Function{
			Name: "uid",
			Args: []dql.Arg{{Value: authRw.varName}},
		},
		Filter: filter,
	}

	// The user query doesn't need the filter parameter anymore,
	// as it has been taken care of by the var and root queries generated above.
	// But, it still needs the order parameter, even though it is also applied in root query.
	// So, not setting order to nil.
	dgQuery[0].Filter = nil

	// if @cascade is not applied on the user query at root then shift pagination arguments
	// from user query to root query for optimization and copy the order arguments for paginated
	// query to work correctly.
	if len(dgQuery[0].Cascade) == 0 {
		rootQry.Args = dgQuery[0].Args
		dgQuery[0].Args = nil
		rootQry.Order = dgQuery[0].Order
	}

	// The user query starts from the root query generated above and so gets filtered
	// input from auth processing, so now we build
	//   queryTodo(func: uid(TodoRoot), ...) { ... }
	dgQuery[0].Func = &dql.Function{
		Name: "uid",
		Args: []dql.Arg{{Value: authRw.parentVarName}},
	}

	// The final query that includes the user's filter and auth processing is thus like
	//
	// queryTodo(func: uid(Todo1)) @filter(uid(Todo2) AND uid(Todo3)) { ... }
	// Todo1 as var(func: ... ) @filter(...)
	// Todo2 as var(func: uid(Todo1)) @cascade { ...auth query 1... }
	// Todo3 as var(func: uid(Todo1)) @cascade { ...auth query 2... }
	ret := append(dgQuery, rootQry, varQry)
	ret = append(ret, fldAuthQueries...)
	return ret
}

func (authRw *authRewriter) addVariableUIDFunc(q *dql.GraphQuery) {
	varName := authRw.parentVarName
	if authRw.varName != "" {
		varName = authRw.varName
	}

	q.Func = &dql.Function{
		Name: "uid",
		Args: []dql.Arg{{Value: varName}},
	}
}

func queryAuthSelector(t schema.Type) *schema.RuleNode {
	auth := t.AuthRules()
	if auth == nil || auth.Rules == nil {
		return nil
	}

	return auth.Rules.Query
}

// passwordAuthSelector is used as auth selector for checkPassword queries
func passwordAuthSelector(t schema.Type) *schema.RuleNode {
	auth := t.AuthRules()
	if auth == nil || auth.Rules == nil {
		return nil
	}

	return auth.Rules.Password
}

func (authRw *authRewriter) rewriteAuthQueries(typ schema.Type) ([]*dql.GraphQuery, *dql.FilterTree) {
	if authRw == nil || authRw.isWritingAuth {
		return nil, nil
	}

	return (&authRewriter{
		authVariables: authRw.authVariables,
		varGen:        authRw.varGen,
		isWritingAuth: true,
		varName:       authRw.varName,
		selector:      authRw.selector,
		parentVarName: authRw.parentVarName,
		hasAuthRules:  authRw.hasAuthRules,
	}).rewriteRuleNode(typ, authRw.selector(typ))
}

func (authRw *authRewriter) evaluateStaticRules(typ schema.Type) schema.RuleResult {
	if authRw == nil || authRw.isWritingAuth {
		return schema.Uncertain
	}

	rn := authRw.selector(typ)
	return rn.EvaluateStatic(authRw.authVariables)
}

func (authRw *authRewriter) rewriteRuleNode(
	typ schema.Type,
	rn *schema.RuleNode) ([]*dql.GraphQuery, *dql.FilterTree) {

	if typ == nil || rn == nil {
		return nil, nil
	}

	nodeList := func(
		typ schema.Type,
		rns []*schema.RuleNode) ([]*dql.GraphQuery, []*dql.FilterTree) {

		var qrys []*dql.GraphQuery
		var filts []*dql.FilterTree
		for _, orRn := range rns {
			q, f := authRw.rewriteRuleNode(typ, orRn)
			qrys = append(qrys, q...)
			if f != nil {
				filts = append(filts, f)
			}
		}
		return qrys, filts
	}

	switch {
	case len(rn.And) > 0:
		// if there is atleast one RBAC rule which is false, then this
		// whole And block needs to be ignored.
		if rn.EvaluateStatic(authRw.authVariables) == schema.Negative {
			return nil, nil
		}
		qrys, filts := nodeList(typ, rn.And)
		if len(filts) == 0 {
			return qrys, nil
		}
		if len(filts) == 1 {
			return qrys, filts[0]
		}
		return qrys, &dql.FilterTree{
			Op:    "and",
			Child: filts,
		}
	case len(rn.Or) > 0:
		qrys, filts := nodeList(typ, rn.Or)
		if len(filts) == 0 {
			return qrys, nil
		}
		if len(filts) == 1 {
			return qrys, filts[0]
		}
		return qrys, &dql.FilterTree{
			Op:    "or",
			Child: filts,
		}
	case rn.Not != nil:
		qrys, filter := authRw.rewriteRuleNode(typ, rn.Not)
		if filter == nil {
			return qrys, nil
		}
		return qrys, &dql.FilterTree{
			Op:    "not",
			Child: []*dql.FilterTree{filter},
		}
	case rn.Rule != nil:
		if rn.EvaluateStatic(authRw.authVariables) == schema.Negative {
			return nil, nil
		}

		// create a copy of the auth query that's specialized for the values from the JWT
		qry := rn.Rule.AuthFor(authRw.authVariables)

		// build
		// Todo2 as var(func: uid(Todo1)) @cascade { ...auth query 1... }
		varName := authRw.varGen.Next(typ, "", "", authRw.isWritingAuth)
		r1 := rewriteAsQuery(qry, authRw)
		r1[0].Var = varName
		r1[0].Attr = "var"
		if len(r1[0].Cascade) == 0 {
			r1[0].Cascade = append(r1[0].Cascade, "__all__")
		}

		return []*dql.GraphQuery{r1[0]}, &dql.FilterTree{
			Func: &dql.Function{
				Name: "uid",
				Args: []dql.Arg{{Value: varName}},
			},
		}
	case rn.DQLRule != nil:
		return []*dql.GraphQuery{rn.DQLRule}, &dql.FilterTree{
			Func: &dql.Function{
				Name: "uid",
				Args: []dql.Arg{{Value: rn.DQLRule.Var}},
			},
		}
	}
	return nil, nil
}

func addTypeFilter(q *dql.GraphQuery, typ schema.Type) {
	thisFilter := &dql.FilterTree{
		Func: buildTypeFunc(typ.DgraphName()),
	}
	addToFilterTree(q, thisFilter)
}

func addToFilterTree(q *dql.GraphQuery, filter *dql.FilterTree) {
	if q.Filter == nil {
		q.Filter = filter
	} else {
		q.Filter = &dql.FilterTree{
			Op:    "and",
			Child: []*dql.FilterTree{q.Filter, filter},
		}
	}
}

func addUIDFunc(q *dql.GraphQuery, uids []uint64) {
	q.Func = &dql.Function{
		Name: "uid",
		UID:  uids,
	}
}

func addEqFunc(q *dql.GraphQuery, dgPred string, values []interface{}) {
	args := []dql.Arg{{Value: dgPred}}
	for _, v := range values {
		args = append(args, dql.Arg{Value: maybeQuoteArg("eq", v)})
	}
	q.Func = &dql.Function{
		Name: "eq",
		Args: args,
	}
}

func addTypeFunc(q *dql.GraphQuery, typ string) {
	q.Func = buildTypeFunc(typ)
}

func buildTypeFunc(typ string) *dql.Function {
	return &dql.Function{
		Name: "type",
		Args: []dql.Arg{{Value: typ}},
	}
}

// Builds parentQry for auth rules and selectionQry to aggregate all filter and
// auth rules. This is used to build common auth rules by addSelectionSetFrom and
// buildAggregateFields function.
func buildCommonAuthQueries(
	f schema.Field,
	auth *authRewriter,
	parentSelectionName string) commonAuthQueryVars {
	// This adds the following query.
	//	var(func: uid(Ticket1)) {
	//		User4 as Ticket.assignedTo
	//	}
	// where `Ticket1` is the nodes selected at parent level after applying auth and `User4` is the
	// nodes we need on the current level.
	parentQry := &dql.GraphQuery{
		Func: &dql.Function{
			Name: "uid",
			Args: []dql.Arg{{Value: parentSelectionName}},
		},
		Attr:     "var",
		Children: []*dql.GraphQuery{{Attr: f.ConstructedForDgraphPredicate(), Var: auth.varName}},
	}

	// This query aggregates all filters and auth rules and is used by root query to filter
	// the final nodes for the current level.
	// User3 as var(func: uid(User4)) @filter((eq(User.username, "User1") AND (...Auth Filter))))
	selectionQry := &dql.GraphQuery{
		Var:  auth.parentVarName,
		Attr: "var",
		Func: &dql.Function{
			Name: "uid",
			Args: []dql.Arg{{Value: auth.varName}},
		},
	}

	return commonAuthQueryVars{
		parentQry:    parentQry,
		selectionQry: selectionQry,
	}
}

// buildAggregateFields builds DQL queries for aggregate fields like count, avg, max etc.
// It returns related DQL fields and Auth Queries which are then added to the final DQL query
// by the caller.
func buildAggregateFields(
	f schema.Field,
	auth *authRewriter) ([]*dql.GraphQuery, []*dql.GraphQuery) {
	constructedForType := f.ConstructedFor()
	constructedForDgraphPredicate := f.ConstructedForDgraphPredicate()

	// aggregateChildren contains the count query field and mainField (described below).
	// otherAggregateChildren contains other min,max,sum,avg fields.
	// These fields are considered separately as filters (auth and other filters) need to
	// be added to count fields and mainFields but not for other aggregate fields.
	var aggregateChildren []*dql.GraphQuery
	var otherAggregateChildren []*dql.GraphQuery
	// mainField contains the queried Aggregate Field and has all var fields inside it.
	// Eg. the mainQuery for
	// postsAggregate {
	//   titleMin
	// }
	// is
	// Author.postsAggregate : Author.posts {
	//   Author.postsAggregate_titleVar as Post.title
	//   ... other queried aggregate fields
	// }
	mainField := &dql.GraphQuery{
		Alias: f.DgraphAlias(),
		Attr:  constructedForDgraphPredicate,
	}

	// Filter for aggregate Fields. This is added to all count aggregate fields
	// and mainField
	fieldFilter, _ := f.ArgValue("filter").(map[string]interface{})
	_ = addFilter(mainField, constructedForType, fieldFilter)

	// Add type filter in case the Dgraph predicate for which the aggregate
	// field belongs to is a reverse edge
	if strings.HasPrefix(constructedForDgraphPredicate, "~") {
		addTypeFilter(mainField, f.ConstructedFor())
	}

	// isAggregateVarAdded is a map from field name to boolean. It is used to
	// ensure that a field is added to Var query at maximum once.
	// Eg. Even if scoreMax and scoreMin are queried, the corresponding field will
	// contain "scoreVar as Tweets.score" only once.
	isAggregateVarAdded := make(map[string]bool)

	// Iterate over fields queried inside aggregate.
	for _, aggregateField := range f.SelectionSet() {

		// Handle count fields inside aggregate fields.
		if aggregateField.Name() == "count" {
			aggregateChild := &dql.GraphQuery{
				Alias: aggregateField.DgraphAlias() + "_" + f.DgraphAlias(),
				Attr:  "count(" + constructedForDgraphPredicate + ")",
			}
			// Add filter to count aggregation field.
			_ = addFilter(aggregateChild, constructedForType, fieldFilter)

			// Add type filter in case the Dgraph predicate for which the aggregate
			// field belongs to is a reverse edge
			if strings.HasPrefix(constructedForDgraphPredicate, "~") {
				addTypeFilter(aggregateChild, f.ConstructedFor())
			}

			aggregateChildren = append(aggregateChildren, aggregateChild)
			continue
		}
		// Handle other aggregate functions than count
		aggregateFunctions := []string{"Max", "Min", "Sum", "Avg"}
		for _, function := range aggregateFunctions {
			aggregateFldName := aggregateField.Name()
			// A field can have at maximum one aggregation function as suffix.
			if strings.HasSuffix(aggregateFldName, function) {
				// constructedForField contains the field name for which aggregate function
				// has been queried. Eg. name for nameMax. Removing last 3 characters as all
				// aggregation functions have length 3
				constructedForField := aggregateFldName[:len(aggregateFldName)-3]
				// constructedForDgraphPredicate stores the Dgraph predicate for which aggregate function
				// has been queried. Eg. Post.name for nameMin
				constructedForDgraphPredicateField := aggregateField.DgraphPredicateForAggregateField()
				// Adding the corresponding var field if it has not been added before. isAggregateVarAdded
				// ensures that a var queried is added at maximum once.
				if !isAggregateVarAdded[constructedForField] {
					child := &dql.GraphQuery{
						Var:  f.DgraphAlias() + "_" + constructedForField + "Var",
						Attr: constructedForDgraphPredicateField,
					}
					// The var field is added to mainQuery. This adds the following DQL query.
					// Author.postsAggregate : Author.posts {
					//   Author.postsAggregate_nameVar as Post.name
					// }
					mainField.Children = append(mainField.Children, child)
					isAggregateVarAdded[constructedForField] = true
				}
				aggregateChild := &dql.GraphQuery{
					Alias: aggregateField.DgraphAlias() + "_" + f.DgraphAlias(),
					Attr: strings.ToLower(function) +
						"(val(" + "" + f.DgraphAlias() + "_" + constructedForField + "Var))",
				}
				// This adds the following DQL query
				// PostAggregateResult.nameMin_Author.postsAggregate : min(val(Author.postsAggregate_nameVar))
				otherAggregateChildren = append(otherAggregateChildren, aggregateChild)
				break
			}
		}
	}
	// mainField is only added as an aggregate child if it has any children fields inside it.
	// This ensures that if only count aggregation field is there, the mainField is not added.
	// As mainField contains only var fields. It is not needed in case of count.
	if len(mainField.Children) > 0 {
		aggregateChildren = append([]*dql.GraphQuery{mainField}, aggregateChildren...)
	}
	rbac := auth.evaluateStaticRules(constructedForType)
	if rbac == schema.Negative {
		return nil, nil
	}
	var parentVarName, parentQryName string
	if len(f.SelectionSet()) > 0 && !auth.isWritingAuth && auth.hasAuthRules {
		parentVarName = auth.parentVarName
		parentQryName = auth.varName
		auth.parentVarName = auth.varGen.Next(f.Type(), "", "", auth.isWritingAuth)
		auth.varName = auth.varGen.Next(f.Type(), "", "", auth.isWritingAuth)
	}
	var fieldAuth, retAuthQueries []*dql.GraphQuery
	var authFilter *dql.FilterTree
	if rbac == schema.Uncertain {
		fieldAuth, authFilter = auth.rewriteAuthQueries(constructedForType)
	}
	// At this stage aggregateChildren only contains the count aggregate fields and
	// possibly mainField. Auth filters are added to count aggregation fields and
	// mainField. Adding filters only for mainField is sufficient for other aggregate
	// functions as the aggregation functions use var from mainField.

	// Adds auth queries. The variable authQueriesAppended ensures that auth queries are
	// appended only once. This also merges auth filters and any other filters of count
	// aggregation fields / mainField.
	if len(f.SelectionSet()) > 0 && !auth.isWritingAuth && auth.hasAuthRules {
		commonAuthQueryVars := buildCommonAuthQueries(f, auth, parentVarName)
		// add child filter to parent query, auth filters to selection query and
		// selection query as a filter to child
		commonAuthQueryVars.selectionQry.Filter = authFilter
		var authQueriesAppended = false
		for _, aggregateChild := range aggregateChildren {
			if !authQueriesAppended {
				commonAuthQueryVars.parentQry.Children[0].Filter = aggregateChild.Filter
				retAuthQueries = append(retAuthQueries, commonAuthQueryVars.parentQry, commonAuthQueryVars.selectionQry)
				authQueriesAppended = true
			}
			aggregateChild.Filter = &dql.FilterTree{
				Func: &dql.Function{
					Name: "uid",
					Args: []dql.Arg{{Value: commonAuthQueryVars.selectionQry.Var}},
				},
			}
		}
		// Restore the auth state after processing is done.
		auth.parentVarName = parentVarName
		auth.varName = parentQryName
	}
	// otherAggregation Children are appended to aggregationChildren to return them.
	// This step is performed at the end to ensure that auth and other filters are
	// not added to them.
	aggregateChildren = append(aggregateChildren, otherAggregateChildren...)
	retAuthQueries = append(retAuthQueries, fieldAuth...)
	return aggregateChildren, retAuthQueries
}

// TODO(GRAPHQL-874), Optimise Query rewriting in case of multiple alias with same filter.
// addSelectionSetFrom adds all the selections from field into q, and returns a list
// of extra queries needed to satisfy auth requirements
func addSelectionSetFrom(
	q *dql.GraphQuery,
	field schema.Field,
	auth *authRewriter) []*dql.GraphQuery {

	var authQueries []*dql.GraphQuery

	selSet := field.SelectionSet()
	if len(selSet) > 0 {
		// Only add dgraph.type as a child if this field is an abstract type and has some children.
		// dgraph.type would later be used in CompleteObject as different objects in the resulting
		// JSON would return different fields based on their concrete type.
		if field.AbstractType() {
			q.Children = append(q.Children, &dql.GraphQuery{
				Attr: "dgraph.type",
			})

		} else if !auth.writingAuth() &&
			len(selSet) == 1 &&
			selSet[0].Name() == schema.Typename {
			q.Children = append(q.Children, &dql.GraphQuery{
				// we don't need this for auth queries because they are added by us used for internal purposes.
				// Querying it for them would just add an overhead which we can avoid.
				Attr:  "uid",
				Alias: "dgraph.uid",
			})
		}
	}

	// These fields might not have been requested by the user directly as part of the query but
	// are required in the body template for other @custom fields requested within the query.
	// We must fetch them from Dgraph.
	requiredFields := make(map[string]schema.FieldDefinition)
	// fieldAdded is a map from field's dgraph alias to bool.
	// It tells whether a field with that dgraph alias has been added to DQL query or not.
	fieldAdded := make(map[string]bool)

	for _, f := range field.SelectionSet() {
		if f.IsCustomHTTP() {
			for dgAlias, fieldDef := range f.CustomRequiredFields() {
				requiredFields[dgAlias] = fieldDef
			}
			// This field is resolved through a custom directive so its selection set doesn't need
			// to be part of query rewriting.
			continue
		}
		// We skip typename because we can generate the information from schema or
		// dgraph.type depending upon if the type is interface or not. For interface type
		// we always query dgraph.type and can pick up the value from there.
		if f.Skip() || !f.Include() || f.Name() == schema.Typename {
			continue
		}

		// Handle aggregation queries
		if f.IsAggregateField() {
			aggregateChildren, aggregateAuthQueries := buildAggregateFields(f, auth)

			authQueries = append(authQueries, aggregateAuthQueries...)
			q.Children = append(q.Children, aggregateChildren...)
			// As all child fields inside aggregate have been looked at. We can continue
			fieldAdded[f.DgraphAlias()] = true
			continue
		}

		child := &dql.GraphQuery{
			Alias: f.DgraphAlias(),
		}

		// if field of IDType has @external directive then it means that
		// it stored as String with Hash index internally in the dgraph.
		if f.Type().Name() == schema.IDType && !f.IsExternal() {
			child.Attr = "uid"
		} else {
			child.Attr = f.DgraphPredicate()
		}

		filter, _ := f.ArgValue("filter").(map[string]interface{})
		// if this field has been filtered out by the filter, then don't add it in DQL query
		if includeField := addFilter(child, f.Type(), filter); !includeField {
			continue
		}

		// Add type filter in case the Dgraph predicate is a reverse edge
		if strings.HasPrefix(f.DgraphPredicate(), "~") {
			addTypeFilter(child, f.Type())
		}

		addOrder(child, f)
		addPagination(child, f)
		addCascadeDirective(child, f)
		rbac := auth.evaluateStaticRules(f.Type())

		// Since the recursion processes the query in bottom up way, we store the state of the so
		// that we can restore it later.
		var parentVarName, parentQryName string
		if len(f.SelectionSet()) > 0 && !auth.isWritingAuth && auth.hasAuthRules {
			parentVarName = auth.parentVarName
			parentQryName = auth.varName
			auth.parentVarName = auth.varGen.Next(f.Type(), "", "", auth.isWritingAuth)
			auth.varName = auth.varGen.Next(f.Type(), "", "", auth.isWritingAuth)
		}

		var selectionAuth []*dql.GraphQuery
		if !f.Type().IsGeo() {
			selectionAuth = addSelectionSetFrom(child, f, auth)
		}

		restoreAuthState := func() {
			if len(f.SelectionSet()) > 0 && !auth.isWritingAuth && auth.hasAuthRules {
				// Restore the auth state after processing is done.
				auth.parentVarName = parentVarName
				auth.varName = parentQryName
			}
		}

		fieldAdded[f.DgraphAlias()] = true

		if rbac == schema.Positive || rbac == schema.Uncertain {
			q.Children = append(q.Children, child)
		}

		var fieldAuth []*dql.GraphQuery
		var authFilter *dql.FilterTree
		if rbac == schema.Negative && auth.hasAuthRules && auth.hasCascade && !auth.isWritingAuth {
			// If RBAC rules are evaluated to Negative but we have cascade directive we continue
			// to write the query and add a dummy filter that doesn't return anything.
			// Example: AdminTask5 as var(func: uid())
			q.Children = append(q.Children, child)
			varName := auth.varGen.Next(f.Type(), "", "", auth.isWritingAuth)
			fieldAuth = append(fieldAuth, &dql.GraphQuery{
				Var:  varName,
				Attr: "var",
				Func: &dql.Function{
					Name: "uid",
				},
			})
			authFilter = &dql.FilterTree{
				Func: &dql.Function{
					Name: "uid",
					Args: []dql.Arg{{Value: varName}},
				},
			}
			rbac = schema.Positive
		} else if rbac == schema.Negative {
			// If RBAC rules are evaluated to Negative, we don't write queries for deeper levels.
			// Hence we don't need to do any further processing for this field.
			restoreAuthState()
			continue
		}

		// If RBAC rules are evaluated to `Uncertain` then we add the Auth rules.
		if rbac == schema.Uncertain {
			fieldAuth, authFilter = auth.rewriteAuthQueries(f.Type())
		}

		if len(f.SelectionSet()) > 0 && !auth.isWritingAuth && auth.hasAuthRules {
			commonAuthQueryVars := buildCommonAuthQueries(f, auth, parentVarName)
			// add child filter to parent query, auth filters to selection query and
			// selection query as a filter to child
			commonAuthQueryVars.parentQry.Children[0].Filter = child.Filter
			commonAuthQueryVars.selectionQry.Filter = authFilter
			child.Filter = &dql.FilterTree{
				Func: &dql.Function{
					Name: "uid",
					Args: []dql.Arg{{Value: commonAuthQueryVars.selectionQry.Var}},
				},
			}
			authQueries = append(authQueries, commonAuthQueryVars.parentQry, commonAuthQueryVars.selectionQry)
		}
		authQueries = append(authQueries, selectionAuth...)
		authQueries = append(authQueries, fieldAuth...)
		restoreAuthState()
	}

	// Sort the required fields before adding them to q.Children so that the query produced after
	// rewriting has a predictable order.
	rfset := make([]string, 0, len(requiredFields))
	for dgAlias := range requiredFields {
		rfset = append(rfset, dgAlias)
	}
	sort.Strings(rfset)

	// Add fields required by other custom fields which haven't already been added as a
	// child to be fetched from Dgraph.
	for _, dgAlias := range rfset {
		if !fieldAdded[dgAlias] {
			f := requiredFields[dgAlias]
			child := &dql.GraphQuery{
				Alias: f.DgraphAlias(),
			}

			if f.Type().Name() == schema.IDType && !f.IsExternal() {
				child.Attr = "uid"
			} else {
				child.Attr = f.DgraphPredicate()
			}
			q.Children = append(q.Children, child)
		}
	}

	return authQueries
}

func addOrder(q *dql.GraphQuery, field schema.Field) {
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

func addPagination(q *dql.GraphQuery, field schema.Field) {
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

func addCascadeDirective(q *dql.GraphQuery, field schema.Field) {
	q.Cascade = field.Cascade()
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

func extractQueryFilter(f schema.Field) map[string]interface{} {
	filter, _ := f.ArgValue("filter").(map[string]interface{})
	return filter
}

func idFilter(filter map[string]interface{}, idField schema.FieldDefinition) []uint64 {
	if filter == nil || idField == nil {
		return nil
	}

	idsFilter := filter[idField.Name()]
	if idsFilter == nil {
		return nil
	}
	var idsSlice []interface{}
	// idsFilter can be an single string value (most common) or
	// an interface{} slice
	switch f := idsFilter.(type) {
	case string:
		idsSlice = append(idsSlice, f)
	case []interface{}:
		idsSlice = f
	default:
		// if an unexpected type is encountered, fail silently
		return nil
	}
	return convertIDs(idsSlice)
}

// addFilter adds a filter to the input DQL query. It returns false if the field for which the
// filter was specified should not be included in the DQL query.
// Currently, it would only be false for a union field when no memberTypes are queried.
func addFilter(q *dql.GraphQuery, typ schema.Type, filter map[string]interface{}) bool {
	if len(filter) == 0 {
		return true
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

	if typ.IsUnion() {
		if filter, includeField := buildUnionFilter(typ, filter); includeField {
			q.Filter = filter
		} else {
			return false
		}
	} else {
		q.Filter = buildFilter(typ, filter)
	}
	if filterAtRoot {
		addTypeFilter(q, typ)
	}
	return true
}

// buildFilter builds a Dgraph dql.FilterTree from a GraphQL 'filter' arg.
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
func buildFilter(typ schema.Type, filter map[string]interface{}) *dql.FilterTree {

	var ands []*dql.FilterTree
	var or *dql.FilterTree
	// Get a stable ordering so we generate the same thing each time.
	var keys []string
	for key := range filter {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Each key in filter is either "and", "or", "not" or the field name it
	// applies to such as "title" in: `title: { anyofterms: "GraphQL" }``
	for _, field := range keys {
		if filter[field] == nil {
			continue
		}
		switch field {

		// In 'and', 'or' and 'not' cases, filter[field] must be a map[string]interface{}
		// or it would have failed GraphQL validation - e.g. 'filter: { and: 10 }'
		// would have failed validation.

		case "and":
			// title: { anyofterms: "GraphQL" }, and: { ... }
			//                       we are here ^^
			// ->
			// @filter(anyofterms(Post.title, "GraphQL") AND ... )

			// The value of the and argument can be either an object or an array, hence we handle
			// both.
			// ... and: {}
			// ... and: [{}]
			switch v := filter[field].(type) {
			case map[string]interface{}:
				ft := buildFilter(typ, v)
				ands = append(ands, ft)
			case []interface{}:
				for _, obj := range v {
					ft := buildFilter(typ, obj.(map[string]interface{}))
					ands = append(ands, ft)
				}
			}
		case "or":
			// title: { anyofterms: "GraphQL" }, or: { ... }
			//                       we are here ^^
			// ->
			// @filter(anyofterms(Post.title, "GraphQL") OR ... )

			// The value of the or argument can be either an object or an array, hence we handle
			// both.
			// ... or: {}
			// ... or: [{}]
			switch v := filter[field].(type) {
			case map[string]interface{}:
				or = buildFilter(typ, v)
			case []interface{}:
				ors := make([]*dql.FilterTree, 0, len(v))
				for _, obj := range v {
					ft := buildFilter(typ, obj.(map[string]interface{}))
					ors = append(ors, ft)
				}
				or = &dql.FilterTree{
					Child: ors,
					Op:    "or",
				}
			}
		case "not":
			// title: { anyofterms: "GraphQL" }, not: { isPublished: true}
			//                       we are here ^^
			// ->
			// @filter(anyofterms(Post.title, "GraphQL") AND NOT eq(Post.isPublished, true))
			not := buildFilter(typ, filter[field].(map[string]interface{}))
			ands = append(ands,
				&dql.FilterTree{
					Op:    "not",
					Child: []*dql.FilterTree{not},
				})
		default:
			//// It's a base case like:
			//// title: { anyofterms: "GraphQL" } ->  anyofterms(Post.title: "GraphQL")
			//// numLikes: { between : { min : 10,  max:100 }}
			switch dgFunc := filter[field].(type) {
			case map[string]interface{}:
				// title: { anyofterms: "GraphQL" } ->  anyofterms(Post.title, "GraphQL")
				// OR
				// numLikes: { le: 10 } -> le(Post.numLikes, 10)

				fn, val := first(dgFunc)
				if val == nil {
					// If it is `eq` filter for eg: {filter: { title: {eq: null }}} then
					// it will be interpreted as {filter: {not: {has: title}}}, rest of
					// the filters with null values will be ignored in query rewriting.
					if fn == "eq" {
						hasFilterMap := map[string]interface{}{"not": map[string]interface{}{"has": []interface{}{field}}}
						ands = append(ands, buildFilter(typ, hasFilterMap))
					}
					continue
				}
				args := []dql.Arg{{Value: typ.DgraphPredicate(field)}}
				switch fn {
				// in takes List of Scalars as argument, for eg:
				// code : { in: ["abc", "def", "ghi"] } -> eq(State.code,"abc","def","ghi")
				case "in":
					// No need to check for List types as this would pass GraphQL validation
					// if val was not list
					vals := val.([]interface{})
					fn = "eq"

					for _, v := range vals {
						args = append(args, dql.Arg{Value: maybeQuoteArg(fn, v)})
					}
				case "between":
					// numLikes: { between : { min : 10,  max:100 }} should be rewritten into
					// 	between(numLikes,10,20). Order of arguments (min,max) is neccessary or
					// it will return empty
					vals := val.(map[string]interface{})
					args = append(args, dql.Arg{Value: maybeQuoteArg(fn, vals["min"])},
						dql.Arg{Value: maybeQuoteArg(fn, vals["max"])})
				case "near":
					// For Geo type we have `near` filter which is written as follows:
					// { near: { distance: 33.33, coordinate: { latitude: 11.11, longitude: 22.22 } } }
					near := val.(map[string]interface{})
					coordinate := near["coordinate"].(map[string]interface{})
					var buf bytes.Buffer
					buildPoint(coordinate, &buf)
					args = append(args, dql.Arg{Value: buf.String()},
						dql.Arg{Value: fmt.Sprintf("%v", near["distance"])})
				case "within":
					// For Geo type we have `within` filter which is written as follows:
					// { within: { polygon: { coordinates: [ { points: [
					// { latitude: 11.11, longitude: 22.22}, { latitude: 15.15, longitude: 16.16} ,
					// { latitude: 20.20, longitude: 21.21} ]}] } } }
					within := val.(map[string]interface{})
					polygon := within["polygon"].(map[string]interface{})
					var buf bytes.Buffer
					buildPolygon(polygon, &buf)
					args = append(args, dql.Arg{Value: buf.String()})
				case "contains":
					// For Geo type we have `contains` filter which is either point or polygon and is written
					// as follows:
					// For point: { contains: { point: { latitude: 11.11, longitude: 22.22 }}}
					// For polygon: { contains: { polygon: { coordinates: [ { points: [
					// { latitude: 11.11, longitude: 22.22}, { latitude: 15.15, longitude: 16.16} ,
					// { latitude: 20.20, longitude: 21.21} ]}] } } }
					contains := val.(map[string]interface{})
					var buf bytes.Buffer
					if polygon, ok := contains["polygon"].(map[string]interface{}); ok {
						buildPolygon(polygon, &buf)
					} else if point, ok := contains["point"].(map[string]interface{}); ok {
						buildPoint(point, &buf)
					}
					args = append(args, dql.Arg{Value: buf.String()})
					// TODO: for both contains and intersects, we should use @oneOf in the inbuilt
					// schema. Once we have variable validation hook available in gqlparser, we can
					// do this. So, if either both the children are given or none of them is given,
					// we should get an error at parser level itself. Right now, if both "polygon"
					// and "point" are given, we only use polygon. If none of them are given,
					// an incorrect DQL query will be formed and will error out from Dgraph.
				case "intersects":
					// For Geo type we have `intersects` filter which is either multi-polygon or polygon and is written
					// as follows:
					// For polygon: { intersect: { polygon: { coordinates: [ { points: [
					// { latitude: 11.11, longitude: 22.22}, { latitude: 15.15, longitude: 16.16} ,
					// { latitude: 20.20, longitude: 21.21} ]}] } } }
					// For multi-polygon : { intersect: { multiPolygon: { polygons: [{ coordinates: [ { points: [
					// { latitude: 11.11, longitude: 22.22}, { latitude: 15.15, longitude: 16.16} ,
					// { latitude: 20.20, longitude: 21.21} ]}] }] } } }
					intersects := val.(map[string]interface{})
					var buf bytes.Buffer
					if polygon, ok := intersects["polygon"].(map[string]interface{}); ok {
						buildPolygon(polygon, &buf)
					} else if multiPolygon, ok := intersects["multiPolygon"].(map[string]interface{}); ok {
						buildMultiPolygon(multiPolygon, &buf)
					}
					args = append(args, dql.Arg{Value: buf.String()})
				default:
					args = append(args, dql.Arg{Value: maybeQuoteArg(fn, val)})
				}
				ands = append(ands, &dql.FilterTree{
					Func: &dql.Function{
						Name: fn,
						Args: args,
					},
				})
			case []interface{}:
				// has: [comments, text] -> has(comments) AND has(text)
				// ids: [ 0x123, 0x124]
				switch field {
				case "has":
					ands = append(ands, buildHasFilterList(typ, dgFunc)...)
				default:
					// If ids is an @external field then it gets rewritten just like `in` filter
					//  ids: [0x123, 0x124] -> eq(typeName.ids, "0x123", 0x124)
					if typ.Field(field).IsExternal() {
						fn := "eq"
						args := []dql.Arg{{Value: typ.DgraphPredicate(field)}}
						for _, v := range dgFunc {
							args = append(args, dql.Arg{Value: maybeQuoteArg(fn, v)})
						}
						ands = append(ands, &dql.FilterTree{
							Func: &dql.Function{
								Name: fn,
								Args: args,
							},
						})
					} else {
						// if it is not an @external field then it is rewritten as uid filter.
						// ids: [ 0x123, 0x124 ] -> uid(0x123, 0x124)
						ids := convertIDs(dgFunc)
						ands = append(ands, &dql.FilterTree{
							Func: &dql.Function{
								Name: "uid",
								UID:  ids,
							},
						})
					}
				}
			case interface{}:
				// isPublished: true -> eq(Post.isPublished, true)
				// OR an enum case
				// postType: Question -> eq(Post.postType, "Question")

				fn := "eq"
				ands = append(ands, &dql.FilterTree{
					Func: &dql.Function{
						Name: fn,
						Args: []dql.Arg{
							{Value: typ.DgraphPredicate(field)},
							{Value: fmt.Sprintf("%v", dgFunc)},
						},
					},
				})
			}
		}
	}

	var andFt *dql.FilterTree
	if len(ands) == 0 {
		return or
	} else if len(ands) == 1 {
		andFt = ands[0]
	} else if len(ands) > 1 {
		andFt = &dql.FilterTree{
			Op:    "and",
			Child: ands,
		}
	}

	if or == nil {
		return andFt
	}

	return &dql.FilterTree{
		Op:    "or",
		Child: []*dql.FilterTree{andFt, or},
	}
}

func buildHasFilterList(typ schema.Type, fieldsSlice []interface{}) []*dql.FilterTree {
	var ands []*dql.FilterTree
	fn := "has"
	for _, fieldName := range fieldsSlice {
		ands = append(ands, &dql.FilterTree{
			Func: &dql.Function{
				Name: fn,
				Args: []dql.Arg{
					{Value: typ.DgraphPredicate(fieldName.(string))},
				},
			},
		})
	}
	return ands
}

func buildPoint(point map[string]interface{}, buf *bytes.Buffer) {
	x.Check2(buf.WriteString(fmt.Sprintf("[%v,%v]", point[schema.Longitude],
		point[schema.Latitude])))
}

func buildPolygon(polygon map[string]interface{}, buf *bytes.Buffer) {
	coordinates, _ := polygon[schema.Coordinates].([]interface{})
	comma1 := ""

	x.Check2(buf.WriteString("["))
	for _, r := range coordinates {
		ring, _ := r.(map[string]interface{})
		points, _ := ring[schema.Points].([]interface{})
		comma2 := ""

		x.Check2(buf.WriteString(comma1))
		x.Check2(buf.WriteString("["))
		for _, p := range points {
			x.Check2(buf.WriteString(comma2))
			point, _ := p.(map[string]interface{})
			buildPoint(point, buf)
			comma2 = ","
		}
		x.Check2(buf.WriteString("]"))
		comma1 = ","
	}
	x.Check2(buf.WriteString("]"))
}

func buildMultiPolygon(multipolygon map[string]interface{}, buf *bytes.Buffer) {
	polygons, _ := multipolygon[schema.Polygons].([]interface{})
	comma := ""

	x.Check2(buf.WriteString("["))
	for _, p := range polygons {
		polygon, _ := p.(map[string]interface{})
		x.Check2(buf.WriteString(comma))
		buildPolygon(polygon, buf)
		comma = ","
	}
	x.Check2(buf.WriteString("]"))
}

func buildUnionFilter(typ schema.Type, filter map[string]interface{}) (*dql.FilterTree, bool) {
	memberTypesList, ok := filter["memberTypes"].([]interface{})
	// if memberTypes was specified to be an empty list like: { memberTypes: [], ...},
	// then we don't need to include the field, on which the filter was specified, in the query.
	if ok && len(memberTypesList) == 0 {
		return nil, false
	}

	ft := &dql.FilterTree{
		Op: "or",
	}

	// now iterate over the filtered member types for this union and build FilterTree for them
	for _, memberType := range typ.UnionMembers(memberTypesList) {
		memberTypeFilter, _ := filter[schema.CamelCase(memberType.Name())+"Filter"].(map[string]interface{})
		var memberTypeFt *dql.FilterTree
		if len(memberTypeFilter) == 0 {
			// if the filter for a member type wasn't specified, was null, or was specified as {};
			// then we need to query all nodes of that member type for the field on which the filter
			// was specified.
			memberTypeFt = &dql.FilterTree{Func: buildTypeFunc(memberType.DgraphName())}
		} else {
			// else we need to query only the nodes which match the filter for that member type
			memberTypeFt = &dql.FilterTree{
				Op: "and",
				Child: []*dql.FilterTree{
					{Func: buildTypeFunc(memberType.DgraphName())},
					buildFilter(memberType, memberTypeFilter),
				},
			}
		}
		ft.Child = append(ft.Child, memberTypeFt)
	}

	// return true because we want to include the field with filter in query
	return ft, true
}

func maybeQuoteArg(fn string, arg interface{}) string {
	switch arg := arg.(type) {
	case string: // dateTime also parsed as string
		if fn == "regexp" {
			return arg
		}
		return fmt.Sprintf("%q", arg)
	case float64, float32:
		return fmt.Sprintf("\"%v\"", arg)
	default:
		return fmt.Sprintf("%v", arg)
	}
}

// first returns the first element it finds in a map - we bump into lots of one-element
// maps like { "anyofterms": "GraphQL" }.  fst helps extract that single mapping.
func first(aMap map[string]interface{}) (string, interface{}) {
	for key, val := range aMap {
		return key, val
	}
	return "", nil
}
