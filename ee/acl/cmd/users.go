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
	"time"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/ee/acl"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/spf13/viper"
)

func userAdd(conf *viper.Viper) error {
	userid := conf.GetString("user")
	password := conf.GetString("password")

	if len(userid) == 0 {
		return fmt.Errorf("the user must not be empty")
	}
	if len(password) == 0 {
		return fmt.Errorf("the password must not be empty")
	}

	dc := getDgraphClient(conf)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	txn := dc.NewTxn()
	defer txn.Discard(ctx)

	user, err := queryUser(ctx, txn, userid)
	if err != nil {
		return fmt.Errorf("error while querying user:%v", err)
	}
	if user != nil {
		return fmt.Errorf("unable to create user because of conflict: %v", userid)
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

	if _, err := txn.Mutate(ctx, mu); err != nil {
		return fmt.Errorf("unable to create user: %v", err)
	}

	glog.Infof("Created new user with id %v", userid)
	return nil
}

func userDel(conf *viper.Viper) error {
	userid := conf.GetString("user")
	// validate the userid
	if len(userid) == 0 {
		return fmt.Errorf("the user id should not be empty")
	}

	dc := getDgraphClient(conf)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	txn := dc.NewTxn()
	defer txn.Discard(ctx)

	user, err := queryUser(ctx, txn, userid)
	if err != nil {
		return fmt.Errorf("error while querying user:%v", err)
	}

	if user == nil || len(user.Uid) == 0 {
		return fmt.Errorf("unable to delete user because it does not exist: %v", userid)
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

	if _, err = txn.Mutate(ctx, mu); err != nil {
		return fmt.Errorf("unable to delete user: %v", err)
	}

	glog.Infof("Deleted user with id %v", userid)
	return nil
}

func userLogin(conf *viper.Viper) error {
	userid := conf.GetString("user")
	password := conf.GetString("password")

	if len(userid) == 0 {
		return fmt.Errorf("the user must not be empty")
	}
	if len(password) == 0 {
		return fmt.Errorf("the password must not be empty")
	}

	dc := getDgraphClient(conf)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	txn := dc.NewTxn()
	defer txn.Discard(ctx)

	if err := dc.Login(ctx, userid, password); err != nil {
		return fmt.Errorf("unable to login:%v", err)
	}
	updatedContext := dc.GetContext(ctx)
	glog.Infof("Login successfully with jwt:\n%v", updatedContext.Value("jwt"))
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
		return nil, fmt.Errorf("error while query user with id %s: %v", userid, err)
	}
	user, err = acl.UnmarshalUser(queryResp, "user")
	if err != nil {
		return nil, err
	}
	return user, nil
}

func userMod(conf *viper.Viper) error {
	userId := conf.GetString("user")
	groups := conf.GetString("groups")
	if len(userId) == 0 {
		return fmt.Errorf("the user must not be empty")
	}

	dc := getDgraphClient(conf)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	txn := dc.NewTxn()
	defer txn.Discard(ctx)

	user, err := queryUser(ctx, txn, userId)
	if err != nil {
		return fmt.Errorf("error while querying user:%v", err)
	}
	if user == nil {
		return fmt.Errorf("the user does not exist: %v", userId)
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
	newGroups, groupsToBeDeleted := x.Diff(targetGroupsMap, existingGroupsMap)

	mu := &api.Mutation{
		CommitNow: true,
		Set:       []*api.NQuad{},
		Del:       []*api.NQuad{},
	}

	for _, g := range newGroups {
		glog.Infof("Adding user %v to group %v", userId, g)
		nquad, err := getUserModNQuad(ctx, txn, user.Uid, g)
		if err != nil {
			return fmt.Errorf("error while getting the user mod nquad:%v", err)
		}
		mu.Set = append(mu.Set, nquad)
	}

	for _, g := range groupsToBeDeleted {
		glog.Infof("Deleting user %v from group %v", userId, g)
		nquad, err := getUserModNQuad(ctx, txn, user.Uid, g)
		if err != nil {
			return fmt.Errorf("error while getting the user mod nquad:%v", err)
		}
		mu.Del = append(mu.Del, nquad)
	}
	if len(mu.Del) == 0 && len(mu.Set) == 0 {
		glog.Infof("Nothing needs to be changed for the groups of user:%v", userId)
		return nil
	}

	if _, err := txn.Mutate(ctx, mu); err != nil {
		return fmt.Errorf("error while mutating the group:%+v", err)
	}
	glog.Infof("Successfully modified groups for user %v", userId)
	return nil
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
