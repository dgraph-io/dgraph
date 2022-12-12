package resolve

import (
	"context"
	"encoding/json"

	"github.com/dgraph-io/dgraph/dql"
	"github.com/dgraph-io/dgraph/graphql/schema"
)

//// Introduction ////

// Separation of concerns. Best-case scenario: This file contains all code
// related to query resolution in the context of Apollo Federation.

// TODO Add support for multiple `@key` directives on a type (see `schema.Query.RepresentationsArg()`)
// TODO Add support for different `__typename` arguments in one `_entities` query (see `schema.Query.RepresentationsArg()`)
// TODO Possibly move `schema.Query.RepresentationsArg()` here? But it's used in `outputnode_graphql.go` as well.

// Unfortunately, I can't just add a new field to the selection set in a
// `QueryRewriter`. Depending on where you put it, it'll either trigger errors,
// or just be stripped / ignored in the JSON serialization step during Dgraph
// query execution.
// In order to properly fulfill the `_entities` query contract, we need this
// ability though.
// Options:
// A) Adapt code in `wrappers.go` and rewrite the GQL query itself in
//    `entitiesQueryResolver.rewrite`.
//    Drawback: We'd make the wrappers even more mutable - is that desirable?
// B) Wrap `QueryRewriter` and `DgraphExecutor` individually, rewrite query to a
//    DQL query (as before) and use the executor make sure it gets run in DQL
//    mode (so that we can actually use the added fields).
//    Drawback: Might lead to more code duplication, or not work at all, if the
//    DQL result is too different from what we need.
//    Benefit: Better encapsulation / self-contained.
//    Benefit: More in line with the rest of the code.
//    Benefit: Possibly more efficient because we skip the intermediary rewrite
// 	  step.
// Decision Log:
// * I guess it all comes down to whether option B) will work or not. I'll use
//   option A as fallback.
// * First of all, it works :) But the results are too different, and I didn't
//   find a straightforward way of converting them to GraphQL. What about a
//   compromise?
// * I started trying a compromise: Adding a single method to the schema wrapper
//   to enable adding the entity key field to the selection set, but in all
//   other regards proceeding as before.
// * I now believe that my current idea is actually less clean than changing the
//   code in `outputnode_graphql.go`. However, since it's easier to implement,
//   and since I'm almost finished, I'll stick to my current idea. Revisit
//   later.
// * It works! Now on to cleanup and tests.

//// QueryResolver Implementation ////

const (
	// Used as alias for the custom id field that is added to the selection set.
	// It's quintessential that the alias contains a dot (".") - since a dot
	// is not a a valid character in a GraphQL alias, we can be sure that there
	// won't be a client-provided selection/field with the same alias.
	dgraphEntityKey = "dgraph.entityKey"
)

type entitiesQueryResolver struct {
	nested QueryResolver
}

// NewEntitiesQueryResolver creates a new query resolver for Apollo Federation
// `_entities` queries. The query resolution strategy is slightly different when
// compared to normal queries.
func NewEntitiesQueryResolver(rewriter QueryRewriter, executor DgraphExecutor) QueryResolver {
	return &entitiesQueryResolver{
		NewQueryResolver(
			&entitiesQueryRewriter{rewriter},
			// &entitiesQueryExecutor{executor},
			executor,
		),
	}
}

func (resolver *entitiesQueryResolver) Resolve(context context.Context, query schema.Query) *Resolved {
	// In the future we might (correctly) support different `__typename`
	// arguments in a single `_entities` query. In this case, we would need to
	// split the query into multiple queries and merge the results.

	// query, err := resolver.rewrite(query)
	// if err != nil {
	// 	panic("not implemented yet")
	// }

	result := resolver.nested.Resolve(context, query)

	// TODO WIP Should also be timed and traced, i.e. run inside the resolver
	// On the other side, how to proceed if we're going to support multiple
	// `@key` directives later on?
	resolver.complete(result)

	return result
}

// This function was originally called `entitiesQueryCompletion` and was moved
// here from `resolver.go`.
// Transforms the result of the `_entities` query. It changes the order of the
// result to the order of keyField in the `_representations` argument.
func (resolver *entitiesQueryResolver) complete(result *Resolved) {
	// return if Data is not present
	if len(result.Data) == 0 {
		return
	}
	query, ok := result.Field.(schema.Query)
	if !ok {
		// this function shouldn't be called for anything other than a query
		return
	}

	// Fetch the 'representations' argument from the '_entities' query
	representationsArg, err := query.RepresentationsArg()
	if err != nil {
		result.Err = schema.AppendGQLErrs(result.Err, err)
		return
	}

	var data map[string][]interface{}
	err = schema.Unmarshal(result.Data, &data)
	if err != nil {
		result.Err = schema.AppendGQLErrs(result.Err, err)
		return
	}

	// Arrange entities (returned by Dgraph) in a map: <key> to <entity>

	entitiesMap := make(map[interface{}]interface{}, len(data["_entities"]))

	for _, entity := range data["_entities"] {
		entity, ok := entity.(map[string]interface{})
		if !ok {
			// TODO WIP Should this ever happen?
			continue
		}

		key := entity[dgraphEntityKey]
		delete(entity, dgraphEntityKey)
		entitiesMap[key] = entity
	}

	// Build a new array with all entities matching the order defined
	// by the 'representations' argument.
	// It's essential to return the exact number of elements that
	// have been requested by the client.
	// If there are any non-unique keys, then the entity should
	// accordingly be placed multiple times in the result.
	// If there are any entites which have not been found/returned
	// by Dgraph, it's essential to add the original representation
	// in their place.
	// Should this not be implemented properly, the Apollo Router
	// (or equivalent) will end up merging the results incorrectly.

	orderedEntities := make([]interface{}, len(representationsArg.KeyVals))

	for i, key := range representationsArg.KeyVals {
		// TODO WIP Do we need to convert any keys?
		orderedEntities[i] = entitiesMap[key]
	}

	// replace the result obtained from the dgraph and marshal back.
	data["_entities"] = orderedEntities
	result.Data, err = json.Marshal(data)
	if err != nil {
		result.Err = schema.AppendGQLErrs(result.Err, err)
	}
}

//// QueryRewriter Implementation ////

type entitiesQueryRewriter struct {
	nested QueryRewriter
}

func (rewriter *entitiesQueryRewriter) Rewrite(context context.Context, query schema.Query) ([]*dql.GraphQuery, error) {
	if query.QueryType() != schema.EntitiesQuery {
		// TODO WIP Return error instead?
		panic("Should never happen: entitiesQueryRewriter.Rewrite has been invoked for a query other than schema.EntitiesQuery.")
	}

	authRw, err := prepareAuthRewriter(context, query)
	if err != nil {
		return nil, err
	}

	return rewriter.rewriteImpl(query, authRw)

	// representationsArg, err := query.RepresentationsArg()
	// if err != nil {
	// 	return nil, err
	// }
	// return rewriter.nested.Rewrite(context, query)
}

// This function was originally called `entitiesQuery` and was moved here from
// `query_rewriter.go`.
// Rewrites the Apollo `_entities` Query which is sent from the federation
// router to a DQL query. This query is sent to the Dgraph service to resolve
// types `extended` and defined by this service (in Federation v1 lingo).
func (rewriter *entitiesQueryRewriter) rewriteImpl(gqlQuery schema.Query, authRw *authRewriter) ([]*dql.GraphQuery, error) {
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

	parsedRepr, err := gqlQuery.RepresentationsArg()
	if err != nil {
		return nil, err
	}

	typeDefn := parsedRepr.TypeDefn
	rbac := authRw.evaluateStaticRules(typeDefn)

	dgQuery := &dql.GraphQuery{
		Attr: gqlQuery.Name(),
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
	if parsedRepr.KeyField.IsID() && !parsedRepr.KeyField.IsExternal() {
		addUIDFunc(dgQuery, convertIDs(parsedRepr.KeyVals))
	} else {
		addEqFunc(dgQuery, typeDefn.DgraphPredicate(parsedRepr.KeyField.Name()), parsedRepr.KeyVals)
	}

	// AddTypeFilter in as the Filter to the Root the Query.
	// Query will be like :-
	// 	_entities(func: ...) @filter(type(typeName)) {
	//		...
	// 	}
	addTypeFilter(dgQuery, typeDefn)

	gqlQuery.AddFieldToQuery(dgraphEntityKey, parsedRepr.KeyField)

	selectionAuth := addSelectionSetFrom(dgQuery, gqlQuery, authRw)
	addUID(dgQuery)

	// TODO Remove
	// // Add the key field back to the selection set - this will be used
	// // by `entitiesQueryCompletion` in 'resolver.go' to rearrange/map
	// // the entities according to the `representations` arg.
	// // Make sure to add any custom fields like this one after the
	// // original selection set, or else it breaks (see
	// // 'query/outputnode_graphql.go').
	// dgQuery.Children = append(dgQuery.Children, &dql.GraphQuery{
	// 	Attr:  parsedRepr.KeyField.DgraphPredicate(),
	// 	Alias: "dgraph.entityKey",
	// })

	dgQueries := authRw.addAuthQueries(typeDefn, []*dql.GraphQuery{dgQuery}, rbac)
	return append(dgQueries, selectionAuth...), nil
}

//// DgraphExecutor Wrapper ////

// TODO Remove. This was used for option "B" (see beginning of the file).
// type entitiesQueryExecutor struct {
// 	nested DgraphExecutor
// }
//
// // Wraps the Executor. Doesn't pass through the `field` variable, making Dgraph
// // run in DQL mode, so that we can add aditional fields to the query without
// // actually changing the GQL query.
// func (executor *entitiesQueryExecutor) Execute(ctx context.Context, req *dgoapi.Request, field schema.Field) (*dgoapi.Response, error) {
// 	return executor.nested.Execute(ctx, req, nil)
// }
//
// func (executor *entitiesQueryExecutor) CommitOrAbort(ctx context.Context, tc *dgoapi.TxnContext) (*dgoapi.TxnContext, error) {
// 	panic("Should never happen: entitiesQueryExecutor.CommitOrAbort has been invoked.")
// }

//// Other stuff ////

// TODO Remove. This was considered as a fallback plan - see option "A" in the
// first few lines of this file.
// func (resolver *entitiesQueryResolver) rewrite(query schema.Query) (schema.Query, error) {
// 	entityRepresentations, err := query.RepresentationsArg()
// 	if err != nil {
// 		return nil, err
// 	}
// 	// Algorithm:
// 	// * Fetch return type from entityRepresentations and set on query
// 	// * Use entityRepresentations to map the argument to the corresponding API args
// 	// * Add the entityRepresentations key arg to the selection set in order to allow mapping later
// 	// We'd need to add a few abilities to wrappers.go:
// 	// Change the return type
// 	// Change the arguments
// 	// Add new fields to the selection set
// 	return query, nil
// }
