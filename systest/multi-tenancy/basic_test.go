//go:build integration || upgrade

/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
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

package main

import (
	"context"
	"fmt"
	//"net/http"
	"os"
	//"path/filepath"
	"testing"
	"time"
	"strings"
	"encoding/json"

	"github.com/stretchr/testify/suite"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/dgraph-io/dgo/v210"
	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/ee/acl"
	//"github.com/dgraph-io/dgraph/graphql/e2e/common"
	//"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
)

type Rule struct {
	Predicate  string `json:"predicate"`
	Permission int32  `json:"permission"`
}

type MultitenancyTestSuite struct {
	suite.Suite
	lc *dgraphtest.LocalCluster
	client *dgo.Dgraph
}

func (suite *MultitenancyTestSuite) SetupSuite() {
	fmt.Println("*** SetupSuite Start ***")

	os.Setenv("GOOS", "linux")
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(3).WithNumZeros(3).WithReplicas(3).
		WithACL(20 * time.Second).WithEncryption().WithVersion("0c9f60156")

	var err error
	suite.lc, err = dgraphtest.NewLocalCluster(conf)
	x.Panic(err)
	suite.lc.Start()

	suite.client, err = suite.lc.Client()
	x.Panic(err)
}

func (suite *MultitenancyTestSuite) TearDownSuite() {
	suite.lc.Cleanup()

	fmt.Println("*** TearDownSuite End ***")
}

func (suite *MultitenancyTestSuite) SetupTest() {
	fmt.Println("### SetupTest Start ###")
}

func (suite *MultitenancyTestSuite) TearDownTest() {
	fmt.Println("### TearDownTest End ###")
}

type inputTripletsCount struct {
	lowerLimit int
	upperLimit int
}

//func (suite *MultitenancyTestSuite) prepare(t *testing.T) {
func (suite *MultitenancyTestSuite) prepare() {
	suite.client = suite.DgClientWithLogin("groot", "password", x.GalaxyNamespace)
	dc := suite.client
	t := suite.T()
	require.NoError(t, dc.Alter(context.Background(), &api.Operation{DropAll: true}))
}

var timeout = 5 * time.Second

// TODO(Ahsan): This is just a basic test, for the purpose of development. The functions used in
// this file can me made common to the other acl tests as well. Needs some refactoring as well.
//func (suite *MultitenancyTestSuite) TestAclBasic(t *testing.T) {
func (suite *MultitenancyTestSuite) TestAclBasic() {
	suite.prepare()
/*
	galaxyToken, err := testutil.Login(t,
		&testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: x.GalaxyNamespace})

	require.NotNil(t, galaxyToken, "galaxy token is nil")
	require.NoError(t, err, "login failed")
*/
	suite.client = suite.DgClientWithLogin("groot", "password", x.GalaxyNamespace)
	// Create a new namespace
	ns, err := suite.CreateNamespaceWithRetry()
	t := suite.T()
	require.NoError(t, err)
	require.Greater(t, int(ns), 0)

	// Add some data to namespace 1
	suite.client = suite.DgClientWithLogin("groot", "password", ns)
	suite.AddData()

	query := `
		{
			me(func: has(name)) {
				nickname
				name
			}
		}
	`
	resp := suite.QueryData(query)
	testutil.CompareJSON(t,
		`{"me": [{"name":"guy1","nickname":"RG"},
		{"name": "guy2", "nickname":"RG2"}]}`,
		string(resp))

	// groot of namespace 0 should not see the data of namespace-1
	suite.client = suite.DgClientWithLogin("groot", "password", 0)
	resp = suite.QueryData(query)
	testutil.CompareJSON(t, `{"me": []}`, string(resp))

	// Login to namespace 1 via groot and create new user alice.
	// token, err := testutil.Login(t, &testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: ns})
	suite.client = suite.DgClientWithLogin("groot", "password", ns)
	require.NoError(t, err, "login failed")
	suite.CreateUser("alice", "newpassword")

	// Alice should not be able to see data added by groot in namespace 1
	suite.client = suite.DgClientWithLogin("alice", "newpassword", ns)
	t.Logf("After ALICE AND NEWPASSWORD")
	resp = suite.QueryData(query)
	testutil.CompareJSON(t, `{}`, string(resp))

	// Create a new group, add alice to that group and give read access of <name> to dev group.
	suite.CreateGroup("dev")
	suite.AddToGroup("alice", "dev")
	suite.AddRulesToGroup("dev",
		[]Rule{{Predicate: "name", Permission: acl.Read.Code}}, true)

	// Now alice should see the name predicate but not nickname.
	suite.client = suite.DgClientWithLogin("alice", "newpassword", ns)
	testutil.PollTillPassOrTimeout(t, suite.client, query, `{"me": [{"name":"guy1"},{"name": "guy2"}]}`, timeout)
}

/*
func createGroupAndSetPermissions(t *testing.T, namespace uint64, group, user, predicate string) {
	token, err := testutil.Login(t,
		&testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: namespace})
	require.NoError(t, err, "login failed")
	testutil.CreateGroup(t, token, group)
	testutil.AddToGroup(t, token, user, group)
	testutil.AddRulesToGroup(t, token, group,
		[]testutil.Rule{{Predicate: predicate, Permission: acl.Read.Code}}, true)
}

func TestTwoPermissionSetsInNameSpacesWithAcl(t *testing.T) {
	prepare(t)
	galaxyToken, err := testutil.Login(t,
		&testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: x.GalaxyNamespace})
	require.NoError(t, err, "login failed")
	query := `
		{
			me(func: has(name)) {
				nickname
				name
			}
		}
	`
	// Create first namespace
	ns1, err := testutil.CreateNamespaceWithRetry(t, galaxyToken)
	require.NoError(t, err)

	// Add data
	dc := testutil.DgClientWithLogin(t, "groot", "password", ns1)
	testutil.AddData(t, dc)

	// Create user alice
	token1, err := testutil.Login(t,
		&testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: ns1})
	require.NoError(t, err, "login failed")
	testutil.CreateUser(t, token1, "alice", "newpassword")

	// Create a new group, add alice to that group and give read access of <name> to dev group.
	createGroupAndSetPermissions(t, ns1, "dev", "alice", "name")

	// Alice should not be able to see <nickname> in namespace 1
	dc = testutil.DgClientWithLogin(t, "alice", "newpassword", ns1)
	testutil.PollTillPassOrTimeout(t, dc, query, `{"me": [{"name":"guy2"}, {"name":"guy1"}]}`, timeout)

	// Create second namespace
	ns2, err := testutil.CreateNamespaceWithRetry(t, galaxyToken)
	require.NoError(t, err)

	// Add data
	dc = testutil.DgClientWithLogin(t, "groot", "password", ns2)
	testutil.AddData(t, dc)

	// Create user bob
	token2, err := testutil.Login(t,
		&testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: ns2})
	require.NoError(t, err, "login failed")
	testutil.CreateUser(t, token2, "bob", "newpassword")

	// Create a new group, add bob to that group and give read access of <nickname> to dev group.
	createGroupAndSetPermissions(t, ns2, "dev", "bob", "nickname")

	// Query via bob and check result
	dc = testutil.DgClientWithLogin(t, "bob", "newpassword", ns2)
	testutil.PollTillPassOrTimeout(t, dc, query, `{}`, timeout)

	// Query namespace-1 via alice and check result to ensure it still works
	dc = testutil.DgClientWithLogin(t, "alice", "newpassword", ns1)
	resp := testutil.QueryData(t, dc, query)
	testutil.CompareJSON(t, `{"me": [{"name":"guy2"}, {"name":"guy1"}]}`, string(resp))

	// Change permissions in namespace-2
	token, err := testutil.Login(t,
		&testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: ns2})
	require.NoError(t, err, "login failed")
	testutil.AddRulesToGroup(t, token, "dev",
		[]testutil.Rule{{Predicate: "name", Permission: acl.Read.Code}}, false)

	// Query namespace-2
	dc = testutil.DgClientWithLogin(t, "bob", "newpassword", ns2)
	testutil.PollTillPassOrTimeout(t, dc, query,
		`{"me": [{"name":"guy2", "nickname": "RG2"}, {"name":"guy1", "nickname": "RG"}]}`, timeout)

	// Query namespace-1
	dc = testutil.DgClientWithLogin(t, "alice", "newpassword", ns1)
	resp = testutil.QueryData(t, dc, query)
	testutil.CompareJSON(t, `{"me": [{"name":"guy2"}, {"name":"guy1"}]}`, string(resp))
}

func TestCreateNamespace(t *testing.T) {
	prepare(t)
	galaxyToken, err := testutil.Login(t,
		&testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: x.GalaxyNamespace})
	require.NoError(t, err, "login failed")
	// Create a new namespace
	ns, err := testutil.CreateNamespaceWithRetry(t, galaxyToken)
	require.NoError(t, err)

	token, err := testutil.Login(t,
		&testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: ns})
	require.NoError(t, err, "login failed")
	// Create a new namespace using guardian of other namespace.
	_, err = testutil.CreateNamespaceWithRetry(t, token)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only guardian of galaxy is allowed to do this operation")
}

func TestResetPassword(t *testing.T) {
	prepare(t)

	galaxyToken, err := testutil.Login(t,
		&testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: x.GalaxyNamespace})
	require.NoError(t, err, "login failed")
	// Create a new namespace
	ns, err := testutil.CreateNamespaceWithRetry(t, galaxyToken)
	require.NoError(t, err)

	// Reset Password
	_, err = testutil.ResetPassword(t, galaxyToken, "groot", "newpassword", ns)
	require.NoError(t, err)

	// Try and Fail with old password for groot
	token, err := testutil.Login(t,
		&testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: ns})

	require.Error(t, err, "expected error because incorrect login")
	require.Nil(t, token, "nil token because incorrect login")

	// Try and success with new password for groot
	token, err = testutil.Login(t,
		&testutil.LoginParams{UserID: "groot", Passwd: "newpassword", Namespace: ns})
	require.NoError(t, err, "login failed")
	require.Equal(t, token.Password, "newpassword", "new password matches the reset password")
}

func TestDeleteNamespace(t *testing.T) {
	prepare(t)
	galaxyToken, err := testutil.Login(t,
		&testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: x.GalaxyNamespace})
	require.NoError(t, err, "login failed")
	dg := make(map[uint64]*dgo.Dgraph)
	dg[x.GalaxyNamespace] = testutil.DgClientWithLogin(t, "groot", "password", x.GalaxyNamespace)
	// Create a new namespace
	ns, err := testutil.CreateNamespaceWithRetry(t, galaxyToken)
	require.NoError(t, err)
	dg[ns] = testutil.DgClientWithLogin(t, "groot", "password", ns)

	addData := func(ns uint64) error {
		mutation := &api.Mutation{
			SetNquads: []byte(fmt.Sprintf(`
			_:a <name> "%d" .
		`, ns)),
			CommitNow: true,
		}
		_, err := dg[ns].NewTxn().Mutate(context.Background(), mutation)
		return err
	}
	check := func(ns uint64, expected string) {
		query := `
		{
			me(func: has(name)) {
				name
			}
		}
	`
		resp := testutil.QueryData(t, dg[ns], query)
		testutil.CompareJSON(t, expected, string(resp))
	}

	err = addData(x.GalaxyNamespace)
	require.NoError(t, err)
	check(x.GalaxyNamespace, `{"me": [{"name":"0"}]}`)
	err = addData(ns)
	require.NoError(t, err)
	check(ns, fmt.Sprintf(`{"me": [{"name":"%d"}]}`, ns))

	require.NoError(t, testutil.DeleteNamespace(t, galaxyToken, ns))

	err = addData(x.GalaxyNamespace)
	require.NoError(t, err)
	check(x.GalaxyNamespace, `{"me": [{"name":"0"}, {"name":"0"}]}`)
	err = addData(ns)
	require.Contains(t, err.Error(), "Key is using the banned prefix")
	check(ns, `{"me": []}`)

	// No one should be able to delete the default namespace. Not even guardian of galaxy.
	err = testutil.DeleteNamespace(t, galaxyToken, x.GalaxyNamespace)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Cannot delete default namespace")
}

type liveOpts struct {
	rdfs      string
	schema    string
	gqlSchema string
	creds     *testutil.LoginParams
	forceNs   int64
}

func liveLoadData(t *testing.T, opts *liveOpts) error {
	// Prepare directories.
	dir, err := os.MkdirTemp("", "multi")
	require.NoError(t, err)
	defer func() {
		os.RemoveAll(dir)
	}()
	rdfFile := filepath.Join(dir, "rdfs.rdf")
	require.NoError(t, os.WriteFile(rdfFile, []byte(opts.rdfs), 0644))
	schemaFile := filepath.Join(dir, "schema.txt")
	require.NoError(t, os.WriteFile(schemaFile, []byte(opts.schema), 0644))
	gqlSchemaFile := filepath.Join(dir, "gql_schema.txt")
	require.NoError(t, os.WriteFile(gqlSchemaFile, []byte(opts.gqlSchema), 0644))
	// Load the data.
	return testutil.LiveLoad(testutil.LiveOpts{
		Zero:       testutil.ContainerAddr("zero1", 5080),
		Alpha:      testutil.ContainerAddr("alpha1", 9080),
		RdfFile:    rdfFile,
		SchemaFile: schemaFile,
		Creds:      opts.creds,
		ForceNs:    opts.forceNs,
	})
}

func TestLiveLoadMulti(t *testing.T) {
	prepare(t)
	dc0 := testutil.DgClientWithLogin(t, "groot", "password", x.GalaxyNamespace)
	galaxyCreds := &testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: x.GalaxyNamespace}
	galaxyToken, err := testutil.Login(t, galaxyCreds)
	require.NoError(t, err, "login failed")
	// Create a new namespace
	ns, err := testutil.CreateNamespaceWithRetry(t, galaxyToken)
	require.NoError(t, err)
	dc1 := testutil.DgClientWithLogin(t, "groot", "password", ns)

	// Load data.
	require.NoError(t, liveLoadData(t, &liveOpts{
		rdfs: fmt.Sprintf(`
		_:a <name> "galaxy alice" .
		_:b <name> "galaxy bob" .
		_:a <name> "ns alice" <%#x> .
		_:b <name> "ns bob" <%#x> .
`, ns, ns),
		schema: fmt.Sprintf(`
		name: string @index(term) .
		[%#x] name: string .
`, ns),
		creds:   galaxyCreds,
		forceNs: -1,
	}))

	query1 := `
		{
			me(func: has(name)) {
				name
			}
		}
	`
	query2 := `
		{
			me(func: anyofterms(name, "galaxy")) {
				name
			}
		}
	`
	query3 := `
		{
			me(func: anyofterms(name, "ns")) {
				name
			}
		}
	`

	resp := testutil.QueryData(t, dc0, query1)
	testutil.CompareJSON(t,
		`{"me": [{"name":"galaxy alice"}, {"name": "galaxy bob"}]}`, string(resp))
	resp = testutil.QueryData(t, dc1, query1)
	testutil.CompareJSON(t,
		`{"me": [{"name":"ns alice"}, {"name": "ns bob"}]}`, string(resp))

	resp = testutil.QueryData(t, dc0, query2)
	testutil.CompareJSON(t,
		`{"me": [{"name":"galaxy alice"}, {"name": "galaxy bob"}]}`, string(resp))

	_, err = dc1.NewReadOnlyTxn().Query(context.Background(), query3)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Attribute name is not indexed")

	// live load data into namespace ns using the guardian of galaxy.
	require.NoError(t, liveLoadData(t, &liveOpts{
		rdfs: fmt.Sprintf(`
		_:a <name> "ns chew" .
		_:b <name> "ns dan" <%#x> .
		_:c <name> "ns eon" <%#x> .
`, ns, 0x100),
		schema: `
		name: string @index(term) .
`,
		creds:   galaxyCreds,
		forceNs: int64(ns),
	}))

	resp = testutil.QueryData(t, dc1, query3)
	testutil.CompareJSON(t,
		`{"me": [{"name":"ns alice"}, {"name": "ns bob"},{"name":"ns chew"},
		{"name": "ns dan"},{"name":"ns eon"}]}`, string(resp))

	// Try loading data into a namespace that does not exist. Expect a failure.
	err = liveLoadData(t, &liveOpts{
		rdfs:   fmt.Sprintf(`_:c <name> "ns eon" <%#x> .`, ns),
		schema: `name: string @index(term) .`,
		creds: &testutil.LoginParams{UserID: "groot", Passwd: "password",
			Namespace: x.GalaxyNamespace},
		forceNs: int64(0x123456), // Assuming this namespace does not exist.
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Cannot load into namespace 0x123456")

	// Try loading into a multiple namespaces.
	err = liveLoadData(t, &liveOpts{
		rdfs:    fmt.Sprintf(`_:c <name> "ns eon" <%#x> .`, ns),
		schema:  `[0x123456] name: string @index(term) .`,
		creds:   galaxyCreds,
		forceNs: -1,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Namespace 0x123456 doesn't exist for pred")

	err = liveLoadData(t, &liveOpts{
		rdfs:    `_:c <name> "ns eon" <0x123456> .`,
		schema:  `name: string @index(term) .`,
		creds:   galaxyCreds,
		forceNs: -1,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Cannot load nquad")

	// Load data by non-galaxy user.
	err = liveLoadData(t, &liveOpts{
		rdfs: `_:c <name> "ns hola" .`,
		schema: `
		name: string @index(term) .
`,
		creds:   &testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: ns},
		forceNs: -1,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot force namespace")

	err = liveLoadData(t, &liveOpts{
		rdfs: `_:c <name> "ns hola" .`,
		schema: `
		name: string @index(term) .
`,
		creds:   &testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: ns},
		forceNs: 10,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot force namespace")

	require.NoError(t, liveLoadData(t, &liveOpts{
		rdfs: fmt.Sprintf(`
		_:a <name> "ns free" .
		_:b <name> "ns gary" <%#x> .
		_:c <name> "ns hola" <%#x> .
`, ns, 0x100),
		schema: `
		name: string @index(term) .
`,
		creds: &testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: ns},
	}))

	resp = testutil.QueryData(t, dc1, query3)
	testutil.CompareJSON(t, `{"me": [{"name":"ns alice"}, {"name": "ns bob"},{"name":"ns chew"},
		{"name": "ns dan"},{"name":"ns eon"}, {"name": "ns free"},{"name":"ns gary"},
		{"name": "ns hola"}]}`, string(resp))
}

func postGqlSchema(t *testing.T, schema string, accessJwt string) {
	groupOneHTTP := testutil.ContainerAddr("alpha1", 8080)
	header := http.Header{}
	header.Set("X-Dgraph-AccessToken", accessJwt)
	common.SafelyUpdateGQLSchema(t, groupOneHTTP, schema, header)
}

func postPersistentQuery(t *testing.T, query, sha, accessJwt string) *common.GraphQLResponse {
	header := http.Header{}
	header.Set("X-Dgraph-AccessToken", accessJwt)
	queryCountryParams := &common.GraphQLParams{
		Query: query,
		Extensions: &schema.RequestExtensions{PersistedQuery: schema.PersistedQuery{
			Sha256Hash: sha,
		}},
		Headers: header,
	}
	url := "http://" + testutil.ContainerAddr("alpha1", 8080) + "/graphql"
	return queryCountryParams.ExecuteAsPost(t, url)
}

func TestPersistentQuery(t *testing.T) {
	prepare(t)
	galaxyToken, err := testutil.Login(t,
		&testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: x.GalaxyNamespace})
	require.NoError(t, err, "login failed")
	// Create a new namespace
	ns, err := testutil.CreateNamespaceWithRetry(t, galaxyToken)
	require.NoError(t, err)

	token, err := testutil.Login(t,
		&testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: ns})
	require.NoError(t, err, "login failed")
	sch := `type Product {
			productID: ID!
			name: String @search(by: [term])
		}`
	postGqlSchema(t, sch, galaxyToken.AccessJwt)
	postGqlSchema(t, sch, token.AccessJwt)

	p1 := "query {queryProduct{productID}}"
	sha1 := "7a8ff7a69169371c1eb52a8921387079ca281bb2d55feb4b535cbf0ab3896be5"
	resp := postPersistentQuery(t, p1, sha1, galaxyToken.AccessJwt)
	common.RequireNoGQLErrors(t, resp)

	p2 := "query {queryProduct{name}}"
	sha2 := "0efcdde144167b1046360b73c7f6bec325d9f555099a2ae9b820a13328d270e4"
	resp = postPersistentQuery(t, p2, sha2, token.AccessJwt)
	common.RequireNoGQLErrors(t, resp)

	// User cannnot see persistent query from other namespace.
	resp = postPersistentQuery(t, "", sha2, galaxyToken.AccessJwt)
	require.Equal(t, 1, len(resp.Errors))
	require.Contains(t, resp.Errors[0].Message, "PersistedQueryNotFound")

	resp = postPersistentQuery(t, "", sha1, token.AccessJwt)
	require.Equal(t, 1, len(resp.Errors))
	require.Contains(t, resp.Errors[0].Message, "PersistedQueryNotFound")

	resp = postPersistentQuery(t, "", sha1, "")
	require.Equal(t, 1, len(resp.Errors))
	require.Contains(t, resp.Errors[0].Message, "no accessJwt available")
}

func TestTokenExpired(t *testing.T) {
	prepare(t)
	galaxyToken, err := testutil.Login(t,
		&testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: x.GalaxyNamespace})
	require.NoError(t, err, "login failed")

	// Create a new namespace
	ns, err := testutil.CreateNamespaceWithRetry(t, galaxyToken)
	require.NoError(t, err)
	token, err := testutil.Login(t,
		&testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: ns})
	require.NoError(t, err, "login failed")

	// Relogin using refresh JWT.
	token, err = testutil.Login(t,
		&testutil.LoginParams{RefreshJwt: token.RefreshToken})
	require.NoError(t, err, "login failed")

	_, err = testutil.CreateNamespaceWithRetry(t, token)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Only guardian of galaxy is allowed to do this operation")
}

func TestNameSpaceLimitFlag(t *testing.T) {
	testInputs := []inputTripletsCount{{1, 53}, {60, 100}, {141, 153}}
	galaxyToken, err := testutil.Login(t,
		&testutil.LoginParams{UserID: "groot", Passwd: "password", Namespace: x.GalaxyNamespace})
	require.NoError(t, err)
	// Create a new namespace
	ns, err := testutil.CreateNamespaceWithRetry(t, galaxyToken)
	require.NoError(t, err)
	dc := testutil.DgClientWithLogin(t, "groot", "password", ns)
	require.NoError(t, dc.Alter(context.Background(), &api.Operation{
		Schema: `name: string .`}))
	// trying to load more triplets than allowed,It should return error.
	_, err = testutil.AddNumberOfTriples(t, dc, testInputs[0].lowerLimit, testInputs[0].upperLimit)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Requested UID lease(53) is greater than allowed(50).")
	_, err = testutil.AddNumberOfTriples(t, dc, testInputs[1].lowerLimit, testInputs[1].upperLimit)
	require.NoError(t, err)
	// we have set uid-lease=50 so we are trying lease more uids,it should return error.
	_, err = testutil.AddNumberOfTriples(t, dc, testInputs[2].lowerLimit, testInputs[2].upperLimit)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Cannot lease UID because UID lease for the namespace")
}
*/

func (suite *MultitenancyTestSuite) DgClientWithLogin(id, password string, ns uint64) *dgo.Dgraph {
	t := suite.T()
	//userClient, err := c.Client()
	userClient := suite.client
	t.Logf("1. userClient = %+v", userClient)

	t.Logf("2. userClient = %+v", userClient)
	require.NoError(t, x.RetryUntilSuccess(10, 100*time.Millisecond, func() error {
	t.Logf("3. id = %+v\tpassword = %+v\tns = %+v\tuserClient = %+v", id, password, ns, userClient)
		return userClient.LoginIntoNamespace(context.Background(), id, password, ns) 
	})) 
	t.Logf("4. userClient = %+v", userClient)
	return userClient
}

func (suite *MultitenancyTestSuite) CreateNamespaceWithRetry() (uint64, error) {
	createNs := `mutation {
					 addNamespace
					  {   
					    namespaceId
					    message
					  }   
					}`  

	//params := GraphQLParams{
	params := dgraphtest.GraphQLParams{
		Query: createNs,
	}
	resp, err := dgraphtest.RunAdminQuery(suite.lc, params)
	if err != nil {
		return 0, err 
	}   

	var result struct {
		AddNamespace struct {
			NamespaceId int    `json:"namespaceId"`
			Message     string `json:"message"`
		}   
	}   
	if err := json.Unmarshal(resp, &result); err != nil {
		return 0, errors.Wrap(err, "error unmarshalling CreateNamespaceWithRetry() response")
	}   
	if strings.Contains(result.AddNamespace.Message, "Created namespace successfully") {
		return uint64(result.AddNamespace.NamespaceId), nil
	}
	return 0, errors.New(result.AddNamespace.Message)
}

func (suite *MultitenancyTestSuite) AddData() {
		mutation := &api.Mutation{
		SetNquads: []byte(`
			_:a <name> "guy1" .
			_:a <nickname> "RG" .
			_:b <name> "guy2" .
			_:b <nickname> "RG2" .
		`),  
		CommitNow: true,
	}
	_, err := suite.client.NewTxn().Mutate(context.Background(), mutation)
	t := suite.T()
	require.NoError(t, err)
}

func (suite *MultitenancyTestSuite) QueryData(query string) []byte {
	resp, err := suite.client.NewReadOnlyTxn().Query(context.Background(), query)
	t := suite.T()
	require.NoError(t, err)
	return resp.GetJson()
}

func (suite *MultitenancyTestSuite) CreateUser(username, password string) []byte {
	t := suite.T()
	addUser := `
	mutation addUser($name: String!, $pass: String!) {
		addUser(input: [{name: $name, password: $pass}]) {
			user {
				name
			}
		}
	}`

	params := dgraphtest.GraphQLParams{
		Query: addUser,
		Variables: map[string]interface{}{
			"name": username,
			"pass": password,
		},
	}
	resp, err := dgraphtest.RunAdminQuery(suite.lc, params)
	if err != nil {
		t.Log("Error not NIL")
		return nil
	}

	type Response struct {
		AddUser struct {
			User []struct {
				Name string
			}
		}
	}
	var r Response
	err = json.Unmarshal(resp, &r)
	require.NoError(t, err)
	t.Logf("RESPONSE = %+v", r)
	return resp
}

func (suite *MultitenancyTestSuite) CreateGroup(name string) {
	addGroup := `
	mutation addGroup($name: String!) {
		addGroup(input: [{name: $name}]) {
			group {
				name
			}
		}
	}`

	params := dgraphtest.GraphQLParams{
		Query: addGroup,
		Variables: map[string]interface{}{
			"name": name,
		},
	}
	resp, err := dgraphtest.RunAdminQuery(suite.lc, params)
	t := suite.T()
	require.NoError(t, err)

	//resp.RequireNoGraphQLErrors(t)
	type Response struct {
		AddGroup struct {
			Group []struct {
				Name string
			}
		}
	}
	var r Response
	err = json.Unmarshal(resp, &r)
	require.NoError(t, err)
}

func (suite *MultitenancyTestSuite) AddToGroup(userName, group string) {
	addUserToGroup := `mutation updateUser($name: String!, $group: String!) {
		updateUser(input: {
			filter: {
				name: {
					eq: $name
				}
			},
			set: {
				groups: [
					{ name: $group }
				]
			}
		}) {
			user {
				name
				groups {
					name
				}
			}
		}
	}`

	params := dgraphtest.GraphQLParams{
		Query: addUserToGroup,
		Variables: map[string]interface{}{
			"name":  userName,
			"group": group,
		},
	}
	resp, err := dgraphtest.RunAdminQuery(suite.lc, params)
	t := suite.T()
	require.NoError(t, err)

	var result struct {
		UpdateUser struct {
			User []struct {
				Name   string
				Groups []struct {
					Name string
				}
			}
			Name string
		}
	}
	err = json.Unmarshal(resp, &result)
	require.NoError(t, err)

	// There should be a user in response.
	require.Len(t, result.UpdateUser.User, 1)
	// User's name must be <userName>
	require.Equal(t, userName, result.UpdateUser.User[0].Name)

	var foundGroup bool
	for _, usr := range result.UpdateUser.User {
		for _, grp := range usr.Groups {
			if grp.Name == group {
				foundGroup = true
				break
			}
		}
	}
	require.True(t, foundGroup)
}

func (suite *MultitenancyTestSuite) AddRulesToGroup(group string, rules []Rule, newGroup bool) {
	addRuleToGroup := `mutation updateGroup($name: String!, $rules: [RuleRef!]!) {
		updateGroup(input: {
			filter: {
				name: {
					eq: $name
				}
			},
			set: {
				rules: $rules
			}
		}) {
			group {
				name
				rules {
					predicate
					permission
				}
			}
		}
	}`

	params := dgraphtest.GraphQLParams{
		Query: addRuleToGroup,
		Variables: map[string]interface{}{
			"name":  group,
			"rules": rules,
		},
	}
	resp, err := dgraphtest.RunAdminQuery(suite.lc, params)
	t := suite.T()
	require.NoError(t, err)

	rulesb, err := json.Marshal(rules)
	require.NoError(t, err)
	expectedOutput := fmt.Sprintf(`{
		  "updateGroup": {
			"group": [
			  {
				"name": "%s",
				"rules": %s
			  }
			]
		  }
	  }`, group, rulesb)
	if newGroup {
		testutil.CompareJSON(t, expectedOutput, string(resp))
	}
}

func TestMultitenancyTestSuite(t *testing.T) {
	suite.Run(t, new(MultitenancyTestSuite))
}

/*
func TestMain(m *testing.M) {
	fmt.Printf("Using adminEndpoint : %s for multi-tenancy test.\n", testutil.AdminUrl())
	_ = m.Run()
}
*/
