/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package auth

import "context"

// TokenExtractor defines the interface for extracting authentication context
// from various token types (JWT, API keys, etc.).
type TokenExtractor interface {
	// Extract extracts authentication context from the given context.
	// Returns an error if extraction fails (e.g., invalid token).
	Extract(ctx context.Context) (*AuthContext, error)

	// Name returns the name of this extractor (e.g., "jwt", "apikey").
	Name() string
}
