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
	"encoding/json"
	"fmt"
	"strings"

	dgoapi "github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/schema"
)

func newMutationWalker(sch *schema.Schema, queryProcedures []*QueryProcedure,
	mutationProcedures []*MutationProcedure) *MutationWalker {
	qw := newQueryWalker(sch, queryProcedures)
	mw := &MutationWalker{QueryWalker: qw}
	mw.setMutationProcedure(mutationProcedures)
	return mw
}

type MutationWalker struct {
	mutationProcedures []*MutationProcedure
	varToType          map[string]schema.Type

	*QueryWalker
}

func (mw *MutationWalker) setMutationProcedure(mutationP []*MutationProcedure) {
	mw.mutationProcedures = mutationP
	mw.varToType = make(map[string]schema.Type)
}

func (mw *MutationWalker) findType(mutation map[string]interface{}) schema.Type {
	uid, ok := mutation["uid"].(string)
	if ok {
		if strings.HasPrefix(uid, "_:") {
			uid = uid[2:]
		} else if strings.HasPrefix(uid, "uid(") {
			uid = uid[4 : len(uid)-1]
		}

		if val, ok := mw.varToType[uid]; ok {
			return val
		}
	}

	dgraphType, ok := mutation["dgraph.type"].([]interface{})
	if !ok {
		return nil
	}

	return mw.types[dgraphType[0].(string)]
}

func (mw *MutationWalker) fieldMutation(mutation interface{}, typ schema.Type, field schema.FieldDefinition) {
	for _, i := range mw.mutationProcedures {
		(*i).OnMutationField(mutation, typ, field)
	}
}

func (mw *MutationWalker) fieldMutationWalk(mutation map[string]interface{}, typ schema.Type) {
	if typ == nil {
		fmt.Println("Type failed", mutation)
		return
	}

	for _, i := range mw.mutationProcedures {
		(*i).OnMutation(mutation, typ)
	}

	for _, field := range typ.Fields() {
		name := typ.DgraphPredicate(field.Name())
		val, ok := mutation[name]
		if !ok {
			continue
		}

		fldType := field.Type()
		switch child := val.(type) {
		case []interface{}:
			for _, listValue := range child {
				listItem, ok := listValue.(map[string]interface{})
				if !ok {
					mw.fieldMutation(listItem, typ, field)
				} else {
					mw.fieldMutationWalk(listItem, fldType)
				}
			}
		case map[string]interface{}:
			mw.fieldMutationWalk(child, fldType)
		case interface{}:
			mw.fieldMutation(child, typ, field)
		}
	}
}

func (mw *MutationWalker) runMutation(mutation *dgoapi.Mutation) {
	var v map[string]interface{}
	json.Unmarshal(mutation.SetJson, &v)
	typ := mw.findType(v)

	for _, i := range mw.mutationProcedures {
		(*i).OnMutationRoot(mutation)
	}

	mw.fieldMutationWalk(v, typ)
}

func (mw *MutationWalker) readConditions(mutations []*dgoapi.Mutation) {
	conditions := make(map[string][]bool)

	for _, mutation := range mutations {
		cond := mutation.Cond
		if cond == "" {
			continue
		}

		// trim @if( and )
		cond = cond[4:]
		cond = cond[:len(cond)-1]

		// split by AND
		splits := strings.Split(cond, " AND ")
		for _, split := range splits {
			split = split[7:]
			splitAgain := strings.Split(split, ",")

			varName := splitAgain[0][:len(splitAgain[0])-1]
			varVal := splitAgain[1][1] - '0'

			if _, ok := conditions[varName]; !ok {
				conditions[varName] = []bool{false, false}
			}
			conditions[varName][varVal] = true
		}
	}

	for _, i := range mw.mutationProcedures {
		(*i).OnMutationCond(conditions)
	}
}

func (mw *MutationWalker) walkMutation(queries []*gql.GraphQuery, mutations []*dgoapi.Mutation) {
	mw.readConditions(mutations)

	for _, i := range queries {
		typ := mw.getTypeFromRoot(i)
		if i.Var != "" {
			mw.varToType[i.Var] = typ
		}
		mw.walkQuery(i)
	}

	for _, i := range mutations {
		mw.runMutation(i)
	}
}
