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

package dgschema

import (
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"

	gschema "github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"gopkg.in/yaml.v2"
)

type Tests map[string][]TestCase

type TestCase struct {
	Name   string
	Input  string
	Output string
}

func TestDGSchemaGen(t *testing.T) {
	fileName := "schemagen_test.yml" // run from pwd
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
			t.Run(sch.Name, func(t *testing.T) {
				dgsch, err := gschema.GenerateCompleteSchema(sch.Input)
				if err != nil {
					t.Errorf(err.Error())
					return
				}

				require.Equal(t, sch.Output, GenDgSchema(dgsch))
			})
		}
	}

}
