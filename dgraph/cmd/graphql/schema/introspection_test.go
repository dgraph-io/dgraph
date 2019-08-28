package schema

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/99designs/gqlgen/graphql"
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

func TestIntrospectionQuery_Typekind(t *testing.T) {
	sch := gqlparser.MustLoadSchema(
		&ast.Source{Name: "schema.graphql", Input: `
	schema {
		query: QueryRoot
	}

	type QueryRoot {
		onlyField: String
	}
`})

	query := `{
        typeKindType: __type(name: "__TypeKind") {
          name,
          enumValues {
            name,
          }
        }
      }`
	doc, gqlErr := parser.ParseQuery(&ast.Source{Input: query})
	require.Nil(t, gqlErr)

	listErr := validator.Validate(sch, doc)
	require.Equal(t, 0, len(listErr))

	op := doc.Operations.ForName(doc.Operations[0].Name)
	require.NotNil(t, op)

	reqCtx := graphql.NewRequestContext(doc, query, map[string]interface{}{})
	ctx := graphql.WithRequestContext(context.Background(), reqCtx)

	resp := IntrospectionQuery(ctx, op, sch)
	b, err := json.Marshal(resp)
	require.NoError(t, err)

	expected := `{"data":{"typeKindType":{"name":"__TypeKind","enumValues":[{"name":"SCALAR"},{"name":"OBJECT"},{"name":"INTERFACE"},{"name":"UNION"},{"name":"ENUM"},{"name":"INPUT_OBJECT"},{"name":"LIST"},{"name":"NON_NULL"}]}}}`
	require.Equal(t, expected, string(b))
}

func TestIntrospectionQuery_TypeSchema(t *testing.T) {
	sch := gqlparser.MustLoadSchema(
		&ast.Source{Name: "schema.graphql", Input: `
	schema {
		query: QueryRoot
	}

	type QueryRoot {
		onlyField: String
	}
`})

	query := `
      {
        schemaType: __type(name: "__Schema") {
          name,
          fields {
            name,
          }
        }
      }`
	doc, gqlErr := parser.ParseQuery(&ast.Source{Input: query})
	require.Nil(t, gqlErr)

	listErr := validator.Validate(sch, doc)
	require.Equal(t, 0, len(listErr))

	op := doc.Operations.ForName(doc.Operations[0].Name)
	require.NotNil(t, op)

	reqCtx := graphql.NewRequestContext(doc, query, map[string]interface{}{})
	ctx := graphql.WithRequestContext(context.Background(), reqCtx)

	resp := IntrospectionQuery(ctx, op, sch)
	b, err := json.Marshal(resp)
	require.NoError(t, err)

	expected := `{"data":{"schemaType":{"name":"__Schema","fields":[{"name":"types"},{"name":"queryType"},{"name":"mutationType"},{"name":"subscriptionType"},{"name":"directives"}]}}}`
	require.Equal(t, expected, string(b))
}

func TestIntrospectionQuery_TypeAtRoot(t *testing.T) {
	sch := gqlparser.MustLoadSchema(
		&ast.Source{Name: "schema.graphql", Input: `
	schema {
		query: TestType
	}

	type TestType {
		testField: String
	}
`})

	query := `
      {
        __type(name: "TestType") {
          name
        }
      }`
	doc, gqlErr := parser.ParseQuery(&ast.Source{Input: query})
	require.Nil(t, gqlErr)

	listErr := validator.Validate(sch, doc)
	require.Equal(t, 0, len(listErr))

	op := doc.Operations.ForName(doc.Operations[0].Name)
	require.NotNil(t, op)

	reqCtx := graphql.NewRequestContext(doc, query, map[string]interface{}{})
	ctx := graphql.WithRequestContext(context.Background(), reqCtx)

	resp := IntrospectionQuery(ctx, op, sch)
	b, err := json.Marshal(resp)
	require.NoError(t, err)

	expected := `{"data":{"__type":{"name":"TestType"}}}`
	require.Equal(t, expected, string(b))

}

func TestIntrospectionQuery_InputObject(t *testing.T) {
	sch := gqlparser.MustLoadSchema(
		&ast.Source{Name: "schema.graphql", Input: `
	schema {
		query: TestType
	}

	input TestInputObject {
		a: String = "tes\t de\fault"
		b: [String]
		c: String = null
	}


	input TestType {
		complex: TestInputObject
	}
	`})

	query := `     {
        __type(name: "TestInputObject") {
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

	doc, gqlErr := parser.ParseQuery(&ast.Source{Input: query})
	require.Nil(t, gqlErr)

	listErr := validator.Validate(sch, doc)
	require.Equal(t, 0, len(listErr))

	op := doc.Operations.ForName(doc.Operations[0].Name)
	require.NotNil(t, op)

	reqCtx := graphql.NewRequestContext(doc, query, map[string]interface{}{})
	ctx := graphql.WithRequestContext(context.Background(), reqCtx)

	resp := IntrospectionQuery(ctx, op, sch)
	b, err := json.Marshal(resp)
	require.NoError(t, err)

	expected := `{"data":{"__type":{"kind":"INPUT_OBJECT","name":"TestInputObject","inputFields":[{"name":"a","type":{"kind":"SCALAR","name":"String","ofType":null},"defaultValue":"\"tes\\t de\\fault\""},{"name":"b","type":{"kind":"LIST","name":null,"ofType":{"kind":"SCALAR","name":"String","ofType":null}},"defaultValue":null},{"name":"c","type":{"kind":"SCALAR","name":"String","ofType":null},"defaultValue":"null"}]}}}`
	require.Equal(t, expected, string(b))

}
