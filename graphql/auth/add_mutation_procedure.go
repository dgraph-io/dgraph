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
	"strconv"
	"strings"

	dgoapi "github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/schema"
)

func NewPostMutationAddProcedure() *PostMutationAddProcedure {
	fqp := PostMutationAddProcedure{BaseMutationProcedure: &BaseMutationProcedure{
		BaseProcedure: &BaseProcedure{}},
		blankNodes: make(map[string]schema.Type),
	}
	fqp.SetRuleExtractor(AddRuleExtractor)
	return &fqp
}

type PostMutationAddProcedure struct {
	*BaseMutationProcedure

	blankNodes map[string]schema.Type
	currentTyp schema.Type
}

func (amp *PostMutationAddProcedure) OnMutationRoot(mutation *dgoapi.Mutation) {
}

func (amp *PostMutationAddProcedure) OnMutationField(mutation interface{}, typ schema.Type,
	fld schema.FieldDefinition) {
}

func (amp *PostMutationAddProcedure) OnMutation(mutation map[string]interface{}, typ schema.Type) {
	uid, ok := mutation["uid"].(string)
	if !ok || (!strings.HasPrefix(uid, "_:") && len(mutation) > 1) {
		return
	}

	if amp.getTypeRules(typ.Name()) != nil {
		amp.blankNodes[strings.TrimPrefix(uid, "_:")] = typ
	}
}

func createQuery(typ schema.Type, name string, uid uint64,
	authFitlers *gql.FilterTree) *gql.GraphQuery {

	filter := &gql.FilterTree{
		Func: &gql.Function{
			Name: "type",
			Args: []gql.Arg{{Value: typ.Name()}},
		},
	}

	if authFitlers != nil {
		filter = &gql.FilterTree{
			Op: "and",
			Child: []*gql.FilterTree{
				authFitlers,
				filter,
			},
		}
	}

	return &gql.GraphQuery{
		Attr: name,
		Func: &gql.Function{
			Name: "uid",
			Args: []gql.Arg{{Value: strconv.FormatUint(uid, 10)}},
		},
		Filter: filter,
		Children: []*gql.GraphQuery{{
			Attr: "uid",
		}},
	}
}

func (amp *PostMutationAddProcedure) OnMutationResult(mutation schema.Mutation,
	assigned map[string]string, result map[string]interface{}) {

	amp.queries = make([]*gql.GraphQuery, 0)

	fmt.Println("assigned", assigned)
	fmt.Println("result", result)

	for node, uid := range assigned {
		typ := amp.blankNodes[node]
		if typ == nil {
			continue
		}
		uidInt, err := strconv.ParseUint(uid, 0, 64)
		if err != nil {
			continue
		}
		rules := amp.getTypeRules(typ.Name())
		if rules == nil {
			continue
		}
		amp.queries = append(amp.queries, createQuery(typ, node, uidInt,
			rules.GetFilter(amp.authState)))
		amp.queries = append(amp.queries, rules.GetQueries(amp.authState)...)
	}
}
