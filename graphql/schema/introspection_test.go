package schema

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/dgraph-io/dgraph/testutil"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
	"github.com/vektah/gqlparser/v2/validator"
)

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
		{"Deprecated directive on enum without deprecated",
			simpleSchema + deprecatedSchema,
			filepath.Join(iprefix, "enum_withoutdeprecated.txt"),
			filepath.Join(oprefix, "enum_withoutdeprecated.json"),
		},
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

func TestIntrospectionQueryMissingNameArg(t *testing.T) {
	sch := gqlparser.MustLoadSchema(
		&ast.Source{Name: "schema.graphql", Input: `
		schema {
			query: TestType
		}

		type TestType {
			testField: String
		}
	`})
	missingNameArgQuery := `
	{
    	__type {
	        name
    	}
	}`

	doc, gqlErr := parser.ParseQuery(&ast.Source{Input: missingNameArgQuery})
	require.Nil(t, gqlErr)

	listErr := validator.Validate(sch, doc)
	require.Equal(t, 1, len(listErr))
	require.Equal(t, "Field \"__type\" argument \"name\" of type \"String!\" is required but not provided.", listErr[0].Message)
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

const (
	testIntrospectionQuery = `query {
		__schema {
		  __typename
		  queryType {
			name
			__typename
		  }
		  mutationType {
			name
			__typename
		  }
		  subscriptionType {
			name
			__typename
		  }
		  types {
			...FullType
		  }
		  directives {
			__typename
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
		  __typename
		  name
		  args {
			...InputValue
			__typename
		  }
		  type {
			...TypeRef
			__typename
		  }
		  isDeprecated
		  deprecationReason
		}
		inputFields {
		  ...InputValue
		  __typename
		}
		interfaces {
		  ...TypeRef
		  __typename
		}
		enumValues(includeDeprecated: true) {
		  name
		  isDeprecated
		  deprecationReason
		  __typename
		}
		possibleTypes {
		  ...TypeRef
		  __typename
		}
		__typename
	  }
	  fragment InputValue on __InputValue {
		__typename
		name
		type {
		  ...TypeRef
		}
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
					  __typename
					}
					__typename
				  }
				  __typename
				}
				__typename
			  }
			  __typename
			}
			__typename
		  }
		  __typename
		}
		__typename
	  }`
)

func TestFullIntrospectionQuery(t *testing.T) {
	sch := gqlparser.MustLoadSchema(
		&ast.Source{Name: "schema.graphql", Input: `
	schema {
		query: TestType
	}

	type TestType {
		testField: String
	}
`})

	doc, gqlErr := parser.ParseQuery(&ast.Source{Input: testIntrospectionQuery})
	require.Nil(t, gqlErr)

	listErr := validator.Validate(sch, doc)
	require.Equal(t, 0, len(listErr))

	op := doc.Operations.ForName("")
	require.NotNil(t, op)
	oper := &operation{op: op,
		vars:     map[string]interface{}{},
		query:    string(testIntrospectionQuery),
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
