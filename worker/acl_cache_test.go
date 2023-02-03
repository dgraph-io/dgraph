//go:build !oss
// +build !oss

/*
 * Copyright 2022 Dgraph Labs, Inc. All rights reserved.
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package worker

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/ee/acl"
	"github.com/dgraph-io/dgraph/x"
)

func TestAclCache(t *testing.T) {
	AclCachePtr = &AclCache{
		predPerms: make(map[string]map[string]int32),
	}

	var emptyGroups []string
	group := "dev"
	predicate := x.GalaxyAttr("friend")
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
	AclCachePtr.Update(x.GalaxyNamespace, groups)
	// after a rule is defined, the anonymous user should no longer have access
	require.Error(t, AclCachePtr.AuthorizePredicate(emptyGroups, predicate, acl.Read),
		"the anonymous user should not have access when the predicate has acl defined")
	require.NoError(t, AclCachePtr.AuthorizePredicate([]string{group}, predicate, acl.Read),
		"the user with group authorized should have access")

	// update the cache with empty acl list in order to clear the cache
	AclCachePtr.Update(x.GalaxyNamespace, []acl.Group{})
	// the anonymous user should have access again
	require.Error(t, AclCachePtr.AuthorizePredicate(emptyGroups, predicate, acl.Read),
		"the anonymous user should not have access when the acl cache is empty")
}
