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
	"fmt"
	"testing"

	"github.com/dgraph-io/dgraph/dgraphtest"
	"github.com/stretchr/testify/require"
)

const (
	exampleSchema = ` 
	 type Project  {
		 id: ID!
		 title: String!  @search(by: [term])
		 grade: String @search(by: [hash])
		 category: Category
		 score: Float
		 title_v: [Float!] @hm_embedding @search(by: ["hnsw(metric: euclidian, exponent: 4)"]) 
	 }
	
	 type Category  {
		 id: ID!
		 name: String!
		 name_v: [Float!] @hm_embedding @search(by: ["hnsw(metric: euclidian, exponent: 4)"])
	   }`

	categories = `mutation {
		 addCategory(
		   input: [
			 { name: "Math & Science", name_v: [0.1, 0.2, 0.3, 0.4, 0.5] }
			 { name: "Health & Sports", name_v: [0.6, 0.7, 0.8, 0.9, 0.10] }
			 { name: "History & Civics", name_v: [0.11, 0.12, 0.13, 0.14, 0.15] }
			 { name: "Literacy & Language", name_v: [0.16, 0.17, 0.18, 0.19, 0.20] }
			 { name: "Music & The Arts", name_v: [0.21, 0.22, 0.23, 0.24, 0.25] }
		   ]
		 ) {
		   category {
			 id
			 name
			 name_v
		   }
		 }
	   }`
)

type ProjectInput struct {
	Title    string    `json:"title"`
	TitleV   []float32 `json:"title_v"`
	Category *Category `json:"category"`
}

type Category struct {
	Name  string    `json:"name"`
	NameV []float32 `json:"name_v"`
}

func addProject(t *testing.T, project ProjectInput, hc *dgraphtest.HTTPClient) {

	// add category if not exists already
	if project.Category == nil {
		// here we are querying the category by the vector of the project
		queryCategory := fmt.Sprintf(`query {
			 querySimilarCategoryByEmbedding(
			   by: name_v
			   topK: 1
			   vector: %v
			 ) {
			   name
			   name_v
			 }
		   }
		 `, project.TitleV)

		params := dgraphtest.GraphQLParams{
			Query: queryCategory,
		}
		response, err := hc.RunGraphqlQuery(params, false)
		require.NoError(t, err)

		type CategoryResponse struct {
			QuerySimilarCategoryByEmbedding []Category `json:"querySimilarCategoryByEmbedding"`
		}

		var resp CategoryResponse
		err = json.Unmarshal([]byte(string(response)), &resp)
		require.NoError(t, err)

		data := resp.QuerySimilarCategoryByEmbedding[0]
		project.Category = &Category{Name: data.Name, NameV: data.NameV}
	}

	variables := make(map[string]interface{})
	variables["project"] = project

	query := `
	 mutation addProject($project: AddProjectInput!) {
		 addProject(input: [$project]) {
			 project {
				   title
				   category{
					   name
				   }
			 }
		 }
	 }`

	params := dgraphtest.GraphQLParams{
		Query:     query,
		Variables: variables,
	}

	_, err := hc.RunGraphqlQuery(params, false)
	require.NoError(t, err)
}

func TestVectorExample(t *testing.T) {
	require.NoError(t, client.DropAll())

	hc, err := dc.HTTPClient()
	require.NoError(t, err)
	hc.LoginIntoNamespace("groot", "password", 0)

	// add schema
	require.NoError(t, hc.UpdateGQLSchema(exampleSchema))

	// add categories
	query := dgraphtest.GraphQLParams{
		Query: categories,
	}
	response, err := hc.RunGraphqlQuery(query, false)
	require.NoError(t, err)

	// adding project which vector similar to the category math & science
	project := ProjectInput{
		Title:  "iCreate with a Mini iPad",
		TitleV: []float32{0.1, 0.2, 0.3, 0.4, 0.5},
	}

	addProject(t, project, hc)

	// query similar project by embedding
	// it should return the project with title "iCreate with a Mini iPad" and category "Math & Science"
	queryProduct := `query {
		 querySimilarProjectByEmbedding(
		   by: title_v
		   topK: 1
		   vector: [0.1, 0.2, 0.3, 0.4, 0.5]
		 ) {
		   id
		   title
		   title_v
		   category{
			   name
			   name_v
		   }
		 }
	   }
	 `

	query = dgraphtest.GraphQLParams{
		Query: queryProduct,
	}
	response, err = hc.RunGraphqlQuery(query, false)
	require.NoError(t, err)

	type QueryResult struct {
		QuerySimilarProjectByEmbedding []ProjectInput `json:"querySimilarProjectByEmbedding"`
	}

	var resp QueryResult
	err = json.Unmarshal([]byte(string(response)), &resp)
	require.NoError(t, err)

	// fmt.Println("Project :", string(response))

	fmt.Println("Project Title:", resp.QuerySimilarProjectByEmbedding[0].Title)
	fmt.Println("Project Vector:", resp.QuerySimilarProjectByEmbedding[0].TitleV)
	fmt.Println("Project Category Name:", resp.QuerySimilarProjectByEmbedding[0].Category.Name)
	fmt.Println("Project Category Vector ", resp.QuerySimilarProjectByEmbedding[0].Category.NameV)
}
