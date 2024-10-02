//go:build integration

/*
 *    Copyright 2023 Dgraph Labs, Inc. and Contributors
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

package normal

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v24/graphql/e2e/common"
	"github.com/dgraph-io/dgraph/v24/x"
	"github.com/dgraph-io/ristretto/z"
)

func TestRunAll_Normal(t *testing.T) {
	common.RunAll(t)
}

func TestSchema_Normal(t *testing.T) {
	b, err := os.ReadFile("schema_response.json")
	require.NoError(t, err)

	t.Run("graphql schema", func(t *testing.T) {
		common.SchemaTest(t, string(b))
	})
}

func TestMain(m *testing.M) {
	schemaFile := "schema.graphql"
	schema, err := os.ReadFile(schemaFile)
	x.Panic(err)

	jsonFile := "test_data.json"
	data, err := os.ReadFile(jsonFile)
	x.Panic(err)

	// set up the lambda url for unit tests
	x.Config.GraphQL = z.NewSuperFlag("lambda-url=http://localhost:8086/graphql-worker;").
		MergeAndCheckDefault("lambda-url=;")

	common.BootstrapServer(schema, data)

	m.Run()
}
