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
	"errors"
	"fmt"
	"strings"

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/dgraph-io/dgraph/gql"
)

// QueryBuilder is an implementation of the builder pattern that can construct
// a gql.GraphQuery.
type QueryBuilder struct {
	graphQuery *gql.GraphQuery
	err        error
}

// NewQueryBuilder returns a fresh QueryBuilder
func NewQueryBuilder() *QueryBuilder {
	return &QueryBuilder{
		graphQuery: &gql.GraphQuery{},
	}
}

// HasErrors returns true if qb contains errors and false otherwise.
func (qb *QueryBuilder) HasErrors() bool {
	return qb.err != nil
}

// WithAttr sets the root query name.
func (qb *QueryBuilder) WithAttr(attr string) *QueryBuilder {
	if qb.HasErrors() {
		return qb
	}

	qb.graphQuery.Attr = attr
	return qb
}

// WithAlias sets the root query alias
func (qb *QueryBuilder) WithAlias(alias string) *QueryBuilder {
	if qb.HasErrors() {
		return qb
	}

	qb.graphQuery.Alias = alias
	return qb
}

// WithIDArgRoot sets the query root func to a query for the ID argument in q.
// qb will contain an error if there's no ID argument in q.
func (qb *QueryBuilder) WithIDArgRoot(q schema.Query) *QueryBuilder {
	if qb.HasErrors() {
		return qb
	}

	uid, err := q.IDArgValue()
	if err != nil {
		qb.err = err
		return qb
	}

	return qb.WithUIDRoot(uid)
}

// WithUIDRoot sets the root func as a uid() function for uid.
func (qb *QueryBuilder) WithUIDRoot(uid uint64) *QueryBuilder {
	if qb.HasErrors() {
		return qb
	}

	qb.graphQuery.Func = &gql.Function{
		Name: "uid",
		UID:  []uint64{uid},
	}

	return qb
}

// WithTypeFilter sets the root filter as requiring a type named as per typ.
func (qb *QueryBuilder) WithTypeFilter(typ string) *QueryBuilder {
	if qb.HasErrors() {
		return qb
	}

	qb.graphQuery.Filter = &gql.FilterTree{
		Func: &gql.Function{
			Name: "type",
			Args: []gql.Arg{{Value: typ}},
		},
	}

	return qb
}

// WithField adds a child query for the given field name.
func (qb *QueryBuilder) WithField(field string) *QueryBuilder {
	if qb.HasErrors() {
		return qb
	}

	qb.graphQuery.Children = append(qb.graphQuery.Children,
		&gql.GraphQuery{
			Attr: field,
		})

	return qb
}

// WithSelectionSetFrom adds child queries for all fields in the selection set
// of fld, and recursively so for all fields in the selection set of those fields.
func (qb *QueryBuilder) WithSelectionSetFrom(fld schema.Field) *QueryBuilder {
	if qb.HasErrors() {
		return qb
	}

	for _, f := range fld.SelectionSet() {
		qbld := NewQueryBuilder()
		if f.Alias() != "" {
			qbld.WithAlias(f.Alias())
		} else {
			qbld.WithAlias(f.Name())
		}

		if f.TypeName() == schema.IDType {
			qbld.WithAttr("uid")
		} else {
			qbld.WithAttr(fld.TypeName() + "." + f.Name())
		}

		// TODO: filters, pagination, etc in here
		qbld.WithSelectionSetFrom(f)

		q, err := qbld.query()
		if err != nil {
			qb.err = err
			return qb
		}

		qb.graphQuery.Children = append(qb.graphQuery.Children, q)
	}

	return qb
}

// query builds QueryBuilder's query.  If qb.hasErrors(), then the returned
// error is non-nil.
func (qb *QueryBuilder) query() (*gql.GraphQuery, error) {
	if qb == nil || qb.graphQuery == nil {
		return nil, errors.New("nil query builder")
	}

	return qb.graphQuery, qb.err
}

// AsQueryString renders a QueryBuilder as the string that a user would recognise as
// a Dgraph query.
func (qb *QueryBuilder) AsQueryString() (string, error) {
	gq, err := qb.query()
	if err != nil {
		return "", err
	}

	return asString(gq), nil
}

func asString(query *gql.GraphQuery) string {
	if query == nil {
		return ""
	}

	var b strings.Builder

	b.WriteString("query {\n")
	writeQuery(&b, query, "  ")
	b.WriteString("}")

	return b.String()
}

func writeQuery(b *strings.Builder, query *gql.GraphQuery, prefix string) {
	b.WriteString(prefix)
	if query.Alias != "" {
		b.WriteString(query.Alias)
		b.WriteString(" : ")
	}
	b.WriteString(query.Attr)

	writeFunction(b, query.Func)
	writeFilter(b, query.Filter)

	if len(query.Children) > 0 {
		b.WriteString(" {\n")
		for _, c := range query.Children {
			writeQuery(b, c, prefix+"  ")
		}
		b.WriteString(prefix)
		b.WriteString("}")
	}
	b.WriteString("\n")
}

func writeFunction(b *strings.Builder, f *gql.Function) {
	if f != nil {
		b.WriteString(fmt.Sprintf("(func: %s(0x%x))", f.Name, f.UID[0]))
		// there's only uid(...) functions so far
	}
}

func writeFilterFunction(b *strings.Builder, f *gql.Function) {
	if f != nil {
		b.WriteString(fmt.Sprintf("%s(%s)", f.Name, f.Args[0].Value))
	}
}

func writeFilter(b *strings.Builder, f *gql.FilterTree) {
	if f == nil {
		return
	}

	if f.Func.Name == "type" {
		b.WriteString(fmt.Sprintf(" @filter(type(%s))", f.Func.Args[0].Value))
	} else {
		b.WriteString(" @filter(")
		writeFilterFunction(b, f.Func)
		b.WriteString(")")
	}
}
