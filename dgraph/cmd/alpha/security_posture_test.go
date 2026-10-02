/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package alpha

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/x"
)

func TestSecurityWarnings(t *testing.T) {
	tests := []struct {
		name      string
		posture   x.AnonymousPosture
		whitelist string
		authToken string
		acl       bool
		want      []string // substrings that must appear, one per expected warning
	}{{
		// The shipped default. An empty whitelist admits loopback only, so the admin
		// plane has not left the host and there is nothing to say.
		name:    "stock alpha is quiet",
		posture: x.AnonymousFull,
	}, {
		name:      "widened whitelist with no credential",
		posture:   x.AnonymousFull,
		whitelist: "0.0.0.0/0",
		want:      []string{"WITHOUT ANY CREDENTIAL"},
	}, {
		// A plausible-looking CIDR is still every pod in the cluster.
		name:      "private CIDR with no credential",
		posture:   x.AnonymousFull,
		whitelist: "10.0.0.0/8",
		want:      []string{"WITHOUT ANY CREDENTIAL"},
	}, {
		name:      "widened whitelist with a token",
		posture:   x.AnonymousFull,
		whitelist: "0.0.0.0/0",
		authToken: "s3cr3t",
	}, {
		name:      "widened whitelist with ACL",
		posture:   x.AnonymousFull,
		whitelist: "0.0.0.0/0",
		acl:       true,
	}, {
		// The closed posture answers the exposure, so the first warning must stand
		// down rather than tell an operator to fix something they already fixed.
		name:      "widened whitelist with a closed posture",
		posture:   x.AnonymousData,
		whitelist: "0.0.0.0/0",
		authToken: "s3cr3t",
	}, {
		// A closed posture with nothing that can produce an identity. Every
		// capability check denies, including the operator's own.
		name:    "closed posture with no credential at all",
		posture: x.AnonymousData,
		want:    []string{"cannot be administered"},
	}, {
		name:      "anonymous=none with no credential at all",
		posture:   x.AnonymousNone,
		whitelist: "0.0.0.0/0",
		want:      []string{"cannot be administered"},
	}, {
		name:      "closed posture with ACL is fine",
		posture:   x.AnonymousNone,
		whitelist: "0.0.0.0/0",
		acl:       true,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ips, err := getIPsFromString(tt.whitelist)
			require.NoError(t, err)

			got := securityWarnings(tt.posture, tt.whitelist, ips, tt.authToken, tt.acl, 8080)
			require.Len(t, got, len(tt.want))
			for i, want := range tt.want {
				require.Contains(t, got[i], "SECURITY:")
				require.Contains(t, got[i], want)
			}
		})
	}
}

// TestDefaultWhitelistAdmitsLoopbackOnly pins the model the v25 default rests on:
// a stock `dgraph alpha`, with no --security flag, serves admin operations to this
// host and nothing else. That is what makes anonymous=full a defensible default,
// and what makes widening the whitelist the moment the admin plane leaves the
// host.
func TestDefaultWhitelistAdmitsLoopbackOnly(t *testing.T) {
	prev := x.WorkerConfig.WhiteListedIPRanges
	t.Cleanup(func() { x.WorkerConfig.WhiteListedIPRanges = prev })

	ips, err := getIPsFromString("")
	require.NoError(t, err)
	x.WorkerConfig.WhiteListedIPRanges = ips

	for _, ip := range []string{"127.0.0.1", "::1"} {
		require.Truef(t, x.IsIpWhitelisted(ip), "loopback %s must reach admin operations", ip)
	}
	for _, ip := range []string{
		"203.0.113.7",  // arbitrary remote host
		"172.17.0.1",   // Docker bridge, i.e. the host reaching a published port
		"10.0.5.7",     // private network peer
		"192.168.1.20", // LAN peer
	} {
		require.Falsef(t, x.IsIpWhitelisted(ip),
			"non-loopback %s must not reach admin operations by default", ip)
	}
}

// identityExceptions names the handlers that deliberately assemble the request
// context by hand, and why. Everything else must go through
// x.AttachRequestIdentity.
var identityExceptions = map[string]string{
	// Login is how an access JWT is obtained, so it must not depend on one being
	// present, and it needs no Principal of its own: edgraph.Login authorizes on
	// hasAdminAuth, which reads the peer and the auth token straight out of the
	// metadata this prelude attaches.
	"loginHandler": "login must not require a credential it is the means of issuing",
}

// TestHTTPEdgeResolvesIdentityThroughOneHelper pins an invariant that a live test
// caught the hard way.
//
// x.AttachRequestIdentity is the whole HTTP-edge prelude: access JWT, remote IP,
// auth token, then identity resolution. A handler that reaches for the individual
// pieces instead gets some of them, and the one it is most likely to omit is the
// auth token -- which is invisible until a closed --security "anonymous=..."
// posture is configured, at which point a caller presenting the token arrives
// unidentified and is refused. /state and /health?all both had exactly that shape.
//
// So: no handler in this package resolves identity by hand unless it is listed
// above with a reason.
func TestHTTPEdgeResolvesIdentityThroughOneHelper(t *testing.T) {
	banned := map[string]string{
		"AttachAccessJwt": "attaches the access JWT but not the --security auth token",
		"AttachAuthToken": "attaches the auth token but resolves no Principal",
		"AttachRemoteIP":  "attaches the peer but resolves no Principal",
	}

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	require.NoError(t, err)

	seenExceptions := map[string]bool{}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}
				_, exempt := identityExceptions[fn.Name.Name]
				ast.Inspect(fn, func(n ast.Node) bool {
					sel, ok := n.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					pkgIdent, ok := sel.X.(*ast.Ident)
					if !ok || pkgIdent.Name != "x" {
						return true
					}
					why, bad := banned[sel.Sel.Name]
					if !bad {
						return true
					}
					if exempt {
						seenExceptions[fn.Name.Name] = true
						return true
					}
					t.Errorf("%s: %s calls x.%s, which %s. Use x.AttachRequestIdentity, or add "+
						"it to identityExceptions with a reason.",
						fset.Position(sel.Pos()), fn.Name.Name, sel.Sel.Name, why)
					return true
				})
			}
		}
	}

	// A stale exception is its own problem: it reads as a documented carve-out for
	// something that no longer exists.
	for name := range identityExceptions {
		require.Truef(t, seenExceptions[name],
			"identityExceptions lists %q, but it no longer assembles the context by hand", name)
	}
}
