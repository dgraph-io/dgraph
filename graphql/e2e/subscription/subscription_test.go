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

package subscription_test

import (
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/stretchr/testify/require"
)

func TestSubscription(t *testing.T) {
	graphQLEndpoint := "http://localhost:8180/graphql"
	subscriptionEndpoint := "ws://localhost:8180/graphql"
	adminEndpoint := "http://localhost:8180/admin"

	sch := `
	type Product {
		productID: ID!
		name: String @search(by: [term])
		reviews: [Review] @hasInverse(field: about)
	}
	
	type Customer {
		username: String! @id @search(by: [hash, regexp])
		reviews: [Review] @hasInverse(field: by)
	}
	
	type Review {
		id: ID!
		about: Product!
		by: Customer!
		comment: String @search(by: [fulltext])
		rating: Int @search
	}
	`
	add := &common.GraphQLParams{
		Query: `mutation updateGQLSchema($sch: String!) {
			updateGQLSchema(input: { set: { schema: $sch }}) {
				gqlSchema {
					schema
				}
			}
		}`,
		Variables: map[string]interface{}{"sch": sch},
	}
	addResult := add.ExecuteAsPost(t, adminEndpoint)
	require.Nil(t, addResult.Errors)

	add = &common.GraphQLParams{
		Query: `mutation {
			addProduct(input: [
			  { name: "sanitizer"}
			]) {
			  product {
				productID
				name
			  }
			}
		  }`,
	}
	addResult = add.ExecuteAsPost(t, graphQLEndpoint)
	require.Nil(t, addResult.Errors)

	subscriptionClient, err := common.NewGraphQLSubscription(subscriptionEndpoint, &schema.Request{
		Query: `subscription{
			getProduct(productID: "0x2"){
			  name
			}
		  }`,
	})
	require.Nil(t, err)
	res, err := subscriptionClient.RecvMsg()

	require.NoError(t, err)

	require.JSONEq(t, `{"data":{"getProduct":{"name":"sanitizer"}}}`, string(res))

	// Background indexing is happening so wait till it get indexed.
	time.Sleep(time.Second * 2)

	// Update the product to get the latest update.
	add = &common.GraphQLParams{
		Query: `mutation{
			updateProduct(input:{filter:{name:{allofterms:"sanitizer"}}, set:{name:"mask"}},){
			  product{
				name
			  }
			}
		  }		  
		  `,
	}
	addResult = add.ExecuteAsPost(t, graphQLEndpoint)
	require.Nil(t, addResult.Errors)

	res, err = subscriptionClient.RecvMsg()
	require.NoError(t, err)

	// Check the latest update.
	require.JSONEq(t, `{"data":{"getProduct":{"name":"mask"}}}`, string(res))

	time.Sleep(2 * time.Second)
	// Change schema to terminate subscription..
	add = &common.GraphQLParams{
		Query: `mutation updateGQLSchema($sch: String!) {
			updateGQLSchema(input: { set: { schema: $sch }}) {
				gqlSchema {
					schema
				}
			}
		}`,
		Variables: map[string]interface{}{"sch": sch},
	}
	addResult = add.ExecuteAsPost(t, adminEndpoint)
	require.Nil(t, addResult.Errors)

	res, err = subscriptionClient.RecvMsg()
	require.NoError(t, err)

	require.Nil(t, res)
}
