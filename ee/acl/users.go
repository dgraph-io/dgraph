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

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
)

func queryUser(ctx context.Context, txn *dgo.Txn, userid string) (user *User, err error) {
	query := `
    query search($userid: string){
      user(func: eq(dgraph.xid, $userid)) {
	    uid
        dgraph.xid
        dgraph.user.group {
          uid
          dgraph.xid
        }
      }
    }`

	queryVars := make(map[string]string)
	queryVars["$userid"] = userid

	queryResp, err := txn.QueryWithVars(ctx, query, queryVars)
	if err != nil {
		return nil, fmt.Errorf("error while query user with id %s: %v", userid, err)
	}
	user, err = UnmarshalUser(queryResp, "user")
	if err != nil {
		return nil, err
	}
	return user, nil
}

func getUserModNQuad(ctx context.Context, txn *dgo.Txn, userId string,
	groupId string) (*api.NQuad, error) {
	group, err := queryGroup(ctx, txn, groupId)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, fmt.Errorf("the group does not exist:%v", groupId)
	}

	createUserGroupNQuads := &api.NQuad{
		Subject:   userId,
		Predicate: "dgraph.user.group",
		ObjectId:  group.Uid,
	}

	return createUserGroupNQuads, nil
}
