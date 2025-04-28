/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hypermodeinc/dgraph/v25/acl"
	"github.com/hypermodeinc/dgraph/v25/x"
)

func TestAclCache(t *testing.T) {
	AclCachePtr = &AclCache{
		predPerms: make(map[string]map[string]int32),
	}

	var emptyGroups []string
	group := "dev"
	predicate := x.AttrInRootNamespace("friend")
	require.Error(t, AclCachePtr.AuthorizePredicate(emptyGroups, predicate, acl.Read),
		"the anonymous user should not have access when the acl cache is empty")

	acls := []acl.Acl{
		{
			// update operation on acl cache needs predicate without namespace.
			Predicate: x.ParseAttr(predicate),
			Perm:      4,
		},
	}
	groups := []acl.Group{
		{
			GroupID: group,
			Rules:   acls,
		},
	}
	AclCachePtr.Update(x.RootNamespace, groups)
	// after a rule is defined, the anonymous user should no longer have access
	require.Error(t, AclCachePtr.AuthorizePredicate(emptyGroups, predicate, acl.Read),
		"the anonymous user should not have access when the predicate has acl defined")
	require.NoError(t, AclCachePtr.AuthorizePredicate([]string{group}, predicate, acl.Read),
		"the user with group authorized should have access")

	// update the cache with empty acl list in order to clear the cache
	AclCachePtr.Update(x.RootNamespace, []acl.Group{})
	// the anonymous user should have access again
	require.Error(t, AclCachePtr.AuthorizePredicate(emptyGroups, predicate, acl.Read),
		"the anonymous user should not have access when the acl cache is empty")
}
