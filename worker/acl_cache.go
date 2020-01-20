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
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/dgraph/ee/acl"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgrijalva/jwt-go"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"google.golang.org/grpc/metadata"
)

var ErrNoJwt = errors.New("no accessJwt available")

// aclCache is the cache mapping group names to the corresponding group acls
type aclCache struct {
	sync.RWMutex
	predPerms map[string]map[string]int32
}

var aclCachePtr = &aclCache{
	predPerms: make(map[string]map[string]int32),
}

func Update(groups []acl.Group) {
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

	aclCachePtr.Lock()
	defer aclCachePtr.Unlock()
	aclCachePtr.predPerms = predPerms
}

func AuthorizePredicate(groups []string, predicate string,
	operation *acl.Operation) error {
	if x.IsAclPredicate(predicate) {
		return errors.Errorf("only groot is allowed to access the ACL predicate: %s", predicate)
	}

	aclCachePtr.RLock()
	predPerms := aclCachePtr.predPerms
	aclCachePtr.RUnlock()

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

// extract the userId, groupIds from the accessJwt in the context
func ExtractUserAndGroups(ctx context.Context) ([]string, error) {
	// extract the jwt and unmarshal the jwt to get the list of groups
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, ErrNoJwt
	}
	accessJwt := md.Get("accessJwt")
	if len(accessJwt) == 0 {
		return nil, ErrNoJwt
	}

	return ValidateToken(accessJwt[0])
}

func AuthorizePreds(userId string, groupIds, preds []string,
	aclOp *acl.Operation) map[string]struct{} {

	blockedPreds := make(map[string]struct{})
	for _, pred := range preds {
		if err := AuthorizePredicate(groupIds, pred, aclOp); err != nil {
			logAccess(&AccessEntry{
				userId:    userId,
				groups:    groupIds,
				preds:     preds,
				operation: aclOp,
				allowed:   false,
			})

			blockedPreds[pred] = struct{}{}
		}
	}
	return blockedPreds
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

// validateToken verifies the signature and expiration of the jwt, and if validation passes,
// returns a slice of strings, where the first element is the extracted userId
// and the rest are groupIds encoded in the jwt.
func ValidateToken(jwtStr string) ([]string, error) {
	token, err := jwt.Parse(jwtStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return Config.HmacSecret, nil
	})

	if err != nil {
		return nil, errors.Errorf("unable to parse jwt token:%v", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, errors.Errorf("claims in jwt token is not map claims")
	}

	// by default, the MapClaims.Valid will return true if the exp field is not set
	// here we enforce the checking to make sure that the refresh token has not expired
	now := time.Now().Unix()
	if !claims.VerifyExpiresAt(now, true) {
		return nil, errors.Errorf("Token is expired") // the same error msg that's used inside jwt-go
	}

	userId, ok := claims["userid"].(string)
	if !ok {
		return nil, errors.Errorf("userid in claims is not a string:%v", userId)
	}

	groups, ok := claims["groups"].([]interface{})
	var groupIds []string
	if ok {
		groupIds = make([]string, 0, len(groups))
		for _, group := range groups {
			groupId, ok := group.(string)
			if !ok {
				// This shouldn't happen. So, no need to make the client try to refresh the tokens.
				return nil, errors.Errorf("unable to convert group to string:%v", group)
			}

			groupIds = append(groupIds, groupId)
		}
	}
	return append([]string{userId}, groupIds...), nil
}

type AccessEntry struct {
	userId    string
	groups    []string
	preds     []string
	operation *acl.Operation
	allowed   bool
}

func NewAccessEntry(userId string, groups, preds []string, op *acl.Operation, allowed bool) *AccessEntry {
	return &AccessEntry{
		userId:    userId,
		groups:    groups,
		preds:     preds,
		operation: op,
		allowed:   allowed,
	}
}

func (log *AccessEntry) String() string {
	return fmt.Sprintf("ACL-LOG Authorizing user %q with groups %q on predicates %q "+
		"for %q, allowed:%v", log.userId, strings.Join(log.groups, ","),
		strings.Join(log.preds, ","), log.operation.Name, log.allowed)
}

func logAccess(log *AccessEntry) {
	glog.V(1).Infof(log.String())
}
