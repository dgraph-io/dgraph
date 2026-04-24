/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package admin

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/graphql/resolve"
)

// mwPointer returns the function pointer for a middleware so it can be compared
// by identity rather than value (Go prohibits direct function equality checks).
func mwPointer(mw resolve.MutationMiddleware) uintptr {
	return reflect.ValueOf(mw).Pointer()
}

func containsMW(mws resolve.MutationMiddlewares, target resolve.MutationMiddleware) bool {
	want := mwPointer(target)
	for _, mw := range mws {
		if mwPointer(mw) == want {
			return true
		}
	}
	return false
}

// TestAdminMutationMiddlewareConfig asserts the security posture for every
// registered admin mutation.
//
// Each mutation must be present in adminMutationMWConfig. An absent entry
// causes the middleware chain to be empty, bypassing all authentication,
// IP whitelisting, and audit logging (see resolve/middlewares.go: Then()).
func TestAdminMutationMiddlewareConfig(t *testing.T) {
	type securityRequirements struct {
		// desc is shown in failure messages to explain why this mutation needs these middlewares.
		desc string
		// ipWhitelist: must include IpWhitelistingMW4Mutation
		ipWhitelist bool
		// superAdminAuth: must include GuardianOfTheGalaxyAuthMW4Mutation
		superAdminAuth bool
		// guardianAuth: must include GuardianAuthMW4Mutation
		guardianAuth bool
		// aclOnly: must include AclOnlyMW4Mutation
		aclOnly bool
	}

	tests := map[string]securityRequirements{
		// Superadmin (Guardian-of-Galaxy) auth — highest privilege operations.
		"backup":   {desc: "database backups", ipWhitelist: true, superAdminAuth: true},
		"config":   {desc: "cluster config changes", ipWhitelist: true, superAdminAuth: true},
		"draining": {desc: "draining mode", ipWhitelist: true, superAdminAuth: true},
		"restore":  {desc: "backup restore", ipWhitelist: true, superAdminAuth: true},
		"restoreTenant": { // CVE: previously absent from this map (CVSS 10.0)
			desc:           "cross-namespace backup restore — accepts attacker-controlled URLs",
			ipWhitelist:    true,
			superAdminAuth: true,
		},
		"shutdown":   {desc: "node shutdown", ipWhitelist: true, superAdminAuth: true},
		"removeNode": {desc: "cluster topology change", ipWhitelist: true, superAdminAuth: true},
		"moveTablet": {desc: "tablet relocation", ipWhitelist: true, superAdminAuth: true},
		"assign":     {desc: "UID/timestamp assignment", ipWhitelist: true, superAdminAuth: true},

		// Superadmin + ACL — namespace lifecycle mutations.
		"addNamespace":    {desc: "namespace creation", ipWhitelist: true, superAdminAuth: true, aclOnly: true},
		"deleteNamespace": {desc: "namespace deletion", ipWhitelist: true, superAdminAuth: true, aclOnly: true},
		"resetPassword":   {desc: "password reset", ipWhitelist: true, superAdminAuth: true, aclOnly: true},

		// Guardian auth — standard admin operations.
		"export":          {desc: "data export", ipWhitelist: true, guardianAuth: true},
		"updateGQLSchema": {desc: "GraphQL schema update", ipWhitelist: true, guardianAuth: true},

		// Minimal (IP whitelist + logging only) — dgraph handles auth internally for these.
		"login":       {desc: "login (auth handled internally)", ipWhitelist: true},
		"addUser":     {desc: "user management (dgraph handles guardian auth)", ipWhitelist: true},
		"addGroup":    {desc: "group management (dgraph handles guardian auth)", ipWhitelist: true},
		"updateUser":  {desc: "user management (dgraph handles guardian auth)", ipWhitelist: true},
		"updateGroup": {desc: "group management (dgraph handles guardian auth)", ipWhitelist: true},
		"deleteUser":  {desc: "user management (dgraph handles guardian auth)", ipWhitelist: true},
		"deleteGroup": {desc: "group management (dgraph handles guardian auth)", ipWhitelist: true},
	}

	for mutation, req := range tests {
		t.Run(mutation, func(t *testing.T) {
			mws, ok := adminMutationMWConfig[mutation]
			require.Truef(t, ok,
				"mutation %q (%s) is missing from adminMutationMWConfig — "+
					"absent entries bypass ALL authentication, IP whitelisting, and audit logging",
				mutation, req.desc)

			if req.ipWhitelist {
				assert.Truef(t, containsMW(mws, resolve.IpWhitelistingMW4Mutation),
					"mutation %q (%s) must include IpWhitelistingMW4Mutation", mutation, req.desc)
			}
			if req.superAdminAuth {
				assert.Truef(t, containsMW(mws, resolve.GuardianOfTheGalaxyAuthMW4Mutation),
					"mutation %q (%s) must include GuardianOfTheGalaxyAuthMW4Mutation", mutation, req.desc)
			}
			if req.guardianAuth {
				assert.Truef(t, containsMW(mws, resolve.GuardianAuthMW4Mutation),
					"mutation %q (%s) must include GuardianAuthMW4Mutation", mutation, req.desc)
			}
			if req.aclOnly {
				assert.Truef(t, containsMW(mws, resolve.AclOnlyMW4Mutation),
					"mutation %q (%s) must include AclOnlyMW4Mutation", mutation, req.desc)
			}
		})
	}
}
