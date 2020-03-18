package common

import (
	"encoding/json"
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/require"
	"testing"
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

	gqlResponse := addStarshipParams.ExecuteAsPost(t, graphqlURL)
	requireNoGQLErrors(t, gqlResponse)

	addStarshipExpected := fmt.Sprintf(`{"addStarship":{
		"starship":[{
			"name":"Millennium Falcon",
			"length":2
		}]
	}}`)

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

	gqlResponse := queryStarshipParams.ExecuteAsPost(t, graphqlURL)
	requireNoGQLErrors(t, gqlResponse)

	queryStarshipExpected := fmt.Sprintf(`
	{
		"queryStarship":[{
			"id": "%s",
			"name":"Millennium Falcon",
			"length":2
		}]
	}`, newStarship.ID)

	var expected, result struct {
		QueryStarship []*starship
	}
	err := json.Unmarshal([]byte(queryStarshipExpected), &expected)
	require.NoError(t, err)
	err = json.Unmarshal(gqlResponse.Data, &result)
	require.NoError(t, err)

	require.Equal(t, expected, result)

	cleanupStarwars(t, result.QueryStarship[0].ID, "", "")
}

func fragmentInQueryOnInterface(t *testing.T) {
	newStarship := addStarship(t)
	humanID := addHuman(t, newStarship.ID)
	droidID := addDroid(t)

	queryCharacterParams := &GraphQLParams{
		Query: `query {
			queryCharacter(filter: null) {
				...fullCharacterFrag
			}
		}
		fragment fullCharacterFrag on Character {
			...commonCharacterFrag
			...humanFrag
			...droidFrag
		}
		fragment commonCharacterFrag on Character {
			id
			name
			appearsIn
		}
		fragment humanFrag on Human {
			starships {
				... on Starship {
					id
					name
					length
				}
			}
			totalCredits
			ename
		}
		fragment droidFrag on Droid {
			primaryFunction
		}
		`,
	}

	gqlResponse := queryCharacterParams.ExecuteAsPost(t, graphqlURL)
	requireNoGQLErrors(t, gqlResponse)

	queryCharacterExpected := fmt.Sprintf(`
	{
		"queryCharacter":[
			{
				"id": "%s",
				"name": "Han",
				"appearsIn": ["EMPIRE"],
				"starships": [{
					"id": "%s",
					"name": "Millennium Falcon",
					"length": 2
				}],
				"totalCredits": 10,
				"ename": "Han_employee"
			},
			{
				"id": "%s",
				"name": "R2-D2",
				"appearsIn": ["EMPIRE"],
				"primaryFunction": "Robot"
			}
		]
	}`, humanID, newStarship.ID, droidID)

	var expected, result struct {
		QueryCharacter []map[string]interface{}
	}
	err := json.Unmarshal([]byte(queryCharacterExpected), &expected)
	require.NoError(t, err)
	err = json.Unmarshal(gqlResponse.Data, &result)
	require.NoError(t, err)

	require.Equal(t, expected, result)

	cleanupStarwars(t, newStarship.ID, humanID, droidID)
}

func fragmentInQueryOnObject(t *testing.T) {
	newStarship := addStarship(t)
	humanID := addHuman(t, newStarship.ID)

	queryHumanParams := &GraphQLParams{
		Query: `query {
			queryHuman(filter: null) {
				ename
				...characterFrag
				...humanFrag
			}
		}
		fragment characterFrag on Character {
			id
			name
			appearsIn
		}
		fragment humanFrag on Human {
			starships {
				... {
					id
					name
					length
				}
			}
			totalCredits
		}
		`,
	}

	gqlResponse := queryHumanParams.ExecuteAsPost(t, graphqlURL)
	requireNoGQLErrors(t, gqlResponse)

	queryCharacterExpected := fmt.Sprintf(`
	{
		"queryHuman":[
			{
				"id": "%s",
				"name": "Han",
				"appearsIn": ["EMPIRE"],
				"starships": [{
					"id": "%s",
					"name": "Millennium Falcon",
					"length": 2
				}],
				"totalCredits": 10,
				"ename": "Han_employee"
			}
		]
	}`, humanID, newStarship.ID)

	var expected, result struct {
		QueryCharacter []map[string]interface{}
	}
	err := json.Unmarshal([]byte(queryCharacterExpected), &expected)
	require.NoError(t, err)
	err = json.Unmarshal(gqlResponse.Data, &result)
	require.NoError(t, err)

	require.Equal(t, expected, result)

	cleanupStarwars(t, newStarship.ID, humanID, "")
}
