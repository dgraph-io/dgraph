/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package alpha

import (
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"path/filepath"
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
		// customIdentity means ConfigureIdentity installed its own authenticator.
		customIdentity bool
		want           []string // substrings that must appear, one per expected warning
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
		// Loopback is admitted regardless of the whitelist, so naming it explicitly
		// does not take the admin plane off the host. The public docs give exactly
		// this as the "allow localhost only" example.
		name:      "explicit loopback whitelist is quiet",
		posture:   x.AnonymousFull,
		whitelist: "127.0.0.1",
	}, {
		name:      "loopback range is quiet",
		posture:   x.AnonymousFull,
		whitelist: "127.0.0.1:127.0.0.3",
	}, {
		name:      "IPv6 loopback is quiet",
		posture:   x.AnonymousFull,
		whitelist: "::1",
	}, {
		name:      "loopback alongside a real address still warns",
		posture:   x.AnonymousFull,
		whitelist: "127.0.0.1,10.0.0.1",
		want:      []string{"WITHOUT ANY CREDENTIAL"},
	}, {
		// A range starting at loopback but running past it admits non-loopback
		// addresses, so a check on Lower alone would be wrong.
		name:      "range leaving loopback still warns",
		posture:   x.AnonymousFull,
		whitelist: "127.0.0.1:128.0.0.1",
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
	}, {
		// A deployment authenticator (an external JWT issuer, say) can identify
		// callers with neither ACL nor a token, so claiming the cluster cannot be
		// administered would be false.
		name:           "closed posture with a deployment authenticator is quiet",
		posture:        x.AnonymousData,
		customIdentity: true,
	}, {
		// The other warning does not depend on identity at all: under full, a
		// widened whitelist with no credential is exposed whoever installed what.
		name:           "deployment authenticator does not silence the exposure warning",
		posture:        x.AnonymousFull,
		whitelist:      "0.0.0.0/0",
		customIdentity: true,
		want:           []string{"WITHOUT ANY CREDENTIAL"},
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ips, err := getIPsFromString(tt.whitelist)
			require.NoError(t, err)

			got := securityWarnings(tt.posture, tt.whitelist, ips, tt.authToken, tt.acl,
				!tt.customIdentity, 8080)
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

// identityRequired names the handlers that reach an authorization decision and so
// must resolve the caller's identity. Checking for the banned helpers alone only
// catches a partial prelude. A handler that drops the prelude entirely and starts
// from a bare context.Background() calls none of them, and would pass.
var identityRequired = map[string]string{
	"queryHandler":           "/query, refused to anonymous callers under anonymous=none",
	"mutationHandler":        "/mutate, refused to anonymous callers under anonymous=none",
	"commitHandler":          "/commit, refused to anonymous callers under anonymous=none",
	"alterHandler":           "/alter, which can drop all data",
	"healthCheck":            "/health?all, a tenant-admin capability check",
	"stateHandler":           "/state, a tenant-admin capability check",
	"resolveWithAdminServer": "the HTTP admin routes, which re-enter the /admin GraphQL server",
	// graphql/subscription. Every subscription poll is a fresh query with no request
	// behind it, so this is where its identity comes from. It sat outside this test's
	// reach until a reviewer found it attaching only the access JWT.
	"subscriberContext": "every GraphQL subscription poll",
}

// identityScanDirs are the packages whose code builds a request context from
// client-supplied headers. The invariant is about that edge, not about a package,
// which is why this test reaches outside its own directory: the subscription
// poller is the same edge, and the same bug, in a different package.
var identityScanDirs = []string{
	".",
	"../../../graphql/subscription",
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

	// Every non-test file in the package, regardless of build tags. parser.ParseDir is
	// deprecated, and ignoring tags is what this test wants anyway: a handler compiled
	// only on one platform is still a handler.
	var paths []string
	for _, dir := range identityScanDirs {
		matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
		require.NoError(t, err)
		require.NotEmptyf(t, matches, "no Go files in %s; has the package moved?", dir)
		paths = append(paths, matches...)
	}

	// Aliased: the integration-tagged run_test.go declares a package-level `token`.
	fset := gotoken.NewFileSet()
	seenExceptions := map[string]bool{}
	seenRequired := map[string]bool{}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		require.NoError(t, err)

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			_, exempt := identityExceptions[fn.Name.Name]
			resolves := false
			ast.Inspect(fn, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkgIdent, ok := sel.X.(*ast.Ident)
				if !ok || pkgIdent.Name != "x" {
					return true
				}
				if sel.Sel.Name == "AttachRequestIdentity" {
					resolves = true
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

			if reason, required := identityRequired[fn.Name.Name]; required {
				seenRequired[fn.Name.Name] = true
				if !resolves {
					t.Errorf("%s: %s serves %s but never calls x.AttachRequestIdentity, so a "+
						"caller presenting a credential arrives unidentified.",
						fset.Position(fn.Pos()), fn.Name.Name, reason)
				}
			}
		}
	}

	// A renamed or deleted handler would otherwise drop out of the check silently.
	for name := range identityRequired {
		require.Truef(t, seenRequired[name],
			"identityRequired lists %q, but no function by that name exists in this package", name)
	}

	// A stale exception is its own problem: it reads as a documented carve-out for
	// something that no longer exists.
	for name := range identityExceptions {
		require.Truef(t, seenExceptions[name],
			"identityExceptions lists %q, but it no longer assembles the context by hand", name)
	}
}
