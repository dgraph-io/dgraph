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

package graphql

import (
	"io/ioutil"
	"testing"

	"github.com/dgraph-io/dgraph/x"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

type ErrorCase struct {
	Name       string
	GQLRequest string
	variables  map[string]interface{}
	Errors     x.GqlErrorList
}

// TestRequestValidationErrors just makes sure we are catching validation failures.
// Mostly this is provided by an external lib, so just checking we hit common cases.
func TestRequestValidationErrors(t *testing.T) {
	b, err := ioutil.ReadFile("e2e_error_test.yaml")
	require.NoError(t, err, "Unable to read test file")

	var tests []ErrorCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal test cases from yaml.")

	for _, tcase := range tests {
		t.Run(tcase.Name, func(t *testing.T) {
			test := &GraphQLParams{
				Query:     tcase.GQLRequest,
				Variables: tcase.variables,
			}
			gqlResponse := test.ExecuteAsPost(t, graphqlURL)

			require.Nil(t, gqlResponse.Data)
			if diff := cmp.Diff(tcase.Errors, gqlResponse.Errors); diff != "" {
				t.Errorf("errors mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
