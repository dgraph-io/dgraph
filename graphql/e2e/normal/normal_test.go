//go:build integration

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package normal

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/ristretto/v2/z"
	"github.com/hypermodeinc/dgraph/v25/graphql/e2e/common"
	"github.com/hypermodeinc/dgraph/v25/x"
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
