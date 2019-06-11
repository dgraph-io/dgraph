package schema

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/parser"
	"github.com/vektah/gqlparser/validator"
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

		AddScalars(doc)

		schema, gqlerr := validator.ValidateSchemaDocument(doc)
		if gqlerr != nil {
			t.Errorf("Unable to validate schema for " + fileName + " " + gqlerr.Message)
			continue
		}

		GenerateCompleteSchema(schema)

		newSchemaStr := Stringify(schema)
		fmt.Println(newSchemaStr)
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
