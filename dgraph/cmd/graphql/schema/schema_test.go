package schema

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"testing"

	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/parser"
	"github.com/vektah/gqlparser/validator"
)

func TestSchemaString(t *testing.T) {
	numTests := 2

	for i := 0; i < numTests; i++ {
		ans, er := os.Getwd()
		if er == nil {
			t.Errorf(ans)
		}

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

		fmt.Println("Success!! New schema valdiated")
	}
}
