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

package auth

import (
	dgoapi "github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/schema"
)

// Extracts Query/Add/Update/Delete out of AuthContainer.
type RuleExtractor func(*schema.AuthContainer) *schema.RuleNode

type ProcedureBase interface {
	Init(sch *schema.Schema, authState *schema.AuthState)
	GetTypeRule(ac *schema.AuthContainer) *schema.RuleNode
	CollectQueries() []*gql.GraphQuery
}

// Changes that needs to be made to a query. Would require one for
// queries(also delete), update, handling field auth and interfaces.
type QueryProcedure interface {
	OnQueryRoot(gqlQuery *gql.GraphQuery, typ schema.Type)
	OnField(path []*gql.GraphQuery, typ schema.Type, field schema.FieldDefinition)

	ProcedureBase
}

// Changes that needs to be made to a mutation fragment. Would require one for
// add, update, handling field auth and interfaces.
type MutationProcedure interface {
	OnMutationField(mutation interface{}, typ schema.Type, fld schema.FieldDefinition)
	OnMutation(mutation map[string]interface{}, typ schema.Type)
	OnMutationRoot(mutation *dgoapi.Mutation)
	OnMutationCond(conditions map[string][]bool)

	OnMutationResult(mutation schema.Mutation, assigned map[string]string,
		result map[string]interface{})

	CollectMutations() []*dgoapi.Mutation
	ProcedureBase
}

// Each rewriter would initialize this and add the corresponding
// implementations of query and mutation procedures. Then it would
// call OnQuery() or OnMutation() as per requirement.
type AuthResolver struct {
	// schema and schema.AuthState
	sch       *schema.Schema
	authState *schema.AuthState

	queryProcedures    []*QueryProcedure
	mutationProcedures []*MutationProcedure
}

func (a *AuthResolver) Init(sch *schema.Schema, authState *schema.AuthState) {
	//Store schema and schema.Authorrizer here
	a.sch = sch
	a.authState = authState

	// Init all queryProcedures
	for _, procedure := range a.queryProcedures {
		(*procedure).Init(a.sch, a.authState)
	}

	// Init all mutationProcedures
	for _, procedure := range a.mutationProcedures {
		(*procedure).Init(a.sch, a.authState)
	}
}

func (a *AuthResolver) OnQuery(query *gql.GraphQuery) []*gql.GraphQuery {
	// Create a queryWalker, pass it all the queryProcedures
	qw := newQueryWalker(a.sch, a.queryProcedures)
	qw.walkQuery(query)

	// Collect all queries from all the queryProcedures
	queries := make([]*gql.GraphQuery, 1)
	queries[0] = query
	visited := make(map[string]struct{})
	visited[getName(query)] = struct{}{}

	for _, procedure := range a.queryProcedures {
		for _, q := range (*procedure).CollectQueries() {
			if _, ok := visited[getName(q)]; ok {
				continue
			}
			visited[getName(q)] = struct{}{}
			queries = append(queries, q)
		}
	}

	return queries
}

func getName(query *gql.GraphQuery) string {
	if query.Var != "" {
		return query.Var
	}

	if query.Alias != "" {
		return query.Alias
	}

	return query.Attr
}

func (a *AuthResolver) OnMutationResult(mutation schema.Mutation, assigned map[string]string,
	result map[string]interface{}) []*gql.GraphQuery {

	for _, procedure := range a.mutationProcedures {
		(*procedure).OnMutationResult(mutation, assigned, result)
	}

	visited := make(map[string]struct{})
	var queries []*gql.GraphQuery

	// Collect all queries from all the mutationProcedures
	for _, procedure := range a.mutationProcedures {
		for _, q := range (*procedure).CollectQueries() {
			if _, ok := visited[getName(q)]; ok {
				continue
			}
			visited[getName(q)] = struct{}{}
			queries = append(queries, q)
		}
	}

	return queries
}

func (a *AuthResolver) OnMutation(query *gql.GraphQuery, mutation []*dgoapi.Mutation) {
	// Create a mutationWalker, pass it all the mutationProcedures
	mw := newMutationWalker(a.sch, a.queryProcedures, a.mutationProcedures)
	var queries []*gql.GraphQuery
	if query != nil {
		queries = append(queries, query.Children...)
		if query.Attr != "" {
			queries = []*gql.GraphQuery{query}
		}
	}

	mw.walkMutation(queries, mutation)

	// Collect all queries from all the queryProcedures
	visited := make(map[string]struct{})
	for _, query := range queries {
		visited[getName(query)] = struct{}{}
	}

	for _, procedure := range a.queryProcedures {
		for _, q := range (*procedure).CollectQueries() {
			if _, ok := visited[getName(q)]; ok {
				continue
			}
			visited[getName(q)] = struct{}{}
			queries = append(queries, q)
		}
	}

	// Collect all queries from all the mutationProcedures
	for _, procedure := range a.mutationProcedures {
		for _, q := range (*procedure).CollectQueries() {
			if _, ok := visited[getName(q)]; ok {
				continue
			}
			visited[getName(q)] = struct{}{}
			queries = append(queries, q)
		}
	}

	if query != nil && query.Attr == "" {
		query.Children = queries
	}

	// Collect all mutations from all the mutationProcedures
}

func (a *AuthResolver) AddQueryProcedure(q QueryProcedure) {
	a.queryProcedures = append(a.queryProcedures, &q)
}

func (a *AuthResolver) AddMutaionProcedure(m MutationProcedure) {
	a.mutationProcedures = append(a.mutationProcedures, &m)
}
