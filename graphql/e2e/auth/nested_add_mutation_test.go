//go:build integration

/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package auth

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/graphql/e2e/common"
)

func TestNestedAddHasInverseParentUpdateAuthBlocked(t *testing.T) {
	adminHeaders := common.GetJWT(t, "admin", "ADMIN", metaInfo)
	userHeaders := common.GetJWT(t, "regular", "USER", metaInfo)

	addFooParams := &common.GraphQLParams{
		Headers: adminHeaders,
		Query: `mutation {
			addProtectedFoo(input: [{ id: "nested-auth-foo1", items: [] }]) {
				numUids
			}
		}`,
	}
	gqlResponse := addFooParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)

	addItemParams := &common.GraphQLParams{
		Headers: userHeaders,
		Query: `mutation {
			addFooItem(input: [{ parent: { id: "nested-auth-foo1" } }]) {
				numUids
			}
		}`,
	}
	gqlResponse = addItemParams.ExecuteAsPost(t, common.GraphqlURL)
	require.NotEmpty(t, gqlResponse.Errors)
	require.Contains(t, gqlResponse.Errors[0].Message, "authorization failed")

	queryParams := &common.GraphQLParams{
		Headers: adminHeaders,
		Query: `query {
			queryProtectedFoo(filter: { id: { eq: "nested-auth-foo1" } }) {
				items { __typename }
			}
		}`,
	}
	gqlResponse = queryParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)
	require.JSONEq(t, `{"queryProtectedFoo":[{"items":[]}]}`, string(gqlResponse.Data))

	deleteParams := &common.GraphQLParams{
		Headers: adminHeaders,
		Query: `mutation {
			deleteProtectedFoo(filter: { id: { eq: "nested-auth-foo1" } }) {
				msg
			}
		}`,
	}
	gqlResponse = deleteParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)
}

func TestNestedAddDeepLeafAuthBlocked(t *testing.T) {
	userHeaders := common.GetJWT(t, "member", "USER", metaInfo)

	addParams := &common.GraphQLParams{
		Headers: userHeaders,
		Query: `mutation {
			addGuardedBase(input: [{
				id: "nested-auth-base1",
				files: [{}]
			}]) {
				numUids
			}
		}`,
	}
	gqlResponse := addParams.ExecuteAsPost(t, common.GraphqlURL)
	require.NotEmpty(t, gqlResponse.Errors)
	require.Contains(t, gqlResponse.Errors[0].Message, "authorization failed")
}

func TestNestedAddDeepLeafAuthAllowedForAdmin(t *testing.T) {
	adminHeaders := common.GetJWT(t, "admin", "ADMIN", metaInfo)

	addParams := &common.GraphQLParams{
		Headers: adminHeaders,
		Query: `mutation {
			addGuardedBase(input: [{
				id: "nested-auth-base2",
				files: [{}]
			}]) {
				guardedBase { id }
			}
		}`,
	}
	gqlResponse := addParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)

	deleteParams := &common.GraphQLParams{
		Headers: adminHeaders,
		Query: `mutation {
			deleteGuardedBase(filter: { id: { eq: "nested-auth-base2" } }) {
				msg
			}
		}`,
	}
	gqlResponse = deleteParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)
}

func TestNestedAddDeepLeafAuthAllowedForUserWithoutNestedFiles(t *testing.T) {
	userHeaders := common.GetJWT(t, "member", "USER", metaInfo)

	addParams := &common.GraphQLParams{
		Headers: userHeaders,
		Query: `mutation {
			addGuardedBase(input: [{ id: "nested-auth-base3", files: [] }]) {
				guardedBase { id }
			}
		}`,
	}
	gqlResponse := addParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)

	deleteParams := &common.GraphQLParams{
		Headers: userHeaders,
		Query: `mutation {
			deleteGuardedBase(filter: { id: { eq: "nested-auth-base3" } }) {
				msg
			}
		}`,
	}
	gqlResponse = deleteParams.ExecuteAsPost(t, common.GraphqlURL)
	common.RequireNoGQLErrors(t, gqlResponse)
}
