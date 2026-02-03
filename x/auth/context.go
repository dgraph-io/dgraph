/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package auth

import "context"

// authContextKey is the context key for storing AuthContext.
type authContextKey struct{}

// AuthContext contains authentication and authorization information extracted
// from credentials (JWT, API key, etc.). It is used for both ACL and label-based
// authorization decisions.
type AuthContext struct {
	UserID    string   // User identifier
	Namespace uint64   // Dgraph namespace
	Groups    []string // ACL groups the user belongs to
	Level     string   // Security classification level (e.g., "secret", "top_secret")
	Programs  []string // NEED-TO-KNOW compartments/programs
	IsNil     bool     // True when no credentials were provided (nil token pattern)
}

// NilAuthContext returns a default AuthContext for unauthenticated requests.
// This implements the "nil token" pattern to avoid if/else checks throughout the codebase.
func NilAuthContext() *AuthContext {
	return &AuthContext{
		UserID:    "",
		Namespace: 0,
		Groups:    nil,
		Level:     "",  // No level = can only access unlabeled predicates
		Programs:  nil, // No programs = can only access non-compartmented data
		IsNil:     true,
	}
}

// WithAuthContext returns a new context with the given AuthContext attached.
func WithAuthContext(ctx context.Context, authCtx *AuthContext) context.Context {
	return context.WithValue(ctx, authContextKey{}, authCtx)
}

// FromContext extracts the AuthContext from the context, returning nil if not present.
func FromContext(ctx context.Context) *AuthContext {
	if authCtx, ok := ctx.Value(authContextKey{}).(*AuthContext); ok {
		return authCtx
	}
	return nil
}

// ExtractOrNil extracts the AuthContext from the context, or returns NilAuthContext
// if no credentials were provided. This ensures callers always get a valid AuthContext.
// It checks in order: context value, transport metadata (gRPC/HTTP headers).
func ExtractOrNil(ctx context.Context) *AuthContext {
	// First check if AuthContext was explicitly set in context
	if authCtx := FromContext(ctx); authCtx != nil {
		return authCtx
	}
	// Then try to extract from transport metadata
	return ExtractFromMetadata(ctx)
}
