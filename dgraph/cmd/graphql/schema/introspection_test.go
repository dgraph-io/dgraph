package schema

import (
	"context"
	"io/ioutil"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser"
	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/parser"
	"github.com/vektah/gqlparser/validator"
)

var introspectionQuery = `
  query {
    __schema {
      queryType { name }
      mutationType { name }
      subscriptionType { name }
      types {
        ...FullType
      }
      directives {
        name
        locations
        args {
          ...InputValue
        }
      }
    }
  }
  fragment FullType on __Type {
    kind
    name
    fields(includeDeprecated: true) {
      name
      args {
        ...InputValue
      }
      type {
        ...TypeRef
      }
      isDeprecated
      deprecationReason
    }
    inputFields {
      ...InputValue
    }
    interfaces {
      ...TypeRef
    }
    enumValues(includeDeprecated: true) {
      name
      isDeprecated
      deprecationReason
    }
    possibleTypes {
      ...TypeRef
    }
  }
  fragment InputValue on __InputValue {
    name
    type { ...TypeRef }
    defaultValue
  }
  fragment TypeRef on __Type {
    kind
    name
    ofType {
      kind
      name
      ofType {
        kind
        name
        ofType {
          kind
          name
          ofType {
            kind
            name
            ofType {
              kind
              name
              ofType {
                kind
                name
                ofType {
                  kind
                  name
                }
              }
            }
          }
        }
      }
    }
  }
`

func TestIntrospectionQuery(t *testing.T) {
	simpleSchema := `schema {
		query: QueryRoot
	}

	type QueryRoot {
		onlyField: String
	}`

	var tests = []struct {
		name       string
		schema     string
		queryFile  string
		outputFile string
	}{
		{
			"Filter on __type",
			simpleSchema,
			"testdata/input/introspection_type_filter.txt",
			"testdata/output/introspection_type_filter.json",
		},
		{"Filter __Schema on __type",
			simpleSchema,
			"testdata/input/introspection_type_schema_filter.txt",
			"testdata/output/introspection_type_schema_filter.json",
		},
		{"Filter object type __type",
			simpleSchema,
			"testdata/input/introspection_type_object_name_filter.txt",
			"testdata/output/introspection_type_object_name_filter.json",
		},
		{"Filter complex object type __type",
			`
    schema {
      query: TestType
    }

    input TestInputObject {
      a: String = test
      b: [String]
      c: String = null
    }


    input TestType {
      complex: TestInputObject
    }
    `,
			"testdata/input/introspection_type_complex_object_name_filter.txt",
			"testdata/output/introspection_type_complex_object_name_filter.json",
		},
	}

	for _, tt := range tests {
		sch := gqlparser.MustLoadSchema(
			&ast.Source{Name: "schema.graphql", Input: tt.schema})

		q, err := ioutil.ReadFile(tt.queryFile)
		require.NoError(t, err)

		doc, gqlErr := parser.ParseQuery(&ast.Source{Input: string(q)})
		require.Nil(t, gqlErr)
		listErr := validator.Validate(sch, doc)
		require.Equal(t, 0, len(listErr))

		op := doc.Operations.ForName(doc.Operations[0].Name)
		oper := &operation{op: op,
			vars:  map[string]interface{}{},
			query: string(q),
			doc:   doc,
		}
		require.NotNil(t, op)
		reqCtx := graphql.NewRequestContext(doc, string(q), map[string]interface{}{})
		ctx := graphql.WithRequestContext(context.Background(), reqCtx)

		resp, err := Introspect(ctx, oper, AsSchema(sch))
		require.NoError(t, err)

		expectedBuf, err := ioutil.ReadFile(tt.outputFile)
		require.NoError(t, err)
		testutil.CompareJSON(t, string(expectedBuf), string(resp))
	}
}

// TODO - Add some tests to check for reading name from graphql variables.

func TestIntrospectioNQuery_full(t *testing.T) {
	// The output doesn't quite match the output in the graphql-js repo. Look into this later.
	// https://github.com/graphql/graphql-js/blob/master/src/type/__tests__/introspection-test.js#L35
	sch := gqlparser.MustLoadSchema(
		&ast.Source{Name: "schema.graphql", Input: `
	schema {
		query: TestType
	}

	type TestType {
		testField: String
	}
`})

	doc, gqlErr := parser.ParseQuery(&ast.Source{Input: introspectionQuery})
	require.Nil(t, gqlErr)

	listErr := validator.Validate(sch, doc)
	require.Equal(t, 0, len(listErr))

	op := doc.Operations.ForName(doc.Operations[0].Name)
	require.NotNil(t, op)
	oper := &operation{op: op,
		vars:  map[string]interface{}{},
		query: string(introspectionQuery),
		doc:   doc,
	}

	reqCtx := graphql.NewRequestContext(doc, introspectionQuery, map[string]interface{}{})
	ctx := graphql.WithRequestContext(context.Background(), reqCtx)

	resp, err := Introspect(ctx, oper, AsSchema(sch))
	require.NoError(t, err)

	expectedBuf, err := ioutil.ReadFile("testdata/output/introspection_full_query.json")
	require.NoError(t, err)
	testutil.CompareJSON(t, string(expectedBuf), string(resp))
}
