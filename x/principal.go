/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import "context"

// Principal is the verified answer to "who is calling". It says nothing about
// what tenant the request operates in, and nothing about what the caller may do.
// Those are the tenant resolver's and the authorizer's jobs respectively.
//
// Deliberately no namespace field. Tenancy already has exactly one home — the
// `namespace` gRPC metadata key, read by ExtractNamespace — and it lives there
// because it has to survive the hop to Zero's UID rate limiter and to the
// group-1 leader. Putting a second copy on the Principal would recreate the
// divergence this whole separation exists to remove: today the tenant is in both
// md["namespace"] and the ACL JWT's claim, and authorization reads the claim
// while storage reads the metadata.
type Principal struct {
	// Issuer identifies who vouched for this identity: "dgraph-acl" for a token
	// Dgraph minted itself, or the verified `iss` claim for an external issuer.
	Issuer string
	// Subject is the stable identity of the caller — the ACL userId, or `sub` for
	// an external issuer.
	Subject string
	// Groups are authorization-relevant memberships as asserted by the issuer.
	Groups []string
	// Claims carries the remaining verified claims, for policy layers that need
	// more than identity and groups.
	Claims map[string]any
	// Method names how the caller was authenticated: one of the Method* constants
	// below. Useful for audit, and load-bearing for any policy that must not treat
	// an external issuer's assertion as equivalent to one Dgraph made itself.
	Method string
}

// Authentication methods a Principal can carry.
//
// Comparing against these is not bookkeeping. Dgraph's ACL groups are the ones
// x.IsSuperAdmin consults, so a policy that reads Principal.Groups without
// checking Method would let any issuer that can mint a `guardians` group
// membership confer superadmin. Whoever asserted the identity decides what the
// assertion is worth.
const (
	// MethodACL is a token Dgraph minted and verified itself.
	MethodACL = "acl"
	// MethodExternalJWT is a token from a configured external issuer, verified
	// against its published keys.
	MethodExternalJWT = "external-jwt"
	// MethodPreshared is a shared secret, which identifies a client service rather
	// than an end user.
	MethodPreshared = "preshared"
	// MethodInternal is in-process Dgraph code acting on its own behalf, holding no
	// request credential.
	MethodInternal = "internal"
)

// principalKey is the context key under which a verified Principal travels.
//
// A context value, never metadata. Incoming metadata is entirely
// client-controlled, so a metadata-borne principal would be forgeable — and the
// internal worker port has no interceptor chain at all, so anything arriving
// there is unauthenticated by construction. A Principal also never needs to
// cross a process boundary: authorization is decided once, on the edge alpha,
// before any fan-out.
type principalKey struct{}

// WithPrincipal returns a context carrying p as the verified caller identity.
// Only an authenticator should call this: the point of the type is that its
// presence means a credential was actually verified.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom returns the verified caller identity, or nil when the request
// presented no credential or one that did not verify. A nil Principal is the
// normal state for unauthenticated endpoints — Login, health checks, and
// CheckVersion — so callers must treat it as "unknown", not as an error.
func PrincipalFrom(ctx context.Context) *Principal {
	p, _ := ctx.Value(principalKey{}).(*Principal)
	return p
}
