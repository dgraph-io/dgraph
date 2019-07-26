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

package schema

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
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

		if gqlErrList := PreGQLValidtion(doc); gqlErrList != nil {
			var errStr strings.Builder
			for _, err := range gqlErrList {
				errStr.WriteString(err.Message)
			}

			t.Errorf(errStr.String())
		}

		AddScalars(doc)
		AddDirectives(doc)

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

func TestInvalidSchemas(t *testing.T) {
	numTests := 8

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

		gqlErrList1 := PreGQLValidtion(doc)
		if gqlErrList1 != nil {
			continue
		}
		AddScalars(doc)
		AddDirectives(doc)
		sch, gqlErr := validator.ValidateSchemaDocument(doc)
		if gqlErr != nil {
			continue
		}

		gqlErrList2 := PostGQLValidation(sch)
		if gqlErrList2 == nil {
			t.Errorf("Invalid schema %s passed tests", fileName)
		}
	}
}

func TestEqualSchema(t *testing.T) {
	numTests := 2

	for i := 1; i <= numTests; i++ {
		fileName1 := "../testdata/equalschema" + strconv.Itoa(i) + "_1.txt" // run from pwd
		str1, err := ioutil.ReadFile(fileName1)
		if err != nil {
			t.Errorf("Unable to read file %s", fileName1)
		}

		doc1, gqlerr := parser.ParseSchema(&ast.Source{Input: string(str1)})
		if gqlerr != nil {
			t.Errorf("Unable to parse file %s\n%s", fileName1, gqlerr.Error())
			continue
		}

		gqlErrList1 := PreGQLValidtion(doc1)
		if gqlErrList1 != nil {
			t.Errorf("Unable to validate schemafile %s\n%s", fileName1, gqlErrList1.Error())
		}

		AddScalars(doc1)

		schema1, gqlerr := validator.ValidateSchemaDocument(doc1)
		if gqlerr != nil {
			t.Errorf("Unable to validate schemafile %s\n%s", fileName1, gqlerr.Error())
			continue
		}

		GenerateCompleteSchema(schema1)

		fileName2 := "../testdata/equalschema" + strconv.Itoa(i) + "_2.txt" // run from pwd
		str2, err := ioutil.ReadFile(fileName2)
		if err != nil {
			t.Errorf("Unable to read file %s", fileName2)
		}

		doc2, gqlerr := parser.ParseSchema(&ast.Source{Input: string(str2)})
		if gqlerr != nil {
			t.Errorf("Unable to parse file %s\n%s", fileName2, gqlerr.Error())
			continue
		}

		gqlErrList2 := PreGQLValidtion(doc2)
		if gqlErrList2 != nil {
			t.Errorf("Unable to validate schemafile %s\n%s", fileName2, gqlErrList2.Error())
		}

		AddScalars(doc2)

		schema2, gqlerr := validator.ValidateSchemaDocument(doc2)
		if gqlerr != nil {
			t.Errorf("Unable to validate schemafile %s\n%s", fileName2, gqlerr.Error())
			continue
		}

		GenerateCompleteSchema(schema2)

		require.True(t, AreEqualSchema(schema1, schema2))
	}
}
