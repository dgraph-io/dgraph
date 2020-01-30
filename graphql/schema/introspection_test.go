package schema

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/dgraph-io/dgraph/testutil"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser"
	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/parser"
	"github.com/vektah/gqlparser/validator"
)

const introspectionQuery = `
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

const complexSchema = `schema {
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
  `

func TestIntrospectionQuery(t *testing.T) {
	simpleSchema := `schema {
		query: QueryRoot
	}

	type QueryRoot {
		onlyField: String
	}`

	deprecatedSchema := `
	type TestDeprecatedObject {
		dep: String @deprecated
		depReason: String @deprecated(reason: "because")
		notDep: String
	}

	enum TestDeprecatedEnum {
		dep @deprecated
		depReason @deprecated(reason: "because")
		notDep
	}
	`

	iprefix := "testdata/introspection/input"
	oprefix := "testdata/introspection/output"

	var tests = []struct {
		name       string
		schema     string
		queryFile  string
		outputFile string
	}{
		{
			"Filter on __type",
			simpleSchema,
			filepath.Join(iprefix, "type_filter.txt"),
			filepath.Join(oprefix, "type_filter.json"),
		},
		{"Filter __Schema on __type",
			simpleSchema,
			filepath.Join(iprefix, "type_schema_filter.txt"),
			filepath.Join(oprefix, "type_schema_filter.json"),
		},
		{"Filter object type __type",
			simpleSchema,
			filepath.Join(iprefix, "type_object_name_filter.txt"),
			filepath.Join(oprefix, "type_object_name_filter.json"),
		},
		{"Filter complex object type __type",
			complexSchema,
			filepath.Join(iprefix, "type_complex_object_name_filter.txt"),
			filepath.Join(oprefix, "type_complex_object_name_filter.json"),
		},
		{"Deprecated directive on type with deprecated",
			simpleSchema + deprecatedSchema,
			filepath.Join(iprefix, "type_withdeprecated.txt"),
			filepath.Join(oprefix, "type_withdeprecated.json"),
		},
		{"Deprecated directive on type without deprecated",
			simpleSchema + deprecatedSchema,
			filepath.Join(iprefix, "type_withoutdeprecated.txt"),
			filepath.Join(oprefix, "type_withoutdeprecated.json"),
		},
		{"Deprecated directive on enum with deprecated",
			simpleSchema + deprecatedSchema,
			filepath.Join(iprefix, "enum_withdeprecated.txt"),
			filepath.Join(oprefix, "enum_withdeprecated.json"),
		},
		// TODO: There's a bug in the gqlparser lib that needs fixing for enums
		// Fields(includeDeprecated bool) for a type respects the includeDeprecated arg:
		// https://github.com/99designs/gqlgen/blob/
		//    f7a67722a6baf2612fa429bd21ceb9c6b9cbed1c/graphql/introspection/type.go#L73-L75
		// but EnumValues does not
		// https://github.com/99designs/gqlgen/blob/
		//    f7a67722a6baf2612fa429bd21ceb9c6b9cbed1c/graphql/introspection/type.go#L148
		// {"Deprecated directive on enum without deprecated",
		// 	simpleSchema + deprecatedSchema,
		// 	filepath.Join(iprefix, "enum_withoutdeprecated.txt"),
		// 	filepath.Join(oprefix, "enum_withoutdeprecated.json"),
		// },
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sch := gqlparser.MustLoadSchema(
				&ast.Source{Name: "schema.graphql", Input: tt.schema})

			q, err := ioutil.ReadFile(tt.queryFile)
			require.NoError(t, err)

			doc, gqlErr := parser.ParseQuery(&ast.Source{Input: string(q)})
			require.Nil(t, gqlErr)
			listErr := validator.Validate(sch, doc)
			require.Equal(t, 0, len(listErr))

			op := doc.Operations.ForName("")
			oper := &operation{op: op,
				vars:     map[string]interface{}{},
				query:    string(q),
				doc:      doc,
				inSchema: &schema{schema: sch},
			}
			require.NotNil(t, op)

			queries := oper.Queries()
			resp, err := Introspect(queries[0])
			require.NoError(t, err)

			expectedBuf, err := ioutil.ReadFile(tt.outputFile)
			require.NoError(t, err)
			testutil.CompareJSON(t, string(expectedBuf), string(resp))
		})
	}
}

func TestIntrospectionQueryWithVars(t *testing.T) {
	sch := gqlparser.MustLoadSchema(
		&ast.Source{Name: "schema.graphql", Input: complexSchema})

	q := `query filterNameOnType($name: String!) {
			__type(name: $name) {
				kind
				name
				inputFields {
					name
					type { ...TypeRef }
					defaultValue
				}
			}
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
					}
				}
			}
		}`

	doc, gqlErr := parser.ParseQuery(&ast.Source{Input: q})
	require.Nil(t, gqlErr)
	listErr := validator.Validate(sch, doc)
	require.Equal(t, 0, len(listErr))

	op := doc.Operations.ForName("")
	oper := &operation{op: op,
		vars:     map[string]interface{}{"name": "TestInputObject"},
		query:    q,
		doc:      doc,
		inSchema: &schema{schema: sch},
	}
	require.NotNil(t, op)

	queries := oper.Queries()
	resp, err := Introspect(queries[0])
	require.NoError(t, err)

	fname := "testdata/introspection/output/type_complex_object_name_filter.json"
	expectedBuf, err := ioutil.ReadFile(fname)
	require.NoError(t, err)
	testutil.CompareJSON(t, string(expectedBuf), string(resp))
}

func TestFullIntrospectionQuery(t *testing.T) {
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

	op := doc.Operations.ForName("")
	require.NotNil(t, op)
	oper := &operation{op: op,
		vars:     map[string]interface{}{},
		query:    string(introspectionQuery),
		doc:      doc,
		inSchema: &schema{schema: sch},
	}

	queries := oper.Queries()
	resp, err := Introspect(queries[0])
	require.NoError(t, err)

	expectedBuf, err := ioutil.ReadFile("testdata/introspection/output/full_query.json")
	require.NoError(t, err)
	testutil.CompareJSON(t, string(expectedBuf), string(resp))
}
