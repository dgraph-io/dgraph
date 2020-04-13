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
	"fmt"
	"strings"

	dgoapi "github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/schema"
)

func NewMutationQueryProcedure() *MutationQueryTypeProcedure {
	mqtp := MutationQueryTypeProcedure{BaseMutationProcedure: &BaseMutationProcedure{
		BaseProcedure: &BaseProcedure{}},
		toChange: make(map[string]string),
		queryType: []*TypeNodeQueryProcedure{nil,
			NewTypeNodeQueryProcedure(UpdateRuleExtractor)},
	}

	return &mqtp
}

type MutationQueryTypeProcedure struct {
	*BaseMutationProcedure
	queryType []*TypeNodeQueryProcedure

	toChange map[string]string
}

func (mqtp *MutationQueryTypeProcedure) Init(sch *schema.Schema, authState *schema.AuthState) {
	mqtp.BaseProcedure.Init(sch, authState)
	for _, i := range mqtp.queryType {
		if i == nil {
			continue
		}
		i.Init(sch, authState)
	}
}

func (mqtp *MutationQueryTypeProcedure) CollectQueries() []*gql.GraphQuery {
	result := mqtp.BaseProcedure.CollectQueries()
	for _, i := range mqtp.queryType {
		if i == nil {
			continue
		}
		result = append(result, i.CollectQueries()...)
	}
	return result
}

func (mqtp *MutationQueryTypeProcedure) OnMutationRoot(mutation *dgoapi.Mutation) {
	for toChange, result := range mqtp.toChange {
		mutation.Cond = strings.ReplaceAll(mutation.Cond, toChange, result)
	}
}

func (mqtp *MutationQueryTypeProcedure) OnMutationField(mutation interface{}, typ schema.Type,
	fld schema.FieldDefinition) {
}

func (mqtp *MutationQueryTypeProcedure) OnMutation(mutation map[string]interface{},
	typ schema.Type) {
}

func (mqtp *MutationQueryTypeProcedure) OnQueryRoot(gqlQuery *gql.GraphQuery, typ schema.Type) {
	origFilter := gqlQuery.Filter
	conditions, ok := mqtp.conditions[gqlQuery.Var]
	if !ok {
		return
	}

	alreadyUpdated := false
	for i, queryTyp := range mqtp.queryType {
		if !conditions[i] {
			continue
		}

		query := gqlQuery
		if alreadyUpdated {
			query = &gql.GraphQuery{
				Var:      fmt.Sprintf("%s_%d", gqlQuery.Var, i),
				Func:     gqlQuery.Func,
				Filter:   origFilter,
				Children: gqlQuery.Children,
			}
			mqtp.queries = append(mqtp.queries, query)
			query.Attr = query.Var

			toChange := fmt.Sprintf("eq(len(%s), %d)", gqlQuery.Var, i)
			result := fmt.Sprintf("eq(len(%s), %d)", query.Var, i)

			mqtp.toChange[toChange] = result
		}

		if queryTyp != nil {
			queryTyp.OnQueryRoot(query, typ)
		}
		alreadyUpdated = true
	}
}

func (amp *MutationQueryTypeProcedure) OnMutationResult(mutation schema.Mutation,
	assigned map[string]string, result map[string]interface{}) {
}

func (mqtp *MutationQueryTypeProcedure) OnField(path []*gql.GraphQuery, typ schema.Type,
	field schema.FieldDefinition) {
}
