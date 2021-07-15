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
	"sync"

	"github.com/dgraph-io/dgraph/ee/acl"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
)

// aclCache is the cache mapping group names to the corresponding group acls
type AclCache struct {
	sync.RWMutex
	loaded        bool
	predPerms     map[string]map[string]int32
	userPredPerms map[string]map[string]int32
}

func (cache *AclCache) reset() {
	cache.Lock()
	defer cache.Unlock()
	cache.loaded = false
}

func ResetAclCache() {
	AclCachePtr.reset()
}

func (cache *AclCache) Loaded() bool {
	cache.RLock()
	defer cache.RUnlock()
	return cache.loaded
}

func (cache *AclCache) Set() {
	cache.Lock()
	defer cache.Unlock()
	cache.loaded = true
}

var AclCachePtr = &AclCache{
	loaded:        false,
	predPerms:     make(map[string]map[string]int32),
	userPredPerms: make(map[string]map[string]int32),
}

func (cache *AclCache) GetUserPredPerms(userId string) map[string]int32 {
	cache.Lock()
	defer cache.Unlock()
	return cache.userPredPerms[userId]
}

func (cache *AclCache) Update(ns uint64, groups []acl.Group) {
	// In dgraph, acl rules are divided by groups, e.g.
	// the dev group has the following blob representing its ACL rules
	// [friend, 4], [name, 7] where friend and name are predicates,
	// However in the AclCachePtr in memory, we need to change the structure and store
	// the information in two formats for efficient look-ups.
	//
	// First in which ACL rules are divided by predicates, e.g.
	// friend ->
	//     dev -> 4
	//     sre -> 6
	// name ->
	//     dev -> 7
	// the reason is that we want to efficiently determine if any ACL rule has been defined
	// for a given predicate, and allow the operation if none is defined, per the fail open
	// approach
	//
	// Second in which ACL rules are divided by users, e.g.
	// user-alice ->
	//          friend  -> 4
	//          name    -> 6
	// user-bob  ->
	//          friend  -> 7
	// the reason is so that we can efficiently determine a list of predicates (allowedPreds)
	// to which user has access for their queries

	// predPerms is the map, described above in First, that maps a single
	// predicate to a submap, and the submap maps a group to a permission

	// userPredPerms is the map, described above in Second, that maps a single
	// user to a submap, and the submap maps a predicate to a permission

	predPerms := make(map[string]map[string]int32)
	userPredPerms := make(map[string]map[string]int32)
	for _, group := range groups {
		acls := group.Rules
		users := group.Users

		for _, acl := range acls {
			if len(acl.Predicate) > 0 {
				aclPred := x.NamespaceAttr(ns, acl.Predicate)
				if groupPerms, found := predPerms[aclPred]; found {
					groupPerms[group.GroupID] = acl.Perm
				} else {
					groupPerms := make(map[string]int32)
					groupPerms[group.GroupID] = acl.Perm
					predPerms[aclPred] = groupPerms
				}
			}
		}

		for _, user := range users {
			if _, found := userPredPerms[user.UserID]; !found {
				userPredPerms[user.UserID] = make(map[string]int32)
			}
			// For each user we store all the permissions available to that user
			// via different groups. Therefore we take OR if the user already has
			// a permission for a predicate
			for _, acl := range acls {
				aclPred := x.NamespaceAttr(ns, acl.Predicate)
				if _, found := userPredPerms[user.UserID][aclPred]; found {
					userPredPerms[user.UserID][aclPred] |= acl.Perm
				} else {
					userPredPerms[user.UserID][aclPred] = acl.Perm
				}
			}
		}
	}

	AclCachePtr.Lock()
	defer AclCachePtr.Unlock()
	AclCachePtr.predPerms = predPerms
	AclCachePtr.userPredPerms = userPredPerms
}

func (cache *AclCache) AuthorizePredicate(groups []string, predicate string,
	operation *acl.Operation) error {
	if x.IsAclPredicate(x.ParseAttr(predicate)) {
		return errors.Errorf("only groot is allowed to access the ACL predicate: %s", predicate)
	}

	AclCachePtr.RLock()
	predPerms := AclCachePtr.predPerms
	AclCachePtr.RUnlock()

	if groupPerms, found := predPerms[predicate]; found {
		if hasRequiredAccess(groupPerms, groups, operation) {
			return nil
		}
	}

	// no rule has been defined that can match the predicate
	// by default we block operation
	return errors.Errorf("unauthorized to do %s on predicate %s",
		operation.Name, predicate)

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
