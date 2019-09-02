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
	"io/ioutil"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/gqlerror"
	"gopkg.in/yaml.v2"
)

type Tests map[string][]TestCase

type TestCase struct {
	Name    string
	Input   string
	Errlist gqlerror.List
	Output  string
}

func TestDGSchemaGen(t *testing.T) {
	fileName := "schemagen_test.yml"
	byts, err := ioutil.ReadFile(fileName)
	require.NoError(t, err, "Unable to read file %s", fileName)

	var tests Tests
	err = yaml.Unmarshal(byts, &tests)
	require.NoError(t, err, "Unable to unmarshal to yaml!")

	for _, schemas := range tests {
		for _, sch := range schemas {
			t.Run(sch.Name, func(t *testing.T) {

				schHandler, errs := NewHandler(sch.Input)
				require.NoError(t, errs)

				dgSchema := schHandler.DGSchema()
				require.Equal(t, sch.Output, dgSchema, sch.Name)
			})
		}
	}
}

func TestSchemaString(t *testing.T) {
	inputDir := "testdata/input/"
	outputDir := "testdata/output/"

	files, err := ioutil.ReadDir(inputDir)
	require.NoError(t, err)

	filesToTest := map[string]bool{"schema1": true, "schema2": true}

	for _, testFile := range files {
		if _, ok := filesToTest[testFile.Name()]; !ok {
			continue
		}
		t.Run(testFile.Name(), func(t *testing.T) {
			inputFileName := inputDir + testFile.Name()
			str1, err := ioutil.ReadFile(inputFileName)
			require.NoError(t, err)

			schHandler, errs := NewHandler(string(str1))
			require.NoError(t, errs)

			newSchemaStr := schHandler.GQLSchema()

			outputFileName := outputDir + testFile.Name()
			str2, err := ioutil.ReadFile(outputFileName)
			require.NoError(t, err)

			if diff := cmp.Diff(string(str2), newSchemaStr); diff != "" {
				t.Errorf("schema mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSchemas(t *testing.T) {
	fileName := "gqlschema_test.yml"
	byts, err := ioutil.ReadFile(fileName)
	require.NoError(t, err, "Unable to read file %s", fileName)

	var tests Tests
	err = yaml.Unmarshal(byts, &tests)
	require.NoError(t, err, "Error Unmarshalling to yaml!")

	t.Run("Valid Schemas", func(t *testing.T) {
		for _, sch := range tests["valid_schemas"] {
			t.Run(sch.Name, func(t *testing.T) {
				_, errlist := NewHandler(sch.Input)
				require.NoError(t, errlist, sch.Name)
			})
		}
	})

	t.Run("Invalid Schemas", func(t *testing.T) {
		for _, sch := range tests["invalid_schemas"] {
			t.Run(sch.Name, func(t *testing.T) {
				_, errlist := NewHandler(sch.Input)
				require.Equal(t, errlist, sch.Errlist, sch.Name)
			})
		}
	})
}
