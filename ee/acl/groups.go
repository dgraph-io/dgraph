// +build !oss

/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package acl

import (
	"context"
	"fmt"
	"strings"

	"github.com/dgraph-io/dgo"
)

func queryGroup(ctx context.Context, txn *dgo.Txn, groupid string,
	fields ...string) (group *Group, err error) {

	// write query header
	query := fmt.Sprintf(`query search($groupid: string){
        group(func: eq(dgraph.xid, $groupid)) {
			uid
		    %s }}`, strings.Join(fields, ", "))

	queryVars := map[string]string{
		"$groupid": groupid,
	}

	queryResp, err := txn.QueryWithVars(ctx, query, queryVars)
	if err != nil {
		fmt.Printf("Error while querying group with id %s: %v\n", groupid, err)
		return nil, err
	}
	group, err = UnmarshalGroup(queryResp.GetJson(), "group")
	if err != nil {
		return nil, err
	}
	return group, nil
}

func isSameAcl(acl1 *Acl, acl2 *Acl) bool {
	return (len(acl1.Predicate) > 0 && len(acl2.Predicate) > 0 &&
		acl1.Predicate == acl2.Predicate) ||
		(len(acl1.Regex) > 0 && len(acl2.Regex) > 0 && acl1.Regex == acl2.Regex)
}

// returns whether the existing acls slice is changed
func updateAcl(acls []Acl, newAcl Acl) ([]Acl, bool) {
	for idx, aclEntry := range acls {
		if isSameAcl(&aclEntry, &newAcl) {
			if aclEntry.Perm == newAcl.Perm {
				// new permission is the same as the current one, no update
				return acls, false
			}
			if newAcl.Perm < 0 {
				// remove the current aclEntry from the array
				copy(acls[idx:], acls[idx+1:])
				return acls[:len(acls)-1], true
			}
			acls[idx].Perm = newAcl.Perm
			return acls, true
		}
	}

	// we do not find any existing aclEntry matching the newAcl predicate
	return append(acls, newAcl), true
}
