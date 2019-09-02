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
	"github.com/dgraph-io/dgraph/gql"
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
		// the query are.  ATM, I'm not sure how to hook into the GraphQL validator
		// to get that to happen
		uid, err := gqlQuery.IDArgValue()
		if err != nil {
			return nil, err
		}

		dgQuery := rewriteAsGetQuery(gqlQuery, uid)
		addTypeFilter(dgQuery, gqlQuery.Type())

		return dgQuery, nil

	default:
		return nil, gqlerror.Errorf("only get queries are implemented")
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

		return rewriteAsGetQuery(gqlMutation.QueryField(), uid), nil

	case schema.UpdateMutation:
		uid, err := getUpdUID(gqlMutation)
		if err != nil {
			return nil, err
		}

		return rewriteAsGetQuery(gqlMutation.QueryField(), uid), nil

	default:
		return nil, gqlerror.Errorf("can't rewrite %s mutations to Dgraph query",
			gqlMutation.MutationType())
	}
}

func rewriteAsGetQuery(field schema.Field, uid uint64) *gql.GraphQuery {
	dgQuery := &gql.GraphQuery{
		Attr: field.ResponseName(),
		Func: &gql.Function{
			Name: "uid",
			UID:  []uint64{uid},
		},
	}

	addSelectionSetFrom(dgQuery, field)

	// TODO: also builder.withPagination() ... etc ...

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

		// TODO: filters, pagination, etc in here

		addSelectionSetFrom(child, f)

		q.Children = append(q.Children, child)
	}
}
