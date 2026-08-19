/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package edgraph

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/dgraph-io/dgraph/v25/acl"
	"github.com/dgraph-io/dgraph/v25/worker"
	"github.com/dgraph-io/dgraph/v25/x"
)

// predicateACL is the built-in policy: Dgraph's own ACL, and the only one an OSS
// build has.
type predicateACL struct{}

func (predicateACL) Name() string { return "predicate-acl" }

func (predicateACL) AuthorizeCapability(ctx context.Context, c Capability) error {
	switch c {
	case CapClusterAdmin:
		return authorizeClusterAdmin(ctx)
	case CapAssumeTenant:
		// Deliberately still authSuperAdmin, which grants unconditionally when ACL
		// is off. Tightening it is the fail-closed galaxy-operation change, and it
		// has a cost the cluster-admin case does not: the live and bulk loaders use
		// --force_namespace against clusters that may have no ACL, so they would
		// need a whitelisted IP or an auth token from wherever they happen to run.
		// That wants its own flag-gated release rather than riding along here.
		//
		// The other CapAssumeTenant site, parseSchemaFromAlterOperation, is reached
		// only from alter, which already required break-glass before it got here —
		// so the gap is the loader path specifically.
		return authSuperAdmin(ctx)
	case CapLeaseUIDs:
		// authSuperAdmin unchanged, which means open when ACL is off. See the constant
		// for why: `dgraph live` leases UIDs through this on every run against an
		// ACL-off cluster, and it has never needed a credential to do so.
		return authSuperAdmin(ctx)
	case CapTenantAdmin:
		// Also unchanged, and also not an oversight. Its sites are Health(all),
		// State, and the external-snapshot pair; the snapshot pair already adds
		// hasPoormansAuth of its own. Requiring break-glass for the first two would
		// newly gate /state and /health?all behind the IP whitelist, which is a
		// monitoring-visible change with a blast radius of its own. Separate
		// decision, separate change.
		return authorizeGuardians(ctx)
	}
	// Fail closed. A capability with no case here is a wiring bug, and reading an
	// unhandled capability as "granted" would make every future addition
	// unrestricted until someone noticed.
	return status.Errorf(codes.PermissionDenied,
		"%s: no rule for the %s capability", predicateACL{}.Name(), c)
}

// authorizeClusterAdmin is the one capability whose ACL-off behavior changed.
//
// With ACL enabled, this is exactly the old rule: a guardian of the root
// namespace. Break-glass is deliberately NOT consulted in that case. ACL already
// provides an identity-based route to cluster authority, and adding a
// secret-and-IP route beside it would let a caller with the --security token
// administer the cluster with no ACL token at all. That is a loosening, and no
// goal here needs it.
//
// With ACL disabled, the old rule granted cluster authority to *everyone*. That
// is the hole: CreateNamespace, DropNamespace, ListNamespaces, and AllocateIDs
// gate on this capability and nothing else, so on an ACL-off cluster they were
// reachable by any client that could open a connection. Now the same controls
// alter already requires — a whitelisted source IP, plus the --security auth
// token when one is configured — are what grant it.
//
// This is a real behavior change and the only one in this stage. It is defensible
// unflagged because it makes namespace lifecycle no stricter than schema change:
// alter has always called hasAdminAuth, so any deployment that alters a schema
// already has a working whitelist or token configuration, and the operations
// being closed are the more destructive ones. A deployment that genuinely wants
// the old behavior sets the same whitelist it already sets for alter.
func authorizeClusterAdmin(ctx context.Context) error {
	if x.WorkerConfig.AclEnabled {
		// Guardianship of the root namespace is the first route, and the only one an
		// OSS build has. If it grants, nothing else needs asking.
		aclErr := authSuperAdmin(ctx)
		if aclErr == nil {
			return nil
		}
		// Then the registered sources. Consulting them here rather than returning
		// aclErr outright is the difference between a configured
		// --external-identity cluster-admin-clients working and being silently
		// inert on any cluster that also runs ACL.
		//
		// Which is not the same as re-admitting break-glass alongside ACL: that
		// exclusion now lives in breakGlassSource.Grants, where it applies to the
		// one source it was reasoned about rather than to every source there will
		// ever be.
		if name, ok := grantedBySource(ctx, CapClusterAdmin); ok {
			glog.V(3).Infof("cluster-admin granted by %s", name)
			return nil
		}
		// The ACL error, not a generic one: its code and message text are what
		// callers and tests key on.
		return aclErr
	}
	if name, ok := grantedBySource(ctx, CapClusterAdmin); ok {
		glog.V(3).Infof("cluster-admin granted by %s", name)
		return nil
	}
	return status.Errorf(codes.PermissionDenied,
		"cluster-admin authority is required and this caller holds none. With ACL disabled it "+
			"comes from: %s. Configure --security whitelist and token, or enable --acl.",
		sourceNames())
}

// breakGlassSource grants cluster authority to a caller that satisfies the
// operator-level controls rather than an identity: a whitelisted source IP, plus
// the --security auth token when one is configured.
//
// It is the recovery path, and it is why cluster authority never depends solely on
// an external identity provider. An outage there, a rotated key, or a bad config
// would otherwise lock an operator out of namespace lifecycle and drop-all.
type breakGlassSource struct{}

func (breakGlassSource) Name() string { return "break-glass (--security whitelist/token)" }

func (breakGlassSource) Grants(ctx context.Context, _ *x.Principal, c Capability) bool {
	if c != CapClusterAdmin {
		return false
	}
	// Not a second route while ACL is on. ACL already provides an identity-based
	// path to cluster authority, and admitting a shared secret and an IP range
	// beside it would let the --security token administer the cluster with no ACL
	// token at all. Break-glass exists for the configuration that has no other
	// path, which is ACL off.
	if x.WorkerConfig.AclEnabled {
		return false
	}
	// The two halves of hasAdminAuth, called directly rather than through it: that
	// function logs every call at Info level, which is right for a rare admin RPC
	// and wrong for a capability check that may run on any request.
	if _, err := x.HasWhitelistedIP(ctx); err != nil {
		return false
	}
	return hasPoormansAuth(ctx) == nil
}

// authSuperAdmin authorizes a caller in the guardians group of the root namespace.
// It was edgraph.AuthSuperAdmin, called directly from eight places; it is now
// reached only through AuthorizeCapability.
//
// NOTE: the caller should not wrap the error returned. If needed, propagate the
// GRPC error code.
func authSuperAdmin(ctx context.Context) error {
	if !x.WorkerConfig.AclEnabled {
		return nil
	}
	// The signed claim, deliberately — not requestNamespace.
	//
	// This test asks whether the CALLER is rooted in the root namespace, which is a
	// property of their credential, not of where the request is being routed. The
	// rekeying that moved authorizePreds onto the resolved namespace was right for
	// that site — it keys ACL lookups on the same channel storage keys on — and
	// wrong here, because requestNamespace prefers x.ExtractNamespace, and that
	// reads md["namespace"], which is client-controlled on the server side.
	//
	// The four RPCs gated on CapClusterAdmin and nothing else — CreateNamespace,
	// DropNamespace, ListNamespaces, AllocateIDs — call AuthorizeCapability as their
	// first statement, with no ResolveTenant before it to overwrite that metadata.
	// So reading it there let a guardian of any tenant claim namespace 0 and
	// administer the cluster, and let hostile metadata demote a real cluster admin.
	ns, err := x.ExtractNamespaceFrom(ctx)
	if err != nil {
		// Unauthenticated, not a bare wrapped error. Every caller propagates
		// status.Convert(err).Code(), and an unwrapped error converts to Unknown — so a
		// missing or expired token on CreateNamespace, DropNamespace, ListNamespaces or
		// the GraphQL admin surface looked like a server fault rather than a credential
		// problem, and a client could not tell "log in again" from "retry later".
		// authorizeGuardians below already classifies the same failure this way.
		return status.Error(codes.Unauthenticated,
			errors.Wrap(err, "Authorize guardian of the galaxy, extracting jwt token, error:").Error())
	}
	if ns != 0 {
		return status.Error(
			codes.PermissionDenied, "Only superadmin is allowed to do this operation")
	}
	// authorizeGuardians will extract (user, []groups) from the JWT claims and will check if
	// any of the group to which the user belongs is "guardians" or not.
	if err := authorizeGuardians(ctx); err != nil {
		s := status.Convert(err)
		return status.Error(
			s.Code(), "AuthSuperAdmin: failed to authorize guardians. "+s.Message())
	}
	glog.V(3).Info("Successfully authorised guardian of the galaxy")
	return nil
}

// authorizeGuardians authorizes a caller in the guardians group of any namespace.
// It was edgraph.AuthorizeGuardians.
//
// NOTE: the caller should not wrap the error returned. If needed, propagate the
// GRPC error code.
func authorizeGuardians(ctx context.Context) error {
	if worker.Config.AclSecretKey == nil {
		// the user has not turned on the acl feature
		return nil
	}

	userData, err := extractUserAndGroups(ctx)
	switch {
	case errors.Is(err, x.ErrNoJwt):
		return status.Error(codes.PermissionDenied, err.Error())
	case err != nil:
		return status.Error(codes.Unauthenticated, err.Error())
	default:
		userId := userData.userId
		groupIds := userData.groupIds

		if !x.IsSuperAdmin(groupIds) {
			// Deny access for members of non-guardian groups
			return status.Error(codes.PermissionDenied, fmt.Sprintf("Only guardians are "+
				"allowed access. User '%v' is not a member of guardians group.", userId))
		}
	}

	return nil
}

// AuthorizePredicates is the built-in policy's predicate half: Dgraph's ACL
// rules, read from worker.AclCachePtr.
//
// It derives identity from ctx rather than taking it as a parameter, per the
// interface. The extra extractUserAndGroups is nearly free now that it reads the
// Principal the identity interceptor already resolved, and it keeps the signature
// free of ACL's own types.
func (predicateACL) AuthorizePredicates(ctx context.Context, preds []string,
	op *acl.Operation) (*PredResult, error) {

	userData, err := extractUserAndGroups(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}
	return authorizePreds(ctx, userData, preds, op), nil
}

func authorizePreds(ctx context.Context, userData *userData, preds []string,
	aclOp *acl.Operation) *PredResult {

	if !worker.AclCachePtr.Loaded() {
		RefreshACLs(ctx)
	}

	userId := userData.userId
	groupIds := userData.groupIds

	// Key the ACL lookup on the namespace the storage layer will actually use.
	//
	// This used to read userData.namespace — the tenant claim inside the access
	// token — while every read and write below keys on x.ExtractNamespace(ctx),
	// the resolved tenancy. The two agree today only because the tenant resolver
	// copies the claim into the context; they are separate channels, and
	// authorizing against one while executing against the other is a
	// confused-deputy bug waiting for the day a resolver derives the tenant from
	// something other than that claim.
	//
	// Deliberately not a hard failure. If the context carries no resolved
	// tenancy, fall back to the claim, which is what this read unconditionally
	// before; a divergence is logged rather than rejected, so an unexpected one
	// becomes visible without turning into an outage.
	ns := namespaceOrClaim(ctx, userData.namespace, "authorizePreds")

	blockedPreds := make(map[string]struct{})
	for _, pred := range preds {
		nsPred := x.NamespaceAttr(ns, pred)
		if err := worker.AclCachePtr.AuthorizePredicate(groupIds, nsPred, aclOp); err != nil {
			logAccess(&accessEntry{
				userId:    userId,
				groups:    groupIds,
				preds:     preds,
				operation: aclOp,
				allowed:   false,
			})
			blockedPreds[pred] = struct{}{}
		}
	}
	if worker.HasAccessToAllPreds(ns, groupIds, aclOp) {
		// Setting allowed to nil allows access to all predicates. Note that the access to ACL
		// predicates will still be blocked.
		return &PredResult{Allowed: nil, Blocked: blockedPreds}
	}
	// User can have multiple permission for same predicate, add predicate
	allowedPreds := make([]string, 0, len(worker.AclCachePtr.GetUserPredPerms(userId)))
	// only if the acl.Op is covered in the set of permissions for the user
	for predicate, perm := range worker.AclCachePtr.GetUserPredPerms(userId) {
		if (perm & aclOp.Code) > 0 {
			allowedPreds = append(allowedPreds, predicate)
		}
	}
	return &PredResult{Allowed: allowedPreds, Blocked: blockedPreds}
}
