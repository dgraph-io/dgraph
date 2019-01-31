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

package acl

import (
	"context"
	"encoding/json"
	"fmt"
	"syscall"
	"time"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/spf13/viper"
	"golang.org/x/crypto/ssh/terminal"
)

func GetGroupIDs(groups []Group) []string {
	if len(groups) == 0 {
		// the user does not have any groups
		return nil
	}

	jwtGroups := make([]string, 0, len(groups))
	for _, g := range groups {
		jwtGroups = append(jwtGroups, g.GroupID)
	}
	return jwtGroups
}

type Operation struct {
	Code int32
	Name string
}

var (
	Read = &Operation{
		Code: 4,
		Name: "Read",
	}
	Write = &Operation{
		Code: 2,
		Name: "Write",
	}
	Modify = &Operation{
		Code: 1,
		Name: "Modify",
	}
)

type User struct {
	Uid           string  `json:"uid"`
	UserID        string  `json:"dgraph.xid"`
	Password      string  `json:"dgraph.password"`
	PasswordMatch bool    `json:"password_match"`
	Groups        []Group `json:"dgraph.user.group"`
}

// Extract the first User pointed by the userKey in the query response
func UnmarshalUser(resp *api.Response, userKey string) (user *User, err error) {
	m := make(map[string][]User)

	err = json.Unmarshal(resp.GetJson(), &m)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal the query user response:%v", err)
	}
	users := m[userKey]
	if len(users) == 0 {
		// the user does not exist
		return nil, nil
	}
	if len(users) > 1 {
		return nil, x.Errorf("Found multiple users: %s", resp.GetJson())
	}
	return &users[0], nil
}

// parse the response and check existing of the uid
type Group struct {
	Uid     string `json:"uid"`
	GroupID string `json:"dgraph.xid"`
	Users   []User `json:"~dgraph.user.group"`
	Acls    string `json:"dgraph.group.acl"`
}

// Extract the first User pointed by the userKey in the query response
func UnmarshalGroup(input []byte, groupKey string) (group *Group, err error) {
	m := make(map[string][]Group)

	if err = json.Unmarshal(input, &m); err != nil {
		glog.Errorf("Unable to unmarshal the query group response:%v", err)
		return nil, err
	}
	groups := m[groupKey]
	if len(groups) == 0 {
		// the group does not exist
		return nil, nil
	}
	if len(groups) > 1 {
		return nil, fmt.Errorf("found multiple groups: %s", input)
	}

	return &groups[0], nil
}

// Extract a sequence of groups from the input
func UnmarshalGroups(input []byte, groupKey string) (group []Group, err error) {
	m := make(map[string][]Group)

	if err = json.Unmarshal(input, &m); err != nil {
		glog.Errorf("Unable to unmarshal the query group response:%v", err)
		return nil, err
	}
	groups := m[groupKey]
	return groups, nil
}

type JwtGroup struct {
	Group string
}

func askUserPassword(userid string, times int) (string, error) {
	x.AssertTrue(times == 1 || times == 2)
	// ask for the user's password
	fmt.Printf("Password for %v:", userid)
	pd, err := terminal.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", fmt.Errorf("error while reading password:%v", err)
	}
	password := string(pd)

	if times == 2 {
		fmt.Printf("\nRetype password for %v:", userid)
		pd2, err := terminal.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return "", fmt.Errorf("error while reading password:%v", err)
		}
		fmt.Println()

		password2 := string(pd2)
		if password2 != password {
			return "", fmt.Errorf("the two typed passwords do not match")
		}
	}
	return password, nil
}

func getClientWithUserCtx(userid string, passwordOpt string, conf *viper.Viper) (*dgo.Dgraph,
	CloseFunc, error) {
	password := conf.GetString(passwordOpt)
	if len(password) == 0 {
		var err error
		password, err = askUserPassword(userid, 1)
		if err != nil {
			return nil, nil, err
		}
	}

	dc, closeClient := getDgraphClient(conf)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	cleanFunc := func() {
		cancel()
		closeClient()
	}

	if err := dc.Login(ctx, userid, password); err != nil {
		return dc, cleanFunc, fmt.Errorf("unable to login to the %v account:%v", userid, err)
	}
	glog.Infof("login successfully to the %v account", userid)
	// update the context so that it has the admin jwt token
	return dc, cleanFunc, nil
}

func getClientWithAdminCtx(conf *viper.Viper) (*dgo.Dgraph, CloseFunc, error) {
	return getClientWithUserCtx(x.GrootId, gPassword, conf)
}
