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
	for i, test := range tests {
		t.Run("should remain error "+string(i), func(t *testing.T) {

			qb := &QueryBuilder{err: fmt.Errorf("An Error")}

			require.True(t, qb.HasErrors())
			beforeErr := qb.err

			qb = test.builder(qb)

			require.True(t, qb.HasErrors())
			require.Equal(t, beforeErr, qb.err)
		})
	}

}

func TestQueryBuilder(t *testing.T) {
	tests := map[string]struct {
		builder  *QueryBuilder
		expected string
	}{
		"minimal": {buildMinimalTestQuery(),
			"query {\n  q(func: uid(0x7b))\n}"},
		"with alias": {buildMinimalTestQuery().WithAlias("p"),
			"query {\n  p : q(func: uid(0x7b))\n}"},
		"filter": {buildMinimalTestQuery().WithTypeFilter("T"),
			"query {\n  q(func: uid(0x7b)) @filter(type(T))\n}"},
		"with field": {buildMinimalTestQuery().WithField("f"),
			"query {\n  q(func: uid(0x7b)) {\n    f\n  }\n}"},
		"with two fields": {buildMinimalTestQuery().WithField("f").WithField("g"),
			"query {\n  q(func: uid(0x7b)) {\n    f\n    g\n  }\n}"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			qs, err := test.builder.AsQueryString()
			require.NoError(t, err)
			require.Equal(t, test.expected, qs)
		})
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
	typ: &fieldType{"T"},
	selSet: []schema.Field{
		&field{typ: &fieldType{"Str"}, name: "f", alias: "a"},
		&field{typ: &fieldType{"R"}, name: "g",
			selSet: []schema.Field{&field{typ: &fieldType{"Str"}, name: "e"}}}}}

type field struct {
	name   string
	alias  string
	typ    *fieldType
	selSet []schema.Field
}

type fieldType struct {
	name string
}

func (f *field) Name() string {
	return f.name
}

func (f *field) Alias() string {
	return f.alias
}

func (f *field) Type() schema.Type {
	return f.typ
}

func (ft *fieldType) Name() string {
	return ft.name
}

func (ft *fieldType) String() string {
	return ft.Name()
}

func (f *field) Location() *schema.Location {
	return &schema.Location{}
}

func (ft *fieldType) Nullable() bool {
	return true
}

func (ft *fieldType) ListType() schema.Type {
	return nil
}

func (f *field) SelectionSet() []schema.Field {
	return f.selSet
}

func (ft *fieldType) Field(name string) schema.FieldDefinition {
	return nil
}

func (ft *fieldType) IDField() schema.FieldDefinition {
	return nil
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
