/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestApolloServiceQueryExcludesAuxiliaryTypes verifies that auxiliary types
// are not generated for @extends types when apolloServiceQuery=true
func TestApolloServiceQueryExcludesAuxiliaryTypes(t *testing.T) {
	schema := `
type Review {
	id: ID!
	rating: Int! @search
	comment: String @search(by: [fulltext])
}

extend type User @key(fields: "userId") {
	userId: ID! @external
	reviews: [Review]
}
`

	// Parse with apolloServiceQuery=true (simulating _service query)
	handler, err := NewHandler(schema, true)
	require.NoError(t, err)

	// Get the SDL that would be returned by _service
	sdl := handler.GQLSchemaWithoutApolloExtras()

	// These auxiliary types should NOT be present for extended User type
	auxiliaryTypes := []string{
		"input UserPatch",
		"input AddUserInput",
		"enum UserOrderable",
		"enum UserHasFilter",
		"input UserFilter",
		"input UserRef",
		"type AddUserPayload",
		"type UpdateUserPayload",
		"type DeleteUserPayload",
		"addUser(",
		"updateUser(",
		"deleteUser(",
		"getUser(",
		"queryUser(",
	}

	for _, auxType := range auxiliaryTypes {
		if strings.Contains(sdl, auxType) {
			t.Errorf("SDL should NOT contain '%s' for extended User type", auxType)
		}
	}

	// Review types SHOULD be present (not extended)
	reviewTypes := []string{
		"input ReviewPatch",
		"input AddReviewInput",
		"enum ReviewOrderable",
		"enum ReviewHasFilter",
	}

	for _, reviewType := range reviewTypes {
		if !strings.Contains(sdl, reviewType) {
			t.Errorf("SDL SHOULD contain '%s' for Review type", reviewType)
		}
	}
}

// TestApolloServiceQueryWithoutExtends verifies that auxiliary types ARE generated
// for regular types when apolloServiceQuery=true
func TestApolloServiceQueryWithoutExtends(t *testing.T) {
	schema := `
type User @key(fields: "userId") {
	userId: ID!
	username: String! @search(by: [hash])
	email: String
}
`

	// Parse with apolloServiceQuery=true
	handler, err := NewHandler(schema, true)
	require.NoError(t, err)

	sdl := handler.GQLSchemaWithoutApolloExtras()

	// Auxiliary types SHOULD be present for non-extended User type
	auxiliaryTypes := []string{
		"input UserPatch",
		"input AddUserInput",
		"enum UserOrderable",
		"enum UserHasFilter",
	}

	for _, auxType := range auxiliaryTypes {
		if !strings.Contains(sdl, auxType) {
			t.Errorf("SDL SHOULD contain '%s' for regular User type", auxType)
		}
	}
}
