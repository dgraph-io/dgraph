/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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

package custom_logic

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/dgraph/x"
)

var (
	subscriptionEndpoint = "ws://" + testutil.ContainerAddr("alpha1", 8080) + "/graphql"
)

const (
	customTypes = `type MovieDirector @remote {
		 id: ID!
		 name: String!
		 directed: [Movie]
	 }

	 type Movie @remote {
		 id: ID!
		 name: String!
		 director: [MovieDirector]
	 }
	  type Country @remote {
		code(size: Int): String
		name: String
		states: [State]
		std: Int
	}

	type State @remote {
		code: String
		name: String
		country: Country
	}

	input CountryInput {
		code: String!
		name: String!
		states: [StateInput]
	}

	input StateInput {
		code: String!
		name: String!
	}`
)

func TestCustomGetQuery(t *testing.T) {
	schema := customTypes + `
	 type Query {
		 myFavoriteMovies(id: ID!, name: String!, num: Int): [Movie] @custom(http: {
				 url: "http://mock:8888/favMovies/$id?name=$name&num=$num",
				 method: "GET"
		 })
	 }`
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)

	query := `
	 query {
		 myFavoriteMovies(id: "0x123", name: "Author", num: 10) {
			 id
			 name
			 director {
				 id
				 name
			 }
		 }
	 }`
	params := &common.GraphQLParams{
		Query: query,
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)

	expected := `{"myFavoriteMovies":[{"id":"0x3","name":"Star Wars","director":[{"id":"0x4","name":"George Lucas"}]},{"id":"0x5","name":"Star Trek","director":[{"id":"0x6","name":"J.J. Abrams"}]}]}`
	require.JSONEq(t, expected, string(result.Data))
}

func TestCustomPostQuery(t *testing.T) {
	schema := customTypes + `
	 type Query {
		 myFavoriteMoviesPost(id: ID!, name: String!, num: Int): [Movie] @custom(http: {
				 url: "http://mock:8888/favMoviesPost/$id?name=$name&num=$num",
				 method: "POST"
		 })
	 }`
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)

	query := `
	 query {
		 myFavoriteMoviesPost(id: "0x123", name: "Author", num: 10) {
			 id
			 name
			 director {
				 id
				 name
			 }
		 }
	 }`
	params := &common.GraphQLParams{
		Query: query,
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)

	expected := `{"myFavoriteMoviesPost":[{"id":"0x3","name":"Star Wars","director":[{"id":"0x4","name":"George Lucas"}]},{"id":"0x5","name":"Star Trek","director":[{"id":"0x6","name":"J.J. Abrams"}]}]}`
	require.JSONEq(t, expected, string(result.Data))
}

func TestCustomPostQueryWithBody(t *testing.T) {
	schema := customTypes + `
	 type Query {
		 myFavoriteMoviesPost(id: ID!, name: String!, num: Int): [Movie] @custom(http: {
				 url: "http://mock:8888/favMoviesPostWithBody/$id?name=$name",
                 body:"{id:$id,name:$name,num:$num,movie_type:\"space\"}"
				 method: "POST"
		 })
	 }`
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)

	query := `
	 query {
		 myFavoriteMoviesPost(id: "0x123", name: "Author") {
			 id
			 name
			 director {
				 id
				 name
			 }
		 }
	 }`
	params := &common.GraphQLParams{
		Query: query,
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)

	expected := `{"myFavoriteMoviesPost":[{"id":"0x3","name":"Star Wars","director":
    [{"id":"0x4","name":"George Lucas"}]},{"id":"0x5","name":"Star Trek","director":
    [{"id":"0x6","name":"J.J. Abrams"}]}]}`
	require.JSONEq(t, expected, string(result.Data))
}

func TestCustomQueryShouldForwardHeaders(t *testing.T) {
	schema := customTypes + `
	 type Query {
		 verifyHeaders(id: ID!): [Movie] @custom(http: {
				 url: "http://mock:8888/verifyHeaders",
				 method: "GET",
				 forwardHeaders: ["X-App-Token", "X-User-Id"],
				 secretHeaders: ["Github-Api-Token"]
		 })
	 }

		 # Dgraph.Secret Github-Api-Token "random-fake-token"
		 # Dgraph.Secret app "should-be-overriden"
	 `
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)

	query := `
	 query {
		 verifyHeaders(id: "0x123") {
			 id
			 name
		 }
	 }`
	params := &common.GraphQLParams{
		Query: query,
		Headers: map[string][]string{
			"X-App-Token":   []string{"app-token"},
			"X-User-Id":     []string{"123"},
			"Random-header": []string{"random"},
		},
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)
	expected := `{"verifyHeaders":[{"id":"0x3","name":"Star Wars"}]}`
	require.Equal(t, expected, string(result.Data))
}

func TestCustomNameForwardHeaders(t *testing.T) {
	schema := customTypes + `
	 type Query {
		 verifyHeaders(id: ID!): [Movie] @custom(http: {
				 url: "http://mock:8888/verifyCustomNameHeaders",
				 method: "GET",
				 forwardHeaders: ["X-App-Token:App", "X-User-Id"],
				 secretHeaders: ["Authorization:Github-Api-Token"]
				 introspectionHeaders: ["API:Github-Api-Token"]
		 })
	 }

		 # Dgraph.Secret Github-Api-Token "random-fake-token"
	 `
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)

	query := `
	 query {
		 verifyHeaders(id: "0x123") {
			 id
			 name
		 }
	 }`
	params := &common.GraphQLParams{
		Query: query,
		Headers: map[string][]string{
			"App":           []string{"app-token"},
			"X-User-Id":     []string{"123"},
			"Random-header": []string{"random"},
		},
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)
	expected := `{"verifyHeaders":[{"id":"0x3","name":"Star Wars"}]}`
	require.Equal(t, expected, string(result.Data))
}

func TestSchemaIntrospectionForCustomQueryShouldForwardHeaders(t *testing.T) {
	schema := `
		type Country @remote {
			code: String
			name: String
		}

		input CountryInput {
			code: String!
			name: String!
		}

		type Query {
			myCustom(yo: CountryInput!): [Country!]!
			  @custom(
				http: {
				  url: "http://mock:8888/validatesecrettoken"
				  method: "POST"
				  forwardHeaders: ["Content-Type"]
				  introspectionHeaders: ["GITHUB-API-TOKEN"]
          graphql: "query($yo: CountryInput!) {countries(filter: $yo)}"
				}
			  )
		  }

		# Dgraph.Secret GITHUB-API-TOKEN "random-api-token"
		  `
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)
}

func TestServerShouldAllowForwardHeaders(t *testing.T) {
	schema := `
	type User {
		id: ID!
		name: String!
	}
	type Movie {
		id: ID!
		name: String! @custom(http: {
			url: "http://mock:8888/movieName",
			method: "POST",
			forwardHeaders: ["X-App-User", "X-Group-Id"]
		})
		director: [User] @custom(http: {
			url: "http://mock:8888/movieName",
			method: "POST",
			forwardHeaders: ["User-Id", "X-App-Token"]
		})
		foo: String
	}

	type Query {
		verifyHeaders(id: ID!): [Movie] @custom(http: {
				url: "http://mock:8888/verifyHeaders",
				method: "GET",
				forwardHeaders: ["X-App-Token", "X-User-Id"]
		})
	}`

	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)

	req, err := http.NewRequest(http.MethodOptions, common.GraphqlURL, nil)
	require.NoError(t, err)

	resp, err := (&http.Client{}).Do(req)
	require.NoError(t, err)

	headers := strings.Split(resp.Header.Get("Access-Control-Allow-Headers"), ",")
	require.Subset(t, headers, []string{"X-App-Token", "X-User-Id", "User-Id", "X-App-User", "X-Group-Id"})
}

func TestCustomFieldsInSubscription(t *testing.T) {
	common.SafelyUpdateGQLSchemaOnAlpha1(t, `
	type Teacher @withSubscription {
		tid: ID!
		age: Int!
		name: String
		  @custom(
			http: {
			  url: "http://mock:8888/teacherName"
			  method: "POST"
			  body: "{tid: $tid}"
			  mode: SINGLE
			}
		  )
	  }
	`)

	client, err := common.NewGraphQLSubscription(subscriptionEndpoint, &schema.Request{
		Query: `subscription{
			getTeacher(tid: "0x2712"){
			  name
			}
		  }`,
	}, `{}`)
	require.NoError(t, err)
	_, err = client.RecvMsg()
	require.Contains(t, err.Error(), "Custom field `name` is not supported in graphql subscription")
}

func TestSubscriptionInNestedCustomField(t *testing.T) {
	common.SafelyUpdateGQLSchemaOnAlpha1(t, `
	type Episode {
		name: String! @id
		anotherName: String! @custom(http: {
					url: "http://mock:8888/userNames",
					method: "GET",
					body: "{uid: $name}",
					mode: BATCH
				})
	}

	type Character @withSubscription {
		name: String! @id
		lastName: String @custom(http: {
						url: "http://mock:8888/userNames",
						method: "GET",
						body: "{uid: $name}",
						mode: BATCH
					})
		episodes: [Episode]
	}`)

	client, err := common.NewGraphQLSubscription(subscriptionEndpoint, &schema.Request{
		Query: `subscription{
			queryCharacter {
				name
				episodes {
					name
					anotherName
				}
			 }
		  }`,
	}, `{}`)
	require.NoError(t, err)
	_, err = client.RecvMsg()
	require.Contains(t, err.Error(), "Custom field `anotherName` is not supported in graphql subscription")
}

func addPerson(t *testing.T) *user {
	addTeacherParams := &common.GraphQLParams{
		Query: `mutation addPerson {
			addPerson(input: [{ age: 28 }]) {
				person {
					id
					age
				}
			}
		}`,
	}

	result := addTeacherParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)

	var res struct {
		AddPerson struct {
			Person []*user
		}
	}
	err := json.Unmarshal([]byte(result.Data), &res)
	require.NoError(t, err)

	require.Equal(t, len(res.AddPerson.Person), 1)
	return res.AddPerson.Person[0]
}

func TestCustomQueryWithNonExistentURLShouldReturnError(t *testing.T) {
	schema := customTypes + `
	type Query {
        myFavoriteMovies(id: ID!, name: String!, num: Int): [Movie] @custom(http: {
                url: "http://mock:8888/nonExistentURL/$id?name=$name&num=$num",
                method: "GET"
        })
	}`
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)

	query := `
	query {
		myFavoriteMovies(id: "0x123", name: "Author", num: 10) {
			id
			name
			director {
				id
				name
			}
		}
	}`
	params := &common.GraphQLParams{
		Query: query,
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	require.JSONEq(t, `{ "myFavoriteMovies": [] }`, string(result.Data))
	require.Equal(t, x.GqlErrorList{
		{
			Message: "Evaluation of custom field failed because external request returned an " +
				"error: unexpected error with: 404 for field: myFavoriteMovies within type: Query.",
			Locations: []x.Location{{Line: 3, Column: 3}},
		},
	}, result.Errors)
}

func TestCustomQueryShouldPropagateErrorFromFields(t *testing.T) {
	schema := `
	type Car {
		id: ID!
		name: String!
	}

	type MotorBike {
		id: ID!
		name: String!
	}

	type School {
		id: ID!
		name: String!
	}

	type Person {
		id: ID!
		name: String @custom(http: {
						url: "http://mock:8888/userNames",
						method: "GET",
						body: "{uid: $id}",
						mode: BATCH
					})
		age: Int! @search
		cars: Car @custom(http: {
						url: "http://mock:8888/carsWrongURL",
						method: "GET",
						body: "{uid: $id}",
						mode: BATCH
					})
		bikes: MotorBike @custom(http: {
						url: "http://mock:8888/bikesWrongURL",
						method: "GET",
						body: "{uid: $id}",
						mode: SINGLE
					})
	}`
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)
	p := addPerson(t)

	queryPerson := `
	query {
		queryPerson {
			name
			age
			cars {
				name
			}
			bikes {
				name
			}
		}
	}`
	params := &common.GraphQLParams{
		Query: queryPerson,
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	expected := fmt.Sprintf(`
	{
		"queryPerson": [
			{
				"name": "uname-%s",
				"age": 28,
				"cars": null,
				"bikes": null
			}
		]
	}`, p.ID)
	require.JSONEq(t, expected, string(result.Data))
	require.Equal(t, 2, len(result.Errors))

	expectedErrors := x.GqlErrorList{
		&x.GqlError{Message: "Evaluation of custom field failed because external request " +
			"returned an error: unexpected error with: 404 for field: cars within type: Person.",
			Locations: []x.Location{{Line: 6, Column: 4}},
			Path:      []interface{}{"queryPerson"},
		},
		&x.GqlError{Message: "Evaluation of custom field failed because external request returned" +
			" an error: unexpected error with: 404 for field: bikes within type: Person.",
			Locations: []x.Location{{Line: 9, Column: 4}},
			Path:      []interface{}{"queryPerson"},
		},
	}
	require.Contains(t, result.Errors, expectedErrors[0])
	require.Contains(t, result.Errors, expectedErrors[1])
}

type teacher struct {
	ID  string `json:"tid,omitempty"`
	Age int
}

func addTeachers(t *testing.T) []*teacher {
	addTeacherParams := &common.GraphQLParams{
		Query: `mutation {
			addTeacher(input: [{ age: 28 }, { age: 27 }, { age: 26 }]) {
				teacher {
					tid
					age
				}
			}
		}`,
	}

	result := addTeacherParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)

	var res struct {
		AddTeacher struct {
			Teacher []*teacher
		}
	}
	err := json.Unmarshal([]byte(result.Data), &res)
	require.NoError(t, err)

	require.Equal(t, len(res.AddTeacher.Teacher), 3)

	// sort in descending order
	sort.Slice(res.AddTeacher.Teacher, func(i, j int) bool {
		return res.AddTeacher.Teacher[i].Age > res.AddTeacher.Teacher[j].Age
	})
	return res.AddTeacher.Teacher
}

type school struct {
	ID          string `json:"id,omitempty"`
	Established int
}

func addSchools(t *testing.T, teachers []*teacher) []*school {

	params := &common.GraphQLParams{
		Query: `mutation addSchool($t1: [TeacherRef], $t2: [TeacherRef], $t3: [TeacherRef]) {
			 addSchool(input: [{ established: 1980, teachers: $t1 },
				 { established: 1981, teachers: $t2 }, { established: 1982, teachers: $t3 }]) {
				 school {
					 id
					 established
				 }
			 }
		 }`,
		Variables: map[string]interface{}{
			// teachers work at multiple schools.
			"t1": []map[string]interface{}{{"tid": teachers[0].ID}, {"tid": teachers[1].ID}},
			"t2": []map[string]interface{}{{"tid": teachers[1].ID}, {"tid": teachers[2].ID}},
			"t3": []map[string]interface{}{{"tid": teachers[2].ID}, {"tid": teachers[0].ID}},
		},
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)

	var res struct {
		AddSchool struct {
			School []*school
		}
	}
	err := json.Unmarshal([]byte(result.Data), &res)
	require.NoError(t, err)

	require.Equal(t, len(res.AddSchool.School), 3)
	// The order of mutation result is not the same as the input order, so we sort and return here.
	sort.Slice(res.AddSchool.School, func(i, j int) bool {
		return res.AddSchool.School[i].Established < res.AddSchool.School[j].Established
	})
	return res.AddSchool.School
}

type user struct {
	ID  string `json:"id,omitempty"`
	Age int    `json:"age,omitempty"`
}

func addUsersWithSchools(t *testing.T, schools []*school) []*user {
	params := &common.GraphQLParams{
		Query: `mutation addUser($s1: [SchoolRef], $s2: [SchoolRef], $s3: [SchoolRef]) {
			 addUser(input: [{ age: 10, schools: $s1 },
				 { age: 11, schools: $s2 }, { age: 12, schools: $s3 }]) {
				 user {
					 id
					 age
				 }
			 }
		 }`,
		Variables: map[string]interface{}{
			// Users could have gone to multiple schools
			"s1": []map[string]interface{}{{"id": schools[0].ID}, {"id": schools[1].ID}},
			"s2": []map[string]interface{}{{"id": schools[1].ID}, {"id": schools[2].ID}},
			"s3": []map[string]interface{}{{"id": schools[2].ID}, {"id": schools[0].ID}},
		},
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)

	var res struct {
		AddUser struct {
			User []*user
		}
	}
	err := json.Unmarshal([]byte(result.Data), &res)
	require.NoError(t, err)

	require.Equal(t, len(res.AddUser.User), 3)
	// The order of mutation result is not the same as the input order, so we sort and return users here.
	sort.Slice(res.AddUser.User, func(i, j int) bool {
		return res.AddUser.User[i].Age < res.AddUser.User[j].Age
	})
	return res.AddUser.User
}

func addUsers(t *testing.T) []*user {
	params := &common.GraphQLParams{
		Query: `mutation addUser {
			 addUser(input: [{ age: 10 }, { age: 11 }, { age: 12 }]) {
				 user {
					 id
					 age
				 }
			 }
		 }`,
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)

	var res struct {
		AddUser struct {
			User []*user
		}
	}
	err := json.Unmarshal([]byte(result.Data), &res)
	require.NoError(t, err)

	require.Equal(t, len(res.AddUser.User), 3)
	// The order of mutation result is not the same as the input order, so we sort and return users here.
	sort.Slice(res.AddUser.User, func(i, j int) bool {
		return res.AddUser.User[i].Age < res.AddUser.User[j].Age
	})
	return res.AddUser.User
}

func verifyData(t *testing.T, users []*user, teachers []*teacher, schools []*school) {
	queryUser := `
	 query ($id: [ID!]){
		 queryUser(filter: {id: $id}, order: {asc: age}) {
			name
			age
			cars {
				name
			}
			schools(order: {asc: established}) {
				name
				established
				teachers(order: {desc: age}) {
					name
					age
				}
				classes {
					name
				}
			}
		 }
	 }`
	params := &common.GraphQLParams{
		Query: queryUser,
		Variables: map[string]interface{}{
			"id": []interface{}{users[0].ID, users[1].ID, users[2].ID},
		},
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)

	expected := `{
		 "queryUser": [
		   {
			 "name": "uname-` + users[0].ID + `",
			 "age": 10,
			 "cars": {
				 "name": "car-` + users[0].ID + `"
			 },
			 "schools": [
				 {
					 "name": "sname-` + schools[0].ID + `",
					 "established": 1980,
					 "teachers": [
						 {
							 "name": "tname-` + teachers[0].ID + `",
							 "age": 28
						 },
						 {
							 "name": "tname-` + teachers[1].ID + `",
							 "age": 27
						 }
					 ],
					 "classes": [
						 {
							 "name": "class-` + schools[0].ID + `"
						 }
					 ]
				 },
				 {
					 "name": "sname-` + schools[1].ID + `",
					 "established": 1981,
					 "teachers": [
						 {
							 "name": "tname-` + teachers[1].ID + `",
							 "age": 27
						 },
						 {
							 "name": "tname-` + teachers[2].ID + `",
							 "age": 26
						 }
					 ],
					 "classes": [
						 {
							 "name": "class-` + schools[1].ID + `"
						 }
					 ]
				 }
			 ]
		   },
		   {
			 "name": "uname-` + users[1].ID + `",
			 "age": 11,
			 "cars": {
				 "name": "car-` + users[1].ID + `"
			 },
			 "schools": [
				 {
					 "name": "sname-` + schools[1].ID + `",
					 "established": 1981,
					 "teachers": [
						 {
							 "name": "tname-` + teachers[1].ID + `",
							 "age": 27
						 },
						 {
							 "name": "tname-` + teachers[2].ID + `",
							 "age": 26
						 }
					 ],
					 "classes": [
						 {
							 "name": "class-` + schools[1].ID + `"
						 }
					 ]
				 },
				 {
					 "name": "sname-` + schools[2].ID + `",
					 "established": 1982,
					 "teachers": [
						 {
							 "name": "tname-` + teachers[0].ID + `",
							 "age": 28
						 },
						 {
							 "name": "tname-` + teachers[2].ID + `",
							 "age": 26
						 }
					 ],
					 "classes": [
						 {
							 "name": "class-` + schools[2].ID + `"
						 }
					 ]
				 }
			 ]
		   },
		   {
			 "name": "uname-` + users[2].ID + `",
			 "age": 12,
			 "cars": {
				 "name": "car-` + users[2].ID + `"
			 },
			 "schools": [
				 {
					 "name": "sname-` + schools[0].ID + `",
					 "established": 1980,
					 "teachers": [
						 {
							 "name": "tname-` + teachers[0].ID + `",
							 "age": 28
						 },
						 {
							 "name": "tname-` + teachers[1].ID + `",
							 "age": 27
						 }
					 ],
					 "classes": [
						 {
							 "name": "class-` + schools[0].ID + `"
						 }
					 ]
				 },
				 {
					 "name": "sname-` + schools[2].ID + `",
					 "established": 1982,
					 "teachers": [
						 {
							 "name": "tname-` + teachers[0].ID + `",
							 "age": 28
						 },
						 {
							 "name": "tname-` + teachers[2].ID + `",
							 "age": 26
						 }
					 ],
					 "classes": [
						 {
							 "name": "class-` + schools[2].ID + `"
						 }
					 ]
				 }
			 ]
		   }
		 ]
	   }`

	testutil.CompareJSON(t, expected, string(result.Data))

	singleUserQuery := `
	 query {
		 getUser(id: "` + users[0].ID + `") {
			 name
			 age
			 cars {
				name
			 }
			 schools(order: {asc: established}) {
				 name
				 established
				 teachers(order: {desc: age}) {
					 name
					 age
				 }
				 classes {
					 name
				 }
			 }
		 }
	 }`
	params = &common.GraphQLParams{
		Query: singleUserQuery,
	}

	result = params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)

	expected = `{
		 "getUser": {
			 "name": "uname-` + users[0].ID + `",
			 "age": 10,
			 "cars": {
				 "name": "car-` + users[0].ID + `"
			 },
			 "schools": [
				 {
					 "name": "sname-` + schools[0].ID + `",
					 "established": 1980,
					 "teachers": [
						 {
							 "name": "tname-` + teachers[0].ID + `",
							 "age": 28
						 },
						 {
							 "name": "tname-` + teachers[1].ID + `",
							 "age": 27
						 }
					 ],
					 "classes": [
						 {
							 "name": "class-` + schools[0].ID + `"
						 }
					 ]
				 },
				 {
					 "name": "sname-` + schools[1].ID + `",
					 "established": 1981,
					 "teachers": [
						 {
							 "name": "tname-` + teachers[1].ID + `",
							 "age": 27
						 },
						 {
							 "name": "tname-` + teachers[2].ID + `",
							 "age": 26
						 }
					 ],
					 "classes": [
						 {
							 "name": "class-` + schools[1].ID + `"
						 }
					 ]
				 }
			 ]
		 }
	 }`

	testutil.CompareJSON(t, expected, string(result.Data))
}

func readFile(t *testing.T, name string) string {
	b, err := ioutil.ReadFile(name)
	require.NoError(t, err)
	return string(b)
}

func TestCustomFieldsShouldForwardHeaders(t *testing.T) {
	schema := `
	type Car @remote {
		id: ID!
		name: String!
	}

	type User {
		id: ID!
		name: String
			@custom(
				http: {
					url: "http://mock:8888/checkHeadersForUserName"
					method: "GET"
					body: "{uid: $id}"
					mode: SINGLE,
					secretHeaders: ["GITHUB-API-TOKEN"]
				}
			)
		age: Int! @search
		cars: Car
		@custom(
			http: {
			url: "http://mock:8888/checkHeadersForCars"
			method: "GET"
			body: "{uid: $id}"
			mode: BATCH,
			secretHeaders: ["STRIPE-API-KEY"]
			}
		)
  	}

# Dgraph.Secret GITHUB-API-TOKEN "some-api-token"
# Dgraph.Secret STRIPE-API-KEY "some-api-key"
  `

	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)

	users := addUsers(t)

	queryUser := `
	query ($id: [ID!]){
		queryUser(filter: {id: $id}, order: {asc: age}) {
			name
			age
			cars {
				name
			}
		}
	}`
	params := &common.GraphQLParams{
		Query: queryUser,
		Variables: map[string]interface{}{"id": []interface{}{
			users[0].ID, users[1].ID, users[2].ID}},
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)
}

func TestCustomFieldsShouldSkipNonEmptyVariable(t *testing.T) {
	schema := `
    type User {
		id: ID!
        address:String
		name: String
			@custom(
				http: {
					url: "http://mock:8888/userName"
					method: "GET"
					body: "{uid: $id,address:$address}"
					mode: SINGLE,
					secretHeaders: ["GITHUB-API-TOKEN"]
				}
			)
		age: Int! @search
  	}

# Dgraph.Secret GITHUB-API-TOKEN "some-api-token"
  `
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)

	users := addUsers(t)
	queryUser := `
	query ($id: [ID!]){
		queryUser(filter: {id: $id}, order: {asc: age}) {
			name
			age
		}
	}`
	params := &common.GraphQLParams{
		Query: queryUser,
		Variables: map[string]interface{}{"id": []interface{}{
			users[0].ID, users[1].ID, users[2].ID}},
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)
}

func TestCustomFieldsShouldPassBody(t *testing.T) {
	common.SafelyDropAll(t)

	schema := `
    type User {
		id: String! @id @search(by: [hash, regexp])
        address:String
		name: String
			@custom(
				http: {
					url: "http://mock:8888/userNameWithoutAddress"
					method: "GET"
                    body: "{uid: $id,address:$address}"
                    mode: SINGLE,
					secretHeaders: ["GITHUB-API-TOKEN"]
				}
			)
		age: Int! @search
  	}
# Dgraph.Secret GITHUB-API-TOKEN "some-api-token"
  `
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)

	params := &common.GraphQLParams{
		Query: `mutation addUser {
			 addUser(input: [{ id:"0x5", age: 10 }]) {
				 user {
					 id
					 age
				 }
			 }
		 }`,
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)

	queryUser := `
	query ($id: String!){
		queryUser(filter: {id: {eq: $id}}) {
		   name
           age
		}
     }`

	params = &common.GraphQLParams{
		Query:     queryUser,
		Variables: map[string]interface{}{"id": "0x5"},
	}

	result = params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)
}

func TestCustomFieldsShouldBeResolved(t *testing.T) {
	// This test adds data, modifies the schema multiple times and fetches the data.
	// It has the following modes.
	// 1. Batch operation mode along with REST.
	// 2. Single operation mode along with REST.
	// 3. Batch operation mode along with GraphQL.
	// 4. Single operation mode along with GraphQL.

	schema := readFile(t, "schemas/batch-mode-rest.graphql")
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)

	// add some data
	teachers := addTeachers(t)
	schools := addSchools(t, teachers)
	users := addUsersWithSchools(t, schools)

	// lets check batch mode first using REST endpoints.
	t.Run("rest batch operation mode", func(t *testing.T) {
		verifyData(t, users, teachers, schools)
	})

	t.Run("rest single operation mode", func(t *testing.T) {
		// lets update the schema and check single mode now
		schema := readFile(t, "schemas/single-mode-rest.graphql")
		common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)
		verifyData(t, users, teachers, schools)
	})

	t.Run("graphql single operation mode", func(t *testing.T) {
		// update schema to single mode where fields are resolved using GraphQL endpoints.
		schema := readFile(t, "schemas/single-mode-graphql.graphql")
		common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)
		verifyData(t, users, teachers, schools)
	})

	t.Run("graphql batch operation mode", func(t *testing.T) {
		// update schema to single mode where fields are resolved using GraphQL endpoints.
		schema := readFile(t, "schemas/batch-mode-graphql.graphql")
		common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)
		verifyData(t, users, teachers, schools)
	})

	// Fields are fetched through a combination of REST/GraphQL and single/batch mode.
	t.Run("mixed mode", func(t *testing.T) {
		// update schema to single mode where fields are resolved using GraphQL endpoints.
		schema := readFile(t, "schemas/mixed-modes.graphql")
		common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)
		verifyData(t, users, teachers, schools)
	})
}

func TestCustomFieldResolutionShouldPropagateGraphQLErrors(t *testing.T) {
	schema := `type Car @remote {
		id: ID!
		name: String!
	}

	type User {
		id: ID!
		name: String
		  @custom(
			http: {
			  url: "http://mock:8888/gqlUserNameWithError"
			  method: "POST"
			  mode: SINGLE
			  graphql: "query($id: ID!) { userName(id: $id) }"
			  skipIntrospection: true
			}
		  )
		age: Int! @search
		cars: Car
		  @custom(
			http: {
			  url: "http://mock:8888/gqlCarsWithErrors"
			  method: "POST"
			  mode: BATCH
			  graphql: "query($input: [UserInput]) { cars(input: $input) }"
			  body: "{ id: $id, age: $age}"
			  skipIntrospection: true
			}
		  )
	}`
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)
	users := addUsers(t)
	// Sleep so that schema update can come through in Alpha.
	time.Sleep(time.Second)

	queryUser := `
	query ($id: [ID!]){
		queryUser(filter: {id: $id}, order: {asc: age}) {
			name
			age
			cars {
				name
			}
		}
	}`
	params := &common.GraphQLParams{
		Query: queryUser,
		Variables: map[string]interface{}{"id": []interface{}{
			users[0].ID, users[1].ID, users[2].ID}},
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	sort.Slice(result.Errors, func(i, j int) bool {
		return result.Errors[i].Message < result.Errors[j].Message
	})
	expectedErrs := x.GqlErrorList{
		{
			Message: "error-1 from cars",
		},
		{
			Message: "error-1 from username",
		},
		{
			Message: "error-1 from username",
		},
		{
			Message: "error-1 from username",
		},
		{
			Message: "error-2 from cars",
		},
		{
			Message: "error-2 from username",
		},
		{
			Message: "error-2 from username",
		},
		{
			Message: "error-2 from username",
		},
	}
	for _, err := range expectedErrs {
		err.Path = []interface{}{"queryUser"}
	}
	require.Equal(t, expectedErrs, result.Errors)

	expected := `{
		"queryUser": [
		  {
			"name": "uname-` + users[0].ID + `",
			"age": 10,
			"cars": {
				"name": "car-` + users[0].ID + `"
			}
		  },
		  {
			"name": "uname-` + users[1].ID + `",
			"age": 11,
			"cars": {
				"name": "car-` + users[1].ID + `"
			}
		  },
		  {
			"name": "uname-` + users[2].ID + `",
			"age": 12,
			"cars": {
				"name": "car-` + users[2].ID + `"
			}
		  }
		]
	}`

	testutil.CompareJSON(t, expected, string(result.Data))
}

func TestForInvalidCustomQuery(t *testing.T) {
	schema := customTypes + `
	type Query {
		getCountry1(id: ID!): Country! @custom(http: {
			url: "http://mock:8888/noquery",
			method: "POST",
			forwardHeaders: ["Content-Type"],
			graphql: "query($id: ID!) { country(code: $id) }"
		})
	}`
	common.AssertUpdateGQLSchemaFailure(t, common.Alpha1HTTP, schema, nil,
		[]string{"query `country` is not present in remote schema"})
}

func TestForInvalidArgument(t *testing.T) {
	schema := `
	type Country @remote {
        code: String
        name: String
        states: [State]
        std: Int
      }

      type State @remote {
        code: String
        name: String
        country: Country
      }
	type Query {
		getCountry1(id: ID!): Country! @custom(http: {
			url: "http://mock:8888/invalidargument",
			method: "POST",
			forwardHeaders: ["Content-Type"],
			graphql: "query($id: ID!) { country(code: $id) }"
		})
	}`
	common.AssertUpdateGQLSchemaFailure(t, common.Alpha1HTTP, schema, nil,
		[]string{"argument `code` is not present in remote query `country`"})
}

func TestForInvalidType(t *testing.T) {
	schema := `
	type Country @remote {
        code: String
        name: String
        states: [State]
        std: Int
    }

    type State @remote {
        code: String
        name: String
        country: Country
    }
	type Query {
		getCountry1(id: ID!): Country! @custom(http: {
			url: "http://mock:8888/invalidtype",
			method: "POST",
			forwardHeaders: ["Content-Type"],
			graphql: "query($id: ID!) { country(code: $id) }"
		})
	}`
	common.AssertUpdateGQLSchemaFailure(t, common.Alpha1HTTP, schema, nil, []string{
		"found type mismatch for variable `$id` in query `country`, expected `ID!`, got `Int!`"})
}

func TestCustomLogicGraphql(t *testing.T) {
	schema := `
	type Country @remote {
      code: String
      name: String
	}

	type Query {
		getCountry1(id: ID!): Country!
		  @custom(
			http: {
			  url: "http://mock:8888/validcountry"
			  method: "POST"
			  graphql: "query($id: ID!) { country(code: $id) }"
			}
		  )
	  }`
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)
	query := `
	query {
		getCountry1(id: "BI"){
			code
			name
		}
	}`
	params := &common.GraphQLParams{
		Query: query,
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)
	require.JSONEq(t, string(result.Data), `{"getCountry1":{"code":"BI","name":"Burundi"}}`)
}

func TestCustomLogicGraphqlWithArgumentsOnFields(t *testing.T) {
	schema := `
	type Country @remote {
      code(size: Int!): String
      name: String
	}

	type Query {
		getCountry2(id: ID!): Country!
		  @custom(
			http: {
			  url: "http://mock:8888/argsonfields"
			  method: "POST"
			  forwardHeaders: ["Content-Type"]
			  graphql: "query($id: ID!) { country(code: $id) }"
			}
		  )
	  }`
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)
	query := `
	query {
		getCountry2(id: "BI"){
			code(size: 100)
			name
		}
	}`
	params := &common.GraphQLParams{
		Query: query,
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)
	require.JSONEq(t, string(result.Data), `{"getCountry2":{"code":"BI","name":"Burundi"}}`)
}

func TestCustomLogicGraphqlWithError(t *testing.T) {
	schema := `
	type Country @remote {
        code: String
        name: String
        states: [State]
        std: Int
      }

      type State @remote {
        code: String
        name: String
        country: Country
      }

      input CountryInput {
        code: String!
        name: String!
        states: [StateInput]
      }

      input StateInput {
        code: String!
        name: String!
      }
	type Query {
		getCountryOnlyErr(id: ID!): Country! @custom(http: {
			url: "http://mock:8888/validcountrywitherror",
			method: "POST",
			graphql: "query($id: ID!) { country(code: $id) }"
		})
	}`
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)
	query := `
	query {
		getCountryOnlyErr(id: "BI"){
			code
			name
		}
	}`
	params := &common.GraphQLParams{
		Query: query,
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	require.JSONEq(t, string(result.Data), `{"getCountryOnlyErr":{"code":"BI","name":"Burundi"}}`)
	require.Equal(t, "dummy error", result.Errors.Error())
}

func TestCustomLogicGraphQLValidArrayResponse(t *testing.T) {
	schema := `
	type Country @remote {
      code: String
      name: String
	}

	type Query {
		getCountries(id: ID!): [Country]
		  @custom(
			http: {
			  url: "http://mock:8888/validcountries"
			  method: "POST"
			  graphql: "query($id: ID!) { validCountries(code: $id) }"
			}
		  )
	  }`
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)
	query := `
	query {
		getCountries(id: "BI"){
			code
			name
		}
	}`
	params := &common.GraphQLParams{
		Query: query,
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)
	require.JSONEq(t, string(result.Data), `{"getCountries":[{"name":"Burundi","code":"BI"}]}`)
}

func TestCustomLogicWithErrorResponse(t *testing.T) {
	schema := `
	type Country @remote {
        code: String
        name: String
        states: [State]
        std: Int
      }

      type State @remote {
        code: String
        name: String
        country: Country
      }

      input CountryInput {
        code: String!
        name: String!
        states: [StateInput]
      }

      input StateInput {
        code: String!
        name: String!
      }
	type Query {
		getCountriesErr(id: ID!): [Country] @custom(http: {
			url: "http://mock:8888/graphqlerr",
			method: "POST",
			graphql: "query($id: ID!) { country(code: $id) }"
		})
	}`
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)
	query := `
	query {
		getCountriesErr(id: "BI"){
			code
			name
		}
	}`
	params := &common.GraphQLParams{
		Query: query,
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	require.Equal(t, `{"getCountriesErr":[]}`, string(result.Data))
	require.Equal(t, x.GqlErrorList{
		&x.GqlError{Message: "dummy error"},
		&x.GqlError{
			Message: "Evaluation of custom field failed because key: country could not be found " +
				"in the JSON response returned by external request for field: getCountriesErr" +
				" within type: Query.",
			Locations: []x.Location{{Line: 3, Column: 3}},
		},
	}, result.Errors)
}

type episode struct {
	Name string
}

func addEpisode(t *testing.T, name string) {
	params := &common.GraphQLParams{
		Query: `mutation addEpisode($name: String!) {
			addEpisode(input: [{ name: $name }]) {
				episode {
					name
				}
			}
		}`,
		Variables: map[string]interface{}{
			"name": name,
		},
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)

	var res struct {
		AddEpisode struct {
			Episode []*episode
		}
	}
	err := json.Unmarshal([]byte(result.Data), &res)
	require.NoError(t, err)

	require.Equal(t, len(res.AddEpisode.Episode), 1)
}

type character struct {
	Name string
}

func addCharacter(t *testing.T, name string, episodes interface{}) {
	params := &common.GraphQLParams{
		Query: `mutation addCharacter($name: String!, $episodes: [EpisodeRef]) {
			addCharacter(input: [{ name: $name, episodes: $episodes }]) {
				character {
					name
					episodes {
						name
					}
				}
			}
		}`,
		Variables: map[string]interface{}{
			"name":     name,
			"episodes": episodes,
		},
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)

	var res struct {
		AddCharacter struct {
			Character []*character
		}
	}
	err := json.Unmarshal([]byte(result.Data), &res)
	require.NoError(t, err)

	require.Equal(t, len(res.AddCharacter.Character), 1)
}

func TestCustomFieldsWithXidShouldBeResolved(t *testing.T) {
	schema := `
	type Episode {
		name: String! @id
		anotherName: String! @custom(http: {
					url: "http://mock:8888/userNames",
					method: "GET",
					body: "{uid: $name}",
					mode: BATCH
				})
	}

	type Character {
		name: String! @id
		lastName: String @custom(http: {
						url: "http://mock:8888/userNames",
						method: "GET",
						body: "{uid: $name}",
						mode: BATCH
					})
		episodes: [Episode]
	}`
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)

	ep1 := "episode-1"
	ep2 := "episode-2"
	ep3 := "episode-3"

	addEpisode(t, ep1)
	addEpisode(t, ep2)
	addEpisode(t, ep3)

	addCharacter(t, "character-1", []map[string]interface{}{{"name": ep1}, {"name": ep2}})
	addCharacter(t, "character-2", []map[string]interface{}{{"name": ep2}, {"name": ep3}})
	addCharacter(t, "character-3", []map[string]interface{}{{"name": ep3}, {"name": ep1}})

	queryCharacter := `
	query {
		queryCharacter {
			name
			lastName
			episodes {
				name
				anotherName
			}
		}
	}`
	params := &common.GraphQLParams{
		Query: queryCharacter,
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)

	expected := `{
		"queryCharacter": [
		  {
			"name": "character-1",
			"lastName": "uname-character-1",
			"episodes": [
			  {
				"name": "episode-1",
				"anotherName": "uname-episode-1"
			  },
			  {
				"name": "episode-2",
				"anotherName": "uname-episode-2"
			  }
			]
		  },
		  {
			"name": "character-2",
			"lastName": "uname-character-2",
			"episodes": [
			  {
				"name": "episode-2",
				"anotherName": "uname-episode-2"
			  },
			  {
				"name": "episode-3",
				"anotherName": "uname-episode-3"
			  }
			]
		  },
		  {
			"name": "character-3",
			"lastName": "uname-character-3",
			"episodes": [
			  {
				"name": "episode-1",
				"anotherName": "uname-episode-1"
			  },
			  {
				"name": "episode-3",
				"anotherName": "uname-episode-3"
			  }
			]
		  }
		]
	  }`

	testutil.CompareJSON(t, expected, string(result.Data))

	// In this case the types have ID! field but it is not being requested as part of the query
	// explicitly, so custom logic de-duplication should check for "dgraph-uid" field.
	schema = `
	type Episode {
		id: ID!
		name: String! @id
		anotherName: String! @custom(http: {
					url: "http://mock:8888/userNames",
					method: "GET",
					body: "{uid: $name}",
					mode: BATCH
				})
	}

	type Character {
		id: ID!
		name: String! @id
		lastName: String @custom(http: {
						url: "http://mock:8888/userNames",
						method: "GET",
						body: "{uid: $name}",
						mode: BATCH
					})
		episodes: [Episode]
	}`
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)

	result = params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)
	testutil.CompareJSON(t, expected, string(result.Data))

	// cleanup
	common.DeleteGqlType(t, "Episode", common.GetXidFilter("name", []interface{}{"episode-1",
		"episode-2", "episode-3"}), 3, nil)
	common.DeleteGqlType(t, "Character", common.GetXidFilter("name", []interface{}{"character-1",
		"character-2", "character-3"}), 3, nil)
}

func TestCustomPostMutation(t *testing.T) {
	schema := customTypes + `
	input MovieDirectorInput {
		id: ID
		name: String
		directed: [MovieInput]
	}
	input MovieInput {
		id: ID
		name: String
		director: [MovieDirectorInput]
	}
	type Mutation {
        createMyFavouriteMovies(input: [MovieInput!]): [Movie] @custom(http: {
			url: "http://mock:8888/favMoviesCreate",
			method: "POST",
			body: "{ movies: $input}"
        })
	}`
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)

	params := &common.GraphQLParams{
		Query: `
		mutation createMovies($movs: [MovieInput!]) {
			createMyFavouriteMovies(input: $movs) {
				id
				name
				director {
					id
					name
				}
			}
		}`,
		Variables: map[string]interface{}{
			"movs": []interface{}{
				map[string]interface{}{
					"name":     "Mov1",
					"director": []interface{}{map[string]interface{}{"name": "Dir1"}},
				},
				map[string]interface{}{"name": "Mov2"},
			}},
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)

	expected := `
	{
      "createMyFavouriteMovies": [
        {
          "id": "0x1",
          "name": "Mov1",
          "director": [
            {
              "id": "0x2",
              "name": "Dir1"
            }
          ]
        },
        {
          "id": "0x3",
          "name": "Mov2",
          "director": []
        }
      ]
    }`
	require.JSONEq(t, expected, string(result.Data))
}

func TestCustomPostMutationNullInBody(t *testing.T) {
	schema := `type MovieDirector @remote {
		 id: ID!
		 name: String!
		 directed: [Movie]
	 }
    type Movie @remote {
		 id: ID!
		 name: String
		 director: [MovieDirector]
	 }
	input MovieDirectorInput {
		id: ID
		name: String
		directed: [MovieInput]
	}
	input MovieInput {
		id: ID
		name: String
		director: [MovieDirectorInput]
	}
	type Mutation {
        createMyFavouriteMovies(input: [MovieInput!]): [Movie] @custom(http: {
			url: "http://mock:8888/favMoviesCreateWithNullBody",
			method: "POST",
			body: "{ movies: $input}"
        })
	}`
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)

	params := &common.GraphQLParams{
		Query: `
		mutation createMovies($movs: [MovieInput!]) {
			createMyFavouriteMovies(input: $movs) {
				id
				name
				director {
					id
					name
				}
			}
		}`,
		Variables: map[string]interface{}{
			"movs": []interface{}{
				map[string]interface{}{
					"name":     "Mov1",
					"director": []interface{}{map[string]interface{}{"name": "Dir1"}},
				},
				map[string]interface{}{"name": nil},
			}},
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)

	expected := `
	{
      "createMyFavouriteMovies": [
        {
          "id": "0x1",
          "name": "Mov1",
          "director": [
            {
              "id": "0x2",
              "name": "Dir1"
            }
          ]
        },
        {
          "id": "0x3",
          "name": null,
          "director": []
        }
      ]
    }`
	require.JSONEq(t, expected, string(result.Data))
}

func TestCustomPatchMutation(t *testing.T) {
	schema := customTypes + `
	input MovieDirectorInput {
		id: ID
		name: String
		directed: [MovieInput]
	}
	input MovieInput {
		id: ID
		name: String
		director: [MovieDirectorInput]
	}
	type Mutation {
        updateMyFavouriteMovie(id: ID!, input: MovieInput!): Movie @custom(http: {
			url: "http://mock:8888/favMoviesUpdate/$id",
			method: "PATCH",
			body: "$input"
        })
	}`
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)

	params := &common.GraphQLParams{
		Query: `
		mutation updateMovies($id: ID!, $mov: MovieInput!) {
			updateMyFavouriteMovie(id: $id, input: $mov) {
				id
				name
				director {
					id
					name
				}
			}
		}`,
		Variables: map[string]interface{}{
			"id": "0x1",
			"mov": map[string]interface{}{
				"name":     "Mov1",
				"director": []interface{}{map[string]interface{}{"name": "Dir1"}},
			}},
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)

	expected := `
	{
      "updateMyFavouriteMovie": {
        "id": "0x1",
        "name": "Mov1",
        "director": [
          {
            "id": "0x2",
            "name": "Dir1"
          }
        ]
      }
    }`
	require.JSONEq(t, expected, string(result.Data))
}

func TestCustomMutationShouldForwardHeaders(t *testing.T) {
	schema := customTypes + `
	type Mutation {
        deleteMyFavouriteMovie(id: ID!): Movie @custom(http: {
			url: "http://mock:8888/favMoviesDelete/$id",
			method: "DELETE",
			forwardHeaders: ["X-App-Token", "X-User-Id"]
        })
	}`
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)

	params := &common.GraphQLParams{
		Query: `
		mutation {
			deleteMyFavouriteMovie(id: "0x1") {
				id
				name
			}
		}`,
		Headers: map[string][]string{
			"X-App-Token":   {"app-token"},
			"X-User-Id":     {"123"},
			"Random-header": {"random"},
		},
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)

	expected := `
	{
      "deleteMyFavouriteMovie": {
        "id": "0x1",
        "name": "Mov1"
      }
    }`
	require.JSONEq(t, expected, string(result.Data))
}

func TestCustomGraphqlNullQueryType(t *testing.T) {
	schema := customTypes + `
	type Query {
		getCountry1(id: ID!): Country! @custom(http: {
			url: "http://mock:8888/nullQueryAndMutationType",
			method: "POST",
			graphql: "query($id: ID!) { getCountry(id: $id) }"
		})
	}`
	common.AssertUpdateGQLSchemaFailure(t, common.Alpha1HTTP, schema, nil,
		[]string{"remote schema doesn't have any queries."})
}

func TestCustomGraphqlNullMutationType(t *testing.T) {
	schema := customTypes + `
	type Mutation {
		addCountry1(input: CountryInput!): Country! @custom(http: {
			url: "http://mock:8888/nullQueryAndMutationType",
			method: "POST",
			graphql: "mutation($input: CountryInput!) { putCountry(country: $input) }"
		})
	}`
	common.AssertUpdateGQLSchemaFailure(t, common.Alpha1HTTP, schema, nil,
		[]string{"remote schema doesn't have any mutations."})
}

func TestCustomGraphqlMissingQueryType(t *testing.T) {
	schema := customTypes + `
	type Query {
		getCountry1(id: ID!): Country! @custom(http: {
			url: "http://mock:8888/missingQueryAndMutationType",
			method: "POST",
			graphql: "query($id: ID!) { getCountry(id: $id) }"
		})
	}`
	common.AssertUpdateGQLSchemaFailure(t, common.Alpha1HTTP, schema, nil,
		[]string{"remote schema doesn't have any type named Query."})
}

func TestCustomGraphqlMissingMutationType(t *testing.T) {
	schema := customTypes + `
	type Mutation {
		addCountry1(input: CountryInput!): Country! @custom(http: {
			url: "http://mock:8888/missingQueryAndMutationType",
			method: "POST",
			graphql: "mutation($input: CountryInput!) { putCountry(country: $input) }"
		})
	}`
	common.AssertUpdateGQLSchemaFailure(t, common.Alpha1HTTP, schema, nil,
		[]string{"remote schema doesn't have any type named Mutation"})
}

func TestCustomGraphqlMissingMutation(t *testing.T) {
	schema := customTypes + `
	type Mutation {
		addCountry1(input: CountryInput!): Country!
		  @custom(
			http: {
			  url: "http://mock:8888/setCountry"
			  method: "POST"
			  graphql: "mutation($input: CountryInput!) { putCountry(country: $input) }"
			}
		  )
	  }`
	common.AssertUpdateGQLSchemaFailure(t, common.Alpha1HTTP, schema, nil,
		[]string{"mutation `putCountry` is not present in remote schema"})
}

func TestCustomGraphqlReturnTypeMismatch(t *testing.T) {
	schema := customTypes + `
	type Mutation {
		addCountry1(input: CountryInput!): Movie! @custom(http: {
			url: "http://mock:8888/setCountry",
			method: "POST",
			graphql: "mutation($input: CountryInput!) { setCountry(country: $input) }"
		})
	}`
	common.AssertUpdateGQLSchemaFailure(t, common.Alpha1HTTP, schema, nil, []string{
		"found return type mismatch for mutation `setCountry`, expected `Movie!`, got `Country!`"})
}

func TestCustomGraphqlReturnTypeMismatchForBatchedField(t *testing.T) {
	schema := `
	type Author {
		id: ID!
		name: String!
	}
	type Post {
		id: ID!
		text: String!
		author: Author! @custom(http: {
			url: "http://mock:8888/getPosts",
			method: "POST",
			mode: BATCH
			graphql: "query ($abc: [PostInput]) { getPosts(input: $abc) }"
			body: "{id: $id}"
		})
	}
	`
	common.AssertUpdateGQLSchemaFailure(t, common.Alpha1HTTP, schema, nil, []string{
		"resolving updateGQLSchema failed because input:13: Type Post; Field author: inside " +
			"graphql in @custom directive, found return type mismatch for query `getPosts`, " +
			"expected `[Author!]`, got `[Post!]`.\n"})
}

func TestCustomGraphqlInvalidInputFormatForBatchedField(t *testing.T) {
	schema := `
	type Post {
		id: ID!
		text: String
		comments: Post! @custom(http: {
			url: "http://mock:8888/invalidInputForBatchedField",
			method: "POST",
			mode: BATCH
			graphql: "query { getPosts(input: [{id: $id}]) }"
			body: "{id: $id}"
		})
	}
	`
	common.AssertUpdateGQLSchemaFailure(t, common.Alpha1HTTP, schema, nil, []string{
		"resolving updateGQLSchema failed because input:9: Type Post; Field comments: inside " +
			"graphql in @custom directive, for BATCH mode, query `getPosts` can have only one " +
			"argument whose value should be a variable.\n"})
}

func TestCustomGraphqlMissingTypeForBatchedFieldInput(t *testing.T) {
	schema := `
	type Post {
		id: ID!
		text: String!
		comments: String! @custom(http: {
							url: "http://mock:8888/missingTypeForBatchedFieldInput",
							method: "POST",
							mode: BATCH
							graphql: "query ($abc: [PostInput]) { getPosts(input: $abc) }"
							body: "{id: $id}"
						})
	}
	`
	common.AssertUpdateGQLSchemaFailure(t, common.Alpha1HTTP, schema, nil, []string{
		"resolving updateGQLSchema failed because input:9: Type Post; Field comments: inside " +
			"graphql in @custom directive, remote schema doesn't have any type named " +
			"PostFilterInput.\n"})
}

func TestCustomGraphqlMissingRequiredArgument(t *testing.T) {
	schema := `
	type Country @remote {
      code: String
      name: String
      states: [State]
      std: Int
    }

    type State @remote {
      code: String
      name: String
      country: Country
    }

    input CountryInput {
      code: String!
      name: String!
      states: [StateInput]
    }

    input StateInput {
      code: String!
      name: String!
	}

	type Mutation {
		addCountry1(input: CountryInput!): Country! @custom(http: {
										url: "http://mock:8888/setCountry",
										method: "POST",
										graphql: "mutation { setCountry() }"
									})
	}`
	common.AssertUpdateGQLSchemaFailure(t, common.Alpha1HTTP, schema, nil, []string{
		"argument `country` in mutation" +
			" `setCountry` is missing, it is required by remote mutation."})
}

// this one accepts an object and returns an object
func TestCustomGraphqlMutation1(t *testing.T) {
	schema := `
	type Country @remote {
		code: String
		name: String
		states: [State]
		std: Int
    }

    type State @remote {
      code: String
      name: String
      country: Country
    }

    input CountryInput {
      code: String!
      name: String!
      states: [StateInput]
    }

    input StateInput {
      code: String!
      name: String!
	}

	type Mutation {
		addCountry1(input: CountryInput!): Country! @custom(http: {
					url: "http://mock:8888/setCountry"
					method: "POST"
					graphql: "mutation($input: CountryInput!) { setCountry(country: $input) }"
			})
	  }`
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)

	params := &common.GraphQLParams{
		Query: `
		mutation addCountry1($input: CountryInput!) {
			addCountry1(input: $input) {
				code
				name
				states {
					code
					name
				}
			}
		}`,
		Variables: map[string]interface{}{
			"input": map[string]interface{}{
				"code": "IN",
				"name": "India",
				"states": []interface{}{
					map[string]interface{}{
						"code": "RJ",
						"name": "Rajasthan",
					},
					map[string]interface{}{
						"code": "KA",
						"name": "Karnataka",
					},
				},
			},
		},
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)

	expected := `
	{
		"addCountry1": {
			"code": "IN",
			"name": "India",
			"states": [
				{
					"code": "RJ",
					"name": "Rajasthan"
				},
				{
					"code": "KA",
					"name": "Karnataka"
				}
			]
		}
    }`
	require.JSONEq(t, expected, string(result.Data))
}

// this one accepts multiple scalars and returns a list of objects
func TestCustomGraphqlMutation2(t *testing.T) {
	schema := `
	type Country @remote {
        code: String
        name: String
        states: [State]
        std: Int
      }

      type State @remote {
        code: String
        name: String
        country: Country
      }

      input CountryInput {
        code: String!
        name: String!
        states: [StateInput]
      }

      input StateInput {
        code: String!
        name: String!
	  }

	type Mutation {
		updateCountries(name: String, std: Int): [Country!]! @custom(http: {
								url: "http://mock:8888/updateCountries",
								method: "POST",
								graphql: "mutation($name: String, $std: Int) { updateCountries(name: $name, std: $std) }"
							})
	}`
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)

	params := &common.GraphQLParams{
		Query: `
		mutation updateCountries($name: String, $std: Int) {
			updateCountries(name: $name, std: $std) {
				name
				std
			}
		}`,
		Variables: map[string]interface{}{
			"name": "Australia",
			"std":  91,
		},
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)

	expected := `
	{
		"updateCountries": [
			{
				"name": "India",
				"std": 91
			},
			{
				"name": "Australia",
				"std": 61
			}
		]
    }`
	require.JSONEq(t, expected, string(result.Data))
}

func TestForValidInputArgument(t *testing.T) {
	schema := `
	type Country @remote {
      code: String
      name: String
    }

    input CountryInput {
      code: String!
      name: String!
	}

	type Query {
		myCustom(yo: CountryInput!): [Country!]!
		  @custom(
			http: {
			  url: "http://mock:8888/validinputfield"
			  method: "POST"
			  graphql: "query($yo: CountryInput!) {countries(filter: $yo)}"
			}
		  )
	  }`
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)

	params := &common.GraphQLParams{
		Query: `
		query {
			myCustom(yo:{code:"BI",name:"sd"}){
		 	  name
		  	  code
		 	}
		}`,
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)

	expected := `
	{
		"myCustom": [
		  {
			"name": "sd",
			"code": "BI"
		  }
		]
	  }`
	require.JSONEq(t, expected, string(result.Data))
}

func TestForInvalidInputObject(t *testing.T) {
	schema := `
	 type Country @remote {
        code: String
        name: String
        states: [State]
        std: Int
      }

      type State @remote {
        code: String
        name: String
        country: Country
      }

      input CountryInput @remote {
        code: String!
        name: String!
        states: [StateInput]
      }

      input StateInput @remote {
        code: String!
        name: String!
	 }

		type Query {
			myCustom(yo: CountryInput!): [Country!]! @custom(http: {url: "http://mock:8888/invalidfield", method: "POST", graphql: "query($yo: CountryInput!) {countries(filter: $yo)}"})
		}
	 `
	common.AssertUpdateGQLSchemaFailure(t, common.Alpha1HTTP, schema, nil,
		[]string{"expected type for the field code is Int! but got String! in type CountryInput"})
}

func TestForNestedInvalidInputObject(t *testing.T) {
	schema := `
	type Country @remote {
        code: String
        name: String
        states: [State]
        std: Int
    }

    type State @remote {
        code: String
        name: String
        country: Country
    }

    input CountryInput @remote {
        code: String!
        name: String!
        states: [StateInput]
    }

    input StateInput @remote {
        code: String!
        name: String!
    }

	type Query {
		myCustom(yo: CountryInput!): [Country!]! @custom(http: {url: "http://mock:8888/nestedinvalid", method: "POST",
		graphql: "query($yo: CountryInput!) {countries(filter: $yo)}"})
    }
	 `
	common.AssertUpdateGQLSchemaFailure(t, common.Alpha1HTTP, schema, nil,
		[]string{"expected type for the field name is Int! but got String! in type StateInput"})
}

func TestRestCustomLogicInDeepNestedField(t *testing.T) {
	schema := `
	type SearchTweets {
		id: ID!
		text: String!
		user: User
	}

	type User {
		id: ID!
		screen_name: String! @id
		followers: Followers @custom(http:{
			url: "http://mock:8888/twitterfollowers?screen_name=$screen_name"
			method: "GET",
		})
		tweets: [SearchTweets] @hasInverse(field: user)
	}

	type RemoteUser @remote {
		id: ID!
		name: String
	}

	type Followers@remote{
		users: [RemoteUser]
	}
	`

	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)

	params := &common.GraphQLParams{
		Query: `
		mutation{
			addUser(input:[
			  {
				  screen_name:"manishrjain",
				  tweets:[{text:"hello twitter"}]
			  }
			  {
				  screen_name:"amazingPanda",
				  tweets:[{text:"I love Kung fu."}]
			  }
			]){
			  numUids
			}
		  }`,
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)

	params = &common.GraphQLParams{
		Query: `
		query{
			querySearchTweets{
			  text
			  user{
				screen_name
				followers{
				  users{
					name
				  }
				}
			  }
			}
		  }`,
	}

	result = params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)
	testutil.CompareJSON(t, `
	{
		"querySearchTweets": [
			{
				"text": "hello twitter",
				"user": {
					"screen_name": "manishrjain",
					"followers": {
						"users": [{"name": "hi_balaji"}]
					}
				}
			},{
				"text": "I love Kung fu.",
				"user": {
					"screen_name": "amazingPanda",
					"followers": {
						"users": [{"name": "twitter_bot"}]
					}
				}
			}
		]
	}`, string(result.Data))
}

func TestCustomDQL(t *testing.T) {
	common.SafelyDropAll(t)

	schema := `
	interface Node {
		id: ID!
	}
	type Tweets implements Node {
		id: ID!
		text: String! @search(by: [fulltext, exact])
		user: User
		timestamp: DateTime! @search
	}
	type User implements Node {
		screen_name: String! @id
		followers: Int @search
		tweets: [Tweets] @hasInverse(field: user)
	}
	type UserTweetCount @remote {
		screen_name: String
		tweetCount: Int
	}
	type UserMap @remote {
		followers: Int
		count: Int
	}
	type GroupUserMapQ @remote {
		groupby: [UserMap] @remoteResponse(name: "@groupby")
	}

	type Query {
	  queryNodeR: [Node] @custom(dql: """
		query {
			queryNodeR(func: type(Node), orderasc: User.screen_name) @filter(eq(Tweets.text, "Hello DQL!") OR eq(User.screen_name, "abhimanyu")) {
				dgraph.type
				id: uid
				text: Tweets.text
				screen_name: User.screen_name
			}
		}
	  """)

	  getFirstUserByFollowerCount(count: Int!): User @custom(dql: """
		query getFirstUserByFollowerCount($count: int) {
			getFirstUserByFollowerCount(func: eq(User.followers, $count),orderdesc: User.screen_name, first: 1) {
				screen_name: User.screen_name
				followers: User.followers
			}
		}
		""")
		
	  dqlTweetsByAuthorFollowers: [Tweets] @custom(dql: """
		query {
			var(func: type(Tweets)) @filter(anyoftext(Tweets.text, "DQL")) {
				Tweets.user {
					followers as User.followers
				}
				userFollowerCount as sum(val(followers))
			}
			dqlTweetsByAuthorFollowers(func: uid(userFollowerCount), orderdesc: val(userFollowerCount)) {
				id: uid
				text: Tweets.text
				timestamp: Tweets.timestamp
			}
		}
		""")
		
	  filteredTweetsByAuthorFollowers(search: String!): [Tweets] @custom(dql: """
		query t($search: string) {
			var(func: type(Tweets)) @filter(anyoftext(Tweets.text, $search)) {
				Tweets.user {
					followers as User.followers
				}
				userFollowerCount as sum(val(followers))
			}
			filteredTweetsByAuthorFollowers(func: uid(userFollowerCount), orderdesc: val(userFollowerCount)) {
				id: uid
				text: Tweets.text
				timestamp: Tweets.timestamp
			}
		}
		""")

	queryUserTweetCounts: [UserTweetCount] @custom(dql: """
		query {
			var(func: type(User)) {
				tc as count(User.tweets)
			}
			queryUserTweetCounts(func: uid(tc), orderdesc: User.screen_name) {
				screen_name: User.screen_name
				tweetCount: val(tc)
			}
		}
		""")

	  queryUserKeyMap: [GroupUserMapQ] @custom(dql: """
		{
				queryUserKeyMap(func: type(User)) @groupby(followers: User.followers){
					count(uid)
				}
		}
		""")
	}
	`
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)

	params := &common.GraphQLParams{
		Query: `
		mutation {
		  addTweets(input: [
			{
			  text: "Hello DQL!"
			  user: {
				screen_name: "abhimanyu"
				followers: 5
			  }
			  timestamp: "2020-07-29"
			}
			{
			  text: "Woah DQL works!"
			  user: {
				screen_name: "pawan"
				followers: 10
			  }
			  timestamp: "2020-07-29"
			}
			{
			  text: "hmm, It worked."
			  user: {
				screen_name: "abhimanyu"
				followers: 5
			  }
			  timestamp: "2020-07-30"
			}
			{
			  text: "Nice."
			  user: {
				screen_name: "minhaj"
				followers: 10
			  }
			  timestamp: "2021-02-23"
			}
		  ]) {
			numUids
		  }
		}`,
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)

	params = &common.GraphQLParams{
		Query: `
		query ($count: Int!) {
		  queryNodeR {
			__typename
			... on User { screen_name }
			... on Tweets { text }
		  }
		  queryWithVar: getFirstUserByFollowerCount(count: $count) {
			screen_name
			followers
		  }
		  getFirstUserByFollowerCount(count: 10) {
			screen_name
			followers
		  }
		  dqlTweetsByAuthorFollowers {
			text
		  }
		  filteredTweetsByAuthorFollowers(search: "hello") {
			text
		  }
		  queryUserTweetCounts {
			screen_name
			tweetCount
		  }
		  queryUserKeyMap {
			groupby {
			  followers
			  count
			}
		  }
		}`,
		Variables: map[string]interface{}{"count": 5},
	}

	result = params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)

	require.JSONEq(t, `{
		"queryNodeR": [
			{"__typename": "User", "screen_name": "abhimanyu"},
			{"__typename": "Tweets", "text": "Hello DQL!"}
		],
		"queryWithVar": {
			"screen_name": "abhimanyu",
			"followers": 5
		  },
		  "getFirstUserByFollowerCount": {
			"screen_name": "pawan",
			"followers": 10
		  },
		  "dqlTweetsByAuthorFollowers": [
			{
			  "text": "Woah DQL works!"
			},
			{
			  "text": "Hello DQL!"
			}
		  ],
		  "filteredTweetsByAuthorFollowers": [
			{
			  "text": "Hello DQL!"
			}
		  ],
		  "queryUserTweetCounts": [
			{
			  "screen_name": "pawan",
			  "tweetCount": 1
			},
			{
			  "screen_name": "minhaj",
			  "tweetCount": 1
			},
			{
			  "screen_name": "abhimanyu",
			  "tweetCount": 2
			}
		  ],
		  "queryUserKeyMap": [
			{
			  "groupby": [
				{
				  "followers": 5,
				  "count": 1
				},
				{
				  "followers": 10,
				  "count": 2
				}
			  ]
			}
		  ]
	  }`, string(result.Data))

	userFilter := map[string]interface{}{"screen_name": map[string]interface{}{"in": []string{"minhaj", "pawan", "abhimanyu"}}}
	common.DeleteGqlType(t, "User", userFilter, 3, nil)
	tweetFilter := map[string]interface{}{"text": map[string]interface{}{"in": []string{"Hello DQL!", "Woah DQL works!", "hmm, It worked.", "Nice."}}}
	common.DeleteGqlType(t, "Tweets", tweetFilter, 4, nil)
}

func TestCustomGetQuerywithRESTError(t *testing.T) {
	schema := customTypes + `
	 type Query {
		 myFavoriteMovies(id: ID!, name: String!, num: Int): [Movie] @custom(http: {
				 url: "http://mock:8888/favMoviesError/$id?name=$name&num=$num",
				 method: "GET"
		 })
	 }`
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)

	query := `
	 query {
		 myFavoriteMovies(id: "0x123", name: "Author", num: 10) {
			 id
			 name
			 director {
				 id
				 name
			 }
		 }
	 }`
	params := &common.GraphQLParams{
		Query: query,
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	require.Equal(t, x.GqlErrorList{
		{
			Message:   "Rest API returns Error for myFavoriteMovies query",
			Locations: []x.Location{{Line: 5, Column: 4}},
			Path:      []interface{}{"Movies", "name"},
		},
	}, result.Errors)

}

func TestCustomFieldsWithRestError(t *testing.T) {
	schema := `
    type Car @remote {
		id: ID! 
		name: String!
	}

	type User {
		id: String! @id @search(by: [hash, regexp])
		name: String
			@custom(
				http: {
					url: "http://mock:8888//userNameError"
					method: "GET"
					body: "{uid: $id}"
					mode: SINGLE,
				}
			)
		age: Int! @search
		cars: Car
		@custom(
			http: {
			url: "http://mock:8888/cars"
			method: "GET"
			body: "{uid: $id}"
			mode: BATCH,
			}
	      )		
  	}
  `

	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)

	params := &common.GraphQLParams{
		Query: `mutation addUser {
			 addUser(input: [{ id:"0x1", age: 10 }]) {
				 user {
					 id
					 age
				 }
			 }
		 }`,
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, result)

	queryUser := `
	query ($id: String!){
		queryUser(filter: {id: {eq: $id}}) {
			id
			name
			age
			cars{
			  name
			}
		}
	}`

	params = &common.GraphQLParams{
		Query:     queryUser,
		Variables: map[string]interface{}{"id": "0x1"},
	}

	result = params.ExecuteAsPost(t, common.GraphqlURL)

	expected := `
	{
      "queryUser": [
        {
          "id": "0x1",
          "name": null,
          "age": 10,
          "cars": {
            "name": "car-0x1"
          }	
        }
      ]
    }`

	require.Equal(t, x.GqlErrorList{
		{
			Message: "Rest API returns Error for field name",
			Path:    []interface{}{"queryUser"},
		},
	}, result.Errors)

	require.JSONEq(t, expected, string(result.Data))

}

func TestCustomPostMutationWithRESTError(t *testing.T) {
	schema := customTypes + `
	input MovieDirectorInput {
		id: ID
		name: String
		directed: [MovieInput]
	}
	input MovieInput {
		id: ID
		name: String
		director: [MovieDirectorInput]
	}
	type Mutation {
        createMyFavouriteMovies(input: [MovieInput!]): [Movie] @custom(http: {
			url: "http://mock:8888/favMoviesCreateError",
			method: "POST",
			body: "{ movies: $input}"
        })
	}`
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)

	params := &common.GraphQLParams{
		Query: `
		mutation createMovies($movs: [MovieInput!]) {
			createMyFavouriteMovies(input: $movs) {
				id
				name
				director {
					id
					name
				}
			}
		}`,
		Variables: map[string]interface{}{
			"movs": []interface{}{
				map[string]interface{}{
					"name":     "Mov1",
					"director": []interface{}{map[string]interface{}{"name": "Dir1"}},
				},
				map[string]interface{}{"name": "Mov2"},
			}},
	}

	result := params.ExecuteAsPost(t, common.GraphqlURL)
	require.Equal(t, x.GqlErrorList{
		{
			Message: "Rest API returns Error for FavoriteMoviesCreate query",
		},
	}, result.Errors)

}

func TestCustomResolverInInterfaceImplFrag(t *testing.T) {
	schema := `
	interface Character {
		id: ID!
		name: String! @id
	}
	
	type Human implements Character {
		totalCredits: Int
		bio: String @custom(http: {
			url: "http://mock:8888/humanBio",
			method: "POST",
			body: "{name: $name, totalCredits: $totalCredits}"
		})
	}
	type Droid implements Character {
		primaryFunction: String
	}`
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schema)

	addCharacterParams := &common.GraphQLParams{
		Query: `mutation {
			addHuman(input: [{name: "Han", totalCredits: 10}]) {
				numUids
			}
			addDroid(input: [{name: "R2-D2", primaryFunction: "Robot"}]) {
				numUids
			}
		}`,
	}
	resp := addCharacterParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, resp)

	queryCharacterParams := &common.GraphQLParams{
		Query: `query {
			queryCharacter {
				name
				... on Human {
					bio
				}
				... on Droid {
					primaryFunction
				}
			}
		}`,
	}
	resp = queryCharacterParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, resp)

	testutil.CompareJSON(t, `{
	  "queryCharacter": [
		{
		  "name": "Han",
		  "bio": "My name is Han and I have 10 credits."
		}, {
		  "name": "R2-D2",
		  "primaryFunction": "Robot"
		}
	  ]
	}`, string(resp.Data))

	// cleanup
	common.DeleteGqlType(t, "Character", common.GetXidFilter("name", []interface{}{"Han",
		"R2-D2"}), 2, nil)
}

// See: https://discuss.dgraph.io/t/custom-field-resolvers-are-not-always-called/12489
func TestCustomFieldIsResolvedWhenNoModeGiven(t *testing.T) {
	sch := `
	type ItemType {
	  typeId: String! @id
	  name: String

	  marketStats: MarketStatsR @custom(http: {
		url: "http://localhost:8080/graphql", # We are using the same alpha to serve MarketStats.
		method: POST,
		graphql: "query($typeId:String!) { getMarketStats(typeId: $typeId) }",
		skipIntrospection: true,
	  })
	}
	
	type Blueprint {
	  blueprintId: String! @id
	  shallowProducts: [ItemType]
	  deepProducts: [BlueprintProduct]
	}
	
	type BlueprintProduct {
	  itemType: ItemType
	  amount: Int
	}
	
	type MarketStats  {
	  typeId: String! @id
	  price: Float 
	}
	
	type MarketStatsR @remote {
	  typeId: String
	  price: Float
	}`
	common.SafelyUpdateGQLSchemaOnAlpha1(t, sch)

	mutation := &common.GraphQLParams{
		Query: `mutation AddExampleData {
		  addItemType(input: {
			typeId: "1"
			name: "Test"
		  }) { numUids }
		  addMarketStats(input: {
			typeId: "1"
			price: 9.99
		  }) { numUids }
		  addBlueprint(input: {
			blueprintId: "bp1"
			shallowProducts: [{ typeId: "1" }]
			deepProducts: [{
			  amount: 1
			  itemType: { typeId: "1" }
			}]
		  }) { numUids }
		}`,
	}
	resp := mutation.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, resp)

	query := &common.GraphQLParams{
		Query: `query {
		  works: getItemType(typeId:"1") {
			typeId
			marketStats { price }
		  }
		  doesntWork: getItemType(typeId: "1") {
			marketStats { price }
		  }
		  shallowWorks: getBlueprint(blueprintId:"bp1") {
			shallowProducts {
			  typeId
			  marketStats { price }
			}
		  }
		  deepDoesntWork: getBlueprint(blueprintId:"bp1") {
			deepProducts {
			  itemType {
				typeId
				marketStats { price }
			  }
			}
		  }
		}`,
	}
	resp = query.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, resp)

	testutil.CompareJSON(t, `{
	  "works": {
		"marketStats": {
		  "price": 9.99
		},
		"typeId": "1"
	  },
	  "doesntWork": {
		"marketStats": {
		  "price": 9.99
		}
	  },
	  "shallowWorks": {
		"shallowProducts": [
		  {
			"marketStats": {
			  "price": 9.99
			},
			"typeId": "1"
		  }
		]
	  },
	  "deepDoesntWork": {
		"deepProducts": [
		  {
			"itemType": {
			  "marketStats": {
			    "price": 9.99
			  },
			  "typeId": "1"
			}
		  }
		]
	  }
	}`, string(resp.Data))

	// cleanup
	common.DeleteGqlType(t, "ItemType", map[string]interface{}{}, 1, nil)
	common.DeleteGqlType(t, "Blueprint", map[string]interface{}{}, 1, nil)
	common.DeleteGqlType(t, "BlueprintProduct", map[string]interface{}{}, 1, nil)
	common.DeleteGqlType(t, "MarketStats", map[string]interface{}{}, 1, nil)
}

func TestApolloFederationWithCustom(t *testing.T) {
	sch := `
      type Product @key(fields: "upc") @extends {
        upc: String! @id @external
        weight: Int @external
        price: Int @external
        inStock: Boolean
        shippingEstimate: Int @requires(fields: "price weight") @custom(http: {
			url: "http://mock:8888/shippingEstimate"
			method: POST
			mode: BATCH
			body: "{upc: $upc, weight: $weight, price: $price}"
			skipIntrospection: true
		})
      }`
	common.SafelyUpdateGQLSchemaOnAlpha1(t, sch)

	mutation := &common.GraphQLParams{
		Query: `mutation {
		  addProduct(input: [
			{ upc: "1", inStock: true },
			{ upc: "2", inStock: false }
		  ]) { numUids }
		}`,
	}
	resp := mutation.ExecuteAsPost(t, common.GraphqlURL)
	resp.RequireNoGQLErrors(t)

	query := &common.GraphQLParams{
		Query: `query _entities($typeName: String!) {
			_entities(representations: [
				{__typename: $typeName, upc: "2", price: 2000, weight: 100}
				{__typename: $typeName, upc: "1", price: 999, weight: 500}
			]) {
				... on Product {
					upc
					shippingEstimate
				}
			}
		}`,
		Variables: map[string]interface{}{"typeName": "Product"},
	}
	resp = query.ExecuteAsPost(t, common.GraphqlURL)
	resp.RequireNoGQLErrors(t)

	testutil.CompareJSON(t, `{
	  "_entities": [
		{ "upc": "2", "shippingEstimate": 0 },
		{ "upc": "1", "shippingEstimate": 250 }
	  ]
	}`, string(resp.Data))

	common.DeleteGqlType(t, "Product", map[string]interface{}{}, 2, nil)
}

func TestMain(m *testing.M) {
	err := common.CheckGraphQLStarted(common.GraphqlAdminURL)
	if err != nil {
		x.Log(err, "Waited for GraphQL test server to become available, but it never did.")
		os.Exit(1)
	}
	os.Exit(m.Run())
}
