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

	dschema "github.com/dgraph-io/dgraph/schema"
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
				_, err := dschema.Parse(dgSchema)
				require.NoError(t, err)
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

// The other tests verify that @search works where it is expected to work,
// and show what the error messages look like.  This test shows all the cases
// that shouldn't work - i.e. we'll never accept a search where we don't
// expect one.  It's too annoying to have all the errors for this, so It just
// makes sure that there are as many errors as cases.
func TestOnlyCorrectSearchArgsWork(t *testing.T) {
	tests := map[string]struct {
		schema         string
		expectedErrors int
	}{
		"String searches don't apply to Int": {schema: `
			type X {
				str1: Int @search(by: hash)
				str2: Int @search(by: exact)
				str3: Int @search(by: term)
				str4: Int @search(by: fulltext)
				str5: Int @search(by: trigram)
			}`,
			expectedErrors: 5},
		"String searches don't apply to Float": {schema: `
			type X {
				str1: Float @search(by: hash)
				str2: Float @search(by: exact)
				str3: Float @search(by: term)
				str4: Float @search(by: fulltext)
				str5: Float @search(by: trigram)
			}`,
			expectedErrors: 5},
		"String searches don't apply to Boolean": {schema: `
			type X {
				str1: Boolean @search(by: hash)
				str2: Boolean @search(by: exact)
				str3: Boolean @search(by: term)
				str4: Boolean @search(by: fulltext)
				str5: Boolean @search(by: trigram)
			}`,
			expectedErrors: 5},
		"String searches don't apply to DateTime": {schema: `
			type X {
				str1: DateTime @search(by: hash)
				str2: DateTime @search(by: exact)
				str3: DateTime @search(by: term)
				str4: DateTime @search(by: fulltext)
				str5: DateTime @search(by: trigram)
			}`,
			expectedErrors: 5},
		"DateTime searches don't apply to Int": {schema: `
			type X {
				dt1: Int @search(by: year)
				dt2: Int @search(by: month)
				dt3: Int @search(by: day)
				dt4: Int @search(by: hour)
			}`,
			expectedErrors: 4},
		"DateTime searches don't apply to Float": {schema: `
			type X {
				dt1: Float @search(by: year)
				dt2: Float @search(by: month)
				dt3: Float @search(by: day)
				dt4: Float @search(by: hour)
			}`,
			expectedErrors: 4},
		"DateTime searches don't apply to Boolean": {schema: `
			type X {
				dt1: Boolean @search(by: year)
				dt2: Boolean @search(by: month)
				dt3: Boolean @search(by: day)
				dt4: Boolean @search(by: hour)
			}`,
			expectedErrors: 4},
		"DateTime searches don't apply to String": {schema: `
			type X {
				dt1: String @search(by: year)
				dt2: String @search(by: month)
				dt3: String @search(by: day)
				dt4: String @search(by: hour)
			}`,
			expectedErrors: 4},
		"Int searches only appy to Int": {schema: `
			type X {
				i1: Float @search(by: int)
				i2: Boolean @search(by: int)
				i3: String @search(by: int)
				i4: DateTime @search(by: int)
			}`,
			expectedErrors: 4},
		"Float searches only appy to Float": {schema: `
			type X {
				f1: Int @search(by: float)
				f2: Boolean @search(by: float)
				f3: String @search(by: float)
				f4: DateTime @search(by: float)
			}`,
			expectedErrors: 4},
		"Boolean searches only appy to Boolean": {schema: `
			type X {
				b1: Int @search(by: bool)
				b2: Float @search(by: bool)
				b3: String @search(by: bool)
				b4: DateTime @search(by: bool)
			}`,
			expectedErrors: 4},
		"Enums can only have no arg search": {schema: `
			type X {
				e1: E @search(by: int)
				e2: E @search(by: float)
				e3: E @search(by: bool)
				e4: E @search(by: year)
				e5: E @search(by: month)
				e6: E @search(by: day)
				e7: E @search(by: hour)
				e8: E @search(by: hash)
				e9: E @search(by: exact)
				e10: E @search(by: term)
				e11: E @search(by: fulltext)
				e12: E @search(by: trigram)
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
				"every field in this test applies @search wrongly and should raise an error")
		})
	}
}
