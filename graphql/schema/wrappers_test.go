/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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

package schema

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
)

func TestDgraphMapping_WithoutDirectives(t *testing.T) {
	schemaStr := `
type Author {
        id: ID!

        name: String! @search(by: [hash, trigram])
        dob: DateTime @search
        reputation: Float @search
        posts: [Post!] @hasInverse(field: author)
}

type Post {
        postID: ID!
        postType: PostType @search
        author: Author! @hasInverse(field: posts)
}

enum PostType {
        Fact
        Question
        Opinion
}

interface Employee {
        ename: String!
}

interface Character {
        id: ID!
        name: String! @search(by: [exact])
        appearsIn: [Episode!] @search
}

type Human implements Character & Employee {
        starships: [Starship]
        totalCredits: Float
}

type Droid implements Character {
        primaryFunction: String
}

enum Episode {
        NEWHOPE
        EMPIRE
        JEDI
}

type Starship {
        id: ID!
        name: String! @search(by: [term])
        length: Float
}`

	schHandler, errs := NewHandler(schemaStr)
	require.NoError(t, errs)
	sch, err := FromString(schHandler.GQLSchema())
	require.NoError(t, err)

	s, ok := sch.(*schema)
	require.True(t, ok, "expected to be able to convert sch to internal schema type")

	author := map[string]string{
		"name":       "Author.name",
		"dob":        "Author.dob",
		"reputation": "Author.reputation",
		"posts":      "Author.posts",
	}
	post := map[string]string{
		"postType": "Post.postType",
		"author":   "Post.author",
	}
	character := map[string]string{
		"name":      "Character.name",
		"appearsIn": "Character.appearsIn",
	}
	human := map[string]string{
		"ename":        "Employee.ename",
		"name":         "Character.name",
		"appearsIn":    "Character.appearsIn",
		"starships":    "Human.starships",
		"totalCredits": "Human.totalCredits",
	}
	droid := map[string]string{
		"name":            "Character.name",
		"appearsIn":       "Character.appearsIn",
		"primaryFunction": "Droid.primaryFunction",
	}
	starship := map[string]string{
		"name":   "Starship.name",
		"length": "Starship.length",
	}

	expected := map[string]map[string]string{
		"Author":              author,
		"UpdateAuthorPayload": author,
		"DeleteAuthorPayload": author,
		"Post":                post,
		"UpdatePostPayload":   post,
		"DeletePostPayload":   post,
		"Employee": map[string]string{
			"ename": "Employee.ename",
		},
		"Character":              character,
		"UpdateCharacterPayload": character,
		"DeleteCharacterPayload": character,
		"Human":                  human,
		"UpdateHumanPayload":     human,
		"DeleteHumanPayload":     human,
		"Droid":                  droid,
		"UpdateDroidPayload":     droid,
		"DeleteDroidPayload":     droid,
		"Starship":               starship,
		"UpdateStarshipPayload":  starship,
		"DeleteStarshipPayload":  starship,
	}

	if diff := cmp.Diff(expected, s.dgraphPredicate); diff != "" {
		t.Errorf("dgraph predicate map mismatch (-want +got):\n%s", diff)
	}
}

func TestDgraphMapping_WithDirectives(t *testing.T) {
	schemaStr := `
	type Author @dgraph(type: "dgraph.author") {
			id: ID!

			name: String! @search(by: [hash, trigram])
			dob: DateTime @search
			reputation: Float @search
			posts: [Post!] @hasInverse(field: author)
	}

	type Post @dgraph(type: "dgraph.Post") {
			postID: ID!
			postType: PostType @search @dgraph(pred: "dgraph.post_type")
			author: Author! @hasInverse(field: posts) @dgraph(pred: "dgraph.post_author")
	}

	enum PostType {
			Fact
			Question
			Opinion
	}

	interface Employee @dgraph(type: "dgraph.employee.en") {
			ename: String!
	}

	interface Character @dgraph(type: "performance.character") {
			id: ID!
			name: String! @search(by: [exact])
			appearsIn: [Episode!] @search @dgraph(pred: "appears_in")
	}

	type Human implements Character & Employee {
			starships: [Starship]
			totalCredits: Float @dgraph(pred: "credits")
	}

	type Droid implements Character @dgraph(type: "roboDroid") {
			primaryFunction: String
	}

	enum Episode {
			NEWHOPE
			EMPIRE
			JEDI
	}

	type Starship @dgraph(type: "star.ship") {
			id: ID!
			name: String! @search(by: [term]) @dgraph(pred: "star.ship.name")
			length: Float
	}`

	schHandler, errs := NewHandler(schemaStr)
	require.NoError(t, errs)
	sch, err := FromString(schHandler.GQLSchema())
	require.NoError(t, err)

	s, ok := sch.(*schema)
	require.True(t, ok, "expected to be able to convert sch to internal schema type")

	author := map[string]string{
		"name":       "dgraph.author.name",
		"dob":        "dgraph.author.dob",
		"reputation": "dgraph.author.reputation",
		"posts":      "dgraph.author.posts",
	}
	post := map[string]string{
		"postType": "dgraph.post_type",
		"author":   "dgraph.post_author",
	}
	character := map[string]string{
		"name":      "performance.character.name",
		"appearsIn": "appears_in",
	}
	human := map[string]string{
		"ename":        "dgraph.employee.en.ename",
		"name":         "performance.character.name",
		"appearsIn":    "appears_in",
		"starships":    "Human.starships",
		"totalCredits": "credits",
	}
	droid := map[string]string{
		"name":            "performance.character.name",
		"appearsIn":       "appears_in",
		"primaryFunction": "roboDroid.primaryFunction",
	}
	starship := map[string]string{
		"name":   "star.ship.name",
		"length": "star.ship.length",
	}

	expected := map[string]map[string]string{
		"Author":              author,
		"UpdateAuthorPayload": author,
		"DeleteAuthorPayload": author,
		"Post":                post,
		"UpdatePostPayload":   post,
		"DeletePostPayload":   post,
		"Employee": map[string]string{
			"ename": "dgraph.employee.en.ename",
		},
		"Character":              character,
		"UpdateCharacterPayload": character,
		"DeleteCharacterPayload": character,
		"Human":                  human,
		"UpdateHumanPayload":     human,
		"DeleteHumanPayload":     human,
		"Droid":                  droid,
		"UpdateDroidPayload":     droid,
		"DeleteDroidPayload":     droid,
		"Starship":               starship,
		"UpdateStarshipPayload":  starship,
		"DeleteStarshipPayload":  starship,
	}

	if diff := cmp.Diff(expected, s.dgraphPredicate); diff != "" {
		t.Errorf("dgraph predicate map mismatch (-want +got):\n%s", diff)
	}
}
