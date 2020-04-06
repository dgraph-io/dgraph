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

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/schema"
)

func NewFieldQueryProcedure(re RuleExtractor) *FieldQueryProcedure {
	fqp := FieldQueryProcedure{BaseProcedure: &BaseProcedure{}}
	fqp.SetRuleExtractor(re)
	return &fqp
}

type FieldQueryProcedure struct {
	*BaseProcedure

	currentTyp schema.Type
}

func (fqp *FieldQueryProcedure) OnQueryRoot(gqlQuery *gql.GraphQuery, typ schema.Type) {
	fqp.currentTyp = typ
}

func (fqp *FieldQueryProcedure) CreateQueryFromPath(path []*gql.GraphQuery,
	rule *schema.RuleNode) {

	filterPut := false
	var query *gql.GraphQuery
	name := path[0].Attr + "."
	for _, i := range path[1:] {
		toks := strings.Split(i.Attr, ".")
		name += toks[len(toks)-1]
	}

	for i := len(path) - 1; i >= 1; i-- {
		f := *path[i]
		child := &gql.GraphQuery{
			Attr: f.Attr,
		}

		if i == len(path)-1 {
			child.Var = fmt.Sprintf("%s", name)
		}

		if query != nil {
			child.Children = []*gql.GraphQuery{{
				Attr: "uid",
			}, query}

			if !filterPut {
				child.Filter = rule.GetFilter(fqp.authState)
				filterPut = true
			}
		}

		query = child
	}

	last := &gql.GraphQuery{
		Attr: fmt.Sprintf("%s", name),
		Func: &gql.Function{
			Name: "type",
			Args: []gql.Arg{{Value: fqp.currentTyp.Name()}},
		},
		Children: []*gql.GraphQuery{{
			Attr: "uid",
		}, query},
		Cascade: true,
	}

	if !filterPut {
		last.Filter = rule.GetFilter(fqp.authState)
	}

	path[len(path)-1].Attr = fmt.Sprintf("val(%s)", name)
	fqp.queries = append(fqp.queries, last)
	fqp.queries = append(fqp.queries, rule.GetQueries(fqp.authState)...)
}

func (fqp *FieldQueryProcedure) OnField(path []*gql.GraphQuery, typ schema.Type,
	field schema.FieldDefinition) {
	rules := fqp.getFieldRules(typ.Name(), field.Name())
	if rules == nil {
		return
	}

	rules.GetRBACRules(fqp.authState)
	if val, ok := fqp.authState.RbacRule[rules.RuleID]; ok && val != schema.Uncertain {
		return
	}

	fqp.CreateQueryFromPath(path, rules)
}
