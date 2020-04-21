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
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/schema"
)

type TypeNodeQueryProcedure struct {
	*BaseProcedure

	currentTyp schema.Type
}

func NewTypeNodeQueryProcedure(re RuleExtractor) *TypeNodeQueryProcedure {
	tnqp := TypeNodeQueryProcedure{BaseProcedure: &BaseProcedure{}}
	tnqp.SetRuleExtractor(re)
	return &tnqp
}

func (tnqp *TypeNodeQueryProcedure) applyRule(gqlQuery *gql.GraphQuery, rules *schema.RuleNode) {
	rules.GetRBACRules(tnqp.authState)
	if val, ok := tnqp.authState.RbacRule[rules.RuleID]; ok && val == schema.Negative {
		*gqlQuery = gql.GraphQuery{}
		return
	} else if val != schema.Uncertain {
		return
	}

	authFilter := GetFilter(rules, tnqp.authState)
	if gqlQuery.Filter != nil && authFilter != nil {
		gqlQuery.Filter = &gql.FilterTree{
			Op: "and",
			Child: []*gql.FilterTree{
				authFilter,
				gqlQuery.Filter,
			},
		}
	} else if authFilter != nil {
		gqlQuery.Filter = authFilter
	}

	tnqp.queries = append(tnqp.queries, GetQueries(rules, tnqp.authState)...)
}

func (tnqp *TypeNodeQueryProcedure) OnQueryRoot(gqlQuery *gql.GraphQuery, typ schema.Type) {
	tnqp.currentTyp = typ
	rules := tnqp.getTypeRules(typ.Name())
	if rules == nil {
		return
	}

	tnqp.applyRule(gqlQuery, rules)
}

func (tnqp *TypeNodeQueryProcedure) OnField(path []*gql.GraphQuery, typ schema.Type,
	field schema.FieldDefinition) {
	rules := tnqp.getTypeRules(field.Type().Name())
	if rules == nil {
		return
	}

	tnqp.applyRule(path[len(path)-1], rules)
}
