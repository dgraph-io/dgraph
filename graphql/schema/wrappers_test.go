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
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2/ast"
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

func TestCheckNonNulls(t *testing.T) {

	gqlSchema, err := FromString(`
	type T { 
		req: String!
		notReq: String
		alsoReq: String!
	}`)
	require.NoError(t, err)

	tcases := map[string]struct {
		obj map[string]interface{}
		exc string
		err error
	}{
		"all present": {
			obj: map[string]interface{}{"req": "here", "notReq": "here", "alsoReq": "here"},
			err: nil,
		},
		"only non-null": {
			obj: map[string]interface{}{"req": "here", "alsoReq": "here"},
			err: nil,
		},
		"missing non-null": {
			obj: map[string]interface{}{"req": "here", "notReq": "here"},
			err: errors.Errorf("type T requires a value for field alsoReq, but no value present"),
		},
		"missing all non-null": {
			obj: map[string]interface{}{"notReq": "here"},
			err: errors.Errorf("type T requires a value for field req, but no value present"),
		},
		"with exclusion": {
			obj: map[string]interface{}{"req": "here", "notReq": "here"},
			exc: "alsoReq",
			err: nil,
		},
	}

	typ := &astType{
		typ:      &ast.Type{NamedType: "T"},
		inSchema: (gqlSchema.(*schema)).schema,
	}

	for name, test := range tcases {
		t.Run(name, func(t *testing.T) {
			err := typ.EnsureNonNulls(test.obj, test.exc)
			if test.err == nil {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, test.err.Error())
			}
		})
	}
}

func TestAuth(t *testing.T) {
	gqlSchema, err := FromString(`
input AuthRule {
	and: [AuthRule]
	or: [AuthRule]
	not: AuthRule
	rule: String
}

directive @auth(query: AuthRule, add: AuthRule, update: AuthRule, delete:AuthRule) on OBJECT
directive @id on FIELD_DEFINITION

type Todo @auth(
    query: {
        or: [
          { rule: "owner (filter: { username: { eq: $X-MyApp-User }})" },
          { rule: "sharedWith (filter: { username: { eq: $X-MyApp-User }})" },
          { rule: "isPublic {eq: true}" },
          { rule: "$X-MyApp-Role: {eq: ADMIN}" }
        ]
    },
    add: { rule: "owner (filter: { username: { eq: $X-MyApp-User }})" },
    update: { rule: "owner (filter: { username: { eq: $X-MyApp-User }})" },
    delete: { rule: "$X-MyApp-Role: {eq: ADMIN}" }
) {
    id: ID!
    title: String
    text: String
    isPublic: Boolean
    dateCompleted: String @auth(
        add: { rule: "DENY" },
        update: { rule: "filter: { dateCompleted: { eq: null } })" }
    )
    sharedWith: [User]
    owner: User
    somethingPrivate: String @auth(
        query: { rule: "owner (filter: { username: { eq: $X-MyApp-User } })" }
    )
}

type User @auth(
  add: { rule: "$MyApp.Role: { eq: ADD-BOT }" },
  update: { rule: "filter: { username: { eq: $X-MyApp-User } }" }
){
  username: String! @id
  todos: [Todo]
}
	`)
	require.NoError(t, err)

	ownerRule := &RuleAst{
		name: "owner",
		typ:  GqlTyp,
		value: &RuleAst{
			name: "filter",
			typ:  Op,
			value: &RuleAst{
				name: "username",
				typ:  GqlTyp,
				value: &RuleAst{
					name: "eq",
					typ:  Op,
					value: &RuleAst{
						name: "X-MyApp-User",
						typ:  JwtVar,
					},
				},
			},
		},
	}

	sharedWithRule := &RuleAst{
		name: "sharedWith",
		typ:  GqlTyp,
		value: &RuleAst{
			name: "filter",
			typ:  Op,
			value: &RuleAst{
				name: "username",
				typ:  GqlTyp,
				value: &RuleAst{
					name: "eq",
					typ:  Op,
					value: &RuleAst{
						name: "X-MyApp-User",
						typ:  JwtVar,
					},
				},
			},
		},
	}

	isPublicRule := &RuleAst{
		name: "isPublic",
		typ:  GqlTyp,
		value: &RuleAst{
			name: "eq",
			typ:  Op,
			value: &RuleAst{
				name: "true",
				typ:  Constant,
			},
		},
	}

	isAdmin := &RuleAst{
		name: "X-MyApp-Role",
		typ:  JwtVar,
		value: &RuleAst{
			name: "eq",
			typ:  Op,
			value: &RuleAst{
				name: "ADMIN",
				typ:  Constant,
			},
		},
	}

	isAddBot := &RuleAst{
		name: "MyApp.Role",
		typ:  JwtVar,
		value: &RuleAst{
			name: "eq",
			typ:  Op,
			value: &RuleAst{
				name: "ADD-BOT",
				typ:  Constant,
			},
		},
	}

	TodoAuth := &AuthContainer{
		Query: &RuleNode{
			Or: []*RuleNode{
				{Rule: ownerRule},
				{Rule: sharedWithRule},
				{Rule: isPublicRule},
				{Rule: isAdmin},
			},
		},
		Add:    &RuleNode{Rule: ownerRule},
		Update: &RuleNode{Rule: ownerRule},
		Delete: &RuleNode{Rule: isAdmin},
	}

	DateCompleted := &AuthContainer{
		Add: &RuleNode{
			Rule: &RuleAst{
				name: "DENY",
				typ:  Constant,
			},
		},
		Update: &RuleNode{
			Rule: &RuleAst{
				name: "filter",
				typ:  Op,
				value: &RuleAst{
					name: "dateCompleted",
					typ:  GqlTyp,
					value: &RuleAst{
						name: "eq",
						typ:  Op,
						value: &RuleAst{
							name: "null",
							typ:  Constant,
						},
					},
				},
			},
		},
	}

	require.Equal(t, gqlSchema.AuthTypeRules("Todo"), TodoAuth)

	UserAuth := &AuthContainer{
		Add:    &RuleNode{Rule: isAddBot},
		Update: &RuleNode{Rule: ownerRule.value},
	}

	require.Equal(t, gqlSchema.AuthTypeRules("User"), UserAuth)

	require.Equal(t, gqlSchema.AuthFieldRules("Todo", "somethingPrivate"),
		&AuthContainer{Query: &RuleNode{Rule: ownerRule}})

	require.Equal(t, gqlSchema.AuthFieldRules("Todo", "dateCompleted"),
		DateCompleted)
}
