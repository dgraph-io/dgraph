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

package bench

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/graphql/authorization"
	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/dgraph-io/dgraph/testutil"
)

const (
	graphqlURL = "http://localhost:8080/graphql"
)

func getJWT(b require.TestingT, metaInfo *testutil.AuthMeta) http.Header {
	jwtToken, err := metaInfo.GetSignedToken("", 300*time.Second)
	require.NoError(b, err)

	h := make(http.Header)
	h.Add(metaInfo.Header, jwtToken)
	return h
}

func getAuthMeta(schema string) *testutil.AuthMeta {
	authMeta, err := authorization.Parse(schema)
	if err != nil {
		panic(err)
	}

	return &testutil.AuthMeta{
		PublicKey: authMeta.VerificationKey,
		Namespace: authMeta.Namespace,
		Algo:      authMeta.Algo,
		Header:    authMeta.Header,
	}
}

func clearAll(b require.TestingT, metaInfo *testutil.AuthMeta) {
	getParams := &common.GraphQLParams{
		Headers: getJWT(b, metaInfo),
		Query: `
		mutation deleteCuisine($type: String) {
  			deleteCuisine(filter: {type: { eq: $type}}) {
    			msg
  			}
		}
		`,
		Variables: map[string]interface{}{"type": "TypeCuisineAuth"},
	}
	gqlResponse := getParams.ExecuteAsPost(b, graphqlURL)
	require.Nil(b, gqlResponse.Errors)

	getParams = &common.GraphQLParams{
		Headers: getJWT(b, metaInfo),
		Query: `
		mutation deleteRestaurant($name: String) {
  			deleteRestaurant(filter: {name: { anyofterms: $name}}) {
    			msg
  			}
		}
		`,
		Variables: map[string]interface{}{"name": "TypeRestaurantAuth"},
	}
	gqlResponse = getParams.ExecuteAsPost(b, graphqlURL)
	require.Nil(b, gqlResponse.Errors)

	getParams = &common.GraphQLParams{
		Headers: getJWT(b, metaInfo),
		Query: `
		mutation deleteDish($type: String) {
  			deleteDish(filter: {type: { eq: $type}}) {
    			msg
  			}
		}
		`,
		Variables: map[string]interface{}{"type": "TypeDishAuth"},
	}
	gqlResponse = getParams.ExecuteAsPost(b, graphqlURL)
	require.Nil(b, gqlResponse.Errors)

	getParams = &common.GraphQLParams{
		Headers: getJWT(b, metaInfo),
		Query: `
		mutation deleteOwner($name: String) {
  			deleteOwner(filter: {name: { eq: $name}}) {
    			msg
  			}
		}
		`,
		Variables: map[string]interface{}{"name": "TypeOwnerAuth"},
	}
	gqlResponse = getParams.ExecuteAsPost(b, graphqlURL)
	require.Nil(b, gqlResponse.Errors)
}

// Running the Benchmark:
// Command:  go test -bench=. -benchtime=60s
//	go test -bench=. -benchtime=60s
//	goos: linux
//	goarch: amd64
//	pkg: github.com/dgraph-io/dgraph/graphql/e2e/auth/bench
// Auth
//	BenchmarkNestedQuery-8                88         815315761 ns/op
//	BenchmarkOneLevelQuery-8            4357          15626384 ns/op
// Non-Auth
//	BenchmarkNestedQuery-8                33        2218877846 ns/op
//	BenchmarkOneLevelQuery-8            4446          16100509 ns/op

// Auth Extension (BenchmarkNestedQuery)
//"extensions": {
//   "touched_uids": 8410962,
//   "tracing": {
//     "version": 1,
//     "startTime": "2020-07-16T23:45:27.798693638+05:30",
//     "endTime": "2020-07-16T23:45:28.844749169+05:30",
//     "duration": 1046055551,
//     "execution": {
//       "resolvers": [
//         {
//           "path": [
//             "queryCuisine"
//           ],
//           "parentType": "Query",
//           "fieldName": "queryCuisine",
//           "returnType": "[Cuisine]",
//           "startOffset": 144549,
//           "duration": 1045026189,
//           "dgraph": [
//             {
//               "label": "query",
//               "startOffset": 262828,
//               "duration": 1044381745
//             }
//           ]
//         }
//       ]
//     }
//   }
// }

// Non Auth Extension (BenchmarkNestedQuery)
//"extensions": {
//  "touched_uids": 458610,
//  "tracing": {
//    "version": 1,
//    "startTime": "2020-07-16T23:46:48.73641261+05:30",
//    "endTime": "2020-07-16T23:46:50.281062742+05:30",
//    "duration": 1544650302,
//    "execution": {
//      "resolvers": [{
//        "path": [
//          "queryCuisine"
//        ],
//        "parentType": "Query",
//        "fieldName": "queryCuisine",
//        "returnType": "[Cuisine]",
//        "startOffset": 154997,
//        "duration": 1118614851,
//        "dgraph": [{
//          "label": "query",
//          "startOffset": 256126,
//          "duration": 823062710
//        }]
//      }]
//    }
//  }
//}

func BenchmarkNestedQuery(b *testing.B) {
	schemaFile := "schema_auth.graphql"
	schema, err := ioutil.ReadFile(schemaFile)
	require.NoError(b, err)

	metaInfo := getAuthMeta(string(schema))
	metaInfo.AuthVars = map[string]interface{}{
		"Role":  "ADMIN",
		"Dish":  "Dish",
		"RName": "Restaurant",
		"RCurr": "$",
	}

	query := `
	query {
	  queryCuisine (first: 100) {
		id
		name
		restaurants (first: 100) {
		  id 
		  name
		  dishes (first: 100) {
			id
			name
		  }
		}
	  }
	}
	`

	getUserParams := &common.GraphQLParams{
		Headers: getJWT(b, metaInfo),
		Query:   query,
	}

	for i := 0; i < b.N; i++ {
		gqlResponse := getUserParams.ExecuteAsPost(b, graphqlURL)
		require.Nilf(b, gqlResponse.Errors, "%+v", gqlResponse.Errors)
	}
}

func BenchmarkOneLevelQuery(b *testing.B) {
	schemaFile := "schema_auth.graphql"
	schema, err := ioutil.ReadFile(schemaFile)
	require.NoError(b, err)

	metaInfo := getAuthMeta(string(schema))
	metaInfo.AuthVars = map[string]interface{}{
		"Role":  "ADMIN",
		"Dish":  "Dish",
		"RName": "Restaurant",
		"RCurr": "$",
	}

	query := `
	query {
		queryCuisine (first: 300000) {
	  		id 
	  		name
	  	}
	}
	`

	getUserParams := &common.GraphQLParams{
		Headers: getJWT(b, metaInfo),
		Query:   query,
	}

	for i := 0; i < b.N; i++ {
		gqlResponse := getUserParams.ExecuteAsPost(b, graphqlURL)
		require.Nilf(b, gqlResponse.Errors, "%+v", gqlResponse.Errors)
	}
}

type Cuisine struct {
	Id     string `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
	Public bool   `json:"public,omitempty"`
	Type   string `json:"type,omitempty"`
	Dishes []Dish `json:"dishes,omitempty"`
}

type Restaurant struct {
	Xid      string    `json:"xid,omitempty"`
	Name     string    `json:"name,omitempty"`
	Currency string    `json:"currency,omitempty"`
	Cuisines []Cuisine `json:"cuisines,omitempty"`
}

type Dish struct {
	Id       string    `json:"id,omitempty"`
	Name     string    `json:"name,omitempty"`
	Type     string    `json:"type,omitempty"`
	Cuisines []Cuisine `json:"cuisines,omitempty"`
}

type Owner struct {
	Username       string      `json:"username,omitempty"`
	Name           string      `json:"name,omitempty"`
	HasRestaurants Restaurants `json:"hasRestaurants,omitempty"`
}

type Cuisines []Cuisine

func (c Cuisines) delete(b require.TestingT, metaInfo *testutil.AuthMeta) {
	getParams := &common.GraphQLParams{
		Headers: getJWT(b, metaInfo),
		Query: `
		mutation deleteCuisine($type: String) {
  			deleteCuisine(filter: {type: { eq: $type}}) {
    			msg
  			}
		}
		`,
		Variables: map[string]interface{}{"type": "TypeCuisineAuth"},
	}
	gqlResponse := getParams.ExecuteAsPost(b, graphqlURL)
	require.Nil(b, gqlResponse.Errors)
}

func (c Cuisines) add(b require.TestingT, metaInfo *testutil.AuthMeta) {
	getParams := &common.GraphQLParams{
		Headers: getJWT(b, metaInfo),
		Query: `
			mutation addCuisine($cuisines: [AddCuisineInput!]!) {
		  		addCuisine(input: $cuisines) {
					numUids
		  		}
			}
		`,
		Variables: map[string]interface{}{"cuisines": c},
	}
	gqlResponse := getParams.ExecuteAsPost(b, graphqlURL)
	require.Nil(b, gqlResponse.Errors)
}

type Restaurants []Restaurant

func (r Restaurants) add(b require.TestingT, metaInfo *testutil.AuthMeta) {
	getParams := &common.GraphQLParams{
		Headers: getJWT(b, metaInfo),
		Query: `
			mutation AddR($restaurants: [AddRestaurantInput!]! ) {
				addRestaurant(input: $restaurants) {
    				numUids
  				}
			}
		`,
		Variables: map[string]interface{}{"restaurants": r},
	}
	gqlResponse := getParams.ExecuteAsPost(b, graphqlURL)
	require.Nil(b, gqlResponse.Errors)
}

func BenchmarkOneLevelMutation(b *testing.B) {
	schemaFile := "schema_auth.graphql"
	schema, err := ioutil.ReadFile(schemaFile)
	require.NoError(b, err)

	metaInfo := getAuthMeta(string(schema))
	metaInfo.AuthVars = map[string]interface{}{
		"Role":  "ADMIN",
		"Dish":  "Dish",
		"RName": "Restaurant",
		"RCurr": "$",
	}

	items := 10000
	var cusines Cuisines
	for i := 0; i < items; i++ {
		r := Cuisine{
			Name:   fmt.Sprintf("Test_Cuisine_%d", i),
			Type:   "TypeCuisineAuth",
			Public: true,
		}
		cusines = append(cusines, r)
	}
	mutations := []struct {
		name      string
		operation func(b *testing.B)
	}{
		{"add", func(b *testing.B) {
			before := time.Now()
			cusines.add(b, metaInfo)
			fmt.Println("Add Time: ", time.Since(before))
			b.StopTimer()
			cusines.delete(b, metaInfo)
		}},
		{"delete", func(b *testing.B) {
			b.StopTimer()
			cusines.add(b, metaInfo)
			b.StartTimer()
			before := time.Now()
			cusines.delete(b, metaInfo)
			fmt.Println("Delete Time: ", time.Since(before))
		}},
	}

	for _, mutation := range mutations {
		b.Run(mutation.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				mutation.operation(b)
			}
		})
	}
}

func generateMultiLevelMutationData(items int) Restaurants {
	var restaurants Restaurants
	ci := 1
	di := 1
	ri := 1
	for ; ri <= items; ri++ {
		r := Restaurant{
			Xid:      fmt.Sprintf("Test_Restaurant_%d", ri),
			Name:     "TypeRestaurantAuth",
			Currency: "$",
		}
		var cuisines Cuisines
		for ; ci%items != 0; ci++ {
			c := Cuisine{
				Name:   fmt.Sprintf("Test_Cuisine_%d", ci),
				Type:   "TypeCuisineAuth",
				Public: true,
			}
			var dishes []Dish
			for ; di%items != 0; di++ {
				d := Dish{
					Name: fmt.Sprintf("Test_Dish_%d", di),
					Type: "TypeDishAuth",
				}
				dishes = append(dishes, d)
			}
			di++
			c.Dishes = dishes
			cuisines = append(cuisines, c)
		}
		ci++
		r.Cuisines = cuisines
		restaurants = append(restaurants, r)
	}
	return restaurants
}

func BenchmarkMultiLevelMutation(b *testing.B) {
	schemaFile := "schema_auth.graphql"
	schema, err := ioutil.ReadFile(schemaFile)
	require.NoError(b, err)

	metaInfo := getAuthMeta(string(schema))
	metaInfo.AuthVars = map[string]interface{}{
		"Role":  "ADMIN",
		"Dish":  "Dish",
		"RName": "Restaurant",
		"RCurr": "$",
	}

	restaurants := generateMultiLevelMutationData(20)
	var totalTime time.Duration
	for i := 0; i < b.N; i++ {
		before := time.Now()
		restaurants.add(b, metaInfo)
		reqTime := time.Since(before)
		totalTime += reqTime
		if i%10 == 0 {
			avgTime := int64(totalTime) / int64(i+1)
			fmt.Printf("Avg Time: %d Time: %d \n", avgTime, reqTime)
		}
		// Stopping the timer as we don't want to include the clean up time in benchmark result.
		b.StopTimer()
		clearAll(b, metaInfo)
	}
}

// generateOwnerRestaurant generates `items` number of `Owner`. Each `Owner` having
// `items` number of `Restaurant`.
func generateOwnerRestaurant(items int) Owners {
	var owners Owners
	ri := 1
	oi := 1
	for ; oi < items; oi++ {
		var restaurants Restaurants
		for ; ri%items != 0; ri++ {
			r := Restaurant{
				Xid:      fmt.Sprintf("Test_Restaurant_%d", ri),
				Name:     "TypeRestaurantAuth",
				Currency: "$",
			}
			restaurants = append(restaurants, r)
		}
		ri++
		o := Owner{
			Username:       fmt.Sprintf("Test_User_%d", oi),
			Name:           "TypeOwnerAuth",
			HasRestaurants: restaurants,
		}
		owners = append(owners, o)
	}
	return owners
}

type Owners []Owner

func (o Owners) add(b *testing.B, metaInfo *testutil.AuthMeta) {
	getParams := &common.GraphQLParams{
		Headers: getJWT(b, metaInfo),
		Query: `
		mutation addOwner($owners: [AddOwnerInput!]!) {
			addOwner(input: $owners) {
				numUids
			}
		}
		`,
		Variables: map[string]interface{}{"owners": o},
	}
	gqlResponse := getParams.ExecuteAsPost(b, graphqlURL)
	require.Nil(b, gqlResponse.Errors)
}

func BenchmarkMutation(b *testing.B) {
	schemaFile := "schema_auth.graphql"
	schema, err := ioutil.ReadFile(schemaFile)
	require.NoError(b, err)

	metaInfo := getAuthMeta(string(schema))
	metaInfo.AuthVars = map[string]interface{}{
		"Role":     "ADMIN",
		"USERNAME": "$",
	}

	owners := generateOwnerRestaurant(100)
	owners.add(b, metaInfo)

	query := `
	query {
		queryRestaurant (first: 300000) {
	  		id 
	  		name
			owner {
				username
	  		}	
		}
	}
	`

	getUserParams := &common.GraphQLParams{
		Headers: getJWT(b, metaInfo),
		Query:   query,
	}

	var totalTime time.Duration
	for i := 0; i < b.N; i++ {
		before := time.Now()
		gqlResponse := getUserParams.ExecuteAsPost(b, graphqlURL)
		require.Nilf(b, gqlResponse.Errors, "%+v", gqlResponse.Errors)
		reqTime := time.Since(before)
		totalTime += reqTime
		if i%10 == 0 {
			avgTime := int64(totalTime) / (int64(i + 1))
			fmt.Printf("Avg Time: %d Time: %d \n", avgTime, reqTime)
		}
		// Stopping the timer as we don't want to include the clean up time in benchmark result.
		b.StopTimer()
		clearAll(b, metaInfo)
	}
}
