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
	"context"
	"fmt"
	"strings"

	"github.com/dgraph-io/dgraph/ee/acl"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgrijalva/jwt-go"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	"google.golang.org/grpc/metadata"
)

func (cache *aclCache) authorizePredicate(groups []string, predicate string,
	operation *acl.Operation) error {
	if x.IsAclPredicate(predicate) {
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

func AuthorizePreds(userId string, groupIds, preds []string,
	aclOp *acl.Operation) map[string]struct{} {

	blockedPreds := make(map[string]struct{})
	for _, pred := range preds {
		if err := AclCachePtr.authorizePredicate(groupIds, pred, aclOp); err != nil {
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

type AccessEntry struct {
	userId    string
	groups    []string
	preds     []string
	operation *acl.Operation
	allowed   bool
}

func NewAccessEntry(userId string, groups, preds []string,
	operation *acl.Operation, allowed bool) *AccessEntry {

	return &AccessEntry{
		userId:    userId,
		groups:    groups,
		preds:     preds,
		operation: operation,
		allowed:   allowed,
	}
}

func logAccess(log *AccessEntry) {
	if glog.V(1) {
		glog.Info(log.String())
	}
}

func (log *AccessEntry) String() string {
	return fmt.Sprintf("ACL-LOG Authorizing user %q with groups %q on predicates %q "+
		"for %q, allowed:%v", log.userId, strings.Join(log.groups, ","),
		strings.Join(log.preds, ","), log.operation.Name, log.allowed)
}

func FilterUnauthorizedPreds(ctx context.Context, preds []string) []string {
	// extract the jwt and unmarshal the jwt to get the list of groups
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return preds
	}
	accessJwt := md.Get("accessJwt")
	if len(accessJwt) == 0 {
		return preds
	}
	tokens, _, err := new(jwt.Parser).ParseUnverified(accessJwt[0], jwt.MapClaims{})
	if err != nil {
		return preds
	}
	claims, ok := tokens.Claims.(jwt.MapClaims)
	if !ok {
		return preds
	}
	userId, ok := claims["userid"].(string)
	if !ok {
		return preds
	}
	groups, ok := claims["groups"].([]interface{})
	if !ok {
		return preds
	}
	groupIds := make([]string, 0, len(groups))
	for _, group := range groups {
		groupId, ok := group.(string)
		if !ok {
			// This shouldn't happen. So, no need to make the client try to refresh the tokens.
			return preds
		}

		groupIds = append(groupIds, groupId)
	}
	if x.IsGuardian(groupIds) {
		return preds
	}
	blockedPreds := AuthorizePreds(userId, groupIds, preds, acl.Read)
	filteredPreds := preds[:0]
	for _, pred := range preds {
		if _, ok := blockedPreds[pred]; !ok {
			filteredPreds = append(filteredPreds, pred)
		}
	}
	return filteredPreds
}
