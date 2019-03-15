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
	"encoding/json"
	"testing"

	"github.com/dgraph-io/dgraph/ee/acl"
	"github.com/stretchr/testify/require"
)

func TestAclCache(t *testing.T) {
	aclCache = &AclCache{
		predPerms:      make(map[string]map[string]int32),
		predRegexRules: make([]*PredRegexRule, 0),
	}

	var emptyGroups []string
	group := "dev"
	predicate := "friend"
	require.NoError(t, aclCache.authorizePredicate(emptyGroups, predicate, acl.Read),
		"the anonymous user should have access when the acl cache is empty")

	acls := []acl.Acl{
		{
			Predicate: predicate,
			Perm:      4,
		},
	}
	aclBytes, _ := json.Marshal(acls)
	groups := []acl.Group{
		{
			GroupID: group,
			Acls:    string(aclBytes),
		},
	}
	aclCache.update(groups)
	// after a rule is defined, the anonymous user should no longer have access
	require.Error(t, aclCache.authorizePredicate(emptyGroups, predicate, acl.Read),
		"the anonymous user should not have access when the predicate has acl defined")
	require.NoError(t, aclCache.authorizePredicate([]string{group}, predicate, acl.Read),
		"the user with group authorized should have access")

	// update the cache with empty acl list in order to clear the cache
	aclCache.update([]acl.Group{})
	// the anonymous user should have access again
	require.NoError(t, aclCache.authorizePredicate(emptyGroups, predicate, acl.Read),
		"the anonymous user should have access when the acl cache is empty")

	// define acls using regex
	acls1 := []acl.Acl{
		{
			Regex: "^fri",
			Perm:  4,
		},
	}
	aclBytes1, _ := json.Marshal(acls1)
	groups1 := []acl.Group{
		{
			GroupID: group,
			Acls:    string(aclBytes1),
		},
	}
	aclCache.update(groups1)
	require.Error(t, aclCache.authorizePredicate(emptyGroups, predicate, acl.Read),
		"the anonymous user should not have access when the predicate has acl defined")
	require.NoError(t, aclCache.authorizePredicate([]string{group}, predicate, acl.Read),
		"the user with group authorized should have access")
}
