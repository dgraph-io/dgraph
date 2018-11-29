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
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/ee/acl"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
)

func userAdd(dc *dgo.Dgraph) error {
	userid := UserAdd.Conf.GetString("user")
	password := UserAdd.Conf.GetString("password")

	if len(userid) == 0 {
		return fmt.Errorf("The user must not be empty.")
	}
	if len(password) == 0 {
		return fmt.Errorf("The password must not be empty.")
	}

	ctx := context.Background()
	txn := dc.NewTxn()
	defer txn.Discard(ctx)

	user, err := queryUser(ctx, txn, userid)
	if err != nil {
		return err
	}

	if user != nil {
		return fmt.Errorf("Unable to create user because of conflict: %v", userid)
	}

	createUserNQuads := []*api.NQuad{
		{
			Subject:     "_:newuser",
			Predicate:   "dgraph.xid",
			ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: userid}},
		},
		{
			Subject:     "_:newuser",
			Predicate:   "dgraph.password",
			ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: password}},
		}}

	mu := &api.Mutation{
		CommitNow: true,
		Set:       createUserNQuads,
	}

	_, err = txn.Mutate(ctx, mu)
	if err != nil {
		return fmt.Errorf("Unable to create user: %v", err)
	}

	glog.Infof("Created new user with id %v", userid)
	return nil
}

func userDel(dc *dgo.Dgraph) error {
	userid := UserDel.Conf.GetString("user")
	// validate the userid
	if len(userid) == 0 {
		return fmt.Errorf("The user id should not be empty")
	}

	ctx := context.Background()
	txn := dc.NewTxn()
	defer txn.Discard(ctx)

	user, err := queryUser(ctx, txn, userid)
	if err != nil {
		return err
	}

	if user == nil || len(user.Uid) == 0 {
		return fmt.Errorf("Unable to delete user because it does not exist: %v", userid)
	}

	deleteUserNQuads := []*api.NQuad{
		{
			Subject:     user.Uid,
			Predicate:   x.Star,
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: x.Star}},
		}}

	mu := &api.Mutation{
		CommitNow: true,
		Del:       deleteUserNQuads,
	}

	_, err = txn.Mutate(ctx, mu)
	if err != nil {
		return fmt.Errorf("Unable to delete user: %v", err)
	}

	glog.Infof("Deleted user with id %v", userid)
	return nil
}

func userLogin(dc *dgo.Dgraph) error {
	userid := LogIn.Conf.GetString("user")
	password := LogIn.Conf.GetString("password")

	if len(userid) == 0 {
		return fmt.Errorf("The user must not be empty.")
	}
	if len(password) == 0 {
		return fmt.Errorf("The password must not be empty.")
	}

	ctx := context.Background()

	err := dc.Login(ctx, userid, password)
	if err != nil {
		return fmt.Errorf("Unable to login:%v", err)
	}
	glog.Infof("Login successfully with jwt:\n%v", dc.GetJwt())
	return nil
}

func queryUser(ctx context.Context, txn *dgo.Txn, userid string) (user *acl.User, err error) {
	query := `
    query search($userid: string){
      user(func: eq(dgraph.xid, $userid)) {
	    uid
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
		return nil, fmt.Errorf("Error while query user with id %s: %v", userid, err)
	}
	user, err = acl.UnmarshallUser(queryResp, "user")
	if err != nil {
		return nil, err
	}
	return user, nil
}

func userMod(dc *dgo.Dgraph) error {
	userid := UserMod.Conf.GetString("user")
	groups := UserMod.Conf.GetString("groups")
	if len(userid) == 0 {
		return fmt.Errorf("The user must not be empty")
	}

	ctx := context.Background()
	txn := dc.NewTxn()
	defer txn.Discard(ctx)

	user, err := queryUser(ctx, txn, userid)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("The user does not exist: %v", userid)
	}

	targetGroupsMap := make(map[string]struct{})
	var exists = struct{}{}
	if len(groups) > 0 {
		for _, g := range strings.Split(groups, ",") {
			targetGroupsMap[g] = exists
		}
	}

	existingGroupsMap := make(map[string]struct{})
	for _, g := range user.Groups {
		existingGroupsMap[g.GroupID] = exists
	}
	newGroups, groupsToBeDeleted := x.CalcDiffs(targetGroupsMap, existingGroupsMap)

	mu := &api.Mutation{
		CommitNow: true,
		Set:       []*api.NQuad{},
		Del:       []*api.NQuad{},
	}

	for _, g := range newGroups {
		glog.Infof("Adding user %v to group %v", userid, g)
		nquad, err := getUserModNQuad(ctx, txn, user.Uid, g)
		if err != nil {
			return fmt.Errorf("error while getting the user mod nquad:%v", err)
		}

		mu.Set = append(mu.Set, nquad)
	}

	for _, g := range groupsToBeDeleted {
		glog.Infof("Deleting user %v from group %v", userid, g)
		nquad, err := getUserModNQuad(ctx, txn, user.Uid, g)
		if err != nil {
			return fmt.Errorf("error while getting the user mod nquad:%v", err)
		}
		mu.Del = append(mu.Del, nquad)
	}
	if len(mu.Del) == 0 && len(mu.Set) == 0 {
		glog.Infof("Nothing nees to be changed for the groups of user:%v", userid)
		return nil
	}

	_, err = txn.Mutate(ctx, mu)
	if err != nil {
		return err
	}

	glog.Infof("Successfully modifed groups for user %v", userid)
	return nil
}

func getUserModNQuad(ctx context.Context, txn *dgo.Txn, useruid string,
	groupid string) (*api.NQuad, error) {
	group, err := queryGroup(txn, ctx, groupid, []string{"uid"})
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, fmt.Errorf("the group does not exist:%v", groupid)
	}

	createUserGroupNQuads := &api.NQuad{
		Subject:   useruid,
		Predicate: "dgraph.user.group",
		ObjectId:  group.Uid,
	}

	return createUserGroupNQuads, nil
}
