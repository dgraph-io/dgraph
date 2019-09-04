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
				if diff := cmp.Diff(sch.Output, dgSchema); diff != "" {
					t.Errorf("schema mismatch (-want +got):\n%s", diff)
				}
			})
		}
	}
}

func TestSchemaString(t *testing.T) {
	inputDir := "testdata/schemagen/input/"
	outputDir := "testdata/schemagen/output/"

	files, err := ioutil.ReadDir(inputDir)
	require.NoError(t, err)

	for _, testFile := range files {
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
				if diff := cmp.Diff(sch.Errlist, errlist); diff != "" {
					t.Errorf("error mismatch (-want +got):\n%s", diff)
				}
			})
		}
	})
}

// The other tests verify that @searchable works where it is expected to work,
// and show what the error messages look like.  This test shows all the cases
// that shouldn't work - i.e. we'll never accept a searchable where we don't
// expect one.  It's too annoying to have all the errors for this, so It just
// makes sure that there are as many errors as cases.
func TestOnlyCorrectSearchablesWork(t *testing.T) {
	tests := map[string]struct {
		schema         string
		expectedErrors int
	}{
		"String searchables don't apply to Int": {schema: `
			type X {
				str1: Int @searchable(by: hash)
				str2: Int @searchable(by: exact)
				str3: Int @searchable(by: term)
				str4: Int @searchable(by: fulltext)
				str5: Int @searchable(by: trigram)
			}`,
			expectedErrors: 5},
		"String searchables don't apply to Float": {schema: `
			type X {
				str1: Float @searchable(by: hash)
				str2: Float @searchable(by: exact)
				str3: Float @searchable(by: term)
				str4: Float @searchable(by: fulltext)
				str5: Float @searchable(by: trigram)
			}`,
			expectedErrors: 5},
		"String searchables don't apply to Boolean": {schema: `
			type X {
				str1: Boolean @searchable(by: hash)
				str2: Boolean @searchable(by: exact)
				str3: Boolean @searchable(by: term)
				str4: Boolean @searchable(by: fulltext)
				str5: Boolean @searchable(by: trigram)
			}`,
			expectedErrors: 5},
		"String searchables don't apply to DateTime": {schema: `
			type X {
				str1: DateTime @searchable(by: hash)
				str2: DateTime @searchable(by: exact)
				str3: DateTime @searchable(by: term)
				str4: DateTime @searchable(by: fulltext)
				str5: DateTime @searchable(by: trigram)
			}`,
			expectedErrors: 5},
		"DateTime searchables don't apply to Int": {schema: `
			type X {
				dt1: Int @searchable(by: year)
				dt2: Int @searchable(by: month)
				dt3: Int @searchable(by: day)
				dt4: Int @searchable(by: hour)
			}`,
			expectedErrors: 4},
		"DateTime searchables don't apply to Float": {schema: `
			type X {
				dt1: Float @searchable(by: year)
				dt2: Float @searchable(by: month)
				dt3: Float @searchable(by: day)
				dt4: Float @searchable(by: hour)
			}`,
			expectedErrors: 4},
		"DateTime searchables don't apply to Boolean": {schema: `
			type X {
				dt1: Boolean @searchable(by: year)
				dt2: Boolean @searchable(by: month)
				dt3: Boolean @searchable(by: day)
				dt4: Boolean @searchable(by: hour)
			}`,
			expectedErrors: 4},
		"DateTime searchables don't apply to String": {schema: `
			type X {
				dt1: String @searchable(by: year)
				dt2: String @searchable(by: month)
				dt3: String @searchable(by: day)
				dt4: String @searchable(by: hour)
			}`,
			expectedErrors: 4},
		"Int searchables only appy to Int": {schema: `
			type X {
				i1: Float @searchable(by: int)
				i2: Boolean @searchable(by: int)
				i3: String @searchable(by: int)
				i4: DateTime @searchable(by: int)
			}`,
			expectedErrors: 4},
		"Float searchables only appy to Float": {schema: `
			type X {
				f1: Int @searchable(by: float)
				f2: Boolean @searchable(by: float)
				f3: String @searchable(by: float)
				f4: DateTime @searchable(by: float)
			}`,
			expectedErrors: 4},
		"Boolean searchables only appy to Boolean": {schema: `
			type X {
				b1: Int @searchable(by: bool)
				b2: Float @searchable(by: bool)
				b3: String @searchable(by: bool)
				b4: DateTime @searchable(by: bool)
			}`,
			expectedErrors: 4},
		"Enums can only have no arg searchable": {schema: `
			type X {
				e1: E @searchable(by: int)
				e2: E @searchable(by: float)
				e3: E @searchable(by: bool)
				e4: E @searchable(by: year)
				e5: E @searchable(by: month)
				e6: E @searchable(by: day)
				e7: E @searchable(by: hour)
				e8: E @searchable(by: hash)
				e9: E @searchable(by: exact)
				e10: E @searchable(by: term)
				e11: E @searchable(by: fulltext)
				e12: E @searchable(by: trigram)
			}
			enum E {
				A
			}`,
			expectedErrors: 12},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, errlist := NewHandler(test.schema)
			require.Len(t, errlist, test.expectedErrors,
				"every field in this test applies @searchable wrongly and should raise an error")
		})
	}
}
