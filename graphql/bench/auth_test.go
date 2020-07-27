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

package bench

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/dgraph-io/dgraph/graphql/authorization"
	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/stretchr/testify/require"
)

const (
	graphqlURL = "http://100.83.146.128:8080/graphql"
)

func getJWT(b *testing.B, metaInfo *testutil.AuthMeta) http.Header {
	jwtToken, err := metaInfo.GetSignedToken("")
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

func clearAll(b *testing.B, metaInfo *testutil.AuthMeta) {
	getParams := &common.GraphQLParams{
		Headers: getJWT(b, metaInfo),
		Query: `
		mutation {
  			deleteCuisine(filter: {}) {
    			msg
  			}
		}
		`,
	}
	gqlResponse := getParams.ExecuteAsPost(b, graphqlURL)
	require.Nil(b, gqlResponse.Errors)

	getParams = &common.GraphQLParams{
		Headers: getJWT(b, metaInfo),
		Query: `
		mutation {
  			deleteRestaurant(filter: {}) {
    			msg
  			}
		}
		`,
	}
	gqlResponse = getParams.ExecuteAsPost(b, graphqlURL)
	require.Nil(b, gqlResponse.Errors)

	getParams = &common.GraphQLParams{
		Headers: getJWT(b, metaInfo),
		Query: `
		mutation {
  			deleteDish(filter: {}) {
    			msg
  			}
		}
		`,
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
	schemaFile := "schema.graphql"
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
	schemaFile := "schema.graphql"
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

type Cuisines []Cuisine

func (c Cuisines) delete(b *testing.B, metaInfo *testutil.AuthMeta) {
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

func (c Cuisines) add(b *testing.B, metaInfo *testutil.AuthMeta) {
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

func (r Restaurants) add(b *testing.B, metaInfo *testutil.AuthMeta) {
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
	schemaFile := "schema.graphql"
	schema, err := ioutil.ReadFile(schemaFile)
	require.NoError(b, err)

	metaInfo := getAuthMeta(string(schema))
	metaInfo.AuthVars = map[string]interface{}{
		"Role":  "ADMIN",
		"Dish":  "Dish",
		"RName": "Restaurant",
		"RCurr": "$",
	}

	items := 1000
	var cusines Cuisines
	for i := 0; i < items/2; i++ {
		r := Cuisine{
			Name:   fmt.Sprintf("Test_Cuisine_%d", i),
			Type:   "TypeCuisineAuth",
			Public: true,
		}
		cusines = append(cusines, r)
	}

	for i := items / 2; i < items; i++ {
		r := Cuisine{
			Name:   fmt.Sprintf("Test_Cuisine_%d", i),
			Type:   "TypeCuisineAuth",
			Public: false,
		}
		cusines = append(cusines, r)
	}

	mutations := []struct {
		name      string
		operation func(b *testing.B)
	}{
		{"add", func(b *testing.B) {
			cusines.add(b, metaInfo)
			b.StopTimer()
			cusines.delete(b, metaInfo)
		}},
		{"delete", func(b *testing.B) {
			b.StopTimer()
			cusines.add(b, metaInfo)
			b.StartTimer()
			cusines.delete(b, metaInfo)
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

func BenchmarkMultiLevelMutation(b *testing.B) {
	schemaFile := "schema.graphql"
	schema, err := ioutil.ReadFile(schemaFile)
	require.NoError(b, err)

	metaInfo := getAuthMeta(string(schema))
	metaInfo.AuthVars = map[string]interface{}{
		"Role":  "ADMIN",
		"Dish":  "Dish",
		"RName": "Restaurant",
		"RCurr": "$",
	}

	var restaurants Restaurants
	items := 5
	ci := 1
	di := 1
	for ri := 1; ri <= items; ri++ {
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
			ci++
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
		r.Cuisines = cuisines
		restaurants = append(restaurants, r)
	}

	mutations := []struct {
		name      string
		operation func(b *testing.B)
	}{
		{"add", func(b *testing.B) {
			restaurants.add(b, metaInfo)
			b.StopTimer()
			clearAll(b, metaInfo)
		}},
	}

	for _, mutation := range mutations {
		b.Run(mutation.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				mutation.operation(b)
			}
		})
	}
	clearAll(b, metaInfo)
}
