package schema_test

import (
	"io/ioutil"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/parser"
	"github.com/vektah/gqlparser/validator"

	_ "github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema/schemarules"

	. "github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
)

func TestSchemaString(t *testing.T) {
	numTests := 1

	for i := 0; i < numTests; i++ {
		fileName := "../testdata/schema" + strconv.Itoa(i+1) + ".txt" // run from pwd
		str, err := ioutil.ReadFile(fileName)
		if err != nil {
			t.Errorf("Unable to read schema file " + fileName)
			continue
		}

		doc, gqlerr := parser.ParseSchema(&ast.Source{Input: string(str)})
		if gqlerr != nil {
			t.Errorf("Unable to parse file" + fileName + " " + gqlerr.Message)
			continue
		}

		if gqlErrList := ValidateSchema(doc); gqlErrList != nil {
			var errStr strings.Builder
			for _, err := range gqlErrList {
				errStr.WriteString(err.Message)
			}

			t.Errorf(errStr.String())
		}

		AddScalars(doc)

		schema, gqlerr := validator.ValidateSchemaDocument(doc)
		if gqlerr != nil {
			t.Errorf("Unable to validate schema for " + fileName + " " + gqlerr.Message)
			continue
		}

		GenerateCompleteSchema(schema)

		newSchemaStr := Stringify(schema)
		// fmt.Println(newSchemaStr)
		newDoc, gqlerr := parser.ParseSchema(&ast.Source{Input: newSchemaStr})
		if gqlerr != nil {
			t.Errorf("Unable to parse new schema "+gqlerr.Message+" %+v", gqlerr.Locations[0])
			continue
		}

		_, gqlerr = validator.ValidateSchemaDocument(newDoc)
		if gqlerr != nil {
			t.Errorf("Unable to validate new schema " + gqlerr.Message)
			continue
		}

		outFile := "../testdata/schema" + strconv.Itoa(i+1) + "_output.txt"
		str, err = ioutil.ReadFile(outFile)
		if err != nil {
			t.Errorf("Unable to read output file " + outFile)
			continue
		}

		doc, gqlerr = parser.ParseSchema(&ast.Source{Input: string(str)})
		if gqlerr != nil {
			t.Errorf("Unable to parse file" + fileName + " " + gqlerr.Message)
			continue
		}

		outputSchema, gqlerr := validator.ValidateSchemaDocument(doc)
		if gqlerr != nil {
			t.Errorf("Unable to validate schema for " + fileName + " " + gqlerr.Message)
			continue
		}

		require.Equal(t, true, AreEqualSchema(schema, outputSchema))
	}
}

func TestInvalidSchemas(t *testing.T) {
	numTests := 3

	for i := 0; i < numTests; i++ {
		fileName := "../testdata/invalidschema" + strconv.Itoa(i+1) + ".txt" // run from pwd
		str, err := ioutil.ReadFile(fileName)
		if err != nil {
			t.Errorf("Unable to read file " + fileName)
		}

		doc, gqlerr := parser.ParseSchema(&ast.Source{Input: string(str)})
		if gqlerr != nil {
			t.Errorf("Unable to parse file" + fileName + " " + gqlerr.Message)
			continue
		}

		gqlErrList := ValidateSchema(doc)
		if gqlErrList == nil {
			t.Errorf("Invalid schema passed tests.")
		}

		AddScalars(doc)

		_, gqlerr = validator.ValidateSchemaDocument(doc)
		if gqlerr != nil {
			t.Errorf("Unable to validate schema for " + fileName + " " + gqlerr.Message)
			continue
		}
	}
}
