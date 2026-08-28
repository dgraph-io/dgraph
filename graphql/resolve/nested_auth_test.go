/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package resolve

import (
	"context"
	"os"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/graphql/schema"
	"github.com/dgraph-io/dgraph/v25/graphql/test"
	"github.com/dgraph-io/dgraph/v25/testutil"
)

func loadNestedAuthSchema(t *testing.T) schema.Schema {
	t.Helper()
	sch, err := os.ReadFile("nested_auth_schema.graphql")
	require.NoError(t, err)
	authSchema, err := testutil.AppendAuthInfo(sch, jwt.SigningMethodHS256.Name, "", false)
	require.NoError(t, err)
	return test.LoadSchemaFromString(t, string(authSchema))
}

func TestAddChildRecordsAffectedProtectedParent(t *testing.T) {
	gqlSchema := loadNestedAuthSchema(t)
	authMeta := &testutil.AuthMeta{
		Namespace: "https://xyz.io/jwt/claims",
		Algo:      jwt.SigningMethodHS256.Name,
		AuthVars: map[string]interface{}{
			"ROLE": "USER",
		},
	}

	query := `
	mutation {
		addFooItem(input: [{ parent: { id: "foo1" } }]) {
			numUids
		}
	}`

	op, err := gqlSchema.Operation(&schema.Request{Query: query})
	require.NoError(t, err)
	mut := test.GetMutation(t, op)

	ctx, err := authMeta.AddClaimsToContext(context.Background())
	require.NoError(t, err)

	arw := NewAddRewriter().(*AddRewriter)
	idExistence := map[string]string{
		"ProtectedFoo_1": "0xabc",
	}
	_, _, _ = arw.RewriteQueries(ctx, mut)
	upserts, err := arw.Rewrite(ctx, mut, idExistence)
	require.NoError(t, err)
	require.Len(t, upserts, 1)
	require.Equal(t, "ProtectedFoo", upserts[0].AffectedNodes["0xabc"].Name())
}

func TestNestedAddRecordsDeepNewNodes(t *testing.T) {
	gqlSchema := loadNestedAuthSchema(t)
	authMeta := &testutil.AuthMeta{
		Namespace: "https://xyz.io/jwt/claims",
		Algo:      jwt.SigningMethodHS256.Name,
		AuthVars: map[string]interface{}{
			"ROLE": "USER",
		},
	}

	query := `
	mutation {
		addGuardedBase(input: [{
			id: "base1",
			files: [{}]
		}]) {
			numUids
		}
	}`

	op, err := gqlSchema.Operation(&schema.Request{Query: query})
	require.NoError(t, err)
	mut := test.GetMutation(t, op)

	ctx, err := authMeta.AddClaimsToContext(context.Background())
	require.NoError(t, err)

	arw := NewAddRewriter().(*AddRewriter)
	idExistence := map[string]string{}
	_, _, _ = arw.RewriteQueries(ctx, mut)
	upserts, err := arw.Rewrite(ctx, mut, idExistence)
	require.NoError(t, err)
	require.Len(t, upserts, 1)

	var hasBase, hasFile bool
	for _, typ := range upserts[0].NewNodes {
		switch typ.Name() {
		case "GuardedBase":
			hasBase = true
		case "GuardedFile":
			hasFile = true
		}
	}
	require.True(t, hasBase, "expected nested GuardedBase in newNodes")
	require.True(t, hasFile, "expected nested GuardedFile in newNodes")
}
