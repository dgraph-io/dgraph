/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package auth

import (
	"context"
	"strings"

	"github.com/golang/glog"
	"google.golang.org/grpc/metadata"
)

// Header keys for auth context passed via transport (gRPC metadata or HTTP headers).
// These use lowercase to be compatible with both gRPC (which lowercases) and HTTP.
const (
	// HeaderKeyActive indicates auth context is present (even if level/programs are empty)
	HeaderKeyActive = "x-dgraph-auth-active"
	// HeaderKeyLevel is the header key for security level
	HeaderKeyLevel = "x-dgraph-auth-level"
	// HeaderKeyPrograms is the header key for programs (comma-separated)
	HeaderKeyPrograms = "x-dgraph-auth-programs"
)

// ExtractFromMetadata extracts AuthContext from gRPC incoming metadata.
// Returns NilAuthContext if metadata is missing or auth-active marker not present.
func ExtractFromMetadata(ctx context.Context) *AuthContext {
	md, ok := metadata.FromIncomingContext(ctx)
	glog.V(2).Infof("ExtractFromMetadata: hasMetadata=%v", ok)
	if !ok {
		return NilAuthContext()
	}
	return extractFromHeaders(headerGetter(md.Get))
}

// ExtractFromHeaders extracts AuthContext from a generic header getter function.
// This can be used with HTTP requests, gRPC metadata, or any other transport.
// Returns NilAuthContext if headers are missing or auth-active marker not present.
func ExtractFromHeaders(getHeader func(key string) string) *AuthContext {
	return extractFromHeaders(func(key string) []string {
		if v := getHeader(key); v != "" {
			return []string{v}
		}
		return nil
	})
}

// headerGetter is a function type for getting header values
type headerGetter func(key string) []string

// extractFromHeaders is the internal implementation that works with a header getter
func extractFromHeaders(get headerGetter) *AuthContext {
	// Check if auth is active - if not present, return nil token
	if active := get(HeaderKeyActive); len(active) == 0 || active[0] != "true" {
		glog.V(2).Infof("extractFromHeaders: auth not active, returning nil token")
		return NilAuthContext()
	}

	authCtx := &AuthContext{}

	// Extract level
	if levels := get(HeaderKeyLevel); len(levels) > 0 {
		authCtx.Level = levels[0]
		glog.V(2).Infof("extractFromHeaders: found level=%q", authCtx.Level)
	}

	// Extract programs (comma-separated)
	if programs := get(HeaderKeyPrograms); len(programs) > 0 && programs[0] != "" {
		authCtx.Programs = splitPrograms(programs[0])
		glog.V(2).Infof("extractFromHeaders: found programs=%v", authCtx.Programs)
	}

	glog.V(2).Infof("extractFromHeaders: auth active, level=%q, programs=%v", authCtx.Level, authCtx.Programs)
	return authCtx
}

// AttachToOutgoingContext attaches auth context to outgoing gRPC metadata.
// Used by clients to send auth info to the server via gRPC.
func AttachToOutgoingContext(ctx context.Context, authCtx *AuthContext) context.Context {
	if authCtx == nil {
		return ctx
	}

	// Always include the active marker to indicate auth is present
	pairs := []string{HeaderKeyActive, "true"}
	if authCtx.Level != "" {
		pairs = append(pairs, HeaderKeyLevel, authCtx.Level)
	}
	if len(authCtx.Programs) > 0 {
		pairs = append(pairs, HeaderKeyPrograms, joinPrograms(authCtx.Programs))
	}

	return metadata.AppendToOutgoingContext(ctx, pairs...)
}

// ToHeaders converts an AuthContext to a map of headers.
// Useful for HTTP clients or other transports.
func (a *AuthContext) ToHeaders() map[string]string {
	if a == nil {
		return nil
	}
	headers := map[string]string{
		HeaderKeyActive: "true",
	}
	if a.Level != "" {
		headers[HeaderKeyLevel] = a.Level
	}
	if len(a.Programs) > 0 {
		headers[HeaderKeyPrograms] = joinPrograms(a.Programs)
	}
	return headers
}

// splitPrograms splits a comma-separated string into a slice
func splitPrograms(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// joinPrograms joins a slice into a comma-separated string
func joinPrograms(programs []string) string {
	return strings.Join(programs, ",")
}
