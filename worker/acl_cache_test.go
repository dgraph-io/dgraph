/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/acl"
	"github.com/dgraph-io/dgraph/v25/x"
)

func resetAclCacheForTest(t *testing.T) {
	t.Helper()
	original := AclCachePtr
	AclCachePtr = &AclCache{
		predPerms:     make(map[string]map[string]int32),
		userPredPerms: make(map[string]map[string]int32),
	}
	t.Cleanup(func() {
		AclCachePtr = original
	})
}

func TestAclCache(t *testing.T) {
	resetAclCacheForTest(t)

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

func TestAclCacheMergesSameUserAcrossNamespaces(t *testing.T) {
	const (
		userID = "shared-user"
		nsOne  = uint64(1)
		nsTwo  = uint64(2)
	)

	groups := func(groupID, predicate string, permission int32) []acl.Group {
		return []acl.Group{{
			GroupID: groupID,
			Users:   []acl.User{{UserID: userID}},
			Rules:   []acl.Acl{{Predicate: predicate, Perm: permission}},
		}}
	}

	for _, tc := range []struct {
		name  string
		order []uint64
	}{
		{name: "namespace one then two", order: []uint64{nsOne, nsTwo}},
		{name: "namespace two then one", order: []uint64{nsTwo, nsOne}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetAclCacheForTest(t)

			for _, ns := range tc.order {
				switch ns {
				case nsOne:
					AclCachePtr.Update(nsOne, groups("group-one", "pred-one", acl.Read.Code))
				case nsTwo:
					AclCachePtr.Update(nsTwo, groups("group-two", "pred-two", acl.Write.Code))
				}
			}

			require.Equal(t, map[string]int32{
				x.NamespaceAttr(nsOne, "pred-one"): acl.Read.Code,
				x.NamespaceAttr(nsTwo, "pred-two"): acl.Write.Code,
			}, AclCachePtr.GetUserPredPerms(userID))

			AclCachePtr.Update(nsOne,
				groups("group-one", "pred-one-new", acl.Modify.Code))
			require.Equal(t, map[string]int32{
				x.NamespaceAttr(nsOne, "pred-one-new"): acl.Modify.Code,
				x.NamespaceAttr(nsTwo, "pred-two"):     acl.Write.Code,
			}, AclCachePtr.GetUserPredPerms(userID),
				"refreshing one namespace should replace only that namespace's permissions")

			AclCachePtr.Update(nsOne, nil)
			require.Equal(t, map[string]int32{
				x.NamespaceAttr(nsTwo, "pred-two"): acl.Write.Code,
			}, AclCachePtr.GetUserPredPerms(userID),
				"clearing one namespace should preserve permissions from other namespaces")

			AclCachePtr.Update(nsTwo, nil)
			require.NotContains(t, AclCachePtr.userPredPerms, userID,
				"clearing the last namespace should remove the empty user entry")
		})
	}
}

func TestGetUserPredPermsReturnsSnapshot(t *testing.T) {
	resetAclCacheForTest(t)

	const (
		userID = "alice"
		ns     = uint64(1)
	)
	groups := func(predicate string, permission int32) []acl.Group {
		return []acl.Group{{
			GroupID: "dev",
			Users:   []acl.User{{UserID: userID}},
			Rules:   []acl.Acl{{Predicate: predicate, Perm: permission}},
		}}
	}

	oldPredicate := x.NamespaceAttr(ns, "old-predicate")
	AclCachePtr.Update(ns, groups("old-predicate", acl.Read.Code))
	snapshot := AclCachePtr.GetUserPredPerms(userID)

	AclCachePtr.Update(ns, groups("new-predicate", acl.Write.Code))
	require.Equal(t, map[string]int32{oldPredicate: acl.Read.Code}, snapshot,
		"updating the cache should not mutate a previously returned snapshot")

	snapshot[x.NamespaceAttr(ns, "caller-only")] = acl.Modify.Code
	require.NotContains(t, AclCachePtr.GetUserPredPerms(userID),
		x.NamespaceAttr(ns, "caller-only"),
		"mutating a snapshot should not mutate the cache")
}
