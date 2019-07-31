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
	"testing"

	"github.com/vektah/gqlparser/gqlerror"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/parser"
	"github.com/vektah/gqlparser/validator"
)

func TestSchemaString(t *testing.T) {
	numTests := 1

	for i := 0; i < numTests; i++ {
		fileName := "../testdata/schema" + strconv.Itoa(i+1) + ".txt" // run from pwd
		str1, err := ioutil.ReadFile(fileName)
		if err != nil {
			t.Errorf("Unable to read schema file %s", fileName)
			continue
		}

		doc, gqlErr := parser.ParseSchema(&ast.Source{Input: string(str1)})
		if gqlErr != nil {
			t.Errorf(gqlErr.Error())
			continue
		}

		gqlErrList := ValidateSchema(doc)
		if gqlErrList != nil {
			t.Errorf(gqlErrList.Error())
			continue
		}

		AddScalars(doc)

		sch, gqlErr := validator.ValidateSchemaDocument(doc)
		if gqlErr != nil {
			t.Errorf(gqlErr.Error())
			continue
		}

		GenerateCompleteSchema(sch)

		newSchemaStr := Stringify(sch)
		fmt.Println(newSchemaStr)

		outFile := "../testdata/schema" + strconv.Itoa(i+1) + "_output.txt"
		str2, err := ioutil.ReadFile(outFile)
		if err != nil {
			t.Errorf("Unable to read output file " + outFile)
			continue
		}

		doc, gqlerr := parser.ParseSchema(&ast.Source{Input: string(str2)})
		if gqlerr != nil {
			t.Errorf(gqlerr.Error())
			continue
		}

		outputSch, errlist2 := validator.ValidateSchemaDocument(doc)
		if errlist2 != nil {
			t.Errorf(errlist2.Error())
			continue
		}

		require.Equal(t, true, AreEqualSchema(sch, outputSch))
	}
}

type Tests map[string][]TestCase
type TestCase struct {
	Name   string
	Input  string
	Output gqlerror.List
}

func TestInvalidSchemas(t *testing.T) {
	fileName := "schema_test.yml" // run from pwd
	byts, err := ioutil.ReadFile(fileName)
	require.Nil(t, err, "Unable to read file %s", fileName)

	var tests Tests
	err = yaml.Unmarshal(byts, &tests)
	require.Nil(t, err, "Error Unmarshalling to yaml!")

	for _, schemas := range tests {
		for _, sch := range schemas {
			t.Run(sch.Name, func(t *testing.T) {

				doc, gqlErr := parser.ParseSchema(&ast.Source{Input: sch.Input})
				require.Nil(t, gqlErr, gqlErr.Error())

				gqlErrList := ValidateSchema(doc)

				require.Equal(t, sch.Output, gqlErrList, sch.Name)
			})
		}
	}
}
