package debugoff

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"

	"github.com/dgrijalva/jwt-go/v4"

	"github.com/dgraph-io/dgraph/graphql/authorization"
	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

var (
	metaInfo *testutil.AuthMeta
)

type TestCase struct {
	user      string
	role      string
	result    string
	name      string
	variables map[string]interface{}
}

func TestAddGQL(t *testing.T) {
	testCases := []TestCase{{
		user:   "user1",
		result: `{"addUserSecret":{"usersecret":[{"aSecret":"secret1"}]}}`,
		variables: map[string]interface{}{"user": &common.UserSecret{
			ASecret: "secret1",
			OwnedBy: "user1",
		}},
	}, {
		user:   "user2",
		result: ``,
		variables: map[string]interface{}{"user": &common.UserSecret{
			ASecret: "secret2",
			OwnedBy: "user1",
		}},
	}}

	query := `
		mutation addUser($user: AddUserSecretInput!) {
			addUserSecret(input: [$user]) {
				userSecret {
					aSecret
				}
			}
		}
	`
	var expected, result struct {
		AddUserSecret struct {
			UserSecret []*common.UserSecret
		}
	}

	for _, tcase := range testCases {
		getUserParams := &common.GraphQLParams{
			Headers:   common.GetJWT(t, tcase.user, tcase.role, metaInfo),
			Query:     query,
			Variables: tcase.variables,
		}
		gqlResponse := getUserParams.ExecuteAsPost(t, common.GraphqlURL)
		if tcase.result == "" {
			require.Equal(t, len(gqlResponse.Errors), 0)
			continue
		}

		common.RequireNoGQLErrors(t, gqlResponse)

		err := json.Unmarshal([]byte(tcase.result), &expected)
		require.NoError(t, err)
		err = json.Unmarshal([]byte(gqlResponse.Data), &result)
		require.NoError(t, err)

		opt := cmpopts.IgnoreFields(common.UserSecret{}, "Id")
		if diff := cmp.Diff(expected, result, opt); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}

		for _, i := range result.AddUserSecret.UserSecret {
			i.Delete(t, tcase.user, tcase.role, metaInfo)
		}
	}
}

func TestAddMutationWithXid(t *testing.T) {
	mutation := `
	mutation addTweets($tweet: AddTweetsInput!){
      addTweets(input: [$tweet]) {
        numUids
      }
    }
	`

	tweet := common.Tweets{
		Id:        "tweet1",
		Text:      "abc",
		Timestamp: "2020-10-10",
	}
	user := "foo"
	addTweetsParams := &common.GraphQLParams{
		Headers:   common.GetJWT(t, user, "", metaInfo),
		Query:     mutation,
		Variables: map[string]interface{}{"tweet": tweet},
	}

	// Add the tweet for the first time.
	gqlResponse := addTweetsParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)

	// Re-adding the tweet should fail.
	gqlResponse = addTweetsParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)

	// Clear the tweet.
	tweet.DeleteByID(t, user, metaInfo)
}

func TestAddMutationWithAuthOnIDFieldHavingInterfaceArg(t *testing.T) {

	// add Library Member
	addLibraryMemberParams := &common.GraphQLParams{
		Query: `mutation addLibraryMember($input: [AddLibraryMemberInput!]!) {
                         addLibraryMember(input: $input, upsert: false) {
                          numUids
                         }
                        }`,
		Variables: map[string]interface{}{"input": []interface{}{
			map[string]interface{}{
				"refID":     "101",
				"name":      "Alice",
				"readHours": "4d2hr",
			}},
		},
	}

	gqlResponse := addLibraryMemberParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)

	var resultLibraryMember struct {
		AddLibraryMember struct {
			NumUids int
		}
	}
	err := json.Unmarshal(gqlResponse.Data, &resultLibraryMember)
	require.NoError(t, err)
	require.Equal(t, 1, resultLibraryMember.AddLibraryMember.NumUids)

	// add SportsMember should return error but in debug mode
	// because interface type have auth rules defined on it
	addSportsMemberParams := &common.GraphQLParams{
		Query: `mutation addSportsMember($input: [AddSportsMemberInput!]!) {
                         addSportsMember(input: $input, upsert: false) {
                          numUids
                         }
                        }`,
		Variables: map[string]interface{}{"input": []interface{}{
			map[string]interface{}{
				"refID": "101",
				"name":  "Bob",
				"plays": "football and cricket",
			}},
		},
	}

	gqlResponse = addSportsMemberParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)
	var resultSportsMember struct {
		AddSportsMember struct {
			NumUids int
		}
	}
	err = json.Unmarshal(gqlResponse.Data, &resultSportsMember)
	require.NoError(t, err)
	require.Equal(t, 0, resultSportsMember.AddSportsMember.NumUids)

	// cleanup
	common.DeleteGqlType(t, "LibraryMember", map[string]interface{}{}, 1, nil)
}

func TestUpdateMutationWithIDFields(t *testing.T) {

	addEmployerParams := &common.GraphQLParams{
		Query: `mutation addEmployer($input: [AddEmployerInput!]!) {
                      addEmployer(input: $input, upsert: false) {
                        numUids
                      }
                    }`,
		Variables: map[string]interface{}{"input": []interface{}{
			map[string]interface{}{
				"company": "ABC tech",
				"name":    "ABC",
				"worker": map[string]interface{}{
					"empId": "E01",
					"regNo": 101,
				},
			}, map[string]interface{}{
				"company": " XYZ tech",
				"name":    "XYZ",
				"worker": map[string]interface{}{
					"empId": "E02",
					"regNo": 102,
				},
			},
		},
		},
	}

	gqlResponse := addEmployerParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)
	type resEmployer struct {
		AddEmployer struct {
			NumUids int
		}
	}
	var resultEmployer resEmployer
	err := json.Unmarshal(gqlResponse.Data, &resultEmployer)
	require.NoError(t, err)
	require.Equal(t, 4, resultEmployer.AddEmployer.NumUids)

	// errors while updating node should be returned in debug mode,
	// if type have auth rules defined on it

	tcases := []struct {
		name      string
		query     string
		variables string
		error     string
	}{{
		name: "update mutation gives error when multiple nodes are selected in filter",
		query: `mutation update($patch: UpdateEmployerInput!) {
                  updateEmployer(input: $patch) {
                    numUids
                  }
                }`,
		variables: `{
              "patch": {
                  "filter": {
                      "name": {
                          "in": [
                              "ABC",
                              "XYZ"
                          ]
                      }
                  },
                  "set": {
                      "name": "MNO",
                      "company": "MNO tech"
                  }
              }
        }`,
	}, {
		name: "update mutation gives error when given @id field already exist in some node",
		query: `mutation update($patch: UpdateEmployerInput!) {
                  updateEmployer(input: $patch) {
                    numUids
                  }
                }`,
		variables: `{
                   "patch": {
                       "filter": {
                           "name": {
                               "in": "ABC"
                           }
                       },
                       "set": {
                           "company": "ABC tech"
                       }
                   }
               }`,
	},
		{
			name: "update mutation gives error when multiple nodes are found at nested level" +
				"while linking rot object to nested object",
			query: `mutation update($patch: UpdateEmployerInput!) {
                  updateEmployer(input: $patch) {
                    numUids
                  }
                }`,
			variables: `{
                   "patch": {
                       "filter": {
                           "name": {
                               "in": "ABC"
                           }
                       },
                       "set": {
                           "name": "JKL",
                           "worker":{
                              "empId":"E01",
                              "regNo":102
                          }
                       }
                   }
               }`,
		},
	}

	for _, tcase := range tcases {
		t.Run(tcase.name, func(t *testing.T) {
			var vars map[string]interface{}
			var resultEmployerErr resEmployer
			if tcase.variables != "" {
				err := json.Unmarshal([]byte(tcase.variables), &vars)
				require.NoError(t, err)
			}
			params := &common.GraphQLParams{
				Query:     tcase.query,
				Variables: vars,
			}

			resp := params.ExecuteAsPost(t, common.GraphqlURL)
			err := json.Unmarshal(resp.Data, &resultEmployerErr)
			require.NoError(t, err)
			require.Equal(t, 0, resultEmployerErr.AddEmployer.NumUids)
		})
	}

	// cleanup
	filterEmployer := map[string]interface{}{"name": map[string]interface{}{"in": []string{"ABC", "XYZ"}}}
	filterWorker := map[string]interface{}{"empId": map[string]interface{}{"in": []string{"E01", "E02"}}}
	common.DeleteGqlType(t, "Employer", filterEmployer, 2, nil)
	common.DeleteGqlType(t, "Worker", filterWorker, 2, nil)
}

func TestMain(m *testing.M) {
	schemaFile := "../schema.graphql"
	schema, err := ioutil.ReadFile(schemaFile)
	if err != nil {
		panic(err)
	}

	jsonFile := "../test_data.json"
	data, err := ioutil.ReadFile(jsonFile)
	if err != nil {
		panic(errors.Wrapf(err, "Unable to read file %s.", jsonFile))
	}

	jwtAlgo := []string{jwt.SigningMethodHS256.Name, jwt.SigningMethodRS256.Name}
	for _, algo := range jwtAlgo {
		authSchema, err := testutil.AppendAuthInfo(schema, algo, "../sample_public_key.pem", false)
		if err != nil {
			panic(err)
		}

		authMeta, err := authorization.Parse(string(authSchema))
		if err != nil {
			panic(err)
		}

		metaInfo = &testutil.AuthMeta{
			PublicKey:      authMeta.VerificationKey,
			Namespace:      authMeta.Namespace,
			Algo:           authMeta.Algo,
			Header:         authMeta.Header,
			PrivateKeyPath: "../sample_private_key.pem",
		}

		common.BootstrapServer(authSchema, data)
		// Data is added only in the first iteration, but the schema is added every iteration.
		if data != nil {
			data = nil
		}
		exitCode := m.Run()
		if exitCode != 0 {
			os.Exit(exitCode)
		}
	}
	os.Exit(0)
}
