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
	"encoding/json"
	"fmt"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
)

func ValidateLoginRequest(request *api.LogInRequest) error {
	if request == nil {
		return fmt.Errorf("the request should not be nil")
	}
	if len(request.Userid) == 0 {
		return fmt.Errorf("the userid should not be empty")
	}
	if len(request.Password) == 0 {
		return fmt.Errorf("the password should not be empty")
	}
	return nil
}

func ToJwtGroups(groups []Group) []JwtGroup {
	if len(groups) == 0 {
		// the user does not have any groups
		return nil
	}

	jwtGroups := make([]JwtGroup, 0, len(groups))
	for _, g := range groups {
		jwtGroups = append(jwtGroups, JwtGroup{
			Group: g.GroupID,
		})
	}
	return jwtGroups
}

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
		return nil, fmt.Errorf("Unable to unmarshal the query user response for user:%v", err)
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

	err = json.Unmarshal(input, &m)
	if err != nil {
		glog.Errorf("Unable to unmarshal the query group response:%v", err)
		return nil, err
	}
	groups := m[groupKey]
	if len(groups) == 0 {
		// the group does not exist
		return nil, nil
	}
	if len(groups) > 1 {
		return nil, x.Errorf("Found multiple groups: %s", input)
	}
	return &groups[0], nil
}

type JwtGroup struct {
	Group string
}
