/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"strings"

	"github.com/pkg/errors"
)

// AnonymousPosture says what a caller with no verified identity may do.
//
// It exists because the three controls that gate Alpha's privileged operations --
// the --security whitelist, the --security token, and ACL -- each pass when their
// own feature is unconfigured. The protection an operator gets is whatever they
// configured rather than the union of the three, and the gap that leaves is
// specific: the whitelist answers where a request came from and carries no
// credential in it, so widening the range is by itself enough to make
// administrative operations reachable anonymously from anywhere inside it.
// "whitelist=0.0.0.0/0" is a common setting, and in that posture nothing is left
// to check.
//
// This type names the missing decision directly. An anonymous caller is one whose
// request produced no Principal: either it presented no credential, or the one it
// presented did not verify. See PrincipalFrom.
type AnonymousPosture int

const (
	// AnonymousFull leaves authorization exactly as the configured controls decide
	// it. An anonymous caller reaching an operation whose gate is unconfigured is
	// allowed through, which is the behavior every release before this one had.
	//
	// It is the zero value, so a WorkerOptions that was never populated behaves as
	// it did before this flag existed.
	AnonymousFull AnonymousPosture = iota

	// AnonymousData lets an anonymous caller read and write data -- queries,
	// mutations, commits, Login, and the health endpoints -- while every
	// administrative capability is denied regardless of the whitelist.
	//
	// "Administrative" means a Capability check: namespace create/drop/list, UID
	// leasing, drop-all, arming an external-snapshot import, reading cluster state,
	// and the privileged GraphQL admin surface (backup, restore, export, shutdown,
	// removeNode, moveTablet, assign, draining, config).
	AnonymousData

	// AnonymousNone additionally denies ordinary data access, leaving an anonymous
	// caller only the operations that cannot present a credential in the first
	// place: Login, CheckVersion, and the health and readiness endpoints.
	AnonymousNone
)

func (p AnonymousPosture) String() string {
	switch p {
	case AnonymousFull:
		return "full"
	case AnonymousData:
		return "data"
	case AnonymousNone:
		return "none"
	}
	return "unknown"
}

// ParseAnonymousPosture reads the --security "anonymous=..." value. An empty
// string is AnonymousFull, so an operator who never set the key gets the
// pre-existing behavior.
func ParseAnonymousPosture(s string) (AnonymousPosture, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "full":
		return AnonymousFull, nil
	case "data":
		return AnonymousData, nil
	case "none":
		return AnonymousNone, nil
	}
	return AnonymousFull, errors.Errorf(
		`--security "anonymous=%s" is not a valid value; it must be one of full, data, or none`, s)
}

// RequiresIdentityForCapability reports whether this posture denies an anonymous
// caller every administrative capability.
func (p AnonymousPosture) RequiresIdentityForCapability() bool {
	return p != AnonymousFull
}

// RequiresIdentityForData reports whether this posture denies an anonymous caller
// ordinary queries, mutations, and commits.
func (p AnonymousPosture) RequiresIdentityForData() bool {
	return p == AnonymousNone
}
