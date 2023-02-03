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

package subscription_test

import (
	"encoding/json"
	"fmt"
	"os"
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
	sch = `
	type Product @withSubscription {
		productID: ID!
		name: String @search(by: [term])
		reviews: [Review] @hasInverse(field: about)
	}

	type Customer  {
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
	schAuth = `
	type Todo @withSubscription @auth(
    	query: { rule: """
    		query ($USER: String!) {
    			queryTodo(filter: { owner: { eq: $USER } } ) {
    				__typename
    			}
   			}"""
     	}
   ){
        id: ID!
    	text: String! @search(by: [term])
     	owner: String! @search(by: [hash])
   }
# Dgraph.Authorization {"VerificationKey":"secret","Header":"Authorization","Namespace":"https://dgraph.io","Algo":"HS256"}
`
	schCustomDQL = `
	type Tweets {
		id: ID!
		text: String! @search(by: [fulltext])
		author: User
		timestamp: DateTime @search
   }
   type User {
    	screen_name: String! @id
		followers: Int @search
		tweets: [Tweets] @hasInverse(field: author)
   }
   type UserTweetCount @remote {
		screen_name: String
		tweetCount: Int
   }

  type Query {
  	queryUserTweetCounts: [UserTweetCount] @withSubscription @custom(dql: """
		query {
			queryUserTweetCounts(func: type(User)) {
				screen_name: User.screen_name
				tweetCount: count(User.tweets)
			}
		}
		""")
	}`
	subExp       = 3 * time.Second
	pollInterval = time.Second
)

func TestSubscription(t *testing.T) {
	var subscriptionResp common.GraphQLResponse

	common.SafelyUpdateGQLSchemaOnAlpha1(t, sch)

	add := &common.GraphQLParams{
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
	addResult := add.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, addResult)
	time.Sleep(pollInterval)

	subscriptionClient, err := common.NewGraphQLSubscription(subscriptionEndpoint, &schema.Request{
		Query: `subscription{
			queryProduct{
			  name
			}
		  }`,
	}, `{}`)
	require.Nil(t, err)
	res, err := subscriptionClient.RecvMsg()
	require.NoError(t, err)

	touchedUidskey := "touched_uids"
	err = json.Unmarshal(res, &subscriptionResp)
	require.NoError(t, err)
	common.RequireNoGQLErrors(t, &subscriptionResp)

	require.JSONEq(t, `{"queryProduct":[{"name":"sanitizer"}]}`, string(subscriptionResp.Data))
	require.Contains(t, subscriptionResp.Extensions, touchedUidskey)
	require.Greater(t, int(subscriptionResp.Extensions[touchedUidskey].(float64)), 0)

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
	addResult = add.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, addResult)
	time.Sleep(pollInterval)

	res, err = subscriptionClient.RecvMsg()
	require.NoError(t, err)

	// makes sure that the we have a fresh instance to unmarshal to, otherwise there may be things
	// from the previous unmarshal
	subscriptionResp = common.GraphQLResponse{}
	err = json.Unmarshal(res, &subscriptionResp)
	require.NoError(t, err)
	common.RequireNoGQLErrors(t, &subscriptionResp)

	// Check the latest update.
	require.JSONEq(t, `{"queryProduct":[{"name":"mask"}]}`, string(subscriptionResp.Data))
	require.Contains(t, subscriptionResp.Extensions, touchedUidskey)
	require.Greater(t, int(subscriptionResp.Extensions[touchedUidskey].(float64)), 0)

	// Change schema to terminate subscription..
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schAuth)
	time.Sleep(pollInterval)
	res, err = subscriptionClient.RecvMsg()
	require.NoError(t, err)
	require.Nil(t, res)
}

func TestSubscriptionAuth(t *testing.T) {
	common.SafelyDropAll(t)

	common.SafelyUpdateGQLSchemaOnAlpha1(t, schAuth)

	metaInfo := &testutil.AuthMeta{
		PublicKey: "secret",
		Namespace: "https://dgraph.io",
		Algo:      "HS256",
		Header:    "Authorization",
	}
	metaInfo.AuthVars = map[string]interface{}{
		"USER": "jatin",
		"ROLE": "USER",
	}

	add := &common.GraphQLParams{
		Query: `mutation{
              addTodo(input: [
                 {text : "GraphQL is exciting!!",
                  owner : "jatin"}
               ])
             {
               todo{
                    text
                    owner
               }
           }
         }`,
	}

	addResult := add.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, addResult)
	time.Sleep(pollInterval)

	jwtToken, err := metaInfo.GetSignedToken("secret", subExp)
	require.NoError(t, err)

	payload := fmt.Sprintf(`{"Authorization": "%s"}`, jwtToken)
	subscriptionClient, err := common.NewGraphQLSubscription(subscriptionEndpoint, &schema.Request{
		Query: `subscription{
			queryTodo{
                owner
                text
			}
		}`,
	}, payload)
	require.Nil(t, err)

	res, err := subscriptionClient.RecvMsg()
	require.NoError(t, err)

	var resp common.GraphQLResponse
	err = json.Unmarshal(res, &resp)
	require.NoError(t, err)

	common.RequireNoGQLErrors(t, &resp)
	require.JSONEq(t, `{"queryTodo":[{"owner":"jatin","text":"GraphQL is exciting!!"}]}`,
		string(resp.Data))

	// Add a TODO for alice which should not be visible in the update because JWT belongs to
	// Jatin
	add = &common.GraphQLParams{
		Query: `mutation{
				 addTodo(input: [
					{text : "Dgraph is awesome!!",
					 owner : "alice"}
				  ])
				{
				  todo {
					   text
					   owner
				  }
			  }
			}`,
	}
	addResult = add.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, addResult)

	// Add another TODO for jatin which we should get in the latest update.
	add = &common.GraphQLParams{
		Query: `mutation{
	         addTodo(input: [
	            {text : "Dgraph is awesome!!",
	             owner : "jatin"}
	          ])
	        {
	          todo {
	               text
	               owner
	          }
	      }
	    }`,
	}

	addResult = add.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, addResult)
	time.Sleep(pollInterval)

	res, err = subscriptionClient.RecvMsg()
	require.NoError(t, err)

	err = json.Unmarshal(res, &resp)
	require.NoError(t, err)
	common.RequireNoGQLErrors(t, &resp)
	require.JSONEq(t, `{"queryTodo": [
	 {
	   "owner": "jatin",
	   "text": "GraphQL is exciting!!"
	 },
	{
	   "owner" : "jatin",
	   "text" : "Dgraph is awesome!!"
	}]}`, string(resp.Data))

	// Terminate Subscription
	subscriptionClient.Terminate()
}

func TestSubscriptionWithAuthShouldExpireWithJWT(t *testing.T) {
	common.SafelyDropAll(t)

	common.SafelyUpdateGQLSchemaOnAlpha1(t, schAuth)

	metaInfo := &testutil.AuthMeta{
		PublicKey: "secret",
		Namespace: "https://dgraph.io",
		Algo:      "HS256",
		Header:    "Authorization",
	}
	metaInfo.AuthVars = map[string]interface{}{
		"USER": "bob",
		"ROLE": "USER",
	}

	add := &common.GraphQLParams{
		Query: `mutation{
              addTodo(input: [
                 {text : "GraphQL is exciting!!",
                  owner : "bob"}
               ])
             {
               todo{
                    text
                    owner
               }
           }
         }`,
	}

	addResult := add.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, addResult)
	time.Sleep(pollInterval)

	jwtToken, err := metaInfo.GetSignedToken("secret", subExp)
	require.NoError(t, err)

	payload := fmt.Sprintf(`{"Authorization": "%s"}`, jwtToken)
	subscriptionClient, err := common.NewGraphQLSubscription(subscriptionEndpoint,
		&schema.Request{
			Query: `subscription{
			queryTodo{
                owner
                text
			}
		}`,
		}, payload)
	require.Nil(t, err)

	res, err := subscriptionClient.RecvMsg()
	require.NoError(t, err)

	var resp common.GraphQLResponse
	err = json.Unmarshal(res, &resp)
	require.NoError(t, err)

	common.RequireNoGQLErrors(t, &resp)
	require.JSONEq(t, `{"queryTodo":[{"owner":"bob","text":"GraphQL is exciting!!"}]}`,
		string(resp.Data))

	// Wait for JWT to expire.
	time.Sleep(subExp)

	// Add another TODO for bob but this should not be visible as the subscription should have
	// ended.
	add = &common.GraphQLParams{
		Query: `mutation{
              addTodo(input: [
                 {text : "Dgraph is exciting!!",
                  owner : "bob"}
               ])
             {
               todo{
                    text
                    owner
               }
           }
         }`,
	}

	addResult = add.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, addResult)
	time.Sleep(pollInterval)

	res, err = subscriptionClient.RecvMsg()
	require.NoError(t, err)
	require.Nil(t, res)
	// Terminate Subscription
	subscriptionClient.Terminate()
}

func TestSubscriptionAuthWithoutExpiry(t *testing.T) {
	common.SafelyDropAll(t)

	common.SafelyUpdateGQLSchemaOnAlpha1(t, schAuth)

	metaInfo := &testutil.AuthMeta{
		PublicKey: "secret",
		Namespace: "https://dgraph.io",
		Algo:      "HS256",
		Header:    "Authorization",
	}
	metaInfo.AuthVars = map[string]interface{}{
		"USER": "jatin",
		"ROLE": "USER",
	}

	add := &common.GraphQLParams{
		Query: `mutation{
              addTodo(input: [
                 {text : "GraphQL is exciting!!",
                  owner : "jatin"}
               ])
             {
               todo{
                    text
                    owner
               }
           }
         }`,
	}

	addResult := add.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, addResult)

	jwtToken, err := metaInfo.GetSignedToken("secret", -1)
	require.NoError(t, err)

	payload := fmt.Sprintf(`{"Authorization": "%s"}`, jwtToken)
	subscriptionClient, err := common.NewGraphQLSubscription(subscriptionEndpoint, &schema.Request{
		Query: `subscription{
			queryTodo{
                owner
                text
			}
		}`,
	}, payload)
	require.Nil(t, err)

	res, err := subscriptionClient.RecvMsg()
	require.NoError(t, err)

	var resp common.GraphQLResponse
	err = json.Unmarshal(res, &resp)
	require.NoError(t, err)

	common.RequireNoGQLErrors(t, &resp)
	require.JSONEq(t, `{"queryTodo":[{"owner":"jatin","text":"GraphQL is exciting!!"}]}`,
		string(resp.Data))
}

func TestSubscriptionAuth_SameQueryAndClaimsButDifferentExpiry_ShouldExpireIndependently(t *testing.T) {
	common.SafelyDropAll(t)

	common.SafelyUpdateGQLSchemaOnAlpha1(t, schAuth)

	metaInfo := &testutil.AuthMeta{
		PublicKey: "secret",
		Namespace: "https://dgraph.io",
		Algo:      "HS256",
		Header:    "Authorization",
	}
	metaInfo.AuthVars = map[string]interface{}{
		"USER": "jatin",
		"ROLE": "USER",
	}

	add := &common.GraphQLParams{
		Query: `mutation{
              addTodo(input: [
                 {text : "GraphQL is exciting!!",
                  owner : "jatin"}
               ])
             {
               todo{
                    text
                    owner
               }
           }
         }`,
	}

	addResult := add.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, addResult)
	time.Sleep(pollInterval)

	jwtToken, err := metaInfo.GetSignedToken("secret", subExp)
	require.NoError(t, err)

	// first subscription
	payload := fmt.Sprintf(`{"Authorization": "%s"}`, jwtToken)
	subscriptionClient, err := common.NewGraphQLSubscription(subscriptionEndpoint, &schema.Request{
		Query: `subscription{
			queryTodo{
                owner
                text
			}
		}`,
	}, payload)
	require.Nil(t, err)

	res, err := subscriptionClient.RecvMsg()
	require.NoError(t, err)

	var resp common.GraphQLResponse
	err = json.Unmarshal(res, &resp)
	require.NoError(t, err)
	common.RequireNoGQLErrors(t, &resp)
	require.JSONEq(t, `{"queryTodo":[{"owner":"jatin","text":"GraphQL is exciting!!"}]}`,
		string(resp.Data))

	// 2nd subscription
	jwtToken, err = metaInfo.GetSignedToken("secret", 2*subExp)
	require.NoError(t, err)
	payload = fmt.Sprintf(`{"Authorization": "%s"}`, jwtToken)
	subscriptionClient1, err := common.NewGraphQLSubscription(subscriptionEndpoint, &schema.Request{
		Query: `subscription{
			queryTodo{
                owner
                text
			}
		}`,
	}, payload)
	require.Nil(t, err)

	res, err = subscriptionClient1.RecvMsg()
	require.NoError(t, err)
	err = json.Unmarshal(res, &resp)
	require.NoError(t, err)
	common.RequireNoGQLErrors(t, &resp)
	require.JSONEq(t, `{"queryTodo":[{"owner":"jatin","text":"GraphQL is exciting!!"}]}`,
		string(resp.Data))

	// Wait for JWT to expire for first subscription.
	time.Sleep(subExp)

	// Add another TODO for jatin for which 1st subscription shouldn't get updates.
	add = &common.GraphQLParams{
		Query: `mutation{
	         addTodo(input: [
	            {text : "Dgraph is awesome!!",
	             owner : "jatin"}
	          ])
	        {
	          todo {
	               text
	               owner
	          }
	      }
	    }`,
	}
	addResult = add.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, addResult)
	time.Sleep(pollInterval)

	res, err = subscriptionClient.RecvMsg()
	require.NoError(t, err)
	require.Nil(t, res) // 1st subscription should get the empty response as subscription has expired.

	res, err = subscriptionClient1.RecvMsg()
	require.NoError(t, err)
	err = json.Unmarshal(res, &resp)
	require.NoError(t, err)
	common.RequireNoGQLErrors(t, &resp)
	// 2nd one still running and should get the  update
	require.JSONEq(t, `{"queryTodo": [
	 {
	   "owner": "jatin",
	   "text": "GraphQL is exciting!!"
	 },
	{
	   "owner" : "jatin",
	   "text" : "Dgraph is awesome!!"
	}]}`, string(resp.Data))

	// add extra delay for 2nd subscription to timeout
	time.Sleep(subExp)
	// Add another TODO for jatin for which 2nd subscription shouldn't get update.
	add = &common.GraphQLParams{
		Query: `mutation{
	         addTodo(input: [
	            {text : "Graph Database is the future!!",
	             owner : "jatin"}
	          ])
	        {
	          todo {
	               text
	               owner
	          }
	      }
	    }`,
	}
	addResult = add.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, addResult)
	time.Sleep(pollInterval)

	// 2nd subscription should get the empty response as subscription has expired.
	res, err = subscriptionClient1.RecvMsg()
	require.NoError(t, err)
	require.Nil(t, res)
}

func TestSubscriptionAuth_SameQueryDifferentClaimsAndExpiry_ShouldExpireIndependently(t *testing.T) {
	common.SafelyDropAll(t)

	common.SafelyUpdateGQLSchemaOnAlpha1(t, schAuth)

	metaInfo := &testutil.AuthMeta{
		PublicKey: "secret",
		Namespace: "https://dgraph.io",
		Algo:      "HS256",
		Header:    "Authorization",
	}
	metaInfo.AuthVars = map[string]interface{}{
		"USER": "jatin",
		"ROLE": "USER",
	}
	// for user jatin
	add := &common.GraphQLParams{
		Query: `mutation{
              addTodo(input: [
                 {text : "GraphQL is exciting!!",
                  owner : "jatin"}
               ])
             {
               todo{
                    text
                    owner
               }
           }
         }`,
	}

	addResult := add.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, addResult)
	time.Sleep(pollInterval)

	jwtToken, err := metaInfo.GetSignedToken("secret", subExp)
	require.NoError(t, err)

	// first subscription
	payload := fmt.Sprintf(`{"Authorization": "%s"}`, jwtToken)
	subscriptionClient, err := common.NewGraphQLSubscription(subscriptionEndpoint, &schema.Request{
		Query: `subscription{
			queryTodo{
                owner
                text
			}
		}`,
	}, payload)
	require.Nil(t, err)

	res, err := subscriptionClient.RecvMsg()
	require.NoError(t, err)

	var resp common.GraphQLResponse
	err = json.Unmarshal(res, &resp)
	require.NoError(t, err)

	common.RequireNoGQLErrors(t, &resp)
	require.JSONEq(t, `{"queryTodo":[{"owner":"jatin","text":"GraphQL is exciting!!"}]}`,
		string(resp.Data))

	// for user pawan
	add = &common.GraphQLParams{
		Query: `mutation{
              addTodo(input: [
                 {text : "GraphQL is exciting!!",
                  owner : "pawan"}
               ])
             {
               todo {
                    text
                    owner
               }
           }
         }`,
	}

	addResult = add.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, addResult)
	time.Sleep(pollInterval)

	// 2nd subscription
	metaInfo.AuthVars["USER"] = "pawan"
	jwtToken, err = metaInfo.GetSignedToken("secret", 2*subExp)
	require.NoError(t, err)
	payload = fmt.Sprintf(`{"Authorization": "%s"}`, jwtToken)
	subscriptionClient1, err := common.NewGraphQLSubscription(subscriptionEndpoint, &schema.Request{
		Query: `subscription{
			queryTodo{
                owner
                text
			}
		}`,
	}, payload)
	require.Nil(t, err)

	res, err = subscriptionClient1.RecvMsg()
	require.NoError(t, err)

	err = json.Unmarshal(res, &resp)
	require.NoError(t, err)

	common.RequireNoGQLErrors(t, &resp)
	require.JSONEq(t, `{"queryTodo":[{"owner":"pawan","text":"GraphQL is exciting!!"}]}`,
		string(resp.Data))

	// Wait for JWT to expire for 1st subscription.
	time.Sleep(subExp)

	// Add another TODO for jatin for which 1st subscription shouldn't get updates.
	add = &common.GraphQLParams{
		Query: `mutation{
	         addTodo(input: [
	            {text : "Dgraph is awesome!!",
	             owner : "jatin"}
	          ])
	        {
	          todo {
	               text
	               owner
	          }
	      }
	    }`,
	}
	addResult = add.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, addResult)
	time.Sleep(pollInterval)
	// 1st subscription should get the empty response as subscription has expired
	res, err = subscriptionClient.RecvMsg()
	require.NoError(t, err)
	require.Nil(t, res)

	// Add another TODO for pawan which we should get in the latest update of 2nd subscription.
	add = &common.GraphQLParams{
		Query: `mutation{
	         addTodo(input: [
	            {text : "Dgraph is awesome!!",
	             owner : "pawan"}
	          ])
	        {
	          todo {
	               text
	               owner
	          }
	      }
	    }`,
	}
	addResult = add.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, addResult)
	time.Sleep(pollInterval)

	res, err = subscriptionClient1.RecvMsg()
	require.NoError(t, err)
	err = json.Unmarshal(res, &resp)
	require.NoError(t, err)
	common.RequireNoGQLErrors(t, &resp)
	// 2nd one still running and should get the  update
	require.JSONEq(t, `{"queryTodo": [
	 {
	   "owner": "pawan",
	   "text": "GraphQL is exciting!!"
	 },
	{
	   "owner" : "pawan",
	   "text" : "Dgraph is awesome!!"
	}]}`, string(resp.Data))

	// add delay for 2nd subscription  to timeout
	// Wait for JWT to expire.
	time.Sleep(subExp)
	// Add another TODO for pawan for which 2nd subscription shouldn't get updates.
	add = &common.GraphQLParams{
		Query: `mutation{
	         addTodo(input: [
	            {text : "Graph Database is the future!!",
	             owner : "pawan"}
	          ])
	        {
	          todo {
	               text
	               owner
	          }
	      }
	    }`,
	}

	addResult = add.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, addResult)
	time.Sleep(pollInterval)

	// 2nd subscription should get the empty response as subscription has expired
	res, err = subscriptionClient1.RecvMsg()
	require.NoError(t, err)
	require.Nil(t, res)
}

func TestSubscriptionAuthHeaderCaseInsensitive(t *testing.T) {
	common.SafelyDropAll(t)

	common.SafelyUpdateGQLSchemaOnAlpha1(t, schAuth)

	metaInfo := &testutil.AuthMeta{
		PublicKey: "secret",
		Namespace: "https://dgraph.io",
		Algo:      "HS256",
		Header:    "authorization",
	}
	metaInfo.AuthVars = map[string]interface{}{
		"USER": "jatin",
		"ROLE": "USER",
	}

	add := &common.GraphQLParams{
		Query: `mutation{
              addTodo(input: [
                 {text : "GraphQL is exciting!!",
                  owner : "jatin"}
               ])
             {
               todo{
                    text
                    owner
               }
           }
         }`,
	}

	addResult := add.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, addResult)

	jwtToken, err := metaInfo.GetSignedToken("secret", -1)
	require.NoError(t, err)

	payload := fmt.Sprintf(`{"Authorization": "%s"}`, jwtToken)
	subscriptionClient, err := common.NewGraphQLSubscription(subscriptionEndpoint, &schema.Request{
		Query: `subscription{
			queryTodo{
                owner
                text
			}
		}`,
	}, payload)
	require.Nil(t, err)

	res, err := subscriptionClient.RecvMsg()
	require.NoError(t, err)

	var resp common.GraphQLResponse
	err = json.Unmarshal(res, &resp)
	require.NoError(t, err)

	common.RequireNoGQLErrors(t, &resp)
	require.JSONEq(t, `{"queryTodo":[{"owner":"jatin","text":"GraphQL is exciting!!"}]}`,
		string(resp.Data))

	// Terminate Subscriptions
	subscriptionClient.Terminate()
}

func TestSubscriptionAuth_MultiSubscriptionResponses(t *testing.T) {
	common.SafelyDropAll(t)

	// Upload schema
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schAuth)

	metaInfo := &testutil.AuthMeta{
		PublicKey: "secret",
		Namespace: "https://dgraph.io",
		Algo:      "HS256",
		Header:    "Authorization",
	}
	metaInfo.AuthVars = map[string]interface{}{
		"USER": "jatin",
		"ROLE": "USER",
	}

	jwtToken, err := metaInfo.GetSignedToken("secret", -1)
	require.NoError(t, err)

	payload := fmt.Sprintf(`{"Authorization": "%s"}`, jwtToken)
	// first Subscription
	subscriptionClient, err := common.NewGraphQLSubscription(subscriptionEndpoint, &schema.Request{
		Query: `subscription{
			queryTodo{
                owner
                text
			}
		}`,
	}, payload)
	require.Nil(t, err)

	res, err := subscriptionClient.RecvMsg()
	require.NoError(t, err)

	var resp common.GraphQLResponse
	err = json.Unmarshal(res, &resp)
	require.NoError(t, err)

	common.RequireNoGQLErrors(t, &resp)
	require.JSONEq(t, `{"queryTodo":[]}`,
		string(resp.Data))
	// Terminate subscription and wait for poll interval before starting new subscription
	subscriptionClient.Terminate()
	time.Sleep(pollInterval)

	jwtToken, err = metaInfo.GetSignedToken("secret", 3*time.Second)
	require.NoError(t, err)

	payload = fmt.Sprintf(`{"Authorization": "%s"}`, jwtToken)
	// Second Subscription
	subscriptionClient1, err := common.NewGraphQLSubscription(subscriptionEndpoint, &schema.Request{
		Query: `subscription{
			queryTodo{
                owner
                text
			}
		}`,
	}, payload)
	require.Nil(t, err)

	res, err = subscriptionClient1.RecvMsg()
	require.NoError(t, err)

	err = json.Unmarshal(res, &resp)
	require.NoError(t, err)

	common.RequireNoGQLErrors(t, &resp)
	require.JSONEq(t, `{"queryTodo":[]}`,
		string(resp.Data))

	add := &common.GraphQLParams{
		Query: `mutation{
              addTodo(input: [
                 {text : "GraphQL is exciting!!",
                  owner : "jatin"}
               ])
             {
               todo{
                    text
                    owner
               }
           }
         }`,
	}

	addResult := add.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, addResult)
	time.Sleep(pollInterval)

	// 1st response
	res, err = subscriptionClient1.RecvMsg()
	require.NoError(t, err)
	err = json.Unmarshal(res, &resp)
	require.NoError(t, err)

	common.RequireNoGQLErrors(t, &resp)
	require.JSONEq(t, `{"queryTodo":[{"owner":"jatin","text":"GraphQL is exciting!!"}]}`,
		string(resp.Data))

	// second response should be nil
	res, err = subscriptionClient1.RecvMsg()
	require.NoError(t, err)
	require.Nil(t, res)
	// Terminate Subscription
	subscriptionClient1.Terminate()
}

func TestSubscriptionWithCustomDQL(t *testing.T) {
	common.SafelyDropAll(t)
	var subscriptionResp common.GraphQLResponse

	common.SafelyUpdateGQLSchemaOnAlpha1(t, schCustomDQL)

	add := &common.GraphQLParams{
		Query: `mutation  {
				addTweets(input: [
					{text: "Graphql is best",author:{screen_name:"001"}},
				]) {
				    numUids
				    tweets {
				    	text
					}
				}
			}`,
	}
	addResult := add.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, addResult)
	time.Sleep(pollInterval)

	subscriptionClient, err := common.NewGraphQLSubscription(subscriptionEndpoint, &schema.Request{
		Query: `subscription {
					queryUserTweetCounts{
						screen_name
						tweetCount
					}
				}`,
	}, `{}`)
	require.Nil(t, err)
	res, err := subscriptionClient.RecvMsg()
	require.NoError(t, err)

	touchedUidskey := "touched_uids"
	err = json.Unmarshal(res, &subscriptionResp)
	require.NoError(t, err)
	common.RequireNoGQLErrors(t, &subscriptionResp)

	require.JSONEq(t, `{"queryUserTweetCounts":[{"screen_name":"001","tweetCount": 1}]}`, string(subscriptionResp.Data))
	require.Contains(t, subscriptionResp.Extensions, touchedUidskey)
	require.Greater(t, int(subscriptionResp.Extensions[touchedUidskey].(float64)), 0)

	// add new tweets to get the latest update.
	add = &common.GraphQLParams{
		Query: `mutation  {
				addTweets(input: [
					{text: "Dgraph is best",author:{screen_name:"002"}}
                    {text: "Badger is best",author:{screen_name:"001"}},
				]) {
				    numUids
				    tweets {
				    	text
					}
				}
			}`,
	}
	addResult = add.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, addResult)
	time.Sleep(pollInterval)

	res, err = subscriptionClient.RecvMsg()
	require.NoError(t, err)

	// makes sure that the we have a fresh instance to unmarshal to, otherwise there may be things
	// from the previous unmarshal
	subscriptionResp = common.GraphQLResponse{}
	err = json.Unmarshal(res, &subscriptionResp)
	require.NoError(t, err)
	common.RequireNoGQLErrors(t, &subscriptionResp)

	// Check the latest update.
	require.JSONEq(t, `{"queryUserTweetCounts":[{"screen_name":"001","tweetCount": 2},{"screen_name":"002","tweetCount": 1}]}`, string(subscriptionResp.Data))
	require.Contains(t, subscriptionResp.Extensions, touchedUidskey)
	require.Greater(t, int(subscriptionResp.Extensions[touchedUidskey].(float64)), 0)

	// Change schema to terminate subscription..
	common.SafelyUpdateGQLSchemaOnAlpha1(t, schAuth)
	time.Sleep(pollInterval)
	res, err = subscriptionClient.RecvMsg()
	require.NoError(t, err)
	require.Nil(t, res)
}

func TestMain(m *testing.M) {
	err := common.CheckGraphQLStarted(common.GraphqlAdminURL)
	if err != nil {
		x.Log(err, "Waited for GraphQL test server to become available, but it never did.")
		os.Exit(1)
	}
	os.Exit(m.Run())
}
