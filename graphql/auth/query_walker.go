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

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/schema"
)

type QueryWalker struct {
	sch        *schema.Schema
	procedures []*QueryProcedure
	types      map[string]schema.Type
}

func (qw *QueryWalker) init(sch *schema.Schema, procedures []*QueryProcedure) {
	qw.sch = sch
	qw.procedures = procedures
	qw.types = (*sch).Types()
}

// fieldWalk walks over all the fields recursively,
// and provides onField queryProcedure call
func (qw *QueryWalker) fieldWalk(query *gql.GraphQuery, typ schema.Type, path []*gql.GraphQuery) {
	if query.Attr == "uid" {
		return
	}

	myTyp := typ
	myField := typ.Fields()[0]

	found := false

	for _, field := range typ.Fields() {
		if typ.DgraphPredicate(field.Name()) == query.Attr {
			myTyp = field.Type()
			myField = field
			found = true
			break
		}
	}

	if !found {
		fmt.Println("Error", query, typ, path)
		return
	}

	path = append(path, query)

	for _, i := range qw.procedures {
		(*i).OnField(path, typ, myField)
	}

	for _, child := range query.Children {
		qw.fieldWalk(child, myTyp, path)
	}
	path = path[:len(path)-1]
}

// checkFunction checks a graphql+- function tree, to see if
// the function is "type". If so, then it returns the typeName.
func checkFunction(fun *gql.Function) string {
	if fun == nil || fun.Name != "type" {
		return ""
	}

	return fun.Args[0].Value
}

// checkFunction checks a graphql+- filter tree, to see if
// the function is "type". If so, then it returns the typeName.
func checkFilter(filter *gql.FilterTree) string {
	if filter == nil {
		return ""
	}

	if v := checkFunction(filter.Func); v != "" {
		return v
	}

	for _, child := range filter.Child {
		if v := checkFilter(child); v != "" {
			return v
		}
	}
	return ""
}

// extracts type information from the root node of any query.
func (qw *QueryWalker) getTypeFromRoot(query *gql.GraphQuery) schema.Type {
	v := checkFunction(query.Func)
	if v == "" {
		v = checkFilter(query.Filter)
	}

	if v == "" {
		return nil
	}

	return qw.types[v]
}

// walk function goes to each node of the GraphQuery and calls the
// corresponding procedures, for the query to be modified.
func (qw *QueryWalker) walk(query *gql.GraphQuery) {
	typ := qw.getTypeFromRoot(query)

	if typ == nil {
		return
	}

	for _, i := range qw.procedures {
		(*i).OnQueryRoot(query, typ)
	}

	for _, child := range query.Children {
		qw.fieldWalk(child, typ, []*gql.GraphQuery{query})
	}
}
