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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/spf13/viper"
)

func groupAdd(conf *viper.Viper) error {
	groupId := conf.GetString("group")
	if len(groupId) == 0 {
		return fmt.Errorf("the group id should not be empty")
	}

	dc, clean, err := getClientWithAdminCtx(conf)
	defer clean()
	if err != nil {
		return fmt.Errorf("unable to get admin context:%v", err)
	}

	ctx := context.Background()
	txn := dc.NewTxn()
	defer func() {
		if err := txn.Discard(ctx); err != nil {
			glog.Errorf("Unable to discard transaction:%v", err)
		}
	}()

	group, err := queryGroup(ctx, txn, groupId)
	if err != nil {
		return fmt.Errorf("error while querying group:%v", err)
	}
	if group != nil {
		return fmt.Errorf("the group with id %v already exists", groupId)
	}

	createGroupNQuads := []*api.NQuad{
		{
			Subject:     "_:newgroup",
			Predicate:   "dgraph.xid",
			ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: groupId}},
		},
	}

	mu := &api.Mutation{
		CommitNow: true,
		Set:       createGroupNQuads,
	}
	if _, err = txn.Mutate(ctx, mu); err != nil {
		return fmt.Errorf("unable to create group: %v", err)
	}

	glog.Infof("Created new group with id %v", groupId)
	return nil
}

func groupDel(conf *viper.Viper) error {
	groupId := conf.GetString("group")
	if len(groupId) == 0 {
		return fmt.Errorf("the group id should not be empty")
	}

	dc, clean, err := getClientWithAdminCtx(conf)
	defer clean()
	if err != nil {
		return fmt.Errorf("unable to get admin context:%v", err)
	}

	ctx := context.Background()
	txn := dc.NewTxn()
	defer func() {
		if err := txn.Discard(ctx); err != nil {
			glog.Errorf("Unable to discard transaction:%v", err)
		}
	}()

	group, err := queryGroup(ctx, txn, groupId)
	if err != nil {
		return fmt.Errorf("error while querying group:%v", err)
	}
	if group == nil || len(group.Uid) == 0 {
		return fmt.Errorf("unable to delete group because it does not exist: %v", groupId)
	}

	deleteGroupNQuads := []*api.NQuad{
		{
			Subject:     group.Uid,
			Predicate:   x.Star,
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: x.Star}},
		},
	}
	mu := &api.Mutation{
		CommitNow: true,
		Del:       deleteGroupNQuads,
	}
	if _, err := txn.Mutate(ctx, mu); err != nil {
		return fmt.Errorf("unable to delete group: %v", err)
	}

	glog.Infof("Deleted group with id %v", groupId)
	return nil
}

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
		glog.Errorf("Error while query group with id %s: %v", groupid, err)
		return nil, err
	}
	group, err = UnmarshalGroup(queryResp.GetJson(), "group")
	if err != nil {
		return nil, err
	}
	return group, nil
}

type Acl struct {
	Predicate string `json:"predicate"`
	Perm      int32  `json:"perm"`
}

func chMod(conf *viper.Viper) error {
	groupId := conf.GetString("group")
	predicate := conf.GetString("pred")
	perm := conf.GetInt("perm")
	if len(groupId) == 0 {
		return fmt.Errorf("the groupid must not be empty")
	}
	if len(predicate) == 0 {
		return fmt.Errorf("the predicate must not be empty")
	}

	dc, clean, err := getClientWithAdminCtx(conf)
	defer clean()
	if err != nil {
		return fmt.Errorf("unable to get admin context:%v", err)
	}

	ctx := context.Background()
	txn := dc.NewTxn()
	defer func() {
		if err := txn.Discard(ctx); err != nil {
			glog.Errorf("Unable to discard transaction:%v", err)
		}
	}()

	group, err := queryGroup(ctx, txn, groupId, "dgraph.group.acl")
	if err != nil {
		return fmt.Errorf("error while querying group:%v", err)
	}
	if group == nil || len(group.Uid) == 0 {
		return fmt.Errorf("unable to change permission for group because it does not exist: %v",
			groupId)
	}

	var currentAcls []Acl
	if len(group.Acls) != 0 {
		if err := json.Unmarshal([]byte(group.Acls), &currentAcls); err != nil {
			return fmt.Errorf("unable to unmarshal the acls associated with the group %v:%v",
				groupId, err)
		}
	}

	newAcls, updated := updateAcl(currentAcls, Acl{
		Predicate: predicate,
		Perm:      int32(perm),
	})
	if !updated {
		glog.Infof("Nothing needs to be changed for the permission of group:%v", groupId)
		return nil
	}

	newAclBytes, err := json.Marshal(newAcls)
	if err != nil {
		return fmt.Errorf("unable to marshal the updated acls:%v", err)
	}

	chModNQuads := &api.NQuad{
		Subject:     group.Uid,
		Predicate:   "dgraph.group.acl",
		ObjectValue: &api.Value{Val: &api.Value_BytesVal{BytesVal: newAclBytes}},
	}
	mu := &api.Mutation{
		CommitNow: true,
		Set:       []*api.NQuad{chModNQuads},
	}

	if _, err = txn.Mutate(ctx, mu); err != nil {
		return fmt.Errorf("unable to change mutations for the group %v on predicate %v: %v",
			groupId, predicate, err)
	}
	glog.Infof("Successfully changed permission for group %v on predicate %v to %v",
		groupId, predicate, perm)
	return nil
}

// returns whether the existing acls slice is changed
func updateAcl(acls []Acl, newAcl Acl) ([]Acl, bool) {
	for idx, aclEntry := range acls {
		if aclEntry.Predicate == newAcl.Predicate {
			if aclEntry.Perm == newAcl.Perm {
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
