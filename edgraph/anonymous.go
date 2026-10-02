/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package edgraph

import (
	"context"

	"github.com/golang/glog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/dgraph-io/dgraph/v25/acl"
	"github.com/dgraph-io/dgraph/v25/x"
)

// anonymousPosture wraps an AccessController so that a caller with no verified
// identity is denied every capability, whatever the wrapped policy would have
// said.
//
// It wraps rather than replaces because the posture is a floor, not a policy. The
// question "has this caller been identified at all" is prior to and independent of
// "what may this identity do", and answering the first one in the capability rules
// themselves would mean every policy -- the built-in one, and any a deployment
// installs -- reimplementing the same check and getting it consistent.
//
// Predicate authorization is passed straight through. A query naming predicates
// the caller may not read is answered with the rest dropped rather than refused
// (see AccessController.AuthorizePredicates), so there is no "deny" to express
// here. Whether an anonymous caller may run the query at all is decided earlier,
// by RequireIdentifiedCaller.
type anonymousPosture struct {
	inner   AccessController
	posture x.AnonymousPosture
}

func (a anonymousPosture) Name() string {
	return a.inner.Name() + " (anonymous=" + a.posture.String() + ")"
}

func (a anonymousPosture) AuthorizeCapability(ctx context.Context, c Capability) error {
	if a.posture.RequiresIdentityForCapability() && x.PrincipalFrom(ctx) == nil {
		return status.Errorf(codes.Unauthenticated,
			`the %s capability requires an identified caller, and this request presented no `+
				`credential that verified. This cluster runs with --security "anonymous=%s". `+
				`Present the --security auth token, or log in with ACL.`,
			c, a.posture)
	}
	return a.inner.AuthorizeCapability(ctx, c)
}

func (a anonymousPosture) AuthorizePredicates(ctx context.Context, preds []string,
	op *acl.Operation) (*PredResult, error) {
	return a.inner.AuthorizePredicates(ctx, preds, op)
}

// EnforceAnonymousPosture installs the --security "anonymous=..." floor over
// whatever policy is currently installed. AnonymousFull installs nothing, so a
// cluster running the shipped default carries no extra indirection at all.
//
// Call it during command setup, after any deployment-specific SetAccessController
// and before any listener starts serving: it captures the policy installed at the
// moment it runs, so installing one afterwards would silently discard the floor.
func EnforceAnonymousPosture(posture x.AnonymousPosture) {
	if !posture.RequiresIdentityForCapability() {
		return
	}
	inner := currentAccessController()
	SetAccessController(anonymousPosture{inner: inner, posture: posture})
	glog.Infof(`--security "anonymous=%s": capabilities now require an identified caller `+
		`(policy: %s)`, posture, inner.Name())
}

// RequireIdentifiedCaller rejects an anonymous caller's ordinary data access under
// --security "anonymous=none".
//
// Separate from AuthorizeCapability because queries, mutations, and commits are
// not capabilities and should not become them: a capability is cluster or tenant
// authority, deliberately coarse and deliberately few, while data access is
// decided per predicate. This is the one question the posture asks that predicate
// authorization cannot, because an anonymous caller on an ACL-off cluster has
// blanket predicate access by construction.
//
// It is a no-op under anonymous=full and anonymous=data, which is why the call
// sites can be unconditional.
func RequireIdentifiedCaller(ctx context.Context, op string) error {
	if !x.WorkerConfig.Anonymous.RequiresIdentityForData() {
		return nil
	}
	if x.PrincipalFrom(ctx) != nil {
		return nil
	}
	return status.Errorf(codes.Unauthenticated,
		`%s requires an identified caller, and this request presented no credential that `+
			`verified. This cluster runs with --security "anonymous=none". Present the `+
			`--security auth token, or log in with ACL.`, op)
}
