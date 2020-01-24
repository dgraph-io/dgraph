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

package worker

import (
	"encoding/json"
	"sync"

	"github.com/dgraph-io/dgraph/ee/acl"
	"github.com/golang/glog"
)

// aclCache is the cache mapping group names to the corresponding group acls
type aclCache struct {
	sync.RWMutex
	predPerms map[string]map[string]int32
}

var AclCachePtr = &aclCache{
	predPerms: make(map[string]map[string]int32),
}

func (cache *aclCache) Update(groups []acl.Group) {
	// In dgraph, acl rules are divided by groups, e.g.
	// the dev group has the following blob representing its ACL rules
	// [friend, 4], [name, 7] where friend and name are predicates,
	// However in the aclCachePtr in memory, we need to change the structure so
	// that ACL rules are divided by predicates, e.g.
	// friend ->
	//     dev -> 4
	//     sre -> 6
	// name ->
	//     dev -> 7
	// the reason is that we want to efficiently determine if any ACL rule has been defined
	// for a given predicate, and allow the operation if none is defined, per the fail open
	// approach

	// predPerms is the map descriebed above that maps a single
	// predicate to a submap, and the submap maps a group to a permission
	predPerms := make(map[string]map[string]int32)
	for _, group := range groups {
		aclBytes := []byte(group.Acls)
		var acls []acl.Acl
		if err := json.Unmarshal(aclBytes, &acls); err != nil {
			glog.Errorf("Unable to unmarshal the aclBytes: %v", err)
			continue
		}

		for _, acl := range acls {
			if len(acl.Predicate) > 0 {
				if groupPerms, found := predPerms[acl.Predicate]; found {
					groupPerms[group.GroupID] = acl.Perm
				} else {
					groupPerms := make(map[string]int32)
					groupPerms[group.GroupID] = acl.Perm
					predPerms[acl.Predicate] = groupPerms
				}
			}
		}
	}

	AclCachePtr.Lock()
	defer AclCachePtr.Unlock()
	AclCachePtr.predPerms = predPerms
}

// hasRequiredAccess checks if any group in the passed in groups is allowed to perform the operation
// according to the acl rules stored in groupPerms
func hasRequiredAccess(groupPerms map[string]int32, groups []string,
	operation *acl.Operation) bool {
	for _, group := range groups {
		groupPerm, found := groupPerms[group]
		if found && (groupPerm&operation.Code != 0) {
			return true
		}
	}
	return false
}
