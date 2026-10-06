/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package edgraph

import (
	"context"

	"github.com/dgraph-io/dgraph/v25/worker"
	"github.com/dgraph-io/dgraph/v25/x"
)

// PresharedSubject is the Principal.Subject a caller authenticated by the
// --security auth token carries.
//
// The value matches audit.PoorManAuth deliberately. Audit prefers the resolved
// Principal over re-parsing the credential, so minting a Principal for a
// token-bearing caller would otherwise change what every such request logs. Pinned
// by TestPresharedSubjectMatchesAudit.
const PresharedSubject = "PoorManAuth"

// PresharedIssuer identifies the --security auth token as the thing that vouched
// for a Principal.
const PresharedIssuer = "dgraph-security-token"

// PresharedAuthenticator returns the built-in authenticator: Dgraph's own ACL
// access JWT first, then the --security auth token.
//
// The token half is what makes a closed --security "anonymous=..." posture usable
// without standing up full ACL. Before this, the token was a boolean check --
// hasPoormansAuth answered "may this request proceed" and produced no identity --
// so a token-bearing caller was indistinguishable from an anonymous one, and a
// posture keyed on "did this caller present a credential" would have denied it.
//
// Ordering is ACL first because ACL identifies a user while the token identifies a
// client service, and the more specific identity should win when a request carries
// both. It also costs nothing when ACL is off: aclAuthenticator returns
// (nil, nil) immediately in that configuration.
func PresharedAuthenticator() x.Authenticator { return presharedAuthenticator{} }

type presharedAuthenticator struct{}

func (presharedAuthenticator) Name() string { return "acl+preshared" }

func (presharedAuthenticator) Authenticate(ctx context.Context) (*x.Principal, error) {
	p, aclErr := x.ACLAuthenticator().Authenticate(ctx)
	if p != nil {
		return p, nil
	}
	// No ACL identity, either because ACL is off, the request carried no access
	// JWT, or the one it carried did not verify. The token is the remaining route.
	// On a verification failure we still try it, so a client service with a valid
	// token and a stale JWT is identified as the service rather than as nobody.
	if tp := presharedPrincipal(ctx); tp != nil {
		return tp, nil
	}
	return nil, aclErr
}

// presharedPrincipal returns a Principal for a caller presenting the configured
// --security auth token, or nil.
//
// It deliberately does not reuse hasPoormansAuth's result alone. That function
// returns nil when no token is configured, which is the right answer for "may this
// request proceed" and the wrong one here: an unconfigured token means there is no
// credential to verify and therefore no identity to assert. Minting a Principal in
// that case would make every anonymous caller look authenticated, which is exactly
// the fail-open this work exists to close.
//
// Groups is deliberately empty. x.IsSuperAdmin consults Principal.Groups by name,
// so putting "guardians" here would turn a shared secret into cluster authority
// without going through authorizeClusterAdmin and its ACL-on exclusion.
func presharedPrincipal(ctx context.Context) *x.Principal {
	if worker.Config.AuthToken == "" {
		return nil
	}
	if err := hasPoormansAuth(ctx); err != nil {
		return nil
	}
	return &x.Principal{
		Issuer:  PresharedIssuer,
		Subject: PresharedSubject,
		Method:  x.MethodPreshared,
	}
}
