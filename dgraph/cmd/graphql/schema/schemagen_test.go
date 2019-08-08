/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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

	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
	"github.com/vektah/gqlparser/parser"
	"github.com/vektah/gqlparser/validator"
	"gopkg.in/yaml.v2"
)

func TestDGSchemaGen(t *testing.T) {
	fileName := "schemagen_test.yml"
	byts, err := ioutil.ReadFile(fileName)
	require.Nil(t, err, "Unable to read file %s", fileName)

	var tests Tests
	err = yaml.Unmarshal(byts, &tests)
	require.Nil(t, err, "Unable to unmarshal to yaml!")

	for _, schemas := range tests {
		for _, sch := range schemas {
			t.Run(sch.Name, func(t *testing.T) {

				schHandler, errs := NewSchemaHandler(sch.Input)
				require.Nil(t, errs)

				dgSchema := schHandler.DGSchema()
				require.Equal(t, sch.Output, dgSchema, sch.Name)
			})
		}
	}
}

func TestSchemaString(t *testing.T) {
	numTests := 1

	for i := 0; i < numTests; i++ {
		fileName := "../testdata/schema" + strconv.Itoa(i+1) + ".txt" // run from pwd
		str1, err := ioutil.ReadFile(fileName)
		if err != nil {
			t.Errorf("Unable to read schema file %s", fileName)
			continue
		}

		schHandler, errs := NewSchemaHandler(string(str1))
		require.Nil(t, errs)

		newSchemaStr := schHandler.GQLSchema()

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

		handlerObj := schHandler.(schemaHandler)
		require.Equal(t, true, AreEqualSchema(handlerObj.completeSchema, outputSch))
	}
}

type Tests map[string][]TestCase

type TestCase struct {
	Name    string
	Input   string
	Errlist gqlerror.List
	Output  string
}

func TestInvalidSchemas(t *testing.T) {
	fileName := "gqlschema_test.yml" // run from pwd
	byts, err := ioutil.ReadFile(fileName)
	require.Nil(t, err, "Unable to read file %s", fileName)

	var tests Tests
	err = yaml.Unmarshal(byts, &tests)
	require.Nil(t, err, "Error Unmarshalling to yaml!")

	for _, schemas := range tests {
		for _, sch := range schemas {
			t.Run(sch.Name, func(t *testing.T) {

				_, errlist := NewSchemaHandler(sch.Input)
				require.Equal(t, sch.Errlist, errlist, sch.Name)
			})
		}
	}
}
