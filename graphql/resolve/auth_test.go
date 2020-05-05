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

package resolve

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"testing"
	"time"

	dgoapi "github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/graphql/authorization"
	"github.com/dgraph-io/dgraph/graphql/dgraph"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/graphql/test"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgrijalva/jwt-go"
	"github.com/stretchr/testify/require"
	_ "github.com/vektah/gqlparser/v2/validator/rules" // make gql validator init() all rules
	"google.golang.org/grpc/metadata"
	"gopkg.in/yaml.v2"
)

type AuthQueryRewritingCase struct {
	Name string

	// Values to come in the JWT
	User string
	Role string

	// GQL query and variables
	GQLQuery  string
	Variables string

	// Dgraph upsert query and mutations built from the GQL
	DGQuery     string
	DGMutations []*dgraphMutation

	// UIDS and json from the Dgraph result
	Uids string
	Json string

	// Post-mutation auth query and result Dgraph returns from that query
	AuthQuery string
	AuthJson  string

	Error *x.GqlError
}

type authExecutor struct {
	t     *testing.T
	state int

	// initial mutation
	upsertQuery string
	json        string
	uids        string

	// auth
	authQuery string
	authJson  string
}

func (ex *authExecutor) Execute(ctx context.Context, req *dgoapi.Request) (*dgoapi.Response, error) {
	ex.state++
	switch ex.state {
	case 1:
		// initial mutation

		// check that the upsert has built in auth, if required
		require.Equal(ex.t, ex.upsertQuery, req.Query)

		var assigned map[string]string
		if ex.uids != "" {
			err := json.Unmarshal([]byte(ex.uids), &assigned)
			require.NoError(ex.t, err)
		}

		if len(assigned) == 0 {
			// skip state 2, there's no new nodes to apply auth to
			ex.state++
		}

		return &dgoapi.Response{
			Json:    []byte(ex.json),
			Uids:    assigned,
			Metrics: &dgoapi.Metrics{NumUids: map[string]uint64{touchedUidsKey: 0}},
		}, nil

	case 2:
		// auth

		// check that we got the expected auth query
		require.Equal(ex.t, ex.authQuery, req.Query)

		// respond to query
		return &dgoapi.Response{
			Json:    []byte(ex.authJson),
			Metrics: &dgoapi.Metrics{NumUids: map[string]uint64{touchedUidsKey: 0}},
		}, nil

	case 3:
		// final result

		return &dgoapi.Response{
			Json:    []byte(`{"done": "and done"}`),
			Metrics: &dgoapi.Metrics{NumUids: map[string]uint64{touchedUidsKey: 0}},
		}, nil
	}

	panic("test failed")
}

func (ex *authExecutor) CommitOrAbort(ctx context.Context, tc *dgoapi.TxnContext) error {
	return nil
}

// Tests showing that the query rewriter produces the expected Dgraph queries
// when it also needs to write in auth.
func queryRewriting(t *testing.T, sch string, authMeta *authorization.AuthMeta) {
	b, err := ioutil.ReadFile("auth_query_test.yaml")
	require.NoError(t, err, "Unable to read test file")

	var tests []AuthQueryRewritingCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal tests to yaml.")

	testRewriter := NewQueryRewriter()
	gqlSchema := test.LoadSchemaFromString(t, sch)

	for _, tcase := range tests {
		t.Run(tcase.Name, func(t *testing.T) {

			op, err := gqlSchema.Operation(
				&schema.Request{
					Query: tcase.GQLQuery,
					// Variables: tcase.Variables,
				})
			require.NoError(t, err)
			gqlQuery := test.GetQuery(t, op)

			authVars := map[string]interface{}{
				"USER": "user1",
				"ROLE": tcase.Role,
			}

			ctx := addClaimsToContext(context.Background(), t, authVars, authMeta)

			dgQuery, err := testRewriter.Rewrite(ctx, gqlQuery)
			require.Nil(t, err)
			require.Equal(t, tcase.DGQuery, dgraph.AsString(dgQuery))
		})
	}
}

type ClientCustomClaims struct {
	AuthVariables map[string]interface{} `json:"https://xyz.io/jwt/claims"`
	jwt.StandardClaims
}

func addClaimsToContext(
	ctx context.Context,
	t *testing.T,
	authVars map[string]interface{},
	metainfo *authorization.AuthMeta) context.Context {

	claims := ClientCustomClaims{
		authVars,
		jwt.StandardClaims{
			ExpiresAt: time.Now().Add(10 * time.Minute).Unix(),
			Issuer:    "test",
		},
	}

	var signedString string
	var err error
	if metainfo.Algo == authorization.HMAC256 {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signedString, err = token.SignedString([]byte(metainfo.HMACPublicKey))
		require.NoError(t, err)
	} else if metainfo.Algo == authorization.RSA256 {
		keyData, err := ioutil.ReadFile("../e2e/auth/sample_private_key.pem")
		require.NoError(t, err, "Unable to read private key file")

		privateKey, err := jwt.ParseRSAPrivateKeyFromPEM(keyData)
		require.NoError(t, err, "Unable to parse private key")
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		signedString, err = token.SignedString(privateKey)
		require.NoError(t, err)
	}

	md := metadata.New(nil)
	md.Append("authorizationJwt", signedString)
	return metadata.NewIncomingContext(ctx, md)
}

// Tests that the queries that run after a mutation get auth correctly added in.
func mutationQueryRewriting(t *testing.T, sch string, authMeta *authorization.AuthMeta) {
	tests := map[string]struct {
		gqlMut   string
		rewriter func() MutationRewriter
		assigned map[string]string
		result   map[string]interface{}
		dgQuery  string
	}{
		"Add Ticket": {
			gqlMut: `mutation {
				addTicket(input: [{title: "A ticket", onColumn: {colID: "0x1"}}]) {
				  ticket {
					id
					title
					onColumn {
						colID
						name
					}
				  }
				}
			  }`,
			rewriter: NewAddRewriter,
			assigned: map[string]string{"Ticket1": "0x4"},
			dgQuery: `query {
  ticket(func: uid(Ticket2)) @filter(uid(Ticket3)) {
    id : uid
    title : Ticket.title
    onColumn : Ticket.onColumn @filter(uid(Column1)) {
      colID : uid
      name : Column.name
    }
  }
  Ticket2 as var(func: uid(0x4))
  Ticket3 as var(func: uid(Ticket2)) @cascade {
    onColumn : Ticket.onColumn {
      inProject : Column.inProject {
        roles : Project.roles @filter(eq(Role.permission, "VIEW")) {
          assignedTo : Role.assignedTo @filter(eq(User.username, "user1"))
          dgraph.uid : uid
        }
        dgraph.uid : uid
      }
      dgraph.uid : uid
    }
    dgraph.uid : uid
  }
  Column1 as var(func: type(Column)) @cascade {
    inProject : Column.inProject {
      roles : Project.roles @filter(eq(Role.permission, "VIEW")) {
        assignedTo : Role.assignedTo @filter(eq(User.username, "user1"))
        dgraph.uid : uid
      }
      dgraph.uid : uid
    }
    dgraph.uid : uid
  }
}`,
		},
		"Update Ticket": {
			gqlMut: `mutation {
				updateTicket(input: {filter: {id: ["0x4"]}, set: {title: "Updated title"} }) {
					ticket {
						id
						title
						onColumn {
							colID
							name
						}
					  }
				}
			  }`,
			rewriter: NewUpdateRewriter,
			result: map[string]interface{}{
				"updateTicket": []interface{}{map[string]interface{}{"uid": "0x4"}}},
			dgQuery: `query {
  ticket(func: uid(Ticket2)) @filter(uid(Ticket3)) {
    id : uid
    title : Ticket.title
    onColumn : Ticket.onColumn @filter(uid(Column1)) {
      colID : uid
      name : Column.name
    }
  }
  Ticket2 as var(func: uid(0x4))
  Ticket3 as var(func: uid(Ticket2)) @cascade {
    onColumn : Ticket.onColumn {
      inProject : Column.inProject {
        roles : Project.roles @filter(eq(Role.permission, "VIEW")) {
          assignedTo : Role.assignedTo @filter(eq(User.username, "user1"))
          dgraph.uid : uid
        }
        dgraph.uid : uid
      }
      dgraph.uid : uid
    }
    dgraph.uid : uid
  }
  Column1 as var(func: type(Column)) @cascade {
    inProject : Column.inProject {
      roles : Project.roles @filter(eq(Role.permission, "VIEW")) {
        assignedTo : Role.assignedTo @filter(eq(User.username, "user1"))
        dgraph.uid : uid
      }
      dgraph.uid : uid
    }
    dgraph.uid : uid
  }
}`,
		},
	}

	gqlSchema := test.LoadSchemaFromString(t, sch)

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// -- Arrange --
			rewriter := tt.rewriter()
			op, err := gqlSchema.Operation(&schema.Request{Query: tt.gqlMut})
			require.NoError(t, err)
			gqlMutation := test.GetMutation(t, op)
			authVars := map[string]interface{}{
				"USER": "user1",
			}
			ctx := addClaimsToContext(context.Background(), t, authVars, authMeta)
			_, err = rewriter.Rewrite(ctx, gqlMutation)
			require.Nil(t, err)

			// -- Act --
			dgQuery, err := rewriter.FromMutationResult(
				ctx, gqlMutation, tt.assigned, tt.result)

			// -- Assert --
			require.Nil(t, err)
			require.Equal(t, tt.dgQuery, dgraph.AsString(dgQuery))
		})

	}
}

// Tests showing that the query rewriter produces the expected Dgraph queries
// for delete when it also needs to write in auth - this doesn't extend to other nodes
// it only ever applies at the top level because delete only deletes the nodes
// referenced by the filter, not anything deeper.
func deleteQueryRewriting(t *testing.T, sch string, authMeta *authorization.AuthMeta) {
	b, err := ioutil.ReadFile("auth_delete_test.yaml")
	require.NoError(t, err, "Unable to read test file")

	var tests []AuthQueryRewritingCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal tests to yaml.")

	gqlSchema := test.LoadSchemaFromString(t, sch)

	for _, tcase := range tests {
		t.Run(tcase.Name, func(t *testing.T) {
			// -- Arrange --
			var vars map[string]interface{}
			if tcase.Variables != "" {
				err := json.Unmarshal([]byte(tcase.Variables), &vars)
				require.NoError(t, err)
			}

			op, err := gqlSchema.Operation(
				&schema.Request{
					Query:     tcase.GQLQuery,
					Variables: vars,
				})
			require.NoError(t, err)
			mut := test.GetMutation(t, op)
			rewriterToTest := NewDeleteRewriter()

			authVars := map[string]interface{}{
				"USER": "user1",
				"ROLE": tcase.Role,
			}

			ctx := addClaimsToContext(context.Background(), t, authVars, authMeta)

			// -- Act --
			upsert, err := rewriterToTest.Rewrite(ctx, mut)
			q := upsert.Query
			muts := upsert.Mutations

			// -- Assert --
			if tcase.Error != nil || err != nil {
				require.Equal(t, tcase.Error.Error(), err.Error())
			} else {
				require.Equal(t, tcase.DGQuery, dgraph.AsString(q))
				require.Len(t, muts, len(tcase.DGMutations))
				for i, expected := range tcase.DGMutations {
					require.Equal(t, expected.Cond, muts[i].Cond)
					if len(muts[i].SetJson) > 0 || expected.SetJSON != "" {
						require.JSONEq(t, expected.SetJSON, string(muts[i].SetJson))
					}
					if len(muts[i].DeleteJson) > 0 || expected.DeleteJSON != "" {
						require.JSONEq(t, expected.DeleteJSON, string(muts[i].DeleteJson))
					}
				}
			}
		})
	}
}

// In an add mutation
//
// mutation {
// 	addAnswer(input: [
// 	  {
// 		text: "...",
// 		datePublished: "2020-03-26",
// 		author: { username: "u1" },
// 		inAnswerTo: { id: "0x7e" }
// 	  }
// 	]) {
// 	  answer { ... }
//
// There's no initial auth verification.  We add the nodes and then check the auth rules.
// So the only auth to check is through authorizeNewNodes() function.
//
// We don't need to test the json mutations that are created, because those are the same
// as in add_mutation_test.yaml.  What we need to test is the processing around if
// new nodes are checked properly - the query generated to check them, and the post-processing.
func mutationAdd(t *testing.T, sch string, authMeta *authorization.AuthMeta) {
	b, err := ioutil.ReadFile("auth_add_test.yaml")
	require.NoError(t, err, "Unable to read test file")

	var tests []AuthQueryRewritingCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal tests to yaml.")

	gqlSchema := test.LoadSchemaFromString(t, sch)

	for _, tcase := range tests {
		t.Run(tcase.Name, func(t *testing.T) {
			checkAddUpdateCase(t, gqlSchema, tcase, NewAddRewriter, authMeta)
		})
	}
}

// In an update mutation we first need to check that the generated query only finds the
// authorised nodes - it takes the users filter and applies auth.  Then we need to check
// that any nodes added by the mutation were also allowed.
//
// We don't need to test the json mutations that are created, because those are the same
// as in update_mutation_test.yaml.  What we need to test is the processing around if
// new nodes are checked properly - the query generated to check them, and the post-processing.
func mutationUpdate(t *testing.T, sch string, authMeta *authorization.AuthMeta) {
	b, err := ioutil.ReadFile("auth_update_test.yaml")
	require.NoError(t, err, "Unable to read test file")

	var tests []AuthQueryRewritingCase
	err = yaml.Unmarshal(b, &tests)
	require.NoError(t, err, "Unable to unmarshal tests to yaml.")

	gqlSchema := test.LoadSchemaFromString(t, sch)

	for _, tcase := range tests {
		t.Run(tcase.Name, func(t *testing.T) {
			checkAddUpdateCase(t, gqlSchema, tcase, NewUpdateRewriter, authMeta)
		})
	}
}

func checkAddUpdateCase(
	t *testing.T,
	gqlSchema schema.Schema,
	tcase AuthQueryRewritingCase,
	rewriter func() MutationRewriter,
	authMeta *authorization.AuthMeta) {
	// -- Arrange --
	var vars map[string]interface{}
	if tcase.Variables != "" {
		err := json.Unmarshal([]byte(tcase.Variables), &vars)
		require.NoError(t, err)
	}

	op, err := gqlSchema.Operation(
		&schema.Request{
			Query:     tcase.GQLQuery,
			Variables: vars,
		})
	require.NoError(t, err)
	mut := test.GetMutation(t, op)

	authVars := map[string]interface{}{
		"USER": "user1",
		"ROLE": tcase.Role,
	}
	ctx := addClaimsToContext(context.Background(), t, authVars, authMeta)

	ex := &authExecutor{
		t:           t,
		upsertQuery: tcase.DGQuery,
		json:        tcase.Json,
		uids:        tcase.Uids,
		authQuery:   tcase.AuthQuery,
		authJson:    tcase.AuthJson,
	}
	resolver := NewDgraphResolver(rewriter(), ex, StdMutationCompletion(mut.ResponseName()))

	// -- Act --
	resolved, _ := resolver.Resolve(ctx, mut)

	// -- Assert --
	// most cases are built into the authExecutor
	if tcase.Error != nil || resolved.Err != nil {
		require.Equal(t, tcase.Error.Error(), resolved.Err.Error())
	}
}

func TestAuthSchemaRewriting(t *testing.T) {
	sch, err := ioutil.ReadFile("../e2e/auth/schema.graphql")
	require.NoError(t, err, "Unable to read schema file")

	jwtAlgo := []string{authorization.HMAC256, authorization.RSA256}

	metaInfo := &authorization.AuthMeta{}
	for _, algo := range jwtAlgo {
		result, err := testutil.AppendAuthInfo(sch, algo)
		require.NoError(t, err)
		strSchema := string(result)

		err = metaInfo.Parse(strSchema)
		require.NoError(t, err)

		t.Run("Query Rewriting "+algo, func(t *testing.T) {
			queryRewriting(t, strSchema, metaInfo)
		})

		t.Run("Mutation Query Rewriting "+algo, func(t *testing.T) {
			mutationQueryRewriting(t, strSchema, metaInfo)
		})

		t.Run("Add Mutation "+algo, func(t *testing.T) {
			mutationAdd(t, strSchema, metaInfo)
		})

		t.Run("Update Mutation "+algo, func(t *testing.T) {
			mutationUpdate(t, strSchema, metaInfo)
		})

		t.Run("Delete Query Rewriting "+algo, func(t *testing.T) {
			deleteQueryRewriting(t, strSchema, metaInfo)
		})
	}
}
