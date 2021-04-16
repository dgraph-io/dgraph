/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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

package common

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/testutil"
)

func lambdaOnTypeField(t *testing.T) {
	query := `
		query {
			queryAuthor {
				name
				bio
				rank
			}
		}`
	params := &GraphQLParams{Query: query}
	resp := params.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, resp)

	expectedResponse := `{
		"queryAuthor": [
			{
				"name":"Three Author",
				"bio":"My name is Three Author and I was born on 2001-01-01T00:00:00Z.",
				"rank":1
			},
			{
				"name":"Ann Author",
				"bio":"My name is Ann Author and I was born on 2000-01-01T00:00:00Z.",
				"rank":3
			},
			{
				"name":"Ann Other Author",
				"bio":"My name is Ann Other Author and I was born on 1988-01-01T00:00:00Z.",
				"rank":2
			}
		]
	}`
	testutil.CompareJSON(t, expectedResponse, string(resp.Data))
}

func lambdaOnInterfaceField(t *testing.T) {
	starship := addStarship(t)
	humanID := addHuman(t, starship.ID)
	droidID := addDroid(t)

	// when querying bio on Character (interface) we should get the bio constructed by the lambda
	// registered on Character.bio
	query := `
		query {
			queryCharacter {
				name
				bio
			}
		}`
	params := &GraphQLParams{Query: query}
	resp := params.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, resp)

	expectedResponse := `{
		"queryCharacter": [
			{
				"name":"Han",
				"bio":"My name is Han."
			},
			{
				"name":"R2-D2",
				"bio":"My name is R2-D2."
			}
		]
	}`
	testutil.CompareJSON(t, expectedResponse, string(resp.Data))

	// when querying bio on Human & Droid (type) we should get the bio constructed by the lambda
	// registered on Human.bio and Droid.bio respectively
	query = `
		query {
			queryCharacter {
				name
				... on Human {
					bio
				}
				... on Droid {
					bio
				}
			}
		}`
	params = &GraphQLParams{Query: query}
	resp = params.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, resp)

	expectedResponse = `{
		"queryCharacter": [
			{
				"name":"Han",
				"bio":"My name is Han. I have 10 credits."
			},
			{
				"name":"R2-D2",
				"bio":"My name is R2-D2. My primary function is Robot."
			}
		]
	}`
	testutil.CompareJSON(t, expectedResponse, string(resp.Data))

	// cleanup
	cleanupStarwars(t, starship.ID, humanID, droidID)
}

func lambdaOnQueryUsingDql(t *testing.T) {
	query := `
		query {
			authorsByName(name: "Ann Author") {
				name
				dob
				reputation
			}
		}`
	params := &GraphQLParams{Query: query}
	resp := params.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, resp)

	expectedResponse := `{
		"authorsByName": [
			{
				"name":"Ann Author",
				"dob":"2000-01-01T00:00:00Z",
				"reputation":6.6
			}
		]
	}`
	testutil.CompareJSON(t, expectedResponse, string(resp.Data))
}

func lambdaOnMutationUsingGraphQL(t *testing.T) {
	// first, add the author using @lambda
	query := `
		mutation {
			newAuthor(name: "Lambda")
		}`
	params := &GraphQLParams{Query: query}
	resp := params.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, resp)

	// let's get the author ID of the newly added author as returned by lambda
	var addResp struct {
		AuthorID string `json:"newAuthor"`
	}
	require.NoError(t, json.Unmarshal(resp.Data, &addResp))

	// now, lets query the same author and verify that its reputation was set as 3.0 by lambda func
	query = `
		query ($id: ID!){
			getAuthor(id: $id) {
				name
				reputation
			}
		}`
	params = &GraphQLParams{Query: query, Variables: map[string]interface{}{"id": addResp.AuthorID}}
	resp = params.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, resp)

	expectedResponse := `{
		"getAuthor": {
				"name":"Lambda",
				"reputation":3.0
			}
	}`
	testutil.CompareJSON(t, expectedResponse, string(resp.Data))

	// cleanup
	deleteAuthors(t, []string{addResp.AuthorID}, nil)
}

func lambdaOnQueryWithNoUniqueParents(t *testing.T) {
	queryBookParams := &GraphQLParams{Query: `
	query{
		getBook(bookId: 1){
		  name
		  desc
		  summary
		}
	  }
	`}

	resp := queryBookParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, resp)
	testutil.CompareJSON(t, `{
		"getBook": null
	}`, string(resp.Data))
}

// See: https://discuss.dgraph.io/t/slash-graphql-lambda-bug/12233
func lambdaInMutationWithDuplicateId(t *testing.T) {
	addStudentParams := &GraphQLParams{Query: `
	mutation {
		addChapter(input: [
			{chapterId: 1, name: "Alice", book: {bookId: 1, name: "Fictional Characters"}},
			{chapterId: 2, name: "Bob", book: {bookId: 1, name: "Fictional Characters"}},
			{chapterId: 3, name: "Charlie", book: {bookId: 1, name: "Fictional Characters"}},
			{chapterId: 4, name: "Uttarakhand", book: {bookId: 2, name: "Indian States"}}
		]) {
			numUids
			chapter {
				chapterId
				name
				book {
					bookId
					name
					summary
				}
			}
		}
	}`}
	resp := addStudentParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, resp)

	testutil.CompareJSON(t, `{
		"addChapter": {
		  "numUids": 6,
		  "chapter": [
			{
			  "chapterId": 4,
			  "name": "Uttarakhand",
			  "book": {
				"bookId": 2,
				"name": "Indian States",
				"summary": "hi"
			  }
			},
			{
			  "chapterId": 1,
			  "name": "Alice",
			  "book": {
				"bookId": 1,
				"name": "Fictional Characters",
				"summary": "hi"
			  }
			},
			{
			  "chapterId": 2,
			  "name": "Bob",
			  "book": {
				"bookId": 1,
				"name": "Fictional Characters",
				"summary": "hi"
			  }
			},
			{
			  "chapterId": 3,
			  "name": "Charlie",
			  "book": {
				"bookId": 1,
				"name": "Fictional Characters",
				"summary": "hi"
			  }
			}
		  ]
		}
	}`, string(resp.Data))

	//cleanup
	DeleteGqlType(t, "Chapter", GetXidFilter("chapterId", []interface{}{1, 2, 3, 4}), 4, nil)
	DeleteGqlType(t, "Book", GetXidFilter("bookId", []interface{}{1, 2}), 2, nil)
}

func lambdaWithApolloFederation(t *testing.T) {
	addMissionParams := &GraphQLParams{
		Query: `mutation {
			addMission(input: [
				{id: "M1", designation: "Apollo 1", crew: [
					{id: "14", name: "Gus Grissom", isActive: false}
					{id: "30", name: "Ed White", isActive: true}
					{id: "7", name: "Roger B. Chaffee", isActive: false}
				]}
			]) {
				numUids
			}
		}`,
	}
	resp := addMissionParams.ExecuteAsPost(t, GraphqlURL)
	resp.RequireNoGQLErrors(t)

	// entities query should get correct bio built using the age & name given in representations
	entitiesQueryParams := &GraphQLParams{
		Query: `query _entities($typeName: String!) {
			_entities(representations: [
				{__typename: $typeName, id: "14", name: "Gus Grissom", age: 70}
				{__typename: $typeName, id: "30", name: "Ed White", age: 80}
				{__typename: $typeName, id: "7", name: "An updated name", age: 65}
			]) {
				... on Astronaut {
					name
					bio
				}
			}
		}`,
		Variables: map[string]interface{}{
			"typeName": "Astronaut",
		},
	}
	resp = entitiesQueryParams.ExecuteAsPost(t, GraphqlURL)
	resp.RequireNoGQLErrors(t)

	testutil.CompareJSON(t, `{
		"_entities": [
			{"name": "Gus Grissom", "bio": "Name - Gus Grissom, Age - 70, isActive - false"},
			{"name": "Ed White", "bio": "Name - Ed White, Age - 80, isActive - true"},
			{"name": "Roger B. Chaffee", "bio": "Name - An updated name, Age - 65, isActive - false"}
		]
	}`, string(resp.Data))

	// directly querying from an auto-generated query should give undefined age in bio
	// name in bio should be from dgraph
	dgQueryParams := &GraphQLParams{
		Query: `query {
			queryAstronaut {
				name
				bio
			}
		}`,
	}
	resp = dgQueryParams.ExecuteAsPost(t, GraphqlURL)
	resp.RequireNoGQLErrors(t)

	testutil.CompareJSON(t, `{
		"queryAstronaut": [
			{"name": "Gus Grissom", "bio": "Name - Gus Grissom, Age - undefined, isActive - false"},
			{"name": "Ed White", "bio": "Name - Ed White, Age - undefined, isActive - true"},
			{"name": "Roger B. Chaffee", "bio": "Name - Roger B. Chaffee, Age - undefined, isActive - false"}
		]
	}`, string(resp.Data))

	// cleanup
	DeleteGqlType(t, "Mission", GetXidFilter("id", []interface{}{"M1"}), 1, nil)
	DeleteGqlType(t, "Astronaut", map[string]interface{}{"id": []interface{}{"14", "30", "7"}}, 3,
		nil)
}

// TODO(GRAPHQL-1123): need to find a way to make it work on TeamCity machines.
// The host `172.17.0.1` used to connect to host machine from within docker, doesn't seem to
// work in teamcity machines, neither does `host.docker.internal` works there. So, we are
// skipping the related test for now.
func lambdaOnMutateHooks(t *testing.T) {
	t.Skipf("can't reach host machine from within docker")
	// let's listen to the changes coming in from the lambda hook and store them in this array
	var changelog []string
	server := http.Server{Addr: lambdaHookServerAddr, Handler: http.NewServeMux()}
	defer server.Shutdown(context.Background())
	go func() {
		serverMux := server.Handler.(*http.ServeMux)
		serverMux.HandleFunc("/changelog", func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()
			b, err := ioutil.ReadAll(r.Body)
			require.NoError(t, err)

			var event map[string]interface{}
			require.NoError(t, json.Unmarshal(b, &event))
			require.Greater(t, event["commitTs"], float64(0))
			delete(event, "commitTs")

			b, err = json.Marshal(event)
			require.NoError(t, err)

			changelog = append(changelog, string(b))
		})
		t.Log(server.ListenAndServe())
	}()

	// wait a bit to make sure the server has started
	time.Sleep(2 * time.Second)

	// 1. Add 2 districts: D1, D2
	addDistrictParams := &GraphQLParams{
		Query: `mutation ($input: [AddDistrictInput!]!, $upsert: Boolean){
			addDistrict(input: $input, upsert: $upsert) {
				district {
					dgId
					id
				}
			}
		}`,
		Variables: map[string]interface{}{
			"input": []interface{}{
				map[string]interface{}{"id": "D1", "name": "Dist-1"},
				map[string]interface{}{"id": "D2", "name": "Dist-2"},
			},
			"upsert": false,
		},
	}
	resp := addDistrictParams.ExecuteAsPost(t, GraphqlURL)
	resp.RequireNoGQLErrors(t)

	var addResp struct {
		AddDistrict struct{ District []struct{ DgId, Id string } }
	}
	require.NoError(t, json.Unmarshal(resp.Data, &addResp))
	require.Len(t, addResp.AddDistrict.District, 2)

	// find the uid for each district, to be used later in comparing expectation with reality
	var d1Uid, d2Uid string
	for _, dist := range addResp.AddDistrict.District {
		switch dist.Id {
		case "D1":
			d1Uid = dist.DgId
		case "D2":
			d2Uid = dist.DgId
		}
	}

	// 2. Upsert the district D1 with an updated name
	addDistrictParams.Variables = map[string]interface{}{
		"input": []interface{}{
			map[string]interface{}{"id": "D1", "name": "Dist_1"},
		},
		"upsert": true,
	}
	resp = addDistrictParams.ExecuteAsPost(t, GraphqlURL)
	resp.RequireNoGQLErrors(t)

	// 3. Update the name for district D2
	updateDistrictParams := &GraphQLParams{
		Query: `mutation {
			updateDistrict(input: {
				filter: { id: {eq: "D2"}}
				set: {name: "Dist_2"}
				remove: {name: "Dist-2"}
			}) {
				numUids
			}
		}`,
	}
	resp = updateDistrictParams.ExecuteAsPost(t, GraphqlURL)
	resp.RequireNoGQLErrors(t)

	// 4. Delete both the Districts
	DeleteGqlType(t, "District", GetXidFilter("id", []interface{}{"D1", "D2"}), 2, nil)

	// let's wait for at least 5 secs to get all the updates from the lambda hook
	time.Sleep(5 * time.Second)

	// compare the expected vs the actual ones
	testutil.CompareJSON(t, fmt.Sprintf(`{"changelog": [
	  {
		"__typename": "District",
		"operation": "add",
		"add": {
		  "rootUIDs": [
			"%s",
			"%s"
		  ],
		  "input": [
			{
			  "id": "D1",
			  "name": "Dist-1"
			},
			{
			  "id": "D2",
			  "name": "Dist-2"
			}
		  ]
		}
	  },
	  {
		"__typename": "District",
		"operation": "add",
		"add": {
		  "rootUIDs": [
			"%s"
		  ],
		  "input": [
			{
			  "name": "Dist_1"
			}
		  ]
		}
	  },
	  {
		"__typename": "District",
		"operation": "update",
		"update": {
		  "rootUIDs": [
			"%s"
		  ],
		  "setPatch": {
			"name": "Dist_2"
		  },
		  "removePatch": {
			"name": "Dist-2"
		  }
		}
	  },
	  {
		"__typename": "District",
		"operation": "delete",
		"delete": {
		  "rootUIDs": [
			"%s",
			"%s"
		  ]
		}
	  }
	]}`, d1Uid, d2Uid, d1Uid, d2Uid, d1Uid, d2Uid),
		`{"changelog": [`+strings.Join(changelog, ",")+"]}")
}
