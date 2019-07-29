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
			t.Errorf("Unable to read schema file " + fileName)
			continue
		}

		sch, errlist1 := GenerateCompleteSchema(string(str1))
		if errlist1 != nil {
			t.Errorf(errlist1.Error())
		}

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

func TestEqualSchema(t *testing.T) {
	numTests := 2

	for i := 1; i <= numTests; i++ {
		fileName1 := "../testdata/equalschema" + strconv.Itoa(i) + "_1.txt" // run from pwd
		str1, err := ioutil.ReadFile(fileName1)
		if err != nil {
			t.Errorf("Unable to read file %s", fileName1)
			continue
		}

		sch1, errlist1 := GenerateCompleteSchema(string(str1))
		if errlist1 != nil {
			t.Errorf(errlist1.Error())
			continue
		}

		fileName2 := "../testdata/equalschema" + strconv.Itoa(i) + "_2.txt" // run from pwd
		str2, err := ioutil.ReadFile(fileName2)
		if err != nil {
			t.Errorf("Unable to read file %s", fileName2)
		}

		sch2, errlist2 := GenerateCompleteSchema(string(str2))
		if errlist2 != nil {
			t.Errorf(errlist2.Error())
			continue
		}

		require.True(t, AreEqualSchema(sch1, sch2))
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
	if err != nil {
		t.Errorf(err.Error())
		return
	}

	var tests Tests
	err = yaml.Unmarshal(byts, &tests)
	if err != nil {
		t.Errorf(err.Error())
		return
	}

	for _, schemas := range tests {
		for _, sch := range schemas {
			doc, gqlerr := parser.ParseSchema(&ast.Source{Input: string(sch.Input)})
			if gqlerr != nil {
				t.Errorf(gqlerr.Error())
				continue
			}

			t.Run(sch.Name, func(t *testing.T) {
				gqlErrList := ValidateSchema(doc)
				require.Equal(t, sch.Output, gqlErrList, fmt.Sprintf(sch.Name))
			})
		}
	}
}
