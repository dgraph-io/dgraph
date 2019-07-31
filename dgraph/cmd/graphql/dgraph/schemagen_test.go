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

package dgraph

import (
	"io/ioutil"
	"testing"

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/parser"
	"github.com/vektah/gqlparser/validator"
	"gopkg.in/yaml.v2"
)

type Tests map[string][]TestCase

type TestCase struct {
	Name   string
	Input  string
	Output string
}

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

				doc, gqlErr := parser.ParseSchema(&ast.Source{Input: sch.Input})
				require.Nil(t, gqlErr, gqlErr.Error())

				gqlErrList := schema.ValidateSchema(doc)
				require.Nil(t, gqlErrList, gqlErrList.Error())

				schema.AddScalars(doc)

				dgsch, gqlErr := validator.ValidateSchemaDocument(doc)
				require.Nil(t, gqlErr, gqlErr.Error())

				require.Equal(t, sch.Output, GenDgSchema(dgsch), sch.Name)
			})
		}
	}
}
