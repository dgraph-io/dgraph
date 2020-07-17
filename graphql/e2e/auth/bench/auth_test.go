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
//    "touched_uids": 8410962,
//    "tracing": {
//      "version": 1,
//      "startTime": "2020-07-16T23:45:27.798693638+05:30",
//      "endTime": "2020-07-16T23:45:28.844749169+05:30",
//      "duration": 1046055551,
//      "execution": {
//        "resolvers": [
//          {
//            "path": [
//              "queryCuisine"
//            ],
//            "parentType": "Query",
//            "fieldName": "queryCuisine",
//            "returnType": "[Cuisine]",
//            "startOffset": 144549,
//            "duration": 1045026189,
//            "dgraph": [
//              {
//                "label": "query",
//                "startOffset": 262828,
//                "duration": 1044381745
//              }
//            ]
//          }
//        ]
//      }
//    }
//  }

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
