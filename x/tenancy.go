/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"context"
	"net/http"
	"sync/atomic"

	"github.com/pkg/errors"
	"google.golang.org/grpc/metadata"
)

// TenantResolver attributes an incoming request to a tenant — a Dgraph
// namespace. It returns a context whose tenancy channel names the tenant the
// request acts in (set with AttachNamespace, read by ExtractNamespace), or an
// error if the request cannot be attributed.
//
// A resolver MUST NOT trust the incoming `namespace` metadata value: on the
// server side, incoming metadata is entirely client-controlled. It must either
// overwrite that value from a verified credential or return an error.
//
// The built-in resolver derives the namespace from Dgraph's own ACL access JWT,
// which is why tenancy currently requires ACL: with ACL disabled there is no
// credential to read a namespace from, so every request is the root namespace.
// A deployment that authenticates elsewhere can install its own resolver and
// decouple the two.
type TenantResolver func(ctx context.Context) (context.Context, error)

// tenantResolver holds the installed resolver, or nil for the built-in one.
//
// An atomic pointer rather than a mutex: this is one word read on every request,
// written once at startup. (The reserved-namespace registry above uses an
// RWMutex because it guards several maps and a slice, not a single pointer.)
var tenantResolver atomic.Pointer[TenantResolver]

// SetTenantResolver installs a deployment-specific tenant resolver, replacing
// the built-in one that derives the namespace from Dgraph's own ACL access JWT.
// Call it during command setup, after flags are parsed and before any listener
// starts serving. Passing nil restores the built-in resolver.
func SetTenantResolver(r TenantResolver) {
	if r == nil {
		tenantResolver.Store(nil)
		return
	}
	tenantResolver.Store(&r)
}

// TenantResolverInstalled reports whether a resolver other than the built-in one
// is installed. See MultiTenancyEnabled.
func TenantResolverInstalled() bool {
	return tenantResolver.Load() != nil
}

// MultiTenancyEnabled reports whether this cluster can serve more than one
// namespace — i.e. whether any request can name a namespace other than
// RootNamespace. True when ACL is on (the ACL access JWT carries the namespace)
// or when a deployment-specific tenant resolver is installed.
//
// Prefer this over reading WorkerConfig.AclEnabled directly wherever the
// question being asked is "is this cluster multi-tenant" rather than "is ACL
// configured". The two have been the same bit historically; they are not the
// same question.
func MultiTenancyEnabled() bool {
	return WorkerConfig.AclEnabled || TenantResolverInstalled()
}

// ResolveTenant attributes ctx to a tenant using the installed resolver.
func ResolveTenant(ctx context.Context) (context.Context, error) {
	if isTrustedTenantCtx(ctx) {
		// Already attributed by in-process Dgraph code that holds no request
		// credential to present. Leave it alone.
		return ctx, nil
	}
	if r := tenantResolver.Load(); r != nil {
		rctx, err := (*r)(ctx)
		if rctx == nil {
			// A resolver that fails closed will naturally return only an error, and
			// returning that nil onward is worse than the rejection it meant: a caller
			// that logs the error and proceeds, or that drops it, panics on first use
			// of the context instead. Hand back the original so the failure is always
			// an error rather than sometimes a crash.
			return ctx, err
		}
		return rctx, err
	}
	return aclTenantResolver(ctx)
}

// ClearIncomingNamespace removes any namespace the client put on the request.
//
// Call it at a gRPC entry point, before ResolveTenant. On the server side
// md["namespace"] is entirely client-controlled, and the built-in resolver leaves
// whatever is there when it cannot derive a namespace from the access JWT — so
// without this, a caller who omits or corrupts their credential still gets the
// tenant they asked for. Clearing it first turns that case into ExtractNamespace's
// "No namespace in the metadata" error, which is a rejection.
//
// Only for entry points. A continuation of an already-resolved request must keep
// the namespace it was attributed to.
func ClearIncomingNamespace(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}
	md = md.Copy()
	md.Delete("namespace")
	return metadata.NewIncomingContext(ctx, md)
}

// aclTenantResolver is the built-in resolver, and the only one an OSS build has.
// It is the pre-seam body of AttachJWTNamespace, unchanged: same branches, same
// returns, and the error is always nil.
func aclTenantResolver(ctx context.Context) (context.Context, error) {
	if !WorkerConfig.AclEnabled {
		// Single-tenant cluster: everything is the root namespace.
		return AttachNamespace(ctx, RootNamespace), nil
	}

	ns, err := ExtractNamespaceFrom(ctx)
	if err != nil {
		// Tolerate the failure and leave whatever namespace the context already
		// carries. Under ACL the request is rejected downstream by
		// authorizeRequest, which needs the same JWT this just failed to parse,
		// so nothing reaches storage on an unattributed context.
		return ctx, nil
	}
	return AttachNamespace(ctx, ns), nil
}

// ResolveTenantHTTP attributes an incoming HTTP request to a tenant, so the HTTP
// surface derives a namespace through the same resolver as the gRPC surface
// rather than reading the ACL JWT claim directly. It replaces the former
// ExtractNamespaceHTTP.
//
// It deliberately keeps that function's fail-open behavior: a request whose
// tenant cannot be determined resolves to the root namespace. The error return
// carries only a resolver's own failure, which the built-in resolver never
// produces.
//
// Failing open looks wrong until you enumerate the callers, which have
// materially different requirements — a blanket rejection breaks most of them:
//
//   - /admin sets resolver=0 unconditionally and uses this value only for
//     LazyLoadSchema, so the root namespace is the correct answer there. It also
//     serves the login mutation and the test harness's health check.
//   - /probe/graphql is unauthenticated by construction.
//   - audit must never reject; it only records the event.
//   - only /graphql routes by the resolved namespace, and it uses
//     ResolveTenantHTTPStrict instead.
//
// Rejecting a credential-less request on the first two deadlocks the cluster:
// login stops working, so nothing can ever obtain the token that would have
// satisfied the check.
func ResolveTenantHTTP(r *http.Request) (uint64, error) {
	ctx, err := ResolveTenant(AttachAccessJwt(context.Background(), r))
	if err != nil {
		return 0, err
	}
	ns, nsErr := ExtractNamespace(ctx)
	if nsErr != nil {
		return RootNamespace, nil
	}
	return ns, nil
}

// ResolveTenantHTTPStrict is ResolveTenantHTTP for the one caller that routes by
// the resolved namespace: the /graphql handler, which uses it to choose whose
// GraphQL schema serves the request.
//
// It differs on exactly one input. A request presenting an access token that
// cannot be resolved is rejected, rather than quietly served the root namespace's
// schema — leaking the shape of the root namespace's public API to a caller whose
// own tenant could not be determined.
//
// A request presenting NO token still resolves to the root namespace. That
// distinction is the whole design: /admin serves the login mutation and the
// harness health check, and health probes are unauthenticated by construction, so
// rejecting a credential-less request deadlocks the cluster — login stops working,
// which is how a token would have been obtained. Only /graphql needs the stricter
// rule, and only /graphql gets it.
func ResolveTenantHTTPStrict(r *http.Request) (uint64, error) {
	ctx := AttachAccessJwt(context.Background(), r)
	resolved, err := ResolveTenant(ctx)
	if err != nil {
		return 0, err
	}
	if ns, nsErr := ExtractNamespace(resolved); nsErr == nil {
		return ns, nil
	}

	// Nothing resolved, so distinguish "no credential" from "unusable
	// credential". Asking ExtractJwt rather than re-reading the header name keeps
	// this on the same accessor AttachAccessJwt feeds — two copies of that name
	// could drift apart, and the failure would be silent: the strict rule would
	// simply never fire.
	if _, jwtErr := ExtractJwt(ctx); jwtErr == nil {
		return 0, errors.Errorf("unable to determine the namespace from the supplied access token")
	}
	return RootNamespace, nil
}

// trustedTenantKey marks a context whose tenancy was set by trusted in-process
// code rather than derived from a request credential.
type trustedTenantKey struct{}

// AttachTrustedTenant attributes ctx to ns on behalf of in-process Dgraph code
// that holds no request credential — schema bootstrap, ACL upserts, GetGQLSchema
// — and marks it so ResolveTenant leaves the attribution alone.
//
// The marker is a context value, never metadata, so it cannot arrive over the
// wire: a network client has no way to claim it. That is what makes it safe for
// a resolver to otherwise distrust the incoming namespace entirely.
func AttachTrustedTenant(ctx context.Context, ns uint64) context.Context {
	return context.WithValue(AttachNamespace(ctx, ns), trustedTenantKey{}, true)
}

// isTrustedTenantCtx reports whether ctx was attributed by AttachTrustedTenant.
func isTrustedTenantCtx(ctx context.Context) bool {
	trusted, _ := ctx.Value(trustedTenantKey{}).(bool)
	return trusted
}
