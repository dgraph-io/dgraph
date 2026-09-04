/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package edgraph

import (
	"bytes"
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/acl"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/worker"
	"github.com/dgraph-io/dgraph/v25/x"
)

// capabilitySites is every operation that requires a capability, and which one.
//
// This is a decision table, not a description. Each entry was AuthSuperAdmin or
// AuthorizeGuardians before capabilities existed, and the mapping is where the
// judgement lives: whether an operation is cluster-wide, tenant-scoped, or a
// cross-tenant write. Getting one wrong grants or removes real authority.
//
// TestEveryCapabilitySiteIsDeclared parses the source and requires this table to
// match, in both directions. A silent reclassification is the failure it exists to
// prevent — changing the constant at a call site is a one-word edit that reads as
// harmless in review, and the tests that would catch it are integration tests that
// only cover the paths someone thought to exercise.
var capabilitySites = map[string]Capability{
	// Cluster-wide authority.
	"edgraph/server.go:alter":              CapClusterAdmin, // drop-all
	"edgraph/namespace.go:CreateNamespace": CapClusterAdmin,
	"edgraph/namespace.go:DropNamespace":   CapClusterAdmin,
	"edgraph/namespace.go:ListNamespaces":  CapClusterAdmin,

	"graphql/resolve/middlewares.go:resolveGuardianOfTheGalaxyAuth": CapClusterAdmin,
	// Leasing UIDs. Not administration: `dgraph live` does it on every run, which is
	// why it is not CapClusterAdmin.
	"edgraph/zero.go:AllocateIDs": CapLeaseUIDs,

	// Acting in a namespace other than the caller's own. Distinct from
	// CapClusterAdmin even though the built-in policy resolves them identically —
	// the live loader needs a cross-tenant write and has no business dropping the
	// cluster.
	"edgraph/server.go:parseSchemaFromAlterOperation": CapAssumeTenant,
	"edgraph/server.go:doQuery":                       CapAssumeTenant,

	// Administrative within a tenant: guardians of any namespace, which is what
	// these checked before.
	"edgraph/server.go:Health":                           CapTenantAdmin,
	"edgraph/server.go:State":                            CapTenantAdmin,
	"edgraph/server.go:UpdateExtSnapshotStreamingState":  CapTenantAdmin,
	"edgraph/server.go:StreamExtSnapshot":                CapTenantAdmin,
	"graphql/resolve/middlewares.go:resolveGuardianAuth": CapTenantAdmin,
}

// capabilityScanRoot is the repo root, walked in full.
//
// This was a list of directories, which is how it came to miss
// a caller in a package nobody thought to add to the list. A decision table whose
// scan can miss a decision site is worse than no table: it reports a completeness it
// has not checked. Walking everything removes the class, and keeps working for a
// caller added in a package that does not exist yet.
const capabilityScanRoot = ".."

// capabilityScanSkip are directories with nothing to say about capabilities and a lot
// of files, skipped to keep the walk quick.
var capabilityScanSkip = map[string]bool{
	".git": true, "vendor": true, "testdata": true, "protos": true,
	"compose": true, "contrib": true, ".trunk": true, "systest": true,
}

// scanCapabilitySites parses the source and returns "dir/file.go:FuncName" for
// every AuthorizeCapability call, mapped to the capability constant it passes.
func scanCapabilitySites(t *testing.T) map[string]string {
	t.Helper()
	found := make(map[string]string)
	var parsed int

	err := filepath.WalkDir(capabilityScanRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if capabilityScanSkip[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		// Cheap pre-filter: parsing every Go file in the repo is wasteful when almost
		// none of them mention the function.
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if !bytes.Contains(src, []byte("AuthorizeCapability")) {
			return nil
		}
		parsed++

		fset := token.NewFileSet()
		af, parseErr := parser.ParseFile(fset, path, src, 0)
		require.NoError(t, parseErr, "parsing %s", path)

		for _, decl := range af.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			// The dispatcher and the interface method take a capability as a parameter
			// rather than naming one, so they are not decision sites.
			if fd.Name.Name == "AuthorizeCapability" {
				continue
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || callee(call.Fun) != "AuthorizeCapability" || len(call.Args) < 2 {
					return true
				}
				key := sitePath(path) + ":" + fd.Name.Name
				if prev, dup := found[key]; dup && prev != identName(call.Args[1]) {
					t.Errorf("%s gates on two different capabilities (%s and %s); split the "+
						"function or the table cannot describe it", key, prev, identName(call.Args[1]))
				}
				found[key] = identName(call.Args[1])
				return true
			})
		}
		return nil
	})
	require.NoError(t, err)
	require.Positive(t, parsed, "the walk parsed no files mentioning AuthorizeCapability; "+
		"capabilityScanRoot is wrong and this test proves nothing")
	return found
}

func callee(fn ast.Expr) string {
	switch f := fn.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	}
	return ""
}

func identName(e ast.Expr) string {
	switch a := e.(type) {
	case *ast.Ident:
		return a.Name
	case *ast.SelectorExpr:
		return a.Sel.Name
	}
	return ""
}

// sitePath normalizes a walked path to be repo-relative, so the table reads the same
// regardless of where the test runs from.
func sitePath(path string) string {
	clean := filepath.ToSlash(filepath.Clean(path))
	return strings.TrimPrefix(clean, "../")
}

// capabilityNames maps the constant identifiers back to values, so the table can
// be written in terms of the constants while the scan sees only names.
var capabilityNames = map[string]Capability{
	"CapClusterAdmin": CapClusterAdmin,
	"CapTenantAdmin":  CapTenantAdmin,
	"CapAssumeTenant": CapAssumeTenant,
	"CapLeaseUIDs":    CapLeaseUIDs,
}

func TestEveryCapabilitySiteIsDeclared(t *testing.T) {
	found := scanCapabilitySites(t)

	for site, gotName := range found {
		want, declared := capabilitySites[site]
		if !declared {
			t.Errorf("%s gates on %s but is not in capabilitySites. Add it, and say why that "+
				"capability rather than another — the table is where the decision is recorded.",
				site, gotName)
			continue
		}
		got, known := capabilityNames[gotName]
		if !known {
			t.Errorf("%s passes %s, which is not a known capability constant", site, gotName)
			continue
		}
		if got != want {
			t.Errorf("%s gates on %v, but capabilitySites says %v. If the change is intended, "+
				"change the table too — that is the review the constant alone does not get.",
				site, got, want)
		}
	}

	for site := range capabilitySites {
		if _, ok := found[site]; !ok {
			t.Errorf("capabilitySites declares %s, which no longer gates on a capability. "+
				"Either the check was dropped — which is a privilege change — or it moved and "+
				"the table needs updating.", site)
		}
	}
}

// TestCapabilitySiteCoverage guards against the whole table being deleted or the
// scan silently matching nothing, which would make the test above vacuously pass.
func TestCapabilitySiteCoverage(t *testing.T) {
	found := scanCapabilitySites(t)
	require.NotEmpty(t, found, "the scan found no capability sites at all")
	require.Len(t, found, len(capabilitySites))

	// Every capability must be exercised somewhere, or it is dead surface.
	var used []Capability
	for _, c := range capabilitySites {
		if !slices.Contains(used, c) {
			used = append(used, c)
		}
	}
	require.Len(t, used, len(capabilityNames), "some capability has no call site")
}

// fromIP builds a context that looks like a gRPC request from ip, which is what
// the IP whitelist reads. Every test harness sets whitelist=0.0.0.0/0, so without
// constructing this by hand the deny half of break-glass is never exercised.
func fromIP(t *testing.T, ip string) context.Context {
	t.Helper()
	addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(ip, "50051"))
	require.NoError(t, err)
	return peer.NewContext(context.Background(), &peer.Peer{Addr: addr})
}

// withAuthToken adds the --security token header a caller would present.
func withAuthToken(ctx context.Context, token string) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		md = metadata.New(nil)
	} else {
		md = md.Copy()
	}
	md.Set("auth-token", token)
	return metadata.NewIncomingContext(ctx, md)
}

// aclCtx builds a request context carrying a signed ACL token, resolved to a
// Principal the way the identity interceptor does.
func aclCtx(t *testing.T, namespace uint64, userID string, groups []string) context.Context {
	t.Helper()
	token := generateJWT(namespace, userID, groups, time.Now().Add(30*time.Minute).Unix())
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("accessJwt", token))
	return x.WithResolvedIdentity(ctx)
}

// TestPredicateACLCapabilities pins the built-in policy's behavior, including the
// error codes and the message text. Both matter beyond this package: several
// endpoints wrap the message and clients distinguish Unauthenticated from
// PermissionDenied.
func TestPredicateACLCapabilities(t *testing.T) {
	prevEnabled, prevSecret := x.WorkerConfig.AclEnabled, worker.Config.AclSecretKey
	t.Cleanup(func() {
		x.WorkerConfig.AclEnabled, worker.Config.AclSecretKey = prevEnabled, prevSecret
	})

	policy := predicateACL{}

	t.Run("with ACL off, cluster authority requires break-glass", func(t *testing.T) {
		x.WorkerConfig.AclEnabled = false
		worker.Config.AclSecretKey = nil

		// This is the behavior change. Before, an ACL-off cluster granted
		// CapClusterAdmin to anyone, which is why CreateNamespace, DropNamespace,
		// ListNamespaces, and AllocateIDs were reachable by any client that could
		// open a connection.
		err := policy.AuthorizeCapability(context.Background(), CapClusterAdmin)
		require.Error(t, err, "an unattributable caller must not be a cluster admin")
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		require.Contains(t, err.Error(), "break-glass",
			"the denial should name the route an operator can actually use")

		// A whitelisted source IP with no auth token configured is the default
		// break-glass configuration, and it grants.
		require.NoError(t, policy.AuthorizeCapability(fromIP(t, "127.0.0.1"), CapClusterAdmin))

		// A source IP outside the whitelist does not, which is the half of the rule
		// every test cluster hides: the harnesses all set whitelist=0.0.0.0/0.
		err = policy.AuthorizeCapability(fromIP(t, "203.0.113.7"), CapClusterAdmin)
		require.Error(t, err)
		require.Equal(t, codes.PermissionDenied, status.Code(err))

		// The others are unchanged with ACL off, each for its own reason recorded at
		// the switch in predicate_acl.go. Asserted here so that changing any of them
		// is a deliberate act rather than a side effect.
		require.NoError(t, policy.AuthorizeCapability(context.Background(), CapAssumeTenant),
			"tightening this is the deferred fail-closed galaxy-operation change")
		require.NoError(t, policy.AuthorizeCapability(context.Background(), CapTenantAdmin),
			"tightening this would newly gate /state and /health?all")
		require.NoError(t, policy.AuthorizeCapability(context.Background(), CapLeaseUIDs),
			"`dgraph live` leases UIDs on every run against an ACL-off cluster and has "+
				"never needed a credential; requiring one is a loader-breaking change")
	})

	t.Run("with ACL off, the auth token is required when one is configured", func(t *testing.T) {
		x.WorkerConfig.AclEnabled = false
		worker.Config.AclSecretKey = nil
		prevToken := worker.Config.AuthToken
		t.Cleanup(func() { worker.Config.AuthToken = prevToken })
		worker.Config.AuthToken = "operator-token"

		// Whitelisted IP alone is no longer enough once a token exists.
		err := policy.AuthorizeCapability(fromIP(t, "127.0.0.1"), CapClusterAdmin)
		require.Error(t, err, "a configured auth token must be presented")
		require.Equal(t, codes.PermissionDenied, status.Code(err))

		require.NoError(t, policy.AuthorizeCapability(
			withAuthToken(fromIP(t, "127.0.0.1"), "operator-token"), CapClusterAdmin))

		err = policy.AuthorizeCapability(
			withAuthToken(fromIP(t, "127.0.0.1"), "wrong-token"), CapClusterAdmin)
		require.Error(t, err, "a wrong token must not grant")
	})

	t.Run("with ACL on, break-glass is not a second route to cluster admin", func(t *testing.T) {
		x.WorkerConfig.AclEnabled = true
		worker.Config.AclSecretKey = x.Sensitive("6ABBAA2014CFF00289D20D20DA296F67")
		prevToken := worker.Config.AuthToken
		t.Cleanup(func() { worker.Config.AuthToken = prevToken })
		worker.Config.AuthToken = "operator-token"

		// Satisfying break-glass completely, but presenting no ACL token. Granting
		// here would let the --security token administer the cluster with no ACL
		// identity at all, which is a loosening rather than the tightening this
		// change is for.
		ctx := withAuthToken(fromIP(t, "127.0.0.1"), "operator-token")
		err := policy.AuthorizeCapability(ctx, CapClusterAdmin)
		require.Error(t, err, "with ACL on, guardianship is the only route")
		require.NotContains(t, err.Error(), "break-glass",
			"the ACL-on denial should come from the ACL rule, not the source list")
	})

	x.WorkerConfig.AclEnabled = true
	worker.Config.AclSecretKey = x.Sensitive("6ABBAA2014CFF00289D20D20DA296F67")

	tests := []struct {
		name     string
		ctx      func(t *testing.T) context.Context
		cap      Capability
		wantCode codes.Code
		wantMsg  string
	}{
		{
			name: "a root-namespace guardian is a cluster admin",
			ctx:  func(t *testing.T) context.Context { return aclCtx(t, 0, "groot", []string{"guardians"}) },
			cap:  CapClusterAdmin,
		},
		{
			name: "and may assume another tenant",
			ctx:  func(t *testing.T) context.Context { return aclCtx(t, 0, "groot", []string{"guardians"}) },
			cap:  CapAssumeTenant,
		},
		{
			name: "and is a tenant admin",
			ctx:  func(t *testing.T) context.Context { return aclCtx(t, 0, "groot", []string{"guardians"}) },
			cap:  CapTenantAdmin,
		},
		{
			name:     "a guardian of another namespace is not a cluster admin",
			ctx:      func(t *testing.T) context.Context { return aclCtx(t, 7, "groot", []string{"guardians"}) },
			cap:      CapClusterAdmin,
			wantCode: codes.PermissionDenied,
			wantMsg:  "Only superadmin is allowed to do this operation",
		},
		{
			name:     "nor may it assume another tenant",
			ctx:      func(t *testing.T) context.Context { return aclCtx(t, 7, "groot", []string{"guardians"}) },
			cap:      CapAssumeTenant,
			wantCode: codes.PermissionDenied,
			wantMsg:  "Only superadmin is allowed to do this operation",
		},
		{
			name: "but it is a tenant admin, which is the distinction",
			ctx:  func(t *testing.T) context.Context { return aclCtx(t, 7, "groot", []string{"guardians"}) },
			cap:  CapTenantAdmin,
		},
		{
			name:     "a non-guardian in the root namespace is denied",
			ctx:      func(t *testing.T) context.Context { return aclCtx(t, 0, "alice", []string{"dev"}) },
			cap:      CapClusterAdmin,
			wantCode: codes.PermissionDenied,
			wantMsg:  "is not a member of guardians group",
		},
		{
			name:     "a non-guardian is not a tenant admin either",
			ctx:      func(t *testing.T) context.Context { return aclCtx(t, 0, "alice", []string{"dev"}) },
			cap:      CapTenantAdmin,
			wantCode: codes.PermissionDenied,
			wantMsg:  "is not a member of guardians group",
		},
		{
			name:     "a caller with no groups at all is denied",
			ctx:      func(t *testing.T) context.Context { return aclCtx(t, 0, "alice", nil) },
			cap:      CapClusterAdmin,
			wantCode: codes.PermissionDenied,
			wantMsg:  "is not a member of guardians group",
		},
		{
			name:     "no token is PermissionDenied for a tenant admin",
			ctx:      func(t *testing.T) context.Context { return context.Background() },
			cap:      CapTenantAdmin,
			wantCode: codes.PermissionDenied,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := policy.AuthorizeCapability(tt.ctx(t), tt.cap)
			if tt.wantCode == codes.OK {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Equal(t, tt.wantCode, status.Code(err), "err=%v", err)
			if tt.wantMsg != "" {
				require.Contains(t, err.Error(), tt.wantMsg)
			}
		})
	}
}

// TestUnknownCapabilityFailsClosed covers the branch that has no case. A
// capability added without a rule must be denied, because the alternative reading
// leaves every future addition unrestricted until someone notices.
func TestUnknownCapabilityFailsClosed(t *testing.T) {
	err := predicateACL{}.AuthorizeCapability(context.Background(), Capability(99))
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	require.Contains(t, err.Error(), "no rule for")
	require.Equal(t, "unknown-capability", Capability(99).String())
}

// fakeController records what it was asked and grants everything.
type fakeController struct {
	asked      []Capability
	askedPreds [][]string
}

func (*fakeController) Name() string { return "fake" }

func (f *fakeController) AuthorizeCapability(_ context.Context, c Capability) error {
	f.asked = append(f.asked, c)
	return nil
}

func (f *fakeController) AuthorizePredicates(_ context.Context, preds []string,
	_ *acl.Operation) (*PredResult, error) {
	f.askedPreds = append(f.askedPreds, preds)
	return &PredResult{Blocked: map[string]struct{}{}}, nil
}

func TestSetAccessController(t *testing.T) {
	prevEnabled := x.WorkerConfig.AclEnabled
	t.Cleanup(func() {
		x.WorkerConfig.AclEnabled = prevEnabled
		SetAccessController(nil)
	})
	// ACL on, so the built-in policy would deny an unauthenticated caller. If the
	// installed one is consulted instead, the call succeeds.
	x.WorkerConfig.AclEnabled = true

	require.Equal(t, "predicate-acl", currentAccessController().Name())

	fake := &fakeController{}
	SetAccessController(fake)
	require.Equal(t, "fake", currentAccessController().Name())
	require.NoError(t, AuthorizeCapability(context.Background(), CapClusterAdmin))
	require.Equal(t, []Capability{CapClusterAdmin}, fake.asked)

	SetAccessController(nil)
	require.Equal(t, "predicate-acl", currentAccessController().Name(),
		"nil must restore the built-in policy")
}

// TestCapabilityStringsAreStable guards the names that appear in the fail-closed
// error, which is what tells an operator which capability to grant.
func TestCapabilityStringsAreStable(t *testing.T) {
	require.Equal(t, "cluster-admin", CapClusterAdmin.String())
	require.Equal(t, "tenant-admin", CapTenantAdmin.String())
	require.Equal(t, "assume-tenant", CapAssumeTenant.String())
}

// erroringController stands in for a policy that cannot reach an answer — a
// one backed by a remote authorization service whose query fails, say.
type erroringController struct{}

func (erroringController) Name() string { return "erroring" }
func (erroringController) AuthorizeCapability(context.Context, Capability) error {
	return status.Error(codes.Unavailable, "policy unavailable")
}
func (erroringController) AuthorizePredicates(context.Context, []string,
	*acl.Operation) (*PredResult, error) {
	return nil, status.Error(codes.Unavailable, "policy unavailable")
}

// TestAuthorizePredicatesGoesThroughThePolicy checks the plumbing rather than
// ACL's rules — those are covered end to end by acl/acl_test.go against a live
// cluster, which is the gate for this change.
func TestAuthorizePredicatesGoesThroughThePolicy(t *testing.T) {
	t.Cleanup(func() { SetAccessController(nil) })

	fake := &fakeController{}
	SetAccessController(fake)

	res, err := AuthorizePredicates(context.Background(), []string{"name", "age"}, acl.Read)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, [][]string{{"name", "age"}}, fake.askedPreds)

	SetAccessController(nil)
	require.Equal(t, "predicate-acl", currentAccessController().Name())
}

// TestPredicateErrorIsNotReadAsPermitted is the property the new error return
// creates a way to get wrong.
//
// authorizePreds could not fail, so every call site treated "no blocked
// predicates" as the only outcome. A policy that has to query the graph can fail,
// and an empty PredResult from a failed call would look exactly like "nothing was
// refused" — which is to say, like permission. Each site must return the error
// instead.
func TestPredicateErrorIsNotReadAsPermitted(t *testing.T) {
	prevEnabled, prevSecret := x.WorkerConfig.AclEnabled, worker.Config.AclSecretKey
	t.Cleanup(func() {
		x.WorkerConfig.AclEnabled, worker.Config.AclSecretKey = prevEnabled, prevSecret
		SetAccessController(nil)
	})
	x.WorkerConfig.AclEnabled = true
	worker.Config.AclSecretKey = x.Sensitive("6ABBAA2014CFF00289D20D20DA296F67")
	SetAccessController(erroringController{})

	// A non-guardian, so the superadmin short-circuit does not fire and the
	// predicate decision is actually consulted.
	ctx := aclCtx(t, 0, "alice", []string{"dev"})

	err := authorizeAlter(ctx, &api.Operation{Schema: "name: string ."})
	require.Error(t, err, "a policy that could not decide must not be read as permitting")
	require.Equal(t, codes.Unavailable, status.Code(err),
		"the policy's own code should survive rather than becoming PermissionDenied")
}

// TestPredResultSentinel pins the meaning of a nil Allowed, which is the contract
// the interface had to carry over and the one a plain error return could not
// express. Nil means every predicate; an empty slice means none.
func TestPredResultSentinel(t *testing.T) {
	all := &PredResult{Allowed: nil, Blocked: map[string]struct{}{"dgraph.xid": {}}}
	require.Nil(t, all.Allowed, "nil Allowed is 'all predicates', and Blocked still applies")
	require.Len(t, all.Blocked, 1)

	none := &PredResult{Allowed: []string{}, Blocked: map[string]struct{}{}}
	require.NotNil(t, none.Allowed, "an empty slice is 'no predicates' and must not be nil")
	require.Empty(t, none.Allowed)
}

// TestClusterAdminDoesNotTrustClientNamespace is the regression test for a
// privilege escalation introduced by the tenancy rekeying.
//
// authSuperAdmin's `ns != 0` test asks whether the CALLER is rooted in the root
// namespace. That is a property of the credential, not of where the request is
// being routed — so it must read the signed claim. Rekeying it onto
// requestNamespace made it prefer x.ExtractNamespace, which reads md["namespace"]
// from incoming metadata, and that is client-controlled server-side.
//
// The four RPCs that gate on CapClusterAdmin and nothing else — CreateNamespace,
// DropNamespace, ListNamespaces, AllocateIDs — call AuthorizeCapability as their
// first statement, with no ResolveTenant before it. So on exactly those paths the
// metadata is whatever the caller sent, and a guardian of any tenant could claim
// namespace 0 and administer the cluster.
func TestClusterAdminDoesNotTrustClientNamespace(t *testing.T) {
	prevEnabled, prevSecret := x.WorkerConfig.AclEnabled, worker.Config.AclSecretKey
	t.Cleanup(func() {
		x.WorkerConfig.AclEnabled, worker.Config.AclSecretKey = prevEnabled, prevSecret
	})
	x.WorkerConfig.AclEnabled = true
	worker.Config.AclSecretKey = x.Sensitive("6ABBAA2014CFF00289D20D20DA296F67")

	// A real guardian, but of namespace 7 — not a cluster admin.
	token := generateJWT(7, "tenant-groot", []string{"guardians"}, time.Now().Add(30*time.Minute).Unix())

	t.Run("a tenant guardian claiming namespace 0 in metadata is denied", func(t *testing.T) {
		// The forgery: a valid ns-7 token, plus metadata asserting the request is in
		// namespace 0. No ResolveTenant runs on this path to overwrite it.
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
			"accessJwt", token,
			"namespace", "0",
		))
		ctx = x.WithResolvedIdentity(ctx)

		err := predicateACL{}.AuthorizeCapability(ctx, CapClusterAdmin)
		require.Error(t, err, "client-supplied namespace metadata must not confer cluster admin")
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		require.Contains(t, err.Error(), "Only superadmin is allowed to do this operation")
	})

	t.Run("the same token is still denied with no namespace metadata at all", func(t *testing.T) {
		ctx := x.WithResolvedIdentity(metadata.NewIncomingContext(
			context.Background(), metadata.Pairs("accessJwt", token)))
		err := predicateACL{}.AuthorizeCapability(ctx, CapClusterAdmin)
		require.Error(t, err)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
	})

	t.Run("a genuine root guardian is still allowed", func(t *testing.T) {
		rootToken := generateJWT(0, "groot", []string{"guardians"}, time.Now().Add(30*time.Minute).Unix())
		ctx := x.WithResolvedIdentity(metadata.NewIncomingContext(
			context.Background(), metadata.Pairs("accessJwt", rootToken)))
		require.NoError(t, predicateACL{}.AuthorizeCapability(ctx, CapClusterAdmin))
	})

	t.Run("a root guardian is allowed even if metadata claims another namespace", func(t *testing.T) {
		// The mirror image: the claim governs, so hostile metadata cannot demote
		// a legitimate cluster admin either.
		rootToken := generateJWT(0, "groot", []string{"guardians"}, time.Now().Add(30*time.Minute).Unix())
		ctx := x.WithResolvedIdentity(metadata.NewIncomingContext(context.Background(),
			metadata.Pairs("accessJwt", rootToken, "namespace", "7")))
		require.NoError(t, predicateACL{}.AuthorizeCapability(ctx, CapClusterAdmin))
	})
}

// TestFilterTabletsDoesNotTrustClientNamespace is the /state half of the same
// escalation. filterTablets returns early without filtering when the namespace is
// the root one, and State reaches it with no ResolveTenant ahead of it — so reading
// md["namespace"] let a guardian of any tenant ask for namespace 0 and receive
// every tenant's predicates.
func TestFilterTabletsDoesNotTrustClientNamespace(t *testing.T) {
	prevEnabled := x.WorkerConfig.AclEnabled
	t.Cleanup(func() { x.WorkerConfig.AclEnabled = prevEnabled })
	x.WorkerConfig.AclEnabled = true

	state := func() *pb.MembershipState {
		return &pb.MembershipState{Groups: map[uint32]*pb.Group{1: {Tablets: map[string]*pb.Tablet{
			x.NamespaceAttr(7, "mine"):     {Predicate: x.NamespaceAttr(7, "mine")},
			x.NamespaceAttr(9, "theirs"):   {Predicate: x.NamespaceAttr(9, "theirs")},
			x.NamespaceAttr(0, "rootpred"): {Predicate: x.NamespaceAttr(0, "rootpred")},
		}}}}
	}
	token := generateJWT(7, "tenant-groot", []string{"guardians"}, time.Now().Add(30*time.Minute).Unix())

	t.Run("claiming namespace 0 in metadata does not lift the filter", func(t *testing.T) {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
			"accessJwt", token, "namespace", "0"))
		ms := state()
		require.NoError(t, filterTablets(ctx, ms))

		tablets := ms.GetGroups()[1].GetTablets()
		require.Contains(t, tablets, "mine", "the caller's own tenant is visible")
		require.NotContains(t, tablets, "theirs", "another tenant's predicate must not be disclosed")
		require.NotContains(t, tablets, "rootpred", "the root namespace's predicate must not be disclosed")
		require.Len(t, tablets, 1)
	})

	t.Run("a genuine root guardian still sees everything", func(t *testing.T) {
		rootToken := generateJWT(0, "groot", []string{"guardians"}, time.Now().Add(30*time.Minute).Unix())
		ctx := metadata.NewIncomingContext(context.Background(),
			metadata.Pairs("accessJwt", rootToken))
		ms := state()
		require.NoError(t, filterTablets(ctx, ms))
		require.Len(t, ms.GetGroups()[1].GetTablets(), 3, "root sees unfiltered state")
	})
}

// identitySource is a stand-in for an identity-based CapabilitySource, e.g.
// --external-identity cluster-admin-clients.
type identitySource struct {
	subject string
	asked   int
}

func (*identitySource) Name() string { return "test-identity-source" }
func (s *identitySource) Grants(_ context.Context, p *x.Principal, c Capability) bool {
	s.asked++
	return c == CapClusterAdmin && p != nil && p.Subject == s.subject
}

// TestCapabilitySourcesAreConsultedUnderACL is the regression test for a silent
// misconfiguration: authorizeClusterAdmin returned authSuperAdmin's result directly
// whenever ACL was enabled, so no registered source was ever asked. A deployment
// that configured cluster-admin-clients AND ran ACL would have found the roster
// inert, with nothing in the logs or the error saying so.
func TestCapabilitySourcesAreConsultedUnderACL(t *testing.T) {
	prevEnabled, prevSecret := x.WorkerConfig.AclEnabled, worker.Config.AclSecretKey
	prevSources := capabilitySources
	t.Cleanup(func() {
		x.WorkerConfig.AclEnabled, worker.Config.AclSecretKey = prevEnabled, prevSecret
		capabilitySources = prevSources
	})
	x.WorkerConfig.AclEnabled = true
	worker.Config.AclSecretKey = x.Sensitive("6ABBAA2014CFF00289D20D20DA296F67")

	src := &identitySource{subject: "migrator"}
	capabilitySources = append(prevSources, src)

	admin := &x.Principal{
		Issuer:  "https://idp.example/identity",
		Subject: "migrator",
		Claims:  map[string]any{"user_type": "agent"},
		Method:  x.MethodExternalJWT,
	}

	t.Run("a source can grant even while ACL is enabled", func(t *testing.T) {
		ctx := x.WithPrincipal(context.Background(), admin)
		require.NoError(t, predicateACL{}.AuthorizeCapability(ctx, CapClusterAdmin))
		require.Positive(t, src.asked, "the source must actually be consulted")
	})

	t.Run("a caller no source recognizes still gets the ACL error", func(t *testing.T) {
		// A real guardian of a non-root namespace, so the ACL rule produces its own
		// PermissionDenied. The point is that consulting sources must not replace
		// that message with the generic source-list one: an operator debugging an
		// ACL deployment needs to be told about guardianship, not about
		// --security whitelist.
		token := generateJWT(7, "tenant-groot", []string{"guardians"},
			time.Now().Add(30*time.Minute).Unix())
		ctx := x.WithResolvedIdentity(metadata.NewIncomingContext(
			context.Background(), metadata.Pairs("accessJwt", token)))

		err := predicateACL{}.AuthorizeCapability(ctx, CapClusterAdmin)
		require.Error(t, err)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		require.Contains(t, err.Error(), "Only superadmin is allowed to do this operation",
			"the ACL rule's own text must survive rather than the generic denial")
		require.NotContains(t, err.Error(), "With ACL disabled it comes from")
	})

	t.Run("a source is not asked for a capability it does not grant", func(t *testing.T) {
		ctx := x.WithPrincipal(context.Background(), admin)
		// CapAssumeTenant has no source route; the ACL rule decides alone.
		err := predicateACL{}.AuthorizeCapability(ctx, CapAssumeTenant)
		require.Error(t, err, "naming a cluster-admin client must not confer assume-tenant")
	})
}

// TestBreakGlassSelfExcludesUnderACL pins where the ACL-on restriction now lives.
// It moved from authorizeClusterAdmin into the source so that it applies to the one
// route it was reasoned about — a shared secret plus an IP range — rather than to
// every source anyone adds later.
func TestBreakGlassSelfExcludesUnderACL(t *testing.T) {
	prevEnabled, prevToken := x.WorkerConfig.AclEnabled, worker.Config.AuthToken
	t.Cleanup(func() {
		x.WorkerConfig.AclEnabled, worker.Config.AuthToken = prevEnabled, prevToken
	})
	worker.Config.AuthToken = ""

	// Fully satisfying break-glass: a loopback caller with no token configured.
	ctx := fromIP(t, "127.0.0.1")

	x.WorkerConfig.AclEnabled = false
	require.True(t, breakGlassSource{}.Grants(ctx, nil, CapClusterAdmin),
		"with ACL off, break-glass is the route")

	x.WorkerConfig.AclEnabled = true
	require.False(t, breakGlassSource{}.Grants(ctx, nil, CapClusterAdmin),
		"with ACL on, the --security token must not be a second route to cluster admin")
}

// TestClusterAdminWithoutATokenIsUnauthenticated pins the status code on the
// credential-missing path.
//
// authSuperAdmin wrapped ExtractNamespaceFrom's plain error, and every caller
// propagates status.Convert(err).Code() — so an absent or expired token on
// CreateNamespace, DropNamespace, ListNamespaces or the GraphQL admin surface arrived
// as codes.Unknown. A client cannot distinguish that from a server fault, and
// authorizeGuardians twelve lines below already classified the same failure as
// Unauthenticated.
func TestClusterAdminWithoutATokenIsUnauthenticated(t *testing.T) {
	prevEnabled, prevSecret := x.WorkerConfig.AclEnabled, worker.Config.AclSecretKey
	t.Cleanup(func() {
		x.WorkerConfig.AclEnabled, worker.Config.AclSecretKey = prevEnabled, prevSecret
	})
	x.WorkerConfig.AclEnabled = true
	worker.Config.AclSecretKey = x.Sensitive("6ABBAA2014CFF00289D20D20DA296F67")

	for _, tt := range []struct {
		name string
		ctx  context.Context
	}{
		{"no metadata at all", context.Background()},
		{"metadata but no token", metadata.NewIncomingContext(
			context.Background(), metadata.Pairs("namespace", "0"))},
		{"a token that does not parse", metadata.NewIncomingContext(
			context.Background(), metadata.Pairs("accessJwt", "not-a-jwt"))},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := predicateACL{}.AuthorizeCapability(tt.ctx, CapClusterAdmin)
			require.Error(t, err)
			require.Equal(t, codes.Unauthenticated, status.Code(err),
				"a credential problem must not reach the client as Unknown")
		})
	}

	t.Run("an expired token is also Unauthenticated", func(t *testing.T) {
		expired := generateJWT(0, "groot", []string{"guardians"},
			time.Now().Add(-time.Hour).Unix())
		ctx := metadata.NewIncomingContext(context.Background(),
			metadata.Pairs("accessJwt", expired))
		err := predicateACL{}.AuthorizeCapability(ctx, CapClusterAdmin)
		require.Error(t, err)
		require.Equal(t, codes.Unauthenticated, status.Code(err))
	})
}
