//go:build integration

/*
 * Copyright 2016-2023 Dgraph Labs, Inc. and Contributors
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

package query

import (
	"encoding/json"
	"testing"

	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/stretchr/testify/require"
)

const (
	graphQLVectorSchema = `
	type Project  {
		id: ID!
		title: String!  @search(by: [exact])
		title_v: [Float!] @hm_embedding @search(by: ["hnsw(metric: euclidian, exponent: 4)"]) 
	}
	`
)

type ProjectInput struct {
	Title  string    `json:"title"`
	TitleV []float32 `json:"title_v"`
}

func TestVectorGraphQLAddVectorPredicate(t *testing.T) {
	require.NoError(t, client.DropAll())

	hc, err := dc.HTTPClient()
	require.NoError(t, err)
	hc.LoginIntoNamespace("groot", "password", 0)
	// add schema
	require.NoError(t, hc.UpdateGQLSchema(graphQLVectorSchema))
}

func TestVectorGraphQlMutationAndQuery(t *testing.T) {
	require.NoError(t, client.DropAll())

	hc, err := dc.HTTPClient()
	require.NoError(t, err)
	hc.LoginIntoNamespace("groot", "password", 0)

	// add schema
	require.NoError(t, hc.UpdateGQLSchema(graphQLVectorSchema))

	// add project
	project := ProjectInput{
		Title:  "iCreate with a Mini iPad",
		TitleV: []float32{0.1, 0.2, 0.3, 0.4, 0.5},
	}

	query := `
	 mutation addProject($project: AddProjectInput!) {
		 addProject(input: [$project]) {
			 project {
				   title
				   title_v
			 }
		 }
	 }`

	params := dgraphtest.GraphQLParams{
		Query:     query,
		Variables: map[string]interface{}{"project": project},
	}

	_, err = hc.RunGraphqlQuery(params, false)
	require.NoError(t, err)
	// query project
	queryProduct := `query {
		 queryProject(filter: { title: { eq: "iCreate with a Mini iPad" } }) {
		   title
		   title_v
		 }
	   }
	 `

	params = dgraphtest.GraphQLParams{
		Query: queryProduct,
	}
	response, err := hc.RunGraphqlQuery(params, false)
	require.NoError(t, err)

	type QueryResult struct {
		QueryProject []ProjectInput `json:"queryProject"`
	}

	var resp QueryResult
	err = json.Unmarshal([]byte(string(response)), &resp)
	require.NoError(t, err)
	require.Equal(t, project.Title, resp.QueryProject[0].Title)
	require.Equal(t, project.TitleV, resp.QueryProject[0].TitleV)

	// query similar project by embedding
	queryProduct = `query {
		querySimilarProjectByEmbedding(
		  by: title_v
		  topK: 1
		  vector: [0.1, 0.2, 0.3, 0.4, 0.5]
		) {
		  id
		  title
		  title_v
		}
	  }
	`

	params = dgraphtest.GraphQLParams{
		Query: queryProduct,
	}
	response, err = hc.RunGraphqlQuery(params, false)
	require.NoError(t, err)

	type QueryResult1 struct {
		QuerySimilarProjectByEmbedding []ProjectInput `json:"querySimilarProjectByEmbedding"`
	}

	var resp1 QueryResult1
	err = json.Unmarshal([]byte(string(response)), &resp1)
	require.NoError(t, err)

	require.Equal(t, project.Title, resp1.QuerySimilarProjectByEmbedding[0].Title)
	require.Equal(t, project.TitleV, resp1.QuerySimilarProjectByEmbedding[0].TitleV)
}
