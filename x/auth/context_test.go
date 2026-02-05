/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNilAuthContext(t *testing.T) {
	ctx := NilAuthContext()

	require.NotNil(t, ctx)
	require.Equal(t, "", ctx.UserID)
	require.Equal(t, uint64(0), ctx.Namespace)
	require.Nil(t, ctx.Groups)
	require.Equal(t, "", ctx.Level)
	require.Nil(t, ctx.Programs)
	require.True(t, ctx.IsNil)
}

func TestAuthContextWithValues(t *testing.T) {
	ctx := &AuthContext{
		UserID:    "alice",
		Namespace: 1,
		Groups:    []string{"analysts", "readers"},
		Level:     "secret",
		Programs:  []string{"alpha", "omega"},
		IsNil:     false,
	}

	require.Equal(t, "alice", ctx.UserID)
	require.Equal(t, uint64(1), ctx.Namespace)
	require.Equal(t, []string{"analysts", "readers"}, ctx.Groups)
	require.Equal(t, "secret", ctx.Level)
	require.Equal(t, []string{"alpha", "omega"}, ctx.Programs)
	require.False(t, ctx.IsNil)
}

func TestWithAuthContext(t *testing.T) {
	authCtx := &AuthContext{
		UserID: "bob",
		Level:  "top_secret",
	}

	ctx := WithAuthContext(context.Background(), authCtx)
	require.NotNil(t, ctx)

	extracted := FromContext(ctx)
	require.NotNil(t, extracted)
	require.Equal(t, "bob", extracted.UserID)
	require.Equal(t, "top_secret", extracted.Level)
}

func TestFromContextMissing(t *testing.T) {
	// When no auth context is set, should return nil
	extracted := FromContext(context.Background())
	require.Nil(t, extracted)
}

func TestExtractOrNil(t *testing.T) {
	// When auth context exists, return it
	authCtx := &AuthContext{
		UserID: "charlie",
		Level:  "classified",
	}
	ctx := WithAuthContext(context.Background(), authCtx)
	extracted := ExtractOrNil(ctx)
	require.Equal(t, "charlie", extracted.UserID)
	require.False(t, extracted.IsNil)

	// When no auth context, return NilAuthContext
	extracted = ExtractOrNil(context.Background())
	require.NotNil(t, extracted)
	require.True(t, extracted.IsNil)
	require.Equal(t, "", extracted.UserID)
}
