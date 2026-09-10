/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"context"
	"net/http"
	"sync/atomic"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

// Authenticator verifies the credential on a request and reports who is calling.
//
// It MUST NOT consult or produce tenancy — that is the TenantResolver's job — and
// it MUST NOT deny a request merely for lacking a credential. Returning
// (nil, nil) for "no credential presented" is the normal case for the endpoints
// that are unauthenticated by design.
type Authenticator interface {
	// Name identifies the implementation, for logs and audit.
	Name() string
	// Authenticate returns the verified caller, (nil, nil) when the request
	// carries no credential, or an error when it carries one that does not
	// verify.
	Authenticate(ctx context.Context) (*Principal, error)
}

// authenticator holds the installed implementation, or nil for the built-in one.
// An atomic pointer for the same reason as tenantResolver: one word, read per
// request, written once at startup.
var authenticator atomic.Pointer[Authenticator]

// SetAuthenticator installs a deployment-specific authenticator, replacing the
// built-in one that verifies Dgraph's own ACL access JWT. Call it during command
// setup, before any listener starts serving. Passing nil restores the built-in.
func SetAuthenticator(a Authenticator) {
	if a == nil {
		authenticator.Store(nil)
		return
	}
	authenticator.Store(&a)
}

// ACLAuthenticator returns the built-in authenticator, which verifies Dgraph's own
// ACL access token.
//
// Exported so a deployment installing its own Authenticator can compose with this
// one rather than displace it. That matters more than it looks: Login is how an
// ACL token is obtained, so an installed authenticator that cannot also verify an
// ACL token takes away the cluster's ability to log in — and the failure surfaces
// as an authorization error somewhere unrelated.
func ACLAuthenticator() Authenticator { return aclAuthenticator{} }

// currentAuthenticator returns the installed authenticator, or the built-in ACL
// one when none is installed. Mirrors how ResolveTenant falls back to
// aclTenantResolver, so "installed" and "default" behave the same way in both
// halves of the seam.
func currentAuthenticator() Authenticator {
	if a := authenticator.Load(); a != nil {
		return *a
	}
	return aclAuthenticator{}
}

// WithResolvedIdentity authenticates the request if it carries a credential and
// returns a context carrying the resulting Principal.
//
// It never rejects. A request with no credential, or one whose credential does
// not verify, proceeds with no Principal attached, and the authorization layer
// rejects it exactly as it does today — with the accurate error and the right
// gRPC code.
//
// That contract is deliberate, and it is what removes the need for an
// unauthenticated-endpoint allow-list. Login, health checks, and CheckVersion
// cannot present a credential: Login is how one is obtained in the first place.
// A rejecting interceptor would need to enumerate them, which is a second policy
// engine sitting in front of the one that already knows which operations require
// authentication. Worse, an incomplete list fails closed on exactly the endpoint
// that would let you notice — the cluster stops being able to log in or report
// health.
//
// Verification failures are logged rather than returned, so a stale or malformed
// credential is still visible to an operator without being fatal here.
func WithResolvedIdentity(ctx context.Context) context.Context {
	a := currentAuthenticator()
	p, err := a.Authenticate(ctx)
	switch {
	case err != nil:
		glog.V(2).Infof("identity: %s could not verify the presented credential: %v", a.Name(), err)
		return ctx
	case p == nil:
		// No credential presented. Normal for unauthenticated endpoints.
		return ctx
	}
	return WithPrincipal(ctx, p)
}

// IdentityUnaryInterceptor resolves the caller's identity onto the context for
// every unary RPC. See WithResolvedIdentity for why it never rejects.
//
// Install it ahead of the audit interceptor so audit can record the resolved
// principal instead of parsing the credential a second time.
func IdentityUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler) (any, error) {
		return handler(WithResolvedIdentity(ctx), req)
	}
}

// IdentityStreamInterceptor is the streaming counterpart of
// IdentityUnaryInterceptor.
func IdentityStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo,
		handler grpc.StreamHandler) error {
		return handler(srv, &identityServerStream{ServerStream: ss, ctx: WithResolvedIdentity(ss.Context())})
	}
}

// identityServerStream overrides Context so the handler sees the resolved
// identity. grpc.ServerStream has no setter for its context.
type identityServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *identityServerStream) Context() context.Context { return s.ctx }

// AttachRequestIdentity performs the standard HTTP-edge prelude: it moves the
// access token, remote IP, and auth token from the request into gRPC metadata,
// then resolves the caller's identity. It replaces the four-line sequence that
// was duplicated at every HTTP handler.
func AttachRequestIdentity(ctx context.Context, r *http.Request) context.Context {
	ctx = AttachAccessJwt(ctx, r)
	ctx = AttachRemoteIP(ctx, r)
	ctx = AttachAuthToken(ctx, r)
	return WithResolvedIdentity(ctx)
}

// aclAuthenticator is the built-in authenticator, and the only one an OSS build
// has. It verifies Dgraph's own ACL access JWT.
//
// It is edgraph.validateToken minus the namespace claim, which is deliberate:
// identity and tenancy are read by different components now, and the namespace
// belongs to the TenantResolver. A token with no namespace claim still yields a
// valid identity here; the resolver decides separately what that means for
// tenancy.
type aclAuthenticator struct{}

func (aclAuthenticator) Name() string { return "acl" }

func (aclAuthenticator) Authenticate(ctx context.Context) (*Principal, error) {
	if !WorkerConfig.AclEnabled {
		// No ACL configured: there is no credential to verify and no identity to
		// report. Authorization fails open in this configuration, as it does today.
		return nil, nil
	}

	jwtStr, err := ExtractJwt(ctx)
	if err != nil {
		// ErrNoJwt means no credential presented, which is not a failure.
		if errors.Is(err, ErrNoJwt) {
			return nil, nil
		}
		return nil, err
	}

	claims, err := ParseJWT(jwtStr)
	if err != nil {
		return nil, err
	}
	// ParseJWT already rejects an expired token; this additionally requires the
	// claim to be present, because MapClaims treats a missing exp as valid.
	// Mirrors edgraph.validateToken.
	if exp, expErr := claims.GetExpirationTime(); expErr != nil || exp == nil {
		return nil, errors.Errorf("Token is expired")
	}

	userID, ok := claims["userid"].(string)
	if !ok {
		return nil, errors.Errorf("userid in claims is not a string:%v", claims["userid"])
	}

	groups, err := groupsFromClaims(claims["groups"])
	if err != nil {
		return nil, err
	}

	return &Principal{
		Issuer:  "dgraph-acl",
		Subject: userID,
		Groups:  groups,
		Claims:  claims,
		Method:  MethodACL,
	}, nil
}

// groupsFromClaims converts the `groups` claim to a string slice. An absent claim
// is not an error — a user may belong to no groups — but a non-string member is,
// matching edgraph.validateToken.
func groupsFromClaims(claim any) ([]string, error) {
	raw, ok := claim.([]interface{})
	if !ok {
		return nil, nil
	}
	groups := make([]string, 0, len(raw))
	for _, g := range raw {
		s, ok := g.(string)
		if !ok {
			return nil, errors.Errorf("unable to convert group to string:%v", g)
		}
		groups = append(groups, s)
	}
	return groups, nil
}
