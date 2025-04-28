//go:build integration

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package acl

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/hypermodeinc/dgraph/v25/dgraphapi"
	"github.com/hypermodeinc/dgraph/v25/x"
)

func (asuite *AclTestSuite) TestInvalidGetUser() {
	t := asuite.T()
	hc, err := asuite.dc.HTTPClient()
	require.NoError(t, err)
	hc.HttpToken.AccessJwt = "invalid Token"
	currentUser, err := hc.GetCurrentUser()
	require.Contains(t, err.Error(), "couldn't rewrite query getCurrentUser because "+
		"unable to parse jwt token: token is malformed: token contains an invalid number of segments")
	require.Equal(t, "", currentUser)
}

func (asuite *AclTestSuite) TestPasswordReturn() {
	t := asuite.T()
	hc, err := asuite.dc.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser,
		dgraphapi.DefaultPassword, x.RootNamespace))
	query := dgraphapi.GraphQLParams{
		Query: `
	query {
		getCurrentUser {
			name
			password
		}
	}`}

	_, err = hc.RunGraphqlQuery(query, true)
	require.Contains(t, err.Error(),
		`Cannot query field "password" on type "User". (Locations: [{Line: 5, Column: 4}])`)
}

func (asuite *AclTestSuite) TestHealthForAcl() {
	t := asuite.T()
	hc, err := asuite.dc.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser,
		dgraphapi.DefaultPassword, x.RootNamespace))

	resetUser(t, hc)
	require.NoError(t, hc.LoginIntoNamespace(userid, userpassword, x.RootNamespace))
	gqlResp, err := hc.HealthForInstance()
	require.Error(t, err)
	// assert errors for non-guardians
	assertNonGuardianFailure(t, "health", false, gqlResp, err)

	// assert data for guardians
	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser,
		dgraphapi.DefaultPassword, x.RootNamespace))

	resp, err := hc.HealthForInstance()
	require.NoError(t, err, "health request failed")

	var guardianResp struct {
		Health []struct {
			Instance string
			Address  string
			LastEcho int64
			Status   string
			Version  string
			UpTime   int64
			Group    string
		}
	}
	require.NoError(t, json.Unmarshal(resp, &guardianResp))

	// we have 9 instances of alphas/zeros in teamcity environment
	require.Len(t, guardianResp.Health, 9)
	for _, v := range guardianResp.Health {
		t.Logf("Got health: %+v\n", v)
		require.Contains(t, []string{"alpha", "zero"}, v.Instance)
		require.NotEmpty(t, v.Address)
		require.NotEmpty(t, v.LastEcho)
		require.Equal(t, "healthy", v.Status)
		require.NotEmpty(t, v.Version)
		require.NotEmpty(t, v.UpTime)
		require.NotEmpty(t, v.Group)
	}
}

func (asuite *AclTestSuite) TestGuardianOnlyAccessForAdminEndpoints() {
	t := asuite.T()
	tcases := []graphQLAdminEndpointTestCase{
		{
			name: "backup has guardian auth",
			query: `
					mutation {
					  backup(input: {destination: ""}) {
						response {
						  code
						  message
						}
					  }
					}`,
			queryName:          "backup",
			testGuardianAccess: true,
			guardianErr:        "you must specify a 'destination' value",
			guardianData:       `{"backup": null}`,
		},
		{
			name: "listBackups has guardian auth",
			query: `
					query {
					  listBackups(input: {location: ""}) {
					  	backupId
					  }
					}`,
			queryName:          "listBackups",
			respIsArray:        true,
			testGuardianAccess: true,
			guardianErr:        `The uri path: "" doesn't exist`,
			guardianData:       `{"listBackups": []}`,
		},
		{
			name: "config update has guardian auth",
			query: `
					mutation {
					  config(input: {cacheMb: -1}) {
						response {
						  code
						  message
						}
					  }
					}`,
			queryName:          "config",
			testGuardianAccess: true,
			guardianErr:        "cache_mb must be non-negative",
			guardianData:       `{"config": null}`,
		},
		{
			name: "config get has guardian auth",
			query: `
					query {
					  config {
						cacheMb
					  }
					}`,
			queryName:          "config",
			testGuardianAccess: true,
			guardianErr:        "",
			guardianData:       "",
		},
		{
			name: "draining has guardian auth",
			query: `
					mutation {
					  draining(enable: false) {
						response {
						  code
						  message
						}
					  }
					}`,
			queryName:          "draining",
			testGuardianAccess: true,
			guardianErr:        "",
			guardianData: `{
								"draining": {
									"response": {
										"code": "Success",
										"message": "draining mode has been set to false"
									}
								}
							}`,
		},
		{
			name: "export has guardian auth",
			query: `
					mutation {
					  export(input: {format: "invalid"}) {
						response {
						  code
						  message
						}
					  }
					}`,
			queryName:          "export",
			testGuardianAccess: true,
			guardianErr:        "invalid export format: invalid",
			guardianData:       `{"export": null}`,
		},
		{
			name: "restore has guardian auth",
			query: `
					mutation {
					  restore(input: {location: "", backupId: "", encryptionKeyFile: ""}) {
						code
					  }
					}`,
			queryName:          "restore",
			testGuardianAccess: true,
			guardianErr:        `The uri path: "" doesn't exist`,
			guardianData:       `{"restore": {"code": "Failure"}}`,
		},
		{
			name: "removeNode has guardian auth",
			query: `
					mutation {
					  removeNode(input: {nodeId: 1, groupId: 2147483640}) {
						response {
							code
						}
					  }
					}`,
			queryName:          "removeNode",
			testGuardianAccess: true,
			guardianErr:        "No group with groupId 2147483640 found",
			guardianData:       `{"removeNode": null}`,
		},
		{
			name: "moveTablet has guardian auth",
			query: `
					mutation {
					  moveTablet(input: {tablet: "non_existent_pred", groupId: 2147483640}) {
						response {
							code
							message
						}
					  }
					}`,
			queryName:          "moveTablet",
			testGuardianAccess: true,
			guardianErr:        "group: [2147483640] is not a known group",
			guardianData:       `{"moveTablet": null}`,
		},
		{
			name: "assign has guardian auth",
			query: `
					mutation {
					  assign(input: {what: UID, num: 0}) {
						response {
							startId
							endId
							readOnly
						}
					  }
					}`,
			queryName:          "assign",
			testGuardianAccess: true,
			guardianErr:        "Nothing to be leased",
			guardianData:       `{"assign": null}`,
		},
		{
			name: "getGQLSchema has guardian auth",
			query: `
					query {
					  getGQLSchema {
						id
					  }
					}`,
			queryName:          "getGQLSchema",
			testGuardianAccess: true,
			guardianErr:        "",
			guardianData:       "",
		},
		{
			name: "updateGQLSchema has guardian auth",
			query: `
					mutation {
					  updateGQLSchema(input: {set: {schema: ""}}) {
						gqlSchema {
						  id
						}
					  }
					}`,
			queryName:          "updateGQLSchema",
			testGuardianAccess: false,
			guardianErr:        "",
			guardianData:       "",
		},
		{
			name: "shutdown has guardian auth",
			query: `
					mutation {
					  shutdown {
						response {
						  code
						  message
						}
					  }
					}`,
			queryName:          "shutdown",
			testGuardianAccess: false,
			guardianErr:        "",
			guardianData:       "",
		},
	}

	for _, tcase := range tcases {
		t.Run(tcase.name, func(t *testing.T) {
			params := dgraphapi.GraphQLParams{Query: tcase.query}
			hc, err := asuite.dc.HTTPClient()
			require.NoError(t, err)
			require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser,
				dgraphapi.DefaultPassword, x.RootNamespace))

			resetUser(t, hc)
			require.NoError(t, hc.LoginIntoNamespace(userid, userpassword, x.RootNamespace))
			gqlResp, err := hc.RunGraphqlQuery(params, true)
			require.Error(t, err)
			// assert ACL error for non-guardians
			assertNonGuardianFailure(t, tcase.queryName, !tcase.respIsArray, gqlResp, err)

			// for guardians, assert non-ACL error or success
			if tcase.testGuardianAccess {
				require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser,
					dgraphapi.DefaultPassword, x.RootNamespace))

				resp, err := hc.RunGraphqlQuery(params, true)
				if tcase.guardianErr == "" {
					require.NoError(t, err)
				} else {
					// require.Len(t, err, 1)
					require.Contains(t, err.Error(), tcase.guardianErr)
				}

				if tcase.guardianData != "" && err == nil {
					require.JSONEq(t, tcase.guardianData, string(resp))
				}
			}
		})
	}
}

func (asuite *AclTestSuite) TestFailedLogin() {
	t := asuite.T()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	gc, cleanup, err := asuite.dc.Client()
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, gc.LoginIntoNamespace(ctx, dgraphapi.DefaultUser,
		dgraphapi.DefaultPassword, x.RootNamespace))

	hc, err := asuite.dc.HTTPClient()
	require.NoError(t, err)
	require.NoError(t, hc.LoginIntoNamespace(dgraphapi.DefaultUser,
		dgraphapi.DefaultPassword, x.RootNamespace))

	require.NoError(t, gc.DropAll())

	client, _, err := asuite.dc.Client()
	require.NoError(t, err)

	// User is not present
	err = client.LoginIntoNamespace(ctx, userid, "simplepassword", x.RootNamespace)
	require.Error(t, err)
	require.Contains(t, err.Error(), x.ErrorInvalidLogin.Error())

	resetUser(t, hc)
	// User is present
	require.Error(t, client.LoginIntoNamespace(ctx, userid, "randomstring", x.RootNamespace))
	require.Contains(t, err.Error(), x.ErrorInvalidLogin.Error())
}

func (asuite *AclTestSuite) TestWrongPermission() {
	t := asuite.T()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	gc, cleanup, err := asuite.dc.Client()
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, gc.LoginIntoNamespace(ctx, dgraphapi.DefaultUser,
		dgraphapi.DefaultPassword, x.RootNamespace))
	require.NoError(t, gc.DropAll())

	mu := &api.Mutation{SetNquads: []byte(`
	_:dev <dgraph.type> "dgraph.type.Group" .
	_:dev <dgraph.xid> "dev" .
	_:dev <dgraph.acl.rule> _:rule1 .
	_:rule1 <dgraph.rule.predicate> "name" .
	_:rule1 <dgraph.rule.permission> "9" .
	`), CommitNow: true}
	_, err = gc.Mutate(mu)

	require.Error(t, err, "Setting permission to 9 should have returned error")
	require.Contains(t, err.Error(), "Value for this predicate should be between 0 and 7")

	mu = &api.Mutation{SetNquads: []byte(`
	_:dev <dgraph.type> "dgraph.type.Group" .
	_:dev <dgraph.xid> "dev" .
	_:dev <dgraph.acl.rule> _:rule1 .
	_:rule1 <dgraph.rule.predicate> "name" .
	_:rule1 <dgraph.rule.permission> "-1" .
	`), CommitNow: true}
	_, err = gc.Mutate(mu)

	require.Error(t, err, "Setting permission to -1 should have returned error")
	require.Contains(t, err.Error(), "Value for this predicate should be between 0 and 7")
}

func (asuite *AclTestSuite) TestACLNamespaceEdge() {
	t := asuite.T()
	gc, cleanup, err := asuite.dc.Client()
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

	json := `
	{
    "set": [
        {
            "dgraph.xid": "groot",
            "dgraph.password": "password",
            "dgraph.type": "dgraph.type.User",
            "dgraph.user.group": {
                "dgraph.xid": "guardians",
                "dgraph.type": "dgraph.type.Group",
                "namespace": 1
            },
            "namespace": 1
        }
    ]
}`

	mu := &api.Mutation{SetJson: []byte(json), CommitNow: true}
	_, err = gc.Mutate(mu)
	require.Error(t, err)
	require.ErrorContains(t, err, "could not insert duplicate value") // Could be gaurdian or groot
}

func (asuite *AclTestSuite) TestACLDuplicateGrootUser() {
	t := asuite.T()
	gc, cleanup, err := asuite.dc.Client()
	require.NoError(t, err)
	defer cleanup()
	require.NoError(t, gc.LoginIntoNamespace(context.Background(),
		dgraphapi.DefaultUser, dgraphapi.DefaultPassword, x.RootNamespace))

	rdfs := `_:a <dgraph.xid> "groot" .
	         _:a <dgraph.type> "dgraph.type.User"  .`

	mu := &api.Mutation{SetNquads: []byte(rdfs), CommitNow: true}
	_, err = gc.Mutate(mu)
	require.Error(t, err)
	require.ErrorContains(t, err, "could not insert duplicate value [groot] for predicate [dgraph.xid]")
}
