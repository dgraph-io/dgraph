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

package common

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/testutil"
)

func fragmentInMutation(t *testing.T) {
	addStarshipParams := &GraphQLParams{
		Query: `mutation addStarship($starship: AddStarshipInput!) {
			addStarship(input: [$starship]) {
				starship {
					...starshipFrag
			  	}
			}
		}
		fragment starshipFrag on Starship {
			id
			name
			length
		}
		`,
		Variables: map[string]interface{}{"starship": map[string]interface{}{
			"name":   "Millennium Falcon",
			"length": 2,
		}},
	}

	gqlResponse := addStarshipParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

	addStarshipExpected := `
	{"addStarship":{
		"starship":[{
			"name":"Millennium Falcon",
			"length":2
		}]
	}}`

	var expected, result struct {
		AddStarship struct {
			Starship []*starship
		}
	}
	err := json.Unmarshal([]byte(addStarshipExpected), &expected)
	require.NoError(t, err)
	err = json.Unmarshal(gqlResponse.Data, &result)
	require.NoError(t, err)

	requireUID(t, result.AddStarship.Starship[0].ID)

	opt := cmpopts.IgnoreFields(starship{}, "ID")
	if diff := cmp.Diff(expected, result, opt); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}

	cleanupStarwars(t, result.AddStarship.Starship[0].ID, "", "")
}

func fragmentInQuery(t *testing.T) {
	newStarship := addStarship(t)

	queryStarshipParams := &GraphQLParams{
		Query: `query queryStarship($id: ID!) {
			queryStarship(filter: {
					id: [$id]
			}) {
				...starshipFrag
			}
		}
		fragment starshipFrag on Starship {
			id
			name
			length
		}
		`,
		Variables: map[string]interface{}{
			"id": newStarship.ID,
		},
	}

	gqlResponse := queryStarshipParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

	queryStarshipExpected := fmt.Sprintf(`
	{
		"queryStarship":[{
			"id":"%s",
			"name":"Millennium Falcon",
			"length":2.000000
		}]
	}`, newStarship.ID)

	JSONEqGraphQL(t, queryStarshipExpected, string(gqlResponse.Data))

	cleanupStarwars(t, newStarship.ID, "", "")
}

func fragmentInQueryOnInterface(t *testing.T) {
	newStarship := addStarship(t)
	humanID := addHuman(t, newStarship.ID)
	droidID := addDroid(t)
	thingOneId := addThingOne(t)
	thingTwoId := addThingTwo(t)

	queryCharacterParams := &GraphQLParams{
		Query: `query {
			queryCharacter {
				__typename
				...fullCharacterFrag
			}
			qc: queryCharacter {
				__typename
				... on Character {
					... on Character {
						... on Human {
							... on Human {
								id
								name
							}
						}
					}
				}
				... droidAppearsIn
			}
			qc1: queryCharacter {
				... on Human {
					__typename
					id
				}
				... on Droid {
					id
				}
			}
			qc2: queryCharacter {
				... on Human {
					name
					n: name
				}
			}
			qc3: queryCharacter {
				... on Droid{
					__typename
					primaryFunction
				}
				... on Employee {
					__typename
					ename
				}
				... on Human {
					__typename
					name
				}
			}
			qcRep1: queryCharacter {
				name
				... on Human {
					name
					totalCredits
				}
				... on Droid {
					name
					primaryFunction
				}
			}
			qcRep2: queryCharacter {
				... on Human {
					totalCredits
				}
				name
				... on Droid {
					primaryFunction
					name
				}
			}
			qcRep3: queryCharacter {
				...characterName1
				...characterName2
			}
			queryThing {
				__typename
				... on ThingOne {
					id
					name
					color
					usedBy
				}
				... on ThingTwo {
					id
					name
					color
					owner
				}
			}
			qt: queryThing {
				... on ThingOne {
					__typename
					id
				}
				... on ThingTwo {
					__typename
				}
			}
		}
		fragment fullCharacterFrag on Character {
			__typename
			...commonCharacterFrag
			...humanFrag
			...droidFrag
		}
		fragment commonCharacterFrag on Character {
			__typename
			id
			name
			appearsIn
		}
		fragment humanFrag on Human {
			__typename
			starships {
				... on Starship {
					__typename
					id
					name
					length
				}
			}
			totalCredits
			ename
		}
		fragment droidFrag on Droid {
			__typename
			primaryFunction
		}
		fragment droidAppearsIn on Droid {
			appearsIn
		}
		fragment characterName1 on Character {
			name
		}
		fragment characterName2 on Character {
			name
		}
		`,
	}

	gqlResponse := queryCharacterParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

	queryCharacterExpected := fmt.Sprintf(`
	{
		"queryCharacter":[
			{
				"__typename":"Human",
				"id":"%s",
				"name":"Han",
				"appearsIn":["EMPIRE"],
				"starships":[{
					"__typename":"Starship",
					"id":"%s",
					"name":"Millennium Falcon",
					"length":2.000000
				}],
				"totalCredits":10.000000,
				"ename":"Han_employee"
			},
			{
				"__typename":"Droid",
				"id":"%s",
				"name":"R2-D2",
				"appearsIn":["EMPIRE"],
				"primaryFunction":"Robot"
			}
		],
		"qc":[
			{
				"__typename":"Human",
				"id":"%s",
				"name":"Han"
			},
			{
				"__typename":"Droid",
				"appearsIn":["EMPIRE"]
			}
		],
		"qc1":[
			{
				"__typename":"Human",
				"id":"%s"
			},
			{
				"id":"%s"
			}
		],
		"qc2":[
			{
				"name":"Han",
				"n":"Han"
			},
			{
			}
		],
		"qc3":[
			{
				"__typename":"Human",
				"ename":"Han_employee",
				"name":"Han"
			},
			{
				"__typename":"Droid",
				"primaryFunction":"Robot"
			}
		],
		"qcRep1":[
			{
				"name":"Han",
				"totalCredits":10.000000
			},
			{
				"name":"R2-D2",
				"primaryFunction":"Robot"
			}
		],
		"qcRep2":[
			{
				"totalCredits":10.000000,
				"name":"Han"
			},
			{
				"name":"R2-D2",
				"primaryFunction":"Robot"
			}
		],
		"qcRep3":[
			{
				"name":"Han"
			},
			{
				"name":"R2-D2"
			}
		],
		"queryThing":[
			{
				"__typename":"ThingOne",
				"id":"%s",
				"name":"Thing-1",
				"color":"White",
				"usedBy":"me"
			},
			{
				"__typename":"ThingTwo",
				"id":"%s",
				"name":"Thing-2",
				"color":"Black",
				"owner":"someone"
			}
		],
		"qt":[
			{
				"__typename":"ThingOne",
				"id":"%s"
			},
			{
				"__typename":"ThingTwo"
			}
		]
	}`, humanID, newStarship.ID, droidID, humanID, humanID, droidID, thingOneId, thingTwoId,
		thingOneId)

	JSONEqGraphQL(t, queryCharacterExpected, string(gqlResponse.Data))

	cleanupStarwars(t, newStarship.ID, humanID, droidID)
	deleteThingOne(t, thingOneId)
	deleteThingTwo(t, thingTwoId)
}

func fragmentInQueryOnUnion(t *testing.T) {
	newStarship := addStarship(t)
	humanID := addHuman(t, newStarship.ID)
	homeId, dogId, parrotId, plantId := addHome(t, humanID)

	queryHomeParams := &GraphQLParams{
		Query: `query {
			queryHome {
				members {
					__typename
					... on Animal {
						category
					}
					... on Dog {
						id
						breed
					}
					... on Parrot {
						repeatsWords
					}
					... on Employee {
						ename
					}
					... on Character {
						id
					}
					... on Human {
						name
					}
					... on Plant {
						id
					}
				}
			}
			qh: queryHome {
				members {
					... on Animal {
						__typename
					}
					... on Dog {
						breed
					}
					... on Human {
						name
					}
					... on Plant {
						breed
					}
				}
			}
		}
		`,
	}

	gqlResponse := queryHomeParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

	queryHomeExpected := fmt.Sprintf(`
	{
	  "queryHome": [
		{
		  "members": [
			{
			  "__typename": "Human",
			  "ename": "Han_employee",
			  "id": "%s",
			  "name": "Han"
			},
			{
			  "__typename": "Dog",
			  "category": "Mammal",
			  "id": "%s",
			  "breed": "German Shephard"
			},
			{
			  "__typename": "Parrot",
			  "category": "Bird",
			  "repeatsWords": [
				"Good Morning!",
				"squawk"
			  ]
			},
			{
			  "__typename": "Plant",
			  "id": "%s"
			}
		  ]
		}
	  ],
	  "qh": [
		{
		  "members": [
			{
			  "name": "Han"
			},
			{
			  "__typename": "Dog",
			  "breed": "German Shephard"
			},
			{
			  "__typename": "Parrot"
			},
			{
			  "breed": "Flower"
			}
		  ]
		}
	  ]
	}`, humanID, dogId, plantId)
	testutil.CompareJSON(t, queryHomeExpected, string(gqlResponse.Data))

	cleanupStarwars(t, newStarship.ID, humanID, "")
	deleteHome(t, homeId, dogId, parrotId, plantId)
}

func fragmentInQueryOnObject(t *testing.T) {
	newStarship := addStarship(t)
	humanID := addHuman(t, newStarship.ID)

	queryHumanParams := &GraphQLParams{
		Query: `query {
			queryHuman(filter: null) {
				...characterFrag
				...humanFrag
				ename
			}
		}
		fragment characterFrag on Character {
			__typename
			id
			name
			appearsIn
		}
		fragment humanFrag on Human {
			starships {
				... {
					__typename
					id
					name
					length
				}
			}
			totalCredits
		}
		`,
	}

	gqlResponse := queryHumanParams.ExecuteAsPost(t, GraphqlURL)
	RequireNoGQLErrors(t, gqlResponse)

	queryCharacterExpected := fmt.Sprintf(`
	{
		"queryHuman":[
			{
				"__typename":"Human",
				"id":"%s",
				"name":"Han",
				"appearsIn":["EMPIRE"],
				"starships":[{
					"__typename":"Starship",
					"id":"%s",
					"name":"Millennium Falcon",
					"length":2.000000
				}],
				"totalCredits":10.000000,
				"ename":"Han_employee"
			}
		]
	}`, humanID, newStarship.ID)

	JSONEqGraphQL(t, queryCharacterExpected, string(gqlResponse.Data))

	query2HumanParams := &GraphQLParams{
		Query: `query  {
    		queryHuman() {
        		id
        		starships {
            		id
        		}
        		...HumanFrag
    		}
		}

		fragment HumanFrag on Human {
    		starships {
				... {
					__typename
					id
					name
					length
					}
			}
   	 	}`,
	}
	gqlResponse2 := query2HumanParams.ExecuteAsPost(t, GraphqlURL)

	RequireNoGQLErrors(t, gqlResponse2)
	queryCharacterExpected2 := fmt.Sprintf(`
	
		{"queryHuman":[
			{"id":"%s",
			"starships":[
				{"id":"%s",
				"__typename":"Starship",
				"name":"Millennium Falcon",
				"length":2.000000}]
				}
			]
		}
	`, humanID, newStarship.ID)
	JSONEqGraphQL(t, queryCharacterExpected2, string(gqlResponse2.Data))

	cleanupStarwars(t, newStarship.ID, humanID, "")
}
