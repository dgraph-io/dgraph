/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package auth

import "context"

// MockExtractor is a TokenExtractor for testing that returns a preconfigured AuthContext.
type MockExtractor struct {
	AuthCtx *AuthContext
	Err     error
}

// NewMockExtractor creates a new MockExtractor with the given AuthContext.
func NewMockExtractor(authCtx *AuthContext) *MockExtractor {
	return &MockExtractor{AuthCtx: authCtx}
}

// Extract returns the preconfigured AuthContext or error.
func (m *MockExtractor) Extract(ctx context.Context) (*AuthContext, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.AuthCtx, nil
}

// Name returns "mock".
func (m *MockExtractor) Name() string {
	return "mock"
}

// WithMockAuth returns a context with the given AuthContext attached.
// This is a convenience function for testing.
func WithMockAuth(ctx context.Context, userID string, namespace uint64, level string, programs []string) context.Context {
	return WithAuthContext(ctx, &AuthContext{
		UserID:    userID,
		Namespace: namespace,
		Level:     level,
		Programs:  programs,
		IsNil:     false,
	})
}
