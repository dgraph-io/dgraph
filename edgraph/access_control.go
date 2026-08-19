/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package edgraph

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/v25/acl"
	"github.com/dgraph-io/dgraph/v25/x"
)

// Capability is an authority over the cluster or a tenant, as opposed to access to
// particular predicates. Capabilities answer "may this caller perform this class of
// operation at all", and they are deliberately few: an operation that needs a
// finer distinction should be authorized on its predicates instead.
type Capability int

const (
	// CapClusterAdmin is authority over the whole cluster: creating and dropping
	// namespaces, dropping all data, resetting passwords across tenants. Nothing
	// scoped to a single tenant should require it.
	CapClusterAdmin Capability = iota

	// CapTenantAdmin is administrative authority within a tenant — reading cluster
	// state and health, arming an external-snapshot import.
	//
	// It is a weaker capability than CapClusterAdmin, and under the built-in policy
	// the difference is real but narrow: CapClusterAdmin additionally requires the
	// caller to be in the root namespace.
	CapTenantAdmin

	// CapAssumeTenant is the authority to act in a namespace other than the
	// caller's own. The live loader needs it for a galaxy-wide write.
	//
	// Split out from CapClusterAdmin even though the built-in policy resolves both
	// identically, because they are different powers and conflating them costs
	// something later: a scoped service account should be able to write across
	// tenants without also being able to drop the cluster.
	CapAssumeTenant

	// CapLeaseUIDs is the authority to lease a block of UIDs from Zero.
	//
	// Separate from CapClusterAdmin, which it briefly shared, because the callers
	// are not administrators: `dgraph live` allocates UIDs through this RPC on every
	// run — xidmap falls back to it whenever it was built without a direct Zero
	// connection — so folding it in made the loader require a whitelisted IP or an
	// auth token against any cluster running without ACL, where it had needed
	// neither.
	//
	// Tightening it is the same change as fail-closed galaxy-operation, with the same
	// cost and the same answer: it wants a flag-gated release rather than to ride
	// along. Under ACL it is guardian-of-the-root-namespace, exactly as it was before
	// capabilities existed.
	CapLeaseUIDs
)

func (c Capability) String() string {
	switch c {
	case CapClusterAdmin:
		return "cluster-admin"
	case CapTenantAdmin:
		return "tenant-admin"
	case CapAssumeTenant:
		return "assume-tenant"
	case CapLeaseUIDs:
		return "lease-uids"
	}
	return "unknown-capability"
}

// AccessController decides what a verified caller may do.
//
// It answers only authorization. Who the caller is has already been established by
// the time a method here runs — x.PrincipalFrom carries the answer — and which
// tenant the request operates in is the TenantResolver's business. Keeping all
// three apart is the point of the surrounding work; an implementation that
// verifies a credential or derives a namespace has taken on someone else's job.
//
// Two methods rather than one, because the two questions have different shapes.
// A capability is a yes or no. Predicate access is a partition: the answer is
// which of the requested predicates are usable, and the caller proceeds with
// those.
type AccessController interface {
	// Name identifies the policy, for logs and for the error an unimplemented
	// capability produces.
	Name() string

	// AuthorizeCapability reports whether the caller on ctx holds c, returning a
	// gRPC status error when it does not.
	//
	// Callers must not wrap the returned error. They may prepend context to its
	// message, but the code has to survive — several endpoints distinguish
	// Unauthenticated from PermissionDenied, and clients act on that difference.
	AuthorizeCapability(ctx context.Context, c Capability) error

	// AuthorizePredicates partitions preds into what the caller may and may not
	// access under op.
	//
	// It deliberately does not return "denied". A query names predicates the caller
	// may have no business seeing, and the established behavior is to drop them and
	// answer the rest rather than refuse the whole request — so the result is data
	// the caller acts on, not a verdict. See PredResult.
	//
	// The error is for a policy that could not reach an answer at all: the built-in
	// one reads a local cache and never fails, but a policy that has to query the
	// graph can. An error must not be read as a denial.
	//
	// Identity comes from ctx, not from a parameter. Handing a policy Dgraph's own
	// userData would make every implementation speak ACL.
	AuthorizePredicates(ctx context.Context, preds []string, op *acl.Operation) (*PredResult, error)
}

// PredResult is how a policy partitions the predicates a request named.
type PredResult struct {
	// Allowed lists the predicates the caller may access — but a nil Allowed means
	// *all* of them, not none.
	//
	// The sentinel is load-bearing and it predates this interface: a caller with
	// blanket access has no enumerable predicate list, and materializing one would
	// mean listing every predicate in the namespace on every request. Blocked is
	// still honored when Allowed is nil, which is how ACL predicates stay
	// unreadable even to a caller that may read everything else.
	Allowed []string

	// Blocked is the subset of the requested predicates the caller may not access.
	// Empty means none were refused.
	Blocked map[string]struct{}
}

// namespaceOrClaim reports the namespace a request OPERATES IN, preferring the
// resolved tenancy over the token's claim. Its one caller is authorizePreds, and
// that is deliberate.
//
// The distinction this function exists for, learned the hard way: "which tenant's
// data does this request touch" and "who is this caller" are different questions
// with different correct sources. Predicate authorization is the first — it must
// key on the same channel storage keys on, or it authorizes in one namespace while
// executing in another. Everything asking the second — authSuperAdmin's root-
// namespace test, filterTablets, shouldAllowAcls — must read the signed claim,
// because the resolved value comes from md["namespace"], which is client-supplied
// until a resolver overwrites it, and not every path resolves before authorizing.
//
// A divergence is logged rather than rejected: it should be impossible, and turning
// "impossible" into an outage is worse than making it visible.
func namespaceOrClaim(ctx context.Context, claim uint64, site string) uint64 {
	resolved, err := x.ExtractNamespace(ctx)
	if err != nil {
		glog.Warningf("%s: no resolved tenancy on the context (%v); falling back to the "+
			"token's claim %#x", site, err, claim)
		return claim
	}
	if resolved != claim {
		glog.Warningf("%s: resolved namespace %#x differs from the token's claim %#x; "+
			"authorizing against the resolved one", site, resolved, claim)
	}
	return resolved
}

// CapabilitySource is one way a caller can come to hold a capability.
//
// The built-in policy consults its sources in order and the first grant wins, so
// a deployment can add a path to cluster authority without replacing the whole
// policy. That matters because the alternative — SetAccessController — means
// reimplementing Dgraph's ACL rules in order to add one grant beside them.
//
// A source answers only "does this caller hold c". It must not deny: returning
// false means "not by this route", and the next source still gets asked.
type CapabilitySource interface {
	// Name identifies the source in logs and in the denial message, so an
	// operator can see which routes were tried.
	Name() string
	// Grants reports whether the caller holds c by this route. p is the verified
	// principal, or nil when the request carried no credential — a source keyed on
	// operator-level controls rather than identity still works in that case.
	Grants(ctx context.Context, p *x.Principal, c Capability) bool
}

// capabilitySources are consulted in order. breakGlass is first and always
// present: if cluster authority could only come from an identity provider, an
// outage or a key rotation there would lock an operator out of namespace
// lifecycle and drop-all, which is not an acceptable failure mode for a database.
var capabilitySources = []CapabilitySource{breakGlassSource{}}

// RegisterCapabilitySource appends a source, consulted after the ones already
// registered. Call it during command setup, before any listener starts serving;
// it is not safe to call concurrently with request handling.
func RegisterCapabilitySource(s CapabilitySource) {
	if s == nil {
		return
	}
	capabilitySources = append(capabilitySources, s)
}

// grantedBySource reports whether any registered source grants c, and names the
// one that did.
func grantedBySource(ctx context.Context, c Capability) (string, bool) {
	p := x.PrincipalFrom(ctx)
	for _, s := range capabilitySources {
		if s.Grants(ctx, p, c) {
			return s.Name(), true
		}
	}
	return "", false
}

// sourceNames lists the routes that were tried, for the denial message.
func sourceNames() string {
	names := make([]string, 0, len(capabilitySources))
	for _, s := range capabilitySources {
		names = append(names, s.Name())
	}
	return strings.Join(names, ", ")
}

// accessController holds the installed policy, or nil for the built-in one. An
// atomic pointer for the same reason as x.tenantResolver and x.authenticator: one
// word, read per request, written once at startup.
var accessController atomic.Pointer[AccessController]

// SetAccessController installs a deployment-specific policy, replacing the
// built-in one that authorizes against Dgraph's own ACL. Call it during command
// setup, before any listener starts serving. Passing nil restores the built-in.
func SetAccessController(ac AccessController) {
	if ac == nil {
		accessController.Store(nil)
		return
	}
	accessController.Store(&ac)
}

// currentAccessController returns the installed policy, or the built-in one when
// none is installed.
func currentAccessController() AccessController {
	if ac := accessController.Load(); ac != nil {
		return *ac
	}
	return predicateACL{}
}

// AuthorizeCapability authorizes c for the caller on ctx under the installed
// policy. This is the entry point every call site uses; the interface exists so
// what it resolves to can be replaced.
//
// NOTE: do not wrap the returned error. Prepend to the message if needed, and
// propagate the gRPC code.
func AuthorizeCapability(ctx context.Context, c Capability) error {
	return currentAccessController().AuthorizeCapability(ctx, c)
}

// AuthorizePredicates partitions preds for the caller on ctx under the installed
// policy. See AccessController.AuthorizePredicates: an error means the policy could
// not decide, which is not the same as a denial.
func AuthorizePredicates(ctx context.Context, preds []string, op *acl.Operation) (*PredResult, error) {
	return currentAccessController().AuthorizePredicates(ctx, preds, op)
}
