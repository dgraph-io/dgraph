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
	"testing"

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/stretchr/testify/require"
)

func TestQueryBuilderWithErrorDoesNotBuild(t *testing.T) {
	qb := &QueryBuilder{err: fmt.Errorf("An Error")}

	q, err := qb.AsQueryString()
	require.Error(t, err, "An Error")
	require.Empty(t, q, "no query should be built if builder has error")
}

func TestQueryBuilderWithErrorKeepsError(t *testing.T) {
	// If it's in an error state, no build function gets out of that.

	tests := []struct {
		builder func(*QueryBuilder) *QueryBuilder
	}{
		{func(qb *QueryBuilder) *QueryBuilder { return qb.WithAttr("q") }},
		{func(qb *QueryBuilder) *QueryBuilder { return qb.WithAlias("p") }},
		{func(qb *QueryBuilder) *QueryBuilder { return qb.WithIDArgRoot(nil) }},
		{func(qb *QueryBuilder) *QueryBuilder { return qb.WithUIDRoot(123) }},
		{func(qb *QueryBuilder) *QueryBuilder { return qb.WithTypeFilter("T") }},
		{func(qb *QueryBuilder) *QueryBuilder { return qb.WithField("f") }},
		{func(qb *QueryBuilder) *QueryBuilder { return qb.WithSelectionSetFrom(testField) }},
	}
	for _, tt := range tests {
		qb := &QueryBuilder{err: fmt.Errorf("An Error")}

		require.True(t, qb.HasErrors())
		beforeErr := qb.err

		qb = tt.builder(qb)

		require.True(t, qb.HasErrors())
		require.Equal(t, beforeErr, qb.err)
	}

}

func TestQueryBuilder(t *testing.T) {
	tests := []struct {
		builder  *QueryBuilder
		expected string
	}{
		{buildMinimalTestQuery(),
			"query {\n  q(func: uid(0x7b))\n}"},
		{buildMinimalTestQuery().WithAlias("p"),
			"query {\n  p : q(func: uid(0x7b))\n}"},
		{buildMinimalTestQuery().WithTypeFilter("T"),
			"query {\n  q(func: uid(0x7b)) @filter(type(T))\n}"},
		{buildMinimalTestQuery().WithField("f"),
			"query {\n  q(func: uid(0x7b)) {\n    f\n  }\n}"},
		{buildMinimalTestQuery().WithField("f").WithField("g"),
			"query {\n  q(func: uid(0x7b)) {\n    f\n    g\n  }\n}"},
	}

	for _, tt := range tests {
		qs, err := tt.builder.AsQueryString()
		require.NoError(t, err)
		require.Equal(t, tt.expected, qs)
	}
}

func TestQueryBuilderWithSelectionSet(t *testing.T) {

	expected := `query {
  q(func: uid(0x7b)) @filter(type(T)) {
    a : T.f
    g : T.g {
      e : R.e
    }
  }
}`

	qb := buildMinimalTestQuery().
		WithTypeFilter("T").
		WithSelectionSetFrom(testField)

	qs, err := qb.AsQueryString()
	require.NoError(t, err)
	require.Equal(t, expected, qs)
}

func buildMinimalTestQuery() *QueryBuilder {
	return NewQueryBuilder().
		WithAttr("q").
		WithUIDRoot(123)
}

// Implement a schema.Field interface as required by QueryBuilder.WithSelectionSetFrom()

var testField = &field{
	typ: "T",
	selSet: []schema.Field{
		&field{name: "f", alias: "a"},
		&field{typ: "R", name: "g", selSet: []schema.Field{&field{name: "e"}}}}}

type field struct {
	name   string
	alias  string
	typ    string
	selSet []schema.Field
}

func (f *field) Name() string {
	return f.name
}

func (f *field) Alias() string {
	return f.alias
}

func (f *field) TypeName() string {
	return f.typ
}

func (f *field) SelectionSet() []schema.Field {
	return f.selSet
}

func (f *field) ResponseName() string {
	return ""
}

func (f *field) ArgValue(s string) interface{} {
	return nil
}

func (f *field) IDArgValue() (uint64, error) {
	return 0, nil
}
