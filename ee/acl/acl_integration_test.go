//go:build !oss && integration
// +build !oss,integration

/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/main/licenses/DCL.txt
 */

package acl

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
)

func (suite *AclTestSuite) TestInvalidGetUser() {
	t := suite.T()
	currentUser := getCurrentUser(t, &testutil.HttpToken{AccessJwt: "invalid Token"})
	require.Equal(t, `{"getCurrentUser":null}`, string(currentUser.Data))
	require.Equal(t, x.GqlErrorList{{
		Message: "couldn't rewrite query getCurrentUser because unable to parse jwt token: token" +
			" contains an invalid number of segments",
		Path: []interface{}{"getCurrentUser"},
	}}, currentUser.Errors)
}

func (suite *AclTestSuite) TestPasswordReturn() {
	t := suite.T()
	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    "groot",
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")
	query := `
	query {
		getCurrentUser {
			name
			password
		}
	}`

	resp := makeRequestAndRefreshTokenIfNecessary(t, token, testutil.GraphQLParams{Query: query})
	require.Equal(t, resp.Errors, x.GqlErrorList{{
		Message: `Cannot query field "password" on type "User".`,
		Locations: []x.Location{{
			Line:   5,
			Column: 4,
		}},
	}})
}

func (suite *AclTestSuite) TestPreDefinedPredicates() {
	t := suite.T()
	// This test uses the groot account to ensure that pre-defined predicates
	// cannot be altered even if the permissions allow it.
	dg1, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err, "Error while getting a dgraph client")

	alterPreDefinedPredicates(t, dg1)
}
func (suite *AclTestSuite) TestPreDefinedTypes() {
	t := suite.T()
	// This test uses the groot account to ensure that pre-defined types
	// cannot be altered even if the permissions allow it.
	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err, "Error while getting a dgraph client")

	alterPreDefinedTypes(t, dg)
}

func (suite *AclTestSuite) TestNonExistentGroup() {
	t := suite.T()
	t.Skip()
	// This test won't return an error anymore as if an update in a GraphQL mutation doesn't find
	// anything to update then it just returns an empty result.
	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)

	testutil.DropAll(t, dg)

	token, err := testutil.HttpLogin(&testutil.LoginParams{
		Endpoint:  adminEndpoint,
		UserID:    "groot",
		Passwd:    "password",
		Namespace: x.GalaxyNamespace,
	})
	require.NoError(t, err, "login failed")
	addRulesToGroup(t, token, devGroup, []rule{{"name", Read.Code}})
}

func (suite *AclTestSuite) TestWrongPermission() {
	t := suite.T()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)

	ruleMutation := `
		_:dev <dgraph.type> "dgraph.type.Group" .
		_:dev <dgraph.xid> "dev" .
		_:dev <dgraph.acl.rule> _:rule1 .
		_:rule1 <dgraph.rule.predicate> "name" .
		_:rule1 <dgraph.rule.permission> "9" .
	`

	_, err = dg.NewTxn().Mutate(ctx, &api.Mutation{
		SetNquads: []byte(ruleMutation),
		CommitNow: true,
	})

	require.Error(t, err, "Setting permission to 9 should have returned error")
	require.Contains(t, err.Error(), "Value for this predicate should be between 0 and 7")

	ruleMutation = `
		_:dev <dgraph.type> "dgraph.type.Group" .
		_:dev <dgraph.xid> "dev" .
		_:dev <dgraph.acl.rule> _:rule1 .
		_:rule1 <dgraph.rule.predicate> "name" .
		_:rule1 <dgraph.rule.permission> "-1" .
	`

	_, err = dg.NewTxn().Mutate(ctx, &api.Mutation{
		SetNquads: []byte(ruleMutation),
		CommitNow: true,
	})

	require.Error(t, err, "Setting permission to -1 should have returned error")
	require.Contains(t, err.Error(), "Value for this predicate should be between 0 and 7")
}

func (suite *AclTestSuite) TestHealthForAcl() {
	t := suite.T()
	params := testutil.GraphQLParams{
		Query: `
		query {
			health {
				instance
				address
				lastEcho
				status
				version
				uptime
				group
			}
		}`,
	}

	// assert errors for non-guardians
	assertNonGuardianFailure(t, "health", false, params)

	// assert data for guardians
	token := testutil.GrootHttpLogin(adminEndpoint)

	resp := makeRequestAndRefreshTokenIfNecessary(t, token, params)
	resp.RequireNoGraphQLErrors(t)
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
	err := json.Unmarshal(resp.Data, &guardianResp)

	require.NoError(t, err, "health request failed")
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

func (suite *AclTestSuite) TestGuardianOnlyAccessForAdminEndpoints() {
	t := suite.T()
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
			name: "enterpriseLicense has guardian auth",
			query: `
					mutation {
					  enterpriseLicense(input: {license: ""}) {
						response {
							code
						}
					  }
					}`,
			queryName:          "enterpriseLicense",
			testGuardianAccess: true,
			guardianErr: "while extracting enterprise details from the license: while decoding" +
				" license file: EOF",
			guardianData: `{"enterpriseLicense": null}`,
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
			params := testutil.GraphQLParams{Query: tcase.query}

			// assert ACL error for non-guardians
			assertNonGuardianFailure(t, tcase.queryName, !tcase.respIsArray, params)

			// for guardians, assert non-ACL error or success
			if tcase.testGuardianAccess {
				token := testutil.GrootHttpLogin(adminEndpoint)
				resp := makeRequestAndRefreshTokenIfNecessary(t, token, params)

				if tcase.guardianErr == "" {
					resp.RequireNoGraphQLErrors(t)
				} else {
					require.Len(t, resp.Errors, 1)
					require.Contains(t, resp.Errors[0].Message, tcase.guardianErr)
				}

				if tcase.guardianData != "" {
					require.JSONEq(t, tcase.guardianData, string(resp.Data))
				}
			}
		})
	}
}

func (suite *AclTestSuite) TestFailedLogin() {
	t := suite.T()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	grootClient, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)
	op := api.Operation{DropAll: true}
	if err := grootClient.Alter(ctx, &op); err != nil {
		t.Fatalf("Unable to cleanup db:%v", err)
	}
	require.NoError(t, err)

	client, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)

	// User is not present
	err = client.LoginIntoNamespace(ctx, userid, "simplepassword", x.GalaxyNamespace)
	require.Error(t, err)
	require.Contains(t, err.Error(), x.ErrorInvalidLogin.Error())

	resetUser(t)
	// User is present
	err = client.LoginIntoNamespace(ctx, userid, "randomstring", x.GalaxyNamespace)
	require.Error(t, err)
	require.Contains(t, err.Error(), x.ErrorInvalidLogin.Error())
}
