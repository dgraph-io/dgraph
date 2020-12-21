// +build !oss

/*
 * Copyright 2018 Dgraph Labs, Inc. All rights reserved.
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package edgraph

import (
	"testing"

	"github.com/dgraph-io/dgraph/ee/acl"
	"github.com/stretchr/testify/require"
)

func TestAclCache(t *testing.T) {
	aclCachePtr = &aclCache{
		predPerms: make(map[string]map[string]int32),
	}

	var emptyGroups []string
	group := "dev"
	predicate := "friend"
	require.Error(t, aclCachePtr.authorizePredicate(emptyGroups, predicate, acl.Read),
		"the anonymous user should not have access when the acl cache is empty")

	acls := []acl.Acl{
		{
			Predicate: predicate,
			Perm:      4,
		},
	}
	groups := []acl.Group{
		{
			GroupID: group,
			Rules:   acls,
		},
	}
	aclCachePtr.update(groups)
	// after a rule is defined, the anonymous user should no longer have access
	require.Error(t, aclCachePtr.authorizePredicate(emptyGroups, predicate, acl.Read),
		"the anonymous user should not have access when the predicate has acl defined")
	require.NoError(t, aclCachePtr.authorizePredicate([]string{group}, predicate, acl.Read),
		"the user with group authorized should have access")

	// update the cache with empty acl list in order to clear the cache
	aclCachePtr.update([]acl.Group{})
	// the anonymous user should have access again
	require.Error(t, aclCachePtr.authorizePredicate(emptyGroups, predicate, acl.Read),
		"the anonymous user should not have access when the acl cache is empty")
}
