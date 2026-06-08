//go:build integration2

/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/v25/dgraphapi"
	"github.com/dgraph-io/dgraph/v25/dgraphtest"
	"github.com/dgraph-io/dgraph/v25/x"

	"github.com/stretchr/testify/require"
)

const (
	gqlSchema = `[
		{
		  "namespace": 0,
		  "schema": "type Message {id: ID!\ncontent: String!\nauthor: String\nuniqueId: Int64! @id\n}"
		},
		{
		  "namespace": 1,
		  "schema": "type Template {id: ID!\ncontent: String!\nuniqueId: Int64! @id\n}"
		}
	  ]`
	jsonData = `
	[
		{"Message.content": "XBNTBGBHGQ", "Message.author": "PXYNHBWGGD",
			"Message.uniqueId": 7, "dgraph.type": "Message", "namespace": 0},
		{"Message.content": "ILEOMLXRYX", "Message.author": "BBBZKURCJH",
			"Message.uniqueId": 5, "dgraph.type": "Message", "namespace": 0},
		{"Message.content": "RFAXPWUCUN", "Message.author": "CMZEOCORNL",
			"Message.uniqueId": 0, "dgraph.type": "Message", "namespace": 0},
		{"Message.content": "ZKCRMYNBLT", "Message.author": "TYLORHNKJA",
			"Message.uniqueId": 9, "dgraph.type": "Message", "namespace": 0},
		{"Message.content": "HMODLPKCHE", "Message.author": "ZNTIZEYBMV",
			"Message.uniqueId": 4, "dgraph.type": "Message", "namespace": 0},
		{"Message.content": "FBIOEOJBZF", "Message.author": "EQXLNWFYBN",
			"Message.uniqueId": 6, "dgraph.type": "Message", "namespace": 0},
		{"Message.content": "DVTCTXCVYI", "Message.author": "USYMVFJYXA",
			"Message.uniqueId": 3, "dgraph.type": "Message", "namespace": 0},
		{"Message.content": "SOWTAXHTCT", "Message.author": "SAILDEMEJV",
			"Message.uniqueId": 8, "dgraph.type": "Message", "namespace": 0},
		{"Message.content": "MLMQWMJQQW", "Message.author": "ANBSOCYLXB",
			"Message.uniqueId": 1, "dgraph.type": "Message", "namespace": 0},
		{"Message.content": "CVFSBBIDCL", "Message.author": "JONAEYCCTQ",
			"Message.uniqueId": 2, "dgraph.type": "Message", "namespace": 0},
		{"Template.content": "t1", "Template.uniqueId": 1, "dgraph.type": "Template", "namespace": 1},
		{"Template.content": "t2", "Template.uniqueId": 2, "dgraph.type": "Template", "namespace": 1},
		{"Template.content": "t3", "Template.uniqueId": 3, "dgraph.type": "Template", "namespace": 1},
		{"dgraph.xid": "groot", "dgraph.password": "password", "dgraph.type": "dgraph.type.User",
			"dgraph.user.group": {"dgraph.xid": "guardians", "dgraph.type": "dgraph.type.Group",
			"namespace": 1}, "namespace": 1}
	]`
)

func TestBulkLoaderSkipReducePhase(t *testing.T) {
	tmpDir := t.TempDir()

	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).
		WithACL(time.Hour).WithReplicas(1).WithBulkLoadOutDir(t.TempDir())
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()

	require.NoError(t, c.StartZero(0))
	require.NoError(t, c.HealthCheck(true))

	baseDir := t.TempDir()
	dataFile := filepath.Join(baseDir, "data.json")
	require.NoError(t, os.WriteFile(dataFile, []byte(jsonData), os.ModePerm))
	gqlSchemaFile := filepath.Join(baseDir, "gql.schema")
	require.NoError(t, os.WriteFile(gqlSchemaFile, []byte(gqlSchema), os.ModePerm))

	// First run: map phase only, preserve tmp dir
	mapOpts := dgraphtest.BulkOpts{
		DataFiles:       []string{dataFile},
		GQLSchemaFiles:  []string{gqlSchemaFile},
		TmpDir:          tmpDir,
		SkipReducePhase: true,
	}
	require.NoError(t, c.BulkLoad(mapOpts))

	// Second run: reduce phase only, using the same tmp dir.
	// Data and schema files are not needed; all input was processed in the map phase.
	reduceOpts := dgraphtest.BulkOpts{
		TmpDir:       tmpDir,
		SkipMapPhase: true,
	}
	require.NoError(t, c.BulkLoad(reduceOpts))

	require.NoError(t, c.Start())

	hc, err := c.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser,
		dgraphapi.DefaultPassword, x.RootNamespace))

	params := dgraphapi.GraphQLParams{
		Query: `query {
			getMessage(uniqueId: 3) {
				content
				author
			}
		}`,
	}
	data, err := hc.RunGraphqlQuery(params, false)
	require.NoError(t, err)
	require.NoError(t, dgraphapi.CompareJSON(`{
		"getMessage": {
		  "content": "DVTCTXCVYI",
		  "author": "USYMVFJYXA"
		}
	  }`, string(data)))
}

// TestBulkLoaderBM25 verifies the BM25 index is correctly built by the bulk loader:
// term postings must carry their packed (term frequency, document length) value, and
// the corpus statistics must be written, or bm25() queries return nothing. It loads a
// small corpus via bulk, then checks that all documents containing the term are found
// and that the densest/shortest document ranks first.
func TestBulkLoaderBM25(t *testing.T) {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).
		WithACL(time.Hour).WithReplicas(1).WithBulkLoadOutDir(t.TempDir())
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()

	require.NoError(t, c.StartZero(0))
	require.NoError(t, c.HealthCheck(true))

	baseDir := t.TempDir()
	schemaFile := filepath.Join(baseDir, "bm25.schema")
	require.NoError(t, os.WriteFile(schemaFile,
		[]byte("description_bm25: string @index(bm25) .\n"), os.ModePerm))

	dataFile := filepath.Join(baseDir, "bm25.rdf")
	rdf := `
		<0x1> <description_bm25> "the quick brown fox jumps over the lazy dog" .
		<0x2> <description_bm25> "fox fox fox" .
		<0x3> <description_bm25> "the lazy dog sleeps in the warm sun all day" .
		<0x4> <description_bm25> "quick brown foxes are agile animals" .
	`
	require.NoError(t, os.WriteFile(dataFile, []byte(rdf), os.ModePerm))

	require.NoError(t, c.BulkLoad(dgraphtest.BulkOpts{
		DataFiles:   []string{dataFile},
		SchemaFiles: []string{schemaFile},
	}))

	require.NoError(t, c.Start())

	hc, err := c.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser,
		dgraphapi.DefaultPassword, x.RootNamespace))

	// Both documents containing "fox" (0x1 and 0x2) must be found — proves the term
	// postings and corpus statistics survived the bulk build.
	data, err := hc.PostDqlQuery(`{ q(func: bm25(description_bm25, "fox")) { count(uid) } }`)
	require.NoError(t, err)
	require.Contains(t, string(data), `"count":3`,
		"bulk-loaded bm25 index must find every document containing the term")

	// The all-"fox" document (0x2, tf=3, shortest) must rank first.
	data, err = hc.PostDqlQuery(
		`{ q(func: bm25(description_bm25, "fox"), first: 1) { description_bm25 } }`)
	require.NoError(t, err)
	require.True(t, strings.Contains(string(data), "fox fox fox"),
		"densest, shortest document must rank first after bulk load; got: %s", string(data))
}

func TestBulkLoaderNoDqlSchema(t *testing.T) {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(2).WithNumZeros(1).
		WithACL(time.Hour).WithReplicas(1).WithBulkLoadOutDir(t.TempDir())
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()

	// start zero
	require.NoError(t, c.StartZero(0))
	require.NoError(t, c.HealthCheck(true))

	baseDir := t.TempDir()
	gqlSchemaFile := filepath.Join(baseDir, "gql.schema")
	require.NoError(t, os.WriteFile(gqlSchemaFile, []byte(gqlSchema), os.ModePerm))
	dataFile := filepath.Join(baseDir, "data.json")
	require.NoError(t, os.WriteFile(dataFile, []byte(jsonData), os.ModePerm))

	opts := dgraphtest.BulkOpts{
		DataFiles:      []string{dataFile},
		GQLSchemaFiles: []string{gqlSchemaFile},
	}
	require.NoError(t, c.BulkLoad(opts))

	// start Alphas
	require.NoError(t, c.Start())

	// run some queries and ensure everything looks good
	hc, err := c.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser,
		dgraphapi.DefaultPassword, x.RootNamespace))

	params := dgraphapi.GraphQLParams{
		Query: `query {
			getMessage(uniqueId: 3) {
				content
				author
			}
		}`,
	}
	data, err := hc.RunGraphqlQuery(params, false)
	require.NoError(t, err)
	dgraphapi.CompareJSON(`{
		"getMessage": {
		  "content": "DVTCTXCVYI",
		  "author": "USYMVFJYXA"
		}
	  }`, string(data))

	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser, dgraphapi.DefaultPassword, 1))
	params = dgraphapi.GraphQLParams{
		Query: `query {
			getTemplate(uniqueId: 2) {
				content
			}
		}`,
	}
	data, err = hc.RunGraphqlQuery(params, false)
	require.NoError(t, err)
	dgraphapi.CompareJSON(`{
		"getTemplate": {
		  "content": "t2"
		}
	  }`, string(data))
}
