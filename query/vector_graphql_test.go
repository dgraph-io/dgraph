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

type ProjectInput struct {
	Title  string    `json:"title"`
	TitleV []float32 `json:"title_v"`
}

const (
	graphQLVectorSchema = `
	 type Project  {
		 id: ID!
		 title: String!  @search(by: [exact])
		 title_v: [Float!] @embedding @search(by: ["hnsw(metric: euclidian, exponent: 4)"]) 
	 }
	 `
)

var (
	projects = []ProjectInput{ProjectInput{
		Title:  "iCreate with a Mini iPad",
		TitleV: []float32{0.7, 0.8, 0.9, 0.1, 0.2},
	}, ProjectInput{
		Title:  "Resistive Touchscreen",
		TitleV: []float32{0.7, 0.8, 0.9, 0.1, 0.2},
	}, ProjectInput{
		Title:  "Fitness Band",
		TitleV: []float32{0.7, 0.8, 0.9, 0.1, 0.2},
	}, ProjectInput{
		Title:  "Smart Watch",
		TitleV: []float32{0.7, 0.8, 0.9, 0.1, 0.2},
	}, ProjectInput{
		Title:  "Smart Ring",
		TitleV: []float32{0.7, 0.8, 0.9, 0.1, 0.2},
	}}
)

func addProject(t *testing.T, hc *dgraphtest.HTTPClient, project ProjectInput) {
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

	_, err := hc.RunGraphqlQuery(params, false)
	require.NoError(t, err)
}
func queryProjectUsingTitle(t *testing.T, hc *dgraphtest.HTTPClient, title string) ProjectInput {
	query := ` QueryProject($title: String!) {
		 queryProject(filter: { title: { eq: $title } }) {
		   title
		   title_v
		 }
	   }`

	params := dgraphtest.GraphQLParams{
		Query:     query,
		Variables: map[string]interface{}{"title": title},
	}
	response, err := hc.RunGraphqlQuery(params, false)
	require.NoError(t, err)
	type QueryResult struct {
		QueryProject []ProjectInput `json:"queryProject"`
	}

	var resp QueryResult
	err = json.Unmarshal([]byte(string(response)), &resp)
	require.NoError(t, err)

	return resp.QueryProject[0]
}

func queryProjectsSimilarByEmbedding(t *testing.T, hc *dgraphtest.HTTPClient, vector []float32) []ProjectInput {
	// query similar project by embedding
	queryProduct := `query QuerySimilarProjectByEmbedding($by: String!, $topK: Int!, $vector: [Float!]!) {
		 querySimilarProjectByEmbedding(by: $by, topK: $topK, vector: $vector) {
		   id
		   title
		   title_v
		 }
	   }
	   
	 `

	params := dgraphtest.GraphQLParams{
		Query: queryProduct,
		Variables: map[string]interface{}{
			"by":     "title_v",
			"topK":   3,
			"vector": vector,
		}}
	response, err := hc.RunGraphqlQuery(params, false)
	require.NoError(t, err)
	type QueryResult struct {
		QueryProject []ProjectInput `json:"queryProject"`
	}
	var resp QueryResult
	err = json.Unmarshal([]byte(string(response)), &resp)
	require.NoError(t, err)

	return resp.QueryProject

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
	var vectors [][]float32
	for _, project := range projects {
		vectors = append(vectors, project.TitleV)
		addProject(t, hc, project)
	}

	for _, project := range projects {
		p := queryProjectUsingTitle(t, hc, project.Title)
		require.Equal(t, project.Title, p.Title)
		require.Equal(t, project.TitleV, p.TitleV)
	}

	for _, project := range projects {
		p := queryProjectUsingTitle(t, hc, project.Title)
		require.Equal(t, project.Title, p.Title)
		require.Equal(t, project.TitleV, p.TitleV)
	}

	// query similar project by embedding
	for _, project := range projects {
		similarProjects := queryProjectsSimilarByEmbedding(t, hc, project.TitleV)

		for _, similarVec := range similarProjects {
			require.Contains(t, vectors, similarVec.TitleV)
		}
	}
}

func TestVectorSchema(t *testing.T) {
	require.NoError(t, client.DropAll())

	hc, err := dc.HTTPClient()
	require.NoError(t, err)
	hc.LoginIntoNamespace("groot", "password", 0)

	schema := `type Project  {
		 id: ID!
		 title: String!  @search(by: [exact])
		 title_v: [Float!] 
	 }`

	// add schema
	require.NoError(t, hc.UpdateGQLSchema(schema))

	require.NoError(t, hc.UpdateGQLSchema(graphQLVectorSchema))
}
