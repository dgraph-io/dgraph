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
	"sort"
	"strconv"

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/vektah/gqlparser/gqlerror"
)

// QueryRewriter can build a Dgraph gql.GraphQuery from a GraphQL query, and
// can build a Dgraph gql.GraphQuery to follow a GraphQL mutation.
//
// GraphQL queries come in like:
//
// query {
// 	 getAuthor(id: "0x1") {
// 	  name
// 	 }
// }
//
// and get rewritten straight to Dgraph.  But mutations come in like:
//
// mutation addAuthor($auth: AuthorInput!) {
//   addAuthor(input: $auth) {
// 	   author {
// 	     id
// 	     name
// 	   }
//   }
// }
//
// Where `addAuthor(input: $auth)` implies a mutation that must get run, and the
// remainder implies a query to run and return the newly created author, so the
// mutation query rewriting is dependent on the context set up by the result of
// the mutation.
type QueryRewriter interface {
	Rewrite(q schema.Query) (*gql.GraphQuery, error)
	FromMutationResult(m schema.Mutation, uids map[string]string) (*gql.GraphQuery, error)
}

type queryRewriter struct{}

// NewQueryRewriter returns a new query rewriter.
func NewQueryRewriter() QueryRewriter {
	return &queryRewriter{}
}

// Rewrite rewrites a GraphQL query into a Dgraph GraphQuery.
func (qr *queryRewriter) Rewrite(gqlQuery schema.Query) (*gql.GraphQuery, error) {

	// currently only handles getT(id: "0x123") queries

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
		uid, err := gqlQuery.IDArgValue()
		if err != nil {
			return nil, err
		}

		dgQuery := rewriteAsGet(gqlQuery, uid)
		addTypeFilter(dgQuery, gqlQuery.Type())

		return dgQuery, nil

	case schema.FilterQuery:
		return rewriteAsQuery(gqlQuery), nil
	default:
		return nil, gqlerror.Errorf("unimplemented query type %s", gqlQuery.QueryType())
	}
}

// FromMutation rewrites the query part of aGraphQL mutation into a Dgraph query.
func (qr *queryRewriter) FromMutationResult(
	gqlMutation schema.Mutation,
	uids map[string]string) (*gql.GraphQuery, error) {

	switch gqlMutation.MutationType() {
	case schema.AddMutation:
		uid, err := strconv.ParseUint(uids[createdNode], 0, 64)
		if err != nil {
			return nil, schema.GQLWrapf(err,
				"recieved %s as an assigned uid from Dgraph, but couldn't parse it as uint64",
				uids[createdNode])
		}

		return rewriteAsGet(gqlMutation.QueryField(), uid), nil

	case schema.UpdateMutation:
		uid, err := getUpdUID(gqlMutation)
		if err != nil {
			return nil, err
		}

		return rewriteAsGet(gqlMutation.QueryField(), uid), nil

	default:
		return nil, gqlerror.Errorf("can't rewrite %s mutations to Dgraph query",
			gqlMutation.MutationType())
	}
}

func rewriteAsGet(field schema.Field, uid uint64) *gql.GraphQuery {
	dgQuery := &gql.GraphQuery{
		Attr: field.ResponseName(),
		Func: &gql.Function{
			Name: "uid",
			UID:  []uint64{uid},
		},
	}

	addSelectionSetFrom(dgQuery, field)

	return dgQuery
}

func rewriteAsQuery(field schema.Field) *gql.GraphQuery {
	dgQuery := &gql.GraphQuery{
		Attr: field.ResponseName(),
	}

	addTypeFunc(dgQuery, field.Type())
	addFilter(dgQuery, field)
	addOrder(dgQuery, field)
	addPagination(dgQuery, field)
	addSelectionSetFrom(dgQuery, field)

	return dgQuery
}

func addTypeFilter(q *gql.GraphQuery, typ schema.Type) {
	thisFilter := &gql.FilterTree{
		Func: &gql.Function{
			Name: "type",
			Args: []gql.Arg{{Value: typ.Name()}},
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

func addTypeFunc(q *gql.GraphQuery, typ schema.Type) {
	q.Func = &gql.Function{
		Name: "type",
		Args: []gql.Arg{{Value: typ.Name()}},
	}

}

func addSelectionSetFrom(q *gql.GraphQuery, field schema.Field) {
	for _, f := range field.SelectionSet() {
		child := &gql.GraphQuery{}

		if f.Alias() != "" {
			child.Alias = f.Alias()
		} else {
			child.Alias = f.Name()
		}

		if f.Type().Name() == schema.IDType {
			child.Attr = "uid"
		} else {
			child.Attr = fmt.Sprintf("%s.%s", field.Type().Name(), f.Name())
		}

		addFilter(child, f)
		addOrder(child, f)
		addPagination(child, f)

		addSelectionSetFrom(child, f)

		q.Children = append(q.Children, child)
	}
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
				&pb.Order{Attr: fmt.Sprintf("%s.%s", field.Type().Name(), asc)})
		} else if desc, ok := descArg.(string); ok {
			q.Order = append(q.Order,
				&pb.Order{Attr: fmt.Sprintf("%s.%s", field.Type().Name(), desc), Desc: true})
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

func addFilter(q *gql.GraphQuery, field schema.Field) {
	filter, ok := field.ArgValue("filter").(map[string]interface{})
	if !ok {
		return
	}

	q.Filter = buildFilter(field.Type(), filter)
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
// @filter(anyofterms(Post.title, "GraphQL") AND eq(isPublished, true))
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
			// @filter(anyofterms(Post.title, "GraphQL") AND NOT eq(isPublished, true))
			not := buildFilter(typ, filter[field].(map[string]interface{}))
			ands = append(ands,
				&gql.FilterTree{
					Op:    "not",
					Child: []*gql.FilterTree{not},
				})
		default:
			// It's a base case like:
			// title: { anyofterms: "GraphQL" } ->  anyofterms(title: "GraphQL")

			switch dgFunc := filter[field].(type) {
			case map[string]interface{}:
				// title: { anyofterms: "GraphQL" } ->  anyofterms(title, "GraphQL")
				// OR
				// numLikes: { le: 10 } -> le(numLikes, 10)
				fn, val := fst(dgFunc)
				ands = append(ands, &gql.FilterTree{
					Func: &gql.Function{
						Name: fn,
						Args: []gql.Arg{
							{Value: fmt.Sprintf("%s.%s", typ.Name(), field)},
							{Value: maybeQuoteArg(fn, val)},
						},
					},
				})
			case interface{}:
				// isPublished: true -> eq(isPublished, true)
				// OR an enum case
				// postType: Question -> eq(postType, "Question")
				fn := "eq"
				ands = append(ands, &gql.FilterTree{
					Func: &gql.Function{
						Name: fn,
						Args: []gql.Arg{
							{Value: fmt.Sprintf("%s.%s", typ.Name(), field)},
							{Value: maybeQuoteArg(fn, dgFunc)},
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
			return fmt.Sprintf("%v", arg)
		}
		return fmt.Sprintf("%q", arg)
	default:
		return fmt.Sprintf("%v", arg)
	}
}

// fst returns the first element it finds in a map - we bump into lots of one-element
// maps like { "anyofterms": "GraphQL" }.  fst helps extract that single mapping.
func fst(aMap map[string]interface{}) (string, interface{}) {
	for key, val := range aMap {
		return key, val
	}
	return "", nil
}
