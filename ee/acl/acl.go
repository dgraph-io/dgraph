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
	"regexp"
	"strings"
	"time"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/spf13/viper"
)

func getUserAndGroup(conf *viper.Viper) (userId string, groupId string, err error) {
	userId = conf.GetString("user")
	groupId = conf.GetString("group")
	if (len(userId) == 0 && len(groupId) == 0) ||
		(len(userId) != 0 && len(groupId) != 0) {
		return "", "", fmt.Errorf("one of the --user or --group must be specified, but not both")
	}
	return userId, groupId, nil
}

func checkForbiddenOpts(conf *viper.Viper, forbiddenOpts []string) error {
	for _, opt := range forbiddenOpts {
		var isSet bool
		switch conf.Get(opt).(type) {
		case string:
			if opt == "group_list" {
				// handle group_list specially since the default value is not an empty string
				isSet = conf.GetString(opt) != defaultGroupList
			} else {
				isSet = len(conf.GetString(opt)) > 0
			}
		case int:
			isSet = conf.GetInt(opt) > 0
		case bool:
			isSet = conf.GetBool(opt)
		default:
			return fmt.Errorf("unexpected option type for %s", opt)
		}
		if isSet {
			return fmt.Errorf("the option --%s should not be set", opt)
		}
	}

	return nil
}

func add(conf *viper.Viper) error {
	userId, groupId, err := getUserAndGroup(conf)
	if err != nil {
		return err
	}
	password := conf.GetString("password")
	if len(userId) != 0 {
		return userAdd(conf, userId, password)
	}

	// if we are adding a group, then the password should not have been set
	if err := checkForbiddenOpts(conf, []string{"password"}); err != nil {
		return err
	}
	return groupAdd(conf, groupId)
}

func userAdd(conf *viper.Viper, userid string, password string) error {
	dc, cancel, err := getClientWithAdminCtx(conf)
	if err != nil {
		return fmt.Errorf("unable to get admin context:%v", err)
	}
	defer cancel()

	if len(password) == 0 {
		var err error
		password, err = askUserPassword(userid, "New", 2)
		if err != nil {
			return err
		}
	}

	ctx, ctxCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer ctxCancel()
	txn := dc.NewTxn()
	defer func() {
		if err := txn.Discard(ctx); err != nil {
			glog.Errorf("Unable to discard transaction:%v", err)
		}
	}()

	user, err := queryUser(ctx, txn, userid)
	if err != nil {
		return fmt.Errorf("error while querying user:%v", err)
	}
	if user != nil {
		return fmt.Errorf("unable to create user because of conflict: %v", userid)
	}

	createUserNQuads := CreateUserNQuads(userid, password)

	mu := &api.Mutation{
		CommitNow: true,
		Set:       createUserNQuads,
	}

	if _, err := txn.Mutate(ctx, mu); err != nil {
		return fmt.Errorf("unable to create user: %v", err)
	}

	fmt.Printf("Created new user with id %v\n", userid)
	return nil
}

func groupAdd(conf *viper.Viper, groupId string) error {
	dc, cancel, err := getClientWithAdminCtx(conf)
	if err != nil {
		return fmt.Errorf("unable to get admin context: %v", err)
	}
	defer cancel()

	ctx, ctxCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer ctxCancel()
	txn := dc.NewTxn()
	defer func() {
		if err := txn.Discard(ctx); err != nil {
			fmt.Printf("Unable to discard transaction: %v\n", err)
		}
	}()

	group, err := queryGroup(ctx, txn, groupId)
	if err != nil {
		return fmt.Errorf("error while querying group: %v", err)
	}
	if group != nil {
		return fmt.Errorf("group %q already exists", groupId)
	}

	createGroupNQuads := []*api.NQuad{
		{
			Subject:     "_:newgroup",
			Predicate:   "dgraph.xid",
			ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: groupId}},
		},
		{
			Subject:     "_:newgroup",
			Predicate:   "dgraph.type",
			ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: "Group"}},
		},
	}

	mu := &api.Mutation{
		CommitNow: true,
		Set:       createGroupNQuads,
	}
	if _, err = txn.Mutate(ctx, mu); err != nil {
		return fmt.Errorf("unable to create group: %v", err)
	}

	fmt.Printf("Created new group with id %v\n", groupId)
	return nil
}

func del(conf *viper.Viper) error {
	userId, groupId, err := getUserAndGroup(conf)
	if err != nil {
		return err
	}
	if len(userId) != 0 {
		return userOrGroupDel(conf, userId,
			func(ctx context.Context, txn *dgo.Txn, userId string) (AclEntity, error) {
				user, err := queryUser(ctx, txn, userId)
				return user, err
			})
	}
	return userOrGroupDel(conf, groupId,
		func(ctx context.Context, txn *dgo.Txn, groupId string) (AclEntity, error) {
			group, err := queryGroup(ctx, txn, groupId)
			return group, err
		})
}

type AclEntity interface {
	// the implementation of GetUid must check the case that the entity is nil
	// and return an empty string accordingly
	GetUid() string
}

func userOrGroupDel(conf *viper.Viper, userOrGroupId string,
	queryFn func(context.Context, *dgo.Txn, string) (AclEntity, error)) error {
	dc, cancel, err := getClientWithAdminCtx(conf)
	if err != nil {
		return fmt.Errorf("unable to get admin context:%v", err)
	}
	defer cancel()

	ctx, ctxCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer ctxCancel()
	txn := dc.NewTxn()
	defer func() {
		if err := txn.Discard(ctx); err != nil {
			glog.Errorf("Unable to discard transaction:%v", err)
		}
	}()

	entity, err := queryFn(ctx, txn, userOrGroupId)
	if err != nil {
		return err
	}
	if len(entity.GetUid()) == 0 {
		return fmt.Errorf("unable to delete %q since it does not exist",
			userOrGroupId)
	}

	deleteNQuads := []*api.NQuad{
		{
			Subject:     entity.GetUid(),
			Predicate:   x.Star,
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: x.Star}},
		}}

	mu := &api.Mutation{
		CommitNow: true,
		Del:       deleteNQuads,
	}

	if _, err = txn.Mutate(ctx, mu); err != nil {
		return fmt.Errorf("unable to delete %q: %v", userOrGroupId, err)
	}

	fmt.Printf("Successfully deleted %q\n", userOrGroupId)
	return nil
}

func mod(conf *viper.Viper) error {
	userId, _, err := getUserAndGroup(conf)
	if err != nil {
		return err
	}

	if len(userId) != 0 {
		// when modifying the user, some group options are forbidden
		if err := checkForbiddenOpts(conf, []string{"pred", "pred_regex", "perm"}); err != nil {
			return err
		}

		newPassword := conf.GetBool("new_password")
		groupList := conf.GetString("group_list")
		if (newPassword && groupList != defaultGroupList) ||
			(!newPassword && groupList == defaultGroupList) {
			return fmt.Errorf("one of --new_password or --group_list must be provided, but not both")
		}

		if newPassword {
			return changePassword(conf, userId)
		}

		return userMod(conf, userId, groupList)
	}

	// when modifying the group, some user options are forbidden
	if err := checkForbiddenOpts(conf, []string{"group_list", "new_password"}); err != nil {
		return err
	}
	return chMod(conf)
}

// changePassword changes a user's password
func changePassword(conf *viper.Viper, userId string) error {
	// 1. get the dgo client with appropriate access JWT
	dc, cancel, err := getClientWithAdminCtx(conf)
	if err != nil {
		return fmt.Errorf("unable to get dgo client:%v", err)
	}
	defer cancel()

	// 2. get the new password
	newPassword, err := askUserPassword(userId, "New", 2)
	if err != nil {
		return err
	}

	ctx, ctxCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer ctxCancel()
	txn := dc.NewTxn()
	defer func() {
		if err := txn.Discard(ctx); err != nil {
			glog.Errorf("Unable to discard transaction:%v", err)
		}
	}()

	// 3. query the user's current uid
	user, err := queryUser(ctx, txn, userId)
	if err != nil {
		return fmt.Errorf("error while querying user:%v", err)
	}
	if user == nil {
		return fmt.Errorf("user %q does not exist", userId)
	}

	// 4. mutate the user's password
	chPdNQuads := []*api.NQuad{
		{
			Subject:     user.Uid,
			Predicate:   "dgraph.password",
			ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: newPassword}},
		}}
	mu := &api.Mutation{
		CommitNow: true,
		Set:       chPdNQuads,
	}
	if _, err := txn.Mutate(ctx, mu); err != nil {
		return fmt.Errorf("unable to change password for user %v: %v", userId, err)
	}
	fmt.Printf("Successfully changed password for %v\n", userId)
	return nil
}

func userMod(conf *viper.Viper, userId string, groups string) error {
	dc, cancel, err := getClientWithAdminCtx(conf)
	if err != nil {
		return fmt.Errorf("unable to get admin context:%v", err)
	}
	defer cancel()

	ctx, ctxCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer ctxCancel()
	txn := dc.NewTxn()
	defer func() {
		if err := txn.Discard(ctx); err != nil {
			fmt.Printf("Unable to discard transaction: %v\n", err)
		}
	}()

	user, err := queryUser(ctx, txn, userId)
	if err != nil {
		return fmt.Errorf("error while querying user: %v", err)
	}
	if user == nil {
		return fmt.Errorf("user %q does not exist", userId)
	}

	targetGroupsMap := make(map[string]struct{})
	if len(groups) > 0 {
		for _, g := range strings.Split(groups, ",") {
			targetGroupsMap[g] = struct{}{}
		}
	}

	existingGroupsMap := make(map[string]struct{})
	for _, g := range user.Groups {
		existingGroupsMap[g.GroupID] = struct{}{}
	}
	newGroups, groupsToBeDeleted := x.Diff(targetGroupsMap, existingGroupsMap)

	mu := &api.Mutation{
		CommitNow: true,
		Set:       []*api.NQuad{},
		Del:       []*api.NQuad{},
	}

	for _, g := range newGroups {
		fmt.Printf("Adding user %v to group %v\n", userId, g)
		nquad, err := getUserModNQuad(ctx, txn, user.Uid, g)
		if err != nil {
			return err
		}
		mu.Set = append(mu.Set, nquad)
	}

	for _, g := range groupsToBeDeleted {
		fmt.Printf("Deleting user %v from group %v\n", userId, g)
		nquad, err := getUserModNQuad(ctx, txn, user.Uid, g)
		if err != nil {
			return err
		}
		mu.Del = append(mu.Del, nquad)
	}
	if len(mu.Del) == 0 && len(mu.Set) == 0 {
		fmt.Printf("Nothing needs to be changed for the groups of user: %v\n", userId)
		return nil
	}

	if _, err := txn.Mutate(ctx, mu); err != nil {
		return fmt.Errorf("error while mutating the group: %+v", err)
	}
	fmt.Printf("Successfully modified groups for user %v.\n", userId)
	fmt.Println("The latest info is:")
	return queryAndPrintUser(ctx, dc.NewReadOnlyTxn(), userId)
}

func chMod(conf *viper.Viper) error {
	groupId := conf.GetString("group")
	predicate := conf.GetString("pred")
	predRegex := conf.GetString("pred_regex")
	perm := conf.GetInt("perm")
	switch {
	case len(groupId) == 0:
		return fmt.Errorf("the groupid must not be empty")
	case len(predicate) > 0 && len(predRegex) > 0:
		return fmt.Errorf("one of --pred or --pred_regex must be specified, but not both")
	case len(predicate) == 0 && len(predRegex) == 0:
		return fmt.Errorf("one of --pred or --pred_regex must be specified, but not both")
	case perm > 7:
		return fmt.Errorf("the perm value must be less than or equal to 7, "+
			"the provided value is %d", perm)
	case len(predRegex) > 0:
		// make sure the predRegex can be compiled as a regex
		if _, err := regexp.Compile(predRegex); err != nil {
			return fmt.Errorf("unable to compile %v as a regular expression: %v",
				predRegex, err)
		}
	}

	dc, cancel, err := getClientWithAdminCtx(conf)
	if err != nil {
		return fmt.Errorf("unable to get admin context: %v", err)
	}
	defer cancel()

	ctx, ctxCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer ctxCancel()
	txn := dc.NewTxn()
	defer func() {
		if err := txn.Discard(ctx); err != nil {
			fmt.Printf("Unable to discard transaction: %v\n", err)
		}
	}()

	group, err := queryGroup(ctx, txn, groupId, "dgraph.group.acl")
	if err != nil {
		return fmt.Errorf("error while querying group: %v\n", err)
	}
	if group == nil || len(group.Uid) == 0 {
		return fmt.Errorf("unable to change permission for group because it does not exist: %v",
			groupId)
	}

	var currentAcls []Acl
	if len(group.Acls) != 0 {
		if err := json.Unmarshal([]byte(group.Acls), &currentAcls); err != nil {
			return fmt.Errorf("unable to unmarshal the acls associated with the group %v: %v",
				groupId, err)
		}
	}

	var newAcl Acl
	if len(predicate) > 0 {
		newAcl = Acl{
			Predicate: predicate,
			Perm:      int32(perm),
		}
	} else {
		newAcl = Acl{
			Regex: predRegex,
			Perm:  int32(perm),
		}
	}
	newAcls, updated := updateAcl(currentAcls, newAcl)
	if !updated {
		fmt.Printf("Nothing needs to be changed for the permission of group: %v\n", groupId)
		return nil
	}

	newAclBytes, err := json.Marshal(newAcls)
	if err != nil {
		return fmt.Errorf("unable to marshal the updated acls: %v", err)
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
	fmt.Printf("Successfully changed permission for group %v on predicate %v to %v\n",
		groupId, predicate, perm)
	fmt.Println("The latest info is:")
	return queryAndPrintGroup(ctx, dc.NewReadOnlyTxn(), groupId)
}

func queryUser(ctx context.Context, txn *dgo.Txn, userid string) (user *User, err error) {
	query := `
    query search($userid: string){
      user(func: eq(dgraph.xid, $userid)) @filter(type(User)) {
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
		return nil, fmt.Errorf("group %q does not exist", groupId)
	}

	createUserGroupNQuads := &api.NQuad{
		Subject:   userId,
		Predicate: "dgraph.user.group",
		ObjectId:  group.Uid,
	}

	return createUserGroupNQuads, nil
}

func queryGroup(ctx context.Context, txn *dgo.Txn, groupid string,
	fields ...string) (group *Group, err error) {

	// write query header
	query := fmt.Sprintf(`query search($groupid: string){
        group(func: eq(dgraph.xid, $groupid)) @filter(type(Group)) {
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

func queryAndPrintUser(ctx context.Context, txn *dgo.Txn, userId string) error {
	user, err := queryUser(ctx, txn, userId)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("The user %q does not exist.\n", userId)
	}

	fmt.Printf("User  : %s\n", userId)
	fmt.Printf("UID   : %s\n", user.Uid)
	for _, group := range user.Groups {
		fmt.Printf("Group : %-5s\n", group.GroupID)
	}
	return nil
}

func queryAndPrintGroup(ctx context.Context, txn *dgo.Txn, groupId string) error {
	group, err := queryGroup(ctx, txn, groupId, "dgraph.xid", "~dgraph.user.group{dgraph.xid}",
		"dgraph.group.acl")
	if err != nil {
		return err
	}
	if group == nil {
		return fmt.Errorf("The group %q does not exist.\n", groupId)
	}
	fmt.Printf("Group: %s\n", groupId)
	fmt.Printf("UID  : %s\n", group.Uid)
	fmt.Printf("ID   : %s\n", group.GroupID)

	var userNames []string
	for _, user := range group.Users {
		userNames = append(userNames, user.UserID)
	}
	fmt.Printf("Users: %s\n", strings.Join(userNames, " "))

	var acls []Acl
	if len(group.Acls) != 0 {
		if err := json.Unmarshal([]byte(group.Acls), &acls); err != nil {
			return fmt.Errorf("unable to unmarshal the acls associated with the group %v: %v",
				groupId, err)
		}

		for _, acl := range acls {
			fmt.Printf("ACL  : %v\n", acl)
		}
	}
	return nil
}

func info(conf *viper.Viper) error {
	userId, groupId, err := getUserAndGroup(conf)
	if err != nil {
		return err
	}

	dc, cancel, err := getClientWithAdminCtx(conf)
	defer cancel()
	if err != nil {
		return fmt.Errorf("unable to get admin context: %v\n", err)
	}

	ctx, ctxCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer ctxCancel()
	txn := dc.NewTxn()
	defer func() {
		if err := txn.Discard(ctx); err != nil {
			fmt.Printf("Unable to discard transaction: %v\n", err)
		}
	}()

	if len(userId) != 0 {
		return queryAndPrintUser(ctx, txn, userId)
	}

	return queryAndPrintGroup(ctx, txn, groupId)
}
