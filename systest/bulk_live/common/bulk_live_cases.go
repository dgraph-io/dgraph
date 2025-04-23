/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package common

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/hypermodeinc/dgraph/v25/testutil"
)

// TODO: This test was used just to make sure some really basic examples work.
// It can be deleted once the remainder of the tests have been setup.

// run this in sequential order. cleanup is necessary for bulk loader to work
func RunBulkCases(t *testing.T) {
	hsuite := helloWorldSetup(t, true)
	testHelloWorld(t)
	hsuite.cleanup(t)

	// remote hello world only differs from hello world in setup
	hsuite = remoteHelloWorldSetup(t, true)
	testHelloWorld(t)
	hsuite.cleanup(t)

	hsuite = facetsSetup(t, true)
	testFacets(t)
	hsuite.cleanup(t)

	hsuite = countIndexSetup(t, true)
	testCountIndex(t)
	hsuite.cleanup(t)

	hsuite = indexedPredicateSetup(t, true)
	testIndexedPredicate(t)
	hsuite.cleanup(t)

	hsuite = loadTypesSetup(t, true)
	testLoadTypes(t)
	hsuite.cleanup(t)

	hsuite = bulkSingleUidSetup(t, true)
	testBulkSingleUid(t)
	hsuite.cleanup(t)

	hsuite = deleteEdgeWithStarSetup(t, true)
	testDeleteEdgeWithStar(t)
	hsuite.cleanup(t)
}

func RunBulkCasesAcl(t *testing.T) {
	opts := suiteOpts{
		schema:    helloWorldSchema,
		gqlSchema: "",
		rdfs:      helloWorldData,
		bulkSuite: true,
		bulkOpts:  bulkOpts{alpha: "../bulk/alpha_acl.yml", forceNs: 0x10},
	}
	suite := newSuiteInternal(t, opts)

	t.Run("Pan and Jackson", testCaseWithAcl(`
		{q(func: anyofterms(name, "Peter")) {
			name
		}}
	`, `
		{"q": [
			{ "name": "Peter Pan" },
			{ "name": "Peter Jackson" }
		]}
	`, "groot", "password", 0x10))
	suite.cleanup(t)
}

// run this in sequential order. cleanup is necessary for live loader to work
func RunLiveCases(t *testing.T) {
	hsuite := helloWorldSetup(t, false)
	testHelloWorld(t)
	hsuite.cleanup(t)

	// remote hello world only differs from hello world in setup
	hsuite = remoteHelloWorldSetup(t, false)
	testHelloWorld(t)
	hsuite.cleanup(t)

	hsuite = facetsSetup(t, false)
	testFacets(t)
	hsuite.cleanup(t)

	hsuite = indexedPredicateSetup(t, false)
	testIndexedPredicate(t)
	hsuite.cleanup(t)

	hsuite = countIndexSetup(t, false)
	testCountIndex(t)
	hsuite.cleanup(t)

	hsuite = loadTypesSetup(t, false)
	testLoadTypes(t)
	hsuite.cleanup(t)
}

const helloWorldSchema string = `
	name: string @index(term) .
`
const helloWorldData string = `
	_:pj <name> "Peter Jackson" .
	_:pp <name> "Peter Pan" .
`

func helloWorldSetup(t *testing.T, isBulkLoader bool) *bsuite {
	if isBulkLoader {
		s := newBulkOnlySuite(t, helloWorldSchema, helloWorldData, "")
		return s
	}

	s := newLiveOnlySuite(t, helloWorldSchema, helloWorldData, "")
	return s
}

func remoteHelloWorldSetup(t *testing.T, isBulkLoader bool) *bsuite {
	return newSuiteInternal(t, suiteOpts{
		schema:    helloWorldSchema,
		gqlSchema: "",
		rdfs:      helloWorldData,
		bulkSuite: isBulkLoader,
		remote:    true,
		bulkOpts:  bulkOpts{alpha: "../bulk/alpha.yml", forceNs: math.MaxUint64},
	})
}

func testHelloWorld(t *testing.T) {
	t.Run("Pan and Jackson", testCase(`
		{q(func: anyofterms(name, "Peter")) {
			name
		}}
	`, `
		{"q": [
			{ "name": "Peter Pan" },
			{ "name": "Peter Jackson" }
		]}
	`))

	t.Run("Pan only", testCase(`
		{q(func: anyofterms(name, "Pan")) {
			name
		}}
	`, `
		{"q": [
			{ "name": "Peter Pan" }
		]}
	`))

	t.Run("Jackson only", testCase(`
		{q(func: anyofterms(name, "Jackson")) {
			name
		}}
	`, `
		{"q": [
			{ "name": "Peter Jackson" }
		]}
	`))
}

func facetsSetup(t *testing.T, isBulkLoader bool) *bsuite {
	if isBulkLoader {
		s := newBulkOnlySuite(t, `
		name: string @index(exact) .
		boss: uid @reverse .
	`, `
		_:alice <name> "Alice" (middle_initial="J") .
		_:bob   <name> "Bob"   (middle_initial="M") .
		_:bob   <boss> _:alice (since=2017-04-26)   .
	`, "")
		return s
	}

	s := newLiveOnlySuite(t, `
		name: string @index(exact) .
		boss: uid @reverse .
	`, `
		_:alice <name> "Alice" (middle_initial="J") .
		_:bob   <name> "Bob"   (middle_initial="M") .
		_:bob   <boss> _:alice (since=2017-04-26)   .
	`, "")
	return s
}

func testFacets(t *testing.T) {
	t.Run("facet on terminal edge", testCase(`
		{q(func: eq(name, "Alice")) {
			name @facets(middle_initial)
		}}
	`, `
		{"q": [ {
			"name": "Alice",
			"name|middle_initial": "J"
		} ]}
	`))

	t.Run("facets on fwd uid edge", testCase(`
		{q(func: eq(name, "Bob")) {
			boss @facets(since) {
				name
			}
		}}
	`, `
	{
		"q":[
			{
				"boss":{
					"name":"Alice",
					"boss|since":"2017-04-26T00:00:00Z"
				}
			}
		]
	}
	`))

	t.Run("facets on rev uid edge", testCase(`
		{q(func: eq(name, "Alice")) {
			~boss @facets(since) {
				name
			}
		}}
	`, `
	{
		"q":[
			{
				"~boss":[
					{
						"name":"Bob",
						"~boss|since": "2017-04-26T00:00:00Z"
					}
				]
			}
		]
	}
	`))
}

func indexedPredicateSetup(t *testing.T, isBulkLoader bool) *bsuite {
	if isBulkLoader {
		s := newBulkOnlySuite(t, `
		name: string @index(exact) .
	`, `
		_:a <name> "alice" .
		_:b <name> "alice" .
	`, "")
		return s
	}

	s := newLiveOnlySuite(t, `
		name: string @index(exact) .
	`, `
		_:a <name> "alice" .
		_:b <name> "alice" .
	`, "")
	return s
}

func testIndexedPredicate(t *testing.T) {
	t.Run("Count query", testCase(`
	{
		get_count(func: eq(name, "alice")) {
			count(uid)
		},
	}
	`, `
	{
		"get_count": [
			{ "count": 2 }
		]
	}
	`))
}

func countIndexSetup(t *testing.T, isBulkLoader bool) *bsuite {
	schema := `
		name: string @index(exact) .
		friend: [uid] @count @reverse .
	`

	rdfs := `
		_:alice <friend> _:bob   .
		_:alice <friend> _:carol .
		_:alice <friend> _:dave  .

		_:bob   <friend> _:carol .

		_:carol <friend> _:bob   .
		_:carol <friend> _:dave  .

		_:erin  <friend> _:bob   .
		_:erin  <friend> _:carol .

		_:frank <friend> _:carol .
		_:frank <friend> _:dave  .
		_:frank <friend> _:erin  .

		_:grace <friend> _:alice .
		_:grace <friend> _:bob   .
		_:grace <friend> _:carol .
		_:grace <friend> _:dave  .
		_:grace <friend> _:erin  .
		_:grace <friend> _:frank .

		_:alice <name> "Alice" .
		_:bob   <name> "Bob" .
		_:carol <name> "Carol" .
		_:erin  <name> "Erin" .
		_:frank <name> "Frank" .
		_:grace <name> "Grace" .
	`
	if isBulkLoader {
		s := newBulkOnlySuite(t, schema, rdfs, "")
		return s
	}

	s := newLiveOnlySuite(t, schema, rdfs, "")
	return s
}

func testCountIndex(t *testing.T) {
	t.Run("All queries", testCase(`
	{
		alice_friend_count(func: eq(name, "Alice")) {
			count(friend),
		},
		bob_friend_count(func: eq(name, "Bob")) {
			count(friend),
		},
		carol_friend_count(func: eq(name, "Carol")) {
			count(friend),
		},
		erin_friend_count(func: eq(name, "Erin")) {
			count(friend),
		},
		frank_friend_count(func: eq(name, "Frank")) {
			count(friend),
		},
		grace_friend_count(func: eq(name, "Grace")) {
			count(friend),
		},

		has_1_friend(func: has(friend)) @filter(eq(count(friend), 1)) {
			name
		},
		has_2_friends(func: has(friend)) @filter(eq(count(friend), 2)) {
			name
		},
		has_3_friends(func: has(friend)) @filter(eq(count(friend), 3)) {
			name
		},
		has_4_friends(func: has(friend)) @filter(eq(count(friend), 4)) {
			name
		},
		has_5_friends(func: has(friend)) @filter(eq(count(friend), 5)) {
			name
		},
		has_6_friends(func: has(friend)) @filter(eq(count(friend), 6)) {
			name
		},

		has_1_rev_friend(func: has(friend)) @filter(eq(count(~friend), 1)) {
			name
		},
		has_2_rev_friends(func: has(friend)) @filter(eq(count(~friend), 2)) {
			name
		},
		has_3_rev_friends(func: has(friend)) @filter(eq(count(~friend), 3)) {
			name
		},
		has_4_rev_friends(func: has(friend)) @filter(eq(count(~friend), 4)) {
			name
		},
		has_5_rev_friends(func: has(friend)) @filter(eq(count(~friend), 5)) {
			name
		},
		has_6_rev_friends(func: has(friend)) @filter(eq(count(~friend), 6)) {
			name
		},
	}
	`, `
	{
		"alice_friend_count": [
			{ "count(friend)": 3 }
		],
		"bob_friend_count": [
			{ "count(friend)": 1 }
		],
		"carol_friend_count": [
			{ "count(friend)": 2 }
		],
		"erin_friend_count": [
			{ "count(friend)": 2 }
		],
		"frank_friend_count": [
			{ "count(friend)": 3 }
		],
		"grace_friend_count": [
			{ "count(friend)": 6 }
		],

		"has_1_friend": [
			{ "name": "Bob" }
		],
		"has_2_friends": [
			{ "name": "Carol" },
			{ "name": "Erin" }
		],
		"has_3_friends": [
			{ "name": "Alice" },
			{ "name": "Frank" }
		],
		"has_4_friends": [],
		"has_5_friends": [],
		"has_6_friends": [
			{ "name": "Grace" }
		],

		"has_1_rev_friend": [
			{ "name": "Alice" },
			{ "name": "Frank" }
		],
		"has_2_rev_friends": [
			{ "name": "Erin" }
		],
		"has_3_rev_friends": [
		],
		"has_4_rev_friends": [
			{ "name": "Bob" }
		],
		"has_5_rev_friends": [
			{ "name": "Carol" }
		],
		"has_6_rev_friends": []
	}
	`))
}

func loadTypesSetup(t *testing.T, isBulkLoader bool) *bsuite {
	schema := `
		name: string .

		type Person {
			name
		}
	`

	rdfs := `
		_:alice <name> "Alice" .
	`
	if isBulkLoader {
		s := newBulkOnlySuite(t, schema, rdfs, "")
		return s
	}

	s := newLiveOnlySuite(t, schema, rdfs, "")
	return s
}

func testLoadTypes(t *testing.T) {
	t.Run("All queries", testCase("schema(type: Person) {}",
		`{"types":[{"name":"Person", "fields":[{"name":"name"}]}]}`))
}

func bulkSingleUidSetup(t *testing.T, isBulkLoader bool) *bsuite {
	schema := `
		name: string @index(exact) .
		friend: uid @count @reverse .
	`

	rdfs := `
		_:alice <friend> _:bob   .
		_:alice <friend> _:carol .
		_:alice <friend> _:dave  .

		_:bob   <friend> _:carol .

		_:carol <friend> _:bob   .
		_:carol <friend> _:dave  .

		_:erin  <friend> _:bob   .
		_:erin  <friend> _:carol .

		_:frank <friend> _:carol .
		_:frank <friend> _:dave  .
		_:frank <friend> _:erin  .

		_:grace <friend> _:alice .
		_:grace <friend> _:bob   .
		_:grace <friend> _:carol .
		_:grace <friend> _:dave  .
		_:grace <friend> _:erin  .
		_:grace <friend> _:frank .

		_:alice <name> "Alice" .
		_:bob   <name> "Bob" .
		_:carol <name> "Carol" .
		_:erin  <name> "Erin" .
		_:frank <name> "Frank" .
		_:grace <name> "Grace" .
	`
	if isBulkLoader {
		s := newBulkOnlySuite(t, schema, rdfs, "")
		return s
	}

	t.Fatalf("BulkSingleUids cant be run via live loader")
	return nil
}

// This test is similar to TestCount but the friend predicate is not a list. The bulk loader
// should detect this and force it to be a list to avoid any data loss. This test only runs
// in the bulk loader.
func testBulkSingleUid(t *testing.T) {
	t.Run("All queries", testCase(`
	{
		alice_friend_count(func: eq(name, "Alice")) {
			count(friend),
		},
		bob_friend_count(func: eq(name, "Bob")) {
			count(friend),
		},
		carol_friend_count(func: eq(name, "Carol")) {
			count(friend),
		},
		erin_friend_count(func: eq(name, "Erin")) {
			count(friend),
		},
		frank_friend_count(func: eq(name, "Frank")) {
			count(friend),
		},
		grace_friend_count(func: eq(name, "Grace")) {
			count(friend),
		},

		has_1_friend(func: has(friend)) @filter(eq(count(friend), 1)) {
			name
		},
		has_2_friends(func: has(friend)) @filter(eq(count(friend), 2)) {
			name
		},
		has_3_friends(func: has(friend)) @filter(eq(count(friend), 3)) {
			name
		},
		has_4_friends(func: has(friend)) @filter(eq(count(friend), 4)) {
			name
		},
		has_5_friends(func: has(friend)) @filter(eq(count(friend), 5)) {
			name
		},
		has_6_friends(func: has(friend)) @filter(eq(count(friend), 6)) {
			name
		},

		has_1_rev_friend(func: has(friend)) @filter(eq(count(~friend), 1)) {
			name
		},
		has_2_rev_friends(func: has(friend)) @filter(eq(count(~friend), 2)) {
			name
		},
		has_3_rev_friends(func: has(friend)) @filter(eq(count(~friend), 3)) {
			name
		},
		has_4_rev_friends(func: has(friend)) @filter(eq(count(~friend), 4)) {
			name
		},
		has_5_rev_friends(func: has(friend)) @filter(eq(count(~friend), 5)) {
			name
		},
		has_6_rev_friends(func: has(friend)) @filter(eq(count(~friend), 6)) {
			name
		},
	}
	`, `
	{
		"alice_friend_count": [
			{ "count(friend)": 3 }
		],
		"bob_friend_count": [
			{ "count(friend)": 1 }
		],
		"carol_friend_count": [
			{ "count(friend)": 2 }
		],
		"erin_friend_count": [
			{ "count(friend)": 2 }
		],
		"frank_friend_count": [
			{ "count(friend)": 3 }
		],
		"grace_friend_count": [
			{ "count(friend)": 6 }
		],

		"has_1_friend": [
			{ "name": "Bob" }
		],
		"has_2_friends": [
			{ "name": "Carol" },
			{ "name": "Erin" }
		],
		"has_3_friends": [
			{ "name": "Alice" },
			{ "name": "Frank" }
		],
		"has_4_friends": [],
		"has_5_friends": [],
		"has_6_friends": [
			{ "name": "Grace" }
		],

		"has_1_rev_friend": [
			{ "name": "Alice" },
			{ "name": "Frank" }
		],
		"has_2_rev_friends": [
			{ "name": "Erin" }
		],
		"has_3_rev_friends": [
		],
		"has_4_rev_friends": [
			{ "name": "Bob" }
		],
		"has_5_rev_friends": [
			{ "name": "Carol" }
		],
		"has_6_rev_friends": []
	}
	`))
}

func deleteEdgeWithStarSetup(t *testing.T, isBulkLoader bool) *bsuite {
	schema := `
		friend: [uid] .
	`

	rdfs := `
		<0x1> <friend> <0x2>   .
		<0x1> <friend> <0x3>   .

		<0x2> <name> "Alice" .
		<0x3> <name> "Bob" .
	`
	if isBulkLoader {
		s := newBulkOnlySuite(t, schema, rdfs, "")
		return s
	}

	t.Fatalf("TestDeleteEdgeWithStar cant be run via live loader")
	return nil
}

func testDeleteEdgeWithStar(t *testing.T) {
	client, err := testutil.DgraphClient(testutil.ContainerAddr("alpha1", 9080))
	require.NoError(t, err)
	_, err = client.NewTxn().Mutate(context.Background(), &api.Mutation{
		DelNquads: []byte(`<0x1> <friend> * .`),
		CommitNow: true,
	})
	require.NoError(t, err)

	t.Run("Get list of friends", testCase(`
	{
		me(func: uid(0x1)) {
			friend {
				name
			}
		}
	}`, `
		{
			"me": []
		}`))

}

func TestGqlSchema(t *testing.T) {
	s := newBulkOnlySuite(t, "", "", "abc")
	defer s.cleanup(t)

	t.Run("Get GraphQL schema", testCase(`
	{
		schema(func: has(dgraph.graphql.schema)) {
			dgraph.graphql.schema
			dgraph.graphql.xid
			dgraph.type
		}
	}`, `
		{
			"schema": [{
				"dgraph.graphql.schema": "abc",
				"dgraph.graphql.xid": "dgraph.graphql.schema",
				"dgraph.type": ["dgraph.graphql"]
			}]
		}`))

}

func TestGoldenData(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	s := newSuiteFromFile(t,
		os.ExpandEnv("$GOPATH/src/github.com/hypermodeinc/dgraph/v25/systest/data/goldendata.schema"),
		os.ExpandEnv("$GOPATH/src/github.com/hypermodeinc/dgraph/v25/systest/data/goldendata.rdf.gz"),
		"",
	)
	defer s.cleanup(t)

	err := matchExportCount(matchExport{
		expectedRDF:    1120879,
		expectedSchema: 10,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("basic", testCase(`
		{pj_films(func:allofterms(name@en,"Peter Jackson")) {
			director.film (orderasc: name@en, first: 10) {
				name@en
			}
		}}
	`, `
		{"pj_films": [ { "director.film": [
			{ "name@en": "Bad Taste" },
			{ "name@en": "Heavenly Creatures" },
			{ "name@en": "Forgotten Silver" },
			{ "name@en": "Dead Alive" },
			{ "name@en": "The Adventures of Tintin: Prisoners of the Sun" },
			{ "name@en": "Crossing the Line" },
			{ "name@en": "Meet the Feebles" },
			{ "name@en": "King Kong" },
			{ "name@en": "The Frighteners" },
			{ "name@en": "Gollum's Acceptance Speech" }
		] } ]}
	`))

	// TODO: Add the test cases from contrib/goldendata-queries.sh The tests
	// there use grep to find the number of leaf nodes in the queries, so the
	// queries will have to be modified.

	// TODO: Add tests similar to those in
	// https://dgraph.io/docs/query-language/. These test most of the main
	// functionality of dgraph.
}

type matchExport struct {
	expectedRDF    int
	expectedSchema int
	dir            string
	port           int
}

func matchExportCount(opts matchExport) error {
	// Now try and export data from second server.
	adminUrl := fmt.Sprintf("http://localhost:%d/admin", opts.port)
	params := testutil.GraphQLParams{
		Query: testutil.ExportRequest,
	}
	b, err := json.Marshal(params)
	if err != nil {
		return err
	}
	resp, err := http.Post(adminUrl, "application/json", bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	defer func() {
		if err = resp.Body.Close(); err != nil {
			glog.Errorf("Error while closing response body: %v", err)
		}
	}()

	b, err = io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	expected := `{"code": "Success", "message": "Export completed."}`
	if string(b) != expected {
		return errors.Errorf("Unexpected message while exporting: %v", string(b))
	}

	dataFile, err := findFile(filepath.Join(opts.dir, "export"), ".rdf.gz")
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf("gunzip -c %s | wc -l", dataFile)
	out, err := exec.Command("sh", "-c", cmd).Output()
	if err != nil {
		return err
	}
	count := strings.TrimSpace(string(out))
	if count != strconv.Itoa(opts.expectedRDF) {
		return errors.Errorf("Export count mismatch. Got: %s", count)
	}

	schemaFile, err := findFile(filepath.Join(opts.dir, "export"), ".schema.gz")
	if err != nil {
		return err
	}
	cmd = fmt.Sprintf("gunzip -c %s | wc -l", schemaFile)
	out, err = exec.Command("sh", "-c", cmd).Output()
	if err != nil {
		return err
	}
	count = strings.TrimSpace(string(out))
	if count != strconv.Itoa(opts.expectedSchema) {
		return errors.Errorf("Schema export count mismatch. Got: %s", count)
	}
	glog.Infoln("Export count matched.")
	return nil
}

func findFile(dir string, ext string) (string, error) {
	var fp string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(path, ext) {
			fp = path
			return nil
		}
		return nil
	})
	return fp, err
}
